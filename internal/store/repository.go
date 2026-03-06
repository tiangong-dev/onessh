package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound  = errors.New("config store not found")
	ErrInvalidPassword = errors.New("invalid master password or corrupted config")
)

const (
	storeVersion      = 3
	docVersion        = 1
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

func prepareSwapTempDir(targetPath, kind string) (string, func(), error) {
	basePath := filepath.Clean(strings.TrimSpace(targetPath))
	if basePath == "" || basePath == "." || basePath == string(filepath.Separator) {
		return "", nil, fmt.Errorf("invalid target path %q", targetPath)
	}

	parentDir := filepath.Dir(basePath)
	baseName := filepath.Base(basePath)
	tempDir, err := os.MkdirTemp(parentDir, "."+baseName+"."+kind+".*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup, nil
}

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
	if meta.Version != storeVersion {
		return metadataDoc{}, nil, fmt.Errorf("unsupported store version: %d", meta.Version)
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

	return meta, key, nil
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
		Version: storeVersion,
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

func (r Repository) loadUsers(cfg *PlainConfig, key []byte) error {
	files, err := os.ReadDir(r.usersDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read users directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		alias := strings.TrimSuffix(f.Name(), ".yaml")
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid user alias %q: %w", alias, err)
		}

		raw, err := os.ReadFile(filepath.Join(r.usersDir(), f.Name()))
		if err != nil {
			return fmt.Errorf("read user %s: %w", alias, err)
		}

		var doc userDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("decode user %s: %w", alias, err)
		}
		if doc.Version != docVersion {
			return fmt.Errorf("unsupported user doc version for %s: %d", alias, doc.Version)
		}

		name, err := decryptStringField(doc.Name, key)
		if err != nil {
			return fmt.Errorf("decrypt user name for %s: %w", alias, err)
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("user %s has empty name", alias)
		}

		authType := normalizeAuthTypeStore(doc.Auth.Type)
		if authType == "" {
			return fmt.Errorf("user %s has invalid auth type", alias)
		}

		userCfg := UserConfig{Name: strings.TrimSpace(name), Auth: AuthConfig{Type: authType}}
		switch authType {
		case "key":
			keyPath, err := decryptStringField(doc.Auth.KeyPath, key)
			if err != nil {
				return fmt.Errorf("decrypt key_path for user %s: %w", alias, err)
			}
			if strings.TrimSpace(keyPath) == "" {
				return fmt.Errorf("user %s has empty key_path", alias)
			}
			userCfg.Auth.KeyPath = strings.TrimSpace(keyPath)
		case "password":
			password, err := decryptStringField(doc.Auth.Password, key)
			if err != nil {
				return fmt.Errorf("decrypt password for user %s: %w", alias, err)
			}
			if strings.TrimSpace(password) == "" {
				return fmt.Errorf("user %s has empty password", alias)
			}
			userCfg.Auth.Password = password
		}

		cfg.Users[alias] = userCfg
	}
	return nil
}

func (r Repository) loadHosts(cfg *PlainConfig, key []byte) error {
	files, err := os.ReadDir(r.hostsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read hosts directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		alias := strings.TrimSuffix(f.Name(), ".yaml")
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid host alias %q: %w", alias, err)
		}

		raw, err := os.ReadFile(filepath.Join(r.hostsDir(), f.Name()))
		if err != nil {
			return fmt.Errorf("read host %s: %w", alias, err)
		}

		var doc hostDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("decode host %s: %w", alias, err)
		}
		if doc.Version != docVersion {
			return fmt.Errorf("unsupported host doc version for %s: %d", alias, doc.Version)
		}

		hostValue, err := decryptStringField(doc.Host, key)
		if err != nil {
			return fmt.Errorf("decrypt host value for %s: %w", alias, err)
		}
		if strings.TrimSpace(hostValue) == "" {
			return fmt.Errorf("host %s has empty host", alias)
		}
		if strings.TrimSpace(doc.UserRef) == "" {
			return fmt.Errorf("host %s has empty user_ref", alias)
		}

		hostCfg := HostConfig{
			Host:        strings.TrimSpace(hostValue),
			Description: strings.TrimSpace(doc.Description),
			UserRef:     strings.TrimSpace(doc.UserRef),
			Port:        doc.Port,
			ProxyJump:   strings.TrimSpace(doc.ProxyJump),
			Tags:        doc.Tags,
			Env:         map[string]string{},
			PreConnect:  make([]string, 0, len(doc.PreConnect)),
			PostConnect: make([]string, 0, len(doc.PostConnect)),
		}
		if hostCfg.Port <= 0 {
			hostCfg.Port = 22
		}
		for k, encVal := range doc.Env {
			plainVal, err := decryptStringField(encVal, key)
			if err != nil {
				return fmt.Errorf("decrypt env for host %s key %s: %w", alias, k, err)
			}
			hostCfg.Env[k] = plainVal
		}
		if len(hostCfg.Env) == 0 {
			hostCfg.Env = nil
		}
		for i, encCmd := range doc.PreConnect {
			plainCmd, err := decryptStringField(encCmd, key)
			if err != nil {
				return fmt.Errorf("decrypt pre_connect for host %s index %d: %w", alias, i, err)
			}
			plainCmd = strings.TrimSpace(plainCmd)
			if plainCmd == "" {
				return fmt.Errorf("host %s has empty pre_connect command at index %d", alias, i)
			}
			hostCfg.PreConnect = append(hostCfg.PreConnect, plainCmd)
		}
		if len(hostCfg.PreConnect) == 0 {
			hostCfg.PreConnect = nil
		}
		for i, encCmd := range doc.PostConnect {
			plainCmd, err := decryptStringField(encCmd, key)
			if err != nil {
				return fmt.Errorf("decrypt post_connect for host %s index %d: %w", alias, i, err)
			}
			plainCmd = strings.TrimSpace(plainCmd)
			if plainCmd == "" {
				return fmt.Errorf("host %s has empty post_connect command at index %d", alias, i)
			}
			hostCfg.PostConnect = append(hostCfg.PostConnect, plainCmd)
		}
		if len(hostCfg.PostConnect) == 0 {
			hostCfg.PostConnect = nil
		}

		cfg.Hosts[alias] = hostCfg
	}
	return nil
}

func (r Repository) syncUsers(cfg PlainConfig, key []byte) error {
	if err := os.MkdirAll(r.usersDir(), 0o700); err != nil {
		return fmt.Errorf("ensure users directory: %w", err)
	}

	aliases := sortedKeys(cfg.Users)
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid user alias %q: %w", alias, err)
		}

		userCfg := cfg.Users[alias]
		userName := strings.TrimSpace(userCfg.Name)
		if userName == "" {
			return fmt.Errorf("user profile %q has empty name", alias)
		}

		authType := normalizeAuthTypeStore(userCfg.Auth.Type)
		if authType == "" {
			return fmt.Errorf("user profile %q has invalid auth type", alias)
		}

		doc := userDoc{
			Version: docVersion,
			Auth: userAuthDoc{
				Type: authType,
			},
		}

		var err error
		doc.Name, err = encryptStringField(userName, key)
		if err != nil {
			return fmt.Errorf("encrypt user name for %s: %w", alias, err)
		}

		switch authType {
		case "key":
			keyPath := strings.TrimSpace(userCfg.Auth.KeyPath)
			if keyPath == "" {
				return fmt.Errorf("user profile %q key auth requires key_path", alias)
			}
			doc.Auth.KeyPath, err = encryptStringField(keyPath, key)
			if err != nil {
				return fmt.Errorf("encrypt key_path for %s: %w", alias, err)
			}
		case "password":
			if strings.TrimSpace(userCfg.Auth.Password) == "" {
				return fmt.Errorf("user profile %q password auth requires password", alias)
			}
			doc.Auth.Password, err = encryptStringField(userCfg.Auth.Password, key)
			if err != nil {
				return fmt.Errorf("encrypt password for %s: %w", alias, err)
			}
		}

		if err := writeYAMLAtomic(filepath.Join(r.usersDir(), alias+".yaml"), doc); err != nil {
			return err
		}
		seen[alias] = struct{}{}
	}

	return cleanupStaleYAMLFiles(r.usersDir(), seen)
}

func (r Repository) syncHosts(cfg PlainConfig, key []byte) error {
	if err := os.MkdirAll(r.hostsDir(), 0o700); err != nil {
		return fmt.Errorf("ensure hosts directory: %w", err)
	}

	aliases := sortedKeys(cfg.Hosts)
	seen := map[string]struct{}{}
	for _, alias := range aliases {
		if err := validateAlias(alias); err != nil {
			return fmt.Errorf("invalid host alias %q: %w", alias, err)
		}

		hostCfg := cfg.Hosts[alias]
		hostValue := strings.TrimSpace(hostCfg.Host)
		if hostValue == "" {
			return fmt.Errorf("host %q has empty host", alias)
		}
		if strings.TrimSpace(hostCfg.UserRef) == "" {
			return fmt.Errorf("host %q has empty user_ref", alias)
		}
		if _, ok := cfg.Users[hostCfg.UserRef]; !ok {
			return fmt.Errorf("host %q references missing user profile %q", alias, hostCfg.UserRef)
		}

		doc := hostDoc{
			Version:     docVersion,
			Description: strings.TrimSpace(hostCfg.Description),
			UserRef:     strings.TrimSpace(hostCfg.UserRef),
			Port:        hostCfg.Port,
			ProxyJump:   strings.TrimSpace(hostCfg.ProxyJump),
			Tags:        hostCfg.Tags,
			Env:         map[string]string{},
			PreConnect:  make([]string, 0, len(hostCfg.PreConnect)),
			PostConnect: make([]string, 0, len(hostCfg.PostConnect)),
		}
		if doc.Port <= 0 {
			doc.Port = 22
		}

		var err error
		doc.Host, err = encryptStringField(hostValue, key)
		if err != nil {
			return fmt.Errorf("encrypt host value for %s: %w", alias, err)
		}

		for k, v := range hostCfg.Env {
			encVal, err := encryptStringField(v, key)
			if err != nil {
				return fmt.Errorf("encrypt env for host %s key %s: %w", alias, k, err)
			}
			doc.Env[k] = encVal
		}
		if len(doc.Env) == 0 {
			doc.Env = nil
		}
		for i, command := range hostCfg.PreConnect {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				return fmt.Errorf("host %q pre_connect command at index %d is empty", alias, i)
			}
			encCmd, err := encryptStringField(trimmed, key)
			if err != nil {
				return fmt.Errorf("encrypt pre_connect for host %s index %d: %w", alias, i, err)
			}
			doc.PreConnect = append(doc.PreConnect, encCmd)
		}
		if len(doc.PreConnect) == 0 {
			doc.PreConnect = nil
		}
		for i, command := range hostCfg.PostConnect {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				return fmt.Errorf("host %q post_connect command at index %d is empty", alias, i)
			}
			encCmd, err := encryptStringField(trimmed, key)
			if err != nil {
				return fmt.Errorf("encrypt post_connect for host %s index %d: %w", alias, i, err)
			}
			doc.PostConnect = append(doc.PostConnect, encCmd)
		}
		if len(doc.PostConnect) == 0 {
			doc.PostConnect = nil
		}

		if err := writeYAMLAtomic(filepath.Join(r.hostsDir(), alias+".yaml"), doc); err != nil {
			return err
		}
		seen[alias] = struct{}{}
	}

	return cleanupStaleYAMLFiles(r.hostsDir(), seen)
}

func writeYAMLAtomic(path string, data any) error {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode yaml %s: %w", path, err)
	}
	defer zeroBytes(encoded)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".onessh-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempName := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}

	if err := tempFile.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if _, err := tempFile.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file for %s: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("rename temp file for %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod file %s: %w", path, err)
	}
	return nil
}

func cleanupStaleYAMLFiles(dir string, keep map[string]struct{}) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		alias := strings.TrimSuffix(f.Name(), ".yaml")
		if _, ok := keep[alias]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			return fmt.Errorf("remove stale file %s: %w", f.Name(), err)
		}
	}
	return nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return errors.New("alias is empty")
	}
	if !aliasPattern.MatchString(alias) {
		return errors.New("alias must match [A-Za-z0-9._-]+")
	}
	return nil
}

func normalizeAuthTypeStore(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "key":
		return "key"
	case "password":
		return "password"
	default:
		return ""
	}
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

func validateResetPath(path string) error {
	target := filepath.Clean(strings.TrimSpace(path))
	if target == "" || target == "." || target == string(filepath.Separator) {
		return fmt.Errorf("refuse to reset unsafe path: %q", path)
	}

	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat config store path %s: %w", target, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("config store path is not a directory: %s", target)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read config store path %s: %w", target, err)
	}
	if len(entries) == 0 {
		return nil
	}

	allowed := map[string]struct{}{
		metaFileName: {},
		usersDirName: {},
		hostsDirName: {},
		"audit.log":  {},
	}

	hasMeta := false
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("refuse to reset non-onessh directory %s (unexpected entry %q)", target, name)
		}
		if name == metaFileName {
			hasMeta = true
		}
	}
	if !hasMeta {
		return fmt.Errorf("refuse to reset directory %s without %s", target, metaFileName)
	}
	return nil
}

func (r Repository) metaPath() string {
	return filepath.Join(r.Path, metaFileName)
}

func (r Repository) usersDir() string {
	return filepath.Join(r.Path, usersDirName)
}

func (r Repository) hostsDir() string {
	return filepath.Join(r.Path, hostsDirName)
}

func expandPath(input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(input, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if input == "~" {
			return homeDir, nil
		}
		if strings.HasPrefix(input, "~/") {
			return filepath.Join(homeDir, strings.TrimPrefix(input, "~/")), nil
		}
	}
	return filepath.Clean(input), nil
}
