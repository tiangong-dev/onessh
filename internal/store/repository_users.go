package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func (r Repository) loadUsers(cfg *PlainConfig, key []byte) error {
	files, err := os.ReadDir(r.usersDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read users directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		alias := strings.TrimSuffix(f.Name(), ".yaml")
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid user alias %q: %w", alias, err)
		}

		raw, err := os.ReadFile(filepath.Join(r.usersDir(), f.Name()))
		if err != nil {
			return fmt.Errorf("read user %s: %w", alias, err)
		}

		var doc userDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("decode user %s: %w", alias, err)
		}
		if err := validateReadableUserDocVersion(alias, doc.Version); err != nil {
			return err
		}

		name, err := decryptStringField(doc.Name, key)
		if err != nil {
			return fmt.Errorf("decrypt user name for %s: %w", alias, err)
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("user %s has empty name", alias)
		}

		authType := normalizeAuthTypeStore(doc.Auth.Type)
		if authType == "" {
			return fmt.Errorf("user %s has invalid auth type", alias)
		}

		userCfg := UserConfig{Name: strings.TrimSpace(name), Auth: AuthConfig{Type: authType}}
		switch authType {
		case "key":
			keyPath, err := decryptStringField(doc.Auth.KeyPath, key)
			if err != nil {
				return fmt.Errorf("decrypt key_path for user %s: %w", alias, err)
			}
			if strings.TrimSpace(keyPath) == "" {
				return fmt.Errorf("user %s has empty key_path", alias)
			}
			userCfg.Auth.KeyPath = strings.TrimSpace(keyPath)
		case "password":
			password, err := decryptStringField(doc.Auth.Password, key)
			if err != nil {
				return fmt.Errorf("decrypt password for user %s: %w", alias, err)
			}
			if strings.TrimSpace(password) == "" {
				return fmt.Errorf("user %s has empty password", alias)
			}
			userCfg.Auth.Password = password
		}

		cfg.Users[alias] = userCfg
	}
	return nil
}

func (r Repository) syncUsers(cfg PlainConfig, key []byte) error {
	if err := os.MkdirAll(r.usersDir(), 0o700); err != nil {
		return fmt.Errorf("ensure users directory: %w", err)
	}

	aliases := sortedKeys(cfg.Users)
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid user alias %q: %w", alias, err)
		}

		userCfg := cfg.Users[alias]
		userName := strings.TrimSpace(userCfg.Name)
		if userName == "" {
			return fmt.Errorf("user profile %q has empty name", alias)
		}

		authType := normalizeAuthTypeStore(userCfg.Auth.Type)
		if authType == "" {
			return fmt.Errorf("user profile %q has invalid auth type", alias)
		}

		doc := userDoc{
			Version: userDocWriteVersion,
			Auth: userAuthDoc{
				Type: authType,
			},
		}

		var err error
		doc.Name, err = encryptStringField(userName, key)
		if err != nil {
			return fmt.Errorf("encrypt user name for %s: %w", alias, err)
		}

		switch authType {
		case "key":
			keyPath := strings.TrimSpace(userCfg.Auth.KeyPath)
			if keyPath == "" {
				return fmt.Errorf("user profile %q key auth requires key_path", alias)
			}
			doc.Auth.KeyPath, err = encryptStringField(keyPath, key)
			if err != nil {
				return fmt.Errorf("encrypt key_path for %s: %w", alias, err)
			}
		case "password":
			if strings.TrimSpace(userCfg.Auth.Password) == "" {
				return fmt.Errorf("user profile %q password auth requires password", alias)
			}
			doc.Auth.Password, err = encryptStringField(userCfg.Auth.Password, key)
			if err != nil {
				return fmt.Errorf("encrypt password for %s: %w", alias, err)
			}
		}

		if err := writeYAMLAtomic(filepath.Join(r.usersDir(), alias+".yaml"), doc); err != nil {
			return err
		}
		seen[alias] = struct{}{}
	}

	return cleanupStaleYAMLFiles(r.usersDir(), seen)
}
