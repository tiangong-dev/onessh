package cli

import (
	"bufio"
	"fmt"
	"os"

	"onessh/internal/store"

	"github.com/manifoldco/promptui"
	"golang.org/x/term"
)

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
