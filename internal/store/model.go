package store

type AuthConfig struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type HostConfig struct {
	Host      string            `yaml:"host"`
	User      string            `yaml:"user"`
	Port      int               `yaml:"port"`
	Auth      AuthConfig        `yaml:"auth"`
	ProxyJump string            `yaml:"proxy_jump,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

type PlainConfig struct {
	Hosts map[string]HostConfig `yaml:"hosts"`
}

func NewPlainConfig() PlainConfig {
	return PlainConfig{
		Hosts: map[string]HostConfig{},
	}
}
