package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"onessh/internal/store"

	"gopkg.in/yaml.v3"
)

type hostListRow struct {
	Alias     string `json:"alias"`
	Desc      string `json:"desc"`
	Host      string `json:"host"`
	User      string `json:"user"`
	UserRef   string `json:"user_ref"`
	Auth      string `json:"auth"`
	Port      int    `json:"port"`
	ProxyJump string `json:"proxy_jump"`
	Tags      string `json:"tags"`
	Status    string `json:"status"`
}

func buildHostListRows(cfg store.PlainConfig, aliases []string) []hostListRow {
	rows := make([]hostListRow, 0, len(aliases))
	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		userName, authType, status := summarizeHostIdentityForList(cfg, host)
		port := host.Port
		if port <= 0 {
			port = 22
		}
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
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func renderHostListTable(out io.Writer, rows []hostListRow) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tDESC\tHOST\tUSER\tUSER_REF\tAUTH\tPORT\tPROXY_JUMP\tTAGS\tSTATUS")
	for _, row := range rows {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			row.Alias,
			row.Desc,
			row.Host,
			row.User,
			row.UserRef,
			row.Auth,
			row.Port,
			row.ProxyJump,
			row.Tags,
			row.Status,
		)
	}
	return w.Flush()
}

func buildHostDumpConfig(cfg store.PlainConfig, alias string, host store.HostConfig) store.PlainConfig {
	outCfg := store.PlainConfig{
		Hosts: map[string]store.HostConfig{alias: host},
		Users: map[string]store.UserConfig{},
	}
	if host.UserRef != "" {
		if u, ok := cfg.Users[host.UserRef]; ok {
			outCfg.Users[host.UserRef] = u
		}
	}
	return outCfg
}

func renderHostDetailsTable(out io.Writer, alias string, host store.HostConfig, cfg store.PlainConfig) {
	port := host.Port
	if port <= 0 {
		port = 22
	}

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
		for _, key := range keys {
			fmt.Fprintf(out, "  %s=%s\n", key, host.Env[key])
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

func renderHostDetailsYAML(out io.Writer, cfg store.PlainConfig) error {
	outBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	_, err = out.Write(outBytes)
	return err
}
