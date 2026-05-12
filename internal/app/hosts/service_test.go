package hosts

import (
	"reflect"
	"strings"
	"testing"

	"onessh/internal/store"
)

func TestServiceAddHostNormalizesTagsAndEnv(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := store.NewPlainConfig()
	cfg.Users["alice"] = store.UserConfig{Name: "alice", Auth: store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}

	out, err := service.Add(AddInput{
		Config: cfg,
		Alias:  " prod ",
		Host: store.HostConfig{
			Host:    " prod.example.com ",
			UserRef: "alice",
			Port:    2222,
			Tags:    []string{" Prod ", "db", "prod"},
			Env:     map[string]string{"PATH": "/usr/local/bin", "APP_ENV": "production"},
		},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got := out.Config.Hosts["prod"]
	if got.Host != "prod.example.com" {
		t.Fatalf("Host = %q, want trimmed host", got.Host)
	}
	if !reflect.DeepEqual(got.Tags, []string{"db", "prod"}) {
		t.Fatalf("Tags = %#v, want normalized sorted tags", got.Tags)
	}
	if !reflect.DeepEqual(got.Env, map[string]string{"PATH": "/usr/local/bin", "APP_ENV": "production"}) {
		t.Fatalf("Env = %#v", got.Env)
	}
}

func TestServiceUpdateHostAppliesTagsAndEnv(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := store.NewPlainConfig()
	cfg.Users["alice"] = store.UserConfig{Name: "alice", Auth: store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["prod"] = store.HostConfig{
		Host:    "prod.example.com",
		UserRef: "alice",
		Tags:    []string{"legacy", "prod"},
		Env:     map[string]string{"KEEP": "1", "DROP": "1"},
	}

	out, err := service.Update(UpdateInput{
		Config:          cfg,
		Alias:           "prod",
		Env:             []string{"NEW=2", "KEEP=updated"},
		EnvChanged:      true,
		UnsetEnv:        []string{"DROP"},
		UnsetEnvChanged: true,
		Tags:            []string{" Blue ", "prod"},
		TagsChanged:     true,
		Untag:           []string{"legacy"},
		UntagChanged:    true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := out.Config.Hosts["prod"]
	if !reflect.DeepEqual(got.Env, map[string]string{"KEEP": "updated", "NEW": "2"}) {
		t.Fatalf("Env = %#v", got.Env)
	}
	if !reflect.DeepEqual(got.Tags, []string{"blue", "prod"}) {
		t.Fatalf("Tags = %#v", got.Tags)
	}
}

func TestServiceUpdateHostRejectsInvalidEnv(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := store.NewPlainConfig()
	cfg.Users["alice"] = store.UserConfig{Name: "alice", Auth: store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["prod"] = store.HostConfig{Host: "prod.example.com", UserRef: "alice"}

	_, err := service.Update(UpdateInput{
		Config:     cfg,
		Alias:      "prod",
		Env:        []string{"bad-key=value"},
		EnvChanged: true,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid env key") {
		t.Fatalf("expected invalid env error, got %v", err)
	}
}

func TestServiceRemoveHostDeletesEntry(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := store.NewPlainConfig()
	cfg.Hosts["prod"] = store.HostConfig{Host: "prod.example.com"}

	out, err := service.Remove(RemoveInput{Config: cfg, Alias: "prod"})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, exists := out.Config.Hosts["prod"]; exists {
		t.Fatalf("host was not removed: %#v", out.Config.Hosts)
	}
}
