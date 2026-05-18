package common

import "onessh/internal/ports"

// RecordAuditResult writes an operation result and deliberately ignores sink failures.
func RecordAuditResult(audit ports.Audit, action, alias, host, user string, err error) {
	result := "ok"
	if err != nil {
		result = "fail"
	}
	RecordAuditStatus(audit, action, alias, host, user, result, err)
}

// RecordAuditStatus writes an operation status and deliberately ignores sink failures.
func RecordAuditStatus(audit ports.Audit, action, alias, host, user, result string, err error) {
	if audit == nil {
		return
	}
	event := ports.AuditEvent{
		Action: action,
		Alias:  alias,
		Host:   host,
		User:   user,
		Result: result,
	}
	if err != nil {
		event.Error = err.Error()
	}
	_ = audit.Log(event)
}
