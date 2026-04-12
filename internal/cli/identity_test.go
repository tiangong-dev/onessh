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

func TestSummarizeHostIdentityForList(t *testing.T) {
	t.Parallel()

	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{Type: "password", Password: "secret"},
	}

	userName, authType, status := summarizeHostIdentityForList(cfg, store.HostConfig{Host: "1.2.3.4", UserRef: "ops"})
	if userName != "ubuntu" || authType != "password" || status != "ok" {
		t.Fatalf("unexpected summarize result: %q %q %q", userName, authType, status)
	}
}

func TestHostAliasesUsingUser(t *testing.T) {
	t.Parallel()

	cfg := store.NewPlainConfig()
	cfg.Hosts["b"] = store.HostConfig{UserRef: "ops"}
	cfg.Hosts["a"] = store.HostConfig{UserRef: "ops"}
	cfg.Hosts["x"] = store.HostConfig{UserRef: "dev"}

	got := hostAliasesUsingUser(cfg, "ops")
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("unexpected aliases length: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected aliases: %v", got)
		}
	}
}
