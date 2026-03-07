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

	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	socketPath := filepath.Join(baseDir, "agent-a.sock")
	capability := "e2e-capability-a"
	password := "Passw0rd!A"

	initOutPath := filepath.Join(baseDir, "init.out")
	addOutPath := filepath.Join(baseDir, "add.out")

	_, err := runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "start")
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

	baseDir := t.TempDir()
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
	_, err := runOnessh(baseDir, "--agent-socket", socketB, "--agent-capability", capB, "agent", "start")
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

	baseDir := t.TempDir()
	initOutPath := filepath.Join(baseDir, "init.out")
	addOutPath := filepath.Join(baseDir, "add.out")

	_, err := runOnessh(baseDir, "--agent-socket", socketPath, "--agent-capability", capability, "agent", "start")
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

func runWithTTY(workDir, command, input string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "script", "-qec", command, "/dev/null")
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
