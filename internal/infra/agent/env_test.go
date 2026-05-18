package agent

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tiangong-dev/shush"
)

func TestResolveSocketPathPrecedence(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(string(filepath.Separator), "home", "alice")

	t.Run("explicit socket has highest priority and expands tilde", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveSocketPath(SocketPathRequest{
			Explicit:   "~/agent.sock",
			EnvSocket:  "/tmp/env.sock",
			RuntimeDir: "/run/user/1000",
			HomeDir:    homeDir,
			ParentPID:  42,
		})
		if err != nil {
			t.Fatalf("ResolveSocketPath: %v", err)
		}
		want := filepath.Join(homeDir, "agent.sock")
		if got != want {
			t.Fatalf("unexpected socket path: want=%q got=%q", want, got)
		}
	})

	t.Run("environment socket is used before default", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveSocketPath(SocketPathRequest{
			EnvSocket:  "/tmp/onessh.sock",
			RuntimeDir: "/run/user/1000",
			HomeDir:    homeDir,
			ParentPID:  42,
		})
		if err != nil {
			t.Fatalf("ResolveSocketPath: %v", err)
		}
		if got != "/tmp/onessh.sock" {
			t.Fatalf("unexpected socket path: %q", got)
		}
	})

	t.Run("xdg runtime default includes parent pid", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveSocketPath(SocketPathRequest{
			RuntimeDir: "/run/user/1000",
			HomeDir:    homeDir,
			ParentPID:  42,
		})
		if err != nil {
			t.Fatalf("ResolveSocketPath: %v", err)
		}
		want := filepath.Join("/run/user/1000", "onessh", "agent-42.sock")
		if got != want {
			t.Fatalf("unexpected socket path: want=%q got=%q", want, got)
		}
	})

	t.Run("home config default when runtime dir is empty", func(t *testing.T) {
		t.Parallel()

		got, err := ResolveSocketPath(SocketPathRequest{
			HomeDir:   homeDir,
			ParentPID: 42,
		})
		if err != nil {
			t.Fatalf("ResolveSocketPath: %v", err)
		}
		want := filepath.Join(homeDir, ".config", "onessh", "agents", "agent-42.sock")
		if got != want {
			t.Fatalf("unexpected socket path: want=%q got=%q", want, got)
		}
	})
}

func TestResolveCapabilityPrecedence(t *testing.T) {
	t.Parallel()

	if got := ResolveCapability(" explicit-cap ", "env-cap", "session"); got != "explicit-cap" {
		t.Fatalf("expected explicit capability, got %q", got)
	}
	if got := ResolveCapability("", " env-cap ", "session"); got != "env-cap" {
		t.Fatalf("expected environment capability, got %q", got)
	}

	first := ResolveCapability("", "", "uid:1000:ppid:42")
	second := ResolveCapability("", "", "uid:1000:ppid:42")
	if first == "" || first != second {
		t.Fatalf("expected deterministic derived capability, first=%q second=%q", first, second)
	}
	if first == ResolveCapability("", "", "uid:1000:ppid:43") {
		t.Fatalf("expected different sessions to derive different capabilities")
	}
}

func TestBuildAskPassEnv(t *testing.T) {
	t.Parallel()

	got := BuildAskPassEnv(AskPassEnv{
		ScriptPath: "/tmp/askpass.sh",
		Executable: "/usr/bin/onessh",
		SocketPath: "/tmp/agent.sock",
		Token:      "token-1",
		Capability: "cap-1",
	})
	want := []string{
		"SSH_ASKPASS=/tmp/askpass.sh",
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=onessh:0",
		"ONESSH_ASKPASS_EXE=/usr/bin/onessh",
		"ONESSH_ASKPASS_SOCKET=/tmp/agent.sock",
		"ONESSH_ASKPASS_TOKEN=token-1",
		"ONESSH_ASKPASS_CAPABILITY=cap-1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected askpass env:\nwant: %#v\n got: %#v", want, got)
	}

	got = BuildAskPassEnv(AskPassEnv{
		ScriptPath: "/tmp/askpass.sh",
		Executable: "/usr/bin/onessh",
		SocketPath: "/tmp/agent.sock",
		Token:      "token-1",
	})
	for _, kv := range got {
		if kv == "ONESSH_ASKPASS_CAPABILITY=" {
			t.Fatalf("empty askpass capability should not be emitted: %#v", got)
		}
	}
}

func TestWithCapabilityEnvReplacesAgentCapabilityVariables(t *testing.T) {
	t.Parallel()

	got := WithCapabilityEnv([]string{
		"A=1",
		AgentCapabilityEnv + "=old-onessh",
		shush.EnvCapability + "=old-shush",
		"B=2",
	}, "new-cap")
	want := []string{
		"A=1",
		"B=2",
		AgentCapabilityEnv + "=new-cap",
		shush.EnvCapability + "=new-cap",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected env:\nwant: %#v\n got: %#v", want, got)
	}
}
