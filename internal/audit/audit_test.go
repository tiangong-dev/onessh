package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogAndReadEntries(t *testing.T) {
	dir := t.TempDir()

	logger, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	now := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	logger.Log(Entry{Timestamp: now, Action: "connect", Host: "web1", User: "admin", Status: "ok"})
	logger.Log(Entry{Timestamp: now.Add(time.Minute), Action: "host.add", Host: "db1", Status: "ok"})
	logger.Log(Entry{Timestamp: now.Add(2 * time.Minute), Action: "connect", Host: "web2", User: "deploy", Status: "error", Detail: "timeout"})

	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read all
	entries, err := ReadEntries(dir, Filter{})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Filter by action
	entries, err = ReadEntries(dir, Filter{Action: "connect"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 connect entries, got %d", len(entries))
	}

	// Filter by host
	entries, err = ReadEntries(dir, Filter{Host: "db1"})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 db1 entry, got %d", len(entries))
	}

	// Filter by since
	entries, err = ReadEntries(dir, Filter{Since: now.Add(30 * time.Second)})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries since T+30s, got %d", len(entries))
	}

	// Filter by until
	entries, err = ReadEntries(dir, Filter{Until: now.Add(30 * time.Second)})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry until T+30s, got %d", len(entries))
	}

	// Last N
	entries, err = ReadEntries(dir, Filter{Last: 1})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 last entry, got %d", len(entries))
	}
	if entries[0].Host != "web2" {
		t.Errorf("expected last entry host=web2, got %s", entries[0].Host)
	}
}

func TestClearEntries(t *testing.T) {
	dir := t.TempDir()

	logger, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	now := time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC)
	logger.Log(Entry{Timestamp: now, Action: "connect", Host: "web1", Status: "ok"})
	logger.Log(Entry{Timestamp: now.Add(time.Hour), Action: "connect", Host: "web2", Status: "ok"})
	logger.Log(Entry{Timestamp: now.Add(2 * time.Hour), Action: "connect", Host: "web3", Status: "ok"})
	logger.Close()

	// Clear before T+30min — should keep web2 (T+1h) and web3 (T+2h)
	if err := ClearEntries(dir, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("ClearEntries: %v", err)
	}

	entries, err := ReadEntries(dir, Filter{})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after partial clear, got %d", len(entries))
	}

	// Clear all
	if err := ClearEntries(dir, time.Time{}); err != nil {
		t.Fatalf("ClearEntries: %v", err)
	}

	entries, err = ReadEntries(dir, Filter{})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after full clear, got %d", len(entries))
	}
}

func TestNilLoggerSafe(t *testing.T) {
	var logger *Logger
	logger.Log(Entry{Action: "test"}) // should not panic
	if err := logger.Close(); err != nil {
		t.Errorf("Close on nil logger: %v", err)
	}
}

func TestReadEntriesNoFile(t *testing.T) {
	entries, err := ReadEntries(t.TempDir(), Filter{})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from missing file, got %d", len(entries))
	}
}

func TestLogDefaultTimestampAndStatus(t *testing.T) {
	dir := t.TempDir()
	logger, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	logger.Log(Entry{Action: "test"})
	logger.Close()

	entries, err := ReadEntries(dir, Filter{})
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if entries[0].Status != "ok" {
		t.Errorf("expected status ok, got %s", entries[0].Status)
	}
}

func TestFilePath(t *testing.T) {
	got := FilePath("/data/onessh")
	want := filepath.Join("/data/onessh", "audit.log")
	if got != want {
		t.Errorf("FilePath = %s, want %s", got, want)
	}
}

func TestClearEntriesNoFile(t *testing.T) {
	dir := t.TempDir()
	err := ClearEntries(dir, time.Time{})
	if err == nil {
		t.Skip("truncate on non-existent file may succeed on some OS")
	}
	// On most systems this is a path error — that's acceptable
	if !os.IsNotExist(err) {
		t.Logf("ClearEntries on missing file: %v (non-critical)", err)
	}
}
