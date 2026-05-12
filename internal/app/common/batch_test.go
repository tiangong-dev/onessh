package common

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"onessh/internal/store"
)

func TestRunBatchPreservesAliasOrderWithParallelExecution(t *testing.T) {
	t.Parallel()

	cfg := testBatchConfig("first", "second", "third")
	resolver := batchResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}}
	started := make(chan string, 3)
	releases := map[string]chan struct{}{
		"first":  make(chan struct{}),
		"second": make(chan struct{}),
		"third":  make(chan struct{}),
	}

	runner := BatchRunnerFunc(func(_ context.Context, req BatchRequest) BatchResult {
		started <- req.Alias
		<-releases[req.Alias]
		return BatchResult{Stdout: []byte(req.Alias + "\n")}
	})

	var progressMu sync.Mutex
	var progress []int
	resultsCh := make(chan []BatchResult, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := RunBatch(context.Background(), BatchInput{
			Config:           cfg,
			Aliases:          []string{"first", "second", "third"},
			Parallel:         3,
			IdentityResolver: resolver,
			Runner:           runner,
			OnProgress: func(completed, total int) {
				if total != 3 {
					t.Errorf("progress total = %d, want 3", total)
				}
				progressMu.Lock()
				defer progressMu.Unlock()
				progress = append(progress, completed)
			},
		})
		errCh <- err
		resultsCh <- results
	}()

	startedSet := map[string]bool{}
	for i := 0; i < 3; i++ {
		startedSet[<-started] = true
	}
	if !reflect.DeepEqual(startedSet, map[string]bool{"first": true, "second": true, "third": true}) {
		t.Fatalf("started aliases = %#v", startedSet)
	}
	close(releases["second"])
	close(releases["third"])
	close(releases["first"])
	if err := <-errCh; err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	results := <-resultsCh

	gotAliases := resultAliases(results)
	if !reflect.DeepEqual(gotAliases, []string{"first", "second", "third"}) {
		t.Fatalf("result aliases = %#v", gotAliases)
	}
	if string(results[0].Stdout) != "first\n" || string(results[1].Stdout) != "second\n" || string(results[2].Stdout) != "third\n" {
		t.Fatalf("unexpected stdout by result: %#v", results)
	}
	progressMu.Lock()
	gotProgress := append([]int{}, progress...)
	progressMu.Unlock()
	if !reflect.DeepEqual(gotProgress, []int{1, 2, 3}) {
		t.Fatalf("progress = %#v, want [1 2 3]", gotProgress)
	}
}

func TestRunBatchParallelLessThanOneRunsSerially(t *testing.T) {
	t.Parallel()

	cfg := testBatchConfig("one", "two", "three")
	resolver := batchResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}}
	started := make(chan string, 3)
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := RunBatch(context.Background(), BatchInput{
			Config:           cfg,
			Aliases:          []string{"one", "two", "three"},
			Parallel:         0,
			IdentityResolver: resolver,
			Runner: BatchRunnerFunc(func(_ context.Context, req BatchRequest) BatchResult {
				started <- req.Alias
				<-release
				return BatchResult{Stdout: []byte(req.Alias)}
			}),
		})
		done <- err
	}()

	<-started
	select {
	case got := <-started:
		t.Fatalf("parallel<=0 should run serially, but %q started before first completed", got)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
}

func TestRunBatchSkipsIdentityResolutionFailures(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("missing identity")
	resolver := batchResolver{
		userName: "alice",
		auth:     store.AuthConfig{Type: "key"},
		errByHost: map[string]error{
			"skip.example.com": wantErr,
		},
	}
	runnerCalls := 0

	results, err := RunBatch(context.Background(), BatchInput{
		Config:           testBatchConfig("ok", "skip"),
		Aliases:          []string{"ok", "skip"},
		Parallel:         2,
		IdentityResolver: resolver,
		Runner: BatchRunnerFunc(func(_ context.Context, req BatchRequest) BatchResult {
			runnerCalls++
			return BatchResult{Stdout: []byte(req.Alias)}
		}),
	})
	if err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	if runnerCalls != 1 {
		t.Fatalf("runner calls = %d, want 1", runnerCalls)
	}
	if results[1].Alias != "skip" || !results[1].Skip || !errors.Is(results[1].Err, wantErr) {
		t.Fatalf("unexpected skip result: %#v", results[1])
	}
}

func TestRunBatchReturnsFailureAndOutputStructure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("ssh failed")
	results, err := RunBatch(context.Background(), BatchInput{
		Config:           testBatchConfig("prod"),
		Aliases:          []string{"prod"},
		Parallel:         1,
		IdentityResolver: batchResolver{userName: "alice", auth: store.AuthConfig{Type: "password", Password: "secret"}},
		Runner: BatchRunnerFunc(func(_ context.Context, req BatchRequest) BatchResult {
			if req.Alias != "prod" || req.Host.Host != "prod.example.com" || req.UserName != "alice" || req.Auth.Type != "password" {
				t.Fatalf("unexpected request: %#v", req)
			}
			return BatchResult{
				Err:    wantErr,
				Stdout: []byte("remote stdout\n"),
				Stderr: []byte("remote stderr\n"),
			}
		}),
	})
	if err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	want := BatchResult{
		Alias:  "prod",
		Host:   testBatchConfig("prod").Hosts["prod"],
		Err:    wantErr,
		Stdout: []byte("remote stdout\n"),
		Stderr: []byte("remote stderr\n"),
	}
	if !reflect.DeepEqual(results, []BatchResult{want}) {
		t.Fatalf("results = %#v, want %#v", results, []BatchResult{want})
	}
}

func TestRunBatchDoesNotWriteStdoutOrStderr(t *testing.T) {
	stdout := captureFileDescriptor(t, &os.Stdout)
	stderr := captureFileDescriptor(t, &os.Stderr)

	_, err := RunBatch(context.Background(), BatchInput{
		Config:           testBatchConfig("prod"),
		Aliases:          []string{"prod"},
		Parallel:         1,
		IdentityResolver: batchResolver{userName: "alice", auth: store.AuthConfig{Type: "key"}},
		Runner: BatchRunnerFunc(func(_ context.Context, _ BatchRequest) BatchResult {
			return BatchResult{
				Stdout: []byte("buffered stdout\n"),
				Stderr: []byte("buffered stderr\n"),
			}
		}),
		OnProgress: func(int, int) {},
	})
	if err != nil {
		t.Fatalf("RunBatch: %v", err)
	}

	if got := stdout(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := stderr(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func testBatchConfig(aliases ...string) store.PlainConfig {
	cfg := store.NewPlainConfig()
	for _, alias := range aliases {
		cfg.Hosts[alias] = store.HostConfig{
			Host:    alias + ".example.com",
			UserRef: "alice",
		}
	}
	cfg.Users["alice"] = store.UserConfig{
		Name: "alice",
		Auth: store.AuthConfig{Type: "key"},
	}
	return cfg
}

func resultAliases(results []BatchResult) []string {
	aliases := make([]string, len(results))
	for i, result := range results {
		aliases[i] = result.Alias
	}
	return aliases
}

type batchResolver struct {
	userName  string
	auth      store.AuthConfig
	errByHost map[string]error
}

func (r batchResolver) ResolveHostIdentity(_ store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error) {
	if err := r.errByHost[host.Host]; err != nil {
		return "", store.AuthConfig{}, err
	}
	return r.userName, r.auth, nil
}

func captureFileDescriptor(t *testing.T, target **os.File) func() string {
	t.Helper()

	original := *target
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	*target = writer

	return func() string {
		if err := writer.Close(); err != nil {
			t.Fatalf("close writer: %v", err)
		}
		*target = original
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(reader); err != nil {
			t.Fatalf("read pipe: %v", err)
		}
		if err := reader.Close(); err != nil {
			t.Fatalf("close reader: %v", err)
		}
		return buf.String()
	}
}
