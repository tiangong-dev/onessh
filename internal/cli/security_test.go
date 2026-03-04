package cli

import (
	"io"
	"testing"

	"onessh/internal/store"
)

func TestRedactConfigForDump(t *testing.T) {
	t.Parallel()

	cfg := store.NewPlainConfig()
	cfg.Users["ops"] = store.UserConfig{
		Name: "ubuntu",
		Auth: store.AuthConfig{
			Type:     "password",
			Password: "secret-pass",
		},
	}
	cfg.Hosts["web1"] = store.HostConfig{
		Host:    "1.2.3.4",
		UserRef: "ops",
		Port:    22,
		Env: map[string]string{
			"AWS_PROFILE": "prod",
			"TOKEN":       "sensitive",
		},
	}

	redacted := redactConfigForDump(cfg)

	if got := redacted.Users["ops"].Auth.Password; got != redactedSecretValue {
		t.Fatalf("expected redacted password, got %q", got)
	}
	if got := redacted.Hosts["web1"].Env["TOKEN"]; got != redactedSecretValue {
		t.Fatalf("expected redacted env token, got %q", got)
	}

	if cfg.Users["ops"].Auth.Password != "secret-pass" {
		t.Fatalf("source config should remain unchanged")
	}
}

func TestNewPasswordFD(t *testing.T) {
	t.Parallel()

	fd, cleanup, err := newPasswordFD("hello-pass")
	if err != nil {
		t.Fatalf("newPasswordFD: %v", err)
	}
	defer cleanup()

	raw, err := io.ReadAll(fd)
	if err != nil {
		t.Fatalf("read password fd: %v", err)
	}
	if string(raw) != "hello-pass\n" {
		t.Fatalf("unexpected password payload: %q", string(raw))
	}
}
