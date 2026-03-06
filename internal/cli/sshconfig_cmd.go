package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

const (
	sshConfigManagedStart = "# >>> onessh managed start >>>"
	sshConfigManagedEnd   = "# <<< onessh managed end <<<"
)

var sshAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type sshConfigEntry struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	ProxyJump    string
	IdentityFile string
	Env          map[string]string
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
