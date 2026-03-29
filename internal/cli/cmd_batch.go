package cli

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

type batchResult struct {
	skip   bool
	err    error
	stdout []byte
	stderr []byte
}

type batchRunner func(alias string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult

func runBatch(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, parallel int, fn batchRunner) bool {
	results := make([]batchResult, len(aliases))
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
				results[i] = batchResult{skip: true, err: err}
				return
			}
			results[i] = fn(alias, host, userName, auth)
		}(i, alias)
	}
	wg.Wait()

	return printBatchResults(cmd.OutOrStdout(), cmd.ErrOrStderr(), aliases, results)
}

func printBatchResults(out, errOut io.Writer, aliases []string, results []batchResult) bool {
	anyFailed := false
	for i, alias := range aliases {
		r := results[i]
		if r.skip {
			fmt.Fprintf(errOut, "SKIP %s: %v\n", alias, r.err)
			continue
		}
		if len(r.stdout) > 0 || len(r.stderr) > 0 {
			fmt.Fprintf(out, "=== %s ===\n", alias)
			if len(r.stdout) > 0 {
				out.Write(r.stdout)
			}
			if len(r.stderr) > 0 {
				errOut.Write(r.stderr)
			}
		}
		if r.err != nil {
			if len(r.stdout) == 0 && len(r.stderr) == 0 {
				fmt.Fprintf(out, "%-20s  FAIL\n", alias)
			} else {
				fmt.Fprintf(errOut, "FAIL %s: %v\n", alias, r.err)
			}
			anyFailed = true
		} else if len(r.stdout) == 0 && len(r.stderr) == 0 {
			fmt.Fprintf(out, "%-20s  OK\n", alias)
		}
	}
	return anyFailed
}

func runBatchTest(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, timeout, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		return batchResult{err: runSSHTest(host, userName, auth, timeout, agentSocket, agentCapability)}
	})
}

func runBatchExec(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remoteCmd []string, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeRemoteCmd(host, userName, auth, remoteCmd, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{err: err, stdout: outBuf.Bytes(), stderr: errBuf.Bytes()}
	})
}

func runBatchCp(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remotePath string, localPaths []string, recursive bool, parallel int, agentSocket, agentCapability string) bool {
	return runBatch(cmd, cfg, aliases, parallel, func(_ string, host store.HostConfig, userName string, auth store.AuthConfig) batchResult {
		var outBuf, errBuf bytes.Buffer
		err := executeSCP(host, userName, auth, remotePath, localPaths, true, recursive, agentSocket, agentCapability, &outBuf, &errBuf)
		return batchResult{err: err, stdout: outBuf.Bytes(), stderr: errBuf.Bytes()}
	})
}
