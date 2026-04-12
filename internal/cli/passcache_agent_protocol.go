package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tiangong-dev/shush"
)

const (
	defaultAskPassTTL     = 10 * time.Second
	defaultAskPassMaxUses = 1
)

type passphraseAgentClient struct {
	client *shush.Client
}

func newPassphraseAgentClient(
	cacheKey string,
	ttl time.Duration,
	disabled bool,
	customSocket string,
	customCapability string,
) (passphraseAgentClient, error) {
	if disabled {
		return passphraseAgentClient{}, nil
	}
	socketPath, err := resolveAgentSocketPath(customSocket)
	if err != nil {
		return passphraseAgentClient{}, err
	}
	normalizedTTL := normalizeTTL(ttl)
	client := shush.NewClient(socketPath, cacheKey, normalizedTTL)
	client.Capability = resolveAgentCapability(customCapability)
	if exePath, exeErr := os.Executable(); exeErr == nil {
		client.ServeArgs = []string{exePath, "agent", "serve", "--socket"}
	}
	return passphraseAgentClient{
		client: client,
	}, nil
}

func (c passphraseAgentClient) IsEnabled() bool {
	return c.client != nil && c.client.IsEnabled()
}

func (c passphraseAgentClient) Get() ([]byte, bool, error) {
	if !c.IsEnabled() {
		return nil, false, nil
	}
	return c.client.Get()
}

func (c passphraseAgentClient) Set(passphrase []byte) error {
	if !c.IsEnabled() || len(passphrase) == 0 {
		return nil
	}
	return c.client.Set(passphrase)
}

func (c passphraseAgentClient) Clear() error {
	if !c.IsEnabled() {
		return nil
	}
	return c.client.Clear()
}

func registerAskPassToken(socketPath, password string, ttl time.Duration, maxUses int, capability string) (string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return "", nil, errors.New("password auth requires non-empty password")
	}

	normalizedTTL := defaultAskPassTTL
	if ttl > 0 {
		normalizedTTL = ttl
	}

	normalizedMaxUses := defaultAskPassMaxUses
	if maxUses > 0 {
		normalizedMaxUses = maxUses
	}

	return shush.RegisterTokenWithCapability(socketPath, resolveAgentCapability(capability), password, normalizedTTL, normalizedMaxUses)
}

func resolveAskPassTokenSecret(socketPath, token, capability string) (string, error) {
	return shush.ResolveTokenWithCapability(socketPath, resolveAgentCapability(capability), token)
}

func resolveAgentSocketPath(custom string) (string, error) {
	if strings.TrimSpace(custom) != "" {
		return expandTilde(custom)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("ONESSH_AGENT_SOCKET")); fromEnv != "" {
		return expandTilde(fromEnv)
	}
	return defaultAgentSocketPath()
}

func startPassphraseAgentProcess(socketPath, capability string) error {
	capability = strings.TrimSpace(resolveAgentCapability(capability))
	if err := pingPassphraseAgent(socketPath, capability); err == nil {
		return nil
	} else if isShushCapabilityAuthError(err) {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	cmd := exec.Command(exePath, "agent", "serve", "--socket", socketPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if capability != "" {
		cmd.Env = withAgentCapabilityEnv(os.Environ(), capability)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent process: %w", err)
	}
	_ = cmd.Process.Release()

	var lastErr error
	for i := 0; i < 40; i++ {
		time.Sleep(25 * time.Millisecond)
		lastErr = pingPassphraseAgent(socketPath, capability)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("wait for agent startup: %w", lastErr)
}

func isShushCapabilityAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "forbidden: capability") || strings.Contains(msg, "forbidden: invalid capability")
}

func withAgentCapabilityEnv(env []string, capability string) []string {
	onesshPrefix := onesshAgentCapabilityEnv + "="
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

func pingPassphraseAgent(socketPath, capability string) error {
	return shush.PingWithCapability(socketPath, resolveAgentCapability(capability))
}

func requestPassphraseAgentStop(socketPath, capability string) error {
	return shush.StopWithCapability(socketPath, resolveAgentCapability(capability))
}

func clearPassphraseAgentAll(socketPath, capability string) error {
	return runWithCapabilityEnv(resolveAgentCapability(capability), func() error {
		return shush.ClearAll(socketPath)
	})
}

func clearPassphraseCacheByPrefix(socketPath, prefix, capability string) error {
	client := shush.NewClient(socketPath, "", defaultCacheTTL)
	client.Capability = resolveAgentCapability(capability)
	return client.ClearPrefix(prefix)
}

func runWithCapabilityEnv(capability string, fn func() error) error {
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
