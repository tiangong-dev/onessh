package connectivity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	commonapp "onessh/internal/app/common"
	"onessh/internal/domain"
	"onessh/internal/ports"
)

type IdentityResolver = ports.IdentityResolver

type AgentConfig = ports.AgentConfig

type Runner interface {
	Ping(ctx context.Context, req Request) error
}

type Service struct {
	IdentityResolver IdentityResolver
	Runner           Runner
	Audit            ports.Audit
}

type TimeoutConfig struct {
	Seconds int
}

type Input struct {
	Config  domain.PlainConfig
	Alias   string
	Timeout TimeoutConfig
	Agent   AgentConfig
}

type Output struct {
	Alias         string
	Host          string
	UserName      string
	DisplayTarget string
	Port          int
}

type Request struct {
	Config   domain.PlainConfig
	Host     domain.HostConfig
	UserName string
	Auth     domain.AuthConfig
	Timeout  TimeoutConfig
	Agent    AgentConfig
}

func (s Service) Ping(ctx context.Context, input Input) (Output, error) {
	alias := strings.TrimSpace(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}
	if s.IdentityResolver == nil {
		return Output{}, errors.New("connectivity identity resolver is required")
	}
	if s.Runner == nil {
		return Output{}, errors.New("connectivity runner is required")
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

	err = s.Runner.Ping(ctx, Request{
		Config:   input.Config,
		Host:     target,
		UserName: userName,
		Auth:     auth,
		Timeout:  input.Timeout,
		Agent:    input.Agent,
	})
	commonapp.RecordAuditResult(s.Audit, "ping", out.Alias, out.Host, out.UserName, err)
	return out, err
}
