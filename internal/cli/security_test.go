package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestRenderHostDetailsTableRedactsEnv(t *testing.T) {
	t.Parallel()

	cfg := domain.NewPlainConfig()
	host := domain.HostConfig{
		Host: "1.2.3.4",
		Port: 22,
		Env: map[string]string{
			"TOKEN":       "sensitive",
			"AWS_PROFILE": "prod",
		},
	}

	var buf bytes.Buffer
	renderHostDetailsTable(&buf, "web1", host, cfg)

	out := buf.String()
	if strings.Contains(out, "sensitive") || strings.Contains(out, "prod") {
		t.Fatalf("table output leaked env values: %q", out)
	}
	if !strings.Contains(out, "TOKEN="+redactedSecretValue) {
		t.Fatalf("expected redacted TOKEN, got: %q", out)
	}
	if !strings.Contains(out, "AWS_PROFILE="+redactedSecretValue) {
		t.Fatalf("expected redacted AWS_PROFILE, got: %q", out)
	}
}

func TestRedactConfigForDump(t *testing.T) {
	t.Parallel()

	cfg := domain.NewPlainConfig()
	cfg.Users["ops"] = domain.UserConfig{
		Name: "ubuntu",
		Auth: domain.AuthConfig{
			Type:     "password",
			Password: "secret-pass",
		},
	}
	cfg.Hosts["web1"] = domain.HostConfig{
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

func TestBuildOnesshProxyCommandShellQuotesDynamicValues(t *testing.T) {
	t.Parallel()

	got := buildOnesshProxyCommand("/tmp/one ssh/onessh'bin", "jump'; touch /tmp/pwn; echo '")
	want := `'/tmp/one ssh/onessh'"'"'bin' -q 'jump'"'"'; touch /tmp/pwn; echo '"'"'' -- -W '%h:%p'`
	if got != want {
		t.Fatalf("unexpected proxy command:\nwant: %s\n got: %s", want, got)
	}
}

func TestRunExternalCommandRejectsUnsupportedBinary(t *testing.T) {
	t.Parallel()

	err := runExternalCommand(context.Background(), "sh", []string{"-c", "true"}, nil, nil, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unsupported external command") {
		t.Fatalf("expected unsupported command error, got %v", err)
	}
}
