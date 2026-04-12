package audit

import "fmt"

const (
	defaultMaxSizeMB  = 10
	defaultMaxBackups = 5
	defaultMaxAgeDays = 7
	defaultCompress   = true
)

// RotateConfig controls audit log rotation.
type RotateConfig struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// DefaultRotateConfig returns the default rotation settings.
func DefaultRotateConfig() RotateConfig {
	return RotateConfig{
		MaxSizeMB:  defaultMaxSizeMB,
		MaxBackups: defaultMaxBackups,
		MaxAgeDays: defaultMaxAgeDays,
		Compress:   defaultCompress,
	}
}

// ValidateRotateConfig validates rotation settings.
func ValidateRotateConfig(cfg RotateConfig) error {
	if cfg.MaxSizeMB <= 0 {
		return fmt.Errorf("invalid --audit-log-max-size-mb=%d (must be > 0)", cfg.MaxSizeMB)
	}
	if cfg.MaxBackups < 1 {
		return fmt.Errorf("invalid --audit-log-max-backups=%d (must be >= 1)", cfg.MaxBackups)
	}
	if cfg.MaxAgeDays < 1 {
		return fmt.Errorf("invalid --audit-log-max-age=%d (must be >= 1)", cfg.MaxAgeDays)
	}
	return nil
}
