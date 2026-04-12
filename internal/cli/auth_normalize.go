package cli

import (
	"strings"

	"onessh/internal/store"
)

func normalizeAuthType(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "1", "k", "key":
		return "key"
	case "2", "p", "pass", "password":
		return "password"
	default:
		return ""
	}
}

func summarizeAuth(auth store.AuthConfig) string {
	switch normalizeAuthType(auth.Type) {
	case "key":
		if strings.TrimSpace(auth.KeyPath) != "" {
			return "key:" + strings.TrimSpace(auth.KeyPath)
		}
		return "key"
	case "password":
		return "password"
	default:
		return "none"
	}
}
