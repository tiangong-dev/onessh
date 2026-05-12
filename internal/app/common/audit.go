package common

import "onessh/internal/ports"

// RecordAuditResult writes an operation result and deliberately ignores sink failures.
func RecordAuditResult(audit ports.Audit, action, alias, host, user string, err error) {
	if audit == nil {
		return
	}
	event := ports.AuditEvent{
		Action: action,
		Alias:  alias,
		Host:   host,
		User:   user,
		Result: "ok",
	}
	if err != nil {
		event.Result = "fail"
		event.Error = err.Error()
	}
	_ = audit.Log(event)
}
