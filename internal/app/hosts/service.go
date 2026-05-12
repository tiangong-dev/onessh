package hosts

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/domain"
	"onessh/internal/store"
)

type Service struct{}

type AddInput struct {
	Config store.PlainConfig
	Alias  string
	Host   store.HostConfig
}

type UpdateInput struct {
	Config store.PlainConfig
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

	Tags        []string
	TagsChanged bool

	Untag        []string
	UntagChanged bool

	ClearTags bool
}

type RemoveInput struct {
	Config store.PlainConfig
	Alias  string
}

type Output struct {
	Config store.PlainConfig
	Alias  string
	Host   store.HostConfig
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

func normalizeHostConfig(cfg store.PlainConfig, host store.HostConfig) (store.HostConfig, error) {
	host.Host = strings.TrimSpace(host.Host)
	if host.Host == "" {
		return store.HostConfig{}, errors.New("host cannot be empty")
	}

	host.UserRef = domain.NormalizeUserAlias(host.UserRef)
	if host.UserRef == "" {
		return store.HostConfig{}, errors.New("user_ref cannot be empty")
	}
	if _, exists := cfg.Users[host.UserRef]; !exists {
		return store.HostConfig{}, fmt.Errorf("user profile %q not found", host.UserRef)
	}

	if host.Port < 0 || host.Port > 65535 {
		return store.HostConfig{}, errors.New("port must be between 1 and 65535")
	}
	host.Port = domain.EffectivePort(host.Port)

	host.ProxyJump = strings.TrimSpace(host.ProxyJump)
	host.Description = strings.TrimSpace(host.Description)
	host.Tags = domain.NormalizeTags(host.Tags)

	env, err := normalizeEnvMap(host.Env)
	if err != nil {
		return store.HostConfig{}, err
	}
	host.Env = env
	host.PreConnect = cloneStringSlice(host.PreConnect)
	host.PostConnect = cloneStringSlice(host.PostConnect)
	return host, nil
}

func applyEnvUpdate(host *store.HostConfig, input UpdateInput) error {
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

func applyTagUpdate(host *store.HostConfig, input UpdateInput) {
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

func normalizeHostAlias(input string) string {
	return strings.TrimSpace(input)
}

func cloneConfig(cfg store.PlainConfig) store.PlainConfig {
	return store.PlainConfig{
		Users: cloneUsers(cfg.Users),
		Hosts: cloneHosts(cfg.Hosts),
	}
}

func cloneUsers(users map[string]store.UserConfig) map[string]store.UserConfig {
	cloned := make(map[string]store.UserConfig, len(users))
	for alias, user := range users {
		cloned[alias] = user
	}
	return cloned
}

func cloneHosts(hosts map[string]store.HostConfig) map[string]store.HostConfig {
	cloned := make(map[string]store.HostConfig, len(hosts))
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
