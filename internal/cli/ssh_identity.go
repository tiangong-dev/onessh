package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"onessh/internal/domain"
)

func resolveHostIdentity(cfg domain.PlainConfig, host domain.HostConfig) (string, domain.AuthConfig, error) {
	if strings.TrimSpace(host.UserRef) == "" {
		return "", domain.AuthConfig{}, errors.New("host has no user_ref configured")
	}

	userCfg, ok := cfg.Users[host.UserRef]
	if !ok {
		return "", domain.AuthConfig{}, fmt.Errorf("host references missing user profile: %s", host.UserRef)
	}
	if strings.TrimSpace(userCfg.Name) == "" {
		return "", domain.AuthConfig{}, fmt.Errorf("user profile %q has empty name", host.UserRef)
	}

	normalizedAuthType := normalizeAuthType(userCfg.Auth.Type)
	if normalizedAuthType == "" {
		return "", domain.AuthConfig{}, fmt.Errorf("user profile %q has no auth configured", host.UserRef)
	}
	userCfg.Auth.Type = normalizedAuthType
	if userCfg.Auth.Type == "password" && strings.TrimSpace(userCfg.Auth.Password) == "" {
		return "", domain.AuthConfig{}, fmt.Errorf("user profile %q has empty password", host.UserRef)
	}
	return strings.TrimSpace(userCfg.Name), userCfg.Auth, nil
}

func summarizeHostIdentityForList(cfg domain.PlainConfig, host domain.HostConfig) (string, string, string) {
	userRef := strings.TrimSpace(host.UserRef)
	if userRef == "" {
		return "-", "-", "missing_user_ref"
	}

	userCfg, ok := cfg.Users[userRef]
	if !ok {
		return "-", "-", "missing_user_profile"
	}

	userName := strings.TrimSpace(userCfg.Name)
	if userName == "" {
		userName = "-"
	}

	authType := normalizeAuthType(userCfg.Auth.Type)
	if authType == "" {
		return userName, "-", "missing_auth"
	}
	if authType == "password" && strings.TrimSpace(userCfg.Auth.Password) == "" {
		return userName, authType, "empty_password"
	}

	return userName, authType, "ok"
}

func hostAliasesUsingUser(cfg domain.PlainConfig, userAlias string) []string {
	var hostAliases []string
	for hostAlias, hostCfg := range cfg.Hosts {
		if hostCfg.UserRef == userAlias {
			hostAliases = append(hostAliases, hostAlias)
		}
	}
	sort.Strings(hostAliases)
	return hostAliases
}
