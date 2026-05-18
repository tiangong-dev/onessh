package audit

import (
	"path/filepath"
	"testing"

	legacyaudit "onessh/internal/audit"
	"onessh/internal/ports"
)

func TestLoggerLogProxiesToAuditLogger(t *testing.T) {
	t.Parallel()

	dataPath := filepath.Join(t.TempDir(), "data")
	logger, err := Open(dataPath, DefaultRotateConfig())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	logger.Log(Event{Action: "connect", Alias: "web1", Host: "1.2.3.4", User: "root", Result: "ok"})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events, err := legacyaudit.ReadLast(dataPath, 0, "", "")
	if err != nil {
		t.Fatalf("legacy ReadLast after facade Log: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Action != "connect" || events[0].Alias != "web1" || events[0].Result != "ok" {
		t.Fatalf("unexpected event: %+v", events[0])
	}
}

func TestReadLastProxiesToAuditRead(t *testing.T) {
	t.Parallel()

	dataPath := filepath.Join(t.TempDir(), "data")
	logger, err := legacyaudit.Open(dataPath, legacyaudit.DefaultRotateConfig())
	if err != nil {
		t.Fatalf("legacy Open: %v", err)
	}
	logger.Log(legacyaudit.Event{Action: "connect", Alias: "web1", Result: "ok"})
	logger.Log(legacyaudit.Event{Action: "exec", Alias: "web2", Result: "fail", Error: "timeout"})
	if err := logger.Close(); err != nil {
		t.Fatalf("legacy Close: %v", err)
	}

	events, err := ReadLast(dataPath, 1, "", "")
	if err != nil {
		t.Fatalf("ReadLast: %v", err)
	}
	if len(events) != 1 || events[0].Action != "exec" || events[0].Alias != "web2" {
		t.Fatalf("unexpected last event: %+v", events)
	}

	events, err = ReadLast(dataPath, 0, "connect", "")
	if err != nil {
		t.Fatalf("ReadLast filtered: %v", err)
	}
	if len(events) != 1 || events[0].Alias != "web1" {
		t.Fatalf("unexpected filtered events: %+v", events)
	}
}

func TestSettingsProxyToAuditSettings(t *testing.T) {
	t.Parallel()

	dataPath := filepath.Join(t.TempDir(), "data")

	settings, err := LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("LoadSettings default: %v", err)
	}
	if settings.Enabled {
		t.Fatalf("expected default settings to disable audit logging")
	}

	if err := SetEnabled(dataPath, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	legacySettings, err := legacyaudit.LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("legacy LoadSettings after facade SetEnabled: %v", err)
	}
	if !legacySettings.Enabled {
		t.Fatalf("expected legacy settings to observe enabled=true")
	}

	if err := legacyaudit.SetEnabled(dataPath, false); err != nil {
		t.Fatalf("legacy SetEnabled(false): %v", err)
	}
	settings, err = LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("LoadSettings after legacy SetEnabled: %v", err)
	}
	if settings.Enabled {
		t.Fatalf("expected facade settings to observe enabled=false")
	}
}

func TestPortLoggerAdapterProxiesAuditPortEvents(t *testing.T) {
	t.Parallel()

	dataPath := filepath.Join(t.TempDir(), "data")
	logger, err := Open(dataPath, DefaultRotateConfig())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sink := PortLoggerAdapter{Logger: logger}

	if err := sink.Log(ports.AuditEvent{Action: "exec", Alias: "web1", Result: "ok"}); err != nil {
		t.Fatalf("sink Log: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events, err := legacyaudit.ReadLast(dataPath, 0, "", "")
	if err != nil {
		t.Fatalf("legacy ReadLast after port adapter Log: %v", err)
	}
	if len(events) != 1 || events[0].Action != "exec" || events[0].Alias != "web1" {
		t.Fatalf("unexpected port adapter event: %+v", events)
	}
}
