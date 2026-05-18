package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	connectapp "onessh/internal/app/connect"
	"onessh/internal/domain"
	infraagent "onessh/internal/infra/agent"
	appruntime "onessh/internal/runtime"

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
		Audit:            opts.auditSink(),
	}
	_, connErr := service.Connect(cmd.Context(), connectapp.Input{
		Config:            cfg,
		Alias:             alias,
		SSHArgs:           sshArgs,
		ProxyJumpOverride: proxyJumpOverride,
		ProxyJumpChanged:  proxyJumpChanged,
		Quiet:             opts.quiet,
		Agent: connectapp.AgentConfig{
			Socket:     opts.agentSocket,
			Capability: opts.agentCapability,
		},
		IO: appruntime.IOStreams{
			In:     cmd.InOrStdin(),
			Out:    cmd.OutOrStdout(),
			ErrOut: cmd.ErrOrStderr(),
		},
	})
	return connErr
}

type connectIdentityResolver struct{}

func (connectIdentityResolver) ResolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

type connectTransport struct{}

func (connectTransport) Connect(ctx context.Context, req connectapp.TransportRequest) error {
	return executeSSH(ctx, req.Config, req.Host, req.UserName, req.Auth, req.SSHArgs, req.Stdin, req.Stdout, req.ErrOut, req.Agent.Socket, req.Agent.Capability)
}

func executeSSH(
	ctx context.Context,
	cfg domain.PlainConfig,
	host domain.HostConfig,
	userName string,
	auth domain.AuthConfig,
	sshArgs []string,
	stdin io.Reader,
	stdout io.Writer,
	errOut io.Writer,
	agentSocket string,
	agentCapability string,
) (retErr error) {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
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
	defer func() {
		if cerr := cleanup(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()
	return runExternalCommand(ctx, binary, args, env, extraFiles, stdin, stdout, errOut)
}

func buildRemoteHookCommand(preConnect, postConnect []string) string {
	preparedPre := sanitizeHookCommands(preConnect)
	preparedPost := sanitizeHookCommands(postConnect)
	if len(preparedPre) == 0 && len(preparedPost) == 0 {
		return ""
	}

	// set -e protects PreConnect commands (any failure aborts before opening the shell).
	// We disable it around the interactive shell so that a non-zero exit code from the
	// user's shell does not skip PostConnect — `set -e` would otherwise terminate the
	// outer script at the `${SHELL} -i` line and never reach PostConnect.
	lines := make([]string, 0, len(preparedPre)+len(preparedPost)+7)
	lines = append(lines, "set -e")
	lines = append(lines, preparedPre...)
	lines = append(lines, "set +e")
	lines = append(lines, "${SHELL:-/bin/sh} -i")
	lines = append(lines, "onessh_status=$?")
	lines = append(lines, "set -e")
	lines = append(lines, preparedPost...)
	lines = append(lines, "exit $onessh_status")

	script := strings.Join(lines, "\n")
	return "sh -lc " + shellSingleQuote(script)
}

func prepareAskPassEnv(agentSocket, agentCapability, password string) ([]string, func() error, error) {
	return infraagent.PrepareAskPassEnv(agentSocket, agentCapability, password)
}
