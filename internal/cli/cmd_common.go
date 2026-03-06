package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"onessh/internal/store"

	"golang.org/x/term"
)

func loadConfig(opts *rootOptions, repo store.Repository) (store.PlainConfig, []byte, error) {
	cache, err := opts.passphraseStore(repo.Path)
	if err != nil {
		return store.PlainConfig{}, nil, err
	}
	if cachedPassphrase, ok, _ := cache.Get(); ok {
		cfg, loadErr := repo.Load(cachedPassphrase)
		if loadErr == nil {
			return cfg, cachedPassphrase, nil
		}
		wipe(cachedPassphrase)
		_ = cache.Clear()
	}

	passphrase, err := promptRequiredPassword("Enter master password: ")
	if err != nil {
		return store.PlainConfig{}, nil, err
	}

	cfg, err := repo.Load(passphrase)
	if err != nil {
		wipe(passphrase)
		if errors.Is(err, store.ErrConfigNotFound) {
			return store.PlainConfig{}, nil, fmt.Errorf("%w (run `onessh init` first)", err)
		}
		return store.PlainConfig{}, nil, err
	}

	if cache.IsEnabled() {
		_ = cache.Set(passphrase)
	}

	return cfg, passphrase, nil
}

func redactConfigForDump(cfg store.PlainConfig) store.PlainConfig {
	redacted := store.NewPlainConfig()

	for alias, userCfg := range cfg.Users {
		userCopy := userCfg
		if normalizeAuthType(userCopy.Auth.Type) == "password" && strings.TrimSpace(userCopy.Auth.Password) != "" {
			userCopy.Auth.Password = redactedSecretValue
		}
		redacted.Users[alias] = userCopy
	}

	for alias, hostCfg := range cfg.Hosts {
		hostCopy := hostCfg
		if len(hostCfg.Env) > 0 {
			hostCopy.Env = make(map[string]string, len(hostCfg.Env))
			for key := range hostCfg.Env {
				hostCopy.Env[key] = redactedSecretValue
			}
		}
		redacted.Hosts[alias] = hostCopy
	}

	return redacted
}

func parseEnvAssignments(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, raw := range values {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, errors.New("env entry cannot be empty")
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env entry %q, expected KEY=VALUE", raw)
		}
		key := strings.TrimSpace(parts[0])
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid env key %q", key)
		}
		result[key] = parts[1]
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func parseEnvKeys(values []string) ([]string, error) {
	keys := make([]string, 0, len(values))
	for _, raw := range values {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("env key cannot be empty")
		}
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid env key %q", key)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func parseHookCommands(values []string, flagName string) ([]string, error) {
	commands := make([]string, 0, len(values))
	for i, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%s command at index %d is empty", flagName, i)
		}
		commands = append(commands, trimmed)
	}
	if len(commands) == 0 {
		return nil, nil
	}
	return commands, nil
}

func appendSendEnvOptions(args []string, envMap map[string]string) []string {
	if len(envMap) == 0 {
		return args
	}
	keys := sortedStringMapKeys(envMap)
	for _, key := range keys {
		args = append(args, "-o", "SendEnv="+key)
	}
	return args
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	merged := map[string]string{}
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	for key, value := range overrides {
		merged[key] = value
	}

	keys := sortedStringMapKeys(merged)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+merged[key])
	}
	return result
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func sanitizeHookCommands(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

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

func wipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

func expandTilde(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/" + strings.TrimPrefix(input, "~/"), nil
	}
	return input, nil
}
