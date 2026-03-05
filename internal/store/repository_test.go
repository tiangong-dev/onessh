package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepositorySaveAndLoad(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
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
		Host:        "1.2.3.4",
		UserRef:     "ops",
		Port:        22,
		Env:         map[string]string{"AWS_PROFILE": "prod"},
		PreConnect:  []string{"cd /srv/app"},
		PostConnect: []string{"echo disconnected"},
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

	metaPath := filepath.Join(repo.Path, "meta.yaml")
	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatalf("stat meta file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected file permissions 0600, got %o", perm)
	}

	hostDocPath := filepath.Join(repo.Path, "hosts", "web1.yaml")
	hostDocRaw, err := os.ReadFile(hostDocPath)
	if err != nil {
		t.Fatalf("read host doc: %v", err)
	}
	if string(hostDocRaw) == "" {
		t.Fatalf("expected non-empty host doc")
	}
	if strings.Contains(string(hostDocRaw), "1.2.3.4") {
		t.Fatalf("host doc should not store plaintext host")
	}
}

func TestRepositoryLoadWithWrongPassword(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
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

	repo := Repository{Path: filepath.Join(t.TempDir(), "missing")}
	_, err := repo.Load([]byte("any"))
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestRepositoryLoadRejectsUnsafeKDFParams(t *testing.T) {
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
		t.Fatalf("save config: %v", err)
	}

	metaPath := filepath.Join(repo.Path, metaFileName)
	rawMeta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta file: %v", err)
	}
	mutated := strings.Replace(string(rawMeta), "memory: 65536", "memory: 999999999", 1)
	if mutated == string(rawMeta) {
		t.Fatalf("expected to mutate memory value in meta file")
	}
	if err := os.WriteFile(metaPath, []byte(mutated), 0o600); err != nil {
		t.Fatalf("write mutated meta file: %v", err)
	}

	_, err = repo.Load(pass)
	if err == nil || !strings.Contains(err.Error(), "invalid kdf params") {
		t.Fatalf("expected invalid kdf params error, got %v", err)
	}
}

func TestRepositorySaveWithResetRefusesNonOnesshDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	repoPath := filepath.Join(base, "unsafe")
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatalf("mkdir unsafe dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "notes.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write unexpected file: %v", err)
	}

	repo := Repository{Path: repoPath}
	err := repo.SaveWithReset(NewPlainConfig(), []byte("pass"))
	if err == nil || !strings.Contains(err.Error(), "refuse to reset non-onessh directory") {
		t.Fatalf("expected refusal for non-onessh dir, got %v", err)
	}
}

func TestRepositorySaveWithResetKeepsOldDataOnWriteFailure(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	original := NewPlainConfig()
	original.Users["ops"] = UserConfig{
		Name: "ubuntu",
		Auth: AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	original.Hosts["web1"] = HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}

	oldPass := []byte("old-pass")
	if err := repo.Save(original, oldPass); err != nil {
		t.Fatalf("save original config: %v", err)
	}

	replacement := NewPlainConfig()
	replacement.Users["db"] = UserConfig{
		Name: "root",
		Auth: AuthConfig{
			Type:     "password",
			Password: "new-secret",
		},
	}
	replacement.Hosts["db"] = HostConfig{
		Host:    "10.0.0.12",
		UserRef: "db",
		Port:    2222,
	}

	err := repo.SaveWithReset(replacement, []byte(""))
	if err == nil {
		t.Fatalf("expected SaveWithReset to fail with empty passphrase")
	}

	loaded, err := repo.Load(oldPass)
	if err != nil {
		t.Fatalf("expected old data to remain readable, got %v", err)
	}
	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("expected original config to remain unchanged:\nwant=%#v\ngot=%#v", original, loaded)
	}
}
