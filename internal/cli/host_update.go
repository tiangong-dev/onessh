package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

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
