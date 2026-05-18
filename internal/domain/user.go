package domain

import "strings"

func NormalizeUserAlias(input string) string {
	alias := strings.ToLower(strings.TrimSpace(input))
	if alias == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_")
}
