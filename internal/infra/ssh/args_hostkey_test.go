package ssh

import (
	"testing"

	"onessh/internal/domain"
)

// indexOfOptionValue scans args for a consecutive ("-o", value) pair and
// returns the index of the value element. It returns -1 if no such pair
// exists.
func indexOfOptionValue(args []string, value string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-o" && args[i+1] == value {
			return i + 1
		}
	}
	return -1
}

func TestBuildSSHArgsAppliesAcceptNewHostKeyPolicy(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", domain.AuthConfig{Type: "key"}, nil, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	idx := indexOfOptionValue(got, "StrictHostKeyChecking=accept-new")
	if idx < 0 {
		t.Fatalf("expected injected -o StrictHostKeyChecking=accept-new pair, got: %#v", got)
	}
	if idx >= len(got)-1 {
		t.Fatalf("StrictHostKeyChecking=accept-new must precede the destination element, got: %#v", got)
	}
}

func TestBuildSCPArgsAppliesAcceptNewHostKeyPolicy(t *testing.T) {
	t.Parallel()

	got, err := BuildSCPArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", domain.AuthConfig{Type: "key"}, "/var/log/app.log", []string{"./app.log"}, false, false, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSCPArgs: %v", err)
	}

	idx := indexOfOptionValue(got, "StrictHostKeyChecking=accept-new")
	if idx < 0 {
		t.Fatalf("expected injected -o StrictHostKeyChecking=accept-new pair, got: %#v", got)
	}
	if idx >= len(got)-1 {
		t.Fatalf("StrictHostKeyChecking=accept-new must precede the source/target path elements, got: %#v", got)
	}
}

func TestBuildSSHArgsCallerHostKeyOptionTakesPrecedence(t *testing.T) {
	t.Parallel()

	got, err := BuildSSHArgs(domain.NewPlainConfig(), domain.HostConfig{
		Host: "192.0.2.10",
	}, "ubuntu", domain.AuthConfig{Type: "key"}, []string{"-o", "StrictHostKeyChecking=yes"}, ArgsOptions{})
	if err != nil {
		t.Fatalf("BuildSSHArgs: %v", err)
	}

	callerIdx := indexOfOptionValue(got, "StrictHostKeyChecking=yes")
	if callerIdx < 0 {
		t.Fatalf("caller's -o StrictHostKeyChecking=yes pair must be present, got: %#v", got)
	}

	injectedIdx := indexOfOptionValue(got, "StrictHostKeyChecking=accept-new")
	if injectedIdx < 0 {
		t.Fatalf("injected -o StrictHostKeyChecking=accept-new pair must be present, got: %#v", got)
	}

	if callerIdx >= injectedIdx {
		t.Fatalf("caller's StrictHostKeyChecking=yes (index %d) must precede injected accept-new (index %d) so ssh first-value precedence honors the caller, got: %#v", callerIdx, injectedIdx, got)
	}
}
