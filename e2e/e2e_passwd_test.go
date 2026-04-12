package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIPasswd(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent.sock")
	capability := "e2e-capability-passwd"
	oldPassword := "Passw0rd!Old"
	newPassword := "Passw0rd!New"

	bootstrapStoreWithHost(t, dataDir, socketPath, capability, oldPassword)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	passwdOutPath := filepath.Join(baseDir, "passwd.out")
	passwdCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"passwd",
	) + " > " + shellQuote(passwdOutPath)
	passwdInput := oldPassword + "\n" + newPassword + "\n" + newPassword + "\n"
	if _, err := runWithTTY(baseDir, passwdCmd, passwdInput); err != nil {
		t.Fatalf("run passwd with tty: %v", err)
	}

	passwdOut := mustReadFile(t, passwdOutPath)
	if !strings.Contains(passwdOut, "master password updated") {
		t.Fatalf("unexpected passwd output: %q", passwdOut)
	}

	_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")

	socketPath2 := filepath.Join(baseDir, "agent2.sock")
	capability2 := "e2e-capability-passwd2"
	_, err := runOnessh(baseDir, "--agent-socket", socketPath2, "--agent-capability", capability2, "agent", "start")
	if err != nil {
		t.Fatalf("start fresh agent: %v", err)
	}
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath2, "--agent-capability", capability2, "agent", "stop")
	}()

	oldPassLsPath := filepath.Join(baseDir, "old-pass-ls.out")
	oldPassCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath2,
		"--agent-capability", capability2,
		"ls",
	) + " > " + shellQuote(oldPassLsPath) + " 2>&1"
	if _, err := runWithTTY(baseDir, oldPassCmd, oldPassword+"\n"); err == nil {
		oldPassOut := mustReadFile(t, oldPassLsPath)
		if strings.Contains(oldPassOut, "web1") {
			t.Fatalf("old password should not work after passwd change")
		}
	}

	newPassLsPath := filepath.Join(baseDir, "new-pass-ls.out")
	newPassCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath2,
		"--agent-capability", capability2,
		"ls",
	) + " > " + shellQuote(newPassLsPath)
	if _, err := runWithTTY(baseDir, newPassCmd, newPassword+"\n"); err != nil {
		t.Fatalf("ls with new password: %v", err)
	}
	newPassOut := mustReadFile(t, newPassLsPath)
	if !strings.Contains(newPassOut, "web1") {
		t.Fatalf("expected web1 in ls output with new password: %q", newPassOut)
	}
}
