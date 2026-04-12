package audit

import "path/filepath"

const (
	defaultLogFileName      = "audit.log"
	defaultSettingsFileName = "audit.yaml"
)

func resolveLogPath(dataPath string) string {
	// Place audit.log in the parent of the data directory (e.g. ~/.config/onessh/).
	return filepath.Join(filepath.Dir(dataPath), defaultLogFileName)
}

func resolveSettingsPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), defaultSettingsFileName)
}
