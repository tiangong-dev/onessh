package ssh

import (
	"errors"
	"fmt"
	"strings"

	"onessh/internal/domain"
	"onessh/internal/store"
)

// ProxyJumpStrategy resolves ProxyJump configuration into SSH arguments.
type ProxyJumpStrategy struct {
	ResolveOnesshPath func() (string, error)
}

// BuildProxyJumpArgs resolves the ProxyJump value into SSH arguments.
func BuildProxyJumpArgs(cfg store.PlainConfig, proxyJump string, opts ArgsOptions) ([]string, error) {
	return ProxyJumpStrategy{ResolveOnesshPath: opts.resolveOnesshPath}.BuildArgs(cfg, proxyJump)
}

// BuildArgs resolves the ProxyJump value into SSH arguments.
func (s ProxyJumpStrategy) BuildArgs(cfg store.PlainConfig, proxyJump string) ([]string, error) {
	if proxyJump == "" {
		return nil, nil
	}

	jumpHostCfg, isAlias := cfg.Hosts[proxyJump]
	if !isAlias {
		return []string{"-J", proxyJump}, nil
	}

	jumpUser, ok := cfg.Users[jumpHostCfg.UserRef]
	if !ok {
		return nil, fmt.Errorf("jump host alias %q references unknown user profile %q", proxyJump, jumpHostCfg.UserRef)
	}

	port := domain.EffectivePort(jumpHostCfg.Port)

	switch domain.NormalizeAuthType(jumpUser.Auth.Type) {
	case domain.AuthTypeKey:
		dest := fmt.Sprintf("%s@%s:%d", jumpUser.Name, jumpHostCfg.Host, port)
		return []string{"-J", dest}, nil
	case domain.AuthTypePassword:
		exePath, err := s.resolveOnesshPath()
		if err != nil {
			return nil, fmt.Errorf("resolve onessh path for proxy: %w", err)
		}
		proxyCmd := BuildOnesshProxyCommand(exePath, proxyJump)
		return []string{"-o", "ProxyCommand=" + proxyCmd}, nil
	default:
		return nil, fmt.Errorf("jump host %q has unsupported auth type %q", proxyJump, jumpUser.Auth.Type)
	}
}

// BuildOnesshProxyCommand builds the shell command used for password-auth jump hosts.
func BuildOnesshProxyCommand(exePath, proxyJump string) string {
	return strings.Join([]string{
		shellSingleQuote(exePath),
		"-q",
		shellSingleQuote(proxyJump),
		"--",
		"-W",
		shellSingleQuote("%h:%p"),
	}, " ")
}

func (s ProxyJumpStrategy) resolveOnesshPath() (string, error) {
	if s.ResolveOnesshPath == nil {
		return "", errors.New("onessh path is required for password proxy jump")
	}
	return s.ResolveOnesshPath()
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
