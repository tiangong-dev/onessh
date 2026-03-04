package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepositorySaveAndLoad(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config.enc")}
	pass := []byte("top-secret-master-password")

	source := NewPlainConfig()
	source.Users["ops"] = UserConfig{
		Name: "ubuntu",
		Auth: AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	source.Hosts["web1"] = HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}

	if err := repo.Save(source, pass); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := repo.Load(pass)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !reflect.DeepEqual(source, loaded) {
		t.Fatalf("loaded config mismatch:\nsource=%#v\nloaded=%#v", source, loaded)
	}

	info, err := os.Stat(repo.Path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected file permissions 0600, got %o", perm)
	}
}

func TestRepositoryLoadWithWrongPassword(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config.enc")}
	pass := []byte("correct-pass")

	cfg := NewPlainConfig()
	cfg.Users["dbuser"] = UserConfig{
		Name: "root",
		Auth: AuthConfig{
			Type:     "password",
			Password: "secret-pass",
		},
	}
	cfg.Hosts["db"] = HostConfig{
		Host:    "10.0.0.12",
		UserRef: "dbuser",
		Port:    2222,
	}

	if err := repo.Save(cfg, pass); err != nil {
		t.Fatalf("save config: %v", err)
	}

	_, err := repo.Load([]byte("wrong-pass"))
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestRepositoryLoadMissingFile(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "missing.enc")}
	_, err := repo.Load([]byte("any"))
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}
