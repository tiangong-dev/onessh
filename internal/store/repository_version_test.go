package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryLoadUpgradesLegacyStoreMetadataVersion(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("correct-pass")

	cfg := NewPlainConfig()
	cfg.Users["ops"] = UserConfig{
		Name: "ubuntu",
		Auth: AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	cfg.Hosts["web1"] = HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}

	if err := repo.Save(cfg, pass); err != nil {
		t.Fatalf("save: %v", err)
	}

	metaPath := filepath.Join(repo.Path, metaFileName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	patched := strings.Replace(string(raw), "version: 3", "version: 2", 1)
	if patched == string(raw) {
		t.Fatalf("expected to patch store version in meta file")
	}
	if err := os.WriteFile(metaPath, []byte(patched), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	loaded, err := repo.Load(pass)
	if err != nil {
		t.Fatalf("load after legacy version: %v", err)
	}
	if !reflect.DeepEqual(cfg, loaded) {
		t.Fatalf("config changed after metadata upgrade")
	}

	rawAfter, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta after load: %v", err)
	}
	if !strings.Contains(string(rawAfter), "version: 3") {
		t.Fatalf("expected meta upgraded to version 3 on disk, got:\n%s", string(rawAfter))
	}
}

func TestRepositoryLoadRejectsStoreVersionTooOld(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("correct-pass")

	cfg := NewPlainConfig()
	cfg.Users["ops"] = UserConfig{
		Name: "ubuntu",
		Auth: AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	cfg.Hosts["web1"] = HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}

	if err := repo.Save(cfg, pass); err != nil {
		t.Fatalf("save: %v", err)
	}

	metaPath := filepath.Join(repo.Path, metaFileName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	patched := strings.Replace(string(raw), "version: 3", "version: 1", 1)
	if err := os.WriteFile(metaPath, []byte(patched), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	_, err = repo.Load(pass)
	if err == nil || !strings.Contains(err.Error(), "unsupported store version") {
		t.Fatalf("expected unsupported store version error, got %v", err)
	}
}
