package cli

import (
	"strings"
	"testing"

	"onessh/internal/store"
)

func TestNormalizeAuthType(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		" key ":    "key",
		"PASSWORD": "password",
		"1":        "key",
		"2":        "password",
		"invalid":  "",
		"":         "",
	}

	for input, want := range cases {
		if got := normalizeAuthType(input); got != want {
			t.Fatalf("normalizeAuthType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSummarizeAuth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		auth store.AuthConfig
		want string
	}{
		{name: "key with path", auth: store.AuthConfig{Type: "key", KeyPath: "~/.ssh/id_ed25519"}, want: "key:~/.ssh/id_ed25519"},
		{name: "key without path", auth: store.AuthConfig{Type: "key"}, want: "key"},
		{name: "password", auth: store.AuthConfig{Type: "password"}, want: "password"},
		{name: "unknown", auth: store.AuthConfig{Type: "oauth"}, want: "none"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeAuth(tc.auth); got != tc.want {
				t.Fatalf("summarizeAuth() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuthConfigFromFlags(t *testing.T) {
	t.Parallel()

	auth, err := authConfigFromFlags("key", "~/.ssh/id_ed25519", "")
	if err != nil {
		t.Fatalf("authConfigFromFlags key: %v", err)
	}
	if auth.Type != "key" || auth.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("unexpected key auth: %#v", auth)
	}

	auth, err = authConfigFromFlags("password", "", "secret")
	if err != nil {
		t.Fatalf("authConfigFromFlags password: %v", err)
	}
	if auth.Type != "password" || auth.Password != "secret" {
		t.Fatalf("unexpected password auth: %#v", auth)
	}

	if _, err := authConfigFromFlags("key", "~/.ssh/id_ed25519", "secret"); err == nil || !strings.Contains(err.Error(), "cannot set --key-path and --password") {
		t.Fatalf("expected key/password conflict error, got %v", err)
	}
}

func TestAuthConfigFromFlagValues(t *testing.T) {
	t.Parallel()

	current := store.AuthConfig{Type: "KEY", KeyPath: "~/.ssh/id_ed25519"}

	auth, err := authConfigFromFlagValues(current, "", "", "", false, false, false)
	if err != nil {
		t.Fatalf("authConfigFromFlagValues passthrough: %v", err)
	}
	if auth.Type != "key" || auth.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("unexpected passthrough auth: %#v", auth)
	}

	auth, err = authConfigFromFlagValues(current, "key", "", "", true, false, false)
	if err != nil {
		t.Fatalf("authConfigFromFlagValues key fallback: %v", err)
	}
	if auth.Type != "key" || auth.KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("unexpected key fallback auth: %#v", auth)
	}

	auth, err = authConfigFromFlagValues(current, "password", "", "secret", true, false, true)
	if err != nil {
		t.Fatalf("authConfigFromFlagValues password update: %v", err)
	}
	if auth.Type != "password" || auth.Password != "secret" {
		t.Fatalf("unexpected password update auth: %#v", auth)
	}
}
