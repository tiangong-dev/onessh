package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func hostHasTag(host store.HostConfig, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	for _, t := range host.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

func printDryRunHosts(out io.Writer, cfg store.PlainConfig, aliases []string) {
	fmt.Fprintf(out, "Matched %d host(s):\n", len(aliases))
	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		port := host.Port
		if port <= 0 {
			port = 22
		}
		userName, _, err := resolveHostIdentity(cfg, host)
		if err != nil {
			fmt.Fprintf(out, "  %-20s %s (SKIP: %v)\n", alias, host.Host, err)
		} else {
			fmt.Fprintf(out, "  %-20s %s@%s:%d\n", alias, userName, host.Host, port)
		}
	}
}

func collectFilteredHosts(cfg store.PlainConfig, tag, filter string) []string {
	aliases := make([]string, 0, len(cfg.Hosts))
	for alias := range cfg.Hosts {
		if tag != "" && !hostHasTag(cfg.Hosts[alias], tag) {
			continue
		}
		if filter != "" && !matchHostFilter(alias, cfg.Hosts[alias], filter) {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func matchHostFilter(alias string, host store.HostConfig, pattern string) bool {
	if matched, _ := filepath.Match(pattern, alias); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, host.Host); matched {
		return true
	}
	if host.Description != "" {
		if matched, _ := filepath.Match(pattern, host.Description); matched {
			return true
		}
	}
	return false
}

// completionHostAliases returns a ValidArgsFunction that completes host aliases
// using the cached master password (silently skips completion if no cache is available).
func completionHostAliases(opts *rootOptions) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		repo, err := opts.repository()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cache, err := opts.passphraseStore(repo.Path)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		pass, ok, _ := cache.Get()
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer wipe(pass)
		cfg, err := repo.Load(pass)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		aliases := make([]string, 0, len(cfg.Hosts))
		for alias := range cfg.Hosts {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		return aliases, cobra.ShellCompDirectiveNoFileComp
	}
}
