package ports

import "onessh/internal/store"

// IdentityResolver resolves the effective user and auth settings for a host.
type IdentityResolver interface {
	ResolveHostIdentity(cfg store.PlainConfig, host store.HostConfig) (string, store.AuthConfig, error)
}
