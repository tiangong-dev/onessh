package ssh

import (
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestBuildSSHArgsUsesDefaultPort(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", domain.AuthConfig{Type: "key"}, []string{"-T"}, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-T", "-o", "StrictHostKeyChecking=accept-new", "ubuntu@192.0.2.10"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSCPArgsUsesDefaultPort(t *testing.T) {
	t.Parallel()

	got, err := BuildSCPArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", domain.AuthConfig{Type: "key"}, "/var/log/app.log", []string{"./app.log"}, false, false, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSCPArgs: %v", err)
	}

	want := []string{"-P", "22", "-o", "StrictHostKeyChecking=accept-new", "ubuntu@192.0.2.10:/var/log/app.log", "./app.log"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected scp args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsIncludesKeyAuthIdentity(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "example.com",
		Port: 2222,
	}, "deploy", domain.AuthConfig{Type: "key", KeyPath: "/keys/deploy"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "2222", "-i", "/keys/deploy", "-o", "StrictHostKeyChecking=accept-new", "deploy@example.com"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsPasswordAuthDoesNotIncludeKeyArg(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "example.com",
	}, "deploy", domain.AuthConfig{Type: "password", KeyPath: "/keys/ignored"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	if containsArg(got, "-i") || containsArg(got, "/keys/ignored") {
		t.Fatalf("password auth should not include identity args: %#v", got)
	}
	want := []string{"-p", "22", "-o", "StrictHostKeyChecking=accept-new", "deploy@example.com"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsRawProxyJump(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host:      "app.internal",
		ProxyJump: "jump@bastion.example:2200",
	}, "deploy", domain.AuthConfig{Type: "key"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-J", "jump@bastion.example:2200", "-o", "StrictHostKeyChecking=accept-new", "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsAliasKeyProxyJump(t *testing.T) {
	t.Parallel()

	cfg := domain.NewPlainConfig()
	cfg.Users["jump-user"] = domain.UserConfig{
		Name: "jump",
		Auth: domain.AuthConfig{Type: "key"},
	}
	cfg.Hosts["bastion"] = domain.HostConfig{
		Host:    "bastion.internal",
		UserRef: "jump-user",
		Port:    2200,
	}

	got, err := BuildSSHArgs(cfg, domain.HostConfig{
		Host:      "app.internal",
		ProxyJump: "bastion",
	}, "deploy", domain.AuthConfig{Type: "key"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-J", "jump@bastion.internal:2200", "-o", "StrictHostKeyChecking=accept-new", "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsAliasPasswordProxyJumpUsesOnesshProxyCommand(t *testing.T) {
	t.Parallel()

	cfg := domain.NewPlainConfig()
	cfg.Users["jump-user"] = domain.UserConfig{
		Name: "jump",
		Auth: domain.AuthConfig{Type: "password"},
	}
	cfg.Hosts["bastion"] = domain.HostConfig{
		Host:    "bastion.internal",
		UserRef: "jump-user",
	}

	got, err := BuildSSHArgs(cfg, domain.HostConfig{
		Host:      "app.internal",
		ProxyJump: "bastion",
	}, "deploy", domain.AuthConfig{Type: "key"}, nil, ArgsOptions{OnesshPath: "/tmp/one ssh/onessh'bin"})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	wantProxy := `ProxyCommand='/tmp/one ssh/onessh'"'"'bin' -q 'bastion' -- -W '%h:%p'`
	want := []string{"-p", "22", "-o", wantProxy, "-o", "StrictHostKeyChecking=accept-new", "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsRejectsUnsupportedAuth(t *testing.T) {
	t.Parallel()

	_, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "example.com",
	}, "deploy", domain.AuthConfig{Type: "token"}, nil, ArgsOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported auth type") {
		t.Fatalf("expected unsupported auth error, got %v", err)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
