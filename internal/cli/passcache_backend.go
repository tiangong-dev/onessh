package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cacheBackendMemory = "memory"
	defaultCacheTTL    = 10 * time.Minute
)

type passphraseStore interface {
	IsEnabled() bool
	Get() ([]byte, bool, error)
	Set([]byte) error
	Clear() error
}

func defaultAgentSocketFlagValue() string {
	if raw := strings.TrimSpace(os.Getenv("ONESSH_AGENT_SOCKET")); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(os.Getenv("SHUSH_SOCKET")); raw != "" {
		return raw
	}
	return ""
}

func (o *rootOptions) passphraseStore(dataPath string) (passphraseStore, error) {
	if o == nil {
		return nil, errors.New("root options are required")
	}
	return newPassphraseAgentClient(canonicalCacheKey(dataPath), o.cacheTTL, o.noCache, o.agentSocket)
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultCacheTTL
	}
	return ttl
}

func canonicalCacheKey(dataPath string) string {
	path := strings.TrimSpace(dataPath)
	if path == "" {
		return ""
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return filepath.Clean(resolvedPath)
	}
	return filepath.Clean(absPath)
}
