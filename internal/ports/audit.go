package ports

// AuditEvent is the stable event shape accepted by audit logging ports.
type AuditEvent struct {
	Time   string
	Action string
	Alias  string
	Host   string
	User   string
	Result string
	Error  string
	Extra  map[string]string
}

// AuditLogger is the narrow write-side audit logging port.
type AuditLogger interface {
	Log(AuditEvent)
}

// AuditLoggerFunc adapts a function to AuditLogger.
type AuditLoggerFunc func(AuditEvent)

func (f AuditLoggerFunc) Log(event AuditEvent) {
	f(event)
}
