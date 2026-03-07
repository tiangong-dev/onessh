package cli

import (
	"testing"
	"time"
)

func TestEnvDurationMillis(t *testing.T) {
	t.Setenv("ONESSH_AGENT_DIAL_TIMEOUT_MS", "123")
	if got := envDurationMillis("ONESSH_AGENT_DIAL_TIMEOUT_MS"); got != 123*time.Millisecond {
		t.Fatalf("unexpected duration: %v", got)
	}

	t.Setenv("ONESSH_AGENT_DIAL_TIMEOUT_MS", "-1")
	if got := envDurationMillis("ONESSH_AGENT_DIAL_TIMEOUT_MS"); got != 0 {
		t.Fatalf("expected zero for invalid negative value, got %v", got)
	}

	t.Setenv("ONESSH_AGENT_DIAL_TIMEOUT_MS", "abc")
	if got := envDurationMillis("ONESSH_AGENT_DIAL_TIMEOUT_MS"); got != 0 {
		t.Fatalf("expected zero for invalid text value, got %v", got)
	}
}

func TestShushClientOptionsFromEnv(t *testing.T) {
	t.Setenv("ONESSH_AGENT_DIAL_TIMEOUT_MS", "200")
	t.Setenv("ONESSH_AGENT_REQUEST_TIMEOUT_MS", "1200")
	t.Setenv("ONESSH_AGENT_STARTUP_TIMEOUT_MS", "1500")
	t.Setenv("ONESSH_AGENT_STARTUP_PROBE_INTERVAL_MS", "40")

	opts := shushClientOptionsFromEnv()
	if opts.DialTimeout != 200*time.Millisecond {
		t.Fatalf("unexpected dial timeout: %v", opts.DialTimeout)
	}
	if opts.RequestTimeout != 1200*time.Millisecond {
		t.Fatalf("unexpected request timeout: %v", opts.RequestTimeout)
	}
	if opts.StartupTimeout != 1500*time.Millisecond {
		t.Fatalf("unexpected startup timeout: %v", opts.StartupTimeout)
	}
	if opts.StartupProbeInterval != 40*time.Millisecond {
		t.Fatalf("unexpected startup probe interval: %v", opts.StartupProbeInterval)
	}
}
