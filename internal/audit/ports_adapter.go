package audit

import "onessh/internal/ports"

var _ ports.AuditLogger = PortLoggerAdapter{}

// PortLoggerAdapter adapts Logger to the ports.AuditLogger interface.
type PortLoggerAdapter struct {
	Logger *Logger
}

func (a PortLoggerAdapter) Log(event ports.AuditEvent) {
	if a.Logger == nil {
		return
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
}
