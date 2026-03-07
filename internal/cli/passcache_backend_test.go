package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAgentSocketFlagValuePrecedence(t *testing.T) {
	t.Run("prefer ONESSH over SHUSH", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "/tmp/onessh.sock")
		t.Setenv("SHUSH_SOCKET", "/tmp/shush.sock")
		if got := defaultAgentSocketFlagValue(); got != "/tmp/onessh.sock" {
			t.Fatalf("expected ONESSH socket, got %q", got)
		}
	})

	t.Run("fallback to SHUSH", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		t.Setenv("SHUSH_SOCKET", "/tmp/shush.sock")
		if got := defaultAgentSocketFlagValue(); got != "/tmp/shush.sock" {
			t.Fatalf("expected SHUSH socket, got %q", got)
		}
	})

	t.Run("empty when no env provided", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		t.Setenv("SHUSH_SOCKET", "")
		if got := defaultAgentSocketFlagValue(); got != "" {
			t.Fatalf("expected empty value, got %q", got)
		}
	})
}

func TestResolveAgentSocketPathPrecedence(t *testing.T) {
	t.Run("explicit path has highest priority", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "/tmp/onessh.sock")
		t.Setenv("SHUSH_SOCKET", "/tmp/shush.sock")
		got, err := resolveAgentSocketPath("/tmp/custom.sock")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != "/tmp/custom.sock" {
			t.Fatalf("expected custom socket, got %q", got)
		}
	})

	t.Run("fallback to SHUSH env", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		t.Setenv("SHUSH_SOCKET", "/tmp/shush.sock")
		got, err := resolveAgentSocketPath("")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != "/tmp/shush.sock" {
			t.Fatalf("expected SHUSH socket, got %q", got)
		}
	})

	t.Run("default path when no env", func(t *testing.T) {
		t.Setenv("ONESSH_AGENT_SOCKET", "")
		t.Setenv("SHUSH_SOCKET", "")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir: %v", err)
		}
		want := filepath.Join(homeDir, ".config", "onessh", "agent.sock")
		got, err := resolveAgentSocketPath("")
		if err != nil {
			t.Fatalf("resolveAgentSocketPath: %v", err)
		}
		if got != want {
			t.Fatalf("unexpected default socket path: want=%q got=%q", want, got)
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
