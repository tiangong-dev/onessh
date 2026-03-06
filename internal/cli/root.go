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
	dataPath           string
	cacheTTL           time.Duration
	noCache            bool
	agentSocket        string
	quiet              bool
	log                bool
	noLog              bool
	auditLogMaxSizeMB  int
	auditLogMaxBackups int
	auditLogMaxAge     int
	auditLogCompress   bool
	auditLog           *audit.Logger
}

func NewRootCmd(version, commit, date string) *cobra.Command {
	opts := &rootOptions{}

	rootCmd := &cobra.Command{
		Use:           "onessh [host] [-- <ssh-args...>]",
		Short:         "Manage and connect SSH hosts from encrypted config",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, sshArgs, err := parseConnectInvocation(cmd, args)
			if err != nil {
				return err
			}
			return runConnect(cmd, opts, alias, sshArgs)
		},
	}

	rootCmd.PersistentFlags().StringVar(&opts.dataPath, "data", "", "Path to data directory")
	rootCmd.PersistentFlags().DurationVar(&opts.cacheTTL, "cache-ttl", 10*time.Minute, "Master password cache duration")
	rootCmd.PersistentFlags().BoolVar(&opts.noCache, "no-cache", false, "Disable master password cache")
	rootCmd.PersistentFlags().StringVar(&opts.agentSocket, "agent-socket", defaultAgentSocketFlagValue(), "Memory cache agent Unix socket path")
	rootCmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "Suppress non-essential output")
	rootCmd.PersistentFlags().BoolVar(&opts.log, "log", false, "Enable audit logging for this command run")
	rootCmd.PersistentFlags().BoolVar(&opts.noLog, "no-log", false, "Disable audit logging")
	rootCmd.PersistentFlags().IntVar(&opts.auditLogMaxSizeMB, "audit-log-max-size-mb", 10, "Audit log rotate max size in MB")
	rootCmd.PersistentFlags().IntVar(&opts.auditLogMaxBackups, "audit-log-max-backups", 5, "Audit log max backup files to keep")
	rootCmd.PersistentFlags().IntVar(&opts.auditLogMaxAge, "audit-log-max-age", 7, "Audit log max backup age in days")
	rootCmd.PersistentFlags().BoolVar(&opts.auditLogCompress, "audit-log-compress", true, "Compress rotated audit logs")
	_ = rootCmd.PersistentFlags().MarkHidden("no-log")
	_ = rootCmd.PersistentFlags().MarkDeprecated("no-log", "default is now disabled; use --log to enable for one run")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if opts.noLog {
			return nil
		}
		repo, err := opts.repository()
		if err != nil {
			return err
		}
		enabled := false
		if opts.log {
			enabled = true
		} else if opts.noLog {
			enabled = false
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
		cfg := audit.RotateConfig{
			MaxSizeMB:  opts.auditLogMaxSizeMB,
			MaxBackups: opts.auditLogMaxBackups,
			MaxAgeDays: opts.auditLogMaxAge,
			Compress:   opts.auditLogCompress,
		}
		if err := audit.ValidateRotateConfig(cfg); err != nil {
			return err
		}
		opts.auditLog, err = audit.Open(repo.Path, cfg)
		if err != nil {
			return err
		}
		return nil
	}
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		_ = opts.auditLog.Close()
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
		newDumpCmd(opts),
		newConnectCmd(opts),
		newTestCmd(opts),
		newExecCmd(opts),
		newCpCmd(opts),
		newSSHConfigCmd(opts),
		newAgentCmd(opts),
		newAskPassCmd(opts),
		newUserCmd(opts),
		newTagCmd(opts),
		newLogoutCmd(opts),
		newLogCmd(opts),
		newVersionCmd(version, commit, date),
	)

	return rootCmd
}
