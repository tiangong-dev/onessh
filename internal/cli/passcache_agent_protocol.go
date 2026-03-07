package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tiangong-dev/shush"
)

const (
	defaultAskPassTTL     = 30 * time.Second
	defaultAskPassMaxUses = 2
)

type passphraseAgentClient struct {
	client *shush.Client
}

func newPassphraseAgentClient(
	dataPath string,
	ttl time.Duration,
	disabled bool,
	customSocket string,
) (passphraseAgentClient, error) {
	if disabled {
		return passphraseAgentClient{}, nil
	}
	socketPath, err := resolveAgentSocketPath(customSocket)
	if err != nil {
		return passphraseAgentClient{}, err
	}
	normalizedTTL := normalizeTTL(ttl)
	client := shush.NewClientWithOptions(socketPath, dataPath, normalizedTTL, shushClientOptionsFromEnv())
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

func registerAskPassToken(socketPath, password string, ttl time.Duration, maxUses int) (string, func(), error) {
	if strings.TrimSpace(password) == "" {
		return "", nil, errors.New("password auth requires non-empty password")
	}

	// Start from short-lived single-use defaults, then apply caller policy.
	opts := shush.NewShortLivedSingleUseTokenOptions()
	if ttl > 0 {
		opts.TTL = ttl
	}
	if maxUses > 0 {
		opts.MaxUses = maxUses
	}
	opts.ClientOptions = shushClientOptionsFromEnv()

	return shush.RegisterTokenWithOptions(socketPath, password, opts)
}

func resolveAskPassTokenSecret(socketPath, token string) (string, error) {
	return shush.ResolveTokenWithOptions(socketPath, token, shushClientOptionsFromEnv())
}

func resolveAgentSocketPath(custom string) (string, error) {
	if strings.TrimSpace(custom) != "" {
		return expandTilde(custom)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("ONESSH_AGENT_SOCKET")); fromEnv != "" {
		return expandTilde(fromEnv)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("SHUSH_SOCKET")); fromEnv != "" {
		return expandTilde(fromEnv)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "onessh", "agent.sock"), nil
}

func startPassphraseAgentProcess(socketPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	return shush.StartProcessWithOptions(
		socketPath,
		[]string{exePath, "agent", "serve", "--socket"},
		shushClientOptionsFromEnv(),
	)
}

func pingPassphraseAgent(socketPath string) error {
	return shush.PingWithOptions(socketPath, shushClientOptionsFromEnv())
}

func requestPassphraseAgentStop(socketPath string) error {
	return shush.StopWithOptions(socketPath, shushClientOptionsFromEnv())
}

func clearPassphraseAgentAll(socketPath string) error {
	return shush.ClearAllWithOptions(socketPath, shushClientOptionsFromEnv())
}

func clearPassphraseCacheByPrefix(socketPath, prefix string) error {
	client := shush.NewClientWithOptions(socketPath, "", defaultCacheTTL, shushClientOptionsFromEnv())
	return client.ClearPrefix(prefix)
}

func shushClientOptionsFromEnv() shush.ClientOptions {
	return shush.ClientOptions{
		DialTimeout:          envDurationMillis("ONESSH_AGENT_DIAL_TIMEOUT_MS"),
		RequestTimeout:       envDurationMillis("ONESSH_AGENT_REQUEST_TIMEOUT_MS"),
		StartupTimeout:       envDurationMillis("ONESSH_AGENT_STARTUP_TIMEOUT_MS"),
		StartupProbeInterval: envDurationMillis("ONESSH_AGENT_STARTUP_PROBE_INTERVAL_MS"),
	}
}

func envDurationMillis(key string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
