package connect

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
	"onessh/internal/store"
)

type IdentityResolver = ports.IdentityResolver

type AgentConfig = ports.AgentConfig

type Transport interface {
	Connect(ctx context.Context, req TransportRequest) error
}

type Service struct {
	IdentityResolver IdentityResolver
	Transport        Transport
	Audit            ports.Audit
}

type Input struct {
	Config            store.PlainConfig
	Alias             string
	SSHArgs           []string
	ProxyJumpOverride string
	ProxyJumpChanged  bool
	Quiet             bool
	Agent             AgentConfig
	IO                appruntime.IOStreams
}

type Output struct {
	Alias         string
	Host          string
	UserName      string
	DisplayTarget string
	Port          int
}

type TransportRequest struct {
	Config   store.PlainConfig
	Host     store.HostConfig
	UserName string
	Auth     store.AuthConfig
	SSHArgs  []string
	ErrOut   io.Writer
	Agent    AgentConfig
}

func (s Service) Connect(ctx context.Context, input Input) (Output, error) {
	alias := strings.TrimSpace(input.Alias)
	if alias == "" {
		return Output{}, errors.New("host alias cannot be empty")
	}
	if s.IdentityResolver == nil {
		return Output{}, errors.New("connect identity resolver is required")
	}
	if s.Transport == nil {
		return Output{}, errors.New("connect transport is required")
	}

	target, exists := input.Config.Hosts[alias]
	if !exists {
		return Output{}, fmt.Errorf("host %q not found", alias)
	}
	if input.ProxyJumpChanged {
		target.ProxyJump = strings.TrimSpace(input.ProxyJumpOverride)
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
	if !input.Quiet && input.IO.ErrOut != nil {
		fmt.Fprintf(input.IO.ErrOut, "Connecting to %s:%d...\n", out.DisplayTarget, out.Port)
	}

	err = s.Transport.Connect(ctx, TransportRequest{
		Config:   input.Config,
		Host:     target,
		UserName: userName,
		Auth:     auth,
		SSHArgs:  append([]string{}, input.SSHArgs...),
		ErrOut:   input.IO.ErrOut,
		Agent:    input.Agent,
	})
	commonapp.RecordAuditResult(s.Audit, "connect", out.Alias, out.Host, out.UserName, err)
	return out, err
}
