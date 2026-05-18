package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	commonapp "onessh/internal/app/common"
	"onessh/internal/ports"
	"onessh/internal/presenters"
	"onessh/internal/store"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type batchResult = commonapp.BatchResult

type batchRunner func(alias string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult

func runBatch(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, parallel int, auditAction string, audit ports.Audit, fn batchRunner) bool {
	total := len(aliases)
	progressOut := cmd.ErrOrStderr()
	progressFile, hasProgressFile := progressOut.(*os.File)
	showProgress := hasProgressFile && term.IsTerminal(int(progressFile.Fd()))

	results, err := commonapp.RunBatch(cmd.Context(), commonapp.BatchInput{
		Config:           cfg,
		Aliases:          aliases,
		Parallel:         parallel,
		AuditAction:      auditAction,
		Audit:            audit,
		IdentityResolver: batchIdentityResolver{},
		Runner: commonapp.BatchRunnerFunc(func(_ context.Context, req commonapp.BatchRequest) commonapp.BatchResult {
			return fn(req.Alias, req.Host, req.UserName, req.Auth)
		}),
		OnProgress: func(completed, _ int) {
			if showProgress {
				fmt.Fprintf(progressOut, "\r[%d/%d] completed", completed, total)
			}
		},
	})

	if showProgress {
		fmt.Fprint(progressOut, "\r\033[K")
	}
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "batch failed: %v\n", err)
		return true
	}

	return printBatchResults(cmd.OutOrStdout(), cmd.ErrOrStderr(), aliases, results)
}

func printBatchResults(out, errOut io.Writer, aliases []string, results []batchResult) bool {
	rows := make([]presenters.BatchResult, len(results))
	for i, result := range results {
		rows[i] = presenters.BatchResult{
			Alias:  result.Alias,
			Skip:   result.Skip,
			Err:    result.Err,
			Stdout: result.Stdout,
			Stderr: result.Stderr,
		}
	}
	return presenters.RenderBatchResults(out, errOut, aliases, rows)
}

type batchIdentityResolver struct{}

func (batchIdentityResolver) ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	return resolveHostIdentity(cfg, host)
}

func runBatchPing(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, timeout, parallel int, agentSocket, agentCapability string, audit ports.Audit) bool {
	return runBatch(cmd, cfg, aliases, parallel, "ping", audit, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		return batchResult{Err: runSSHTest(cfg, host, userName, auth, timeout, agentSocket, agentCapability)}
	})
}

func runBatchExec(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remoteCmd []string, parallel int, agentSocket, agentCapability string, audit ports.Audit) bool {
	return runBatch(cmd, cfg, aliases, parallel, "exec", audit, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeRemoteCmd(cfg, host, userName, auth, remoteCmd, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{Err: err, Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	})
}

func runBatchCp(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remotePath string, localPaths []string, recursive bool, parallel int, agentSocket, agentCapability string, audit ports.Audit) bool {
	return runBatch(cmd, cfg, aliases, parallel, "cp", audit, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeSCP(cfg, host, userName, auth, remotePath, localPaths, true, recursive, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{Err: err, Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}
	})
}
