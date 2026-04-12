package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
					printDryRunHosts(cmd.OutOrStdout(), cfg, aliases)
					fmt.Fprintf(cmd.OutOrStdout(), "Command: %s\n", strings.Join(args, " "))
					return nil
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

			target, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q not found", alias)
			}
			userName, auth, err := resolveHostIdentity(cfg, target)
			if err != nil {
				return err
			}
			execErr := executeRemoteCmd(target, userName, auth, args[1:], opts.agentSocket, opts.agentCapability, nil, nil)
			if execErr != nil {
				opts.logEvent("exec", alias, target.Host, userName, "fail", execErr)
			} else {
				opts.logEvent("exec", alias, target.Host, userName, "ok", nil)
			}
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

func executeRemoteCmd(host store.HostConfig, userName string, auth store.AuthConfig, remoteCmd []string, agentSocket, agentCapability string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	args, err := buildSSHArgs(host, userName, auth, []string{"-T"})
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
