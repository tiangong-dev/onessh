package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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
