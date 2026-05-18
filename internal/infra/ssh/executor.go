package ssh

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// RunExternalCommand executes supported SSH transport binaries without invoking a shell.
func RunExternalCommand(binary string, args []string, env []string, extraFiles []*os.File, stdin io.Reader, stdout, stderr io.Writer) error {
	var cmd *exec.Cmd
	switch binary {
	case "ssh":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.Command("ssh", args...)
	case "scp":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.Command("scp", args...)
	case "sshpass":
		// #nosec G204 -- command is fixed; args are passed as argv without a shell.
		cmd = exec.Command("sshpass", args...)
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
