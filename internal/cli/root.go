package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"time"

	"onessh/internal/store"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type rootOptions struct {
	configPath string
	cacheTTL   time.Duration
	noCache    bool
}

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "onessh [host]",
		Short:         "Manage and connect SSH hosts from encrypted config",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runConnect(cmd, opts, args[0])
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to encrypted config file")
	rootCmd.PersistentFlags().DurationVar(&opts.cacheTTL, "cache-ttl", 10*time.Minute, "Master password cache duration")
	rootCmd.PersistentFlags().BoolVar(&opts.noCache, "no-cache", false, "Disable master password cache")

	rootCmd.AddCommand(
		newInitCmd(opts),
		newAddCmd(opts),
		newUpdateCmd(opts),
		newRmCmd(opts),
		newListCmd(opts),
		newDumpCmd(opts),
		newConnectCmd(opts),
		newUserCmd(opts),
		newLogoutCmd(opts),
	)

	return rootCmd
}

func newInitCmd(opts *rootOptions) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize encrypted OneSSH config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			if repo.Exists() && !force {
				return fmt.Errorf("config already exists at %s (use --force to overwrite)", repo.Path)
			}

			pass1, err := promptRequiredPassword("Enter master password: ")
			if err != nil {
				return err
			}
			defer wipe(pass1)

			pass2, err := promptRequiredPassword("Confirm master password: ")
			if err != nil {
				return err
			}
			defer wipe(pass2)

			if !bytes.Equal(pass1, pass2) {
				return errors.New("passwords do not match")
			}

			cfg := store.NewPlainConfig()
			if err := repo.Save(cfg, pass1); err != nil {
				return err
			}
			if cache, err := newPassphraseCache(repo.Path, opts.cacheTTL, opts.noCache); err == nil {
				_ = cache.Set(pass1)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ onessh configuration initialized: %s\n", repo.Path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing encrypted config")
	return cmd
}

func newAddCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <host-alias>",
		Short: "Add a host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
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

			if _, exists := cfg.Hosts[alias]; exists {
				return fmt.Errorf("host %q already exists (use update)", alias)
			}

			newHost, err := promptHostConfig(&cfg, nil)
			if err != nil {
				return err
			}
			cfg.Hosts[alias] = newHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s added\n", alias)
			return nil
		},
	}
	return cmd
}

func newUpdateCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <host-alias>",
		Short: "Update an existing host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
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

			existing, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q does not exist", alias)
			}

			updatedHost, err := promptHostConfig(&cfg, &existing)
			if err != nil {
				return err
			}
			cfg.Hosts[alias] = updatedHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s updated\n", alias)
			return nil
		},
	}
	return cmd
}

func newRmCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <host-alias>",
		Short: "Remove a host entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
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

			if _, exists := cfg.Hosts[alias]; !exists {
				return fmt.Errorf("host %q does not exist", alias)
			}

			delete(cfg.Hosts, alias)

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s removed\n", alias)
			return nil
		},
	}
	return cmd
}

func newListCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all host aliases",
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

			aliases := make([]string, 0, len(cfg.Hosts))
			for alias := range cfg.Hosts {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)

			for _, alias := range aliases {
				fmt.Fprintln(cmd.OutOrStdout(), alias)
			}

			return nil
		},
	}
	return cmd
}

func newDumpCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump decrypted YAML to stdout",
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

			out, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal yaml: %w", err)
			}

			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	return cmd
}

func newConnectCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <host-alias>",
		Short: "Connect to a host alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConnect(cmd, opts, args[0])
		},
	}
	return cmd
}

func newUserCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage reusable user profiles",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newUserListCmd(opts),
		newUserAddCmd(opts),
		newUserUpdateCmd(opts),
		newUserRmCmd(opts),
	)
	return cmd
}

func newUserListCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List user profiles",
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

			aliases := sortedUserAliases(cfg.Users)
			if len(aliases) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no user profiles)")
				return nil
			}

			usage := map[string]int{}
			for _, hostCfg := range cfg.Hosts {
				if hostCfg.UserRef != "" {
					usage[hostCfg.UserRef]++
				}
			}

			for _, alias := range aliases {
				inUse := usage[alias]
				auth := summarizeAuth(cfg.Users[alias].Auth)
				if inUse > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tauth=%s\tused_by=%d\n", alias, cfg.Users[alias].Name, auth, inUse)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\tauth=%s\n", alias, cfg.Users[alias].Name, auth)
				}
			}

			return nil
		},
	}
	return cmd
}

func newUserAddCmd(opts *rootOptions) *cobra.Command {
	var name string
	var authType string
	var keyPath string
	var password string

	cmd := &cobra.Command{
		Use:   "add <user-alias>",
		Short: "Add a user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := normalizeUserAlias(args[0])
			if alias == "" {
				return errors.New("user alias cannot be empty")
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

			if _, exists := cfg.Users[alias]; exists {
				return fmt.Errorf("user profile %q already exists", alias)
			}

			userName := strings.TrimSpace(name)
			if userName == "" {
				reader := bufio.NewReader(os.Stdin)
				userName, err = promptNonEmpty(reader, "User", currentUserName())
				if err != nil {
					return err
				}
			}

			var auth store.AuthConfig
			authType = normalizeAuthType(authType)
			if authType != "" {
				auth, err = authConfigFromFlags(authType, keyPath, password)
				if err != nil {
					return err
				}
			} else {
				reader := bufio.NewReader(os.Stdin)
				auth, err = promptAuthConfig(reader, nil)
				if err != nil {
					return err
				}
			}

			cfg.Users[alias] = store.UserConfig{
				Name: strings.TrimSpace(userName),
				Auth: auth,
			}
			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ user profile %s added (%s)\n", alias, cfg.Users[alias].Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Username value for this profile")
	cmd.Flags().StringVar(&authType, "auth-type", "", "Auth type (key|password)")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "SSH private key path when auth-type=key")
	cmd.Flags().StringVar(&password, "password", "", "SSH password when auth-type=password")
	return cmd
}

func newUserUpdateCmd(opts *rootOptions) *cobra.Command {
	var name string
	var authType string
	var keyPath string
	var password string

	cmd := &cobra.Command{
		Use:   "update <user-alias>",
		Short: "Update a user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := normalizeUserAlias(args[0])
			if alias == "" {
				return errors.New("user alias cannot be empty")
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

			userCfg, exists := cfg.Users[alias]
			if !exists {
				return fmt.Errorf("user profile %q does not exist", alias)
			}

			if strings.TrimSpace(name) != "" {
				userCfg.Name = strings.TrimSpace(name)
			}

			authType = normalizeAuthType(authType)
			if authType != "" {
				userCfg.Auth, err = authConfigFromFlags(authType, keyPath, password)
				if err != nil {
					return err
				}
			} else if strings.TrimSpace(name) == "" {
				reader := bufio.NewReader(os.Stdin)
				userCfg.Name, err = promptNonEmpty(reader, "User", userCfg.Name)
				if err != nil {
					return err
				}
				userCfg.Auth, err = promptAuthConfig(reader, &userCfg.Auth)
				if err != nil {
					return err
				}
			}

			cfg.Users[alias] = userCfg
			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ user profile %s updated\n", alias)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Username value for this profile")
	cmd.Flags().StringVar(&authType, "auth-type", "", "Auth type (key|password)")
	cmd.Flags().StringVar(&keyPath, "key-path", "", "SSH private key path when auth-type=key")
	cmd.Flags().StringVar(&password, "password", "", "SSH password when auth-type=password")
	return cmd
}

func newUserRmCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <user-alias>",
		Short: "Remove a user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := normalizeUserAlias(args[0])
			if alias == "" {
				return errors.New("user alias cannot be empty")
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

			if _, exists := cfg.Users[alias]; !exists {
				return fmt.Errorf("user profile %q does not exist", alias)
			}

			inUseBy := hostAliasesUsingUser(cfg, alias)
			if len(inUseBy) > 0 {
				return fmt.Errorf("user profile %q is used by hosts: %s", alias, strings.Join(inUseBy, ", "))
			}

			delete(cfg.Users, alias)
			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ user profile %s removed\n", alias)
			return nil
		},
	}
	return cmd
}

func newLogoutCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear cached master password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cache, err := newPassphraseCache(repo.Path, opts.cacheTTL, opts.noCache)
			if err != nil {
				return err
			}
			if !cache.IsEnabled() {
				fmt.Fprintln(cmd.OutOrStdout(), "Master password cache is disabled.")
				return nil
			}
			if err := cache.Clear(); err != nil {
				return fmt.Errorf("clear cache: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✔ master password cache cleared")
			return nil
		},
	}
	return cmd
}

func runConnect(cmd *cobra.Command, opts *rootOptions, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("host alias cannot be empty")
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

	target, exists := cfg.Hosts[alias]
	if !exists {
		return fmt.Errorf("host %q not found", alias)
	}
	userName, auth, err := resolveHostIdentity(cfg, target)
	if err != nil {
		return err
	}

	displayPort := target.Port
	if displayPort <= 0 {
		displayPort = 22
	}
	displayTarget := target.Host
	if userName != "" {
		displayTarget = fmt.Sprintf("%s@%s", userName, target.Host)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s:%d...\n", displayTarget, displayPort)
	return executeSSH(target, userName, auth, cmd.ErrOrStderr())
}

func executeSSH(host store.HostConfig, userName string, auth store.AuthConfig, errOut io.Writer) error {
	args := make([]string, 0, 10)

	if host.Port <= 0 {
		host.Port = 22
	}

	args = append(args, "-p", strconv.Itoa(host.Port))

	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}

	switch strings.ToLower(auth.Type) {
	case "key":
		if auth.KeyPath != "" {
			keyPath, err := expandTilde(auth.KeyPath)
			if err != nil {
				return err
			}
			args = append(args, "-i", keyPath)
		}
	case "password":
	default:
		return fmt.Errorf("unsupported auth type: %s", auth.Type)
	}

	destination := host.Host
	if userName != "" {
		destination = fmt.Sprintf("%s@%s", userName, host.Host)
	}
	args = append(args, destination)

	binary := "ssh"
	env := os.Environ()
	if strings.ToLower(auth.Type) == "password" && auth.Password != "" {
		if _, err := exec.LookPath("sshpass"); err == nil {
			binary = "sshpass"
			args = append([]string{"-e", "ssh"}, args...)
			env = append(env, "SSHPASS="+auth.Password)
		} else {
			fmt.Fprintln(errOut, "sshpass not found, ssh will prompt password interactively.")
		}
	}

	execCmd := exec.Command(binary, args...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = env
	return execCmd.Run()
}

func resolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	if host.UserRef != "" {
		userCfg, ok := cfg.Users[host.UserRef]
		if !ok || strings.TrimSpace(userCfg.Name) == "" {
			return "", store.AuthConfig{}, fmt.Errorf("host references missing user profile: %s", host.UserRef)
		}
		if normalized := normalizeAuthType(userCfg.Auth.Type); normalized != "" {
			userCfg.Auth.Type = normalized
			return strings.TrimSpace(userCfg.Name), userCfg.Auth, nil
		}
		if normalized := normalizeAuthType(host.Auth.Type); normalized != "" {
			host.Auth.Type = normalized
			return strings.TrimSpace(userCfg.Name), host.Auth, nil
		}
		return "", store.AuthConfig{}, fmt.Errorf("user profile %q has no auth configured", host.UserRef)
	}

	if strings.TrimSpace(host.User) != "" {
		if normalized := normalizeAuthType(host.Auth.Type); normalized != "" {
			host.Auth.Type = normalized
			return strings.TrimSpace(host.User), host.Auth, nil
		}
		return "", store.AuthConfig{}, fmt.Errorf("host %s has no auth configured", host.Host)
	}

	return "", store.AuthConfig{}, errors.New("host has no user configured")
}

func loadConfig(opts *rootOptions, repo store.Repository) (store.PlainConfig, []byte, error) {
	cache, err := newPassphraseCache(repo.Path, opts.cacheTTL, opts.noCache)
	if err == nil {
		if cachedPassphrase, ok, _ := cache.Get(); ok {
			cfg, loadErr := repo.Load(cachedPassphrase)
			if loadErr == nil {
				return cfg, cachedPassphrase, nil
			}
			wipe(cachedPassphrase)
			_ = cache.Clear()
		}
	}

	passphrase, err := promptRequiredPassword("Enter master password: ")
	if err != nil {
		return store.PlainConfig{}, nil, err
	}

	cfg, err := repo.Load(passphrase)
	if err != nil {
		wipe(passphrase)
		if errors.Is(err, store.ErrConfigNotFound) {
			return store.PlainConfig{}, nil, fmt.Errorf("%w (run `onessh init` first)", err)
		}
		return store.PlainConfig{}, nil, err
	}

	if cache.IsEnabled() {
		_ = cache.Set(passphrase)
	}

	return cfg, passphrase, nil
}

func promptHostConfig(cfg *store.PlainConfig, existing *store.HostConfig) (store.HostConfig, error) {
	if cfg == nil {
		return store.HostConfig{}, errors.New("config is required")
	}
	if cfg.Users == nil {
		cfg.Users = map[string]store.UserConfig{}
	}

	inputReader := bufio.NewReader(os.Stdin)
	defaultUserName := currentUserName()
	defaultUserRef := ""
	defaultUserAuth := store.AuthConfig{
		Type:    "key",
		KeyPath: "~/.ssh/id_ed25519",
	}

	defaultHost := ""
	defaultPort := 22
	defaultProxyJump := ""
	defaultEnv := map[string]string{}
	hostAuthFallback := store.AuthConfig{}

	if existing != nil {
		defaultHost = existing.Host
		hostAuthFallback = existing.Auth
		if existing.UserRef != "" {
			if userCfg, ok := cfg.Users[existing.UserRef]; ok && strings.TrimSpace(userCfg.Name) != "" {
				defaultUserRef = existing.UserRef
				defaultUserName = strings.TrimSpace(userCfg.Name)
				if normalizeAuthType(userCfg.Auth.Type) != "" {
					defaultUserAuth = userCfg.Auth
				}
			}
		}
		if defaultUserRef == "" && existing.User != "" {
			defaultUserName = existing.User
			if normalizeAuthType(existing.Auth.Type) != "" {
				defaultUserAuth = existing.Auth
			}
		}
		if existing.Port > 0 {
			defaultPort = existing.Port
		}
		defaultProxyJump = existing.ProxyJump
		defaultEnv = existing.Env
	}

	host, err := promptNonEmpty(inputReader, "Host IP/Domain", defaultHost)
	if err != nil {
		return store.HostConfig{}, err
	}
	userRef, err := promptUserRefForHost(inputReader, cfg, defaultUserRef, defaultUserName, &defaultUserAuth)
	if err != nil {
		return store.HostConfig{}, err
	}
	port, err := promptPort(inputReader, defaultPort)
	if err != nil {
		return store.HostConfig{}, err
	}

	proxyJump, err := promptOptional(inputReader, "Proxy jump", defaultProxyJump)
	if err != nil {
		return store.HostConfig{}, err
	}

	return store.HostConfig{
		Host:      host,
		UserRef:   userRef,
		Port:      port,
		Auth:      hostAuthFallback,
		ProxyJump: proxyJump,
		Env:       defaultEnv,
	}, nil
}

func promptUserRefForHost(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserRef, defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	if cfg.Users == nil {
		cfg.Users = map[string]store.UserConfig{}
	}

	if len(cfg.Users) == 0 {
		return createOrReuseUserProfile(reader, cfg, defaultUserName, defaultUserAuth)
	}

	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return promptUserRefSelect(reader, cfg, defaultUserRef, defaultUserName, defaultUserAuth)
	}
	return promptUserRefText(reader, cfg, defaultUserRef, defaultUserName, defaultUserAuth)
}

func promptUserRefSelect(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserRef, defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	aliases := sortedUserAliases(cfg.Users)
	items := make([]string, 0, len(aliases)+1)
	items = append(items, "Create new user profile")
	for _, alias := range aliases {
		items = append(items, fmt.Sprintf("%s (%s)", alias, cfg.Users[alias].Name))
	}

	cursorPos := 0
	for i, alias := range aliases {
		if alias == defaultUserRef {
			cursorPos = i + 1
			break
		}
	}

	prompt := promptui.Select{
		Label:             "User profile (use ↑/↓ and Enter)",
		Items:             items,
		CursorPos:         cursorPos,
		Size:              len(items),
		HideHelp:          true,
		StartInSearchMode: false,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return "", err
	}
	if index == 0 {
		return createOrReuseUserProfile(reader, cfg, defaultUserName, defaultUserAuth)
	}
	return aliases[index-1], nil
}

func promptUserRefText(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserRef, defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	aliases := sortedUserAliases(cfg.Users)
	fmt.Fprintln(os.Stderr, "Available user profiles:")
	for i, alias := range aliases {
		fmt.Fprintf(os.Stderr, "  %d) %s (%s)\n", i+1, alias, cfg.Users[alias].Name)
	}

	defaultChoice := defaultUserRef
	if defaultChoice == "" {
		defaultChoice = "new"
	}

	for {
		raw, err := promptOptional(reader, "User profile (alias/index/new)", defaultChoice)
		if err != nil {
			return "", err
		}
		choice := strings.TrimSpace(raw)
		if strings.EqualFold(choice, "new") || choice == "0" {
			return createOrReuseUserProfile(reader, cfg, defaultUserName, defaultUserAuth)
		}
		if _, ok := cfg.Users[choice]; ok {
			return choice, nil
		}
		index, err := strconv.Atoi(choice)
		if err == nil && index >= 1 && index <= len(aliases) {
			return aliases[index-1], nil
		}
		fmt.Fprintln(os.Stderr, "Invalid user profile. Use alias/index or type new.")
	}
}

func createOrReuseUserProfile(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	userName, err := promptNonEmpty(reader, "User", defaultUserName)
	if err != nil {
		return "", err
	}
	userName = strings.TrimSpace(userName)

	if existingAlias := findUserAliasByName(cfg.Users, userName); existingAlias != "" {
		fmt.Fprintf(os.Stderr, "Using existing user profile: %s (%s)\n", existingAlias, userName)
		return existingAlias, nil
	}

	defaultAlias := normalizeUserAlias(userName)
	for {
		alias, err := promptNonEmpty(reader, "User profile alias", defaultAlias)
		if err != nil {
			return "", err
		}
		alias = normalizeUserAlias(alias)
		if alias == "" {
			fmt.Fprintln(os.Stderr, "User profile alias cannot be empty.")
			continue
		}

		if existing, exists := cfg.Users[alias]; exists {
			if strings.EqualFold(strings.TrimSpace(existing.Name), userName) {
				return alias, nil
			}
			fmt.Fprintf(os.Stderr, "User profile alias %q already exists.\n", alias)
			continue
		}

		auth, err := promptAuthConfig(reader, defaultUserAuth)
		if err != nil {
			return "", err
		}

		cfg.Users[alias] = store.UserConfig{
			Name: userName,
			Auth: auth,
		}
		return alias, nil
	}
}

func sortedUserAliases(users map[string]store.UserConfig) []string {
	aliases := make([]string, 0, len(users))
	for alias := range users {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func findUserAliasByName(users map[string]store.UserConfig, name string) string {
	normalizedName := strings.TrimSpace(name)
	for alias, cfg := range users {
		if strings.EqualFold(strings.TrimSpace(cfg.Name), normalizedName) {
			return alias
		}
	}
	return ""
}

func hostAliasesUsingUser(cfg store.PlainConfig, userAlias string) []string {
	var hostAliases []string
	for hostAlias, hostCfg := range cfg.Hosts {
		if hostCfg.UserRef == userAlias {
			hostAliases = append(hostAliases, hostAlias)
		}
	}
	sort.Strings(hostAliases)
	return hostAliases
}

func normalizeUserAlias(input string) string {
	raw := strings.ToLower(strings.TrimSpace(input))
	raw = strings.ReplaceAll(raw, " ", "-")
	if raw == "" {
		return ""
	}

	var b strings.Builder
	for _, ch := range raw {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			b.WriteRune(ch)
		}
	}
	alias := strings.Trim(b.String(), "-_.")
	if alias == "" {
		return "user"
	}
	return alias
}

func promptNonEmpty(reader *bufio.Reader, label, defaultValue string) (string, error) {
	for {
		value, err := promptOptional(reader, label, defaultValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
}

func promptOptional(reader *bufio.Reader, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	text := strings.TrimSpace(line)
	if text == "" {
		return defaultValue, nil
	}
	return text, nil
}

func promptPort(reader *bufio.Reader, defaultPort int) (int, error) {
	for {
		raw, err := promptOptional(reader, "Port", strconv.Itoa(defaultPort))
		if err != nil {
			return 0, err
		}
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port <= 0 || port > 65535 {
			fmt.Fprintln(os.Stderr, "Port must be a number between 1 and 65535.")
			continue
		}
		return port, nil
	}
}

func promptAuthConfig(reader *bufio.Reader, existing *store.AuthConfig) (store.AuthConfig, error) {
	defaultType := "key"
	defaultKeyPath := "~/.ssh/id_ed25519"
	defaultPassword := ""
	if existing != nil {
		if normalized := normalizeAuthType(existing.Type); normalized != "" {
			defaultType = normalized
		}
		if existing.KeyPath != "" {
			defaultKeyPath = existing.KeyPath
		}
		defaultPassword = existing.Password
	}

	authType, err := promptAuthType(reader, defaultType)
	if err != nil {
		return store.AuthConfig{}, err
	}

	auth := store.AuthConfig{Type: authType}
	switch authType {
	case "key":
		keyPath, err := promptNonEmpty(reader, "Key path", defaultKeyPath)
		if err != nil {
			return store.AuthConfig{}, err
		}
		auth.KeyPath = keyPath
	case "password":
		if existing != nil && defaultPassword != "" {
			password, changed, err := promptOptionalSecret("Password (press Enter to keep current): ")
			if err != nil {
				return store.AuthConfig{}, err
			}
			if changed {
				auth.Password = string(password)
				wipe(password)
			} else {
				auth.Password = defaultPassword
			}
		} else {
			password, err := promptRequiredPassword("Password: ")
			if err != nil {
				return store.AuthConfig{}, err
			}
			auth.Password = string(password)
			wipe(password)
		}
	default:
		return store.AuthConfig{}, fmt.Errorf("unsupported auth type: %s", authType)
	}

	return auth, nil
}

func authConfigFromFlags(authType, keyPath, password string) (store.AuthConfig, error) {
	auth := store.AuthConfig{Type: authType}
	switch authType {
	case "key":
		if strings.TrimSpace(keyPath) == "" {
			return store.AuthConfig{}, errors.New("key-path is required when auth-type=key")
		}
		auth.KeyPath = strings.TrimSpace(keyPath)
	case "password":
		if strings.TrimSpace(password) == "" {
			return store.AuthConfig{}, errors.New("password is required when auth-type=password")
		}
		auth.Password = password
	default:
		return store.AuthConfig{}, errors.New("auth-type must be key or password")
	}
	return auth, nil
}

func promptAuthType(reader *bufio.Reader, defaultType string) (string, error) {
	defaultType = normalizeAuthType(defaultType)
	if defaultType == "" {
		defaultType = "key"
	}
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return promptAuthTypeSelect(defaultType)
	}

	for {
		raw, err := promptOptional(reader, "Auth type (key/password or 1/2)", defaultType)
		if err != nil {
			return "", err
		}
		authType := normalizeAuthType(raw)
		if authType != "" {
			return authType, nil
		}
		fmt.Fprintln(os.Stderr, "Auth type must be key/password or 1/2.")
	}
}

func promptAuthTypeSelect(defaultType string) (string, error) {
	items := []string{"key", "password"}
	cursorPos := 0
	if defaultType == "password" {
		cursorPos = 1
	}

	prompt := promptui.Select{
		Label:             "Auth type (use ↑/↓ and Enter)",
		Items:             items,
		CursorPos:         cursorPos,
		Size:              len(items),
		HideHelp:          true,
		StartInSearchMode: false,
	}

	_, result, err := prompt.Run()
	if err != nil {
		return "", err
	}

	return normalizeAuthType(result), nil
}

func normalizeAuthType(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "1", "k", "key":
		return "key"
	case "2", "p", "pass", "password":
		return "password"
	default:
		return ""
	}
}

func summarizeAuth(auth store.AuthConfig) string {
	switch normalizeAuthType(auth.Type) {
	case "key":
		if strings.TrimSpace(auth.KeyPath) != "" {
			return "key:" + strings.TrimSpace(auth.KeyPath)
		}
		return "key"
	case "password":
		return "password"
	default:
		return "none"
	}
}

func promptRequiredPassword(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	return secret, nil
}

func promptOptionalSecret(prompt string) ([]byte, bool, error) {
	fmt.Fprint(os.Stderr, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, false, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, false, nil
	}
	return secret, true, nil
}

func (o *rootOptions) repository() (store.Repository, error) {
	path, err := store.ResolvePath(o.configPath)
	if err != nil {
		return store.Repository{}, err
	}
	return store.Repository{Path: path}, nil
}

func currentUserName() string {
	u, err := user.Current()
	if err != nil {
		return "root"
	}
	if u.Username == "" {
		return "root"
	}
	return u.Username
}

func wipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func expandTilde(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/" + strings.TrimPrefix(input, "~/"), nil
	}
	return input, nil
}
