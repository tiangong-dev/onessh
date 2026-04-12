package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		if doc.Version != docVersion {
			return fmt.Errorf("unsupported user doc version for %s: %d", alias, doc.Version)
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

func (r Repository) loadHosts(cfg *PlainConfig, key []byte) error {
	files, err := os.ReadDir(r.hostsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read hosts directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		alias := strings.TrimSuffix(f.Name(), ".yaml")
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid host alias %q: %w", alias, err)
		}

		raw, err := os.ReadFile(filepath.Join(r.hostsDir(), f.Name()))
		if err != nil {
			return fmt.Errorf("read host %s: %w", alias, err)
		}

		var doc hostDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("decode host %s: %w", alias, err)
		}
		if doc.Version != docVersion {
			return fmt.Errorf("unsupported host doc version for %s: %d", alias, doc.Version)
		}

		hostValue, err := decryptStringField(doc.Host, key)
		if err != nil {
			return fmt.Errorf("decrypt host value for %s: %w", alias, err)
		}
		if strings.TrimSpace(hostValue) == "" {
			return fmt.Errorf("host %s has empty host", alias)
		}
		if strings.TrimSpace(doc.UserRef) == "" {
			return fmt.Errorf("host %s has empty user_ref", alias)
		}

		hostCfg := HostConfig{
			Host:        strings.TrimSpace(hostValue),
			Description: strings.TrimSpace(doc.Description),
			UserRef:     strings.TrimSpace(doc.UserRef),
			Port:        doc.Port,
			ProxyJump:   strings.TrimSpace(doc.ProxyJump),
			Tags:        doc.Tags,
			Env:         map[string]string{},
			PreConnect:  make([]string, 0, len(doc.PreConnect)),
			PostConnect: make([]string, 0, len(doc.PostConnect)),
		}
		if hostCfg.Port <= 0 {
			hostCfg.Port = 22
		}
		for k, encVal := range doc.Env {
			plainVal, err := decryptStringField(encVal, key)
			if err != nil {
				return fmt.Errorf("decrypt env for host %s key %s: %w", alias, k, err)
			}
			hostCfg.Env[k] = plainVal
		}
		if len(hostCfg.Env) == 0 {
			hostCfg.Env = nil
		}
		for i, encCmd := range doc.PreConnect {
			plainCmd, err := decryptStringField(encCmd, key)
			if err != nil {
				return fmt.Errorf("decrypt pre_connect for host %s index %d: %w", alias, i, err)
			}
			plainCmd = strings.TrimSpace(plainCmd)
			if plainCmd == "" {
				return fmt.Errorf("host %s has empty pre_connect command at index %d", alias, i)
			}
			hostCfg.PreConnect = append(hostCfg.PreConnect, plainCmd)
		}
		if len(hostCfg.PreConnect) == 0 {
			hostCfg.PreConnect = nil
		}
		for i, encCmd := range doc.PostConnect {
			plainCmd, err := decryptStringField(encCmd, key)
			if err != nil {
				return fmt.Errorf("decrypt post_connect for host %s index %d: %w", alias, i, err)
			}
			plainCmd = strings.TrimSpace(plainCmd)
			if plainCmd == "" {
				return fmt.Errorf("host %s has empty post_connect command at index %d", alias, i)
			}
			hostCfg.PostConnect = append(hostCfg.PostConnect, plainCmd)
		}
		if len(hostCfg.PostConnect) == 0 {
			hostCfg.PostConnect = nil
		}

		cfg.Hosts[alias] = hostCfg
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
			Version: docVersion,
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

func (r Repository) syncHosts(cfg PlainConfig, key []byte) error {
	if err := os.MkdirAll(r.hostsDir(), 0o700); err != nil {
		return fmt.Errorf("ensure hosts directory: %w", err)
	}

	aliases := sortedKeys(cfg.Hosts)
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid host alias %q: %w", alias, err)
		}

		hostCfg := cfg.Hosts[alias]
		hostValue := strings.TrimSpace(hostCfg.Host)
		if hostValue == "" {
			return fmt.Errorf("host %q has empty host", alias)
		}
		if strings.TrimSpace(hostCfg.UserRef) == "" {
			return fmt.Errorf("host %q has empty user_ref", alias)
		}
		if _, ok := cfg.Users[hostCfg.UserRef]; !ok {
			return fmt.Errorf("host %q references missing user profile %q", alias, hostCfg.UserRef)
		}

		doc := hostDoc{
			Version:     docVersion,
			Description: strings.TrimSpace(hostCfg.Description),
			UserRef:     strings.TrimSpace(hostCfg.UserRef),
			Port:        hostCfg.Port,
			ProxyJump:   strings.TrimSpace(hostCfg.ProxyJump),
			Tags:        hostCfg.Tags,
			Env:         map[string]string{},
			PreConnect:  make([]string, 0, len(hostCfg.PreConnect)),
			PostConnect: make([]string, 0, len(hostCfg.PostConnect)),
		}
		if doc.Port <= 0 {
			doc.Port = 22
		}

		var err error
		doc.Host, err = encryptStringField(hostValue, key)
		if err != nil {
			return fmt.Errorf("encrypt host value for %s: %w", alias, err)
		}

		for k, v := range hostCfg.Env {
			encVal, err := encryptStringField(v, key)
			if err != nil {
				return fmt.Errorf("encrypt env for host %s key %s: %w", alias, k, err)
			}
			doc.Env[k] = encVal
		}
		if len(doc.Env) == 0 {
			doc.Env = nil
		}
		for i, command := range hostCfg.PreConnect {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				return fmt.Errorf("host %q pre_connect command at index %d is empty", alias, i)
			}
			encCmd, err := encryptStringField(trimmed, key)
			if err != nil {
				return fmt.Errorf("encrypt pre_connect for host %s index %d: %w", alias, i, err)
			}
			doc.PreConnect = append(doc.PreConnect, encCmd)
		}
		if len(doc.PreConnect) == 0 {
			doc.PreConnect = nil
		}
		for i, command := range hostCfg.PostConnect {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				return fmt.Errorf("host %q post_connect command at index %d is empty", alias, i)
			}
			encCmd, err := encryptStringField(trimmed, key)
			if err != nil {
				return fmt.Errorf("encrypt post_connect for host %s index %d: %w", alias, i, err)
			}
			doc.PostConnect = append(doc.PostConnect, encCmd)
		}
		if len(doc.PostConnect) == 0 {
			doc.PostConnect = nil
		}

		if err := writeYAMLAtomic(filepath.Join(r.hostsDir(), alias+".yaml"), doc); err != nil {
			return err
		}
		seen[alias] = struct{}{}
	}

	return cleanupStaleYAMLFiles(r.hostsDir(), seen)
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return errors.New("alias is empty")
	}
	if !aliasPattern.MatchString(alias) {
		return errors.New("alias must match [A-Za-z0-9._-]+")
	}
	return nil
}

func normalizeAuthTypeStore(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "key":
		return "key"
	case "password":
		return "password"
	default:
		return ""
	}
}
