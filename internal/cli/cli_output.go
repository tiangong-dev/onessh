package cli

import (
	"fmt"
	"strings"
)

func validateOutputFormat(value string, allowed ...string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(allowed) == 0 {
		return normalized, nil
	}
	for _, candidate := range allowed {
		if normalized == strings.ToLower(strings.TrimSpace(candidate)) {
			return normalized, nil
		}
	}
	return "", fmt.Errorf("unsupported output format %q (allowed: %s)", value, strings.Join(allowed, "|"))
}
