package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"onessh/internal/store"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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
		Use:   "ls",
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

			// Build map: user alias -> list of host aliases that reference it
			hostsByUser := make(map[string][]string)
			for hostAlias, hostCfg := range cfg.Hosts {
				if hostCfg.UserRef != "" {
					hostsByUser[hostCfg.UserRef] = append(hostsByUser[hostCfg.UserRef], hostAlias)
				}
			}
			for _, hosts := range hostsByUser {
				sort.Strings(hosts)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tUSER\tAUTH\tUSED_BY")
			for _, alias := range aliases {
				auth := summarizeAuth(cfg.Users[alias].Auth)
				hosts := hostsByUser[alias]
				usedBy := "-"
				if len(hosts) > 0 {
					// Show at most 3 hosts, then "+N more" for the rest.
					usedBy = strings.Join(hosts[:min(3, len(hosts))], ", ")
					if len(hosts) > 3 {
						usedBy += fmt.Sprintf(" (+%d more)", len(hosts)-3)
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", alias, cfg.Users[alias].Name, auth, usedBy)
			}
			_ = w.Flush()

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

			opts.logEvent("add_user", alias, "", cfg.Users[alias].Name, "ok", nil)
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

			opts.logEvent("update_user", alias, "", userCfg.Name, "ok", nil)
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
				return fmt.Errorf("user profile %q is used by host(s): %s. Please remove these hosts first", alias, strings.Join(inUseBy, ", "))
			}

			delete(cfg.Users, alias)
			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			opts.logEvent("rm_user", alias, "", "", "ok", nil)
			fmt.Fprintf(cmd.OutOrStdout(), "✔ user profile %s removed\n", alias)
			return nil
		},
	}
	return cmd
}

func newPasswdCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "passwd",
		Short: "Change master password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}

			currentPassphrase, err := promptRequiredPassword("Enter current master password: ")
			if err != nil {
				return err
			}
			defer wipe(currentPassphrase)

			newPassphrase, err := promptRequiredPassword("Enter new master password: ")
			if err != nil {
				return err
			}
			defer wipe(newPassphrase)

			confirmPassphrase, err := promptRequiredPassword("Confirm new master password: ")
			if err != nil {
				return err
			}
			defer wipe(confirmPassphrase)

			if !bytes.Equal(newPassphrase, confirmPassphrase) {
				return errors.New("new passwords do not match")
			}

			cache, err := opts.passphraseStore(repo.Path)
			if err != nil {
				return err
			}
			if err := changeMasterPassword(repo, cache, currentPassphrase, newPassphrase); err != nil {
				if errors.Is(err, store.ErrConfigNotFound) {
					return fmt.Errorf("%w (run `onessh init` first)", err)
				}
				return err
			}

			opts.logEvent("passwd", "", "", "", "ok", nil)
			fmt.Fprintln(cmd.OutOrStdout(), "✔ master password updated")
			return nil
		},
	}
	return cmd
}

func newLogoutCmd(opts *rootOptions) *cobra.Command {
	var clearAll bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear cached master password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clearAll {
				socketPath, err := resolveAgentSocketPath(resolveSocketFlag("", opts))
				if err != nil {
					return err
				}
				if err := clearPassphraseAgentAll(socketPath); err != nil {
					fmt.Fprintln(cmd.OutOrStdout(), "Agent is not running.")
					return nil
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✔ all agent cache entries cleared")
				return nil
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}

			cache, err := opts.passphraseStore(repo.Path)
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
	cmd.Flags().BoolVar(&clearAll, "all", false, "Clear all agent cache entries and askpass tokens")
	return cmd
}

func changeMasterPassword(
	repo store.Repository,
	cache passphraseStore,
	currentPassphrase, newPassphrase []byte,
) error {
	if len(bytes.TrimSpace(currentPassphrase)) == 0 {
		return errors.New("current password cannot be empty")
	}
	if len(bytes.TrimSpace(newPassphrase)) == 0 {
		return errors.New("new password cannot be empty")
	}
	if bytes.Equal(currentPassphrase, newPassphrase) {
		return errors.New("new password must be different from current password")
	}

	cfg, err := repo.Load(currentPassphrase)
	if err != nil {
		return err
	}
	if err := repo.SaveWithReset(cfg, newPassphrase); err != nil {
		return err
	}
	if cache != nil && cache.IsEnabled() {
		_ = cache.Set(newPassphrase)
	}
	return nil
}

func resolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	if strings.TrimSpace(host.UserRef) == "" {
		return "", store.AuthConfig{}, errors.New("host has no user_ref configured")
	}

	userCfg, ok := cfg.Users[host.UserRef]
	if !ok {
		return "", store.AuthConfig{}, fmt.Errorf("host references missing user profile: %s", host.UserRef)
	}
	if strings.TrimSpace(userCfg.Name) == "" {
		return "", store.AuthConfig{}, fmt.Errorf("user profile %q has empty name", host.UserRef)
	}

	normalizedAuthType := normalizeAuthType(userCfg.Auth.Type)
	if normalizedAuthType == "" {
		return "", store.AuthConfig{}, fmt.Errorf("user profile %q has no auth configured", host.UserRef)
	}
	userCfg.Auth.Type = normalizedAuthType
	if userCfg.Auth.Type == "password" && strings.TrimSpace(userCfg.Auth.Password) == "" {
		return "", store.AuthConfig{}, fmt.Errorf("user profile %q has empty password", host.UserRef)
	}
	return strings.TrimSpace(userCfg.Name), userCfg.Auth, nil
}

func summarizeHostIdentityForList(cfg store.PlainConfig, host store.HostConfig) (string, string, string) {
	userRef := strings.TrimSpace(host.UserRef)
	if userRef == "" {
		return "-", "-", "missing_user_ref"
	}

	userCfg, ok := cfg.Users[userRef]
	if !ok {
		return "-", "-", "missing_user_profile"
	}

	userName := strings.TrimSpace(userCfg.Name)
	if userName == "" {
		userName = "-"
	}

	authType := normalizeAuthType(userCfg.Auth.Type)
	if authType == "" {
		return userName, "-", "missing_auth"
	}
	if authType == "password" && strings.TrimSpace(userCfg.Auth.Password) == "" {
		return userName, authType, "empty_password"
	}

	return userName, authType, "ok"
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
	defaultDescription := ""
	defaultEnv := map[string]string{}
	defaultPreConnect := []string{}
	defaultPostConnect := []string{}

	if existing != nil {
		defaultHost = existing.Host
		if existing.UserRef != "" {
			if userCfg, ok := cfg.Users[existing.UserRef]; ok && strings.TrimSpace(userCfg.Name) != "" {
				defaultUserRef = existing.UserRef
				defaultUserName = strings.TrimSpace(userCfg.Name)
				if normalizeAuthType(userCfg.Auth.Type) != "" {
					defaultUserAuth = userCfg.Auth
				}
			}
		}
		if existing.Port > 0 {
			defaultPort = existing.Port
		}
		defaultProxyJump = existing.ProxyJump
		defaultEnv = existing.Env
		defaultPreConnect = cloneStringSlice(existing.PreConnect)
		defaultPostConnect = cloneStringSlice(existing.PostConnect)
		defaultDescription = existing.Description
	}

	host, err := promptNonEmpty(inputReader, "Host IP/Domain", defaultHost)
	if err != nil {
		return store.HostConfig{}, err
	}

	existingUserAliases := map[string]struct{}{}
	for alias := range cfg.Users {
		existingUserAliases[alias] = struct{}{}
	}

	userRef, err := promptUserRefForHost(inputReader, cfg, defaultUserRef, defaultUserName, &defaultUserAuth)
	if err != nil {
		return store.HostConfig{}, err
	}
	if existing != nil {
		if _, existedBefore := existingUserAliases[userRef]; existedBefore {
			if err := maybeEditSelectedUserProfile(inputReader, cfg, userRef); err != nil {
				return store.HostConfig{}, err
			}
		}
	}

	port, err := promptPort(inputReader, defaultPort)
	if err != nil {
		return store.HostConfig{}, err
	}

	proxyJump, err := promptOptional(inputReader, "Proxy jump", defaultProxyJump)
	if err != nil {
		return store.HostConfig{}, err
	}

	description, err := promptOptional(inputReader, "Description", defaultDescription)
	if err != nil {
		return store.HostConfig{}, err
	}

	return store.HostConfig{
		Host:        host,
		Description: strings.TrimSpace(description),
		UserRef:     userRef,
		Port:        port,
		ProxyJump:   proxyJump,
		Env:         defaultEnv,
		PreConnect:  defaultPreConnect,
		PostConnect: defaultPostConnect,
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
	return promptUserRefSelect(reader, cfg, defaultUserRef, defaultUserName, defaultUserAuth)
}

const selectPageSize = 10

func promptUserRefSelect(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserRef, defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	aliases := sortedUserAliases(cfg.Users)
	items := make([]string, 0, len(aliases)+2)
	items = append(items, "Create new user profile")
	items = append(items, "Input alias")
	for _, alias := range aliases {
		items = append(items, fmt.Sprintf("%s (%s)", alias, cfg.Users[alias].Name))
	}

	cursorPos := 0
	if defaultUserRef != "" {
		for i, alias := range aliases {
			if alias == defaultUserRef {
				cursorPos = i + 2 // +2 for "new" and "input alias"
				break
			}
		}
	}

	pageSize := selectPageSize
	if len(items) < pageSize {
		pageSize = len(items)
	}
	prompt := promptui.Select{
		Label:             "User profile (use ↑/↓ and Enter)",
		Items:             items,
		CursorPos:         cursorPos,
		Size:              pageSize,
		HideHelp:          true,
		StartInSearchMode: false,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return "", err
	}
	switch index {
	case 0:
		return createOrReuseUserProfile(reader, cfg, defaultUserName, defaultUserAuth)
	case 1:
		return promptUserRefByAlias(reader, cfg)
	default:
		return aliases[index-2], nil
	}
}

func promptUserRefByAlias(reader *bufio.Reader, cfg *store.PlainConfig) (string, error) {
	out := promptWriter()
	for {
		alias, err := promptNonEmpty(reader, "User profile alias", "")
		if err != nil {
			return "", err
		}
		alias = normalizeUserAlias(strings.TrimSpace(alias))
		if alias == "" {
			fmt.Fprintln(out, "User profile alias cannot be empty.")
			continue
		}
		if _, ok := cfg.Users[alias]; ok {
			return alias, nil
		}
		fmt.Fprintf(out, "User profile %q not found. Please try again.\n", alias)
	}
}

func promptUserRefText(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserRef, defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	out := promptWriter()
	aliases := sortedUserAliases(cfg.Users)
	fmt.Fprintln(out, "Available user profiles:")
	for i, alias := range aliases {
		fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, alias, cfg.Users[alias].Name)
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
		fmt.Fprintln(out, "Invalid user profile. Use alias/index or type new.")
	}
}

func createOrReuseUserProfile(
	reader *bufio.Reader,
	cfg *store.PlainConfig,
	defaultUserName string,
	defaultUserAuth *store.AuthConfig,
) (string, error) {
	out := promptWriter()
	userName, err := promptNonEmpty(reader, "User", defaultUserName)
	if err != nil {
		return "", err
	}
	userName = strings.TrimSpace(userName)

	if existingAlias := findUserAliasByName(cfg.Users, userName); existingAlias != "" {
		fmt.Fprintf(out, "Using existing user profile: %s (%s)\n", existingAlias, userName)
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
			fmt.Fprintln(out, "User profile alias cannot be empty.")
			continue
		}

		if existing, exists := cfg.Users[alias]; exists {
			if strings.EqualFold(strings.TrimSpace(existing.Name), userName) {
				return alias, nil
			}
			fmt.Fprintf(out, "User profile alias %q already exists.\n", alias)
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

func maybeEditSelectedUserProfile(reader *bufio.Reader, cfg *store.PlainConfig, userRef string) error {
	userCfg, exists := cfg.Users[userRef]
	if !exists {
		return nil
	}

	shouldEdit, err := promptYesNo(reader, fmt.Sprintf("Edit user profile %s now", userRef), false)
	if err != nil {
		return err
	}
	if !shouldEdit {
		return nil
	}

	userName, err := promptNonEmpty(reader, "User", userCfg.Name)
	if err != nil {
		return err
	}
	userCfg.Name = strings.TrimSpace(userName)

	auth, err := promptAuthConfig(reader, &userCfg.Auth)
	if err != nil {
		return err
	}
	userCfg.Auth = auth

	cfg.Users[userRef] = userCfg
	return nil
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
	out := promptWriter()
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
		fmt.Fprintln(out, "Auth type must be key/password or 1/2.")
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

func normalizeUserAlias(input string) string {
	alias := strings.ToLower(strings.TrimSpace(input))
	if alias == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_")
}
