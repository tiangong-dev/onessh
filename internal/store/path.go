package store

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveDataPath determines the on-disk location of the encrypted store.
//
// Precedence: an explicit customPath wins, then envPath, otherwise we fall
// back to ~/.config/onessh/data. The homeDir argument is required when
// customPath/envPath start with "~" or when neither is provided.
func ResolveDataPath(customPath, envPath, homeDir string) (string, error) {
	if strings.TrimSpace(customPath) != "" {
		return expandWithHome(customPath, homeDir)
	}
	if strings.TrimSpace(envPath) != "" {
		return expandWithHome(envPath, homeDir)
	}
	if strings.TrimSpace(homeDir) == "" {
		return "", fmt.Errorf("resolve home directory: home directory is empty")
	}
	return filepath.Join(homeDir, ".config", "onessh", "data"), nil
}

func expandWithHome(path, homeDir string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" {
		if strings.TrimSpace(homeDir) == "" {
			return "", fmt.Errorf("expand ~: home directory is empty")
		}
		return homeDir, nil
	}
	if strings.HasPrefix(path, "~/") {
		if strings.TrimSpace(homeDir) == "" {
			return "", fmt.Errorf("expand ~: home directory is empty")
		}
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}
