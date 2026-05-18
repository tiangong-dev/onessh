package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// newPasswordFD returns a pipe reader pre-loaded with the SSH password followed
// by a newline. The write happens in a background goroutine so passwords longer
// than the OS pipe buffer (typically 64KiB on Linux) do not deadlock the
// caller. The returned cleanup closes the reader, waits for the writer
// goroutine to drain, and returns any non-benign write error so the caller
// can surface it alongside the command result.
func newPasswordFD(password string) (*os.File, func() error, error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create password pipe: %w", err)
	}

	secret := append([]byte(password), '\n')
	writeErr := make(chan error, 1)
	go func() {
		defer close(writeErr)
		// password string copy stays in heap; Go has no API to wipe immutable strings
		defer wipe(secret)
		defer func() { _ = writer.Close() }()
		if _, err := writer.Write(secret); err != nil {
			writeErr <- err
		}
	}()

	cleanup := func() error {
		_ = reader.Close()
		// Drain the goroutine. Closing the reader before the goroutine finishes
		// can cause Write to fail with EPIPE / ErrClosed; that race is benign
		// because the consumer (typically sshpass) already exited.
		err, ok := <-writeErr
		if !ok || err == nil {
			return nil
		}
		if errors.Is(err, os.ErrClosed) {
			return nil
		}
		return fmt.Errorf("password fd write: %w", err)
	}
	return reader, cleanup, nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func containsShortFlag(args []string, flag rune) bool {
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' || strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.ContainsRune(arg[1:], flag) {
			return true
		}
	}
	return false
}
