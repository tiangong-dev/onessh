package exec

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

type RemoteRunner interface {
	ExecRemote(ctx context.Context, req RemoteRequest) error
}

type Service struct {
	IdentityResolver IdentityResolver
	Runner           RemoteRunner
	Audit            ports.Audit
}

type AgentConfig = ports.AgentConfig

type Input struct {
	Config    domain.PlainConfig
	Alias     string
	RemoteCmd []string
	Agent     AgentConfig
	IO        appruntime.IOStreams
}

type Output struct {
	Alias         string
	Host          string
	UserName      string
	DisplayTarget string
	Port          int
}

type RemoteRequest struct {
	Config    domain.PlainConfig
	Host      domain.HostConfig
	UserName  string
	Auth      domain.AuthConfig
	RemoteCmd []string
	Agent     AgentConfig
	Stdout    io.Writer
	Stderr    io.Writer
}

func (s Service) Exec(ctx context.Context, input Input) (Output, error) {
	alias := strings.TrimSpace(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}
	if len(input.RemoteCmd) == 0 {
		return Output{}, errors.New("remote command cannot be empty")
	}
	if s.IdentityResolver == nil {
		return Output{}, errors.New("exec identity resolver is required")
	}
	if s.Runner == nil {
		return Output{}, errors.New("exec runner is required")
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

	err = s.Runner.ExecRemote(ctx, RemoteRequest{
		Config:    input.Config,
		Host:      target,
		UserName:  userName,
		Auth:      auth,
		RemoteCmd: append([]string{}, input.RemoteCmd...),
		Agent:     input.Agent,
		Stdout:    input.IO.Out,
		Stderr:    input.IO.ErrOut,
	})
	commonapp.RecordAuditResult(s.Audit, "exec", out.Alias, out.Host, out.UserName, err)
	return out, err
}
