package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func newUserCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage reusable user profiles",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newUserListCmd(opts),
		newUserShowCmd(opts),
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

func newUserShowCmd(opts *rootOptions) *cobra.Command {
	var outputFormat string
	var showSecrets bool

	cmd := &cobra.Command{
		Use:   "show <user-alias>",
		Short: "Show detailed information for a user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedFormat, err := validateOutputFormat(outputFormat, "table", "yaml")
			if err != nil {
				return err
			}

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
				return fmt.Errorf("user profile %q not found", alias)
			}

			if normalizedFormat == "yaml" {
				outCfg := store.PlainConfig{
					Hosts: map[string]store.HostConfig{},
					Users: map[string]store.UserConfig{alias: userCfg},
				}
				if !showSecrets {
					outCfg = redactConfigForDump(outCfg)
				}
				return renderHostDetailsYAML(cmd.OutOrStdout(), outCfg)
			}

			// Table output
			fmt.Fprintf(cmd.OutOrStdout(), "Alias:     %s\n", alias)
			fmt.Fprintf(cmd.OutOrStdout(), "User:      %s\n", userCfg.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Auth:      %s\n", summarizeAuth(userCfg.Auth))
			usedBy := hostAliasesUsingUser(cfg, alias)
			if len(usedBy) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Used by:   %s\n", strings.Join(usedBy, ", "))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Used by:   -\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|yaml)")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "Include sensitive values (only applies to yaml output)")
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
			if err := validateUserAuthFlagUsage(cmd, authType, keyPath, password); err != nil {
				return err
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
			if err := validateUserAuthFlagUsage(cmd, authType, keyPath, password); err != nil {
				return err
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
	var clearAllConfigs bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear cached master password for current config",
		Long: "Clear the cached master password for the current data directory.\n\n" +
			"Use --all to clear cached master passwords for every data directory in this agent (other agent keys are left intact).\n\n" +
			"To clear all cached passwords and tokens in the agent, use 'onessh agent clear-all'.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clearAllConfigs {
				socketPath, err := resolveAgentSocketPath(opts.agentSocket)
				if err != nil {
					return err
				}
				if err := clearPassphraseCacheByPrefix(
					socketPath,
					passphraseCacheKeyPrefixV1,
					resolveAgentCapability(opts.agentCapability),
				); err != nil {
					return fmt.Errorf("clear cached passphrases: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✔ all cached master passwords cleared")
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
	cmd.Flags().BoolVar(&clearAllConfigs, "all", false, "Clear cached master passwords for all data directories in this agent")
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
