package audit

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Settings controls audit logging behavior.
type Settings struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultSettings returns default audit settings.
func DefaultSettings() Settings {
	return Settings{Enabled: false}
}

// LoadSettings loads persisted audit settings.
func LoadSettings(dataPath string) (Settings, error) {
	path := resolveSettingsPath(dataPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return Settings{}, fmt.Errorf("read audit settings: %w", err)
	}
	var s Settings
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("decode audit settings: %w", err)
	}
	return s, nil
}

// SaveSettings persists audit settings atomically.
func SaveSettings(dataPath string, s Settings) error {
	path := resolveSettingsPath(dataPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create audit settings directory: %w", err)
	}
	encoded, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode audit settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".audit-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create audit settings temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod audit settings temp file: %w", err)
	}
	if _, err := tmp.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write audit settings temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync audit settings temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close audit settings temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename audit settings temp file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod audit settings file: %w", err)
	}
	return nil
}

// SetEnabled updates persisted audit enabled state.
func SetEnabled(dataPath string, enabled bool) error {
	return SaveSettings(dataPath, Settings{Enabled: enabled})
}
