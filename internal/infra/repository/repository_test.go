package repository

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"onessh/internal/store"
)

func TestRepositorySaveProxiesToStoreRepository(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	passphrase := []byte("top-secret-master-password")
	cfg := testConfig()

	if err := repo.Save(cfg, passphrase); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := (store.Repository{Path: repo.Path}).Load(passphrase)
	if err != nil {
		t.Fatalf("store Load after facade Save: %v", err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("loaded config mismatch:\nwant=%#v\ngot=%#v", cfg, loaded)
	}
}

func TestRepositoryLoadProxiesToStoreRepository(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config")
	passphrase := []byte("top-secret-master-password")
	cfg := testConfig()

	if err := (store.Repository{Path: path}).Save(cfg, passphrase); err != nil {
		t.Fatalf("store Save: %v", err)
	}

	loaded, err := (Repository{Path: path}).Load(passphrase)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("loaded config mismatch:\nwant=%#v\ngot=%#v", cfg, loaded)
	}
}

func TestRepositoryFacadeKeepsStoreErrors(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "missing")}
	_, err := repo.Load([]byte("any"))
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}

func testConfig() PlainConfig {
	cfg := NewPlainConfig()
	cfg.Users["ops"] = UserConfig{
		Name: "ubuntu",
		Auth: AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	cfg.Hosts["web1"] = HostConfig{
		Host:        "1.2.3.4",
		Description: "Production web server",
		UserRef:     "ops",
		Port:        22,
		Tags:        []string{"prod", "ssh"},
		Env:         map[string]string{"AWS_PROFILE": "prod"},
		PreConnect:  []string{"cd /srv/app"},
		PostConnect: []string{"echo disconnected"},
	}
	return cfg
}
