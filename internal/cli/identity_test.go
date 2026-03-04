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

func TestResolveHostIdentityRequiresConfiguredAuth(t *testing.T) {
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
	}
	host := store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
	}

	_, _, err := resolveHostIdentity(cfg, host)
	if err == nil {
		t.Fatalf("expected error when user profile auth is missing")
	}
}

func TestResolveHostIdentityRequiresUserRef(t *testing.T) {
	cfg := store.NewPlainConfig()
	host := store.HostConfig{
		Host: "1.2.3.4",
	}

	_, _, err := resolveHostIdentity(cfg, host)
	if err == nil {
		t.Fatalf("expected error when host has no user_ref")
	}
}

func TestResolveHostIdentityRequiresPasswordWhenPasswordAuth(t *testing.T) {
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{
			Type: "password",
		},
	}
	host := store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
	}

	_, _, err := resolveHostIdentity(cfg, host)
	if err == nil {
		t.Fatalf("expected error when password auth has empty password")
	}
}
