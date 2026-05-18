package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	infraagent "onessh/internal/infra/agent"
	appruntime "onessh/internal/runtime"
)

const (
	defaultCacheTTL = appruntime.DefaultCacheTTL
	// #nosec G101 -- this is a public cache-key namespace, not secret material.
	cacheKeyNamespaceV1      = "onessh:passphrase:v1:"
	onesshAgentCapabilityEnv = infraagent.AgentCapabilityEnv
)

type passphraseStore interface {
	IsEnabled() bool
	Get() ([]byte, bool, error)
	Set([]byte) error
	Clear() error
}

func defaultAgentSocketFlagValue() string {
	return infraagent.DefaultSocketFlagValue()
}

func defaultAgentCapabilityFlagValue() string {
	return infraagent.DefaultCapabilityFlagValue()
}

func resolveAgentCapability(explicit string) string {
	return infraagent.ResolveCapabilityFromEnv(explicit)
}

func defaultAgentSessionID() string {
	return infraagent.DefaultSessionID()
}

func deriveSessionCapability(sessionID string) string {
	return infraagent.DeriveSessionCapability(sessionID)
}

func defaultAgentSocketPath() (string, error) {
	return infraagent.DefaultSocketPathFromEnv()
}

func (o *rootOptions) passphraseStore(dataPath string) (passphraseStore, error) {
	if o == nil {
		return nil, errors.New("root options are required")
	}
	cacheKey := passphraseCacheKey(dataPath)
	return newPassphraseAgentClient(cacheKey, o.cacheTTL, o.noCache, o.agentSocket, o.agentCapability)
}

func normalizeTTL(ttl time.Duration) time.Duration {
	return appruntime.NormalizeCacheTTL(ttl)
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
	return cacheKeyNamespaceV1 + canonicalCacheKey(dataPath)
}
