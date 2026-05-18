package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// RunExternalCommand executes supported SSH transport binaries without invoking a shell.
// The provided ctx governs the lifetime of the child process: when ctx is cancelled the
// process is killed via exec.CommandContext, which is essential for Ctrl-C / timeout
// propagation through to the underlying ssh/scp/sshpass binary.
func RunExternalCommand(ctx context.Context, binary string, args []string, env []string, extraFiles []*os.File, stdin io.Reader, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var cmd *exec.Cmd
	switch binary {
	case "ssh":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.CommandContext(ctx, "ssh", args...)
	case "scp":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.CommandContext(ctx, "scp", args...)
	case "sshpass":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.CommandContext(ctx, "sshpass", args...)
	default:
		return fmt.Errorf("unsupported external command: %s", binary)
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if len(extraFiles) > 0 {
		cmd.ExtraFiles = extraFiles
	}
	return cmd.Run()
}
