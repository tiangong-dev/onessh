package cli

import (
	"bytes"
	"testing"

	"onessh/internal/domain"
)

// TestRenderHostDetailsYAMLGolden locks the byte-exact output of
// `onessh host show -o yaml` so any future refactor of the underlying
// PlainConfig/HostConfig/UserConfig/AuthConfig types (yaml tags, field
// order) is caught immediately. The fixture intentionally exercises
// every optional field on HostConfig and an Auth key-path entry on
// UserConfig.
func TestRenderHostDetailsYAMLGolden(t *testing.T) {
	cfg := domain.PlainConfig{
		Users: map[string]domain.UserConfig{
			"ops": {
				Name: "ubuntu",
				Auth: domain.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
			},
		},
		Hosts: map[string]domain.HostConfig{
			"web1": {
				Host:        "web1.example.com",
				Description: "primary web",
				UserRef:     "ops",
				Port:        22,
				ProxyJump:   "bastion",
				Tags:        []string{"prod", "web"},
				Env:         map[string]string{"FOO": "bar"},
				PreConnect:  []string{"echo hi"},
				PostConnect: []string{"echo bye"},
			},
		},
	}

	var buf bytes.Buffer
	if err := renderHostDetailsYAML(&buf, cfg); err != nil {
		t.Fatalf("renderHostDetailsYAML: %v", err)
	}

	want := `users:
    ops:
        name: ubuntu
        auth:
            type: key
            key_path: ~/.ssh/id_ed25519
hosts:
    web1:
        host: web1.example.com
        description: primary web
        user_ref: ops
        port: 22
        proxy_jump: bastion
        tags:
            - prod
            - web
        env:
            FOO: bar
        pre_connect:
            - echo hi
        post_connect:
            - echo bye
`
	if got := buf.String(); got != want {
		t.Fatalf("yaml output drift\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}
