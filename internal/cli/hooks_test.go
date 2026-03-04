package cli

import (
	"strings"
	"testing"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func TestParseHookCommands(t *testing.T) {
	t.Parallel()

	got, err := parseHookCommands([]string{"cd /srv/app", "echo ready"}, "pre-connect")
	if err != nil {
		t.Fatalf("parseHookCommands: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(got))
	}
}

func TestParseHookCommandsRejectsEmpty(t *testing.T) {
	t.Parallel()

	_, err := parseHookCommands([]string{" "}, "pre-connect")
	if err == nil {
		t.Fatalf("expected parseHookCommands to reject empty command")
	}
}

func TestApplyHostHookUpdateFlags(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("pre-connect", nil, "")
	cmd.Flags().StringArray("post-connect", nil, "")
	cmd.Flags().Bool("clear-pre-connect", false, "")
	cmd.Flags().Bool("clear-post-connect", false, "")

	if err := cmd.Flags().Set("pre-connect", "cd /srv/app"); err != nil {
		t.Fatalf("set pre-connect: %v", err)
	}
	if err := cmd.Flags().Set("post-connect", "echo bye"); err != nil {
		t.Fatalf("set post-connect: %v", err)
	}

	host := store.HostConfig{}
	if err := applyHostHookUpdateFlags(cmd, &host, []string{"cd /srv/app"}, []string{"echo bye"}, false, false); err != nil {
		t.Fatalf("applyHostHookUpdateFlags: %v", err)
	}
	if len(host.PreConnect) != 1 || host.PreConnect[0] != "cd /srv/app" {
		t.Fatalf("unexpected pre_connect: %#v", host.PreConnect)
	}
	if len(host.PostConnect) != 1 || host.PostConnect[0] != "echo bye" {
		t.Fatalf("unexpected post_connect: %#v", host.PostConnect)
	}
}

func TestBuildRemoteHookCommand(t *testing.T) {
	t.Parallel()

	command := buildRemoteHookCommand([]string{"cd /srv/app"}, []string{"echo done"})
	if command == "" {
		t.Fatalf("expected non-empty remote hook command")
	}
	if !strings.Contains(command, "cd /srv/app") {
		t.Fatalf("expected pre command in generated command: %s", command)
	}
	if !strings.Contains(command, "echo done") {
		t.Fatalf("expected post command in generated command: %s", command)
	}
}

func TestContainsShortFlag(t *testing.T) {
	t.Parallel()

	if !containsShortFlag([]string{"-NT", "-L", "8080:localhost:80"}, 'N') {
		t.Fatalf("expected containsShortFlag to find -N")
	}
	if containsShortFlag([]string{"-o", "RequestTTY=yes"}, 'N') {
		t.Fatalf("did not expect to match long option values")
	}
}

