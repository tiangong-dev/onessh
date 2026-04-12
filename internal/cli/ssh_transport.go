package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"onessh/internal/store"
)

func sshDestination(host store.HostConfig, userName string) string {
	destination := host.Host
	if userName != "" {
		destination = fmt.Sprintf("%s@%s", userName, host.Host)
	}
	return destination
}

func applySSHCommonArgs(args []string, host store.HostConfig, portFlag string) []string {
	port := host.Port
	if port <= 0 {
		port = 22
	}
	args = append(args, portFlag, strconv.Itoa(port))
	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}
	return args
}

func applyKeyAuthArg(args []string, auth store.AuthConfig) ([]string, error) {
	switch strings.ToLower(auth.Type) {
	case "key":
		if auth.KeyPath == "" {
			return args, nil
		}
		keyPath, err := expandTilde(auth.KeyPath)
		if err != nil {
			return nil, err
		}
		return append(args, "-i", keyPath), nil
	case "password":
		return args, nil
	default:
		return nil, fmt.Errorf("unsupported auth type: %s", auth.Type)
	}
}

func buildSSHArgs(host store.HostConfig, userName string, auth store.AuthConfig, extra []string) ([]string, error) {
	args := applySSHCommonArgs(nil, host, "-p")
	args = appendSendEnvOptions(args, host.Env)
	args, err := applyKeyAuthArg(args, auth)
	if err != nil {
		return nil, err
	}
	args = append(args, extra...)
	args = append(args, sshDestination(host, userName))
	return args, nil
}

func buildSCPArgs(host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool) ([]string, error) {
	args := applySSHCommonArgs(nil, host, "-P")
	if recursive {
		args = append(args, "-r")
	}
	args, err := applyKeyAuthArg(args, auth)
	if err != nil {
		return nil, err
	}

	remote := sshDestination(host, userName) + ":" + remotePath
	if isUpload {
		args = append(args, localPaths...)
		args = append(args, remote)
		return args, nil
	}
	args = append(args, remote, localPaths[0])
	return args, nil
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
	cmd := exec.Command(binary, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	if len(extraFiles) > 0 {
		cmd.ExtraFiles = extraFiles
	}
	return cmd.Run()
}
