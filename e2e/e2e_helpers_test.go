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
