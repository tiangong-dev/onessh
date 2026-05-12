package cli

import (
	"fmt"
	"io"
	"sort"

	"onessh/internal/domain"
	"onessh/internal/store"

	"github.com/spf13/cobra"
)

func hostHasTag(host store.HostConfig, tag string) bool {
	return domain.HostHasTag(host.Tags, tag)
}

func printDryRunHosts(out io.Writer, cfg store.PlainConfig, aliases []string) {
	fmt.Fprintf(out, "Matched %d host(s):\n", len(aliases))
	for _, alias := range aliases {
		host := cfg.Hosts[alias]
		port := domain.EffectivePort(host.Port)
		userName, _, err := resolveHostIdentity(cfg, host)
		if err != nil {
			fmt.Fprintf(out, "  %-20s %s (SKIP: %v)\n", alias, host.Host, err)
		} else {
			fmt.Fprintf(out, "  %-20s %s@%s:%d\n", alias, userName, host.Host, port)
		}
	}
}

func collectFilteredHosts(cfg store.PlainConfig, tag, filter string) []string {
	hosts := make(map[string]domain.HostQuery, len(cfg.Hosts))
	for alias, host := range cfg.Hosts {
		hosts[alias] = domain.HostQuery{
			Address:     host.Host,
			Description: host.Description,
			Tags:        host.Tags,
		}
	}
	return domain.CollectFilteredHosts(hosts, tag, filter)
}

func matchHostFilter(alias string, host store.HostConfig, pattern string) bool {
	return domain.MatchHostFilter(alias, domain.HostQuery{
		Address:     host.Host,
		Description: host.Description,
		Tags:        host.Tags,
	}, pattern)
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
