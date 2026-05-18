package ssh

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestProxyJumpStrategyPassesThroughRawProxyJump(t *testing.T) {
	t.Parallel()

	strategy := ProxyJumpStrategy{
		ResolveOnesshPath: func() (string, error) {
			t.Fatal("ResolveOnesshPath should not be called for raw proxy jump")
			return "", nil
		},
	}

	got, err := strategy.BuildArgs(domain.NewPlainConfig(), "jump@example.com:2200")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	want := []string{"-J", "jump@example.com:2200"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected proxy args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestProxyJumpStrategyBuildsKeyAliasProxyJump(t *testing.T) {
	t.Parallel()

	cfg := domain.NewPlainConfig()
	cfg.Users["jump-user"] = domain.UserConfig{
		Name: "jump",
		Auth: domain.AuthConfig{Type: "key"},
	}
	cfg.Hosts["bastion"] = domain.HostConfig{
		Host:    "bastion.internal",
		UserRef: "jump-user",
	}

	strategy := ProxyJumpStrategy{
		ResolveOnesshPath: func() (string, error) {
			t.Fatal("ResolveOnesshPath should not be called for key proxy jump")
			return "", nil
		},
	}

	got, err := strategy.BuildArgs(cfg, "bastion")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	want := []string{"-J", "jump@bastion.internal:22"}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected proxy args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestProxyJumpStrategyBuildsPasswordAliasProxyCommand(t *testing.T) {
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

	strategy := ProxyJumpStrategy{
		ResolveOnesshPath: func() (string, error) {
			return "/tmp/one ssh/onessh'bin", nil
		},
	}

	got, err := strategy.BuildArgs(cfg, "bastion")
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}

	want := []string{"-o", `ProxyCommand='/tmp/one ssh/onessh'"'"'bin' -q 'bastion' -- -W '%h:%p'`}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected proxy args:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestProxyJumpStrategyWrapsPasswordAliasResolverError(t *testing.T) {
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

	strategy := ProxyJumpStrategy{
		ResolveOnesshPath: func() (string, error) {
			return "", errors.New("boom")
		},
	}

	_, err := strategy.BuildArgs(cfg, "bastion")
	if err == nil || !strings.Contains(err.Error(), "resolve onessh path for proxy: boom") {
		t.Fatalf("expected wrapped resolver error, got %v", err)
	}
}
