package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"onessh/internal/audit"
	"onessh/internal/store"

	"github.com/spf13/cobra"
)

const (
	sshConfigManagedStart = "# >>> onessh managed start >>>"
	sshConfigManagedEnd   = "# <<< onessh managed end <<<"
)

var sshAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type sshConfigEntry struct {
	Alias       string
	HostName    string
	User        string
	Port        int
	ProxyJump   string
	IdentityFile string
	Env         map[string]string
}

func newSSHConfigCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sshconfig",
		Short: "Import/export ~/.ssh/config entries",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newSSHConfigExportCmd(opts),
		newSSHConfigImportCmd(opts),
	)
	return cmd
}

func newSSHConfigExportCmd(opts *rootOptions) *cobra.Command {
	var file string
	var stdout bool

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export hosts to ~/.ssh/config managed block",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}
			cfg, pass, err := loadConfig(opts, repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			rendered := renderSSHConfigManagedBlock(cfg)
			if stdout {
				_, err := cmd.OutOrStdout().Write([]byte(rendered))
				return err
			}

			targetPath, err := resolveSSHConfigPath(file)
			if err != nil {
				return err
			}

			var existing string
			if raw, err := os.ReadFile(targetPath); err == nil {
				existing = string(raw)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("read ssh config %s: %w", targetPath, err)
			}

			next := upsertSSHConfigManagedBlock(existing, rendered)
			if err := writeTextFileAtomic(targetPath, []byte(next), 0o600); err != nil {
				return err
			}

			al, _ := opts.auditLogger()
			if al != nil {
				defer al.Close()
			}
			al.Log(audit.Entry{Action: "sshconfig.export", Detail: targetPath})

			fmt.Fprintf(cmd.OutOrStdout(), "✔ ssh config exported to %s\n", targetPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "SSH config file path (default ~/.ssh/config)")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Print managed block to stdout")
	return cmd
}

func newSSHConfigImportCmd(opts *rootOptions) *cobra.Command {
	var file string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import compatible Host entries from ~/.ssh/config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := resolveSSHConfigPath(file)
			if err != nil {
				return err
			}

			entries, err := parseSSHConfigFile(targetPath)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No importable Host entries found.")
				return nil
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}
			cfg, pass, err := loadConfig(opts, repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			imported := 0
			updated := 0
			skipped := 0

			for _, entry := range entries {
				if !sshAliasPattern.MatchString(entry.Alias) {
					skipped++
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: unsupported alias\n", entry.Alias)
					continue
				}
				_, exists := cfg.Hosts[entry.Alias]
				if exists && !overwrite {
					skipped++
					continue
				}
				if strings.TrimSpace(entry.User) == "" {
					skipped++
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: missing User\n", entry.Alias)
					continue
				}

				userRef, ok := ensureUserProfileFromSSHEntry(&cfg, entry)
				if !ok {
					skipped++
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: missing IdentityFile for user %s\n", entry.Alias, entry.User)
					continue
				}

				hostName := strings.TrimSpace(entry.HostName)
				if hostName == "" {
					hostName = entry.Alias
				}
				port := entry.Port
				if port <= 0 {
					port = 22
				}

				cfg.Hosts[entry.Alias] = store.HostConfig{
					Host:      hostName,
					UserRef:   userRef,
					Port:      port,
					ProxyJump: strings.TrimSpace(entry.ProxyJump),
					Env:       cloneStringMap(entry.Env),
				}
				if exists {
					updated++
				} else {
					imported++
				}
			}

			if imported == 0 && updated == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No entries imported.")
				return nil
			}
			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			al, _ := opts.auditLogger()
			if al != nil {
				defer al.Close()
			}
			al.Log(audit.Entry{Action: "sshconfig.import", Detail: fmt.Sprintf("imported=%d updated=%d skipped=%d", imported, updated, skipped)})

			fmt.Fprintf(cmd.OutOrStdout(), "✔ imported=%d updated=%d skipped=%d\n", imported, updated, skipped)
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "SSH config file path (default ~/.ssh/config)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing host aliases in onessh")
	return cmd
}

func resolveSSHConfigPath(customPath string) (string, error) {
	if strings.TrimSpace(customPath) != "" {
		return expandTilde(customPath)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".ssh", "config"), nil
}

func renderSSHConfigManagedBlock(cfg store.PlainConfig) string {
	aliases := make([]string, 0, len(cfg.Hosts))
	for alias := range cfg.Hosts {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	var b strings.Builder
	b.WriteString(sshConfigManagedStart)
	b.WriteString("\n")
	b.WriteString("# generated by onessh\n")

	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		userName, auth, err := resolveHostIdentity(cfg, host)
		if err != nil {
			continue
		}

		port := host.Port
		if port <= 0 {
			port = 22
		}

		b.WriteString("\n")
		b.WriteString("Host ")
		b.WriteString(alias)
		b.WriteString("\n")
		b.WriteString("  HostName ")
		b.WriteString(host.Host)
		b.WriteString("\n")
		if userName != "" {
			b.WriteString("  User ")
			b.WriteString(userName)
			b.WriteString("\n")
		}
		b.WriteString("  Port ")
		b.WriteString(strconv.Itoa(port))
		b.WriteString("\n")
		if host.ProxyJump != "" {
			b.WriteString("  ProxyJump ")
			b.WriteString(host.ProxyJump)
			b.WriteString("\n")
		}
		if strings.EqualFold(auth.Type, "key") && strings.TrimSpace(auth.KeyPath) != "" {
			b.WriteString("  IdentityFile ")
			b.WriteString(auth.KeyPath)
			b.WriteString("\n")
		}

		keys := sortedStringMapKeys(host.Env)
		for _, key := range keys {
			assignment := key + "=" + host.Env[key]
			b.WriteString("  SetEnv ")
			b.WriteString(sshConfigValueToken(assignment))
			b.WriteString("\n")
		}
	}

	b.WriteString(sshConfigManagedEnd)
	b.WriteString("\n")
	return b.String()
}

func upsertSSHConfigManagedBlock(existing, managedBlock string) string {
	start := strings.Index(existing, sshConfigManagedStart)
	end := strings.Index(existing, sshConfigManagedEnd)
	if start >= 0 && end > start {
		end += len(sshConfigManagedEnd)
		prefix := strings.TrimRight(existing[:start], "\n")
		suffix := strings.TrimLeft(existing[end:], "\n")
		var b strings.Builder
		if prefix != "" {
			b.WriteString(prefix)
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimRight(managedBlock, "\n"))
		b.WriteString("\n")
		if suffix != "" {
			b.WriteString("\n")
			b.WriteString(suffix)
			if !strings.HasSuffix(suffix, "\n") {
				b.WriteString("\n")
			}
		}
		return b.String()
	}

	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return managedBlock
	}
	return trimmed + "\n\n" + managedBlock
}

func writeTextFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".onessh-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempName := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}

	if err := tempFile.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file for %s: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("rename temp file for %s: %w", path, err)
	}
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("chmod file %s: %w", path, err)
	}
	return nil
}

func parseSSHConfigFile(path string) ([]sshConfigEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read ssh config %s: %w", path, err)
	}
	return parseSSHConfig(string(raw)), nil
}

func parseSSHConfig(content string) []sshConfigEntry {
	type block struct {
		aliases     []string
		hostName    string
		user        string
		port        int
		proxyJump   string
		identityFile string
		env         map[string]string
	}

	var current *block
	entriesByAlias := map[string]sshConfigEntry{}

	flush := func() {
		if current == nil || len(current.aliases) == 0 {
			return
		}
		for _, alias := range current.aliases {
			entry := sshConfigEntry{
				Alias:       alias,
				HostName:    current.hostName,
				User:        current.user,
				Port:        current.port,
				ProxyJump:   current.proxyJump,
				IdentityFile: current.identityFile,
				Env:         cloneStringMap(current.env),
			}
			if strings.TrimSpace(entry.HostName) == "" {
				entry.HostName = alias
			}
			if entry.Port <= 0 {
				entry.Port = 22
			}
			entriesByAlias[alias] = entry
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := splitSSHConfigLine(line)
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			flush()
			aliases := parseHostAliases(value)
			current = &block{
				aliases: aliases,
				port:    22,
				env:     map[string]string{},
			}
		default:
			if current == nil {
				continue
			}
			switch strings.ToLower(key) {
			case "hostname":
				current.hostName = trimSSHValue(value)
			case "user":
				current.user = trimSSHValue(value)
			case "port":
				if p, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && p > 0 {
					current.port = p
				}
			case "proxyjump":
				current.proxyJump = trimSSHValue(value)
			case "identityfile":
				current.identityFile = trimSSHValue(value)
			case "setenv":
				for _, token := range strings.Fields(value) {
					parts := strings.SplitN(token, "=", 2)
					if len(parts) != 2 {
						continue
					}
					key := strings.TrimSpace(parts[0])
					if !envKeyPattern.MatchString(key) {
						continue
					}
					current.env[key] = trimSSHValue(parts[1])
				}
			}
		}
	}
	flush()

	aliases := make([]string, 0, len(entriesByAlias))
	for alias := range entriesByAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	entries := make([]sshConfigEntry, 0, len(aliases))
	for _, alias := range aliases {
		entries = append(entries, entriesByAlias[alias])
	}
	return entries
}

func splitSSHConfigLine(line string) (string, string, bool) {
	if line == "" {
		return "", "", false
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", false
	}
	key := parts[0]
	value := strings.TrimSpace(line[len(key):])
	return key, value, true
}

func parseHostAliases(raw string) []string {
	fields := strings.Fields(raw)
	aliases := make([]string, 0, len(fields))
	for _, field := range fields {
		alias := trimSSHValue(field)
		if alias == "" || strings.ContainsAny(alias, "*?!") {
			continue
		}
		aliases = append(aliases, alias)
	}
	return aliases
}

func trimSSHValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"`)
	trimmed = strings.Trim(trimmed, `'`)
	return trimmed
}

func sshConfigValueToken(value string) string {
	if value == "" {
		return "\"\""
	}
	if strings.ContainsAny(value, " \t\"#") {
		escaped := strings.ReplaceAll(value, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return "\"" + escaped + "\""
	}
	return value
}

func ensureUserProfileFromSSHEntry(cfg *store.PlainConfig, entry sshConfigEntry) (string, bool) {
	userName := strings.TrimSpace(entry.User)
	if userName == "" {
		return "", false
	}

	identity := strings.TrimSpace(entry.IdentityFile)
	if identity == "" {
		if alias := findUserAliasByName(cfg.Users, userName); alias != "" {
			return alias, true
		}
		return "", false
	}

	auth := store.AuthConfig{
		Type:    "key",
		KeyPath: identity,
	}
	for alias, userCfg := range cfg.Users {
		if strings.EqualFold(strings.TrimSpace(userCfg.Name), userName) &&
			strings.EqualFold(normalizeAuthType(userCfg.Auth.Type), "key") &&
			strings.TrimSpace(userCfg.Auth.KeyPath) == identity {
			return alias, true
		}
	}

	base := normalizeUserAlias(userName)
	if base == "" {
		base = "user"
	}
	candidate := base
	for i := 2; ; i++ {
		existing, exists := cfg.Users[candidate]
		if !exists {
			cfg.Users[candidate] = store.UserConfig{Name: userName, Auth: auth}
			return candidate, true
		}
		if strings.EqualFold(strings.TrimSpace(existing.Name), userName) &&
			strings.EqualFold(normalizeAuthType(existing.Auth.Type), "key") &&
			strings.TrimSpace(existing.Auth.KeyPath) == identity {
			return candidate, true
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}
