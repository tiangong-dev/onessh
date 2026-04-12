package cli

import (
	"os"
	"sort"
	"strings"
)

func appendSendEnvOptions(args []string, envMap map[string]string) []string {
	if len(envMap) == 0 {
		return args
	}
	keys := sortedStringMapKeys(envMap)
	for _, key := range keys {
		args = append(args, "-o", "SendEnv="+key)
	}
	return args
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	merged := map[string]string{}
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	for key, value := range overrides {
		merged[key] = value
	}

	keys := sortedStringMapKeys(merged)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+merged[key])
	}
	return result
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func sanitizeHookCommands(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func expandTilde(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return homeDir + "/" + strings.TrimPrefix(input, "~/"), nil
	}
	return input, nil
}

func wipe(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
