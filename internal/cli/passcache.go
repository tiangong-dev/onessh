package cli

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultCacheTTL = 10 * time.Minute

type passphraseCache struct {
	path       string
	ttl        time.Duration
	configPath string
}

type passphraseCacheDoc struct {
	ExpiresAt  int64  `json:"expires_at"`
	SecretB64  string `json:"secret_b64"`
	ConfigPath string `json:"config_path,omitempty"`
}

func newPassphraseCache(configPath string, ttl time.Duration, disabled bool) (passphraseCache, error) {
	if disabled {
		return passphraseCache{}, nil
	}
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}

	cachePath := os.Getenv("ONESSH_CACHE_FILE")
	if cachePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return passphraseCache{}, fmt.Errorf("resolve home directory: %w", err)
		}
		cachePath = filepath.Join(homeDir, ".onessh", "masterpass.cache")
	}

	return passphraseCache{
		path:       cachePath,
		ttl:        ttl,
		configPath: configPath,
	}, nil
}

func (c passphraseCache) IsEnabled() bool {
	return c.path != ""
}

func (c passphraseCache) Get() ([]byte, bool, error) {
	if !c.IsEnabled() {
		return nil, false, nil
	}

	raw, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read cache file: %w", err)
	}

	var doc passphraseCacheDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		_ = c.Clear()
		return nil, false, nil
	}

	if doc.ExpiresAt <= time.Now().Unix() {
		_ = c.Clear()
		return nil, false, nil
	}
	if doc.ConfigPath != "" && doc.ConfigPath != c.configPath {
		_ = c.Clear()
		return nil, false, nil
	}

	passphrase, err := base64.StdEncoding.DecodeString(doc.SecretB64)
	if err != nil {
		_ = c.Clear()
		return nil, false, nil
	}
	if len(passphrase) == 0 {
		_ = c.Clear()
		return nil, false, nil
	}

	return passphrase, true, nil
}

func (c passphraseCache) Set(passphrase []byte) error {
	if !c.IsEnabled() {
		return nil
	}
	if len(passphrase) == 0 {
		return nil
	}

	doc := passphraseCacheDoc{
		ExpiresAt:  time.Now().Add(c.ttl).Unix(),
		SecretB64:  base64.StdEncoding.EncodeToString(passphrase),
		ConfigPath: c.configPath,
	}

	encoded, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode cache file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(c.path), ".onessh-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp cache file: %w", err)
	}
	tempName := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}

	if err := tempFile.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp cache file: %w", err)
	}
	if _, err := tempFile.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write temp cache file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp cache file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temp cache file: %w", err)
	}
	if err := os.Rename(tempName, c.path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("replace cache file: %w", err)
	}
	if err := os.Chmod(c.path, 0o600); err != nil {
		return fmt.Errorf("chmod cache file: %w", err)
	}
	return nil
}

func (c passphraseCache) Clear() error {
	if !c.IsEnabled() {
		return nil
	}
	if err := os.Remove(c.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
