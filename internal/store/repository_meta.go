package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func validateHostUserRefs(cfg PlainConfig) error {
	for hostAlias, hostCfg := range cfg.Hosts {
		if strings.TrimSpace(hostCfg.UserRef) == "" {
			return fmt.Errorf("host %q has empty user_ref", hostAlias)
		}
		if _, ok := cfg.Users[hostCfg.UserRef]; !ok {
			return fmt.Errorf("host %q references missing user profile %q", hostAlias, hostCfg.UserRef)
		}
	}
	return nil
}

func (r Repository) loadMetaAndKey(passphrase []byte, createIfMissing bool) (metadataDoc, []byte, error) {
	if len(passphrase) == 0 {
		return metadataDoc{}, nil, errors.New("empty passphrase")
	}

	raw, err := os.ReadFile(r.metaPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if createIfMissing {
				return r.createMeta(passphrase)
			}
			return metadataDoc{}, nil, ErrConfigNotFound
		}
		return metadataDoc{}, nil, fmt.Errorf("read metadata: %w", err)
	}

	var meta metadataDoc
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return metadataDoc{}, nil, fmt.Errorf("decode metadata: %w", err)
	}
	if err := validateReadableStoreVersion(meta.Version); err != nil {
		return metadataDoc{}, nil, err
	}
	if meta.KDF.Name != "argon2id" {
		return metadataDoc{}, nil, fmt.Errorf("unsupported kdf: %s", meta.KDF.Name)
	}

	salt, err := decodeB64(meta.KDF.Salt)
	if err != nil {
		return metadataDoc{}, nil, fmt.Errorf("decode kdf salt: %w", err)
	}
	if err := validateKDFParams(meta.KDF, salt); err != nil {
		return metadataDoc{}, nil, fmt.Errorf("invalid kdf params: %w", err)
	}
	key := deriveKey(passphrase, salt, meta.KDF.Time, meta.KDF.Memory, meta.KDF.Threads, meta.KDF.KeyLen)

	check, err := decryptStringField(meta.Check, key)
	if err != nil || check != passwordCheckText {
		zeroBytes(key)
		return metadataDoc{}, nil, ErrInvalidPassword
	}

	if err := r.ensureStoreMetaFileCurrent(&meta, key); err != nil {
		zeroBytes(key)
		return metadataDoc{}, nil, err
	}

	return meta, key, nil
}

// ensureStoreMetaFileCurrent rewrites meta.yaml when the on-disk store version is older than
// storeWriteVersion but still supported (storeMinVersion..storeWriteVersion).
func (r Repository) ensureStoreMetaFileCurrent(meta *metadataDoc, key []byte) error {
	if meta.Version >= storeWriteVersion {
		return nil
	}
	prev := meta.Version
	meta.Version = storeWriteVersion
	if err := writeYAMLAtomic(r.metaPath(), *meta); err != nil {
		meta.Version = prev
		return fmt.Errorf("upgrade store metadata: %w", err)
	}
	return nil
}

func (r Repository) createMeta(passphrase []byte) (metadataDoc, []byte, error) {
	if err := r.ensureStoreDirs(); err != nil {
		return metadataDoc{}, nil, err
	}

	salt, err := randomBytes(16)
	if err != nil {
		return metadataDoc{}, nil, fmt.Errorf("generate kdf salt: %w", err)
	}
	params := defaultKDFParams(salt)
	key := deriveKey(passphrase, salt, params.Time, params.Memory, params.Threads, params.KeyLen)

	check, err := encryptStringField(passwordCheckText, key)
	if err != nil {
		zeroBytes(key)
		return metadataDoc{}, nil, fmt.Errorf("encrypt password verifier: %w", err)
	}

	meta := metadataDoc{
		Version: storeWriteVersion,
		KDF:     params,
		Check:   check,
	}
	if err := writeYAMLAtomic(r.metaPath(), meta); err != nil {
		zeroBytes(key)
		return metadataDoc{}, nil, err
	}

	return meta, key, nil
}

func (r Repository) ensureStoreDirs() error {
	if err := os.MkdirAll(r.Path, 0o700); err != nil {
		return fmt.Errorf("create store root: %w", err)
	}
	if err := os.MkdirAll(r.usersDir(), 0o700); err != nil {
		return fmt.Errorf("create users directory: %w", err)
	}
	if err := os.MkdirAll(r.hostsDir(), 0o700); err != nil {
		return fmt.Errorf("create hosts directory: %w", err)
	}
	return nil
}

func validateKDFParams(params kdfParams, salt []byte) error {
	if params.Time < kdfMinTime || params.Time > kdfMaxTime {
		return fmt.Errorf("time must be between %d and %d", kdfMinTime, kdfMaxTime)
	}
	if params.Memory < kdfMinMemoryKiB || params.Memory > kdfMaxMemoryKiB {
		return fmt.Errorf("memory must be between %d and %d KiB", kdfMinMemoryKiB, kdfMaxMemoryKiB)
	}
	if params.Threads < kdfMinThreads || params.Threads > kdfMaxThreads {
		return fmt.Errorf("threads must be between %d and %d", kdfMinThreads, kdfMaxThreads)
	}
	if params.KeyLen != kdfRequiredKeyLen {
		return fmt.Errorf("key_len must be %d", kdfRequiredKeyLen)
	}
	if len(salt) < kdfMinSaltLen || len(salt) > kdfMaxSaltLen {
		return fmt.Errorf("salt length must be between %d and %d bytes", kdfMinSaltLen, kdfMaxSaltLen)
	}
	return nil
}
