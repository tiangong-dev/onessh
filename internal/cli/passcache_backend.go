package cli

import (
	"errors"
	"os"
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
	return ""
}

func (o *rootOptions) passphraseStore(configPath string) (passphraseStore, error) {
	if o == nil {
		return nil, errors.New("root options are required")
	}
	return newPassphraseAgentClient(configPath, o.cacheTTL, o.noCache, o.agentSocket)
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultCacheTTL
	}
	return ttl
}
