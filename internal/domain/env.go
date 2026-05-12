package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidateEnvKey(key string) error {
	if !envKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid env key %q", key)
	}
	return nil
}

func ParseEnvAssignments(values []string) (map[string]string, error) {
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
		if err := ValidateEnvKey(key); err != nil {
			return nil, err
		}
		result[key] = parts[1]
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func ParseEnvKeys(values []string) ([]string, error) {
	keys := make([]string, 0, len(values))
	for _, raw := range values {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, errors.New("env key cannot be empty")
		}
		if err := ValidateEnvKey(key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}
