package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

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
