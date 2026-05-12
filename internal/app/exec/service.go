package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"onessh/internal/domain"
	appruntime "onessh/internal/runtime"
	"onessh/internal/store"
)

type IdentityResolver interface {
	ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error)
}

type RemoteRunner interface {
	ExecRemote(ctx context.Context, req RemoteRequest) error
}

type Service struct {
	IdentityResolver IdentityResolver
	Runner           RemoteRunner
}

type AgentConfig struct {
	Socket     string
	Capability string
}

type Input struct {
	Config    store.PlainConfig
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
	Config    store.PlainConfig
	Host      store.HostConfig
	UserName  string
	Auth      store.AuthConfig
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
	return out, err
}
