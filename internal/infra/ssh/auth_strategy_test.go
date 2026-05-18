package ssh

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
)

func TestPasswordAuthStrategySkipsNonPasswordAuth(t *testing.T) {
	t.Parallel()

	strategy := PasswordAuthStrategy{
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not be called for key auth")
			return "", nil
		},
		NewPasswordFD: func(string) (*os.File, func(), error) {
			t.Fatal("NewPasswordFD should not be called for key auth")
			return nil, nil, nil
		},
		PrepareAskPassEnv: func(string, string, string) ([]string, func(), error) {
			t.Fatal("PrepareAskPassEnv should not be called for key auth")
			return nil, nil, nil
		},
	}

	got, err := strategy.ApplyPasswordAuth(PasswordAuthRequest{
		Binary:     "ssh",
		Args:       []string{"-p", "22", "deploy@example.com"},
		Auth:       domain.AuthConfig{Type: "key", Password: "ignored"},
		Env:        []string{"A=1"},
		BaseBinary: "ssh",
	})
	if err != nil {
		t.Fatalf("ApplyPasswordAuth: %v", err)
	}

	if got.Binary != "ssh" {
		t.Fatalf("unexpected binary: %q", got.Binary)
	}
	if !reflect.DeepEqual(got.Args, []string{"-p", "22", "deploy@example.com"}) {
		t.Fatalf("unexpected args: %#v", got.Args)
	}
	if !reflect.DeepEqual(got.Env, []string{"A=1"}) {
		t.Fatalf("unexpected env: %#v", got.Env)
	}
	if got.ExtraFiles != nil {
		t.Fatalf("expected no extra files, got %#v", got.ExtraFiles)
	}
	if got.Cleanup == nil {
		t.Fatal("expected no-op cleanup")
	}
	got.Cleanup()
}

func TestPasswordAuthStrategyUsesSSHPassWhenAvailable(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	_ = writer.Close()
	t.Cleanup(func() { _ = reader.Close() })

	var fdCleaned bool
	var warnings bytes.Buffer
	strategy := PasswordAuthStrategy{
		LookPath: func(name string) (string, error) {
			if name != "sshpass" {
				t.Fatalf("unexpected LookPath name: %q", name)
			}
			return "/usr/bin/sshpass", nil
		},
		NewPasswordFD: func(password string) (*os.File, func(), error) {
			if password != "secret" {
				t.Fatalf("unexpected password: %q", password)
			}
			return reader, func() { fdCleaned = true }, nil
		},
		PrepareAskPassEnv: func(string, string, string) ([]string, func(), error) {
			t.Fatal("PrepareAskPassEnv should not be called when sshpass is available")
			return nil, nil, nil
		},
		WarningWriter: &warnings,
	}

	got, err := strategy.ApplyPasswordAuth(PasswordAuthRequest{
		Binary:     "scp",
		Args:       []string{"-P", "22", "file", "deploy@example.com:/tmp/file"},
		Auth:       domain.AuthConfig{Type: "password", Password: "secret"},
		Env:        []string{"A=1"},
		BaseBinary: "scp",
	})
	if err != nil {
		t.Fatalf("ApplyPasswordAuth: %v", err)
	}

	wantArgs := []string{"-d", "3", "scp", "-P", "22", "file", "deploy@example.com:/tmp/file"}
	if got.Binary != "sshpass" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("unexpected sshpass invocation: binary=%q args=%#v", got.Binary, got.Args)
	}
	if !reflect.DeepEqual(got.Env, []string{"A=1"}) {
		t.Fatalf("unexpected env: %#v", got.Env)
	}
	if len(got.ExtraFiles) != 1 || got.ExtraFiles[0] != reader {
		t.Fatalf("unexpected extra files: %#v", got.ExtraFiles)
	}
	if warnings.Len() != 0 {
		t.Fatalf("expected no warning, got %q", warnings.String())
	}
	got.Cleanup()
	if !fdCleaned {
		t.Fatal("expected sshpass fd cleanup to run")
	}
}

func TestPasswordAuthStrategyFallsBackToAskPassWhenSSHPassMissing(t *testing.T) {
	t.Parallel()

	var askPassCleaned bool
	var warnings bytes.Buffer
	strategy := PasswordAuthStrategy{
		LookPath: func(name string) (string, error) {
			if name != "sshpass" {
				t.Fatalf("unexpected LookPath name: %q", name)
			}
			return "", errors.New("not found")
		},
		NewPasswordFD: func(string) (*os.File, func(), error) {
			t.Fatal("NewPasswordFD should not be called when sshpass is missing")
			return nil, nil, nil
		},
		PrepareAskPassEnv: func(socket, capability, password string) ([]string, func(), error) {
			if socket != "/tmp/agent.sock" || capability != "cap" || password != "secret" {
				t.Fatalf("unexpected askpass request: socket=%q capability=%q password=%q", socket, capability, password)
			}
			return []string{"SSH_ASKPASS=/tmp/helper", "DISPLAY=onessh:0"}, func() {
				askPassCleaned = true
			}, nil
		},
		WarningWriter: &warnings,
	}

	got, err := strategy.ApplyPasswordAuth(PasswordAuthRequest{
		Binary:          "ssh",
		Args:            []string{"deploy@example.com"},
		Auth:            domain.AuthConfig{Type: "password", Password: "secret"},
		Env:             []string{"A=1"},
		AgentSocket:     "/tmp/agent.sock",
		AgentCapability: "cap",
		BaseBinary:      "ssh",
	})
	if err != nil {
		t.Fatalf("ApplyPasswordAuth: %v", err)
	}

	if got.Binary != "ssh" || !reflect.DeepEqual(got.Args, []string{"deploy@example.com"}) {
		t.Fatalf("unexpected askpass invocation: binary=%q args=%#v", got.Binary, got.Args)
	}
	wantEnv := []string{"A=1", "SSH_ASKPASS=/tmp/helper", "DISPLAY=onessh:0"}
	if !reflect.DeepEqual(got.Env, wantEnv) {
		t.Fatalf("unexpected env:\nwant: %#v\n got: %#v", wantEnv, got.Env)
	}
	if got.ExtraFiles != nil {
		t.Fatalf("expected no extra files, got %#v", got.ExtraFiles)
	}
	if !strings.Contains(warnings.String(), "sshpass not found; using weaker SSH_ASKPASS fallback") {
		t.Fatalf("expected askpass fallback warning, got %q", warnings.String())
	}
	got.Cleanup()
	if !askPassCleaned {
		t.Fatal("expected askpass cleanup to run")
	}
}
