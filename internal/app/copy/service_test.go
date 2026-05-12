package copy

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	appruntime "onessh/internal/runtime"
	"onessh/internal/store"
)

func TestServiceCopyRunsCopy(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	resolver := &fakeResolver{
		userName: "alice",
		auth:     store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
	}
	runner := &fakeRunner{}
	service := Service{
		IdentityResolver: resolver,
		Runner:           runner,
	}

	out, err := service.Copy(context.Background(), Input{
		Config:     testConfig(),
		Alias:      "prod",
		RemotePath: "/tmp/app",
		LocalPaths: []string{"./app"},
		IsUpload:   true,
		Recursive:  true,
		Agent: AgentConfig{
			Socket:     "/tmp/onessh.sock",
			Capability: "capability",
		},
		IO: appruntime.IOStreams{
			Out:    &stdout,
			ErrOut: &stderr,
		},
	})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}

	if out.Alias != "prod" || out.Host != "prod.example.com" || out.UserName != "alice" || out.Port != 22 {
		t.Fatalf("unexpected output: %#v", out)
	}
	if resolver.called != 1 {
		t.Fatalf("resolver called %d times, want 1", resolver.called)
	}
	if !runner.called {
		t.Fatalf("runner was not called")
	}
	if runner.req.RemotePath != "/tmp/app" || !runner.req.IsUpload || !runner.req.Recursive {
		t.Fatalf("unexpected copy request: %#v", runner.req)
	}
	if !reflect.DeepEqual(runner.req.LocalPaths, []string{"./app"}) {
		t.Fatalf("unexpected local paths: %#v", runner.req.LocalPaths)
	}
	if runner.req.Stdout != &stdout || runner.req.Stderr != &stderr {
		t.Fatalf("stdout/stderr were not forwarded")
	}
	if runner.req.Agent.Socket != "/tmp/onessh.sock" || runner.req.Agent.Capability != "capability" {
		t.Fatalf("unexpected agent config: %#v", runner.req.Agent)
	}
}

func TestServiceCopyMissingHost(t *testing.T) {
	t.Parallel()

	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{},
	}

	_, err := service.Copy(context.Background(), Input{
		Config:     testConfig(),
		Alias:      "missing",
		RemotePath: "/tmp/app",
		LocalPaths: []string{"./app"},
		IsUpload:   true,
	})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestServiceCopyRejectsEmptyLocalPaths(t *testing.T) {
	t.Parallel()

	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{},
	}

	_, err := service.Copy(context.Background(), Input{
		Config:     testConfig(),
		Alias:      "prod",
		RemotePath: "/tmp/app",
		IsUpload:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "local paths cannot be empty") {
		t.Fatalf("expected empty local paths error, got %v", err)
	}
}

func TestServiceCopyResolverErrorStopsRunner(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("identity failed")
	runner := &fakeRunner{}
	service := Service{
		IdentityResolver: &fakeResolver{err: wantErr},
		Runner:           runner,
	}

	_, err := service.Copy(context.Background(), Input{
		Config:     testConfig(),
		Alias:      "prod",
		RemotePath: "/tmp/app",
		LocalPaths: []string{"./app"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Copy error = %v, want %v", err, wantErr)
	}
	if runner.called {
		t.Fatalf("runner should not run after resolver error")
	}
}

func TestServiceCopyRunnerErrorReturnsOutputForAudit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("scp failed")
	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{err: wantErr},
	}

	out, err := service.Copy(context.Background(), Input{
		Config:     testConfig(),
		Alias:      "prod",
		RemotePath: "/tmp/app",
		LocalPaths: []string{"./app"},
		IsUpload:   true,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Copy error = %v, want %v", err, wantErr)
	}
	if out.Alias != "prod" || out.Host != "prod.example.com" || out.UserName != "alice" {
		t.Fatalf("expected output for audit, got %#v", out)
	}
}

func testConfig() store.PlainConfig {
	return store.PlainConfig{
		Users: map[string]store.UserConfig{
			"alice": {
				Name: "alice",
				Auth: store.AuthConfig{Type: "key"},
			},
		},
		Hosts: map[string]store.HostConfig{
			"prod": {
				Host:    "prod.example.com",
				UserRef: "alice",
			},
		},
	}
}

type fakeResolver struct {
	userName string
	auth     store.AuthConfig
	err      error
	called   int
}

func (f *fakeResolver) ResolveHostIdentity(store.PlainConfig, store.HostConfig) (string, store.AuthConfig, error) {
	f.called++
	return f.userName, f.auth, f.err
}

type fakeRunner struct {
	called bool
	req    Request
	err    error
}

func (f *fakeRunner) CopyRemote(_ context.Context, req Request) error {
	f.called = true
	f.req = req
	return f.err
}
