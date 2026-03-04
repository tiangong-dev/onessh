package store

type AuthConfig struct {
	Type     string `yaml:"type"`
	KeyPath  string `yaml:"key_path,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type HostConfig struct {
	Host      string            `yaml:"host"`
	UserRef   string            `yaml:"user_ref"`
	Port      int               `yaml:"port"`
	ProxyJump string            `yaml:"proxy_jump,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	PreConnect  []string        `yaml:"pre_connect,omitempty"`
	PostConnect []string        `yaml:"post_connect,omitempty"`
}

type UserConfig struct {
	Name string     `yaml:"name"`
	Auth AuthConfig `yaml:"auth,omitempty"`
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
