package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRepositorySaveReturnsLockConflictWhenWriterHoldsLock(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("top-secret-master-password")
	cfg := validTestConfig()

	lock, err := acquireRepositoryWriteLock(repo.Path)
	if err != nil {
		t.Fatalf("acquire repository write lock: %v", err)
	}

	err = repo.Save(cfg, pass)
	if !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("expected ErrStoreLocked while lock is held, got %v", err)
	}

	if err := lock.Close(); err != nil {
		t.Fatalf("release repository write lock: %v", err)
	}

	if err := repo.Save(cfg, pass); err != nil {
		t.Fatalf("save after releasing repository write lock: %v", err)
	}
}

func TestRepositorySaveWithResetReturnsLockConflictWhenWriterHoldsLock(t *testing.T) {
	t.Parallel()

	repo := Repository{Path: filepath.Join(t.TempDir(), "config")}
	pass := []byte("top-secret-master-password")
	cfg := validTestConfig()

	lock, err := acquireRepositoryWriteLock(repo.Path)
	if err != nil {
		t.Fatalf("acquire repository write lock: %v", err)
	}
	defer func() {
		if lock != nil {
			_ = lock.Close()
		}
	}()

	err = repo.SaveWithReset(cfg, pass)
	if !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("expected ErrStoreLocked while lock is held, got %v", err)
	}

	if err := lock.Close(); err != nil {
		t.Fatalf("release repository write lock: %v", err)
	}
	lock = nil

	if err := repo.SaveWithReset(cfg, pass); err != nil {
		t.Fatalf("save with reset after releasing repository write lock: %v", err)
	}
}

func validTestConfig() PlainConfig {
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
	return cfg
}
