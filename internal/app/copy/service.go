package copy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	commonapp "onessh/internal/app/common"
	"onessh/internal/domain"
	"onessh/internal/ports"
	appruntime "onessh/internal/runtime"
)

type IdentityResolver = ports.IdentityResolver

type Runner interface {
	CopyRemote(ctx context.Context, req Request) error
}

type Service struct {
	IdentityResolver IdentityResolver
	Runner           Runner
	Audit            ports.Audit
}

type AgentConfig = ports.AgentConfig

type Input struct {
	Config     domain.PlainConfig
	Alias      string
	RemotePath string
	LocalPaths []string
	IsUpload   bool
	Recursive  bool
	Agent      AgentConfig
	IO         appruntime.IOStreams
}

type Output struct {
	Alias         string
	Host          string
	UserName      string
	DisplayTarget string
	Port          int
}

type Request struct {
	Config     domain.PlainConfig
	Host       domain.HostConfig
	UserName   string
	Auth       domain.AuthConfig
	RemotePath string
	LocalPaths []string
	IsUpload   bool
	Recursive  bool
	Agent      AgentConfig
	Stdout     io.Writer
	Stderr     io.Writer
}

func (s Service) Copy(ctx context.Context, input Input) (Output, error) {
	alias := strings.TrimSpace(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}
	if len(input.LocalPaths) == 0 {
		return Output{}, errors.New("local paths cannot be empty")
	}
	if s.IdentityResolver == nil {
		return Output{}, errors.New("copy identity resolver is required")
	}
	if s.Runner == nil {
		return Output{}, errors.New("copy runner is required")
	}

	target, exists := input.Config.Hosts[alias]
	if !exists {
		return Output{}, fmt.Errorf("host %q not found", alias)
	}

	userName, auth, err := s.IdentityResolver.ResolveHostIdentity(input.Config, target)
	if err != nil {
		return Output{}, err
	}

	out := Output{
		Alias:         alias,
		Host:          target.Host,
		UserName:      userName,
		DisplayTarget: target.Host,
		Port:          domain.EffectivePort(target.Port),
	}
	if userName != "" {
		out.DisplayTarget = fmt.Sprintf("%s@%s", userName, target.Host)
	}

	err = s.Runner.CopyRemote(ctx, Request{
		Config:     input.Config,
		Host:       target,
		UserName:   userName,
		Auth:       auth,
		RemotePath: input.RemotePath,
		LocalPaths: append([]string{}, input.LocalPaths...),
		IsUpload:   input.IsUpload,
		Recursive:  input.Recursive,
		Agent:      input.Agent,
		Stdout:     input.IO.Out,
		Stderr:     input.IO.ErrOut,
	})
	commonapp.RecordAuditResult(s.Audit, "cp", out.Alias, out.Host, out.UserName, err)
	return out, err
}
