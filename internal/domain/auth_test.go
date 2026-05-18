package domain

import "testing"

func TestNormalizeAuthType(t *testing.T) {
	t.Parallel()

	cases := map[string]AuthType{
		" key ":    AuthTypeKey,
		"PASSWORD": AuthTypePassword,
		"1":        AuthTypeKey,
		"k":        AuthTypeKey,
		"2":        AuthTypePassword,
		"p":        AuthTypePassword,
		"pass":     AuthTypePassword,
		"invalid":  "",
		"":         "",
	}

	for input, want := range cases {
		if got := NormalizeAuthType(input); got != want {
			t.Fatalf("NormalizeAuthType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeStoredAuthType(t *testing.T) {
	t.Parallel()

	cases := map[string]AuthType{
		" key ":    AuthTypeKey,
		"PASSWORD": AuthTypePassword,
		"1":        "",
		"k":        "",
		"pass":     "",
		"invalid":  "",
		"":         "",
	}

	for input, want := range cases {
		if got := NormalizeStoredAuthType(input); got != want {
			t.Fatalf("NormalizeStoredAuthType(%q) = %q, want %q", input, got, want)
		}
	}
}
