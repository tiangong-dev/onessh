package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound  = errors.New("config file not found")
	ErrInvalidPassword = errors.New("invalid master password or corrupted config")
)

type Repository struct {
	Path string
}

func ResolvePath(customPath string) (string, error) {
	if customPath != "" {
		return expandPath(customPath)
	}
	if fromEnv := os.Getenv("ONESSH_CONFIG"); fromEnv != "" {
		return expandPath(fromEnv)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".onessh", "config.enc"), nil
}

func (r Repository) Exists() bool {
	_, err := os.Stat(r.Path)
	return err == nil
}

func (r Repository) Load(passphrase []byte) (PlainConfig, error) {
	raw, err := os.ReadFile(r.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlainConfig{}, ErrConfigNotFound
		}
		return PlainConfig{}, fmt.Errorf("read config: %w", err)
	}

	plaintext, err := decrypt(raw, passphrase)
	if err != nil {
		return PlainConfig{}, err
	}
	defer zeroBytes(plaintext)

	var cfg PlainConfig
	if err := yaml.Unmarshal(plaintext, &cfg); err != nil {
		return PlainConfig{}, fmt.Errorf("decode yaml: %w", err)
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	return cfg, nil
}

func (r Repository) Save(cfg PlainConfig, passphrase []byte) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}

	plaintext, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	defer zeroBytes(plaintext)

	encrypted, err := encrypt(plaintext, passphrase)
	if err != nil {
		return fmt.Errorf("encrypt config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(r.Path), ".onessh-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}

	if err := tempFile.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("set temp file permission: %w", err)
	}
	if _, err := tempFile.Write(encrypted); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempName, r.Path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("replace config file: %w", err)
	}
	if err := os.Chmod(r.Path, 0o600); err != nil {
		return fmt.Errorf("set config file permission: %w", err)
	}

	return nil
}

func expandPath(input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}
	if input[:1] == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if input == "~" {
			return homeDir, nil
		}
		if len(input) > 1 && input[1] == '/' {
			return filepath.Join(homeDir, input[2:]), nil
		}
	}
	return filepath.Clean(input), nil
}
