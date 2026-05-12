package domain

import (
	"path/filepath"
	"sort"
)

type HostQuery struct {
	Address     string
	Description string
	Tags        []string
}

func MatchHostFilter(alias string, host HostQuery, pattern string) bool {
	if matched, _ := filepath.Match(pattern, alias); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, host.Address); matched {
		return true
	}
	if host.Description != "" {
		if matched, _ := filepath.Match(pattern, host.Description); matched {
			return true
		}
	}
	return false
}

func CollectFilteredHosts(hosts map[string]HostQuery, tag, filter string) []string {
	aliases := make([]string, 0, len(hosts))
	for alias, host := range hosts {
		if tag != "" && !HostHasTag(host.Tags, tag) {
			continue
		}
		if filter != "" && !MatchHostFilter(alias, host, filter) {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
