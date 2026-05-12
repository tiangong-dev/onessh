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

// Audit is the narrow write-side audit logging port.
type Audit interface {
	Log(AuditEvent) error
}

// AuditLogger is kept as a compatibility alias for older call sites.
type AuditLogger = Audit

// AuditFunc adapts a function to Audit.
type AuditFunc func(AuditEvent) error

func (f AuditFunc) Log(event AuditEvent) error {
	return f(event)
}

// AuditLoggerFunc adapts a fire-and-forget function to Audit.
type AuditLoggerFunc func(AuditEvent)

func (f AuditLoggerFunc) Log(event AuditEvent) error {
	f(event)
	return nil
}
