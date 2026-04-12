package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func newTestCmd(opts *rootOptions) *cobra.Command {
	var (
		all       bool
		timeout   int
		filterTag string
		filter    string
		dryRun    bool
		parallel  int
	)

	cmd := &cobra.Command{
		Use:   "test [<host-alias>]",
		Short: "Test SSH connectivity to one or all hosts",
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
					printDryRunHosts(cmd.OutOrStdout(), cfg, aliases)
					return nil
				}

				anyFailed := runBatchTest(cmd, cfg, aliases, timeout, parallel, opts.agentSocket, opts.agentCapability)
				if anyFailed {
					return errors.New("one or more hosts failed connectivity check")
				}
				return nil
			}

			if len(args) == 0 {
				return errors.New("specify <host-alias> or use --all/--tag/--filter")
			}
			alias := strings.TrimSpace(args[0])
			target, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q not found", alias)
			}
			userName, auth, err := resolveHostIdentity(cfg, target)
			if err != nil {
				return err
			}
			if testErr := runSSHTest(cfg, target, userName, auth, timeout, opts.agentSocket, opts.agentCapability); testErr != nil {
				opts.logEvent("test", alias, target.Host, userName, "fail", testErr)
				return fmt.Errorf("connectivity check failed: %w", testErr)
			}
			opts.logEvent("test", alias, target.Host, userName, "ok", nil)
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

func runSSHTest(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, timeoutSec int, agentSocket, agentCapability string) error {
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
	defer cleanup()
	return runExternalCommand(binary, args, env, extraFiles, nil, nil, nil)
}
