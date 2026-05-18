package repository

import "onessh/internal/store"

var (
	ErrConfigNotFound  = store.ErrConfigNotFound
	ErrInvalidPassword = store.ErrInvalidPassword
)

type AuthConfig = store.AuthConfig
type HostConfig = store.HostConfig
type PlainConfig = store.PlainConfig
type UserConfig = store.UserConfig

// Repository is the infrastructure-facing facade over the existing encrypted store.
type Repository struct {
	Path string
}

func ResolvePath(customPath string) (string, error) {
	return store.ResolvePath(customPath)
}

func NewPlainConfig() PlainConfig {
	return store.NewPlainConfig()
}

func (r Repository) Exists() bool {
	return r.storeRepository().Exists()
}

func (r Repository) Load(passphrase []byte) (PlainConfig, error) {
	return r.storeRepository().Load(passphrase)
}

func (r Repository) Save(cfg PlainConfig, passphrase []byte) error {
	return r.storeRepository().Save(cfg, passphrase)
}

func (r Repository) SaveWithReset(cfg PlainConfig, passphrase []byte) error {
	return r.storeRepository().SaveWithReset(cfg, passphrase)
}

func (r Repository) storeRepository() store.Repository {
	return store.Repository{Path: r.Path}
}
