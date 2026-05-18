package cli

import (
	"strings"

	"onessh/internal/domain"
)

func normalizeAuthType(input string) string {
	return string(domain.NormalizeAuthType(input))
}

func summarizeAuth(auth domain.AuthConfig) string {
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
