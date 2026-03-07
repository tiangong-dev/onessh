package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	onesshAgentSessionEnv      = "ONESSH_AGENT_SESSION"
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
	if fromEnv := defaultAgentCapabilityFlagValue(); fromEnv != "" {
		return fromEnv
	}
	// Auto-derive a stable per-session capability to avoid manual export.
	return deriveSessionCapability(resolveAgentSessionIdentity())
}

func resolveAgentSessionIdentity() string {
	if raw := strings.TrimSpace(os.Getenv(onesshAgentSessionEnv)); raw != "" {
		return "env:" + raw
	}
	if tty, err := os.Readlink("/proc/self/fd/0"); err == nil && strings.TrimSpace(tty) != "" {
		return "tty:" + tty
	}
	return fmt.Sprintf("ppid:%d", os.Getppid())
}

func deriveSessionCapability(identity string) string {
	sum := sha256.Sum256([]byte("onessh:agent:cap:v1:" + identity))
	return hex.EncodeToString(sum[:])
}

func defaultAgentSocketPath() (string, error) {
	sessionID := resolveAgentSessionIdentity()
	sum := sha256.Sum256([]byte("onessh:agent:socket:v1:" + sessionID))
	socketName := "agent-" + hex.EncodeToString(sum[:8]) + ".sock"

	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		return filepath.Join(runtimeDir, "onessh", socketName), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "onessh", "agents", socketName), nil
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
