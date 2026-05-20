package agent

import (
	"strings"
	"testing"
)

func TestIsPasswordPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{
			name:   "user@host password prompt",
			prompt: "deploy@example.com's password: ",
			want:   true,
		},
		{
			name:   "bare Password prompt",
			prompt: "Password: ",
			want:   true,
		},
		{
			name:   "parenthesized user host Password prompt",
			prompt: "(deploy@host) Password:",
			want:   true,
		},
		{
			name:   "empty prompt is treated as password request",
			prompt: "",
			want:   true,
		},
		{
			name:   "whitespace-only prompt is treated as password request",
			prompt: "   ",
			want:   true,
		},
		{
			name:   "host-key continue connecting prompt",
			prompt: "Are you sure you want to continue connecting (yes/no/[fingerprint])? ",
			want:   false,
		},
		{
			name:   "full multi-line host-key prompt",
			prompt: "The authenticity of host '100.94.248.65 (100.94.248.65)' can't be established.\nED25519 key fingerprint is SHA256:abc123def456.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ",
			want:   false,
		},
		{
			name:   "2FA verification code prompt",
			prompt: "Verification code: ",
			want:   false,
		},
		{
			name:   "host-key prompt with password in hostname",
			prompt: "The authenticity of host 'password.example.com (203.0.113.5)' can't be established.\nAre you sure you want to continue connecting (yes/no)? ",
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPasswordPrompt(tt.prompt); got != tt.want {
				t.Errorf("IsPasswordPrompt(%q) = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestAskPassLauncherScript(t *testing.T) {
	t.Parallel()

	script := AskPassLauncherScript()

	if !strings.Contains(script, "$@") {
		t.Errorf("AskPassLauncherScript() must forward prompt args via \"$@\"; got %#v", script)
	}
	if !strings.Contains(script, "askpass") {
		t.Errorf("AskPassLauncherScript() must invoke the askpass subcommand; got %#v", script)
	}
	if !strings.HasPrefix(script, "#!") {
		t.Errorf("AskPassLauncherScript() must start with a #! shebang line; got %#v", script)
	}
}
