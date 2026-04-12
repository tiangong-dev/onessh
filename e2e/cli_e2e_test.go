package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var builtBinaryPath string

func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find repo root: %v\n", err)
		os.Exit(1)
	}

	tmpDir, err := os.MkdirTemp("", "onessh-e2e-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	builtBinaryPath = filepath.Join(tmpDir, "onessh-e2e")
	if runtime.GOOS == "windows" {
		builtBinaryPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", builtBinaryPath, "./cmd/onessh")
	buildCmd.Dir = root
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		fmt.Fprintf(os.Stderr, "build onessh binary failed: %v\n%s\n", buildErr, string(buildOut))
		os.Exit(1)
	}

	os.Exit(m.Run())
}

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

func bootstrapStoreWithHost(t *testing.T, dataDir, socketPath, capability, password string) {
	t.Helper()

	baseDir := shortTempDir(t)
	initOutPath := filepath.Join(baseDir, "init.out")
	addOutPath := filepath.Join(baseDir, "add.out")

	_, err := startAgentWithRetry(baseDir, socketPath, capability)
	if err != nil {
		t.Fatalf("start bootstrap agent: %v", err)
	}

	initCmd := shellCommand(
		builtBinaryPath,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"init",
	) + " > " + shellQuote(initOutPath)
	if _, err := runWithTTY(baseDir, initCmd, password+"\n"+password+"\n"); err != nil {
		t.Fatalf("bootstrap init: %v", err)
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
		t.Fatalf("bootstrap add host: %v", err)
	}
}

func runOnessh(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, builtBinaryPath, args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "onessh-e2e-")
	if err != nil {
		t.Fatalf("create short temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func startAgentWithRetry(workDir, socketPath, capability string) (string, error) {
	var lastOut string
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		lastOut, lastErr = runOnessh(workDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "start")
		if lastErr == nil {
			return lastOut, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastOut == "" {
		return "", lastErr
	}
	return lastOut, fmt.Errorf("%w\n%s", lastErr, lastOut)
}

func runWithTTY(workDir, command, input string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Use the BSD-compatible invocation form, which also works on GNU script.
	cmd := exec.CommandContext(ctx, "script", "-q", "/dev/null", "sh", "-lc", command)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func requireScript(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires script command and /dev/null")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command not found")
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func shellCommand(binary string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(binary))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
	// Answer "no" to the prompt about deleting the user profile
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

	// Change password
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

	// Stop the agent to clear cached passphrase, then start fresh
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

	// Old password should fail
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

	// New password should work
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

func TestCLIUserAddListRm(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent.sock")
	capability := "e2e-capability-user"
	password := "Passw0rd!User"

	bootstrapStoreWithHost(t, dataDir, socketPath, capability, password)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	// user add
	addOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"user", "add", "deployer",
		"--name", "deploy",
		"--auth-type", "key",
		"--key-path", "/tmp/fake_deploy_key",
	)
	if err != nil {
		t.Fatalf("run user add: %v\n%s", err, addOut)
	}
	if !strings.Contains(addOut, "user profile deployer added") {
		t.Fatalf("unexpected user add output: %q", addOut)
	}

	// user ls
	lsOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"user", "ls",
	)
	if err != nil {
		t.Fatalf("run user ls: %v\n%s", err, lsOut)
	}
	if !strings.Contains(lsOut, "deployer") || !strings.Contains(lsOut, "deploy") {
		t.Fatalf("expected deployer in user ls output: %q", lsOut)
	}

	// user rm
	rmOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"user", "rm", "deployer",
	)
	if err != nil {
		t.Fatalf("run user rm: %v\n%s", err, rmOut)
	}
	if !strings.Contains(rmOut, "user profile deployer removed") {
		t.Fatalf("unexpected user rm output: %q", rmOut)
	}

	// verify user is gone from ls
	lsAfterOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"user", "ls",
	)
	if err != nil {
		t.Fatalf("run user ls after rm: %v\n%s", err, lsAfterOut)
	}
	if strings.Contains(lsAfterOut, "deployer") {
		t.Fatalf("expected deployer to be gone from user ls: %q", lsAfterOut)
	}
}

func TestCLILogEnableDisableStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skip e2e in short mode")
	}
	requireScript(t)

	baseDir := shortTempDir(t)
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent.sock")
	capability := "e2e-capability-log"
	password := "Passw0rd!Log"

	bootstrapStoreWithHost(t, dataDir, socketPath, capability, password)
	defer func() {
		_, _ = runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "stop")
	}()

	// log enable
	enableOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"log", "enable",
	)
	if err != nil {
		t.Fatalf("run log enable: %v\n%s", err, enableOut)
	}
	if !strings.Contains(enableOut, "Audit logging enabled by default.") {
		t.Fatalf("unexpected log enable output: %q", enableOut)
	}

	// log status (should be enabled)
	statusOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"log", "status",
	)
	if err != nil {
		t.Fatalf("run log status: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "Audit logging is enabled by default.") {
		t.Fatalf("unexpected log status output: %q", statusOut)
	}

	// log disable
	disableOut, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"log", "disable",
	)
	if err != nil {
		t.Fatalf("run log disable: %v\n%s", err, disableOut)
	}
	if !strings.Contains(disableOut, "Audit logging disabled by default.") {
		t.Fatalf("unexpected log disable output: %q", disableOut)
	}

	// log status (should be disabled)
	statusOut2, err := runOnessh(baseDir,
		"--data", dataDir,
		"--agent-socket", socketPath,
		"--agent-capability", capability,
		"log", "status",
	)
	if err != nil {
		t.Fatalf("run log status after disable: %v\n%s", err, statusOut2)
	}
	if !strings.Contains(statusOut2, "Audit logging is disabled by default.") {
		t.Fatalf("unexpected log status output after disable: %q", statusOut2)
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goMod := filepath.Join(dir, "go.mod")
		if _, statErr := os.Stat(goMod); statErr == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", dir)
		}
		dir = parent
	}
}
