package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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

			opts.logEvent("init", "", "", "", "ok", nil)
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
		tags        []string
		description string
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
			if cmd.Flags().Changed("tag") {
				newHost.Tags = normalizeTags(tags)
			}
			if cmd.Flags().Changed("description") {
				newHost.Description = strings.TrimSpace(description)
			}
			cfg.Hosts[alias] = newHost

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			opts.logEvent("add_host", alias, newHost.Host, "", "ok", nil)
			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s added\n", alias)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&envFlags, "env", nil, "Host environment variable (KEY=VALUE), repeatable")
	cmd.Flags().StringArrayVar(&preConnect, "pre-connect", nil, "Remote command run before interactive shell, repeatable")
	cmd.Flags().StringArrayVar(&postConnect, "post-connect", nil, "Remote command run after interactive shell exits, repeatable")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Tag to assign to host, repeatable")
	cmd.Flags().StringVar(&description, "description", "", "Host description")
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
		tags         []string
		unsetTags    []string
		clearTags    bool
		descFlag     string
	)

	cmd := &cobra.Command{
		Use:               "update <host-alias>",
		Short:             "Update an existing host entry",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completionHostAliases(opts),
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
				if cmd.Flags().Changed("description") {
					updatedHost.Description = strings.TrimSpace(descFlag)
				}
				if err := applyHostEnvUpdateFlags(cmd, &updatedHost, envFlags, unsetEnv, clearEnv); err != nil {
					return err
				}
				if err := applyHostHookUpdateFlags(cmd, &updatedHost, preConnect, postConnect, clearPre, clearPost); err != nil {
					return err
				}
				applyHostTagUpdateFlags(cmd, &updatedHost, tags, unsetTags, clearTags)

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

			opts.logEvent("update_host", targetAlias, updatedHost.Host, "", "ok", nil)
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
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Add tag to host, repeatable")
	cmd.Flags().StringArrayVar(&unsetTags, "untag", nil, "Remove tag from host, repeatable")
	cmd.Flags().BoolVar(&clearTags, "clear-tags", false, "Remove all tags")
	cmd.Flags().StringVar(&descFlag, "description", "", "Update host description")
	return cmd
}

func newRmCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rm <host-alias>",
		Short:             "Remove a host entry",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completionHostAliases(opts),
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

			hostCfg, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q does not exist", alias)
			}

			userRef := strings.TrimSpace(hostCfg.UserRef)
			if userRef != "" {
				inUseBy := hostAliasesUsingUser(cfg, userRef)
				if len(inUseBy) == 1 && inUseBy[0] == alias {
					if term.IsTerminal(int(os.Stdin.Fd())) {
						reader := bufio.NewReader(os.Stdin)
						shouldDeleteUser, err := promptYesNo(reader, fmt.Sprintf("Also delete user profile %s", userRef), false)
						if err != nil {
							return err
						}
						if shouldDeleteUser {
							delete(cfg.Users, userRef)
						}
					}
				}
			}

			delete(cfg.Hosts, alias)

			if err := repo.Save(cfg, pass); err != nil {
				return err
			}

			opts.logEvent("rm_host", alias, hostCfg.Host, "", "ok", nil)
			fmt.Fprintf(cmd.OutOrStdout(), "✔ host %s removed\n", alias)
			return nil
		},
	}
	return cmd
}

func newLsCmd(opts *rootOptions) *cobra.Command {
	var (
		filterTag string
		filter    string
		format    string
		tagsOnly  bool
	)

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List hosts with summary information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			normalizedFormat, err := validateOutputFormat(format, "table", "json")
			if err != nil {
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

			if tagsOnly {
				seen := make(map[string]struct{})
				for _, host := range cfg.Hosts {
					for _, t := range host.Tags {
						t = strings.ToLower(strings.TrimSpace(t))
						if t != "" {
							seen[t] = struct{}{}
						}
					}
				}
				tags := make([]string, 0, len(seen))
				for t := range seen {
					tags = append(tags, t)
				}
				sort.Strings(tags)
				for _, t := range tags {
					fmt.Fprintln(cmd.OutOrStdout(), t)
				}
				return nil
			}

			aliases := collectFilteredHosts(cfg, filterTag, filter)
			rows := buildHostListRows(cfg, aliases)
			if normalizedFormat == "json" {
				return renderHostListJSON(cmd.OutOrStdout(), rows)
			}
			return renderHostListTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&filterTag, "tag", "", "Filter hosts by tag")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter hosts by glob pattern (matches alias, host, description)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table|json)")
	cmd.Flags().BoolVar(&tagsOnly, "tags", false, "List all tags used across hosts")
	return cmd
}

func newShowCmd(opts *rootOptions) *cobra.Command {
	var outputFormat string
	var showSecrets bool

	cmd := &cobra.Command{
		Use:               "show <host-alias>",
		Short:             "Show detailed information for a host",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completionHostAliases(opts),
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedFormat, err := validateOutputFormat(outputFormat, "table", "yaml")
			if err != nil {
				return err
			}

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

			host, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q not found", alias)
			}

			if normalizedFormat == "yaml" {
				outCfg := buildHostDumpConfig(cfg, alias, host)
				if !showSecrets {
					outCfg = redactConfigForDump(outCfg)
				}
				return renderHostDetailsYAML(cmd.OutOrStdout(), outCfg)
			}

			renderHostDetailsTable(cmd.OutOrStdout(), alias, host, cfg)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|yaml)")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "Include sensitive values (only applies to yaml output)")
	return cmd
}
