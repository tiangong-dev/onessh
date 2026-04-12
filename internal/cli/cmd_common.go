package cli

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/store"
)

func validateOutputFormat(value string, allowed ...string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(allowed) == 0 {
		return normalized, nil
	}
	for _, candidate := range allowed {
		if normalized == strings.ToLower(strings.TrimSpace(candidate)) {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("unsupported output format %q (allowed: %s)", value, strings.Join(allowed, "|"))
}

func loadConfig(opts *rootOptions, repo store.Repository) (store.PlainConfig, []byte, error) {
	cache, err := opts.passphraseStore(repo.Path)
	if err != nil {
		return store.PlainConfig{}, nil, err
	}
	if cachedPassphrase, ok, _ := cache.Get(); ok {
		cfg, loadErr := repo.Load(cachedPassphrase)
		if loadErr == nil {
			return cfg, cachedPassphrase, nil
		}
		wipe(cachedPassphrase)
		_ = cache.Clear()
	}

	passphrase, err := promptRequiredPassword("Enter master password: ")
	if err != nil {
		return store.PlainConfig{}, nil, err
	}

	cfg, err := repo.Load(passphrase)
	if err != nil {
		wipe(passphrase)
		if errors.Is(err, store.ErrConfigNotFound) {
			return store.PlainConfig{}, nil, fmt.Errorf("%w (run `onessh init` first)", err)
		}
		return store.PlainConfig{}, nil, err
	}

	if cache.IsEnabled() {
		_ = cache.Set(passphrase)
	}

	return cfg, passphrase, nil
}

func redactConfigForDump(cfg store.PlainConfig) store.PlainConfig {
	redacted := store.NewPlainConfig()

	for alias, userCfg := range cfg.Users {
		userCopy := userCfg
		if normalizeAuthType(userCopy.Auth.Type) == "password" && strings.TrimSpace(userCopy.Auth.Password) != "" {
			userCopy.Auth.Password = redactedSecretValue
		}
		redacted.Users[alias] = userCopy
	}

	for alias, hostCfg := range cfg.Hosts {
		hostCopy := hostCfg
		if len(hostCfg.Env) > 0 {
			hostCopy.Env = make(map[string]string, len(hostCfg.Env))
			for key := range hostCfg.Env {
				hostCopy.Env[key] = redactedSecretValue
			}
		}
		redacted.Hosts[alias] = hostCopy
	}

	return redacted
}
