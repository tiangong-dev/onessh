package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func newPasswordFD(password string) (*os.File, func(), error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create password pipe: %w", err)
	}

	secret := append([]byte(password), '\n')
	defer wipe(secret)

	if _, err := writer.Write(secret); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("write password to pipe: %w", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		return nil, nil, fmt.Errorf("close password pipe writer: %w", err)
	}

	cleanup := func() {
		_ = reader.Close()
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
