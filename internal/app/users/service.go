package users

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"onessh/internal/domain"
	"onessh/internal/store"
)

type Service struct{}

type AddInput struct {
	Config store.PlainConfig
	Alias  string
	Name   string
	Auth   AuthInput
}

type UpdateInput struct {
	Config      store.PlainConfig
	Alias       string
	Name        string
	NameChanged bool
	Auth        AuthUpdate
}

type RemoveInput struct {
	Config store.PlainConfig
	Alias  string
}

type AuthInput struct {
	Type     string
	KeyPath  string
	Password string
}

type AuthUpdate struct {
	Type        string
	KeyPath     string
	Password    string
	TypeChanged bool
	KeyPathSet  bool
	PasswordSet bool
}

type Output struct {
	Config store.PlainConfig
	Alias  string
	User   store.UserConfig
}

func (Service) Add(input AddInput) (Output, error) {
	alias := domain.NormalizeUserAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("user alias cannot be empty")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Output{}, errors.New("user name cannot be empty")
	}

	auth, err := authFromInput(input.Auth)
	if err != nil {
		return Output{}, err
	}

	cfg := cloneConfig(input.Config)
	if _, exists := cfg.Users[alias]; exists {
		return Output{}, fmt.Errorf("user profile %q already exists", alias)
	}

	user := store.UserConfig{Name: name, Auth: auth}
	cfg.Users[alias] = user
	return Output{Config: cfg, Alias: alias, User: user}, nil
}

func (Service) Update(input UpdateInput) (Output, error) {
	alias := domain.NormalizeUserAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("user alias cannot be empty")
	}

	cfg := cloneConfig(input.Config)
	user, exists := cfg.Users[alias]
	if !exists {
		return Output{}, fmt.Errorf("user profile %q does not exist", alias)
	}

	if input.NameChanged {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return Output{}, errors.New("user name cannot be empty")
		}
		user.Name = name
	}

	auth, err := authFromUpdate(user.Auth, input.Auth)
	if err != nil {
		return Output{}, err
	}
	user.Auth = auth

	cfg.Users[alias] = user
	return Output{Config: cfg, Alias: alias, User: user}, nil
}

func (Service) Remove(input RemoveInput) (Output, error) {
	alias := domain.NormalizeUserAlias(input.Alias)
	if alias == "" {
		return Output{}, errors.New("user alias cannot be empty")
	}
	if _, exists := input.Config.Users[alias]; !exists {
		return Output{}, fmt.Errorf("user profile %q does not exist", alias)
	}

	inUseBy := hostAliasesUsingUser(input.Config, alias)
	if len(inUseBy) > 0 {
		return Output{}, fmt.Errorf("user profile %q is used by host(s): %s", alias, strings.Join(inUseBy, ", "))
	}

	cfg := cloneConfig(input.Config)
	delete(cfg.Users, alias)
	return Output{Config: cfg, Alias: alias}, nil
}

func authFromInput(input AuthInput) (store.AuthConfig, error) {
	authType := domain.NormalizeAuthType(input.Type)
	if authType == "" {
		return store.AuthConfig{}, errors.New("auth-type must be key or password")
	}

	keyPath := strings.TrimSpace(input.KeyPath)
	password := input.Password
	if keyPath != "" && strings.TrimSpace(password) != "" {
		return store.AuthConfig{}, errors.New("cannot set key_path and password at the same time")
	}

	switch authType {
	case domain.AuthTypeKey:
		if keyPath == "" {
			return store.AuthConfig{}, errors.New("key auth requires key_path")
		}
		return store.AuthConfig{Type: string(domain.AuthTypeKey), KeyPath: keyPath}, nil
	case domain.AuthTypePassword:
		if strings.TrimSpace(password) == "" {
			return store.AuthConfig{}, errors.New("password auth requires password")
		}
		return store.AuthConfig{Type: string(domain.AuthTypePassword), Password: password}, nil
	default:
		return store.AuthConfig{}, errors.New("auth-type must be key or password")
	}
}

func authFromUpdate(current store.AuthConfig, input AuthUpdate) (store.AuthConfig, error) {
	if input.KeyPathSet && input.PasswordSet {
		return store.AuthConfig{}, errors.New("cannot set key_path and password at the same time")
	}

	if input.TypeChanged {
		authType := domain.NormalizeAuthType(input.Type)
		if authType == "" {
			return store.AuthConfig{}, errors.New("auth-type must be key or password")
		}
		switch authType {
		case domain.AuthTypeKey:
			path := strings.TrimSpace(input.KeyPath)
			if !input.KeyPathSet {
				path = strings.TrimSpace(current.KeyPath)
			}
			if path == "" {
				return store.AuthConfig{}, errors.New("key auth requires key_path")
			}
			return store.AuthConfig{Type: string(domain.AuthTypeKey), KeyPath: path}, nil
		case domain.AuthTypePassword:
			password := input.Password
			if !input.PasswordSet && domain.NormalizeStoredAuthType(current.Type) == domain.AuthTypePassword {
				password = current.Password
			}
			if strings.TrimSpace(password) == "" {
				return store.AuthConfig{}, errors.New("password auth requires password")
			}
			return store.AuthConfig{Type: string(domain.AuthTypePassword), Password: password}, nil
		}
	}

	if input.KeyPathSet {
		path := strings.TrimSpace(input.KeyPath)
		if path == "" {
			return store.AuthConfig{}, errors.New("key_path cannot be empty")
		}
		return store.AuthConfig{Type: string(domain.AuthTypeKey), KeyPath: path}, nil
	}

	if input.PasswordSet {
		if strings.TrimSpace(input.Password) == "" {
			return store.AuthConfig{}, errors.New("password cannot be empty")
		}
		return store.AuthConfig{Type: string(domain.AuthTypePassword), Password: input.Password}, nil
	}

	if normalized := domain.NormalizeStoredAuthType(current.Type); normalized != "" {
		current.Type = string(normalized)
	}
	return current, nil
}

func hostAliasesUsingUser(cfg store.PlainConfig, userRef string) []string {
	aliases := make([]string, 0)
	for alias, host := range cfg.Hosts {
		if strings.TrimSpace(host.UserRef) == userRef {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
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
