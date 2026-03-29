package cli

import (
	"bytes"
	"fmt"
	"sync"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func runBatchTest(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, timeout, parallel int, agentSocket, agentCapability string) bool {
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
			results[i] = result{err: runSSHTest(host, userName, auth, timeout, agentSocket, agentCapability)}
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

func runBatchExec(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remoteCmd []string, parallel int, agentSocket, agentCapability string) bool {
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
			err = executeRemoteCmd(host, userName, auth, remoteCmd, agentSocket, agentCapability, &outBuf, &errBuf)
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

func runBatchCp(cmd *cobra.Command, cfg store.PlainConfig, aliases []string, remotePath string, localPaths []string, recursive bool, parallel int, agentSocket, agentCapability string) bool {
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
			err = executeSCP(host, userName, auth, remotePath, localPaths, true, recursive, agentSocket, agentCapability, &outBuf, &errBuf)
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
