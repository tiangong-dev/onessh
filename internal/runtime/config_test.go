package runtime

import (
	"path/filepath"
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

func TestResolveDataPath(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	envPath := filepath.Join(home, "env-data")
	customPath := filepath.Join(home, "custom-data")

	cases := []struct {
		name       string
		customPath string
		envPath    string
		want       string
	}{
		{name: "custom path wins", customPath: customPath, envPath: envPath, want: customPath},
		{name: "env path fallback", envPath: envPath, want: envPath},
		{name: "default path", want: filepath.Join(home, ".config", "onessh", "data")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDataPath(tc.customPath, tc.envPath, home)
			if err != nil {
				t.Fatalf("ResolveDataPath: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ResolveDataPath() = %q, want %q", got, tc.want)
			}
		})
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
