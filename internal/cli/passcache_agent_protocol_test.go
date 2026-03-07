package cli

import (
	"strings"
	"testing"
)

func TestRegisterAskPassTokenRejectsEmptyPassword(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)
	if _, _, err := registerAskPassToken(socketPath, "   ", 0, 0, ""); err == nil {
		t.Fatalf("expected error for empty password")
	}
}

func TestRegisterAskPassTokenUsesDefaultPolicy(t *testing.T) {
	socketPath := startTestPassphraseAgent(t)
	token, cleanup, err := registerAskPassToken(socketPath, "ssh-secret", 0, 0, "")
	if err != nil {
		t.Fatalf("registerAskPassToken: %v", err)
	}
	defer cleanup()

	first, err := resolveAskPassTokenSecret(socketPath, token, "")
	if err != nil {
		t.Fatalf("resolveAskPassTokenSecret first: %v", err)
	}
	if first != "ssh-secret" {
		t.Fatalf("unexpected first secret: %q", first)
	}

	second, err := resolveAskPassTokenSecret(socketPath, token, "")
	if err != nil {
		t.Fatalf("resolveAskPassTokenSecret second: %v", err)
	}
	if second != "ssh-secret" {
		t.Fatalf("unexpected second secret: %q", second)
	}

	_, err = resolveAskPassTokenSecret(socketPath, token, "")
	if err == nil || !strings.Contains(err.Error(), "not found or expired") {
		t.Fatalf("expected token exhaustion error, got %v", err)
	}
}
