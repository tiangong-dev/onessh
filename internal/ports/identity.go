package ports

import "onessh/internal/domain"

// IdentityResolver resolves the effective user and auth settings for a host.
type IdentityResolver interface {
	ResolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error)
}
