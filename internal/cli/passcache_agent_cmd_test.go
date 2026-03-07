package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestAgentClearAllCmdClearsSecretsAndTokens(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)

	client, err := newPassphraseAgentClient("/tmp/config-clear-all.enc", time.Minute, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient: %v", err)
	}
	if err := client.Set([]byte("master-secret")); err != nil {
		t.Fatalf("client.Set: %v", err)
	}
	token, cleanupToken, err := registerAskPassToken(socketPath, "askpass-secret", time.Minute, 2)
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanupToken()

	cmd := newAgentClearAllCmd(&rootOptions{agentSocket: socketPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent clear-all command: %v", err)
	}
	if !strings.Contains(out.String(), "agent cache cleared") {
		t.Fatalf("expected success output, got %q", out.String())
	}

	if got, ok, err := client.Get(); err != nil || ok || len(got) != 0 {
		t.Fatalf("expected secret to be cleared, got ok=%v len=%d err=%v", ok, len(got), err)
	}
	if _, err := resolveAskPassTokenSecret(socketPath, token); err == nil {
		t.Fatalf("expected token to be cleared")
	}
}

func TestLogoutAllClearsAgentWithoutRepositoryAccess(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)

	client, err := newPassphraseAgentClient(passphraseCacheKey("/tmp/config-logout-all.enc"), time.Minute, false, socketPath)
	if err != nil {
		t.Fatalf("newPassphraseAgentClient: %v", err)
	}
	if err := client.Set([]byte("master-secret")); err != nil {
		t.Fatalf("client.Set: %v", err)
	}

	token, cleanupToken, err := registerAskPassToken(socketPath, "askpass-secret", time.Minute, 2)
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanupToken()

	cmd := newLogoutCmd(&rootOptions{agentSocket: socketPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout --all command: %v", err)
	}
	if !strings.Contains(out.String(), "all cached master passwords cleared") {
		t.Fatalf("expected success output, got %q", out.String())
	}

	if got, ok, err := client.Get(); err != nil || ok || len(got) != 0 {
		t.Fatalf("expected secret to be cleared, got ok=%v len=%d err=%v", ok, len(got), err)
	}
	if _, err := resolveAskPassTokenSecret(socketPath, token); err != nil {
		t.Fatalf("expected askpass token to remain available, got err=%v", err)
	}
}
