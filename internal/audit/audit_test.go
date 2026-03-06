package audit

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRotateConfig(t *testing.T) {
	if err := ValidateRotateConfig(DefaultRotateConfig()); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if err := ValidateRotateConfig(RotateConfig{MaxSizeMB: 0, MaxBackups: 5, MaxAgeDays: 7, Compress: true}); err == nil {
		t.Fatal("expected invalid max-size")
	}
	if err := ValidateRotateConfig(RotateConfig{MaxSizeMB: 10, MaxBackups: 0, MaxAgeDays: 7, Compress: true}); err == nil {
		t.Fatal("expected invalid max-backups")
	}
	if err := ValidateRotateConfig(RotateConfig{MaxSizeMB: 10, MaxBackups: 5, MaxAgeDays: 0, Compress: true}); err == nil {
		t.Fatal("expected invalid max-age")
	}
}

func TestLogAndReadLast(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatal(err)
	}

	logger, err := Open(dataPath, DefaultRotateConfig())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	logger.Log(Event{Action: "connect", Alias: "web1", Host: "1.2.3.4", User: "root", Result: "ok"})
	logger.Log(Event{Action: "exec", Alias: "web2", Host: "5.6.7.8", User: "admin", Result: "fail", Error: "timeout"})
	logger.Log(Event{Action: "connect", Alias: "db1", Host: "10.0.0.1", User: "root", Result: "ok"})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events, err := ReadLast(dataPath, 0, "", "")
	if err != nil {
		t.Fatalf("ReadLast all: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

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

	events, err = ReadLast(dataPath, 0, "connect", "")
	if err != nil {
		t.Fatalf("ReadLast action=connect: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 connect events, got %d", len(events))
	}

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
	logger.Log(Event{Action: "test", Result: "ok"})
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

func TestFormatAndParseRFC5424(t *testing.T) {
	e := Event{
		Time:   "2026-03-06T12:34:56Z",
		Action: "exec",
		Alias:  `web\"1]`,
		Host:   "10.0.0.1",
		User:   "root",
		Result: "fail",
		Error:  `timeout \"oops\"]`,
	}
	line := formatRFC5424Event(e, "my-host")
	parsed, ok := parseRFC5424Event(line)
	if !ok {
		t.Fatalf("parse failed for line: %s", line)
	}
	if parsed.Action != e.Action || parsed.Alias != e.Alias || parsed.Error != e.Error {
		t.Fatalf("roundtrip mismatch: parsed=%+v want=%+v", parsed, e)
	}
}

func TestReadLastAcrossRotatedAndCompressed(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Dir(dataPath)

	oldPlain := filepath.Join(logDir, "audit-2026-03-06T10-00-00.000.log")
	oldGZ := filepath.Join(logDir, "audit-2026-03-06T11-00-00.000.log.gz")
	current := filepath.Join(logDir, "audit.log")

	line1 := formatRFC5424Event(Event{Time: "2026-03-06T10:00:00Z", Action: "connect", Alias: "a1", Result: "ok"}, "h1")
	line2 := formatRFC5424Event(Event{Time: "2026-03-06T11:00:00Z", Action: "exec", Alias: "a2", Result: "ok"}, "h1")
	line3 := formatRFC5424Event(Event{Time: "2026-03-06T12:00:00Z", Action: "exec", Alias: "a3", Result: "fail", Error: "timeout"}, "h1")

	if err := os.WriteFile(oldPlain, []byte(line1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeGzipFile(oldGZ, []byte(line2+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, []byte(line3+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	_ = os.Chtimes(oldPlain, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	_ = os.Chtimes(oldGZ, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	events, err := ReadLast(dataPath, 0, "", "")
	if err != nil {
		t.Fatalf("ReadLast: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Alias != "a1" || events[1].Alias != "a2" || events[2].Alias != "a3" {
		t.Fatalf("unexpected order: %+v", events)
	}

	execEvents, err := ReadLast(dataPath, 0, "exec", "")
	if err != nil {
		t.Fatalf("ReadLast action filter: %v", err)
	}
	if len(execEvents) != 2 {
		t.Fatalf("expected 2 exec events, got %d", len(execEvents))
	}

	lastOne, err := ReadLast(dataPath, 1, "", "")
	if err != nil {
		t.Fatalf("ReadLast last=1: %v", err)
	}
	if len(lastOne) != 1 || lastOne[0].Alias != "a3" {
		t.Fatalf("expected last alias a3, got %+v", lastOne)
	}
}

func TestRotateBySizeKeepsBoundedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := RotateConfig{MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 7, Compress: true}
	logger, err := Open(dataPath, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	payload := strings.Repeat("x", 200*1024)
	for i := 0; i < 20; i++ {
		logger.Log(Event{Action: "exec", Alias: "rot", Result: "ok", Error: payload})
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	files, err := os.ReadDir(filepath.Dir(dataPath))
	if err != nil {
		t.Fatal(err)
	}

	var currentCount int
	var rotatedCount int
	var compressedCount int
	for _, f := range files {
		name := f.Name()
		switch {
		case name == "audit.log":
			currentCount++
		case strings.HasPrefix(name, "audit-") && (strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz")):
			rotatedCount++
			if strings.HasSuffix(name, ".gz") {
				compressedCount++
			}
		}
	}

	if currentCount != 1 {
		t.Fatalf("expected current audit.log, got %d", currentCount)
	}
	if rotatedCount > cfg.MaxBackups+1 {
		t.Fatalf("expected rotated files <= %d, got %d", cfg.MaxBackups+1, rotatedCount)
	}
	if compressedCount == 0 {
		t.Fatalf("expected at least one compressed rotated file")
	}
}

func TestAuditSettingsDefaultAndSetEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("LoadSettings default: %v", err)
	}
	if s.Enabled {
		t.Fatalf("expected default enabled=false, got true")
	}

	if err := SetEnabled(dataPath, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	s, err = LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("LoadSettings after enable: %v", err)
	}
	if !s.Enabled {
		t.Fatalf("expected enabled=true")
	}

	if err := SetEnabled(dataPath, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	s, err = LoadSettings(dataPath)
	if err != nil {
		t.Fatalf("LoadSettings after disable: %v", err)
	}
	if s.Enabled {
		t.Fatalf("expected enabled=false")
	}
}

func writeGzipFile(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(content); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return nil
}
