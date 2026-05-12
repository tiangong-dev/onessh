package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	connectapp "onessh/internal/app/connect"
	appruntime "onessh/internal/runtime"
	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func parseConnectInvocation(cmd *cobra.Command, args []string) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, errors.New("host alias cannot be empty")
	}

	alias := strings.TrimSpace(args[0])
	if alias == "" {
		return "", nil, errors.New("host alias cannot be empty")
	}

	var sshArgs []string
	if len(args) > 1 {
		sshArgs = append(sshArgs, args[1:]...)
	}
	if dashAt := cmd.ArgsLenAtDash(); dashAt >= 0 {
		if dashAt >= len(args) {
			sshArgs = nil
		} else {
			sshArgs = append([]string{}, args[dashAt:]...)
		}
	}

	return alias, sshArgs, nil
}

func runConnect(cmd *cobra.Command, opts *rootOptions, alias string, sshArgs []string, proxyJumpOverride string, proxyJumpChanged bool) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("host alias cannot be empty")
	}

	repo, err := opts.repository()
	if err != nil {
		return err
	}

	cfg, pass, err := loadConfig(opts, repo)
	if err != nil {
		return err
	}
	defer wipe(pass)

	service := connectapp.Service{
		IdentityResolver: connectIdentityResolver{},
		Transport:        connectTransport{},
	}
	out, connErr := service.Connect(cmd.Context(), connectapp.Input{
		Config:            cfg,
		Alias:             alias,
		SSHArgs:           sshArgs,
		ProxyJumpOverride: proxyJumpOverride,
		ProxyJumpChanged:  proxyJumpChanged,
		Quiet:             opts.quiet,
		AgentSocket:       opts.agentSocket,
		AgentCapability:   opts.agentCapability,
		IO: appruntime.IOStreams{
			ErrOut: cmd.ErrOrStderr(),
		},
	})
	if connErr != nil {
		if out.Host != "" {
			opts.logEvent("connect", out.Alias, out.Host, out.UserName, "fail", connErr)
		}
	} else {
		opts.logEvent("connect", out.Alias, out.Host, out.UserName, "ok", nil)
	}
	return connErr
}

type connectIdentityResolver struct{}

func (connectIdentityResolver) ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

type connectTransport struct{}

func (connectTransport) Connect(_ context.Context, req connectapp.TransportRequest) error {
	return executeSSH(req.Config, req.Host, req.UserName, req.Auth, req.SSHArgs, req.ErrOut, req.AgentSocket, req.AgentCapability)
}

func executeSSH(
	cfg store.PlainConfig,
	host store.HostConfig,
	userName string,
	auth store.AuthConfig,
	sshArgs []string,
	errOut io.Writer,
	agentSocket string,
	agentCapability string,
) error {
	hookCommand := buildRemoteHookCommand(host.PreConnect, host.PostConnect)

	var extraFlags []string
	if hookCommand != "" {
		if containsShortFlag(sshArgs, 'N') {
			return errors.New("pre/post-connect commands are incompatible with -N")
		}
		if containsShortFlag(sshArgs, 'T') {
			return errors.New("pre/post-connect commands are incompatible with -T")
		}
		extraFlags = append(extraFlags, "-tt")
	}
	extraFlags = append(extraFlags, sshArgs...)

	args, err := buildSSHArgs(cfg, host, userName, auth, extraFlags)
	if err != nil {
		return err
	}
	if hookCommand != "" {
		args = append(args, hookCommand)
	}

	binary := "ssh"
	env := mergeCommandEnv(os.Environ(), host.Env)
	binary, args, env, extraFiles, cleanup, err := withPasswordAuth(binary, args, auth, env, agentSocket, agentCapability, errOut, "ssh")
	if err != nil {
		return err
	}
	defer cleanup()
	return runExternalCommand(binary, args, env, extraFiles, os.Stdin, os.Stdout, os.Stderr)
}

func buildRemoteHookCommand(preConnect, postConnect []string) string {
	preparedPre := sanitizeHookCommands(preConnect)
	preparedPost := sanitizeHookCommands(postConnect)
	if len(preparedPre) == 0 && len(preparedPost) == 0 {
		return ""
	}

	lines := make([]string, 0, len(preparedPre)+len(preparedPost)+5)
	lines = append(lines, "set -e")
	lines = append(lines, preparedPre...)
	lines = append(lines, "${SHELL:-/bin/sh} -i")
	lines = append(lines, "onessh_status=$?")
	lines = append(lines, preparedPost...)
	lines = append(lines, "exit $onessh_status")

	script := strings.Join(lines, "\n")
	return "sh -lc " + shellSingleQuote(script)
}

func prepareAskPassEnv(agentSocket, agentCapability, password string) ([]string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	socketPath, err := resolveAgentSocketPath(agentSocket)
	if err != nil {
		return nil, nil, err
	}
	token, clearToken, err := registerAskPassToken(socketPath, password, defaultAskPassTTL, defaultAskPassMaxUses, agentCapability)
	if err != nil {
		return nil, nil, err
	}

	exePath, err := os.Executable()
	if err != nil {
		clearToken()
		return nil, nil, fmt.Errorf("resolve executable path: %w", err)
	}

	scriptFile, err := os.CreateTemp("", "onessh-askpass-*.sh")
	if err != nil {
		clearToken()
		return nil, nil, fmt.Errorf("create askpass launcher: %w", err)
	}
	scriptPath := scriptFile.Name()

	launcher := "#!/bin/sh\nexec \"$ONESSH_ASKPASS_EXE\" askpass --socket \"$ONESSH_ASKPASS_SOCKET\" --token \"$ONESSH_ASKPASS_TOKEN\"\n"
	if _, err := scriptFile.WriteString(launcher); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(scriptPath)
		clearToken()
		return nil, nil, fmt.Errorf("write askpass launcher: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(scriptPath)
		clearToken()
		return nil, nil, fmt.Errorf("close askpass launcher: %w", err)
	}
	// #nosec G302 -- SSH_ASKPASS helper must be owner-executable.
	if err := os.Chmod(scriptPath, 0o500); err != nil {
		_ = os.Remove(scriptPath)
		clearToken()
		return nil, nil, fmt.Errorf("chmod askpass launcher: %w", err)
	}

	capabilityValue := resolveAgentCapability(agentCapability)
	env := []string{
		"SSH_ASKPASS=" + scriptPath,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=onessh:0",
		"ONESSH_ASKPASS_EXE=" + exePath,
		"ONESSH_ASKPASS_SOCKET=" + socketPath,
		"ONESSH_ASKPASS_TOKEN=" + token,
	}
	if capabilityValue != "" {
		env = append(env, "ONESSH_ASKPASS_CAPABILITY="+capabilityValue)
	}
	cleanup := func() {
		clearToken()
		_ = os.Remove(scriptPath)
	}
	return env, cleanup, nil
}
