package audit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

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

// Logger appends audit events to a rotating log file.
type Logger struct {
	mu   sync.Mutex
	file io.WriteCloser
	host string
}

// Open creates or opens the audit log file for appending.
// The log file is placed in the onessh config directory (parent of dataPath).
func Open(dataPath string, cfg RotateConfig) (*Logger, error) {
	if err := ValidateRotateConfig(cfg); err != nil {
		return nil, err
	}

	logPath := resolveLogPath(dataPath)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}

	// lumberjack lazily creates the log file on first write with mode 0600
	// (see lumberjack.Logger.openNew), so no explicit Chmod is needed here.
	// An os.Chmod call before first write would silently no-op via ErrNotExist
	// and be misleading.
	w := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}

	hostname := "-"
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		hostname = strings.TrimSpace(h)
	}

	return &Logger{file: w, host: hostname}, nil
}

// Log writes an event to the audit log.
func (l *Logger) Log(e Event) {
	if l == nil || l.file == nil {
		return
	}
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339)
	}

	line := formatRFC5424Event(e, l.host)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.file.Write([]byte(line + "\n"))
}

// Close flushes and closes the underlying file.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
