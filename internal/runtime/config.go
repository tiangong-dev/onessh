package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
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

func ResolveAgentCapability(explicit, envValue, sessionID string) string {
	if raw := strings.TrimSpace(explicit); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(envValue); raw != "" {
		return raw
	}
	return DeriveAgentCapability(sessionID)
}

// DeriveAgentCapability derives a per-session capability token from the shell
// session identifier (typically the parent shell PID). It is a deterministic
// SHA-256 over a fixed prefix concatenated with the session id.
//
// Capability is NOT a secret. Any process with the same UID can derive it.
// Treat it only as a session discriminator, not authentication. Strong
// isolation between same-UID processes requires an explicit, randomly
// generated capability supplied out of band.
func DeriveAgentCapability(sessionID string) string {
	sum := sha256.Sum256([]byte("onessh:agent:cap:v1:" + sessionID))
	return hex.EncodeToString(sum[:])
}
