package store

type AuthConfig struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type HostConfig struct {
	Host      string            `yaml:"host"`
	UserRef   string            `yaml:"user_ref,omitempty"`
	User      string            `yaml:"user,omitempty"`
	Port      int               `yaml:"port"`
	Auth      AuthConfig        `yaml:"auth"`
	ProxyJump string            `yaml:"proxy_jump,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

type UserConfig struct {
	Name string `yaml:"name"`
}

type PlainConfig struct {
	Users map[string]UserConfig `yaml:"users,omitempty"`
	Hosts map[string]HostConfig `yaml:"hosts"`
}

func NewPlainConfig() PlainConfig {
	return PlainConfig{
		Users: map[string]UserConfig{},
		Hosts: map[string]HostConfig{},
	}
}
