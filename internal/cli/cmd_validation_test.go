package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateOutputFormat(t *testing.T) {
	t.Parallel()

	got, err := validateOutputFormat(" JSON ", "table", "json")
	if err != nil {
		t.Fatalf("validateOutputFormat returned error: %v", err)
	}
	if got != "json" {
		t.Fatalf("unexpected normalized format: %q", got)
	}

	if _, err := validateOutputFormat("xml", "table", "json"); err == nil {
		t.Fatalf("expected invalid format to fail")
	}
}

func TestValidateUserAuthFlagUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{
			name: "no auth flags",
			args: nil,
		},
		{
			name:       "key path without auth type",
			args:       []string{"--key-path", "~/.ssh/id_ed25519"},
			wantErrSub: "--auth-type is required",
		},
		{
			name:       "password without auth type",
			args:       []string{"--password", "secret"},
			wantErrSub: "--auth-type is required",
		},
		{
			name: "valid key auth",
			args: []string{"--auth-type", "key", "--key-path", "~/.ssh/id_ed25519"},
		},
		{
			name: "valid password auth",
			args: []string{"--auth-type", "password", "--password", "secret"},
		},
		{
			name:       "incompatible password for key auth",
			args:       []string{"--auth-type", "key", "--password", "secret"},
			wantErrSub: "--password is only valid when --auth-type=password",
		},
		{
			name:       "incompatible key path for password auth",
			args:       []string{"--auth-type", "password", "--key-path", "~/.ssh/id_ed25519"},
			wantErrSub: "--key-path is only valid when --auth-type=key",
		},
		{
			name:       "key path and password together",
			args:       []string{"--auth-type", "key", "--key-path", "~/.ssh/id_ed25519", "--password", "secret"},
			wantErrSub: "cannot set --key-path and --password at the same time",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUserAuthFlagTestCommand()
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatalf("ParseFlags failed: %v", err)
			}

			authType, _ := cmd.Flags().GetString("auth-type")
			keyPath, _ := cmd.Flags().GetString("key-path")
			password, _ := cmd.Flags().GetString("password")

			err := validateUserAuthFlagUsage(cmd, authType, keyPath, password)
			if tc.wantErrSub == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErrSub, err)
			}
		})
	}
}

func newUserAuthFlagTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("auth-type", "", "")
	cmd.Flags().String("key-path", "", "")
	cmd.Flags().String("password", "", "")
	return cmd
}
