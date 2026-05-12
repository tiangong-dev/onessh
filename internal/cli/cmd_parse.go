package cli

import (
	"fmt"
	"strings"

	"onessh/internal/domain"
)

func parseEnvAssignments(values []string) (map[string]string, error) {
	return domain.ParseEnvAssignments(values)
}

func parseEnvKeys(values []string) ([]string, error) {
	return domain.ParseEnvKeys(values)
}

func parseHookCommands(values []string, flagName string) ([]string, error) {
	commands := make([]string, 0, len(values))
	for i, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%s command at index %d is empty", flagName, i)
		}
		commands = append(commands, trimmed)
	}
	if len(commands) == 0 {
		return nil, nil
	}
	return commands, nil
}
