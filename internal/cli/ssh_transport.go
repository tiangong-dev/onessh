package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	infrassh "onessh/internal/infra/ssh"
	"onessh/internal/store"
)

func sshDestination(host store.HostConfig, userName string) string {
	return infrassh.Destination(host, userName)
}

func buildProxyJumpArgs(cfg store.PlainConfig, proxyJump string) ([]string, error) {
	return infrassh.BuildProxyJumpArgs(cfg, proxyJump, sshArgsOptions())
}

func buildOnesshProxyCommand(exePath, proxyJump string) string {
	return infrassh.BuildOnesshProxyCommand(exePath, proxyJump)
}

// buildSSHFlags builds the SSH option flags (port, proxy, identity, extras) without the destination.
// Use this when you need to insert additional flags or the remote command after building.
func buildSSHFlags(cfg store.PlainConfig, host store.HostConfig, auth store.AuthConfig, extra []string) ([]string, error) {
	return infrassh.BuildSSHFlags(cfg, host, auth, extra, sshArgsOptions())
}

// buildSSHArgs builds the full SSH argument list including the destination.
// Any extra flags are inserted before the destination.
func buildSSHArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, extra []string) ([]string, error) {
	return infrassh.BuildSSHArgs(cfg, host, userName, auth, extra, sshArgsOptions())
}

func buildSCPArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool) ([]string, error) {
	return infrassh.BuildSCPArgs(cfg, host, userName, auth, remotePath, localPaths, isUpload, recursive, sshArgsOptions())
}

func sshArgsOptions() infrassh.ArgsOptions {
	return infrassh.ArgsOptions{ResolveOnesshPath: os.Executable}
}

func withPasswordAuth(binary string, args []string, auth store.AuthConfig, env []string, agentSocket, agentCapability string, errOut io.Writer, baseBinary string) (string, []string, []string, []*os.File, func(), error) {
	if strings.ToLower(auth.Type) != "password" || auth.Password == "" {
		return binary, args, env, nil, func() {}, nil
	}

	if _, err := exec.LookPath("sshpass"); err == nil {
		fd, cleanup, err := newPasswordFD(auth.Password)
		if err != nil {
			return "", nil, nil, nil, nil, err
		}
		return "sshpass", append([]string{"-d", "3", baseBinary}, args...), env, []*os.File{fd}, cleanup, nil
	}

	if errOut != nil {
		fmt.Fprintln(errOut, "sshpass not found; using weaker SSH_ASKPASS fallback with a short-lived single-use agent token.")
	}
	askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, agentCapability, auth.Password)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	return binary, args, append(env, askPassEnv...), nil, cleanup, nil
}

func runExternalCommand(binary string, args []string, env []string, extraFiles []*os.File, stdin io.Reader, stdout, stderr io.Writer) error {
	return infrassh.RunExternalCommand(binary, args, env, extraFiles, stdin, stdout, stderr)
}
