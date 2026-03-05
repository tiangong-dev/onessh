package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"onessh/internal/store"
)

type stubPassphraseStore struct {
	enabled  bool
	setCalls int
	setValue []byte
}

func (s *stubPassphraseStore) IsEnabled() bool {
	return s.enabled
}

func (s *stubPassphraseStore) Get() ([]byte, bool, error) {
	return nil, false, nil
}

func (s *stubPassphraseStore) Set(passphrase []byte) error {
	s.setCalls++
	s.setValue = append([]byte(nil), passphrase...)
	return nil
}

func (s *stubPassphraseStore) Clear() error {
	return nil
}

func TestChangeMasterPasswordSuccess(t *testing.T) {
	t.Parallel()

	repo := store.Repository{Path: filepath.Join(t.TempDir(), "config")}
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	cfg.Hosts["web1"] = store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}
	if err := repo.Save(cfg, []byte("old-pass")); err != nil {
		t.Fatalf("save config: %v", err)
	}

	cache := &stubPassphraseStore{enabled: true}
	if err := changeMasterPassword(repo, cache, []byte("old-pass"), []byte("new-pass")); err != nil {
		t.Fatalf("changeMasterPassword: %v", err)
	}

	if _, err := repo.Load([]byte("new-pass")); err != nil {
		t.Fatalf("load with new password: %v", err)
	}
	if _, err := repo.Load([]byte("old-pass")); !errors.Is(err, store.ErrInvalidPassword) {
		t.Fatalf("expected old password to fail with ErrInvalidPassword, got %v", err)
	}
	if cache.setCalls != 1 {
		t.Fatalf("expected cache set once, got %d", cache.setCalls)
	}
	if string(cache.setValue) != "new-pass" {
		t.Fatalf("expected cache to store new password, got %q", string(cache.setValue))
	}
}

func TestChangeMasterPasswordWrongCurrentPassword(t *testing.T) {
	t.Parallel()

	repo := store.Repository{Path: filepath.Join(t.TempDir(), "config")}
	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{
			Type:    "key",
			KeyPath: "~/.ssh/id_ed25519",
		},
	}
	cfg.Hosts["web1"] = store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
	}
	if err := repo.Save(cfg, []byte("old-pass")); err != nil {
		t.Fatalf("save config: %v", err)
	}

	err := changeMasterPassword(repo, nil, []byte("wrong-pass"), []byte("new-pass"))
	if !errors.Is(err, store.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestChangeMasterPasswordRejectsSamePassword(t *testing.T) {
	t.Parallel()

	repo := store.Repository{Path: filepath.Join(t.TempDir(), "config")}
	err := changeMasterPassword(repo, nil, []byte("same-pass"), []byte("same-pass"))
	if err == nil || !strings.Contains(err.Error(), "different") {
		t.Fatalf("expected same-password rejection, got %v", err)
	}
}
