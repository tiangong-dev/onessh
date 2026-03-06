package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultLogFileName = "audit.log"

// Event represents a single audit log entry.
type Event struct {
	Time   string            `json:"time"`
	Action string            `json:"action"`
	Alias  string            `json:"alias,omitempty"`
	Host   string            `json:"host,omitempty"`
	User   string            `json:"user,omitempty"`
	Result string            `json:"result"`
	Error  string            `json:"error,omitempty"`
	Extra  map[string]string `json:"extra,omitempty"`
}

// Logger appends audit events to a JSON-Lines file.
type Logger struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// Open creates or opens the audit log file for appending.
// The log file is placed in the onessh config directory (parent of dataPath).
func Open(dataPath string) (*Logger, error) {
	logPath := resolveLogPath(dataPath)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &Logger{file: f, enc: json.NewEncoder(f)}, nil
}

// Log writes an event to the audit log.
func (l *Logger) Log(e Event) {
	if l == nil {
		return
	}
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.enc.Encode(e)
}

// Close flushes and closes the underlying file.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// ReadLast reads the last N events from the audit log, optionally filtered.
func ReadLast(dataPath string, n int, action, alias string) ([]Event, error) {
	logPath := resolveLogPath(dataPath)
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	var all []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if action != "" && !strings.EqualFold(e.Action, action) {
			continue
		}
		if alias != "" && !strings.EqualFold(e.Alias, alias) {
			continue
		}
		all = append(all, e)
	}

	if n <= 0 || n >= len(all) {
		return all, nil
	}
	return all[len(all)-n:], nil
}

func resolveLogPath(dataPath string) string {
	// Place audit.log in the parent of the data directory (e.g. ~/.config/onessh/).
	return filepath.Join(filepath.Dir(dataPath), defaultLogFileName)
}
