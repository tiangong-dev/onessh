package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	copyapp "onessh/internal/app/copy"
	"onessh/internal/presenters"
	appruntime "onessh/internal/runtime"
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
					if err := printDryRunHosts(cmd.OutOrStdout(), cfg, aliases); err != nil {
						return err
					}
					return presenters.RenderDryRunUpload(cmd.OutOrStdout(), batchLocalPaths, batchRemotePath)
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
					return executeRemoteToRemoteCopy(cmd.Context(), cfg, args[0], args[1], recursive, opts.agentSocket, opts.agentCapability, appruntime.IOStreams{
						Out:    cmd.OutOrStdout(),
						ErrOut: cmd.ErrOrStderr(),
					})
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

			service := copyapp.Service{
				IdentityResolver: copyIdentityResolver{},
				Runner:           copyRunner{},
				Audit:            opts.auditSink(),
			}
			_, cpErr := service.Copy(cmd.Context(), copyapp.Input{
				Config:     cfg,
				Alias:      alias,
				RemotePath: remotePath,
				LocalPaths: localPaths,
				IsUpload:   isUpload,
				Recursive:  recursive,
				Agent: copyapp.AgentConfig{
					Socket:     opts.agentSocket,
					Capability: opts.agentCapability,
				},
				IO: appruntime.IOStreams{},
			})
			return cpErr
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursively copy directories")
	cmd.Flags().StringVar(&filterTag, "tag", "", "Upload to hosts matching tag")
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

type copyIdentityResolver struct{}

func (copyIdentityResolver) ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

type copyRunner struct{}

func (copyRunner) CopyRemote(_ context.Context, req copyapp.Request) error {
	return executeSCP(req.Config, req.Host, req.UserName, req.Auth, req.RemotePath, req.LocalPaths, req.IsUpload, req.Recursive, req.Agent.Socket, req.Agent.Capability, req.Stdout, req.Stderr)
}

type remoteToRemoteCopyRunner struct{}

func (remoteToRemoteCopyRunner) CopyRemote(_ context.Context, req copyapp.RemoteTransferRequest) error {
	return executeSCP(req.Config, req.Host, req.UserName, req.Auth, req.RemotePath, req.LocalPaths, req.IsUpload, req.Recursive, req.Agent.Socket, req.Agent.Capability, req.Stdout, req.Stderr)
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

func executeRemoteToRemoteCopy(ctx context.Context, cfg store.PlainConfig, srcArg, dstArg string, recursive bool, agentSocket, agentCapability string, ioStreams appruntime.IOStreams) error {
	srcAlias, srcPath, _ := splitCpArg(srcArg)
	dstAlias, dstPath, _ := splitCpArg(dstArg)

	service := copyapp.RemoteToRemoteService{
		IdentityResolver: copyIdentityResolver{},
		Runner:           remoteToRemoteCopyRunner{},
		TempFS:           copyapp.OSTempFilesystem{},
	}
	_, err := service.Copy(ctx, copyapp.RemoteToRemoteInput{
		Config:           cfg,
		SourceAlias:      srcAlias,
		SourcePath:       srcPath,
		DestinationAlias: dstAlias,
		DestinationPath:  dstPath,
		Recursive:        recursive,
		Agent: copyapp.AgentConfig{
			Socket:     agentSocket,
			Capability: agentCapability,
		},
		IO: ioStreams,
	})
	return err
}

func executeSCP(cfg store.PlainConfig, host store.HostConfig, userName string, auth store.AuthConfig, remotePath string, localPaths []string, isUpload, recursive bool, agentSocket, agentCapability string, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	args, err := buildSCPArgs(cfg, host, userName, auth, remotePath, localPaths, isUpload, recursive)
	if err != nil {
		return err
	}

	binary := "scp"
	env := os.Environ()
	binary, args, env, extraFiles, cleanup, err := withPasswordAuth(binary, args, auth, env, agentSocket, agentCapability, nil, "scp")
	if err != nil {
		return err
	}
	defer cleanup()
	return runExternalCommand(binary, args, env, extraFiles, os.Stdin, stdout, stderr)
}
