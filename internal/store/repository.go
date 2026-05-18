package store

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"onessh/internal/domain"
)

var (
	ErrConfigNotFound  = errors.New("config store not found")
	ErrInvalidPassword = errors.New("invalid master password or corrupted config")
)

const (
	metaFileName           = "meta.yaml"
	usersDirName           = "users"
	hostsDirName           = "hosts"
	storeVerifierPlaintext = "onessh-store-check"

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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return ResolveDataPath(customPath, os.Getenv("ONESSH_DATA"), homeDir)
}

func (r Repository) Exists() bool {
	_, err := os.Stat(r.metaPath())
	return err == nil
}

func (r Repository) Load(passphrase []byte) (domain.PlainConfig, error) {
	_, key, err := r.loadMetaAndKey(passphrase, false)
	if err != nil {
		return domain.PlainConfig{}, err
	}
	defer zeroBytes(key)

	cfg := domain.NewPlainConfig()
	if err := r.loadUsers(&cfg, key); err != nil {
		return domain.PlainConfig{}, err
	}
	if err := r.loadHosts(&cfg, key); err != nil {
		return domain.PlainConfig{}, err
	}
	if err := validateHostUserRefs(cfg); err != nil {
		return domain.PlainConfig{}, err
	}

	return cfg, nil
}

func (r Repository) Save(cfg domain.PlainConfig, passphrase []byte) error {
	lock, err := acquireRepositoryWriteLock(r.Path)
	if err != nil {
		return err
	}
	defer lock.Close()

	return r.saveStaged(cfg, passphrase, true)
}

func (r Repository) saveWithoutLock(cfg domain.PlainConfig, passphrase []byte) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]domain.HostConfig{}
	}
	if cfg.Users == nil {
		cfg.Users = map[string]domain.UserConfig{}
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

func (r Repository) SaveWithReset(cfg domain.PlainConfig, passphrase []byte) error {
	if err := validateResetPath(r.Path); err != nil {
		return err
	}
	lock, err := acquireRepositoryWriteLock(r.Path)
	if err != nil {
		return err
	}
	defer lock.Close()

	return r.saveStaged(cfg, passphrase, false)
}

func (r Repository) saveStaged(cfg domain.PlainConfig, passphrase []byte, preserveExistingMeta bool) error {
	stagedPath, cleanupStaged, err := prepareSwapTempDir(r.Path, "stage")
	if err != nil {
		return fmt.Errorf("prepare staged store: %w", err)
	}
	defer cleanupStaged()

	stagedRepo := Repository{Path: stagedPath}
	if preserveExistingMeta {
		if err := r.copyExistingMetaToStage(stagedRepo); err != nil {
			return err
		}
	}
	if err := stagedRepo.saveWithoutLock(cfg, passphrase); err != nil {
		return err
	}

	if err := r.commitStagedStore(stagedRepo); err != nil {
		return err
	}
	return nil
}

func (r Repository) copyExistingMetaToStage(stagedRepo Repository) error {
	if _, err := os.Stat(r.metaPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat metadata: %w", err)
	}
	if err := copyFileAtomic(r.metaPath(), stagedRepo.metaPath()); err != nil {
		return fmt.Errorf("stage existing metadata: %w", err)
	}
	return nil
}

func (r Repository) commitStagedStore(stagedRepo Repository) error {
	backupPath, cleanupBackupPath, err := prepareSwapTempDir(r.Path, "backup")
	if err != nil {
		return fmt.Errorf("prepare backup path: %w", err)
	}
	defer cleanupBackupPath()

	backupRepo := Repository{Path: backupPath}
	if err := backupManagedStoreFiles(r, backupRepo); err != nil {
		return fmt.Errorf("backup current store: %w", err)
	}

	if err := applyManagedStoreFiles(stagedRepo, r); err != nil {
		if rollbackErr := restoreManagedStoreFiles(backupRepo, r); rollbackErr != nil {
			return fmt.Errorf("commit staged store: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("commit staged store: %w", err)
	}
	return nil
}
