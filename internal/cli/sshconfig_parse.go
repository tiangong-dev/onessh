package cli

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func parseSSHConfigFile(path string) ([]sshConfigEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read ssh config %s: %w", path, err)
	}
	return parseSSHConfig(string(raw)), nil
}

func parseSSHConfig(content string) []sshConfigEntry {
	type block struct {
		aliases      []string
		hostName     string
		user         string
		port         int
		proxyJump    string
		identityFile string
		env          map[string]string
	}

	var current *block
	entriesByAlias := map[string]sshConfigEntry{}

	flush := func() {
		if current == nil || len(current.aliases) == 0 {
			return
		}
		for _, alias := range current.aliases {
			entry := sshConfigEntry{
				Alias:        alias,
				HostName:     current.hostName,
				User:         current.user,
				Port:         current.port,
				ProxyJump:    current.proxyJump,
				IdentityFile: current.identityFile,
				Env:          cloneStringMap(current.env),
			}
			if strings.TrimSpace(entry.HostName) == "" {
				entry.HostName = alias
			}
			if entry.Port <= 0 {
				entry.Port = 22
			}
			entriesByAlias[alias] = entry
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := splitSSHConfigLine(line)
		if !ok {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			flush()
			aliases := parseHostAliases(value)
			current = &block{
				aliases: aliases,
				port:    22,
				env:     map[string]string{},
			}
		default:
			if current == nil {
				continue
			}
			switch strings.ToLower(key) {
			case "hostname":
				current.hostName = trimSSHValue(value)
			case "user":
				current.user = trimSSHValue(value)
			case "port":
				if p, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && p > 0 {
					current.port = p
				}
			case "proxyjump":
				current.proxyJump = trimSSHValue(value)
			case "identityfile":
				current.identityFile = trimSSHValue(value)
			case "setenv":
				for _, token := range strings.Fields(value) {
					parts := strings.SplitN(token, "=", 2)
					if len(parts) != 2 {
						continue
					}
					key := strings.TrimSpace(parts[0])
					if !envKeyPattern.MatchString(key) {
						continue
					}
					current.env[key] = trimSSHValue(parts[1])
				}
			}
		}
	}
	flush()

	aliases := make([]string, 0, len(entriesByAlias))
	for alias := range entriesByAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	entries := make([]sshConfigEntry, 0, len(aliases))
	for _, alias := range aliases {
		entries = append(entries, entriesByAlias[alias])
	}
	return entries
}

func splitSSHConfigLine(line string) (string, string, bool) {
	if line == "" {
		return "", "", false
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", false
	}
	key := parts[0]
	value := strings.TrimSpace(line[len(key):])
	return key, value, true
}

func parseHostAliases(raw string) []string {
	fields := strings.Fields(raw)
	aliases := make([]string, 0, len(fields))
	for _, field := range fields {
		alias := trimSSHValue(field)
		if alias == "" || strings.ContainsAny(alias, "*?!") {
			continue
		}
		aliases = append(aliases, alias)
	}
	return aliases
}

func trimSSHValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"`)
	trimmed = strings.Trim(trimmed, `'`)
	return trimmed
}

func sshConfigValueToken(value string) string {
	if value == "" {
		return "\"\""
	}
	if strings.ContainsAny(value, " \t\"#") {
		escaped := strings.ReplaceAll(value, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return "\"" + escaped + "\""
	}
	return value
}
