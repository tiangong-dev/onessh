package store

import (
	"path/filepath"
	"testing"
)

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
