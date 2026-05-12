package audit

import "onessh/internal/ports"

var _ ports.AuditLogger = PortLoggerAdapter{}

// PortLoggerAdapter adapts Logger to the ports.Audit interface.
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
