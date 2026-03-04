package cli

import (
	"testing"

	"onessh/internal/store"
)

func TestResolveHostIdentityWithUserProfileAuth(t *testing.T) {
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	host := store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Auth: store.AuthConfig{
			Type: "password",
		},
	}

	userName, auth, err := resolveHostIdentity(cfg, host)
	if err != nil {
		t.Fatalf("resolveHostIdentity: %v", err)
	}
	if userName != "ubuntu" {
		t.Fatalf("unexpected user name: %s", userName)
	}
	if auth.Type != "key" {
		t.Fatalf("expected key auth from user profile, got %s", auth.Type)
	}
}

func TestResolveHostIdentityFallbackToHostAuth(t *testing.T) {
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
	}
	host := store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Auth: store.AuthConfig{
			Type: "password",
		},
	}

	userName, auth, err := resolveHostIdentity(cfg, host)
	if err != nil {
		t.Fatalf("resolveHostIdentity: %v", err)
	}
	if userName != "ubuntu" {
		t.Fatalf("unexpected user name: %s", userName)
	}
	if auth.Type != "password" {
		t.Fatalf("expected host auth fallback, got %s", auth.Type)
	}
}

func TestResolveHostIdentityLegacyHostUser(t *testing.T) {
	cfg := store.NewPlainConfig()
	host := store.HostConfig{
		Host: "1.2.3.4",
		User: "root",
		Auth: store.AuthConfig{
			Type: "key",
		},
	}

	userName, auth, err := resolveHostIdentity(cfg, host)
	if err != nil {
		t.Fatalf("resolveHostIdentity: %v", err)
	}
	if userName != "root" {
		t.Fatalf("unexpected user name: %s", userName)
	}
	if auth.Type != "key" {
		t.Fatalf("unexpected auth type: %s", auth.Type)
	}
}
