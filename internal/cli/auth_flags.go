package cli

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/domain"

	"github.com/spf13/cobra"
)

// passwordFlagHelp is the shared help text for the --password CLI flag. The
// warning is intentionally surfaced in --help so users are nudged toward the
// interactive prompt before they paste a plaintext password.
const passwordFlagHelp = "SSH password when auth-type=password (WARNING: visible in shell history and process list; prefer interactive prompt)"

// hostsPasswordFlagHelp mirrors passwordFlagHelp for the host update command.
const hostsPasswordFlagHelp = "Update linked user password (WARNING: visible in shell history and process list; prefer interactive prompt)"

// warnIfPasswordFlagInsecure emits a one-line stderr warning when the user
// passed a non-empty --password value on the command line. The warning is
// non-fatal: --password remains supported for scripted/non-interactive use.
func warnIfPasswordFlagInsecure(cmd *cobra.Command, password string) {
	if cmd == nil || !cmd.Flags().Changed("password") {
		return
	}
	if strings.TrimSpace(password) == "" {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "onessh: --password is insecure (visible in ps/history); use interactive prompt or --password-stdin if available")
}

func authConfigFromFlags(authType, keyPath, password string) (domain.AuthConfig, error) {
	if strings.TrimSpace(keyPath) != "" && strings.TrimSpace(password) != "" {
		return domain.AuthConfig{}, errors.New("cannot set --key-path and --password at the same time")
	}

	auth := domain.AuthConfig{Type: authType}
	switch authType {
	case "key":
		if strings.TrimSpace(password) != "" {
			return domain.AuthConfig{}, errors.New("--password is only valid when --auth-type=password")
		}
		if strings.TrimSpace(keyPath) == "" {
			return domain.AuthConfig{}, errors.New("key-path is required when auth-type=key")
		}
		auth.KeyPath = strings.TrimSpace(keyPath)
	case "password":
		if strings.TrimSpace(keyPath) != "" {
			return domain.AuthConfig{}, errors.New("--key-path is only valid when --auth-type=key")
		}
		if strings.TrimSpace(password) == "" {
			prompted, err := promptRequiredPassword("SSH password: ")
			if err != nil {
				return domain.AuthConfig{}, err
			}
			// password string copy stays in heap; Go has no API to wipe immutable strings
			auth.Password = string(prompted)
			wipe(prompted)
		} else {
			auth.Password = password
		}
	default:
		return domain.AuthConfig{}, errors.New("auth-type must be key or password")
	}
	return auth, nil
}

func authConfigFromFlagValues(
	current domain.AuthConfig,
	authTypeFlag, keyPath, password string,
	changedAuthType, changedKeyPath, changedPassword bool,
) (domain.AuthConfig, error) {
	if changedKeyPath && changedPassword {
		return domain.AuthConfig{}, errors.New("cannot set --key-path and --password at the same time")
	}

	if changedAuthType {
		authType := normalizeAuthType(authTypeFlag)
		if authType == "" {
			return domain.AuthConfig{}, errors.New("--auth-type must be key or password")
		}
		switch authType {
		case "key":
			path := strings.TrimSpace(keyPath)
			if !changedKeyPath {
				path = strings.TrimSpace(current.KeyPath)
			}
			if path == "" {
				return domain.AuthConfig{}, errors.New("key auth requires --key-path or existing key path")
			}
			return domain.AuthConfig{Type: "key", KeyPath: path}, nil
		case "password":
			pw := password
			if !changedPassword {
				pw = current.Password
			}
			if strings.TrimSpace(pw) == "" {
				prompted, err := promptRequiredPassword("SSH password: ")
				if err != nil {
					return domain.AuthConfig{}, err
				}
				// password string copy stays in heap; Go has no API to wipe immutable strings
				pw = string(prompted)
				wipe(prompted)
			}
			return domain.AuthConfig{Type: "password", Password: pw}, nil
		}
	}

	if changedKeyPath {
		path := strings.TrimSpace(keyPath)
		if path == "" {
			return domain.AuthConfig{}, errors.New("--key-path cannot be empty")
		}
		return domain.AuthConfig{Type: "key", KeyPath: path}, nil
	}

	if changedPassword {
		if strings.TrimSpace(password) == "" {
			return domain.AuthConfig{}, errors.New("--password cannot be empty")
		}
		return domain.AuthConfig{Type: "password", Password: password}, nil
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
