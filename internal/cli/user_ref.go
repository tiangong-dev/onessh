package cli

import (
	"sort"
	"strings"

	"onessh/internal/store"
)

func sortedUserAliases(users map[string]store.UserConfig) []string {
	aliases := make([]string, 0, len(users))
	for alias := range users {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func findUserAliasByName(users map[string]store.UserConfig, name string) string {
	normalizedName := strings.TrimSpace(name)
	for alias, cfg := range users {
		if strings.EqualFold(strings.TrimSpace(cfg.Name), normalizedName) {
			return alias
		}
	}
	return ""
}

func normalizeUserAlias(input string) string {
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
