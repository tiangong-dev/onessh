package audit

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

const (
	defaultLogFileName      = "audit.log"
	defaultSettingsFileName = "audit.yaml"
	defaultMaxSizeMB        = 10
	defaultMaxBackups       = 5
	defaultMaxAgeDays       = 7
	defaultCompress         = true
	defaultScannerBufferCap = 256 * 1024
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

// RotateConfig controls audit log rotation.
type RotateConfig struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// Settings controls audit logging behavior.
type Settings struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultRotateConfig returns the default rotation settings.
func DefaultRotateConfig() RotateConfig {
	return RotateConfig{
		MaxSizeMB:  defaultMaxSizeMB,
		MaxBackups: defaultMaxBackups,
		MaxAgeDays: defaultMaxAgeDays,
		Compress:   defaultCompress,
	}
}

// ValidateRotateConfig validates rotation settings.
func ValidateRotateConfig(cfg RotateConfig) error {
	if cfg.MaxSizeMB <= 0 {
		return fmt.Errorf("invalid --audit-log-max-size-mb=%d (must be > 0)", cfg.MaxSizeMB)
	}
	if cfg.MaxBackups < 1 {
		return fmt.Errorf("invalid --audit-log-max-backups=%d (must be >= 1)", cfg.MaxBackups)
	}
	if cfg.MaxAgeDays < 1 {
		return fmt.Errorf("invalid --audit-log-max-age=%d (must be >= 1)", cfg.MaxAgeDays)
	}
	return nil
}

// DefaultSettings returns default audit settings.
func DefaultSettings() Settings {
	return Settings{Enabled: false}
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

	w := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}
	if err := os.Chmod(logPath, 0o600); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("chmod audit log: %w", err)
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

// ReadLast reads the last N events from the audit log, optionally filtered.
func ReadLast(dataPath string, n int, action, alias string) ([]Event, error) {
	files, err := discoverLogFiles(resolveLogPath(dataPath))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	var all []Event
	for _, path := range files {
		if err := readLogFile(path, action, alias, &all); err != nil {
			return nil, err
		}
	}

	if n <= 0 || n >= len(all) {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// LoadSettings loads persisted audit settings.
func LoadSettings(dataPath string) (Settings, error) {
	path := resolveSettingsPath(dataPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return Settings{}, fmt.Errorf("read audit settings: %w", err)
	}
	var s Settings
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("decode audit settings: %w", err)
	}
	return s, nil
}

// SaveSettings persists audit settings atomically.
func SaveSettings(dataPath string, s Settings) error {
	path := resolveSettingsPath(dataPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create audit settings directory: %w", err)
	}
	encoded, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode audit settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".audit-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create audit settings temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod audit settings temp file: %w", err)
	}
	if _, err := tmp.Write(encoded); err != nil {
		cleanup()
		return fmt.Errorf("write audit settings temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync audit settings temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close audit settings temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename audit settings temp file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod audit settings file: %w", err)
	}
	return nil
}

// SetEnabled updates persisted audit enabled state.
func SetEnabled(dataPath string, enabled bool) error {
	return SaveSettings(dataPath, Settings{Enabled: enabled})
}

func discoverLogFiles(logPath string) ([]string, error) {
	dir := filepath.Dir(logPath)
	base := filepath.Base(logPath)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext) + "-"

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read audit log directory: %w", err)
	}

	type fileInfo struct {
		path string
		mod  time.Time
	}
	rotated := make([]fileInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !(strings.HasSuffix(name, ext) || strings.HasSuffix(name, ext+".gz")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		rotated = append(rotated, fileInfo{path: filepath.Join(dir, name), mod: info.ModTime()})
	}

	sort.Slice(rotated, func(i, j int) bool {
		if rotated[i].mod.Equal(rotated[j].mod) {
			return rotated[i].path < rotated[j].path
		}
		return rotated[i].mod.Before(rotated[j].mod)
	})

	files := make([]string, 0, len(rotated)+1)
	for _, f := range rotated {
		files = append(files, f.path)
	}

	if _, err := os.Stat(logPath); err == nil {
		files = append(files, logPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("open audit log: %w", err)
	}

	return files, nil
}

func readLogFile(path, action, alias string, out *[]Event) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("open gz audit log: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), defaultScannerBufferCap)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		e, ok := parseRFC5424Event(line)
		if !ok {
			continue
		}
		if action != "" && !strings.EqualFold(e.Action, action) {
			continue
		}
		if alias != "" && !strings.EqualFold(e.Alias, alias) {
			continue
		}
		*out = append(*out, e)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan audit log: %w", err)
	}
	return nil
}

func formatRFC5424Event(e Event, hostname string) string {
	ts := strings.TrimSpace(e.Time)
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(hostname) == "" {
		hostname = "-"
	}

	action := safeValue(e.Action)
	alias := safeValue(e.Alias)
	host := safeValue(e.Host)
	user := safeValue(e.User)
	result := safeValue(e.Result)
	errMsg := safeValue(e.Error)

	sd := fmt.Sprintf(
		"[onessh@32473 action=\"%s\" alias=\"%s\" host=\"%s\" user=\"%s\" result=\"%s\" error=\"%s\"]",
		escapeSD(action), escapeSD(alias), escapeSD(host), escapeSD(user), escapeSD(result), escapeSD(errMsg),
	)
	msg := fmt.Sprintf("action=%s result=%s alias=%s host=%s user=%s", action, result, alias, host, user)
	return fmt.Sprintf("<134>1 %s %s onessh - AUDIT %s %s", ts, hostname, sd, msg)
}

func parseRFC5424Event(line string) (Event, bool) {
	parts := strings.SplitN(line, " ", 7)
	if len(parts) < 7 {
		return Event{}, false
	}
	if !strings.HasPrefix(parts[0], "<") || !strings.HasSuffix(parts[0], ">1") {
		return Event{}, false
	}
	if parts[3] != "onessh" || parts[5] != "AUDIT" {
		return Event{}, false
	}

	rest := parts[6]
	sdPrefix := "[onessh@32473 "
	if !strings.HasPrefix(rest, sdPrefix) {
		return Event{}, false
	}
	closeIdx := findSDClose(rest)
	if closeIdx <= len(sdPrefix)-1 {
		return Event{}, false
	}

	sdBody := rest[len(sdPrefix):closeIdx]
	params, ok := parseSDParams(sdBody)
	if !ok {
		return Event{}, false
	}

	e := Event{
		Time:   parts[1],
		Action: normalizeDash(params["action"]),
		Alias:  normalizeDash(params["alias"]),
		Host:   normalizeDash(params["host"]),
		User:   normalizeDash(params["user"]),
		Result: normalizeDash(params["result"]),
		Error:  normalizeDash(params["error"]),
	}
	if e.Action == "" || e.Result == "" {
		return Event{}, false
	}
	return e, true
}

func findSDClose(s string) int {
	inQuotes := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuotes {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inQuotes = false
			}
			continue
		}
		if ch == '"' {
			inQuotes = true
			continue
		}
		if ch == ']' {
			return i
		}
	}
	return -1
}

func parseSDParams(body string) (map[string]string, bool) {
	params := map[string]string{}
	i := 0
	for i < len(body) {
		for i < len(body) && body[i] == ' ' {
			i++
		}
		if i >= len(body) {
			break
		}

		eq := strings.IndexByte(body[i:], '=')
		if eq < 1 {
			return nil, false
		}
		eq += i
		key := body[i:eq]
		i = eq + 1
		if i >= len(body) || body[i] != '"' {
			return nil, false
		}
		i++ // skip opening quote
		start := i
		for i < len(body) {
			if body[i] == '"' && !isEscaped(body, i) {
				break
			}
			i++
		}
		if i >= len(body) {
			return nil, false
		}
		params[key] = unescapeSD(body[start:i])
		i++ // skip closing quote
	}
	return params, true
}

func isEscaped(s string, pos int) bool {
	count := 0
	for i := pos - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func safeValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func normalizeDash(v string) string {
	if v == "-" {
		return ""
	}
	return v
}

func escapeSD(v string) string {
	var sb strings.Builder
	sb.Grow(len(v))
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if ch == '\\' || ch == '"' || ch == ']' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(ch)
	}
	return sb.String()
}

func unescapeSD(v string) string {
	var sb strings.Builder
	sb.Grow(len(v))
	escaped := false
	for i := 0; i < len(v); i++ {
		ch := v[i]
		if escaped {
			sb.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		sb.WriteByte(ch)
	}
	if escaped {
		sb.WriteByte('\\')
	}
	return sb.String()
}

func resolveLogPath(dataPath string) string {
	// Place audit.log in the parent of the data directory (e.g. ~/.config/onessh/).
	return filepath.Join(filepath.Dir(dataPath), defaultLogFileName)
}

func resolveSettingsPath(dataPath string) string {
	return filepath.Join(filepath.Dir(dataPath), defaultSettingsFileName)
}
