package domain

import "strings"

type AuthConfig struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type AuthType string

const (
	AuthTypeKey      AuthType = "key"
	AuthTypePassword AuthType = "password"
)

func NormalizeAuthType(input string) AuthType {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "1", "k", string(AuthTypeKey):
		return AuthTypeKey
	case "2", "p", "pass", string(AuthTypePassword):
		return AuthTypePassword
	default:
		return ""
	}
}

func NormalizeStoredAuthType(input string) AuthType {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case string(AuthTypeKey):
		return AuthTypeKey
	case string(AuthTypePassword):
		return AuthTypePassword
	default:
		return ""
	}
}
