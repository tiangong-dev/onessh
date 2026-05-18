package users

import (
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestServiceAddUserNormalizesAliasAndAuth(t *testing.T) {
	t.Parallel()

	service := Service{}
	out, err := service.Add(AddInput{
		Config: domain.NewPlainConfig(),
		Alias:  " Alice_Admin ",
		Name:   " alice ",
		Auth: AuthInput{
			Type:    " key ",
			KeyPath: " ~/.ssh/id_ed25519 ",
		},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, exists := out.Config.Users["alice_admin"]
	if !exists {
		t.Fatalf("normalized user alias not found: %#v", out.Config.Users)
	}
	want := domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("user = %#v, want %#v", got, want)
	}
}

func TestServiceUpdateUserAuth(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{
		Name: "alice",
		Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
	}

	out, err := service.Update(UpdateInput{
		Config: cfg,
		Alias:  "alice",
		Auth: AuthUpdate{
			Type:        "password",
			Password:    "secret",
			TypeChanged: true,
			PasswordSet: true,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := out.Config.Users["alice"].Auth
	want := domain.AuthConfig{Type: "password", Password: "secret"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auth = %#v, want %#v", got, want)
	}
}

func TestServiceUpdateUserAuthRejectsMissingPassword(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{
		Name: "alice",
		Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
	}

	_, err := service.Update(UpdateInput{
		Config: cfg,
		Alias:  "alice",
		Auth: AuthUpdate{
			Type:        "password",
			TypeChanged: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "password auth requires password") {
		t.Fatalf("expected missing password error, got %v", err)
	}
}

func TestServiceRemoveUserReferencedByHostReturnsError(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key"}}
	cfg.Hosts["prod"] = domain.HostConfig{Host: "prod.example.com", UserRef: "alice"}

	_, err := service.Remove(RemoveInput{Config: cfg, Alias: "alice"})
	if err == nil || !strings.Contains(err.Error(), `user profile "alice" is used by host(s): prod`) {
		t.Fatalf("expected referenced user error, got %v", err)
	}
	if _, exists := cfg.Users["alice"]; !exists {
		t.Fatalf("Remove mutated input config on error")
	}
}

func TestServiceRemoveUserDeletesUnreferencedProfile(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key"}}

	out, err := service.Remove(RemoveInput{Config: cfg, Alias: "alice"})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, exists := out.Config.Users["alice"]; exists {
		t.Fatalf("user was not removed: %#v", out.Config.Users)
	}
}
