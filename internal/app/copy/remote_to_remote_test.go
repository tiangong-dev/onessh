package copy

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"onessh/internal/domain"
	appruntime "onessh/internal/runtime"
)

func TestRemoteToRemoteCopySourceMissing(t *testing.T) {
	t.Parallel()

	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           &remoteToRemoteRunner{},
		TempFS:           newRemoteToRemoteTempFS("downloaded.txt"),
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "missing",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("expected source missing error, got %v", err)
	}
}

func TestRemoteToRemoteCopyDestinationMissing(t *testing.T) {
	t.Parallel()

	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           &remoteToRemoteRunner{},
		TempFS:           newRemoteToRemoteTempFS("downloaded.txt"),
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "missing",
		DestinationPath:  "/tmp/",
	})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("expected destination missing error, got %v", err)
	}
}

func TestRemoteToRemoteCopySourceIdentityFail(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("source identity failed")
	runner := &remoteToRemoteRunner{}
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{errByHost: map[string]error{"prod.example.com": wantErr}},
		Runner:           runner,
		TempFS:           newRemoteToRemoteTempFS("downloaded.txt"),
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "resolve source host identity") {
		t.Fatalf("Copy error = %v, want wrapped source identity error", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("runner should not run after source identity error")
	}
}

func TestRemoteToRemoteCopyDestinationIdentityFail(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("destination identity failed")
	runner := &remoteToRemoteRunner{}
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{errByHost: map[string]error{"backup.example.com": wantErr}},
		Runner:           runner,
		TempFS:           newRemoteToRemoteTempFS("downloaded.txt"),
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "resolve destination host identity") {
		t.Fatalf("Copy error = %v, want wrapped destination identity error", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("runner should not run after destination identity error")
	}
}

func TestRemoteToRemoteCopyDownloadFail(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("scp download failed")
	runner := &remoteToRemoteRunner{downloadErr: wantErr}
	tempFS := newRemoteToRemoteTempFS("downloaded.txt")
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           runner,
		TempFS:           tempFS,
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "download from prod failed") {
		t.Fatalf("Copy error = %v, want wrapped download error", err)
	}
	if got := tempFS.removed; !reflect.DeepEqual(got, []string{tempFS.tmpDir}) {
		t.Fatalf("cleanup calls = %#v, want tmp dir removed", got)
	}
	if len(runner.requests) != 1 || runner.requests[0].IsUpload {
		t.Fatalf("expected only download request, got %#v", runner.requests)
	}
}

func TestRemoteToRemoteCopyUploadFail(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("scp upload failed")
	runner := &remoteToRemoteRunner{uploadErr: wantErr}
	tempFS := newRemoteToRemoteTempFS("downloaded.txt")
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           runner,
		TempFS:           tempFS,
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "upload to backup failed") {
		t.Fatalf("Copy error = %v, want wrapped upload error", err)
	}
	if got := tempFS.removed; !reflect.DeepEqual(got, []string{tempFS.tmpDir}) {
		t.Fatalf("cleanup calls = %#v, want tmp dir removed", got)
	}
	if len(runner.requests) != 2 || !runner.requests[1].IsUpload {
		t.Fatalf("expected download and upload requests, got %#v", runner.requests)
	}
}

func TestRemoteToRemoteCopyEmptyDownload(t *testing.T) {
	t.Parallel()

	runner := &remoteToRemoteRunner{}
	tempFS := newRemoteToRemoteTempFS()
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           runner,
		TempFS:           tempFS,
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if err == nil || !strings.Contains(err.Error(), "no files were downloaded from source") {
		t.Fatalf("expected empty download error, got %v", err)
	}
	if got := tempFS.removed; !reflect.DeepEqual(got, []string{tempFS.tmpDir}) {
		t.Fatalf("cleanup calls = %#v, want tmp dir removed", got)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected upload to be skipped, got %#v", runner.requests)
	}
}

func TestRemoteToRemoteCopyTempCleanupAfterSuccess(t *testing.T) {
	t.Parallel()

	tempFS := newRemoteToRemoteTempFS("downloaded.txt")
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           &remoteToRemoteRunner{},
		TempFS:           tempFS,
	}

	_, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
	})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if got := tempFS.removed; !reflect.DeepEqual(got, []string{tempFS.tmpDir}) {
		t.Fatalf("cleanup calls = %#v, want tmp dir removed", got)
	}
}

func TestRemoteToRemoteCopySuccessPath(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	runner := &remoteToRemoteRunner{}
	tempFS := newRemoteToRemoteTempFS("b.log", "a.log")
	service := RemoteToRemoteService{
		IdentityResolver: &remoteToRemoteResolver{},
		Runner:           runner,
		TempFS:           tempFS,
	}

	out, err := service.Copy(context.Background(), RemoteToRemoteInput{
		Config:           remoteToRemoteConfig(),
		SourceAlias:      "prod",
		SourcePath:       "/var/log/app.log",
		DestinationAlias: "backup",
		DestinationPath:  "/tmp/",
		Recursive:        true,
		Agent: AgentConfig{
			Socket:     "/tmp/onessh.sock",
			Capability: "capability",
		},
		IO: appruntime.IOStreams{
			ErrOut: &stderr,
		},
	})
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if out.Source.Alias != "prod" || out.Source.Host != "prod.example.com" || out.Source.UserName != "alice" {
		t.Fatalf("unexpected source output: %#v", out.Source)
	}
	if out.Destination.Alias != "backup" || out.Destination.Host != "backup.example.com" || out.Destination.UserName != "bob" {
		t.Fatalf("unexpected destination output: %#v", out.Destination)
	}
	if len(runner.requests) != 2 {
		t.Fatalf("runner requests = %d, want 2", len(runner.requests))
	}

	downloadReq := runner.requests[0]
	if downloadReq.Alias != "prod" || downloadReq.RemotePath != "/var/log/app.log" || downloadReq.IsUpload {
		t.Fatalf("unexpected download request: %#v", downloadReq)
	}
	if !reflect.DeepEqual(downloadReq.LocalPaths, []string{tempFS.tmpDir + string(filepath.Separator)}) {
		t.Fatalf("download local paths = %#v", downloadReq.LocalPaths)
	}
	if !downloadReq.Recursive || downloadReq.Agent.Socket != "/tmp/onessh.sock" || downloadReq.Stderr != &stderr {
		t.Fatalf("download options not forwarded: %#v", downloadReq)
	}

	uploadReq := runner.requests[1]
	wantUploadPaths := []string{
		filepath.Join(tempFS.tmpDir, "a.log"),
		filepath.Join(tempFS.tmpDir, "b.log"),
	}
	if uploadReq.Alias != "backup" || uploadReq.RemotePath != "/tmp/" || !uploadReq.IsUpload {
		t.Fatalf("unexpected upload request: %#v", uploadReq)
	}
	if !reflect.DeepEqual(uploadReq.LocalPaths, wantUploadPaths) {
		t.Fatalf("upload local paths = %#v, want %#v", uploadReq.LocalPaths, wantUploadPaths)
	}
	if !strings.Contains(stderr.String(), "Downloading from prod (prod.example.com) ...") {
		t.Fatalf("missing download progress in stderr: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Uploading to backup (backup.example.com) ...") {
		t.Fatalf("missing upload progress in stderr: %q", stderr.String())
	}
}

func remoteToRemoteConfig() domain.PlainConfig {
	return domain.PlainConfig{
		Users: map[string]domain.UserConfig{
			"alice": {Name: "alice", Auth: domain.AuthConfig{Type: "key"}},
			"bob":   {Name: "bob", Auth: domain.AuthConfig{Type: "key"}},
		},
		Hosts: map[string]domain.HostConfig{
			"prod": {
				Host:    "prod.example.com",
				UserRef: "alice",
			},
			"backup": {
				Host:    "backup.example.com",
				UserRef: "bob",
			},
		},
	}
}

type remoteToRemoteResolver struct {
	errByHost map[string]error
}

func (r *remoteToRemoteResolver) ResolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error) {
	if err := r.errByHost[host.Host]; err != nil {
		return "", domain.AuthConfig{}, err
	}
	userCfg := cfg.Users[host.UserRef]
	return userCfg.Name, userCfg.Auth, nil
}

type remoteToRemoteRunner struct {
	downloadErr error
	uploadErr   error
	requests    []RemoteTransferRequest
}

func (r *remoteToRemoteRunner) CopyRemote(_ context.Context, req RemoteTransferRequest) error {
	r.requests = append(r.requests, req)
	if req.IsUpload {
		return r.uploadErr
	}
	return r.downloadErr
}

type remoteToRemoteTempFS struct {
	tmpDir  string
	entries []fs.DirEntry
	removed []string
}

func newRemoteToRemoteTempFS(names ...string) *remoteToRemoteTempFS {
	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, remoteToRemoteDirEntry{name: name})
	}
	return &remoteToRemoteTempFS{
		tmpDir:  filepath.Join(string(filepath.Separator), "tmp", "onessh-cp-123"),
		entries: entries,
	}
}

func (f *remoteToRemoteTempFS) MkdirTemp(_, _ string) (string, error) {
	return f.tmpDir, nil
}

func (f *remoteToRemoteTempFS) ReadDir(string) ([]fs.DirEntry, error) {
	return append([]fs.DirEntry{}, f.entries...), nil
}

func (f *remoteToRemoteTempFS) RemoveAll(path string) error {
	f.removed = append(f.removed, path)
	return nil
}

type remoteToRemoteDirEntry struct {
	name string
}

func (e remoteToRemoteDirEntry) Name() string {
	return e.name
}

func (e remoteToRemoteDirEntry) IsDir() bool {
	return false
}

func (e remoteToRemoteDirEntry) Type() fs.FileMode {
	return 0
}

func (e remoteToRemoteDirEntry) Info() (fs.FileInfo, error) {
	return nil, nil
}
