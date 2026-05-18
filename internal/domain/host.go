package domain

type HostConfig struct {
	Host        string            `yaml:"host"`
	Description string            `yaml:"description,omitempty"`
	UserRef     string            `yaml:"user_ref"`
	Port        int               `yaml:"port"`
	ProxyJump   string            `yaml:"proxy_jump,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	PreConnect  []string          `yaml:"pre_connect,omitempty"`
	PostConnect []string          `yaml:"post_connect,omitempty"`
}
