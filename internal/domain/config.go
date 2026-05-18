package domain

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
