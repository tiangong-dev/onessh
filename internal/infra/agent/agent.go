package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	appruntime "onessh/internal/runtime"

	"github.com/tiangong-dev/shush"
)

const (
	DefaultAskPassTTL     = 10 * time.Second
	DefaultAskPassMaxUses = 1

	AgentSocketEnv     = "ONESSH_AGENT_SOCKET"
	AgentCapabilityEnv = "ONESSH_AGENT_CAPABILITY"

	AskPassExecutableEnv = "ONESSH_ASKPASS_EXE"
	AskPassSocketEnv     = "ONESSH_ASKPASS_SOCKET"
	AskPassTokenEnv      = "ONESSH_ASKPASS_TOKEN"
	AskPassCapabilityEnv = "ONESSH_ASKPASS_CAPABILITY"
)

// SocketPathRequest contains the pure inputs needed to resolve an agent socket.
type SocketPathRequest struct {
	Explicit   string
	EnvSocket  string
	RuntimeDir string
	HomeDir    string
	ParentPID  int
}

// AskPassEnv contains the values used to construct SSH_ASKPASS environment.
type AskPassEnv struct {
	ScriptPath string
	Executable string
	SocketPath string
	Token      string
	Capability string
}

// PassphraseClient is the infrastructure adapter around the shush client.
type PassphraseClient struct {
	client *shush.Client
}

// ResolveSocketPath applies onessh's agent socket precedence rules.
func ResolveSocketPath(req SocketPathRequest) (string, error) {
	if strings.TrimSpace(req.Explicit) != "" {
		return expandTilde(req.Explicit, req.HomeDir)
	}
	if strings.TrimSpace(req.EnvSocket) != "" {
		return expandTilde(req.EnvSocket, req.HomeDir)
	}
	return DefaultSocketPath(req)
}

// ResolveSocketPathFromEnv resolves an agent socket using process environment.
func ResolveSocketPathFromEnv(explicit string) (string, error) {
	return ResolveSocketPath(SocketPathRequest{
		Explicit:   explicit,
		EnvSocket:  os.Getenv(AgentSocketEnv),
		RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
		HomeDir:    userHomeDirOrEmpty(),
		ParentPID:  os.Getppid(),
	})
}

// DefaultSocketPath builds the default per-session agent socket path.
func DefaultSocketPath(req SocketPathRequest) (string, error) {
	socketName := "agent-" + fmt.Sprintf("%d", req.ParentPID) + ".sock"
	if runtimeDir := strings.TrimSpace(req.RuntimeDir); runtimeDir != "" {
		return filepath.Join(runtimeDir, "onessh", socketName), nil
	}
	if strings.TrimSpace(req.HomeDir) == "" {
		return "", errors.New("resolve home directory: home directory is empty")
	}
	return filepath.Join(req.HomeDir, ".config", "onessh", "agents", socketName), nil
}

// DefaultSocketPathFromEnv returns the process default agent socket path.
func DefaultSocketPathFromEnv() (string, error) {
	return DefaultSocketPath(SocketPathRequest{
		RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"),
		HomeDir:    userHomeDirOrEmpty(),
		ParentPID:  os.Getppid(),
	})
}

// DefaultSocketFlagValue returns the socket flag default from environment.
func DefaultSocketFlagValue() string {
	return strings.TrimSpace(os.Getenv(AgentSocketEnv))
}

// DefaultCapabilityFlagValue returns the capability flag default from environment.
func DefaultCapabilityFlagValue() string {
	return strings.TrimSpace(os.Getenv(AgentCapabilityEnv))
}

// DefaultSessionID identifies the parent process session used for fallback capabilities.
func DefaultSessionID() string {
	return fmt.Sprintf("uid:%d:ppid:%d", os.Getuid(), os.Getppid())
}

// ResolveCapability applies explicit, environment, then session-derived precedence.
func ResolveCapability(explicit, envValue, sessionID string) string {
	return appruntime.ResolveAgentCapability(explicit, envValue, sessionID)
}

// ResolveCapabilityFromEnv resolves a capability using process environment.
func ResolveCapabilityFromEnv(explicit string) string {
	return ResolveCapability(explicit, DefaultCapabilityFlagValue(), DefaultSessionID())
}

// DeriveSessionCapability derives the deterministic capability for a session id.
func DeriveSessionCapability(sessionID string) string {
	return appruntime.DeriveAgentCapability(sessionID)
}

// NewPassphraseClient creates an agent-backed passphrase cache client.
func NewPassphraseClient(
	cacheKey string,
	ttl time.Duration,
	disabled bool,
	customSocket string,
	customCapability string,
) (PassphraseClient, error) {
	if disabled {
		return PassphraseClient{}, nil
	}
	socketPath, err := ResolveSocketPathFromEnv(customSocket)
	if err != nil {
		return PassphraseClient{}, err
	}
	client := shush.NewClient(socketPath, cacheKey, appruntime.NormalizeCacheTTL(ttl))
	client.Capability = ResolveCapabilityFromEnv(customCapability)
	if exePath, exeErr := os.Executable(); exeErr == nil {
		client.ServeArgs = []string{exePath, "agent", "serve", "--socket"}
	}
	return PassphraseClient{client: client}, nil
}

func (c PassphraseClient) IsEnabled() bool {
	return c.client != nil && c.client.IsEnabled()
}

func (c PassphraseClient) Get() ([]byte, bool, error) {
	if !c.IsEnabled() {
		return nil, false, nil
	}
	return c.client.Get()
}

func (c PassphraseClient) Set(passphrase []byte) error {
	if !c.IsEnabled() || len(passphrase) == 0 {
		return nil
	}
	return c.client.Set(passphrase)
}

func (c PassphraseClient) Clear() error {
	if !c.IsEnabled() {
		return nil
	}
	return c.client.Clear()
}

// RegisterAskPassToken stores a short-lived password token in the agent.
func RegisterAskPassToken(socketPath, password string, ttl time.Duration, maxUses int, capability string) (string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return "", nil, errors.New("password auth requires non-empty password")
	}

	normalizedTTL := DefaultAskPassTTL
	if ttl > 0 {
		normalizedTTL = ttl
	}

	normalizedMaxUses := DefaultAskPassMaxUses
	if maxUses > 0 {
		normalizedMaxUses = maxUses
	}

	return shush.RegisterTokenWithCapability(socketPath, ResolveCapabilityFromEnv(capability), password, normalizedTTL, normalizedMaxUses)
}

// ResolveAskPassTokenSecret resolves and consumes an askpass token.
func ResolveAskPassTokenSecret(socketPath, token, capability string) (string, error) {
	return shush.ResolveTokenWithCapability(socketPath, ResolveCapabilityFromEnv(capability), token)
}

// AskPassLauncherScript returns the shell wrapper used by OpenSSH's SSH_ASKPASS.
func AskPassLauncherScript() string {
	return "#!/bin/sh\nexec \"$ONESSH_ASKPASS_EXE\" askpass --socket \"$ONESSH_ASKPASS_SOCKET\" --token \"$ONESSH_ASKPASS_TOKEN\"\n"
}

// BuildAskPassEnv returns the environment consumed by the askpass launcher.
//
// SECURITY: SSH_ASKPASS helpers are exec'd by ssh, which forwards its own
// environment (including ONESSH_ASKPASS_TOKEN) to the helper. The token is
// therefore visible to same-UID processes via /proc/<pid>/environ for the
// helper's lifetime. The OpenSSH protocol does not provide an alternate
// channel (stdin/fd/file) to pass credentials to the helper that would
// avoid this exposure. We rely on three layered mitigations instead:
//   1. Short TTL (DefaultAskPassTTL = 10s).
//   2. Single-use semantics (DefaultAskPassMaxUses = 1).
//   3. Capability binding (ONESSH_ASKPASS_CAPABILITY) so a leaked token
//      cannot be redeemed without the per-session capability secret.
// Same-UID attackers can already ptrace/read the parent onessh process, so
// further hardening of this specific surface yields little marginal value.
// Users requiring stronger isolation should install sshpass (see
// PasswordAuthStrategy.ApplyPasswordAuth).
func BuildAskPassEnv(input AskPassEnv) []string {
	env := []string{
		"SSH_ASKPASS=" + input.ScriptPath,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=onessh:0",
		AskPassExecutableEnv + "=" + input.Executable,
		AskPassSocketEnv + "=" + input.SocketPath,
		AskPassTokenEnv + "=" + input.Token,
	}
	if capability := strings.TrimSpace(input.Capability); capability != "" {
		env = append(env, AskPassCapabilityEnv+"="+capability)
	}
	return env
}

// PrepareAskPassEnv registers a token and creates the temporary SSH_ASKPASS launcher.
func PrepareAskPassEnv(agentSocket, agentCapability, password string) ([]string, func() error, error) {
	if strings.TrimSpace(password) == "" {
		return nil, nil, errors.New("password auth requires non-empty password")
	}

	socketPath, err := ResolveSocketPathFromEnv(agentSocket)
	if err != nil {
		return nil, nil, err
	}
	token, clearToken, err := RegisterAskPassToken(socketPath, password, DefaultAskPassTTL, DefaultAskPassMaxUses, agentCapability)
	if err != nil {
		return nil, nil, err
	}

	exePath, err := os.Executable()
	if err != nil {
		clearToken()
		return nil, nil, fmt.Errorf("resolve executable path: %w", err)
	}

	scriptPath, err := writeAskPassLauncher()
	if err != nil {
		clearToken()
		return nil, nil, err
	}

	capabilityValue := ResolveCapabilityFromEnv(agentCapability)
	env := BuildAskPassEnv(AskPassEnv{
		ScriptPath: scriptPath,
		Executable: exePath,
		SocketPath: socketPath,
		Token:      token,
		Capability: capabilityValue,
	})
	cleanup := func() error {
		clearToken()
		if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove askpass launcher: %w", err)
		}
		return nil
	}
	return env, cleanup, nil
}

// Serve runs the agent server without an explicit capability.
func Serve(socketPath string, errOut io.Writer) error {
	return shush.Serve(socketPath, errOut)
}

// ServeWithCapability runs the agent server with onessh's resolved capability.
func ServeWithCapability(socketPath string, errOut io.Writer, capability string) error {
	return shush.ServeWithCapability(socketPath, errOut, ResolveCapabilityFromEnv(capability))
}

// StartProcess starts the background agent process if it is not already running.
func StartProcess(socketPath, capability string) error {
	capability = strings.TrimSpace(ResolveCapabilityFromEnv(capability))
	if err := Ping(socketPath, capability); err == nil {
		return nil
	} else if IsCapabilityAuthError(err) {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	// #nosec G204 -- exePath is the current onessh executable resolved by os.Executable.
	cmd := exec.Command(exePath, "agent", "serve", "--socket", socketPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if capability != "" {
		cmd.Env = WithCapabilityEnv(os.Environ(), capability)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent process: %w", err)
	}
	_ = cmd.Process.Release()

	var lastErr error
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		lastErr = Ping(socketPath, capability)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("wait for agent startup: %w", lastErr)
}

// IsCapabilityAuthError reports shush capability authorization failures.
func IsCapabilityAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "forbidden: capability") || strings.Contains(msg, "forbidden: invalid capability")
}

// WithCapabilityEnv replaces capability variables in an environment slice.
func WithCapabilityEnv(env []string, capability string) []string {
	onesshPrefix := AgentCapabilityEnv + "="
	shushPrefix := shush.EnvCapability + "="
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		if strings.HasPrefix(kv, onesshPrefix) || strings.HasPrefix(kv, shushPrefix) {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, onesshPrefix+capability, shushPrefix+capability)
	return out
}

// Ping checks whether the agent is reachable with the resolved capability.
func Ping(socketPath, capability string) error {
	return shush.PingWithCapability(socketPath, ResolveCapabilityFromEnv(capability))
}

// Stop requests agent shutdown with the resolved capability.
func Stop(socketPath, capability string) error {
	return shush.StopWithCapability(socketPath, ResolveCapabilityFromEnv(capability))
}

// ClearAll clears all agent entries.
func ClearAll(socketPath, capability string) error {
	return RunWithCapabilityEnv(ResolveCapabilityFromEnv(capability), func() error {
		return shush.ClearAll(socketPath)
	})
}

// ClearPrefix clears entries matching a cache key prefix.
func ClearPrefix(socketPath, prefix, capability string, ttl time.Duration) error {
	client := shush.NewClient(socketPath, "", appruntime.NormalizeCacheTTL(ttl))
	client.Capability = ResolveCapabilityFromEnv(capability)
	return client.ClearPrefix(prefix)
}

// RunWithCapabilityEnv runs fn with shush's capability environment set.
func RunWithCapabilityEnv(capability string, fn func() error) error {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return fn()
	}

	original, existed := os.LookupEnv(shush.EnvCapability)
	if err := os.Setenv(shush.EnvCapability, capability); err != nil {
		return err
	}
	defer func() {
		if existed {
			_ = os.Setenv(shush.EnvCapability, original)
			return
		}
		_ = os.Unsetenv(shush.EnvCapability)
	}()
	return fn()
}

func writeAskPassLauncher() (string, error) {
	scriptFile, err := os.CreateTemp("", "onessh-askpass-*.sh")
	if err != nil {
		return "", fmt.Errorf("create askpass launcher: %w", err)
	}
	scriptPath := scriptFile.Name()

	if _, err := scriptFile.WriteString(AskPassLauncherScript()); err != nil {
		_ = scriptFile.Close()
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf("write askpass launcher: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf("close askpass launcher: %w", err)
	}
	// #nosec G302 -- SSH_ASKPASS helper must be owner-executable.
	if err := os.Chmod(scriptPath, 0o500); err != nil {
		_ = os.Remove(scriptPath)
		return "", fmt.Errorf("chmod askpass launcher: %w", err)
	}
	return scriptPath, nil
}

func expandTilde(input, homeDir string) (string, error) {
	if input == "" {
		return "", nil
	}
	if input == "~" {
		if strings.TrimSpace(homeDir) == "" {
			return "", errors.New("expand ~: home directory is empty")
		}
		return homeDir, nil
	}
	if strings.HasPrefix(input, "~/") {
		if strings.TrimSpace(homeDir) == "" {
			return "", errors.New("expand ~: home directory is empty")
		}
		return filepath.Join(homeDir, strings.TrimPrefix(input, "~/")), nil
	}
	return input, nil
}

func userHomeDirOrEmpty() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return homeDir
}
