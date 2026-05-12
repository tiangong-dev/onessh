package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	execapp "onessh/internal/app/exec"
	"onessh/internal/presenters"
	appruntime "onessh/internal/runtime"
	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func newExecCmd(opts *rootOptions) *cobra.Command {
	var (
		all       bool
		filterTag string
		filter    string
		dryRun    bool
		parallel  int
	)

	cmd := &cobra.Command{
		Use:   "exec <host-alias> <command> [args...]",
		Short: "Run a command on a remote host non-interactively",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}
			cfg, pass, err := loadConfig(opts, repo)
			if err != nil {
				return err
			}
			defer wipe(pass)

			if all || filterTag != "" || filter != "" {
				aliases := collectFilteredHosts(cfg, filterTag, filter)
				if len(aliases) == 0 {
					return errors.New("no matching hosts found")
				}

				if dryRun {
					if err := printDryRunHosts(cmd.OutOrStdout(), cfg, aliases); err != nil {
						return err
					}
					return presenters.RenderDryRunCommand(cmd.OutOrStdout(), args)
				}

				anyFailed := runBatchExec(cmd, cfg, aliases, args, parallel, opts.agentSocket, opts.agentCapability)
				if anyFailed {
					return errors.New("one or more hosts failed")
				}
				return nil
			}

			if len(args) < 2 {
				return errors.New("usage: onessh exec <host-alias> <command> [args...]")
			}
			alias := strings.TrimSpace(args[0])
			if alias == "" {
				return errors.New("host alias cannot be empty")
			}

			service := execapp.Service{
				IdentityResolver: execIdentityResolver{},
				Runner:           execRemoteRunner{},
				Audit:            opts.auditSink(),
			}
			_, execErr := service.Exec(cmd.Context(), execapp.Input{
				Config:    cfg,
				Alias:     alias,
				RemoteCmd: args[1:],
				Agent: execapp.AgentConfig{
					Socket:     opts.agentSocket,
					Capability: opts.agentCapability,
				},
				IO: appruntime.IOStreams{
					Out:    cmd.OutOrStdout(),
					ErrOut: cmd.ErrOrStderr(),
				},
			})
			return execErr
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Run command on all hosts")
	cmd.Flags().StringVar(&filterTag, "tag", "", "Run command on hosts matching tag")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter hosts by glob pattern (matches alias, host, description)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show matched hosts without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "Max concurrent operations in batch mode")
	cmd.ValidArgsFunction = completionHostAliases(opts)
	return cmd
}

type execIdentityResolver struct{}

func (execIdentityResolver) ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

type execRemoteRunner struct{}

func (execRemoteRunner) ExecRemote(_ context.Context, req execapp.RemoteRequest) error {
	return executeRemoteCmd(req.Config, req.Host, req.UserName, req.Auth, req.RemoteCmd, req.Agent.Socket, req.Agent.Capability, req.Stdout, req.Stderr)
}

func executeRemoteCmd(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, remoteCmd []string, agentSocket, agentCapability string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	args, err := buildSSHArgs(cfg, host, userName, auth, []string{"-T"})
	if err != nil {
		return err
	}
	args = append(args, remoteCmd...)

	binary := "ssh"
	env := os.Environ()
	binary, args, env, extraFiles, cleanup, err := withPasswordAuth(binary, args, auth, env, agentSocket, agentCapability, nil, "ssh")
	if err != nil {
		return err
	}
	defer cleanup()
	return runExternalCommand(binary, args, env, extraFiles, os.Stdin, stdout, stderr)
}
