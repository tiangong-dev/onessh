package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tiangong-dev/shush"
)

const (
	defaultCacheTTL            = 10 * time.Minute
	passphraseCacheKeyPrefixV1 = "onessh:passphrase:v1:"
	onesshAgentCapabilityEnv   = "ONESSH_AGENT_CAPABILITY"
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

func defaultAgentCapabilityFlagValue() string {
	if raw := strings.TrimSpace(os.Getenv(onesshAgentCapabilityEnv)); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(os.Getenv(shush.EnvCapability)); raw != "" {
		return raw
	}
	return ""
}

func resolveAgentCapability(explicit string) string {
	if raw := strings.TrimSpace(explicit); raw != "" {
		return raw
	}
	return defaultAgentCapabilityFlagValue()
}

func (o *rootOptions) passphraseStore(dataPath string) (passphraseStore, error) {
	if o == nil {
		return nil, errors.New("root options are required")
	}
	cacheKey := passphraseCacheKey(dataPath)
	return newPassphraseAgentClient(cacheKey, o.cacheTTL, o.noCache, o.agentSocket, o.agentCapability)
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

func passphraseCacheKey(dataPath string) string {
	return passphraseCacheKeyPrefixV1 + canonicalCacheKey(dataPath)
}
