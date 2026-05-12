package ssh

import (
	"reflect"
	"strings"
	"testing"

	"onessh/internal/store"
)

func TestBuildSSHArgsUsesDefaultPort(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(store.NewPlainConfig(), store.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", store.AuthConfig{Type: "key"}, []string{"-T"}, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-T", "ubuntu@192.0.2.10"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSCPArgsUsesDefaultPort(t *testing.T) {
	t.Parallel()

	got, err := BuildSCPArgs(store.NewPlainConfig(), store.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", store.AuthConfig{Type: "key"}, "/var/log/app.log", []string{"./app.log"}, false, false, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSCPArgs: %v", err)
	}

	want := []string{"-P", "22", "ubuntu@192.0.2.10:/var/log/app.log", "./app.log"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected scp args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsIncludesKeyAuthIdentity(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(store.NewPlainConfig(), store.HostConfig{
		Host: "example.com",
		Port: 2222,
	}, "deploy", store.AuthConfig{Type: "key", KeyPath: "/keys/deploy"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "2222", "-i", "/keys/deploy", "deploy@example.com"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsPasswordAuthDoesNotIncludeKeyArg(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(store.NewPlainConfig(), store.HostConfig{
		Host: "example.com",
	}, "deploy", store.AuthConfig{Type: "password", KeyPath: "/keys/ignored"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	if containsArg(got, "-i") || containsArg(got, "/keys/ignored") {
		t.Fatalf("password auth should not include identity args: %#v", got)
	}
	want := []string{"-p", "22", "deploy@example.com"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsRawProxyJump(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(store.NewPlainConfig(), store.HostConfig{
		Host:      "app.internal",
		ProxyJump: "jump@bastion.example:2200",
	}, "deploy", store.AuthConfig{Type: "key"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-J", "jump@bastion.example:2200", "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsAliasKeyProxyJump(t *testing.T) {
	t.Parallel()

	cfg := store.NewPlainConfig()
	cfg.Users["jump-user"] = store.UserConfig{
		Name: "jump",
		Auth: store.AuthConfig{Type: "key"},
	}
	cfg.Hosts["bastion"] = store.HostConfig{
		Host:    "bastion.internal",
		UserRef: "jump-user",
		Port:    2200,
	}

	got, err := BuildSSHArgs(cfg, store.HostConfig{
		Host:      "app.internal",
		ProxyJump: "bastion",
	}, "deploy", store.AuthConfig{Type: "key"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	want := []string{"-p", "22", "-J", "jump@bastion.internal:2200", "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsAliasPasswordProxyJumpUsesOnesshProxyCommand(t *testing.T) {
	t.Parallel()

	cfg := store.NewPlainConfig()
	cfg.Users["jump-user"] = store.UserConfig{
		Name: "jump",
		Auth: store.AuthConfig{Type: "password"},
	}
	cfg.Hosts["bastion"] = store.HostConfig{
		Host:    "bastion.internal",
		UserRef: "jump-user",
	}

	got, err := BuildSSHArgs(cfg, store.HostConfig{
		Host:      "app.internal",
		ProxyJump: "bastion",
	}, "deploy", store.AuthConfig{Type: "key"}, nil, ArgsOptions{OnesshPath: "/tmp/one ssh/onessh'bin"})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	wantProxy := `ProxyCommand='/tmp/one ssh/onessh'"'"'bin' -q 'bastion' -- -W '%h:%p'`
	want := []string{"-p", "22", "-o", wantProxy, "deploy@app.internal"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected ssh args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestBuildSSHArgsRejectsUnsupportedAuth(t *testing.T) {
	t.Parallel()

	_, err := BuildSSHArgs(store.NewPlainConfig(), store.HostConfig{
		Host: "example.com",
	}, "deploy", store.AuthConfig{Type: "token"}, nil, ArgsOptions{})
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
