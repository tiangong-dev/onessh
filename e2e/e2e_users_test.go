package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

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
