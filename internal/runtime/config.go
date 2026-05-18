package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const DefaultCacheTTL = 10 * time.Minute

type RuntimeContext struct {
	DataPath        string
	CacheTTL        time.Duration
	NoCache         bool
	AgentSocket     string
	AgentCapability string
	Quiet           bool
	Audit           AuditRuntimeConfig
	IO              IOStreams
}

type AuditRuntimeConfig struct {
	Enabled    bool
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

func NormalizeCacheTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultCacheTTL
	}
	return ttl
}

func ResolveDataPath(customPath, envPath, homeDir string) (string, error) {
	if strings.TrimSpace(customPath) != "" {
		return expandPath(customPath, homeDir)
	}
	if strings.TrimSpace(envPath) != "" {
		return expandPath(envPath, homeDir)
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", fmt.Errorf("resolve home directory: home directory is empty")
	}
	return filepath.Join(homeDir, ".config", "onessh", "data"), nil
}

func ResolveAgentCapability(explicit, envValue, sessionID string) string {
	if raw := strings.TrimSpace(explicit); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(envValue); raw != "" {
		return raw
	}
	return DeriveAgentCapability(sessionID)
}

func DeriveAgentCapability(sessionID string) string {
	sum := sha256.Sum256([]byte("onessh:agent:cap:v1:" + sessionID))
	return hex.EncodeToString(sum[:])
}

func expandPath(path, homeDir string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" {
		if strings.TrimSpace(homeDir) == "" {
			return "", fmt.Errorf("expand ~: home directory is empty")
		}
		return homeDir, nil
	}
	if strings.HasPrefix(path, "~/") {
		if strings.TrimSpace(homeDir) == "" {
			return "", fmt.Errorf("expand ~: home directory is empty")
		}
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}
