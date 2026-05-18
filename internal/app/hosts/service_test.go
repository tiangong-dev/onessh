package hosts

import (
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestServiceAddHostNormalizesTagsAndEnv(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}

	out, err := service.Add(AddInput{
		Config: cfg,
		Alias:  " prod ",
		Host: domain.HostConfig{
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
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["prod"] = domain.HostConfig{
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

func TestServiceUpdateHostAppliesHooks(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["prod"] = domain.HostConfig{
		Host:        "prod.example.com",
		UserRef:     "alice",
		PreConnect:  []string{"legacy pre"},
		PostConnect: []string{"legacy post"},
	}

	out, err := service.Update(UpdateInput{
		Config:             cfg,
		Alias:              "prod",
		PreConnect:         []string{" cd /srv/app "},
		PreConnectChanged:  true,
		ClearPostConnect:   true,
		PostConnectChanged: true,
		PostConnect:        []string{" echo done "},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := out.Config.Hosts["prod"]
	if !reflect.DeepEqual(got.PreConnect, []string{"cd /srv/app"}) {
		t.Fatalf("PreConnect = %#v", got.PreConnect)
	}
	if !reflect.DeepEqual(got.PostConnect, []string{"echo done"}) {
		t.Fatalf("PostConnect = %#v", got.PostConnect)
	}
}

func TestServiceUpdateHostRejectsInvalidEnv(t *testing.T) {
	t.Parallel()

	service := Service{}
	cfg := domain.NewPlainConfig()
	cfg.Users["alice"] = domain.UserConfig{Name: "alice", Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}}
	cfg.Hosts["prod"] = domain.HostConfig{Host: "prod.example.com", UserRef: "alice"}

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
	cfg := domain.NewPlainConfig()
	cfg.Hosts["prod"] = domain.HostConfig{Host: "prod.example.com"}

	out, err := service.Remove(RemoveInput{Config: cfg, Alias: "prod"})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, exists := out.Config.Hosts["prod"]; exists {
		t.Fatalf("host was not removed: %#v", out.Config.Hosts)
	}
}
