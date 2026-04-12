package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

func promptNonEmpty(reader *bufio.Reader, label, defaultValue string) (string, error) {
	for {
		value, err := promptOptional(reader, label, defaultValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
}

func promptOptional(reader *bufio.Reader, label, defaultValue string) (string, error) {
	out := promptWriter()
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	text := strings.TrimSpace(line)
	if text == "" {
		return defaultValue, nil
	}
	return text, nil
}

func promptYesNo(reader *bufio.Reader, label string, defaultYes bool) (bool, error) {
	defaultValue := "n"
	if defaultYes {
		defaultValue = "y"
	}

	for {
		value, err := promptOptional(reader, label+" (y/N)", defaultValue)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes":
			return true, nil
		case "n", "no", "":
			return false, nil
		default:
			fmt.Fprintln(promptWriter(), "Please enter y or n.")
		}
	}
}

func promptPort(reader *bufio.Reader, defaultPort int) (int, error) {
	out := promptWriter()
	for {
		raw, err := promptOptional(reader, "Port", strconv.Itoa(defaultPort))
		if err != nil {
			return 0, err
		}
		port, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || port <= 0 || port > 65535 {
			fmt.Fprintln(out, "Port must be a number between 1 and 65535.")
			continue
		}
		return port, nil
	}
}

func promptRequiredPassword(prompt string) ([]byte, error) {
	out := promptWriter()
	fmt.Fprint(out, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return nil, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	return secret, nil
}

func promptOptionalSecret(prompt string) ([]byte, bool, error) {
	out := promptWriter()
	fmt.Fprint(out, prompt)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(out)
	if err != nil {
		return nil, false, err
	}
	secret = bytes.TrimSpace(secret)
	if len(secret) == 0 {
		return nil, false, nil
	}
	return secret, true, nil
}

func promptWriter() io.Writer {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		return os.Stderr
	}
	return os.Stdout
}
