package common

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"onessh/internal/store"
)

type BatchIdentityResolver interface {
	ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error)
}

type BatchRunner interface {
	RunBatchHost(ctx context.Context, req BatchRequest) BatchResult
}

type BatchRunnerFunc func(context.Context, BatchRequest) BatchResult

func (f BatchRunnerFunc) RunBatchHost(ctx context.Context, req BatchRequest) BatchResult {
	return f(ctx, req)
}

type BatchInput struct {
	Config           store.PlainConfig
	Aliases          []string
	Parallel         int
	IdentityResolver BatchIdentityResolver
	Runner           BatchRunner
	OnProgress       func(completed, total int)
}

type BatchRequest struct {
	Alias    string
	Host     store.HostConfig
	UserName string
	Auth     store.AuthConfig
}

type BatchResult struct {
	Alias  string
	Host   store.HostConfig
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
		wg.Add(1)
		go func(i int, alias string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			host := input.Config.Hosts[alias]
			result := BatchResult{
				Alias: alias,
				Host:  host,
			}
			userName, auth, err := input.IdentityResolver.ResolveHostIdentity(input.Config, host)
			if err != nil {
				result.Skip = true
				result.Err = err
			} else {
				result = input.Runner.RunBatchHost(ctx, BatchRequest{
					Alias:    alias,
					Host:     host,
					UserName: userName,
					Auth:     auth,
				})
				result.Alias = alias
				result.Host = host
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
