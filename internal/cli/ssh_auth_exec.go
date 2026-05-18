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
// caller. The returned cleanup closes the reader and waits for the writer
// goroutine to drain; any write error is surfaced on stderr because the
// PasswordFDProvider contract does not carry an error channel back to the
// caller.
func newPasswordFD(password string) (*os.File, func(), error) {
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

	cleanup := func() {
		_ = reader.Close()
		// Drain the goroutine and surface any non-EOF write error. Closing the
		// reader before the goroutine finishes can cause Write to fail with
		// EPIPE; we ignore that benign race here because the consumer
		// (typically sshpass) already exited.
		if err, ok := <-writeErr; ok && err != nil && !errors.Is(err, os.ErrClosed) {
			fmt.Fprintf(os.Stderr, "onessh: password fd writer error: %v\n", err)
		}
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
