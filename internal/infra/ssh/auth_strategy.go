package ssh

import (
	"fmt"
	"io"
	"os"
	"strings"

	"onessh/internal/domain"
)

// PasswordFDProvider creates the fd consumed by sshpass -d. The cleanup
// returns any error from finishing the password write or releasing fds so the
// caller can surface it alongside the command result.
type PasswordFDProvider func(password string) (*os.File, func() error, error)

// AskPassEnvProvider prepares SSH_ASKPASS environment variables and cleanup.
// Cleanup returns any error from clearing the agent token or removing the
// launcher script.
type AskPassEnvProvider func(agentSocket, agentCapability, password string) ([]string, func() error, error)

// PasswordAuthStrategy selects the password authentication transport wrapper.
type PasswordAuthStrategy struct {
	LookPath          func(file string) (string, error)
	NewPasswordFD     PasswordFDProvider
	PrepareAskPassEnv AskPassEnvProvider
	WarningWriter     io.Writer
}

// PasswordAuthRequest contains inputs needed to apply password auth.
type PasswordAuthRequest struct {
	Binary          string
	Args            []string
	Auth            domain.AuthConfig
	Env             []string
	AgentSocket     string
	AgentCapability string
	BaseBinary      string
}

// PasswordAuthResult contains the command invocation selected by the strategy.
type PasswordAuthResult struct {
	Binary     string
	Args       []string
	Env        []string
	ExtraFiles []*os.File
	Cleanup    func() error
}

// ApplyPasswordAuth returns the command invocation for password auth without
// executing it.
func (s PasswordAuthStrategy) ApplyPasswordAuth(req PasswordAuthRequest) (PasswordAuthResult, error) {
	result := PasswordAuthResult{
		Binary:  req.Binary,
		Args:    req.Args,
		Env:     req.Env,
		Cleanup: noopCleanup,
	}
	if strings.ToLower(req.Auth.Type) != "password" || req.Auth.Password == "" {
		return result, nil
	}

	if s.LookPath != nil {
		if _, err := s.LookPath("sshpass"); err == nil {
			if s.NewPasswordFD == nil {
				return PasswordAuthResult{}, fmt.Errorf("password fd provider is required")
			}
			fd, cleanup, err := s.NewPasswordFD(req.Auth.Password)
			if err != nil {
				return PasswordAuthResult{}, err
			}
			if cleanup == nil {
				cleanup = noopCleanup
			}
			result.Binary = "sshpass"
			result.Args = append([]string{"-d", "3", req.BaseBinary}, req.Args...)
			result.ExtraFiles = []*os.File{fd}
			result.Cleanup = cleanup
			return result, nil
		}
	}

	if s.WarningWriter != nil {
		fmt.Fprintln(s.WarningWriter, "sshpass not found; using weaker SSH_ASKPASS fallback with a short-lived single-use agent token.")
	}
	if s.PrepareAskPassEnv == nil {
		return PasswordAuthResult{}, fmt.Errorf("askpass env provider is required")
	}
	askPassEnv, cleanup, err := s.PrepareAskPassEnv(req.AgentSocket, req.AgentCapability, req.Auth.Password)
	if err != nil {
		return PasswordAuthResult{}, err
	}
	if cleanup == nil {
		cleanup = noopCleanup
	}
	result.Env = append(req.Env, askPassEnv...)
	result.Cleanup = cleanup
	return result, nil
}

func noopCleanup() error { return nil }
