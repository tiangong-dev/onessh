package hosts

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/domain"
)

type Service struct{}

type AddInput struct {
	Config domain.PlainConfig
	Alias  string
	Host   domain.HostConfig
}

type UpdateInput struct {
	Config domain.PlainConfig
	Alias  string

	TargetAlias        string
	TargetAliasChanged bool

	Host        string
	HostChanged bool

	Port        int
	PortChanged bool

	ProxyJump        string
	ProxyJumpChanged bool

	Description        string
	DescriptionChanged bool

	UserRef        string
	UserRefChanged bool

	Env        []string
	EnvChanged bool

	UnsetEnv        []string
	UnsetEnvChanged bool

	ClearEnv bool

	PreConnect        []string
	PreConnectChanged bool

	PostConnect        []string
	PostConnectChanged bool

	ClearPreConnect  bool
	ClearPostConnect bool

	Tags        []string
	TagsChanged bool

	Untag        []string
	UntagChanged bool

	ClearTags bool
}

type RemoveInput struct {
	Config domain.PlainConfig
	Alias  string
}

type Output struct {
	Config domain.PlainConfig
	Alias  string
	Host   domain.HostConfig
}

func (Service) Add(input AddInput) (Output, error) {
	alias := normalizeHostAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}

	cfg := cloneConfig(input.Config)
	if _, exists := cfg.Hosts[alias]; exists {
		return Output{}, fmt.Errorf("host %q already exists", alias)
	}

	host, err := normalizeHostConfig(cfg, input.Host)
	if err != nil {
		return Output{}, err
	}

	cfg.Hosts[alias] = host
	return Output{Config: cfg, Alias: alias, Host: host}, nil
}

func (Service) Update(input UpdateInput) (Output, error) {
	alias := normalizeHostAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}

	cfg := cloneConfig(input.Config)
	host, exists := cfg.Hosts[alias]
	if !exists {
		return Output{}, fmt.Errorf("host %q does not exist", alias)
	}

	targetAlias := alias
	if input.TargetAliasChanged {
		targetAlias = normalizeHostAlias(input.TargetAlias)
		if targetAlias == "" {
			return Output{}, errors.New("target host alias cannot be empty")
		}
		if targetAlias != alias {
			if _, conflict := cfg.Hosts[targetAlias]; conflict {
				return Output{}, fmt.Errorf("host %q already exists", targetAlias)
			}
		}
	}

	if input.HostChanged {
		host.Host = strings.TrimSpace(input.Host)
	}
	if input.PortChanged {
		host.Port = input.Port
	}
	if input.ProxyJumpChanged {
		host.ProxyJump = strings.TrimSpace(input.ProxyJump)
	}
	if input.DescriptionChanged {
		host.Description = strings.TrimSpace(input.Description)
	}
	if input.UserRefChanged {
		host.UserRef = domain.NormalizeUserAlias(input.UserRef)
	}
	if err := applyEnvUpdate(&host, input); err != nil {
		return Output{}, err
	}
	if err := applyHookUpdate(&host, input); err != nil {
		return Output{}, err
	}
	applyTagUpdate(&host, input)

	normalizedHost, err := normalizeHostConfig(cfg, host)
	if err != nil {
		return Output{}, err
	}

	if targetAlias != alias {
		delete(cfg.Hosts, alias)
	}
	cfg.Hosts[targetAlias] = normalizedHost
	return Output{Config: cfg, Alias: targetAlias, Host: normalizedHost}, nil
}

func (Service) Remove(input RemoveInput) (Output, error) {
	alias := normalizeHostAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}
	if _, exists := input.Config.Hosts[alias]; !exists {
		return Output{}, fmt.Errorf("host %q does not exist", alias)
	}

	cfg := cloneConfig(input.Config)
	host := cfg.Hosts[alias]
	delete(cfg.Hosts, alias)
	return Output{Config: cfg, Alias: alias, Host: host}, nil
}

func normalizeHostConfig(cfg domain.PlainConfig, host domain.HostConfig) (domain.HostConfig, error) {
	host.Host = strings.TrimSpace(host.Host)
	if host.Host == "" {
		return domain.HostConfig{}, errors.New("host cannot be empty")
	}

	host.UserRef = domain.NormalizeUserAlias(host.UserRef)
	if host.UserRef == "" {
		return domain.HostConfig{}, errors.New("user_ref cannot be empty")
	}
	if _, exists := cfg.Users[host.UserRef]; !exists {
		return domain.HostConfig{}, fmt.Errorf("user profile %q not found", host.UserRef)
	}

	if host.Port < 0 || host.Port > 65535 {
		return domain.HostConfig{}, errors.New("port must be between 1 and 65535")
	}
	host.Port = domain.EffectivePort(host.Port)

	host.ProxyJump = strings.TrimSpace(host.ProxyJump)
	host.Description = strings.TrimSpace(host.Description)
	host.Tags = domain.NormalizeTags(host.Tags)

	env, err := normalizeEnvMap(host.Env)
	if err != nil {
		return domain.HostConfig{}, err
	}
	host.Env = env
	host.PreConnect = cloneStringSlice(host.PreConnect)
	host.PostConnect = cloneStringSlice(host.PostConnect)
	return host, nil
}

func applyEnvUpdate(host *domain.HostConfig, input UpdateInput) error {
	if !input.EnvChanged && !input.UnsetEnvChanged && !input.ClearEnv {
		return nil
	}

	current := cloneStringMap(host.Env)
	if input.ClearEnv {
		current = map[string]string{}
	}
	if input.UnsetEnvChanged {
		keys, err := domain.ParseEnvKeys(input.UnsetEnv)
		if err != nil {
			return err
		}
		for _, key := range keys {
			delete(current, key)
		}
	}
	if input.EnvChanged {
		entries, err := domain.ParseEnvAssignments(input.Env)
		if err != nil {
			return err
		}
		for key, value := range entries {
			current[key] = value
		}
	}
	if len(current) == 0 {
		host.Env = nil
		return nil
	}
	host.Env = current
	return nil
}

func applyHookUpdate(host *domain.HostConfig, input UpdateInput) error {
	if !input.PreConnectChanged && !input.PostConnectChanged && !input.ClearPreConnect && !input.ClearPostConnect {
		return nil
	}

	preparedPre := cloneStringSlice(host.PreConnect)
	preparedPost := cloneStringSlice(host.PostConnect)

	if input.ClearPreConnect {
		preparedPre = nil
	}
	if input.ClearPostConnect {
		preparedPost = nil
	}
	if input.PreConnectChanged {
		commands, err := normalizeHookCommands(input.PreConnect, "pre-connect")
		if err != nil {
			return err
		}
		preparedPre = commands
	}
	if input.PostConnectChanged {
		commands, err := normalizeHookCommands(input.PostConnect, "post-connect")
		if err != nil {
			return err
		}
		preparedPost = commands
	}

	host.PreConnect = preparedPre
	host.PostConnect = preparedPost
	return nil
}

func applyTagUpdate(host *domain.HostConfig, input UpdateInput) {
	if !input.TagsChanged && !input.UntagChanged && !input.ClearTags {
		return
	}

	current := domain.NormalizeTags(host.Tags)
	if input.ClearTags {
		current = nil
	}
	if input.UntagChanged {
		remove := make(map[string]struct{}, len(input.Untag))
		for _, tag := range domain.NormalizeTags(input.Untag) {
			remove[tag] = struct{}{}
		}

		filtered := make([]string, 0, len(current))
		for _, tag := range current {
			if _, skip := remove[tag]; !skip {
				filtered = append(filtered, tag)
			}
		}
		current = filtered
	}
	if input.TagsChanged {
		current = append(current, input.Tags...)
	}
	host.Tags = domain.NormalizeTags(current)
}

func normalizeEnvMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if err := domain.ValidateEnvKey(trimmedKey); err != nil {
			return nil, err
		}
		normalized[trimmedKey] = value
	}
	return normalized, nil
}

func normalizeHookCommands(values []string, flagName string) ([]string, error) {
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

func normalizeHostAlias(input string) string {
	return strings.TrimSpace(input)
}

func cloneConfig(cfg domain.PlainConfig) domain.PlainConfig {
	return domain.PlainConfig{
		Users: cloneUsers(cfg.Users),
		Hosts: cloneHosts(cfg.Hosts),
	}
}

func cloneUsers(users map[string]domain.UserConfig) map[string]domain.UserConfig {
	cloned := make(map[string]domain.UserConfig, len(users))
	for alias, user := range users {
		cloned[alias] = user
	}
	return cloned
}

func cloneHosts(hosts map[string]domain.HostConfig) map[string]domain.HostConfig {
	cloned := make(map[string]domain.HostConfig, len(hosts))
	for alias, host := range hosts {
		host.Tags = cloneStringSlice(host.Tags)
		host.Env = cloneStringMap(host.Env)
		host.PreConnect = cloneStringSlice(host.PreConnect)
		host.PostConnect = cloneStringSlice(host.PostConnect)
		cloned[alias] = host
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string{}, values...)
}
