package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrStoreLocked = errors.New("config store is locked by another writer")

type repositoryWriteLock struct {
	file *os.File
	path string
}

func acquireRepositoryWriteLock(storePath string) (*repositoryWriteLock, error) {
	lockPath, err := repositoryWriteLockPath(storePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("create store lock directory: %w", err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open store lock: %w", err)
	}
	if err := lockFileExclusive(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrStoreLocked) {
			return nil, fmt.Errorf("lock store %s: %w", storePath, ErrStoreLocked)
		}
		return nil, fmt.Errorf("lock store %s: %w", storePath, err)
	}

	return &repositoryWriteLock{file: file, path: lockPath}, nil
}

func (l *repositoryWriteLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	file := l.file
	l.file = nil

	unlockErr := unlockFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock store lock %s: %w", l.path, unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close store lock %s: %w", l.path, closeErr)
	}
	return nil
}

func repositoryWriteLockPath(storePath string) (string, error) {
	target := filepath.Clean(strings.TrimSpace(storePath))
	if target == "" || target == "." || target == string(filepath.Separator) {
		return "", fmt.Errorf("invalid store path for lock: %q", storePath)
	}
	if absTarget, err := filepath.Abs(target); err == nil {
		target = absTarget
	}

	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".lock"), nil
}
