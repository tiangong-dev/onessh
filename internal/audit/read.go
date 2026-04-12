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
	"time"
)

const defaultScannerBufferCap = 256 * 1024

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
