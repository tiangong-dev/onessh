package cli

import (
	"fmt"
	"io"
	"strings"

	"onessh/internal/domain"
	"onessh/internal/presenters"

	"gopkg.in/yaml.v3"
)

type hostListRow = presenters.HostListRow

func buildHostListRows(cfg domain.PlainConfig, aliases []string) []hostListRow {
	rows := make([]hostListRow, 0, len(aliases))
	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		userName, authType, status := summarizeHostIdentityForList(cfg, host)
		port := domain.EffectivePort(host.Port)
		proxyJump := strings.TrimSpace(host.ProxyJump)
		if proxyJump == "" {
			proxyJump = "-"
		}
		userRef := strings.TrimSpace(host.UserRef)
		if userRef == "" {
			userRef = "-"
		}
		tagStr := "-"
		if len(host.Tags) > 0 {
			tagStr = strings.Join(host.Tags, ",")
		}
		desc := "-"
		if host.Description != "" {
			desc = host.Description
		}
		rows = append(rows, hostListRow{
			Alias:     alias,
			Desc:      desc,
			Host:      host.Host,
			User:      userName,
			UserRef:   userRef,
			Auth:      authType,
			Port:      port,
			ProxyJump: proxyJump,
			Tags:      tagStr,
			Status:    status,
		})
	}
	return rows
}

func renderHostListJSON(out io.Writer, rows []hostListRow) error {
	return presenters.RenderHostListJSON(out, rows)
}

func renderHostListTable(out io.Writer, rows []hostListRow) error {
	return presenters.RenderHostListTable(out, rows)
}

func buildHostDumpConfig(cfg domain.PlainConfig, alias string, host domain.HostConfig) domain.PlainConfig {
	outCfg := domain.PlainConfig{
		Hosts: map[string]domain.HostConfig{alias: host},
		Users: map[string]domain.UserConfig{},
	}
	if host.UserRef != "" {
		if u, ok := cfg.Users[host.UserRef]; ok {
			outCfg.Users[host.UserRef] = u
		}
	}
	return outCfg
}

func renderHostDetailsTable(out io.Writer, alias string, host domain.HostConfig, cfg domain.PlainConfig) {
	port := domain.EffectivePort(host.Port)

	fmt.Fprintf(out, "Alias:        %s\n", alias)
	fmt.Fprintf(out, "Host:         %s\n", host.Host)
	if host.Description != "" {
		fmt.Fprintf(out, "Description:  %s\n", host.Description)
	}
	fmt.Fprintf(out, "Port:         %d\n", port)

	if host.UserRef != "" {
		fmt.Fprintf(out, "User Ref:     %s\n", host.UserRef)
		if userCfg, ok := cfg.Users[host.UserRef]; ok {
			fmt.Fprintf(out, "User:         %s\n", userCfg.Name)
			fmt.Fprintf(out, "Auth:         %s\n", summarizeAuth(userCfg.Auth))
		}
	}

	if host.ProxyJump != "" {
		fmt.Fprintf(out, "Proxy Jump:   %s\n", host.ProxyJump)
	}

	if len(host.Tags) > 0 {
		fmt.Fprintf(out, "Tags:         %s\n", strings.Join(host.Tags, ", "))
	}

	if len(host.Env) > 0 {
		fmt.Fprintf(out, "Env:\n")
		keys := sortedStringMapKeys(host.Env)
		// Table output has no --show-secrets toggle (yaml only), so env values
		// are always redacted here to match the yaml redaction path
		// (see redactConfigForDump).
		for _, key := range keys {
			fmt.Fprintf(out, "  %s=%s\n", key, redactedSecretValue)
		}
	}

	if len(host.PreConnect) > 0 {
		fmt.Fprintf(out, "Pre Connect:\n")
		for _, c := range host.PreConnect {
			fmt.Fprintf(out, "  %s\n", c)
		}
	}

	if len(host.PostConnect) > 0 {
		fmt.Fprintf(out, "Post Connect:\n")
		for _, c := range host.PostConnect {
			fmt.Fprintf(out, "  %s\n", c)
		}
	}
}

func renderHostDetailsYAML(out io.Writer, cfg domain.PlainConfig) error {
	outBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	_, err = out.Write(outBytes)
	return err
}
