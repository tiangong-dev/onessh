package cli

import (
	"reflect"
	"testing"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func TestParseEnvAssignments(t *testing.T) {
	t.Parallel()

	got, err := parseEnvAssignments([]string{"FOO=bar", "EMPTY=", "PATH=/usr/bin"})
	if err != nil {
		t.Fatalf("parseEnvAssignments: %v", err)
	}

	want := map[string]string{
		"FOO":   "bar",
		"EMPTY": "",
		"PATH":  "/usr/bin",
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected env map: want=%v got=%v", want, got)
	}
}

func TestParseEnvAssignmentsInvalid(t *testing.T) {
	t.Parallel()

	_, err := parseEnvAssignments([]string{"1FOO=bar"})
	if err == nil {
		t.Fatalf("expected error for invalid env key")
	}
}

func TestApplyHostEnvUpdateFlags(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("env", nil, "")
	cmd.Flags().StringArray("unset-env", nil, "")
	cmd.Flags().Bool("clear-env", false, "")

	if err := cmd.Flags().Set("unset-env", "OLD"); err != nil {
		t.Fatalf("set unset-env: %v", err)
	}
	if err := cmd.Flags().Set("env", "NEW=value"); err != nil {
		t.Fatalf("set env: %v", err)
	}

	host := store.HostConfig{
		Env: map[string]string{
			"OLD": "value",
		},
	}

	err := applyHostEnvUpdateFlags(cmd, &host, []string{"NEW=value"}, []string{"OLD"}, false)
	if err != nil {
		t.Fatalf("applyHostEnvUpdateFlags: %v", err)
	}

	want := map[string]string{"NEW": "value"}
	if !reflect.DeepEqual(want, host.Env) {
		t.Fatalf("unexpected host env: want=%v got=%v", want, host.Env)
	}
}

func TestAppendSendEnvOptions(t *testing.T) {
	t.Parallel()

	args := appendSendEnvOptions([]string{"-p", "22"}, map[string]string{"B": "2", "A": "1"})
	want := []string{"-p", "22", "-o", "SendEnv=A", "-o", "SendEnv=B"}
	if !reflect.DeepEqual(want, args) {
		t.Fatalf("unexpected args: want=%v got=%v", want, args)
	}
}

