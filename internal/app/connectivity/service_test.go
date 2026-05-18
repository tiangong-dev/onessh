package connectivity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"onessh/internal/ports"
	"onessh/internal/store"
)

func TestServicePingRunsConnectivityCheck(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{
		userName: "alice",
		auth:     store.AuthConfig{Type: "key"},
	}
	runner := &fakeRunner{}
	audit := &fakeAudit{}
	service := Service{
		IdentityResolver: resolver,
		Runner:           runner,
		Audit:            audit,
	}

	out, err := service.Ping(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
		Timeout: TimeoutConfig{
			Seconds: 7,
		},
		Agent: ports.AgentConfig{
			Socket:     "/tmp/onessh.sock",
			Capability: "capability",
		},
	})
	if err != nil {
		t.Fatalf("Ping: %v", err)
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
	if runner.req.Timeout.Seconds != 7 || runner.req.Agent.Socket != "/tmp/onessh.sock" || runner.req.Agent.Capability != "capability" {
		t.Fatalf("unexpected runner request: %#v", runner.req)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "ping" || audit.events[0].Result != "ok" {
		t.Fatalf("unexpected audit events: %#v", audit.events)
	}
}

func TestServicePingMissingHost(t *testing.T) {
	t.Parallel()

	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{},
	}

	_, err := service.Ping(context.Background(), Input{
		Config: testConfig(),
		Alias:  "missing",
	})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestServicePingResolverErrorStopsRunner(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("identity failed")
	runner := &fakeRunner{}
	service := Service{
		IdentityResolver: &fakeResolver{err: wantErr},
		Runner:           runner,
	}

	_, err := service.Ping(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Ping error = %v, want %v", err, wantErr)
	}
	if runner.called {
		t.Fatalf("runner should not run after resolver error")
	}
}

func TestServicePingRunnerErrorReturnsOutputForAudit(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ssh failed")
	audit := &fakeAudit{}
	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{err: wantErr},
		Audit:            audit,
	}

	out, err := service.Ping(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Ping error = %v, want %v", err, wantErr)
	}
	if out.Alias != "prod" || out.Host != "prod.example.com" || out.UserName != "alice" {
		t.Fatalf("expected output for audit, got %#v", out)
	}
	if len(audit.events) != 1 || audit.events[0].Result != "fail" || audit.events[0].Error == "" {
		t.Fatalf("unexpected audit events: %#v", audit.events)
	}
}

func TestServicePingAuditErrorDoesNotBlock(t *testing.T) {
	t.Parallel()

	service := Service{
		IdentityResolver: &fakeResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner:           &fakeRunner{},
		Audit:            &fakeAudit{err: errors.New("audit failed")},
	}

	if _, err := service.Ping(context.Background(), Input{
		Config: testConfig(),
		Alias:  "prod",
	}); err != nil {
		t.Fatalf("Ping should ignore audit errors: %v", err)
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

func (f *fakeRunner) Ping(_ context.Context, req Request) error {
	f.called = true
	f.req = req
	return f.err
}

type fakeAudit struct {
	events []ports.AuditEvent
	err    error
}

func (f *fakeAudit) Log(event ports.AuditEvent) error {
	f.events = append(f.events, event)
	return f.err
}
