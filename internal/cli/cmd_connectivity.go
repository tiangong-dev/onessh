package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	connectivityapp "onessh/internal/app/connectivity"
	"onessh/internal/domain"

	"github.com/spf13/cobra"
)

func newPingCmd(opts *rootOptions) *cobra.Command {
	var (
		all       bool
		timeout   int
		filterTag string
		filter    string
		dryRun    bool
		parallel  int
	)

	cmd := &cobra.Command{
		Use:   "ping [<host-alias>]",
		Short: "Check SSH connectivity to one or all hosts",
		Args:  cobra.MaximumNArgs(1),
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
					return printDryRunHosts(cmd.OutOrStdout(), cfg, aliases)
				}

				anyFailed := runBatchPing(cmd, cfg, aliases, timeout, parallel, opts.agentSocket, opts.agentCapability, opts.auditSink())
				if anyFailed {
					return errors.New("one or more hosts failed connectivity check")
				}
				return nil
			}

			if len(args) == 0 {
				return errors.New("specify <host-alias> or use --all/--tag/--filter")
			}
			alias := strings.TrimSpace(args[0])
			service := connectivityapp.Service{
				IdentityResolver: pingIdentityResolver{},
				Runner:           pingRunner{},
				Audit:            opts.auditSink(),
			}
			if _, testErr := service.Ping(cmd.Context(), connectivityapp.Input{
				Config: cfg,
				Alias:  alias,
				Timeout: connectivityapp.TimeoutConfig{
					Seconds: timeout,
				},
				Agent: connectivityapp.AgentConfig{
					Socket:     opts.agentSocket,
					Capability: opts.agentCapability,
				},
			}); testErr != nil {
				return fmt.Errorf("connectivity check failed: %w", testErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✔ %s is reachable\n", alias)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Test all hosts")
	cmd.Flags().IntVar(&timeout, "timeout", 5, "Connection timeout in seconds")
	cmd.Flags().StringVar(&filterTag, "tag", "", "Test hosts matching tag")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter hosts by glob pattern (matches alias, host, description)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show matched hosts without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "Max concurrent operations in batch mode")
	cmd.ValidArgsFunction = completionHostAliases(opts)
	return cmd
}

type pingIdentityResolver struct{}

func (pingIdentityResolver) ResolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

type pingRunner struct{}

func (pingRunner) Ping(ctx context.Context, req connectivityapp.Request) error {
	return runSSHTest(ctx, req.Config, req.Host, req.UserName, req.Auth, req.Timeout.Seconds, req.Agent.Socket, req.Agent.Capability)
}

func runSSHTest(ctx context.Context, cfg domain.PlainConfig, host domain.HostConfig, userName string, auth domain.AuthConfig, timeoutSec int, agentSocket, agentCapability string) (retErr error) {
	args := []string{
		"-o", fmt.Sprintf("ConnectTimeout=%d", timeoutSec),
	}
	if auth.Type == "key" {
		args = append(args, "-o", "BatchMode=yes")
	}
	args, err := buildSSHArgs(cfg, host, userName, auth, args)
	if err != nil {
		return err
	}
	args = append(args, "exit 0")

	binary := "ssh"
	env := os.Environ()
	binary, args, env, extraFiles, cleanup, err := withPasswordAuth(binary, args, auth, env, agentSocket, agentCapability, nil, "ssh")
	if err != nil {
		return err
	}
	defer func() {
		if cerr := cleanup(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()
	return runExternalCommand(ctx, binary, args, env, extraFiles, nil, nil, nil)
}
