package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogAndReadLast(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatal(err)
	}

	logger, err := Open(dataPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	logger.Log(Event{Action: "connect", Alias: "web1", Host: "1.2.3.4", User: "root", Result: "ok"})
	logger.Log(Event{Action: "exec", Alias: "web2", Host: "5.6.7.8", User: "admin", Result: "fail", Error: "timeout"})
	logger.Log(Event{Action: "connect", Alias: "db1", Host: "10.0.0.1", User: "root", Result: "ok"})
	logger.Close()

	// Read all
	events, err := ReadLast(dataPath, 0, "", "")
	if err != nil {
		t.Fatalf("ReadLast all: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Read last 2
	events, err = ReadLast(dataPath, 2, "", "")
	if err != nil {
		t.Fatalf("ReadLast 2: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Alias != "web2" {
		t.Errorf("expected web2, got %s", events[0].Alias)
	}

	// Filter by action
	events, err = ReadLast(dataPath, 0, "connect", "")
	if err != nil {
		t.Fatalf("ReadLast action=connect: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 connect events, got %d", len(events))
	}

	// Filter by alias
	events, err = ReadLast(dataPath, 0, "", "db1")
	if err != nil {
		t.Fatalf("ReadLast alias=db1: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for db1, got %d", len(events))
	}
}

func TestLogNilLogger(t *testing.T) {
	var logger *Logger
	logger.Log(Event{Action: "test", Result: "ok"}) // should not panic
	if err := logger.Close(); err != nil {
		t.Errorf("Close nil logger: %v", err)
	}
}

func TestReadLastMissingFile(t *testing.T) {
	events, err := ReadLast("/nonexistent/path/data", 10, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}
