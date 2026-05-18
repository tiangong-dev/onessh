package audit

import (
	legacyaudit "onessh/internal/audit"
	"onessh/internal/ports"
)

type Event = legacyaudit.Event
type RotateConfig = legacyaudit.RotateConfig
type Settings = legacyaudit.Settings

// Logger is the infrastructure-facing facade over the existing audit logger.
type Logger struct {
	logger *legacyaudit.Logger
}

func DefaultRotateConfig() RotateConfig {
	return legacyaudit.DefaultRotateConfig()
}

func ValidateRotateConfig(cfg RotateConfig) error {
	return legacyaudit.ValidateRotateConfig(cfg)
}

func Open(dataPath string, cfg RotateConfig) (*Logger, error) {
	logger, err := legacyaudit.Open(dataPath, cfg)
	if err != nil {
		return nil, err
	}
	return &Logger{logger: logger}, nil
}

func (l *Logger) Log(e Event) {
	if l == nil {
		return
	}
	l.logger.Log(e)
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	return l.logger.Close()
}

func ReadLast(dataPath string, n int, action, alias string) ([]Event, error) {
	return legacyaudit.ReadLast(dataPath, n, action, alias)
}

func DefaultSettings() Settings {
	return legacyaudit.DefaultSettings()
}

func LoadSettings(dataPath string) (Settings, error) {
	return legacyaudit.LoadSettings(dataPath)
}

func SaveSettings(dataPath string, settings Settings) error {
	return legacyaudit.SaveSettings(dataPath, settings)
}

func SetEnabled(dataPath string, enabled bool) error {
	return legacyaudit.SetEnabled(dataPath, enabled)
}

var _ ports.Audit = PortLoggerAdapter{}

// PortLoggerAdapter adapts the facade logger to the audit port.
type PortLoggerAdapter struct {
	Logger *Logger
}

func (a PortLoggerAdapter) Log(event ports.AuditEvent) error {
	if a.Logger == nil {
		return nil
	}
	a.Logger.Log(Event{
		Time:   event.Time,
		Action: event.Action,
		Alias:  event.Alias,
		Host:   event.Host,
		User:   event.User,
		Result: event.Result,
		Error:  event.Error,
		Extra:  event.Extra,
	})
	return nil
}
