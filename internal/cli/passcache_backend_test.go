package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultAgentSocketFlagValueFromEnv(t *testing.T) {
	t.Run("uses ONESSH when set", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "/tmp/onessh.sock")
		if got := defaultAgentSocketFlagValue(); got != "/tmp/onessh.sock" {
			t.Fatalf("expected ONESSH socket, got %q", got)
		}
	})

	t.Run("empty when unset", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		if got := defaultAgentSocketFlagValue(); got != "" {
			t.Fatalf("expected empty value, got %q", got)
		}
	})
}

func TestDefaultAgentCapabilityFlagValueFromEnv(t *testing.T) {
	t.Run("uses ONESSH when set", func(t *testing.T) {
		t.Setenv(onesshAgentCapabilityEnv, "onessh-cap")
		if got := defaultAgentCapabilityFlagValue(); got != "onessh-cap" {
			t.Fatalf("expected ONESSH capability, got %q", got)
		}
	})

	t.Run("empty when unset", func(t *testing.T) {
		t.Setenv(onesshAgentCapabilityEnv, "")
		if got := defaultAgentCapabilityFlagValue(); got != "" {
			t.Fatalf("expected empty value, got %q", got)
		}
	})
}

func TestResolveAgentCapabilityUsesSessionScopedFallback(t *testing.T) {
	t.Setenv(onesshAgentCapabilityEnv, "")
	first := resolveAgentCapability("")
	if len(first) != 64 {
		t.Fatalf("expected 64-char hex capability, got %d", len(first))
	}
	want := deriveSessionCapability(defaultAgentSessionID())
	if first != want {
		t.Fatalf("expected session-derived capability, want=%q got=%q", want, first)
	}
	second := resolveAgentCapability("")
	if second != first {
		t.Fatalf("expected deterministic session capability, first=%q second=%q", first, second)
	}

	if deriveSessionCapability("uid:1000:ppid:1") == deriveSessionCapability("uid:1000:ppid:2") {
		t.Fatalf("expected different capability for different session ids")
	}
}

func TestResolveAgentSocketPathPrecedence(t *testing.T) {
	t.Run("explicit path has highest priority", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "/tmp/onessh.sock")
		got, err := resolveAgentSocketPath("/tmp/custom.sock")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != "/tmp/custom.sock" {
			t.Fatalf("expected custom socket, got %q", got)
		}
	})

	t.Run("uses ONESSH env when no explicit path", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "/tmp/onessh.sock")
		got, err := resolveAgentSocketPath("")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != "/tmp/onessh.sock" {
			t.Fatalf("expected ONESSH socket, got %q", got)
		}
	})

	t.Run("default path when no env", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		want, err := defaultAgentSocketPath()
		if err != nil {
			t.Fatalf("defaultAgentSocketPath: %v", err)
		}
		got, err := resolveAgentSocketPath("")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != want {
			t.Fatalf("unexpected default socket path: want=%q got=%q", want, got)
		}
		suffix := filepath.Base(got)
		wantSuffix := "agent-" + strconv.Itoa(os.Getppid()) + ".sock"
		if suffix != wantSuffix {
			t.Fatalf("unexpected default socket filename: want=%q got=%q", wantSuffix, suffix)
		}
	})
}

func TestCanonicalCacheKeyNormalizesPathVariants(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	absKey := canonicalCacheKey(storeDir)
	if absKey == "" {
		t.Fatalf("expected non-empty canonical key")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	relativePath, err := filepath.Rel(cwd, storeDir)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	relKey := canonicalCacheKey(relativePath)
	if relKey != absKey {
		t.Fatalf("relative path should map to same key: abs=%q rel=%q", absKey, relKey)
	}

	symlinkPath := filepath.Join(t.TempDir(), "store-link")
	if err := os.Symlink(storeDir, symlinkPath); err != nil {
		t.Skipf("symlink is not available in this environment: %v", err)
	}
	linkKey := canonicalCacheKey(symlinkPath)
	if linkKey != absKey {
		t.Fatalf("symlink path should map to same key: abs=%q link=%q", absKey, linkKey)
	}
}

func TestPassphraseCacheKeyAddsNamespacePrefix(t *testing.T) {
	key := passphraseCacheKey("/tmp/onessh-store")
	if !strings.HasPrefix(key, passphraseCacheKeyPrefixV1) {
		t.Fatalf("expected namespaced cache key prefix, got %q", key)
	}
	if key == passphraseCacheKeyPrefixV1 {
		t.Fatalf("expected non-empty canonical component in key")
	}
}
