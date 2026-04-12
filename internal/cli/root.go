package cli

import (
	"regexp"
	"time"

	"onessh/internal/audit"

	"github.com/spf13/cobra"
)

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const redactedSecretValue = "[REDACTED]"

type rootOptions struct {
	dataPath        string
	cacheTTL        time.Duration
	noCache         bool
	agentSocket     string
	agentCapability string
	quiet           bool
	log             bool
	auditLog        *audit.Logger
}

func NewRootCmd(version, commit, date string) *cobra.Command {
	opts := &rootOptions{}

	var proxyJump string

	rootCmd := &cobra.Command{
		Use:           "onessh <host-alias> [-- <ssh-args...>]",
		Short:         "Manage and connect SSH hosts from encrypted config",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, sshArgs, err := parseConnectInvocation(cmd, args)
			if err != nil {
				return err
			}
			return runConnect(cmd, opts, alias, sshArgs, proxyJump, cmd.Flags().Changed("proxy-jump"))
		},
	}
	rootCmd.Flags().StringVarP(&proxyJump, "proxy-jump", "J", "", "ProxyJump via onessh alias or user@host:port (overrides stored proxy-jump)")

	rootCmd.PersistentFlags().StringVar(&opts.dataPath, "data", "", "Path to data directory")
	rootCmd.PersistentFlags().DurationVar(&opts.cacheTTL, "cache-ttl", 10*time.Minute, "Master password cache duration")
	rootCmd.PersistentFlags().BoolVar(&opts.noCache, "no-cache", false, "Disable master password cache")
	rootCmd.PersistentFlags().StringVar(&opts.agentSocket, "agent-socket", defaultAgentSocketFlagValue(), "Memory cache agent Unix socket path")
	rootCmd.PersistentFlags().StringVar(&opts.agentCapability, "agent-capability", defaultAgentCapabilityFlagValue(), "Capability token required by memory cache agent")
	rootCmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&opts.log, "log", false, "Enable audit logging for this command run")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		repo, err := opts.repository()
		if err != nil {
			return err
		}
		enabled := false
		if opts.log {
			enabled = true
		} else {
			settings, err := audit.LoadSettings(repo.Path)
			if err != nil {
				return err
			}
			enabled = settings.Enabled
		}
		if !enabled {
			return nil
		}
		rotateCfg := audit.DefaultRotateConfig()
		if f := cmd.Flags().Lookup("audit-log-max-size-mb"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetInt("audit-log-max-size-mb")
			rotateCfg.MaxSizeMB = v
		}
		if f := cmd.Flags().Lookup("audit-log-max-backups"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetInt("audit-log-max-backups")
			rotateCfg.MaxBackups = v
		}
		if f := cmd.Flags().Lookup("audit-log-max-age"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetInt("audit-log-max-age")
			rotateCfg.MaxAgeDays = v
		}
		if f := cmd.Flags().Lookup("audit-log-compress"); f != nil && f.Changed {
			v, _ := cmd.Flags().GetBool("audit-log-compress")
			rotateCfg.Compress = v
		}
		if err := audit.ValidateRotateConfig(rotateCfg); err != nil {
			return err
		}
		opts.auditLog, err = audit.Open(repo.Path, rotateCfg)
		if err != nil {
			return err
		}
		return nil
	}
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if opts.auditLog != nil {
			_ = opts.auditLog.Close()
			opts.auditLog = nil
		}
	}

	rootCmd.ValidArgsFunction = completionHostAliases(opts)

	rootCmd.AddCommand(
		newInitCmd(opts),
		newPasswdCmd(opts),
		newAddCmd(opts),
		newUpdateCmd(opts),
		newRmCmd(opts),
		newLsCmd(opts),
		newShowCmd(opts),
		newPingCmd(opts),
		newExecCmd(opts),
		newCpCmd(opts),
		newAgentCmd(opts),
		newAskPassCmd(opts),
		newUserCmd(opts),
		newLogoutCmd(opts),
		newLogCmd(opts),
		newVersionCmd(version, commit, date),
	)

	return rootCmd
}
