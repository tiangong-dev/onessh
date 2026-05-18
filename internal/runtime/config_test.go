package runtime

import (
	"testing"
	"time"
)

func TestNormalizeCacheTTL(t *testing.T) {
	t.Parallel()

	cases := map[time.Duration]time.Duration{
		0:                DefaultCacheTTL,
		-1 * time.Second: DefaultCacheTTL,
		time.Second:      time.Second,
	}

	for input, want := range cases {
		if got := NormalizeCacheTTL(input); got != want {
			t.Fatalf("NormalizeCacheTTL(%v) = %v, want %v", input, got, want)
		}
	}
}

func TestResolveAgentCapability(t *testing.T) {
	t.Parallel()

	derived := DeriveAgentCapability("session-a")

	cases := []struct {
		name      string
		explicit  string
		env       string
		sessionID string
		want      string
	}{
		{name: "explicit wins", explicit: " explicit ", env: "env", sessionID: "session-a", want: "explicit"},
		{name: "env fallback", env: " env ", sessionID: "session-a", want: "env"},
		{name: "derived fallback", sessionID: "session-a", want: derived},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveAgentCapability(tc.explicit, tc.env, tc.sessionID)
			if got != tc.want {
				t.Fatalf("ResolveAgentCapability() = %q, want %q", got, tc.want)
			}
		})
	}
}
