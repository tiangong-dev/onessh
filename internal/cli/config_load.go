package cli

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/infra/repository"
	"onessh/internal/store"
)

func loadConfig(opts *rootOptions, repo repository.Repository) (store.PlainConfig, []byte, error) {
	cache, err := opts.passphraseStore(repo.Path)
	if err != nil {
		return store.PlainConfig{}, nil, err
	}
	if cachedPassphrase, ok, _ := cache.Get(); ok {
		cfg, loadErr := repo.Load(cachedPassphrase)
		if loadErr == nil {
			return cfg, cachedPassphrase, nil
		}
		// Only invalidate the cache when the cached passphrase is provably wrong.
		// Other errors (e.g. I/O, ErrConfigNotFound) leave the cache intact and propagate.
		if errors.Is(loadErr, store.ErrInvalidPassword) {
			wipe(cachedPassphrase)
			_ = cache.Clear()
		} else {
			wipe(cachedPassphrase)
			return store.PlainConfig{}, nil, loadErr
		}
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
