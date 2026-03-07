package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"onessh/internal/store"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
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
				// Check if this user is only used by this host
				inUseBy := hostAliasesUsingUser(cfg, userRef)
				if len(inUseBy) == 1 && inUseBy[0] == alias {
					// User is only used by this host, prompt whether to delete user together
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

			if normalizedFormat == "json" {
				rows := make([]map[string]interface{}, 0, len(aliases))
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
					tagStr := "-"
					if len(host.Tags) > 0 {
						tagStr = strings.Join(host.Tags, ",")
					}
					desc := "-"
					if host.Description != "" {
						desc = host.Description
					}
					rows = append(rows, map[string]interface{}{
						"alias":      alias,
						"desc":       desc,
						"host":       host.Host,
						"user":       userName,
						"user_ref":   userRef,
						"auth":       authType,
						"port":       port,
						"proxy_jump": proxyJump,
						"tags":       tagStr,
						"status":     status,
					})
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tDESC\tHOST\tUSER\tUSER_REF\tAUTH\tPORT\tPROXY_JUMP\tTAGS\tSTATUS")
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
				tagStr := "-"
				if len(host.Tags) > 0 {
					tagStr = strings.Join(host.Tags, ",")
				}
				desc := "-"
				if host.Description != "" {
					desc = host.Description
				}
				fmt.Fprintf(
					w,
					"%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
					alias,
					desc,
					host.Host,
					userName,
					userRef,
					authType,
					port,
					proxyJump,
					tagStr,
					status,
				)
			}
			_ = w.Flush()

			return nil
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
				outCfg := store.PlainConfig{
					Hosts: map[string]store.HostConfig{alias: host},
					Users: map[string]store.UserConfig{},
				}
				if host.UserRef != "" {
					if u, ok := cfg.Users[host.UserRef]; ok {
						outCfg.Users[host.UserRef] = u
					}
				}
				if !showSecrets {
					outCfg = redactConfigForDump(outCfg)
				}
				out, err := yaml.Marshal(outCfg)
				if err != nil {
					return fmt.Errorf("marshal yaml: %w", err)
				}
				_, err = cmd.OutOrStdout().Write(out)
				return err
			}

			out := cmd.OutOrStdout()
			port := host.Port
			if port <= 0 {
				port = 22
			}

			fmt.Fprintf(out, "Alias:        %s\n", alias)
			fmt.Fprintf(out, "Host:         %s\n", host.Host)
			if host.Description != "" {
				fmt.Fprintf(out, "Description:  %s\n", host.Description)
			}
			fmt.Fprintf(out, "Port:         %d\n", port)

			if host.UserRef != "" {
				fmt.Fprintf(out, "User Ref:     %s\n", host.UserRef)
				if userCfg, ok := cfg.Users[host.UserRef]; ok {
					fmt.Fprintf(out, "User:         %s\n", userCfg.Name)
					fmt.Fprintf(out, "Auth:         %s\n", summarizeAuth(userCfg.Auth))
				}
			}

			if host.ProxyJump != "" {
				fmt.Fprintf(out, "Proxy Jump:   %s\n", host.ProxyJump)
			}

			if len(host.Tags) > 0 {
				fmt.Fprintf(out, "Tags:         %s\n", strings.Join(host.Tags, ", "))
			}

			if len(host.Env) > 0 {
				fmt.Fprintf(out, "Env:\n")
				keys := sortedStringMapKeys(host.Env)
				for _, key := range keys {
					fmt.Fprintf(out, "  %s=%s\n", key, host.Env[key])
				}
			}

			if len(host.PreConnect) > 0 {
				fmt.Fprintf(out, "Pre Connect:\n")
				for _, c := range host.PreConnect {
					fmt.Fprintf(out, "  %s\n", c)
				}
			}

			if len(host.PostConnect) > 0 {
				fmt.Fprintf(out, "Post Connect:\n")
				for _, c := range host.PostConnect {
					fmt.Fprintf(out, "  %s\n", c)
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|yaml)")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "Include sensitive values (only applies to yaml output)")
	return cmd
}

func hasAnyHostUpdateFlags(cmd *cobra.Command) bool {
	checks := []string{
		"alias",
		"host",
		"port",
		"proxy-jump",
		"description",
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
		"tag",
		"untag",
		"clear-tags",
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

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func applyHostTagUpdateFlags(cmd *cobra.Command, host *store.HostConfig, tags, unsetTags []string, clearTags bool) {
	if !cmd.Flags().Changed("tag") && !cmd.Flags().Changed("untag") && !(cmd.Flags().Changed("clear-tags") && clearTags) {
		return
	}
	current := cloneStringSlice(host.Tags)
	if clearTags && cmd.Flags().Changed("clear-tags") {
		current = nil
	}
	if cmd.Flags().Changed("untag") {
		remove := map[string]struct{}{}
		for _, t := range unsetTags {
			remove[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
		}
		filtered := current[:0]
		for _, t := range current {
			if _, skip := remove[t]; !skip {
				filtered = append(filtered, t)
			}
		}
		current = filtered
	}
	if cmd.Flags().Changed("tag") {
		current = append(current, tags...)
	}
	host.Tags = normalizeTags(current)
}

func hostHasTag(host store.HostConfig, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, t := range host.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func printDryRunHosts(out io.Writer, cfg store.PlainConfig, aliases []string) {
	fmt.Fprintf(out, "Matched %d host(s):\n", len(aliases))
	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		port := host.Port
		if port <= 0 {
			port = 22
		}
		userName, _, err := resolveHostIdentity(cfg, host)
		if err != nil {
			fmt.Fprintf(out, "  %-20s %s (SKIP: %v)\n", alias, host.Host, err)
		} else {
			fmt.Fprintf(out, "  %-20s %s@%s:%d\n", alias, userName, host.Host, port)
		}
	}
}

func collectFilteredHosts(cfg store.PlainConfig, tag, filter string) []string {
	aliases := make([]string, 0, len(cfg.Hosts))
	for alias := range cfg.Hosts {
		if tag != "" && !hostHasTag(cfg.Hosts[alias], tag) {
			continue
		}
		if filter != "" && !matchHostFilter(alias, cfg.Hosts[alias], filter) {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func matchHostFilter(alias string, host store.HostConfig, pattern string) bool {
	if matched, _ := filepath.Match(pattern, alias); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, host.Host); matched {
		return true
	}
	if host.Description != "" {
		if matched, _ := filepath.Match(pattern, host.Description); matched {
			return true
		}
	}
	return false
}

// completionHostAliases returns a ValidArgsFunction that completes host aliases
// using the cached master password (silently skips completion if no cache is available).
func completionHostAliases(opts *rootOptions) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		repo, err := opts.repository()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cache, err := opts.passphraseStore(repo.Path)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		pass, ok, _ := cache.Get()
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer wipe(pass)
		cfg, err := repo.Load(pass)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		aliases := make([]string, 0, len(cfg.Hosts))
		for alias := range cfg.Hosts {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		return aliases, cobra.ShellCompDirectiveNoFileComp
	}
}
