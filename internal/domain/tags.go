package domain

import (
	"sort"
	"strings"
)

func NormalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func HostHasTag(tags []string, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, stored := range tags {
		if stored == tag {
			return true
		}
	}
	return false
}
