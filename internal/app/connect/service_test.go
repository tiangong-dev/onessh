package connect

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

func TestServiceConnectRunsTransport(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	var errOut bytes.Buffer
	resolver := &fakeResolver{
		userName: "alice",
		auth:     store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"},
	}
	transport := &fakeTransport{}
	service := Service{
		IdentityResolver: resolver,
		Transport:        transport,
	}

	out, err := service.Connect(context.Background(), Input{
		Config:            cfg,
		Alias:             "prod",
		SSHArgs:           []string{"-A"},
		ProxyJumpOverride: "jump",
		ProxyJumpChanged:  true,
		AgentSocket:       "/tmp/onessh.sock",
		AgentCapability:   "capability",
		IO: appruntime.IOStreams{
			ErrOut: &errOut,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if out.Alias != "prod" || out.Host != "prod.example.com" || out.UserName != "alice" || out.Port != 22 {
		t.Fatalf("unexpected output: %#v", out)
	}
	if got := errOut.String(); got != "Connecting to alice@prod.example.com:22...\n" {
		t.Fatalf("unexpected status output: %q", got)
	}
	if resolver.called != 1 {
		t.Fatalf("resolver called %d times, want 1", resolver.called)
	}
	if !transport.called {
		t.Fatalf("transport was not called")
	}
	if transport.req.Host.ProxyJump != "jump" {
		t.Fatalf("proxy jump override was not applied: %#v", transport.req.Host)
	}
	if !reflect.DeepEqual(transport.req.SSHArgs, []string{"-A"}) {
		t.Fatalf("unexpected ssh args: %#v", transport.req.SSHArgs)
	}
	if transport.req.AgentSocket != "/tmp/onessh.sock" || transport.req.AgentCapability != "capability" {
		t.Fatalf("unexpected agent fields: %#v", transport.req)
	}
}

func TestServiceConnectQuietSuppressesStatusOutput(t *testing.T) {
	t.Parallel()

	var errOut bytes.Buffer
	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Transport:        &fakeTransport{},
	}

	_, err := service.Connect(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
		Quiet:  true,
		IO: appruntime.IOStreams{
			ErrOut: &errOut,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("quiet mode wrote status output: %q", got)
	}
}

func TestServiceConnectMissingHost(t *testing.T) {
	t.Parallel()

	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Transport:        &fakeTransport{},
	}

	_, err := service.Connect(context.Background(), Input{
		Config: testConfig(),
		Alias:  "missing",
	})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestServiceConnectResolverErrorStopsTransport(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("identity failed")
	transport := &fakeTransport{}
	service := Service{
		IdentityResolver: &fakeResolver{err: wantErr},
		Transport:        transport,
	}

	_, err := service.Connect(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Connect error = %v, want %v", err, wantErr)
	}
	if transport.called {
		t.Fatalf("transport should not run after resolver error")
	}
}

func TestServiceConnectTransportErrorReturnsOutputForAudit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ssh failed")
	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Transport:        &fakeTransport{err: wantErr},
	}

	out, err := service.Connect(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
		Quiet:  true,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Connect error = %v, want %v", err, wantErr)
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

type fakeTransport struct {
	called bool
	req    TransportRequest
	err    error
}

func (f *fakeTransport) Connect(_ context.Context, req TransportRequest) error {
	f.called = true
	f.req = req
	return f.err
}
