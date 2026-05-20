package cli

import (
	"time"

	infraagent "onessh/internal/infra/agent"
)

const defaultAskPassTTL = infraagent.DefaultAskPassTTL
const defaultAskPassMaxUses = infraagent.DefaultAskPassMaxUses

type passphraseAgentClient = infraagent.PassphraseClient

func newPassphraseAgentClient(
	cacheKey string,
	ttl time.Duration,
	disabled bool,
	customSocket string,
	customCapability string,
) (passphraseAgentClient, error) {
	return infraagent.NewPassphraseClient(cacheKey, ttl, disabled, customSocket, customCapability)
}

func registerAskPassToken(socketPath, password string, ttl time.Duration, maxUses int, capability string) (string, func(), error) {
	return infraagent.RegisterAskPassToken(socketPath, password, ttl, maxUses, capability)
}

func resolveAskPassTokenSecret(socketPath, token, capability string) (string, error) {
	return infraagent.ResolveAskPassTokenSecret(socketPath, token, capability)
}

func isPasswordPrompt(prompt string) bool {
	return infraagent.IsPasswordPrompt(prompt)
}

func resolveAgentSocketPath(custom string) (string, error) {
	return infraagent.ResolveSocketPathFromEnv(custom)
}

func startPassphraseAgentProcess(socketPath, capability string) error {
	return infraagent.StartProcess(socketPath, capability)
}

func isShushCapabilityAuthError(err error) bool {
	return infraagent.IsCapabilityAuthError(err)
}

func withAgentCapabilityEnv(env []string, capability string) []string {
	return infraagent.WithCapabilityEnv(env, capability)
}

func pingPassphraseAgent(socketPath, capability string) error {
	return infraagent.Ping(socketPath, capability)
}

func requestPassphraseAgentStop(socketPath, capability string) error {
	return infraagent.Stop(socketPath, capability)
}

func clearPassphraseAgentAll(socketPath, capability string) error {
	return infraagent.ClearAll(socketPath, capability)
}

func clearPassphraseCacheByPrefix(socketPath, prefix, capability string) error {
	return infraagent.ClearPrefix(socketPath, prefix, capability, defaultCacheTTL)
}

func runWithCapabilityEnv(capability string, fn func() error) error {
	return infraagent.RunWithCapabilityEnv(capability, fn)
}
