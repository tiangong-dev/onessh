package cli

import (
	"errors"
	"fmt"
	"strings"

	apphosts "onessh/internal/app/hosts"
	appusers "onessh/internal/app/users"
	"onessh/internal/store"

	"github.com/spf13/cobra"
)

type hostUpdateFlagValues struct {
	aliasFlag    string
	hostFlag     string
	portFlag     int
	proxyJump    string
	userRefFlag  string
	userFlag     string
	authTypeFlag string
	keyPathFlag  string
	passwordFlag string
	envFlags     []string
	unsetEnv     []string
	clearEnv     bool
	preConnect   []string
	postConnect  []string
	clearPre     bool
	clearPost    bool
	tags         []string
	unsetTags    []string
	clearTags    bool
	descFlag     string
}

func applyHostUpdateFlagsWithServices(
	cmd *cobra.Command,
	cfg store.PlainConfig,
	alias string,
	existing store.HostConfig,
	values hostUpdateFlagValues,
) (store.PlainConfig, string, store.HostConfig, error) {
	if cmd.Flags().Changed("alias") && strings.TrimSpace(values.aliasFlag) == "" {
		return store.PlainConfig{}, "", store.HostConfig{}, errors.New("--alias cannot be empty")
	}
	if cmd.Flags().Changed("host") && strings.TrimSpace(values.hostFlag) == "" {
		return store.PlainConfig{}, "", store.HostConfig{}, errors.New("--host cannot be empty")
	}
	if cmd.Flags().Changed("port") && (values.portFlag <= 0 || values.portFlag > 65535) {
		return store.PlainConfig{}, "", store.HostConfig{}, errors.New("--port must be between 1 and 65535")
	}

	targetRef := strings.TrimSpace(existing.UserRef)
	if cmd.Flags().Changed("user-ref") {
		targetRef = normalizeUserAlias(values.userRefFlag)
		if targetRef == "" {
			return store.PlainConfig{}, "", store.HostConfig{}, errors.New("--user-ref cannot be empty")
		}
		if _, ok := cfg.Users[targetRef]; !ok {
			return store.PlainConfig{}, "", store.HostConfig{}, fmt.Errorf("user profile %q not found", targetRef)
		}
	}

	var err error
	cfg, err = updateLinkedUserProfileWithService(
		cmd,
		cfg,
		targetRef,
		values.userFlag,
		values.authTypeFlag,
		values.keyPathFlag,
		values.passwordFlag,
	)
	if err != nil {
		return store.PlainConfig{}, "", store.HostConfig{}, err
	}

	out, err := apphosts.Service{}.Update(apphosts.UpdateInput{
		Config:             cfg,
		Alias:              alias,
		TargetAlias:        values.aliasFlag,
		TargetAliasChanged: cmd.Flags().Changed("alias"),
		Host:               values.hostFlag,
		HostChanged:        cmd.Flags().Changed("host"),
		Port:               values.portFlag,
		PortChanged:        cmd.Flags().Changed("port"),
		ProxyJump:          values.proxyJump,
		ProxyJumpChanged:   cmd.Flags().Changed("proxy-jump"),
		Description:        values.descFlag,
		DescriptionChanged: cmd.Flags().Changed("description"),
		UserRef:            values.userRefFlag,
		UserRefChanged:     cmd.Flags().Changed("user-ref"),
		Env:                values.envFlags,
		EnvChanged:         cmd.Flags().Changed("env"),
		UnsetEnv:           values.unsetEnv,
		UnsetEnvChanged:    cmd.Flags().Changed("unset-env"),
		ClearEnv:           cmd.Flags().Changed("clear-env") && values.clearEnv,
		PreConnect:         values.preConnect,
		PreConnectChanged:  cmd.Flags().Changed("pre-connect"),
		PostConnect:        values.postConnect,
		PostConnectChanged: cmd.Flags().Changed("post-connect"),
		ClearPreConnect:    cmd.Flags().Changed("clear-pre-connect") && values.clearPre,
		ClearPostConnect:   cmd.Flags().Changed("clear-post-connect") && values.clearPost,
		Tags:               values.tags,
		TagsChanged:        cmd.Flags().Changed("tag"),
		Untag:              values.unsetTags,
		UntagChanged:       cmd.Flags().Changed("untag"),
		ClearTags:          cmd.Flags().Changed("clear-tags") && values.clearTags,
	})
	if err != nil {
		return store.PlainConfig{}, "", store.HostConfig{}, err
	}
	if _, _, err := resolveHostIdentity(out.Config, out.Host); err != nil {
		return store.PlainConfig{}, "", store.HostConfig{}, err
	}
	return out.Config, out.Alias, out.Host, nil
}

func updateLinkedUserProfileWithService(
	cmd *cobra.Command,
	cfg store.PlainConfig,
	targetRef string,
	userName string,
	authTypeFlag string,
	keyPath string,
	password string,
) (store.PlainConfig, error) {
	changedUser := cmd.Flags().Changed("user")
	changedAuthType := cmd.Flags().Changed("auth-type")
	changedKeyPath := cmd.Flags().Changed("key-path")
	changedPassword := cmd.Flags().Changed("password")

	if !changedUser && !changedAuthType && !changedKeyPath && !changedPassword {
		return cfg, nil
	}
	if strings.TrimSpace(targetRef) == "" {
		return store.PlainConfig{}, errors.New("host has no user_ref; set --user-ref first")
	}

	userCfg, ok := cfg.Users[targetRef]
	if !ok {
		return store.PlainConfig{}, fmt.Errorf("user profile %q not found", targetRef)
	}

	trimmedUser := strings.TrimSpace(userName)
	if changedUser && trimmedUser == "" {
		return store.PlainConfig{}, errors.New("--user cannot be empty")
	}
	if strings.TrimSpace(userCfg.Name) == "" {
		return store.PlainConfig{}, fmt.Errorf("user profile %q has empty name", targetRef)
	}

	input := appusers.UpdateInput{
		Config: cfg,
		Alias:  targetRef,
	}
	if changedUser {
		input.Name = trimmedUser
		input.NameChanged = true
	}
	if changedAuthType || changedKeyPath || changedPassword {
		newAuth, err := authConfigFromFlagValues(
			userCfg.Auth,
			authTypeFlag,
			keyPath,
			password,
			changedAuthType,
			changedKeyPath,
			changedPassword,
		)
		if err != nil {
			return store.PlainConfig{}, err
		}
		input.Auth = authUpdateFromConfig(newAuth)
	}

	out, err := appusers.Service{}.Update(input)
	if err != nil {
		return store.PlainConfig{}, err
	}
	return out.Config, nil
}

func authUpdateFromConfig(auth store.AuthConfig) appusers.AuthUpdate {
	update := appusers.AuthUpdate{
		Type:        auth.Type,
		TypeChanged: true,
	}
	switch normalizeAuthType(auth.Type) {
	case "key":
		update.KeyPath = auth.KeyPath
		update.KeyPathSet = true
	case "password":
		update.Password = auth.Password
		update.PasswordSet = true
	}
	return update
}
