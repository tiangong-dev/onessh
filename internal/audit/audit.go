package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const auditFileName = "audit.log"

// Entry represents a single audit log record.
type Entry struct {
	Timestamp time.Time `json:"ts"`
	Action    string    `json:"action"`
	Host      string    `json:"host,omitempty"`
	User      string    `json:"user,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status"`
}

// Filter controls which entries are returned by ReadEntries.
type Filter struct {
	Action string
	Host   string
	Since  time.Time
	Until  time.Time
	Last   int
}

// Logger writes audit entries to an append-only JSON Lines file.
type Logger struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// Open creates or opens the audit log file inside dataDir.
func Open(dataDir string) (*Logger, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}

	path := filepath.Join(dataDir, auditFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	return &Logger{file: f, enc: enc}, nil
}

// Log writes a single entry to the audit log. It is safe for concurrent use.
func (l *Logger) Log(e Entry) {
	if l == nil || l.file == nil {
		return
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Status == "" {
		e.Status = "ok"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.enc.Encode(e)
}

// Close closes the underlying file.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// FilePath returns the audit log file path for dataDir.
func FilePath(dataDir string) string {
	return filepath.Join(dataDir, auditFileName)
}

// ReadEntries reads and optionally filters audit log entries from the file.
func ReadEntries(dataDir string, filter Filter) ([]Entry, error) {
	path := FilePath(dataDir)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		if !matchFilter(e, filter) {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}

	if filter.Last > 0 && len(entries) > filter.Last {
		entries = entries[len(entries)-filter.Last:]
	}

	return entries, nil
}

// ClearEntries truncates or prunes the audit log.
// If before is zero, the entire file is truncated.
// Otherwise only entries older than before are removed.
func ClearEntries(dataDir string, before time.Time) error {
	path := FilePath(dataDir)

	if before.IsZero() {
		return os.Truncate(path, 0)
	}

	// Keep entries at or after `before`.
	entries, err := ReadEntries(dataDir, Filter{})
	if err != nil {
		return err
	}

	var keep []Entry
	for _, e := range entries {
		if !e.Timestamp.Before(before) {
			keep = append(keep, e)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("rewrite audit log: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, e := range keep {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("write audit entry: %w", err)
		}
	}

	return nil
}

func matchFilter(e Entry, f Filter) bool {
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if f.Host != "" && e.Host != f.Host {
		return false
	}
	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}
	return true
}
