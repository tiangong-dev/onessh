package cli

import (
	"errors"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func authConfigFromFlags(authType, keyPath, password string) (store.AuthConfig, error) {
	if strings.TrimSpace(keyPath) != "" && strings.TrimSpace(password) != "" {
		return store.AuthConfig{}, errors.New("cannot set --key-path and --password at the same time")
	}

	auth := store.AuthConfig{Type: authType}
	switch authType {
	case "key":
		if strings.TrimSpace(password) != "" {
			return store.AuthConfig{}, errors.New("--password is only valid when --auth-type=password")
		}
		if strings.TrimSpace(keyPath) == "" {
			return store.AuthConfig{}, errors.New("key-path is required when auth-type=key")
		}
		auth.KeyPath = strings.TrimSpace(keyPath)
	case "password":
		if strings.TrimSpace(keyPath) != "" {
			return store.AuthConfig{}, errors.New("--key-path is only valid when --auth-type=key")
		}
		if strings.TrimSpace(password) == "" {
			prompted, err := promptRequiredPassword("SSH password: ")
			if err != nil {
				return store.AuthConfig{}, err
			}
			auth.Password = string(prompted)
			wipe(prompted)
		} else {
			auth.Password = password
		}
	default:
		return store.AuthConfig{}, errors.New("auth-type must be key or password")
	}
	return auth, nil
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
				prompted, err := promptRequiredPassword("SSH password: ")
				if err != nil {
					return store.AuthConfig{}, err
				}
				pw = string(prompted)
				wipe(prompted)
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

func validateUserAuthFlagUsage(cmd *cobra.Command, authType, keyPath, password string) error {
	if cmd == nil {
		return errors.New("command is required")
	}

	changedAuthType := cmd.Flags().Changed("auth-type")
	changedKeyPath := cmd.Flags().Changed("key-path")
	changedPassword := cmd.Flags().Changed("password")

	if !changedAuthType && !changedKeyPath && !changedPassword {
		return nil
	}
	if changedKeyPath && changedPassword {
		return errors.New("cannot set --key-path and --password at the same time")
	}
	if (changedKeyPath || changedPassword) && !changedAuthType {
		return errors.New("--auth-type is required when setting --key-path or --password")
	}
	if !changedAuthType {
		return nil
	}

	normalized := normalizeAuthType(authType)
	if normalized == "" {
		return errors.New("--auth-type must be key or password")
	}
	if normalized == "key" && strings.TrimSpace(password) != "" {
		return errors.New("--password is only valid when --auth-type=password")
	}
	if normalized == "password" && strings.TrimSpace(keyPath) != "" {
		return errors.New("--key-path is only valid when --auth-type=key")
	}
	return nil
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
