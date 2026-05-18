package common

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"onessh/internal/domain"
	"onessh/internal/ports"
)

type BatchIdentityResolver interface {
	ResolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error)
}

type BatchRunner interface {
	RunBatchHost(ctx context.Context, req BatchRequest) BatchResult
}

type BatchRunnerFunc func(context.Context, BatchRequest) BatchResult

func (f BatchRunnerFunc) RunBatchHost(ctx context.Context, req BatchRequest) BatchResult {
	return f(ctx, req)
}

type BatchInput struct {
	Config           domain.PlainConfig
	Aliases          []string
	Parallel         int
	AuditAction      string
	Audit            ports.Audit
	IdentityResolver BatchIdentityResolver
	Runner           BatchRunner
	OnProgress       func(completed, total int)
}

type BatchRequest struct {
	Alias    string
	Host     domain.HostConfig
	UserName string
	Auth     domain.AuthConfig
}

type BatchResult struct {
	Alias  string
	Host   domain.HostConfig
	Skip   bool
	Err    error
	Stdout []byte
	Stderr []byte
}

func RunBatch(ctx context.Context, input BatchInput) ([]BatchResult, error) {
	if input.IdentityResolver == nil {
		return nil, errors.New("batch identity resolver is required")
	}
	if input.Runner == nil {
		return nil, errors.New("batch runner is required")
	}

	total := len(input.Aliases)
	results := make([]BatchResult, total)
	parallel := input.Parallel
	if parallel <= 0 {
		parallel = 1
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var completed atomic.Int32

	for i, alias := range input.Aliases {
		// Stop dispatching new work as soon as the caller cancels (e.g. Ctrl-C).
		// Already-running goroutines still finish via wg.Wait so we don't leak,
		// but the underlying ssh/scp processes are killed via exec.CommandContext.
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i int, alias string) {
			defer wg.Done()

			// Honour cancellation while waiting for a worker slot; otherwise a
			// long parallel batch would queue up after ctx is already done.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				result := BatchResult{
					Alias: alias,
					Host:  input.Config.Hosts[alias],
					Skip:  true,
					Err:   ctx.Err(),
				}
				results[i] = result
				n := completed.Add(1)
				if input.OnProgress != nil {
					input.OnProgress(int(n), total)
				}
				return
			}
			defer func() { <-sem }()

			// Re-check after acquiring the slot in case ctx was cancelled while we waited.
			if ctx.Err() != nil {
				result := BatchResult{
					Alias: alias,
					Host:  input.Config.Hosts[alias],
					Skip:  true,
					Err:   ctx.Err(),
				}
				results[i] = result
				n := completed.Add(1)
				if input.OnProgress != nil {
					input.OnProgress(int(n), total)
				}
				return
			}

			host := input.Config.Hosts[alias]
			result := BatchResult{
				Alias: alias,
				Host:  host,
			}
			userName, auth, err := input.IdentityResolver.ResolveHostIdentity(input.Config, host)
			if err != nil {
				result.Skip = true
				result.Err = err
				recordBatchAudit(input, alias, host, "", "skip", err)
			} else {
				result = input.Runner.RunBatchHost(ctx, BatchRequest{
					Alias:    alias,
					Host:     host,
					UserName: userName,
					Auth:     auth,
				})
				result.Alias = alias
				result.Host = host
				recordBatchRunAudit(input, result, userName)
			}
			results[i] = result

			n := completed.Add(1)
			if input.OnProgress != nil {
				input.OnProgress(int(n), total)
			}
		}(i, alias)
	}
	wg.Wait()

	return results, nil
}

func recordBatchRunAudit(input BatchInput, result BatchResult, userName string) {
	status := "ok"
	if result.Skip {
		status = "skip"
	} else if result.Err != nil {
		status = "fail"
	}
	recordBatchAudit(input, result.Alias, result.Host, userName, status, result.Err)
}

func recordBatchAudit(input BatchInput, alias string, host domain.HostConfig, userName, result string, err error) {
	if input.Audit == nil || input.AuditAction == "" {
		return
	}
	RecordAuditStatus(input.Audit, input.AuditAction, alias, host.Host, userName, result, err)
}
