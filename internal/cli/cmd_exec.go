package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
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
	if host.Port <= 0 {
		host.Port = 22
	}
	args := []string{"-p", strconv.Itoa(host.Port), "-T"}
	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}

	switch strings.ToLower(auth.Type) {
	case "key":
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
	args = append(args, destination)
	args = append(args, remoteCmd...)

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
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	execCmd.Env = env
	if len(extraFiles) > 0 {
		execCmd.ExtraFiles = extraFiles
	}
	return execCmd.Run()
}
