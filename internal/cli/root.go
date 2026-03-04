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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"onessh/internal/store"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const redactedSecretValue = "[REDACTED]"

type rootOptions struct {
	configPath  string
	cacheTTL    time.Duration
	noCache     bool
	agentSocket string
}

func NewRootCmd(version, commit, date string) *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "onessh [host] [-- <ssh-args...>]",
		Short:         "Manage and connect SSH hosts from encrypted config",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, sshArgs, err := parseConnectInvocation(cmd, args)
			if err != nil {
				return err
			}
			return runConnect(cmd, opts, alias, sshArgs)
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "Path to config store directory")
	rootCmd.PersistentFlags().DurationVar(&opts.cacheTTL, "cache-ttl", 10*time.Minute, "Master password cache duration")
	rootCmd.PersistentFlags().BoolVar(&opts.noCache, "no-cache", false, "Disable master password cache")
	rootCmd.PersistentFlags().StringVar(&opts.agentSocket, "agent-socket", defaultAgentSocketFlagValue(), "Memory cache agent Unix socket path")

	rootCmd.AddCommand(
		newInitCmd(opts),
		newAddCmd(opts),
		newUpdateCmd(opts),
		newRmCmd(opts),
		newLsCmd(opts),
		newDumpCmd(opts),
		newConnectCmd(opts),
		newSSHConfigCmd(opts),
		newAgentCmd(opts),
		newUserCmd(opts),
		newLogoutCmd(opts),
		newVersionCmd(version, commit, date),
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
			if force {
				if err := repo.SaveWithReset(cfg, pass1); err != nil {
					return err
				}
			} else if err := repo.Save(cfg, pass1); err != nil {
				return err
			}
			cache, err := opts.passphraseStore(repo.Path)
			if err != nil {
				return err
			}
			if cache.IsEnabled() {
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
	var (
		envFlags    []string
		preConnect  []string
		postConnect []string
	)

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
			if cmd.Flags().Changed("env") {
				envMap, err := parseEnvAssignments(envFlags)
				if err != nil {
					return err
				}
				newHost.Env = envMap
			}
			if cmd.Flags().Changed("pre-connect") {
				cmds, err := parseHookCommands(preConnect, "pre-connect")
				if err != nil {
					return err
				}
				newHost.PreConnect = cmds
			}
			if cmd.Flags().Changed("post-connect") {
				cmds, err := parseHookCommands(postConnect, "post-connect")
				if err != nil {
					return err
				}
				newHost.PostConnect = cmds
			}
			cfg.Hosts[alias] = newHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s added\n", alias)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&envFlags, "env", nil, "Host environment variable (KEY=VALUE), repeatable")
	cmd.Flags().StringArrayVar(&preConnect, "pre-connect", nil, "Remote command run before interactive shell, repeatable")
	cmd.Flags().StringArrayVar(&postConnect, "post-connect", nil, "Remote command run after interactive shell exits, repeatable")
	return cmd
}

func newUpdateCmd(opts *rootOptions) *cobra.Command {
	var (
		aliasFlag    string
		hostFlag     string
		portFlag     int
		proxyJump    string
		userRefFlag  string
		userFlag     string
		authTypeFlag string
		keyPathFlag  string
		passwordFlag string
		envFlags     []string
		unsetEnv     []string
		clearEnv     bool
		preConnect   []string
		postConnect  []string
		clearPre     bool
		clearPost    bool
	)

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

			targetAlias := alias
			updatedHost := existing

			if hasAnyHostUpdateFlags(cmd) {
				if cmd.Flags().Changed("alias") {
					targetAlias = strings.TrimSpace(aliasFlag)
					if targetAlias == "" {
						return errors.New("--alias cannot be empty")
					}
				}
				if cmd.Flags().Changed("host") {
					updatedHost.Host = strings.TrimSpace(hostFlag)
					if updatedHost.Host == "" {
						return errors.New("--host cannot be empty")
					}
				}
				if cmd.Flags().Changed("port") {
					if portFlag <= 0 || portFlag > 65535 {
						return errors.New("--port must be between 1 and 65535")
					}
					updatedHost.Port = portFlag
				}
				if cmd.Flags().Changed("proxy-jump") {
					updatedHost.ProxyJump = strings.TrimSpace(proxyJump)
				}
				if err := applyHostEnvUpdateFlags(cmd, &updatedHost, envFlags, unsetEnv, clearEnv); err != nil {
					return err
				}
				if err := applyHostHookUpdateFlags(cmd, &updatedHost, preConnect, postConnect, clearPre, clearPost); err != nil {
					return err
				}

				if err := applyUserProfileUpdateFlags(
					cmd,
					&cfg,
					&updatedHost,
					userRefFlag,
					userFlag,
					authTypeFlag,
					keyPathFlag,
					passwordFlag,
				); err != nil {
					return err
				}

				if _, _, err := resolveHostIdentity(cfg, updatedHost); err != nil {
					return err
				}
			} else {
				if term.IsTerminal(int(os.Stdin.Fd())) {
					reader := bufio.NewReader(os.Stdin)
					targetAlias, err = promptNonEmpty(reader, "Host alias", alias)
					if err != nil {
						return err
					}
				}
				targetAlias = strings.TrimSpace(targetAlias)
				if targetAlias == "" {
					return errors.New("target host alias cannot be empty")
				}

				updatedHost, err = promptHostConfig(&cfg, &existing)
				if err != nil {
					return err
				}
			}

			if targetAlias != alias {
				if _, conflict := cfg.Hosts[targetAlias]; conflict {
					return fmt.Errorf("host %q already exists", targetAlias)
				}
			}
			if targetAlias != alias {
				delete(cfg.Hosts, alias)
			}
			cfg.Hosts[targetAlias] = updatedHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			if targetAlias != alias {
				fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s renamed to %s and updated\n", alias, targetAlias)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s updated\n", alias)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&aliasFlag, "alias", "", "Update host alias")
	cmd.Flags().StringVar(&hostFlag, "host", "", "Update host address or domain")
	cmd.Flags().IntVar(&portFlag, "port", 0, "Update SSH port")
	cmd.Flags().StringVar(&proxyJump, "proxy-jump", "", "Update ProxyJump (empty value clears it)")
	cmd.Flags().StringVar(&userRefFlag, "user-ref", "", "Bind host to an existing user profile alias")
	cmd.Flags().StringVar(&userFlag, "user", "", "Update linked user profile name")
	cmd.Flags().StringVar(&authTypeFlag, "auth-type", "", "Update linked user auth type (key|password)")
	cmd.Flags().StringVar(&keyPathFlag, "key-path", "", "Update linked user key path")
	cmd.Flags().StringVar(&passwordFlag, "password", "", "Update linked user password")
	cmd.Flags().StringArrayVar(&envFlags, "env", nil, "Set host env entry (KEY=VALUE), repeatable")
	cmd.Flags().StringArrayVar(&unsetEnv, "unset-env", nil, "Remove host env entry by key, repeatable")
	cmd.Flags().BoolVar(&clearEnv, "clear-env", false, "Remove all host env entries")
	cmd.Flags().StringArrayVar(&preConnect, "pre-connect", nil, "Set pre-connect remote command, repeatable")
	cmd.Flags().StringArrayVar(&postConnect, "post-connect", nil, "Set post-connect remote command, repeatable")
	cmd.Flags().BoolVar(&clearPre, "clear-pre-connect", false, "Remove all pre-connect commands")
	cmd.Flags().BoolVar(&clearPost, "clear-post-connect", false, "Remove all post-connect commands")
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

func newLsCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List hosts with summary information",
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

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tHOST\tUSER\tUSER_REF\tAUTH\tPORT\tPROXY_JUMP\tSTATUS")
			for _, alias := range aliases {
				host := cfg.Hosts[alias]
				userName, authType, status := summarizeHostIdentityForList(cfg, host)
				port := host.Port
				if port <= 0 {
					port = 22
				}
				proxyJump := strings.TrimSpace(host.ProxyJump)
				if proxyJump == "" {
					proxyJump = "-"
				}
				userRef := strings.TrimSpace(host.UserRef)
				if userRef == "" {
					userRef = "-"
				}
				fmt.Fprintf(
					w,
					"%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					alias,
					host.Host,
					userName,
					userRef,
					authType,
					port,
					proxyJump,
					status,
				)
			}
			_ = w.Flush()

			return nil
		},
	}
	return cmd
}

func newDumpCmd(opts *rootOptions) *cobra.Command {
	var showSecrets bool

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

			outputCfg := cfg
			if !showSecrets {
				outputCfg = redactConfigForDump(cfg)
			}

			out, err := yaml.Marshal(outputCfg)
			if err != nil {
				return fmt.Errorf("marshal yaml: %w", err)
			}

			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "Include sensitive secret values in output")
	return cmd
}

func newConnectCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <host-alias> [-- <ssh-args...>]",
		Short: "Connect to a host alias",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, sshArgs, err := parseConnectInvocation(cmd, args)
			if err != nil {
				return err
			}
			return runConnect(cmd, opts, alias, sshArgs)
		},
	}
	return cmd
}

func newVersionCmd(version, commit, date string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "onessh %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "commit: %s\n", commit)
			fmt.Fprintf(cmd.OutOrStdout(), "date: %s\n", date)
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
	return cmd
}

func parseConnectInvocation(cmd *cobra.Command, args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, errors.New("host alias cannot be empty")
	}

	alias := strings.TrimSpace(args[0])
	if alias == "" {
		return "", nil, errors.New("host alias cannot be empty")
	}

	var sshArgs []string
	if len(args) > 1 {
		sshArgs = append(sshArgs, args[1:]...)
	}
	if dashAt := cmd.ArgsLenAtDash(); dashAt >= 0 {
		if dashAt >= len(args) {
			sshArgs = nil
		} else {
			sshArgs = append([]string{}, args[dashAt:]...)
		}
	}

	return alias, sshArgs, nil
}

func runConnect(cmd *cobra.Command, opts *rootOptions, alias string, sshArgs []string) error {
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
	return executeSSH(target, userName, auth, sshArgs, cmd.ErrOrStderr())
}

func executeSSH(host store.HostConfig, userName string, auth store.AuthConfig, sshArgs []string, errOut io.Writer) error {
	args := make([]string, 0, 10+len(sshArgs))
	extraFiles := []*os.File{}
	cleanupExtraFiles := []func(){}

	if host.Port <= 0 {
		host.Port = 22
	}

	args = append(args, "-p", strconv.Itoa(host.Port))

	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}
	args = appendSendEnvOptions(args, host.Env)

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

	hookCommand := buildRemoteHookCommand(host.PreConnect, host.PostConnect)
	if hookCommand != "" {
		if containsShortFlag(sshArgs, 'N') {
			return errors.New("pre/post-connect commands are incompatible with -N")
		}
		if containsShortFlag(sshArgs, 'T') {
			return errors.New("pre/post-connect commands are incompatible with -T")
		}
		args = append(args, "-tt")
	}

	args = append(args, sshArgs...)
	args = append(args, destination)
	if hookCommand != "" {
		args = append(args, hookCommand)
	}

	binary := "ssh"
	env := mergeCommandEnv(os.Environ(), host.Env)
	if strings.ToLower(auth.Type) == "password" && auth.Password != "" {
		if _, err := exec.LookPath("sshpass"); err != nil {
			fmt.Fprintln(errOut, "sshpass not found, password auth requires sshpass.")
			return errors.New("password auth requires sshpass installed")
		}
		passwordFD, cleanup, err := newPasswordFD(auth.Password)
		if err != nil {
			return err
		}
		defer cleanup()
		extraFiles = append(extraFiles, passwordFD)
		cleanupExtraFiles = append(cleanupExtraFiles, func() { _ = passwordFD.Close() })
		binary = "sshpass"
		args = append([]string{"-d", "3", "ssh"}, args...)
	}

	defer func() {
		for _, cleanup := range cleanupExtraFiles {
			cleanup()
		}
	}()

	execCmd := exec.Command(binary, args...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = env
	if len(extraFiles) > 0 {
		execCmd.ExtraFiles = extraFiles
	}
	return execCmd.Run()
}

func buildRemoteHookCommand(preConnect, postConnect []string) string {
	preparedPre := sanitizeHookCommands(preConnect)
	preparedPost := sanitizeHookCommands(postConnect)
	if len(preparedPre) == 0 && len(preparedPost) == 0 {
		return ""
	}

	lines := make([]string, 0, len(preparedPre)+len(preparedPost)+5)
	lines = append(lines, "set -e")
	lines = append(lines, preparedPre...)
	lines = append(lines, "${SHELL:-/bin/sh} -i")
	lines = append(lines, "onessh_status=$?")
	lines = append(lines, preparedPost...)
	lines = append(lines, "exit $onessh_status")

	script := strings.Join(lines, "\n")
	return "sh -lc " + shellSingleQuote(script)
}

func newPasswordFD(password string) (*os.File, func(), error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create password pipe: %w", err)
	}

	secret := append([]byte(password), '\n')
	defer wipe(secret)

	if _, err := writer.Write(secret); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("write password to pipe: %w", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		return nil, nil, fmt.Errorf("close password pipe writer: %w", err)
	}

	cleanup := func() {
		_ = reader.Close()
	}
	return reader, cleanup, nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func containsShortFlag(args []string, flag rune) bool {
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' || strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.ContainsRune(arg[1:], flag) {
			return true
		}
	}
	return false
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

func hasAnyHostUpdateFlags(cmd *cobra.Command) bool {
	checks := []string{
		"alias",
		"host",
		"port",
		"proxy-jump",
		"env",
		"unset-env",
		"clear-env",
		"pre-connect",
		"post-connect",
		"clear-pre-connect",
		"clear-post-connect",
		"user-ref",
		"user",
		"auth-type",
		"key-path",
		"password",
	}
	for _, name := range checks {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func applyHostEnvUpdateFlags(
	cmd *cobra.Command,
	host *store.HostConfig,
	envFlags, unsetEnv []string,
	clearEnv bool,
) error {
	changedEnv := cmd.Flags().Changed("env")
	changedUnset := cmd.Flags().Changed("unset-env")
	changedClear := cmd.Flags().Changed("clear-env") && clearEnv
	if !changedEnv && !changedUnset && !changedClear {
		return nil
	}

	current := cloneStringMap(host.Env)
	if changedClear {
		current = map[string]string{}
	}

	if changedUnset {
		keys, err := parseEnvKeys(unsetEnv)
		if err != nil {
			return err
		}
		for _, key := range keys {
			delete(current, key)
		}
	}

	if changedEnv {
		entries, err := parseEnvAssignments(envFlags)
		if err != nil {
			return err
		}
		for key, value := range entries {
			current[key] = value
		}
	}

	if len(current) == 0 {
		host.Env = nil
		return nil
	}
	host.Env = current
	return nil
}

func applyHostHookUpdateFlags(
	cmd *cobra.Command,
	host *store.HostConfig,
	preConnect, postConnect []string,
	clearPre, clearPost bool,
) error {
	changedPre := cmd.Flags().Changed("pre-connect")
	changedPost := cmd.Flags().Changed("post-connect")
	changedClearPre := cmd.Flags().Changed("clear-pre-connect") && clearPre
	changedClearPost := cmd.Flags().Changed("clear-post-connect") && clearPost
	if !changedPre && !changedPost && !changedClearPre && !changedClearPost {
		return nil
	}

	preparedPre := cloneStringSlice(host.PreConnect)
	preparedPost := cloneStringSlice(host.PostConnect)

	if changedClearPre {
		preparedPre = nil
	}
	if changedClearPost {
		preparedPost = nil
	}
	if changedPre {
		commands, err := parseHookCommands(preConnect, "pre-connect")
		if err != nil {
			return err
		}
		preparedPre = commands
	}
	if changedPost {
		commands, err := parseHookCommands(postConnect, "post-connect")
		if err != nil {
			return err
		}
		preparedPost = commands
	}

	host.PreConnect = preparedPre
	host.PostConnect = preparedPost
	return nil
}

func applyUserProfileUpdateFlags(
	cmd *cobra.Command,
	cfg *store.PlainConfig,
	host *store.HostConfig,
	userRefFlag, userName, authTypeFlag, keyPath, password string,
) error {
	if cfg.Users == nil {
		cfg.Users = map[string]store.UserConfig{}
	}

	changedUserRef := cmd.Flags().Changed("user-ref")
	changedUser := cmd.Flags().Changed("user")
	changedAuthType := cmd.Flags().Changed("auth-type")
	changedKeyPath := cmd.Flags().Changed("key-path")
	changedPassword := cmd.Flags().Changed("password")

	if changedUserRef {
		targetRef := normalizeUserAlias(userRefFlag)
		if targetRef == "" {
			return errors.New("--user-ref cannot be empty")
		}
		if _, ok := cfg.Users[targetRef]; !ok {
			return fmt.Errorf("user profile %q not found", targetRef)
		}
		host.UserRef = targetRef
	}

	if !changedUser && !changedAuthType && !changedKeyPath && !changedPassword {
		return nil
	}

	targetRef := strings.TrimSpace(host.UserRef)
	if targetRef == "" {
		return errors.New("host has no user_ref; set --user-ref first")
	}

	userCfg, ok := cfg.Users[targetRef]
	if !ok {
		return fmt.Errorf("user profile %q not found", targetRef)
	}

	if changedUser {
		trimmed := strings.TrimSpace(userName)
		if trimmed == "" {
			return errors.New("--user cannot be empty")
		}
		userCfg.Name = trimmed
	}
	if strings.TrimSpace(userCfg.Name) == "" {
		return fmt.Errorf("user profile %q has empty name", targetRef)
	}

	newAuth, err := authConfigFromFlagValues(
		userCfg.Auth,
		authTypeFlag,
		keyPath,
		password,
		changedAuthType,
		changedKeyPath,
		changedPassword,
	)
	if err != nil {
		return err
	}
	userCfg.Auth = newAuth

	cfg.Users[targetRef] = userCfg
	return nil
}

func authConfigFromFlagValues(
	current store.AuthConfig,
	authTypeFlag, keyPath, password string,
	changedAuthType, changedKeyPath, changedPassword bool,
) (store.AuthConfig, error) {
	if changedKeyPath && changedPassword {
		return store.AuthConfig{}, errors.New("cannot set --key-path and --password at the same time")
	}

	if changedAuthType {
		authType := normalizeAuthType(authTypeFlag)
		if authType == "" {
			return store.AuthConfig{}, errors.New("--auth-type must be key or password")
		}
		switch authType {
		case "key":
			path := strings.TrimSpace(keyPath)
			if !changedKeyPath {
				path = strings.TrimSpace(current.KeyPath)
			}
			if path == "" {
				return store.AuthConfig{}, errors.New("key auth requires --key-path or existing key path")
			}
			return store.AuthConfig{Type: "key", KeyPath: path}, nil
		case "password":
			pw := password
			if !changedPassword {
				pw = current.Password
			}
			if strings.TrimSpace(pw) == "" {
				return store.AuthConfig{}, errors.New("password auth requires --password or existing password")
			}
			return store.AuthConfig{Type: "password", Password: pw}, nil
		}
	}

	if changedKeyPath {
		path := strings.TrimSpace(keyPath)
		if path == "" {
			return store.AuthConfig{}, errors.New("--key-path cannot be empty")
		}
		return store.AuthConfig{Type: "key", KeyPath: path}, nil
	}

	if changedPassword {
		if strings.TrimSpace(password) == "" {
			return store.AuthConfig{}, errors.New("--password cannot be empty")
		}
		return store.AuthConfig{Type: "password", Password: password}, nil
	}

	if normalized := normalizeAuthType(current.Type); normalized != "" {
		current.Type = normalized
		return current, nil
	}
	return current, nil
}

func loadConfig(opts *rootOptions, repo store.Repository) (store.PlainConfig, []byte, error) {
	cache, err := opts.passphraseStore(repo.Path)
	if err != nil {
		return store.PlainConfig{}, nil, err
	}
	if cachedPassphrase, ok, _ := cache.Get(); ok {
		cfg, loadErr := repo.Load(cachedPassphrase)
		if loadErr == nil {
			return cfg, cachedPassphrase, nil
		}
		wipe(cachedPassphrase)
		_ = cache.Clear()
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

	return store.HostConfig{
		Host:        host,
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

func redactConfigForDump(cfg store.PlainConfig) store.PlainConfig {
	redacted := store.NewPlainConfig()

	for alias, userCfg := range cfg.Users {
		userCopy := userCfg
		if normalizeAuthType(userCopy.Auth.Type) == "password" && strings.TrimSpace(userCopy.Auth.Password) != "" {
			userCopy.Auth.Password = redactedSecretValue
		}
		redacted.Users[alias] = userCopy
	}

	for alias, hostCfg := range cfg.Hosts {
		hostCopy := hostCfg
		if len(hostCfg.Env) > 0 {
			hostCopy.Env = make(map[string]string, len(hostCfg.Env))
			for key := range hostCfg.Env {
				hostCopy.Env[key] = redactedSecretValue
			}
		}
		redacted.Hosts[alias] = hostCopy
	}

	return redacted
}

func parseEnvAssignments(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, raw := range values {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, errors.New("env entry cannot be empty")
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env entry %q, expected KEY=VALUE", raw)
		}
		key := strings.TrimSpace(parts[0])
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid env key %q", key)
		}
		result[key] = parts[1]
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func parseEnvKeys(values []string) ([]string, error) {
	keys := make([]string, 0, len(values))
	for _, raw := range values {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("env key cannot be empty")
		}
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid env key %q", key)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func parseHookCommands(values []string, flagName string) ([]string, error) {
	commands := make([]string, 0, len(values))
	for i, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%s command at index %d is empty", flagName, i)
		}
		commands = append(commands, trimmed)
	}
	if len(commands) == 0 {
		return nil, nil
	}
	return commands, nil
}

func appendSendEnvOptions(args []string, envMap map[string]string) []string {
	if len(envMap) == 0 {
		return args
	}
	keys := sortedStringMapKeys(envMap)
	for _, key := range keys {
		args = append(args, "-o", "SendEnv="+key)
	}
	return args
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	merged := map[string]string{}
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	for key, value := range overrides {
		merged[key] = value
	}

	keys := sortedStringMapKeys(merged)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+merged[key])
	}
	return result
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func sanitizeHookCommands(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	out := promptWriter()
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
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

func promptYesNo(reader *bufio.Reader, label string, defaultYes bool) (bool, error) {
	defaultValue := "n"
	if defaultYes {
		defaultValue = "y"
	}

	for {
		value, err := promptOptional(reader, label+" (y/N)", defaultValue)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes":
			return true, nil
		case "n", "no", "":
			return false, nil
		default:
			fmt.Fprintln(promptWriter(), "Please enter y or n.")
		}
	}
}

func promptPort(reader *bufio.Reader, defaultPort int) (int, error) {
	out := promptWriter()
	for {
		raw, err := promptOptional(reader, "Port", strconv.Itoa(defaultPort))
		if err != nil {
			return 0, err
		}
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port <= 0 || port > 65535 {
			fmt.Fprintln(out, "Port must be a number between 1 and 65535.")
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

func promptRequiredPassword(prompt string) ([]byte, error) {
	out := promptWriter()
	fmt.Fprint(out, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
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
	out := promptWriter()
	fmt.Fprint(out, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return nil, false, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, false, nil
	}
	return secret, true, nil
}

func promptWriter() io.Writer {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr
	}
	return os.Stdout
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
