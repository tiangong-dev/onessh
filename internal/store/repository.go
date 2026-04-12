package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var (
	ErrConfigNotFound  = errors.New("config store not found")
	ErrInvalidPassword = errors.New("invalid master password or corrupted config")
)

const (
	metaFileName      = "meta.yaml"
	usersDirName      = "users"
	hostsDirName      = "hosts"
	passwordCheckText = "onessh-store-check"

	kdfMinTime        uint32 = 1
	kdfMaxTime        uint32 = 10
	kdfMinMemoryKiB   uint32 = 8 * 1024
	kdfMaxMemoryKiB   uint32 = 1024 * 1024
	kdfMinThreads     uint8  = 1
	kdfMaxThreads     uint8  = 64
	kdfRequiredKeyLen uint32 = 32
	kdfMinSaltLen            = 16
	kdfMaxSaltLen            = 64
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Repository struct {
	Path string
}

type metadataDoc struct {
	Version int       `yaml:"version"`
	KDF     kdfParams `yaml:"kdf"`
	Check   string    `yaml:"check"`
}

type userAuthDoc struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type userDoc struct {
	Version int         `yaml:"version"`
	Name    string      `yaml:"name"`
	Auth    userAuthDoc `yaml:"auth"`
}

type hostDoc struct {
	Version     int               `yaml:"version"`
	Host        string            `yaml:"host"`
	Description string            `yaml:"description,omitempty"`
	UserRef     string            `yaml:"user_ref"`
	Port        int               `yaml:"port"`
	ProxyJump   string            `yaml:"proxy_jump,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	PreConnect  []string          `yaml:"pre_connect,omitempty"`
	PostConnect []string          `yaml:"post_connect,omitempty"`
}

func ResolvePath(customPath string) (string, error) {
	if customPath != "" {
		return expandPath(customPath)
	}
	if fromEnv := os.Getenv("ONESSH_DATA"); fromEnv != "" {
		return expandPath(fromEnv)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "onessh", "data"), nil
}

func (r Repository) Exists() bool {
	_, err := os.Stat(r.metaPath())
	return err == nil
}

func (r Repository) Load(passphrase []byte) (PlainConfig, error) {
	_, key, err := r.loadMetaAndKey(passphrase, false)
	if err != nil {
		return PlainConfig{}, err
	}
	defer zeroBytes(key)

	cfg := NewPlainConfig()
	if err := r.loadUsers(&cfg, key); err != nil {
		return PlainConfig{}, err
	}
	if err := r.loadHosts(&cfg, key); err != nil {
		return PlainConfig{}, err
	}
	if err := validateHostUserRefs(cfg); err != nil {
		return PlainConfig{}, err
	}

	return cfg, nil
}

func (r Repository) Save(cfg PlainConfig, passphrase []byte) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	if cfg.Users == nil {
		cfg.Users = map[string]UserConfig{}
	}
	if err := validateHostUserRefs(cfg); err != nil {
		return err
	}

	if _, key, err := r.loadMetaAndKey(passphrase, true); err != nil {
		return err
	} else {
		defer zeroBytes(key)
		if err := r.ensureStoreDirs(); err != nil {
			return err
		}
		if err := r.syncUsers(cfg, key); err != nil {
			return err
		}
		if err := r.syncHosts(cfg, key); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) SaveWithReset(cfg PlainConfig, passphrase []byte) error {
	if err := validateResetPath(r.Path); err != nil {
		return err
	}

	stagedPath, cleanupStaged, err := prepareSwapTempDir(r.Path, "stage")
	if err != nil {
		return fmt.Errorf("prepare staged store: %w", err)
	}
	defer cleanupStaged()

	stagedRepo := Repository{Path: stagedPath}
	if err := stagedRepo.Save(cfg, passphrase); err != nil {
		return err
	}

	if _, err := os.Stat(r.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Rename(stagedPath, r.Path); err != nil {
				return fmt.Errorf("activate staged store: %w", err)
			}
			return nil
		}
		return fmt.Errorf("stat config store path %s: %w", r.Path, err)
	}

	backupPath, cleanupBackupPath, err := prepareSwapTempDir(r.Path, "backup")
	if err != nil {
		return fmt.Errorf("prepare backup path: %w", err)
	}
	defer cleanupBackupPath()
	if err := os.RemoveAll(backupPath); err != nil {
		return fmt.Errorf("prepare backup path: %w", err)
	}

	if err := os.Rename(r.Path, backupPath); err != nil {
		return fmt.Errorf("backup current store: %w", err)
	}
	if err := os.Rename(stagedPath, r.Path); err != nil {
		if rollbackErr := os.Rename(backupPath, r.Path); rollbackErr != nil {
			return fmt.Errorf("activate staged store: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("activate staged store: %w", err)
	}

	// Best effort cleanup. The new store is already active at this point.
	_ = os.RemoveAll(backupPath)
	return nil
}
