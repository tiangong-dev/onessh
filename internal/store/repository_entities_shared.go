package store

import (
	"errors"
	"sort"
	"strings"
)

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateAlias(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return errors.New("alias is empty")
	}
	if !aliasPattern.MatchString(alias) {
		return errors.New("alias must match [A-Za-z0-9._-]+")
	}
	return nil
}

func normalizeAuthTypeStore(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "key":
		return "key"
	case "password":
		return "password"
	default:
		return ""
	}
}
