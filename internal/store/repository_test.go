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
		Description: "Production web server",
		UserRef:     "ops",
		Port:        22,
		Tags:        []string{"prod", "cn"},
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

func TestRepositorySaveKeepsOldDataOnStagedWriteFailure(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("top-secret-master-password")
	original := validTestConfig()
	if err := repo.Save(original, pass); err != nil {
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
		Host:    "   ",
		UserRef: "db",
		Port:    2222,
	}

	err := repo.Save(replacement, pass)
	if err == nil || !strings.Contains(err.Error(), "empty host") {
		t.Fatalf("expected save failure for invalid host, got %v", err)
	}

	loaded, err := repo.Load(pass)
	if err != nil {
		t.Fatalf("load after failed save: %v", err)
	}
	if !reflect.DeepEqual(original, loaded) {
		t.Fatalf("expected original config to remain unchanged:\nwant=%#v\ngot=%#v", original, loaded)
	}
	assertPathExists(t, filepath.Join(repo.Path, usersDirName, "ops.yaml"))
	assertPathExists(t, filepath.Join(repo.Path, hostsDirName, "web1.yaml"))
	assertPathMissing(t, filepath.Join(repo.Path, usersDirName, "db.yaml"))
	assertPathMissing(t, filepath.Join(repo.Path, hostsDirName, "db.yaml"))
}

func TestRepositorySaveWithResetPreservesUnmanagedDataPathFiles(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("top-secret-master-password")
	if err := repo.Save(validTestConfig(), pass); err != nil {
		t.Fatalf("save original config: %v", err)
	}
	unmanagedPaths := []string{
		filepath.Join(repo.Path, "agents", "session.sock"),
		filepath.Join(repo.Path, "audit.log"),
		filepath.Join(repo.Path, usersDirName, "README.txt"),
		filepath.Join(repo.Path, hostsDirName, "README.txt"),
	}
	for _, path := range unmanagedPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create unmanaged parent: %v", err)
		}
		if err := os.WriteFile(path, []byte("unmanaged"), 0o600); err != nil {
			t.Fatalf("write unmanaged file: %v", err)
		}
	}

	replacement := replacementTestConfig()
	if err := repo.SaveWithReset(replacement, pass); err != nil {
		t.Fatalf("save with reset: %v", err)
	}
	loaded, err := repo.Load(pass)
	if err != nil {
		t.Fatalf("load after save with reset: %v", err)
	}
	if !reflect.DeepEqual(replacement, loaded) {
		t.Fatalf("loaded config mismatch:\nwant=%#v\ngot=%#v", replacement, loaded)
	}
	for _, path := range unmanagedPaths {
		assertPathExists(t, path)
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

func TestRepositorySaveAndSaveWithResetCleanupStaleYAMLFilesConsistently(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		save func(Repository, PlainConfig, []byte) error
	}{
		{name: "Save", save: Repository.Save},
		{name: "SaveWithReset", save: Repository.SaveWithReset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
			pass := []byte("top-secret-master-password")
			if err := repo.Save(validTestConfig(), pass); err != nil {
				t.Fatalf("save original config: %v", err)
			}
			unmanagedUserFile := filepath.Join(repo.Path, usersDirName, "README.txt")
			unmanagedHostFile := filepath.Join(repo.Path, hostsDirName, "README.txt")
			for _, path := range []string{
				filepath.Join(repo.Path, usersDirName, "manual-stale.yaml"),
				filepath.Join(repo.Path, hostsDirName, "manual-stale.yaml"),
				unmanagedUserFile,
				unmanagedHostFile,
			} {
				if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
					t.Fatalf("write test file %s: %v", path, err)
				}
			}

			replacement := replacementTestConfig()
			if err := tc.save(repo, replacement, pass); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			loaded, err := repo.Load(pass)
			if err != nil {
				t.Fatalf("load after %s: %v", tc.name, err)
			}
			if !reflect.DeepEqual(replacement, loaded) {
				t.Fatalf("loaded config mismatch after %s:\nwant=%#v\ngot=%#v", tc.name, replacement, loaded)
			}

			for _, path := range []string{
				filepath.Join(repo.Path, usersDirName, "ops.yaml"),
				filepath.Join(repo.Path, hostsDirName, "web1.yaml"),
				filepath.Join(repo.Path, usersDirName, "manual-stale.yaml"),
				filepath.Join(repo.Path, hostsDirName, "manual-stale.yaml"),
			} {
				assertPathMissing(t, path)
			}
			for _, path := range []string{
				filepath.Join(repo.Path, usersDirName, "db.yaml"),
				filepath.Join(repo.Path, hostsDirName, "db.yaml"),
				unmanagedUserFile,
				unmanagedHostFile,
			} {
				assertPathExists(t, path)
			}
		})
	}
}

func replacementTestConfig() PlainConfig {
	cfg := NewPlainConfig()
	cfg.Users["db"] = UserConfig{
		Name: "root",
		Auth: AuthConfig{
			Type:     "password",
			Password: "new-secret",
		},
	}
	cfg.Hosts["db"] = HostConfig{
		Host:    "10.0.0.12",
		UserRef: "db",
		Port:    2222,
	}
	return cfg
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %s to exist: %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected path %s to be missing, got %v", path, err)
	}
}
