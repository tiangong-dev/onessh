package cli

import (
	"errors"
	"fmt"
	"strings"
)

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
