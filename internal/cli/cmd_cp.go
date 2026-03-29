package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

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

				anyFailed := runBatchCp(cmd, cfg, aliases, batchRemotePath, batchLocalPaths, recursive, parallel, opts.agentSocket, opts.agentCapability)
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
					return executeRemoteToRemoteCopy(cfg, args[0], args[1], recursive, opts.agentSocket, opts.agentCapability)
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
			cpErr := executeSCP(target, userName, auth, remotePath, localPaths, isUpload, recursive, opts.agentSocket, opts.agentCapability, nil, nil)
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

func executeRemoteToRemoteCopy(cfg store.PlainConfig, srcArg, dstArg string, recursive bool, agentSocket, agentCapability string) error {
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
	if err := executeSCP(srcHost, srcUser, srcAuth, srcPath, []string{tmpDir + "/"}, false, recursive, agentSocket, agentCapability, nil, nil); err != nil {
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
	if err := executeSCP(dstHost, dstUser, dstAuth, dstPath, localPaths, true, recursive, agentSocket, agentCapability, nil, nil); err != nil {
		return fmt.Errorf("upload to %s failed: %w", dstAlias, err)
	}

	return nil
}

func executeSCP(host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool, agentSocket, agentCapability string, stdout, stderr io.Writer) error {
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
