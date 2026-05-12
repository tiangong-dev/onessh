package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	commonapp "onessh/internal/app/common"
	"onessh/internal/store"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type batchResult = commonapp.BatchResult

type batchRunner func(alias string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult

func runBatch(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, parallel int, fn batchRunner) bool {
	total := len(aliases)
	showProgress := term.IsTerminal(int(os.Stderr.Fd()))

	results, err := commonapp.RunBatch(cmd.Context(), commonapp.BatchInput{
		Config:           cfg,
		Aliases:          aliases,
		Parallel:         parallel,
		IdentityResolver: batchIdentityResolver{},
		Runner: commonapp.BatchRunnerFunc(func(_ context.Context, req commonapp.BatchRequest) commonapp.BatchResult {
			return fn(req.Alias, req.Host, req.UserName, req.Auth)
		}),
		OnProgress: func(completed, _ int) {
			if showProgress {
				fmt.Fprintf(os.Stderr, "\r[%d/%d] completed", completed, total)
			}
		},
	})

	if showProgress {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "batch failed: %v\n", err)
		return true
	}

	return printBatchResults(cmd.OutOrStdout(), cmd.ErrOrStderr(), aliases, results)
}

func printBatchResults(out, errOut io.Writer, aliases []string, results []batchResult) bool {
	anyFailed := false
	for i, alias := range aliases {
		r := results[i]
		if r.Skip {
			fmt.Fprintf(errOut, "SKIP %s: %v\n", alias, r.Err)
			continue
		}
		if len(r.Stdout) > 0 || len(r.Stderr) > 0 {
			fmt.Fprintf(out, "=== %s ===\n", alias)
			if len(r.Stdout) > 0 {
				if _, err := out.Write(r.Stdout); err != nil {
					fmt.Fprintf(errOut, "write stdout for %s: %v\n", alias, err)
					anyFailed = true
				}
			}
			if len(r.Stderr) > 0 {
				if _, err := errOut.Write(r.Stderr); err != nil {
					fmt.Fprintf(errOut, "write stderr for %s: %v\n", alias, err)
					anyFailed = true
				}
			}
		}
		if r.Err != nil {
			if len(r.Stdout) == 0 && len(r.Stderr) == 0 {
				fmt.Fprintf(out, "%-20s  FAIL\n", alias)
			} else {
				fmt.Fprintf(errOut, "FAIL %s: %v\n", alias, r.Err)
			}
			anyFailed = true
		} else if len(r.Stdout) == 0 && len(r.Stderr) == 0 {
			fmt.Fprintf(out, "%-20s  OK\n", alias)
		}
	}
	return anyFailed
}

type batchIdentityResolver struct{}

func (batchIdentityResolver) ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

func runBatchPing(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, timeout, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		return batchResult{Err: runSSHTest(cfg, host, userName, auth, timeout, agentSocket, agentCapability)}
	})
}

func runBatchExec(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remoteCmd []string, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeRemoteCmd(cfg, host, userName, auth, remoteCmd, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{Err: err, Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	})
}

func runBatchCp(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remotePath string, localPaths []string, recursive bool, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeSCP(cfg, host, userName, auth, remotePath, localPaths, true, recursive, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{Err: err, Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	})
}
