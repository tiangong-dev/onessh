package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInitAddListShowDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent-a.sock")
	capability := "e2e-capability-a"
	password := "Passw0rd!A"

	initOutPath := filepath.Join(baseDir, "init.out")
	addOutPath := filepath.Join(baseDir, "add.out")

	_, err := startAgentWithRetry(baseDir, socketPath, capability)
	if err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	initCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"init",
	) + " > " + shellQuote(initOutPath)
	if _, err := runWithTTY(baseDir, initCmd, password+"\n"+password+"\n"); err != nil {
		t.Fatalf("run init with tty: %v", err)
	}

	addCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"add", "web1",
		"--tag", "core",
		"--env", "APP_ENV=dev",
	) + " > " + shellQuote(addOutPath)
	addInput := "127.0.0.1\nubuntu\n\n\n/tmp/fake_id_ed25519\n22\n\nlocal host for cli test\n"
	if _, err := runWithTTY(baseDir, addCmd, addInput); err != nil {
		t.Fatalf("run add with tty: %v", err)
	}

	addOut := mustReadFile(t, addOutPath)
	if !strings.Contains(addOut, "host web1 added") {
		t.Fatalf("unexpected add output: %q", addOut)
	}

	lsOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"ls",
	)
	if err != nil {
		t.Fatalf("run ls: %v\n%s", err, lsOut)
	}
	if !strings.Contains(lsOut, "web1") || !strings.Contains(lsOut, "127.0.0.1") {
		t.Fatalf("unexpected ls output: %q", lsOut)
	}

	showOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"show", "web1",
	)
	if err != nil {
		t.Fatalf("run show: %v\n%s", err, showOut)
	}
	if !strings.Contains(showOut, "Alias:        web1") || !strings.Contains(showOut, "APP_ENV=dev") {
		t.Fatalf("unexpected show output: %q", showOut)
	}

	dryRunOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"test", "--all", "--dry-run",
	)
	if err != nil {
		t.Fatalf("run dry-run test: %v\n%s", err, dryRunOut)
	}
	if !strings.Contains(dryRunOut, "Matched 1 host(s):") || !strings.Contains(dryRunOut, "web1") {
		t.Fatalf("unexpected dry-run output: %q", dryRunOut)
	}

	metaDoc := mustReadFile(t, filepath.Join(dataDir, "meta.yaml"))
	userDoc := mustReadFile(t, filepath.Join(dataDir, "users", "ubuntu.yaml"))
	hostDoc := mustReadFile(t, filepath.Join(dataDir, "hosts", "web1.yaml"))
	if !strings.Contains(metaDoc, "ENC[") || !strings.Contains(userDoc, "ENC[") || !strings.Contains(hostDoc, "ENC[") {
		t.Fatalf("expected encrypted fields in store documents")
	}
}

func TestCLISessionIsolationAndLogout(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketA := filepath.Join(baseDir, "agent-a.sock")
	capA := "e2e-capability-a"
	password := "Passw0rd!A"

	bootstrapStoreWithHost(t, dataDir, socketA, capA, password)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketA, "--agent-capability", capA, "agent", "stop")
	}()

	socketB := filepath.Join(baseDir, "agent-b.sock")
	capB := "e2e-capability-b"
	_, err := startAgentWithRetry(baseDir, socketB, capB)
	if err != nil {
		t.Fatalf("start session B agent: %v", err)
	}
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketB, "--agent-capability", capB, "agent", "stop")
	}()

	noCacheOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketB,
		"--agent-capability", capB,
		"ls",
	)
	if err == nil {
		t.Fatalf("expected session B ls without cache to fail")
	}
	if !strings.Contains(noCacheOut, "Enter master password:") {
		t.Fatalf("expected password prompt in failure output, got: %q", noCacheOut)
	}

	lsWithPassPath := filepath.Join(baseDir, "ls-with-pass.out")
	lsWithTTYCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketB,
		"--agent-capability", capB,
		"ls",
	) + " > " + shellQuote(lsWithPassPath)
	if _, err := runWithTTY(baseDir, lsWithTTYCmd, password+"\n"); err != nil {
		t.Fatalf("seed session B cache with tty: %v", err)
	}

	lsWithPassOut := mustReadFile(t, lsWithPassPath)
	if !strings.Contains(lsWithPassOut, "web1") {
		t.Fatalf("unexpected ls-with-pass output: %q", lsWithPassOut)
	}

	cachedOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketB,
		"--agent-capability", capB,
		"ls",
	)
	if err != nil {
		t.Fatalf("session B ls with cache: %v\n%s", err, cachedOut)
	}
	if !strings.Contains(cachedOut, "web1") {
		t.Fatalf("unexpected cached ls output: %q", cachedOut)
	}

	logoutOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketB,
		"--agent-capability", capB,
		"logout",
	)
	if err != nil {
		t.Fatalf("run logout: %v\n%s", err, logoutOut)
	}
	if !strings.Contains(logoutOut, "cache cleared") {
		t.Fatalf("unexpected logout output: %q", logoutOut)
	}

	afterLogoutOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketB,
		"--agent-capability", capB,
		"ls",
	)
	if err == nil {
		t.Fatalf("expected session B ls after logout to fail")
	}
	if !strings.Contains(afterLogoutOut, "Enter master password:") {
		t.Fatalf("expected password prompt after logout, got: %q", afterLogoutOut)
	}
}

func TestCLIUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent.sock")
	capability := "e2e-capability-update"
	password := "Passw0rd!U"

	bootstrapStoreWithHost(t, dataDir, socketPath, capability, password)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	updateOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"update", "web1",
		"--port", "2222",
	)
	if err != nil {
		t.Fatalf("run update: %v\n%s", err, updateOut)
	}
	if !strings.Contains(updateOut, "host web1 updated") {
		t.Fatalf("unexpected update output: %q", updateOut)
	}

	showOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"show", "web1",
	)
	if err != nil {
		t.Fatalf("run show after update: %v\n%s", err, showOut)
	}
	if !strings.Contains(showOut, "2222") {
		t.Fatalf("expected port 2222 in show output: %q", showOut)
	}
}

func TestCLIRm(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent.sock")
	capability := "e2e-capability-rm"
	password := "Passw0rd!R"

	bootstrapStoreWithHost(t, dataDir, socketPath, capability, password)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	rmOutPath := filepath.Join(baseDir, "rm.out")
	rmCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"rm", "web1",
	) + " > " + shellQuote(rmOutPath)
	if _, err := runWithTTY(baseDir, rmCmd, "n\n"); err != nil {
		t.Fatalf("run rm with tty: %v", err)
	}

	rmOut := mustReadFile(t, rmOutPath)
	if !strings.Contains(rmOut, "host web1 removed") {
		t.Fatalf("unexpected rm output: %q", rmOut)
	}

	lsOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"ls",
	)
	if err != nil {
		t.Fatalf("run ls after rm: %v\n%s", err, lsOut)
	}
	if strings.Contains(lsOut, "web1") {
		t.Fatalf("expected web1 to be gone from ls output: %q", lsOut)
	}
}
