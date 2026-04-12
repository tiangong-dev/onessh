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

// buildProxyJumpArgs resolves the ProxyJump value into SSH arguments.
// If proxyJump matches an alias in cfg.Hosts, the alias is expanded automatically:
//   - key auth: resolves to -J user@host:port
//   - password auth: resolves to -o ProxyCommand using the onessh binary itself
//
// If proxyJump does not match any alias, it is treated as a raw SSH jump spec (user@host:port).
func buildProxyJumpArgs(cfg store.PlainConfig, proxyJump string) ([]string, error) {
	if proxyJump == "" {
		return nil, nil
	}

	jumpHostCfg, isAlias := cfg.Hosts[proxyJump]
	if !isAlias {
		// Raw spec (e.g. "user@jumphost:22") — pass through unchanged.
		return []string{"-J", proxyJump}, nil
	}

	jumpUser, ok := cfg.Users[jumpHostCfg.UserRef]
	if !ok {
		return nil, fmt.Errorf("jump host alias %q references unknown user profile %q", proxyJump, jumpHostCfg.UserRef)
	}

	port := jumpHostCfg.Port
	if port <= 0 {
		port = 22
	}

	switch strings.ToLower(jumpUser.Auth.Type) {
	case "key":
		dest := fmt.Sprintf("%s@%s:%d", jumpUser.Name, jumpHostCfg.Host, port)
		return []string{"-J", dest}, nil
	case "password":
		// Run onessh itself as the ProxyCommand so it can handle password auth
		// using the existing passcache agent. The subprocess inherits the current
		// environment (agent socket, data directory, etc.).
		exePath, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve onessh path for proxy: %w", err)
		}
		proxyCmd := fmt.Sprintf("%s -q %s -- -W %%h:%%p", exePath, proxyJump)
		return []string{"-o", "ProxyCommand=" + proxyCmd}, nil
	default:
		return nil, fmt.Errorf("jump host %q has unsupported auth type %q", proxyJump, jumpUser.Auth.Type)
	}
}

func applySSHCommonArgs(args []string, cfg store.PlainConfig, host store.HostConfig, portFlag string) ([]string, error) {
	port := host.Port
	if port <= 0 {
		port = 22
	}
	args = append(args, portFlag, strconv.Itoa(port))
	if host.ProxyJump != "" {
		proxyArgs, err := buildProxyJumpArgs(cfg, host.ProxyJump)
		if err != nil {
			return nil, err
		}
		args = append(args, proxyArgs...)
	}
	return args, nil
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

// buildSSHFlags builds the SSH option flags (port, proxy, identity, extras) without the destination.
// Use this when you need to insert additional flags or the remote command after building.
func buildSSHFlags(cfg store.PlainConfig, host store.HostConfig, auth store.AuthConfig, extra []string) ([]string, error) {
	args, err := applySSHCommonArgs(nil, cfg, host, "-p")
	if err != nil {
		return nil, err
	}
	args = appendSendEnvOptions(args, host.Env)
	args, err = applyKeyAuthArg(args, auth)
	if err != nil {
		return nil, err
	}
	args = append(args, extra...)
	return args, nil
}

// buildSSHArgs builds the full SSH argument list including the destination.
// Any extra flags are inserted before the destination.
func buildSSHArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, extra []string) ([]string, error) {
	args, err := buildSSHFlags(cfg, host, auth, extra)
	if err != nil {
		return nil, err
	}
	args = append(args, sshDestination(host, userName))
	return args, nil
}

func buildSCPArgs(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool) ([]string, error) {
	args, err := applySSHCommonArgs(nil, cfg, host, "-P")
	if err != nil {
		return nil, err
	}
	if recursive {
		args = append(args, "-r")
	}
	args, err = applyKeyAuthArg(args, auth)
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
