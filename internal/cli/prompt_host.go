package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"onessh/internal/store"

	"github.com/manifoldco/promptui"
)

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

	proxyJump, err := promptOptional(inputReader, "Proxy jump (alias or user@host:port, leave empty to skip)", defaultProxyJump)
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
