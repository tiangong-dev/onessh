package ports

import (
	"testing"

	"onessh/internal/store"
)

type testIdentityResolver struct{}

func (testIdentityResolver) ResolveHostIdentity(store.PlainConfig, store.HostConfig) (string, store.AuthConfig, error) {
	return "alice", store.AuthConfig{Type: "key"}, nil
}

func TestIdentityResolverPortShape(t *testing.T) {
	t.Parallel()

	var resolver IdentityResolver = testIdentityResolver{}
	userName, auth, err := resolver.ResolveHostIdentity(store.PlainConfig{}, store.HostConfig{})
	if err != nil {
		t.Fatalf("ResolveHostIdentity returned error: %v", err)
	}
	if userName != "alice" || auth.Type != "key" {
		t.Fatalf("unexpected identity result: user=%q auth=%#v", userName, auth)
	}
}

func TestAgentConfigPortShape(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{
		Socket:     "/tmp/onessh.sock",
		Capability: "capability",
	}
	if cfg.Socket != "/tmp/onessh.sock" || cfg.Capability != "capability" {
		t.Fatalf("unexpected agent config: %#v", cfg)
	}
}

func TestAuditLoggerAdapterForwardsEvents(t *testing.T) {
	t.Parallel()

	var got AuditEvent
	logger := AuditLoggerFunc(func(event AuditEvent) {
		got = event
	})

	logger.Log(AuditEvent{Action: "connect", Alias: "prod", Result: "ok"})
	if got.Action != "connect" || got.Alias != "prod" || got.Result != "ok" {
		t.Fatalf("unexpected forwarded audit event: %#v", got)
	}
}
