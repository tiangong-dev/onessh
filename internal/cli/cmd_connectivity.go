package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
			if testErr := runSSHTest(target, userName, auth, timeout, opts.agentSocket, opts.agentCapability); testErr != nil {
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

func runSSHTest(host store.HostConfig, userName string, auth store.AuthConfig, timeoutSec int, agentSocket, agentCapability string) error {
	if host.Port <= 0 {
		host.Port = 22
	}
	args := []string{
		"-o", fmt.Sprintf("ConnectTimeout=%d", timeoutSec),
		"-p", strconv.Itoa(host.Port),
	}
	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}

	switch strings.ToLower(auth.Type) {
	case "key":
		args = append(args, "-o", "BatchMode=yes")
		if auth.KeyPath != "" {
			keyPath, err := expandTilde(auth.KeyPath)
			if err != nil {
				return err
			}
			args = append(args, "-i", keyPath)
		}
	case "password":
	default:
		return fmt.Errorf("unsupported auth type: %s", auth.Type)
	}

	destination := host.Host
	if userName != "" {
		destination = fmt.Sprintf("%s@%s", userName, host.Host)
	}
	args = append(args, destination, "exit 0")

	binary := "ssh"
	env := os.Environ()
	var extraFiles []*os.File

	if strings.ToLower(auth.Type) == "password" && auth.Password != "" {
		if _, err := exec.LookPath("sshpass"); err == nil {
			fd, cleanup, err := newPasswordFD(auth.Password)
			if err != nil {
				return err
			}
			defer cleanup()
			extraFiles = append(extraFiles, fd)
			binary = "sshpass"
			args = append([]string{"-d", "3", "ssh"}, args...)
		} else {
			askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, agentCapability, auth.Password)
			if err != nil {
				return err
			}
			defer cleanup()
			env = append(env, askPassEnv...)
		}
	}

	execCmd := exec.Command(binary, args...)
	execCmd.Env = env
	if len(extraFiles) > 0 {
		execCmd.ExtraFiles = extraFiles
	}
	return execCmd.Run()
}
