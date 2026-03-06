package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func newConnectCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <host-alias> [-- <ssh-args...>]",
		Short: "Connect to a host alias",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, sshArgs, err := parseConnectInvocation(cmd, args)
			if err != nil {
				return err
			}
			return runConnect(cmd, opts, alias, sshArgs)
		},
	}
	cmd.ValidArgsFunction = completionHostAliases(opts)
	return cmd
}

func newVersionCmd(version, commit, date string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "onessh %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "commit: %s\n", commit)
			fmt.Fprintf(cmd.OutOrStdout(), "date: %s\n", date)
		},
	}
	return cmd
}

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

func runConnect(cmd *cobra.Command, opts *rootOptions, alias string, sshArgs []string) error {
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

	target, exists := cfg.Hosts[alias]
	if !exists {
		return fmt.Errorf("host %q not found", alias)
	}
	userName, auth, err := resolveHostIdentity(cfg, target)
	if err != nil {
		return err
	}

	displayPort := target.Port
	if displayPort <= 0 {
		displayPort = 22
	}
	displayTarget := target.Host
	if userName != "" {
		displayTarget = fmt.Sprintf("%s@%s", userName, target.Host)
	}
	if !opts.quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "Connecting to %s:%d...\n", displayTarget, displayPort)
	}
	connErr := executeSSH(target, userName, auth, sshArgs, cmd.ErrOrStderr(), opts.agentSocket)
	if connErr != nil {
		opts.logEvent("connect", alias, target.Host, userName, "fail", connErr)
	} else {
		opts.logEvent("connect", alias, target.Host, userName, "ok", nil)
	}
	return connErr
}

func executeSSH(
	host store.HostConfig,
	userName string,
	auth store.AuthConfig,
	sshArgs []string,
	errOut io.Writer,
	agentSocket string,
) error {
	args := make([]string, 0, 10+len(sshArgs))
	extraFiles := []*os.File{}
	cleanupExtraFiles := []func(){}

	if host.Port <= 0 {
		host.Port = 22
	}

	args = append(args, "-p", strconv.Itoa(host.Port))

	if host.ProxyJump != "" {
		args = append(args, "-J", host.ProxyJump)
	}
	args = appendSendEnvOptions(args, host.Env)

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

	hookCommand := buildRemoteHookCommand(host.PreConnect, host.PostConnect)
	if hookCommand != "" {
		if containsShortFlag(sshArgs, 'N') {
			return errors.New("pre/post-connect commands are incompatible with -N")
		}
		if containsShortFlag(sshArgs, 'T') {
			return errors.New("pre/post-connect commands are incompatible with -T")
		}
		args = append(args, "-tt")
	}

	args = append(args, sshArgs...)
	args = append(args, destination)
	if hookCommand != "" {
		args = append(args, hookCommand)
	}

	binary := "ssh"
	env := mergeCommandEnv(os.Environ(), host.Env)
	if strings.ToLower(auth.Type) == "password" && auth.Password != "" {
		if _, err := exec.LookPath("sshpass"); err == nil {
			passwordFD, cleanup, err := newPasswordFD(auth.Password)
			if err != nil {
				return err
			}
			defer cleanup()
			extraFiles = append(extraFiles, passwordFD)
			cleanupExtraFiles = append(cleanupExtraFiles, func() { _ = passwordFD.Close() })
			binary = "sshpass"
			args = append([]string{"-d", "3", "ssh"}, args...)
		} else {
			fmt.Fprintln(errOut, "sshpass not found, using SSH_ASKPASS via agent IPC fallback.")
			askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, auth.Password)
			if err != nil {
				return err
			}
			defer cleanup()
			env = append(env, askPassEnv...)
		}
	}

	defer func() {
		for _, cleanup := range cleanupExtraFiles {
			cleanup()
		}
	}()

	execCmd := exec.Command(binary, args...)
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Env = env
	if len(extraFiles) > 0 {
		execCmd.ExtraFiles = extraFiles
	}
	return execCmd.Run()
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

func newPasswordFD(password string) (*os.File, func(), error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create password pipe: %w", err)
	}

	secret := append([]byte(password), '\n')
	defer wipe(secret)

	if _, err := writer.Write(secret); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("write password to pipe: %w", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		return nil, nil, fmt.Errorf("close password pipe writer: %w", err)
	}

	cleanup := func() {
		_ = reader.Close()
	}
	return reader, cleanup, nil
}

func prepareAskPassEnv(agentSocket, password string) ([]string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	socketPath, err := resolveAgentSocketPath(agentSocket)
	if err != nil {
		return nil, nil, err
	}
	token, clearToken, err := registerAskPassToken(socketPath, password, defaultAskPassTTL, defaultAskPassMaxUses)
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
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		_ = os.Remove(scriptPath)
		clearToken()
		return nil, nil, fmt.Errorf("chmod askpass launcher: %w", err)
	}

	env := []string{
		"SSH_ASKPASS=" + scriptPath,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=onessh:0",
		"ONESSH_ASKPASS_EXE=" + exePath,
		"ONESSH_ASKPASS_SOCKET=" + socketPath,
		"ONESSH_ASKPASS_TOKEN=" + token,
	}
	cleanup := func() {
		clearToken()
		_ = os.Remove(scriptPath)
	}
	return env, cleanup, nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func containsShortFlag(args []string, flag rune) bool {
	for _, arg := range args {
		if len(arg) < 2 || arg[0] != '-' || strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.ContainsRune(arg[1:], flag) {
			return true
		}
	}
	return false
}

func newCpCmd(opts *rootOptions) *cobra.Command {
	var (
		recursive bool
		filterTag string
		filter    string
		dryRun    bool
		parallel  int
	)

	cmd := &cobra.Command{
		Use:   "cp <src>... <dst>",
		Short: "Copy files to/from a remote host (alias:path notation)",
		Long: `Copy files between local and remote hosts using scp.

Use alias:path to specify a remote path:
  onessh cp web1:/etc/hosts ./hosts              # download
  onessh cp ./deploy.sh web1:/tmp/               # upload
  onessh cp file1.txt file2.txt web1:/tmp/       # multi-file upload
  onessh cp web1:/var/log/app.log web2:/tmp/     # remote-to-remote
  onessh cp --tag prod deploy.sh :/tmp/          # batch upload to tagged hosts
  onessh cp --filter "web*" app.conf :/etc/app/  # batch upload to filtered hosts`,
		Args: cobra.MinimumNArgs(2),
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

			// Batch upload mode: --tag or --filter
			if filterTag != "" || filter != "" {
				lastArg := args[len(args)-1]
				if !strings.HasPrefix(lastArg, ":") {
					return errors.New("in batch mode, destination must be a remote path (:/path)")
				}
				batchRemotePath := lastArg[1:]
				batchLocalPaths := args[:len(args)-1]
				for _, p := range batchLocalPaths {
					if _, _, ok := splitCpArg(p); ok {
						return errors.New("in batch mode, sources must be local paths")
					}
				}

				aliases := collectFilteredHosts(cfg, filterTag, filter)
				if len(aliases) == 0 {
					return errors.New("no matching hosts found")
				}

				if dryRun {
					printDryRunHosts(cmd.OutOrStdout(), cfg, aliases)
					fmt.Fprintf(cmd.OutOrStdout(), "Upload: %s -> :%s\n", strings.Join(batchLocalPaths, ", "), batchRemotePath)
					return nil
				}

				anyFailed := runBatchCp(cmd, cfg, aliases, batchRemotePath, batchLocalPaths, recursive, parallel, opts.agentSocket)
				if anyFailed {
					return errors.New("one or more hosts failed")
				}
				return nil
			}

			var alias, remotePath string
			var localPaths []string
			var isUpload bool

			if len(args) == 2 {
				_, _, srcRemote := splitCpArg(args[0])
				_, _, dstRemote := splitCpArg(args[1])
				if srcRemote && dstRemote {
					return executeRemoteToRemoteCopy(cfg, args[0], args[1], recursive, opts.agentSocket)
				}

				alias, remotePath, isUpload, err = parseCpArgs(args[0], args[1])
				if err != nil {
					return err
				}
				if isUpload {
					localPaths = []string{args[0]}
				} else {
					localPaths = []string{args[1]}
				}
			} else {
				lastArg := args[len(args)-1]
				dstAlias, dstPath, ok := splitCpArg(lastArg)
				if !ok {
					return errors.New("with multiple sources, the last argument must be a remote path (alias:path)")
				}
				for _, p := range args[:len(args)-1] {
					if _, _, hasAlias := splitCpArg(p); hasAlias {
						return errors.New("with multiple sources, only the last argument can be a remote path")
					}
				}
				alias = dstAlias
				remotePath = dstPath
				localPaths = args[:len(args)-1]
				isUpload = true
			}

			target, exists := cfg.Hosts[alias]
			if !exists {
				return fmt.Errorf("host %q not found", alias)
			}
			userName, auth, err := resolveHostIdentity(cfg, target)
			if err != nil {
				return err
			}
			cpErr := executeSCP(target, userName, auth, remotePath, localPaths, isUpload, recursive, opts.agentSocket, nil, nil)
			if cpErr != nil {
				opts.logEvent("cp", alias, target.Host, userName, "fail", cpErr)
			} else {
				opts.logEvent("cp", alias, target.Host, userName, "ok", nil)
			}
			return cpErr
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively copy directories")
	cmd.Flags().StringVar(&filterTag, "tag", "", "Upload to hosts matching tag (batch mode)")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter hosts by glob pattern (matches alias, host, description)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show matched hosts without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "Max concurrent operations in batch mode")
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 1 || strings.Contains(toComplete, ":") {
			return nil, cobra.ShellCompDirectiveDefault
		}
		aliases, _ := completionHostAliases(opts)(cmd, args, toComplete)
		for i, a := range aliases {
			aliases[i] = a + ":"
		}
		return aliases, cobra.ShellCompDirectiveNoSpace
	}
	return cmd
}

func parseCpArgs(src, dst string) (alias, remotePath string, isUpload bool, err error) {
	srcAlias, srcPath, srcHasAlias := splitCpArg(src)
	dstAlias, dstPath, dstHasAlias := splitCpArg(dst)

	switch {
	case srcHasAlias && dstHasAlias:
		return "", "", false, errors.New("only one side can be a remote path (alias:path)")
	case !srcHasAlias && !dstHasAlias:
		return "", "", false, errors.New("one side must be a remote path (alias:path)")
	case srcHasAlias:
		return srcAlias, srcPath, false, nil // download
	default:
		return dstAlias, dstPath, true, nil // upload
	}
}

func splitCpArg(arg string) (alias, path string, ok bool) {
	idx := strings.Index(arg, ":")
	if idx <= 0 {
		return "", "", false
	}
	return arg[:idx], arg[idx+1:], true
}

func executeRemoteToRemoteCopy(cfg store.PlainConfig, srcArg, dstArg string, recursive bool, agentSocket string) error {
	srcAlias, srcPath, _ := splitCpArg(srcArg)
	dstAlias, dstPath, _ := splitCpArg(dstArg)

	srcHost, ok := cfg.Hosts[srcAlias]
	if !ok {
		return fmt.Errorf("host %q not found", srcAlias)
	}
	dstHost, ok := cfg.Hosts[dstAlias]
	if !ok {
		return fmt.Errorf("host %q not found", dstAlias)
	}

	srcUser, srcAuth, err := resolveHostIdentity(cfg, srcHost)
	if err != nil {
		return fmt.Errorf("resolve source host identity: %w", err)
	}
	dstUser, dstAuth, err := resolveHostIdentity(cfg, dstHost)
	if err != nil {
		return fmt.Errorf("resolve destination host identity: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "onessh-cp-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Step 1: download from source to temp dir
	fmt.Fprintf(os.Stderr, "Downloading from %s (%s) ...\n", srcAlias, srcHost.Host)
	if err := executeSCP(srcHost, srcUser, srcAuth, srcPath, []string{tmpDir + "/"}, false, recursive, agentSocket, nil, nil); err != nil {
		return fmt.Errorf("download from %s failed: %w", srcAlias, err)
	}

	// Collect downloaded files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read temp directory: %w", err)
	}
	if len(entries) == 0 {
		return errors.New("no files were downloaded from source")
	}
	var localPaths []string
	for _, e := range entries {
		localPaths = append(localPaths, filepath.Join(tmpDir, e.Name()))
	}

	// Step 2: upload from temp to destination
	fmt.Fprintf(os.Stderr, "Uploading to %s (%s) ...\n", dstAlias, dstHost.Host)
	if err := executeSCP(dstHost, dstUser, dstAuth, dstPath, localPaths, true, recursive, agentSocket, nil, nil); err != nil {
		return fmt.Errorf("upload to %s failed: %w", dstAlias, err)
	}

	return nil
}

func executeSCP(host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool, agentSocket string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if host.Port <= 0 {
		host.Port = 22
	}
	args := []string{"-P", strconv.Itoa(host.Port)}
	if recursive {
		args = append(args, "-r")
	}
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
	remote := destination + ":" + remotePath
	if isUpload {
		args = append(args, localPaths...)
		args = append(args, remote)
	} else {
		args = append(args, remote, localPaths[0])
	}

	binary := "scp"
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
			args = append([]string{"-d", "3", "scp"}, args...)
		} else {
			askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, auth.Password)
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

				anyFailed := runBatchExec(cmd, cfg, aliases, args, parallel, opts.agentSocket)
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
			execErr := executeRemoteCmd(target, userName, auth, args[1:], opts.agentSocket, nil, nil)
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

func executeRemoteCmd(host store.HostConfig, userName string, auth store.AuthConfig, remoteCmd []string, agentSocket string, stdout, stderr io.Writer) error {
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
			askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, auth.Password)
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

				anyFailed := runBatchTest(cmd, cfg, aliases, timeout, parallel, opts.agentSocket)
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
			if testErr := runSSHTest(target, userName, auth, timeout, opts.agentSocket); testErr != nil {
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

func runBatchTest(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, timeout, parallel int, agentSocket string) bool {
	type result struct {
		skip bool
		err  error
	}
	results := make([]result, len(aliases))
	sem := make(chan struct{}, max(1, parallel))
	var wg sync.WaitGroup
	for i, alias := range aliases {
		wg.Add(1)
		go func(i int, alias string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			host := cfg.Hosts[alias]
			userName, auth, err := resolveHostIdentity(cfg, host)
			if err != nil {
				results[i] = result{skip: true, err: err}
				return
			}
			results[i] = result{err: runSSHTest(host, userName, auth, timeout, agentSocket)}
		}(i, alias)
	}
	wg.Wait()

	anyFailed := false
	out := cmd.OutOrStdout()
	for i, alias := range aliases {
		r := results[i]
		if r.skip {
			fmt.Fprintf(out, "%-20s  SKIP  (%v)\n", alias, r.err)
			continue
		}
		if r.err != nil {
			fmt.Fprintf(out, "%-20s  FAIL\n", alias)
			anyFailed = true
		} else {
			fmt.Fprintf(out, "%-20s  OK\n", alias)
		}
	}
	return anyFailed
}

func runBatchExec(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remoteCmd []string, parallel int, agentSocket string) bool {
	type result struct {
		skip   bool
		err    error
		stdout []byte
		stderr []byte
	}
	results := make([]result, len(aliases))
	sem := make(chan struct{}, max(1, parallel))
	var wg sync.WaitGroup
	for i, alias := range aliases {
		wg.Add(1)
		go func(i int, alias string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			host := cfg.Hosts[alias]
			userName, auth, err := resolveHostIdentity(cfg, host)
			if err != nil {
				results[i] = result{skip: true, err: err}
				return
			}
			var outBuf, errBuf bytes.Buffer
			err = executeRemoteCmd(host, userName, auth, remoteCmd, agentSocket, &outBuf, &errBuf)
			results[i] = result{err: err, stdout: outBuf.Bytes(), stderr: errBuf.Bytes()}
		}(i, alias)
	}
	wg.Wait()

	anyFailed := false
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	for i, alias := range aliases {
		r := results[i]
		if r.skip {
			fmt.Fprintf(errOut, "SKIP %s: %v\n", alias, r.err)
			continue
		}
		fmt.Fprintf(out, "=== %s ===\n", alias)
		if len(r.stdout) > 0 {
			out.Write(r.stdout)
		}
		if len(r.stderr) > 0 {
			errOut.Write(r.stderr)
		}
		if r.err != nil {
			fmt.Fprintf(errOut, "FAIL %s: %v\n", alias, r.err)
			anyFailed = true
		}
	}
	return anyFailed
}

func runBatchCp(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remotePath string, localPaths []string, recursive bool, parallel int, agentSocket string) bool {
	type result struct {
		skip   bool
		err    error
		stdout []byte
		stderr []byte
	}
	results := make([]result, len(aliases))
	sem := make(chan struct{}, max(1, parallel))
	var wg sync.WaitGroup
	for i, alias := range aliases {
		wg.Add(1)
		go func(i int, alias string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			host := cfg.Hosts[alias]
			userName, auth, err := resolveHostIdentity(cfg, host)
			if err != nil {
				results[i] = result{skip: true, err: err}
				return
			}
			var outBuf, errBuf bytes.Buffer
			err = executeSCP(host, userName, auth, remotePath, localPaths, true, recursive, agentSocket, &outBuf, &errBuf)
			results[i] = result{err: err, stdout: outBuf.Bytes(), stderr: errBuf.Bytes()}
		}(i, alias)
	}
	wg.Wait()

	anyFailed := false
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	for i, alias := range aliases {
		r := results[i]
		if r.skip {
			fmt.Fprintf(errOut, "SKIP %s: %v\n", alias, r.err)
			continue
		}
		fmt.Fprintf(out, "=== %s ===\n", alias)
		if len(r.stdout) > 0 {
			out.Write(r.stdout)
		}
		if len(r.stderr) > 0 {
			errOut.Write(r.stderr)
		}
		if r.err != nil {
			fmt.Fprintf(errOut, "FAIL %s: %v\n", alias, r.err)
			anyFailed = true
		}
	}
	return anyFailed
}

func runSSHTest(host store.HostConfig, userName string, auth store.AuthConfig, timeoutSec int, agentSocket string) error {
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
			askPassEnv, cleanup, err := prepareAskPassEnv(agentSocket, auth.Password)
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
