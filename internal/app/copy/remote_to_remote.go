package copy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"onessh/internal/domain"
	appruntime "onessh/internal/runtime"
	"onessh/internal/store"
)

type RemoteToRemoteRunner interface {
	CopyRemote(ctx context.Context, req RemoteTransferRequest) error
}

type TempFilesystem interface {
	MkdirTemp(dir, pattern string) (string, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	RemoveAll(path string) error
}

type OSTempFilesystem struct{}

func (OSTempFilesystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (OSTempFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (OSTempFilesystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

type RemoteToRemoteService struct {
	IdentityResolver IdentityResolver
	Runner           RemoteToRemoteRunner
	TempFS           TempFilesystem
}

type RemoteToRemoteInput struct {
	Config           store.PlainConfig
	SourceAlias      string
	SourcePath       string
	DestinationAlias string
	DestinationPath  string
	Recursive        bool
	Agent            AgentConfig
	IO               appruntime.IOStreams
}

type RemoteEndpointOutput struct {
	Alias         string
	Host          string
	UserName      string
	DisplayTarget string
	Port          int
}

type RemoteToRemoteOutput struct {
	Source      RemoteEndpointOutput
	Destination RemoteEndpointOutput
}

type RemoteTransferRequest struct {
	Alias      string
	Config     store.PlainConfig
	Host       store.HostConfig
	UserName   string
	Auth       store.AuthConfig
	RemotePath string
	LocalPaths []string
	IsUpload   bool
	Recursive  bool
	Agent      AgentConfig
	Stdout     io.Writer
	Stderr     io.Writer
}

func (s RemoteToRemoteService) Copy(ctx context.Context, input RemoteToRemoteInput) (RemoteToRemoteOutput, error) {
	sourceAlias := strings.TrimSpace(input.SourceAlias)
	if sourceAlias == "" {
		return RemoteToRemoteOutput{}, errors.New("source host alias cannot be empty")
	}
	destinationAlias := strings.TrimSpace(input.DestinationAlias)
	if destinationAlias == "" {
		return RemoteToRemoteOutput{}, errors.New("destination host alias cannot be empty")
	}
	if s.IdentityResolver == nil {
		return RemoteToRemoteOutput{}, errors.New("remote-to-remote copy identity resolver is required")
	}
	if s.Runner == nil {
		return RemoteToRemoteOutput{}, errors.New("remote-to-remote copy runner is required")
	}
	tempFS := s.TempFS
	if tempFS == nil {
		tempFS = OSTempFilesystem{}
	}

	sourceHost, ok := input.Config.Hosts[sourceAlias]
	if !ok {
		return RemoteToRemoteOutput{}, fmt.Errorf("host %q not found", sourceAlias)
	}
	destinationHost, ok := input.Config.Hosts[destinationAlias]
	if !ok {
		return RemoteToRemoteOutput{}, fmt.Errorf("host %q not found", destinationAlias)
	}

	sourceUser, sourceAuth, err := s.IdentityResolver.ResolveHostIdentity(input.Config, sourceHost)
	if err != nil {
		return RemoteToRemoteOutput{}, fmt.Errorf("resolve source host identity: %w", err)
	}
	destinationUser, destinationAuth, err := s.IdentityResolver.ResolveHostIdentity(input.Config, destinationHost)
	if err != nil {
		return RemoteToRemoteOutput{}, fmt.Errorf("resolve destination host identity: %w", err)
	}

	out := RemoteToRemoteOutput{
		Source:      endpointOutput(sourceAlias, sourceHost, sourceUser),
		Destination: endpointOutput(destinationAlias, destinationHost, destinationUser),
	}

	tmpDir, err := tempFS.MkdirTemp("", "onessh-cp-*")
	if err != nil {
		return out, fmt.Errorf("create temp directory: %w", err)
	}
	defer func() {
		_ = tempFS.RemoveAll(tmpDir)
	}()

	if input.IO.ErrOut != nil {
		fmt.Fprintf(input.IO.ErrOut, "Downloading from %s (%s) ...\n", sourceAlias, sourceHost.Host)
	}
	if err := s.Runner.CopyRemote(ctx, RemoteTransferRequest{
		Alias:      sourceAlias,
		Config:     input.Config,
		Host:       sourceHost,
		UserName:   sourceUser,
		Auth:       sourceAuth,
		RemotePath: input.SourcePath,
		LocalPaths: []string{withTrailingSeparator(tmpDir)},
		IsUpload:   false,
		Recursive:  input.Recursive,
		Agent:      input.Agent,
		Stdout:     input.IO.Out,
		Stderr:     input.IO.ErrOut,
	}); err != nil {
		return out, fmt.Errorf("download from %s failed: %w", sourceAlias, err)
	}

	entries, err := tempFS.ReadDir(tmpDir)
	if err != nil {
		return out, fmt.Errorf("read temp directory: %w", err)
	}
	if len(entries) == 0 {
		return out, errors.New("no files were downloaded from source")
	}

	localPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		localPaths = append(localPaths, filepath.Join(tmpDir, entry.Name()))
	}
	sort.Strings(localPaths)

	if input.IO.ErrOut != nil {
		fmt.Fprintf(input.IO.ErrOut, "Uploading to %s (%s) ...\n", destinationAlias, destinationHost.Host)
	}
	if err := s.Runner.CopyRemote(ctx, RemoteTransferRequest{
		Alias:      destinationAlias,
		Config:     input.Config,
		Host:       destinationHost,
		UserName:   destinationUser,
		Auth:       destinationAuth,
		RemotePath: input.DestinationPath,
		LocalPaths: localPaths,
		IsUpload:   true,
		Recursive:  input.Recursive,
		Agent:      input.Agent,
		Stdout:     input.IO.Out,
		Stderr:     input.IO.ErrOut,
	}); err != nil {
		return out, fmt.Errorf("upload to %s failed: %w", destinationAlias, err)
	}

	return out, nil
}

func endpointOutput(alias string, host store.HostConfig, userName string) RemoteEndpointOutput {
	out := RemoteEndpointOutput{
		Alias:         alias,
		Host:          host.Host,
		UserName:      userName,
		DisplayTarget: host.Host,
		Port:          domain.EffectivePort(host.Port),
	}
	if userName != "" {
		out.DisplayTarget = fmt.Sprintf("%s@%s", userName, host.Host)
	}
	return out
}

func withTrailingSeparator(path string) string {
	separator := string(filepath.Separator)
	if strings.HasSuffix(path, separator) {
		return path
	}
	return path + separator
}
