package cli

import (
	"fmt"
	"os/user"

	"onessh/internal/infra/audit"
	"onessh/internal/infra/repository"
	"onessh/internal/presenters"

	"github.com/spf13/cobra"
)

func (o *rootOptions) repository() (repository.Repository, error) {
	path, err := repository.ResolvePath(o.dataPath)
	if err != nil {
		return repository.Repository{}, err
	}
	return repository.Repository{Path: path}, nil
}

func (o *rootOptions) logEvent(action, alias, host, user, result string, err error) {
	if o.auditLog == nil {
		return
	}
	e := audit.Event{
		Action: action,
		Alias:  alias,
		Host:   host,
		User:   user,
		Result: result,
	}
	if err != nil {
		e.Error = err.Error()
	}
	o.auditLog.Log(e)
}

func newLogCmd(opts *rootOptions) *cobra.Command {
	var (
		last               int
		action             string
		alias              string
		format             string
		auditLogMaxSizeMB  int
		auditLogMaxBackups int
		auditLogMaxAge     int
		auditLogCompress   bool
	)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show and manage audit logging",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			normalizedFormat, err := validateOutputFormat(format, "table", "json")
			if err != nil {
				return err
			}

			repo, err := opts.repository()
			if err != nil {
				return err
			}

			events, err := audit.ReadLast(repo.Path, last, action, alias)
			if err != nil {
				return err
			}
			return presenters.RenderAuditLog(cmd.OutOrStdout(), toPresenterAuditLogEvents(events), normalizedFormat)
		},
	}
	cmd.Flags().IntVarP(&last, "last", "n", 20, "Number of recent entries to show (0=all)")
	cmd.Flags().StringVar(&action, "action", "", "Filter by action (connect, exec, add_host, etc.)")
	cmd.Flags().StringVar(&alias, "alias", "", "Filter by host/user alias")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table|json)")
	cmd.PersistentFlags().IntVar(&auditLogMaxSizeMB, "audit-log-max-size-mb", 10, "Max audit log file size before rotation (MB)")
	cmd.PersistentFlags().IntVar(&auditLogMaxBackups, "audit-log-max-backups", 5, "Max rotated audit log files to keep")
	cmd.PersistentFlags().IntVar(&auditLogMaxAge, "audit-log-max-age", 7, "Max age of rotated audit log files in days")
	cmd.PersistentFlags().BoolVar(&auditLogCompress, "audit-log-compress", true, "Compress rotated audit log files")
	cmd.AddCommand(
		newLogEnableCmd(opts),
		newLogDisableCmd(opts),
		newLogStatusCmd(opts),
	)
	return cmd
}

func newLogEnableCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable audit logging by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}
			if err := audit.SetEnabled(repo.Path, true); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Audit logging enabled by default.")
			return nil
		},
	}
}

func newLogDisableCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable audit logging by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}
			if err := audit.SetEnabled(repo.Path, false); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Audit logging disabled by default.")
			return nil
		},
	}
}

func newLogStatusCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether audit logging is enabled by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := opts.repository()
			if err != nil {
				return err
			}
			settings, err := audit.LoadSettings(repo.Path)
			if err != nil {
				return err
			}
			state := "disabled"
			if settings.Enabled {
				state = "enabled"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Audit logging is %s by default.\n", state)
			return nil
		},
	}
}

func currentUserName() string {
	u, err := user.Current()
	if err != nil {
		return "root"
	}
	if u.Username == "" {
		return "root"
	}
	return u.Username
}

func toPresenterAuditLogEvents(events []audit.Event) []presenters.AuditLogEvent {
	rows := make([]presenters.AuditLogEvent, len(events))
	for i, event := range events {
		rows[i] = presenters.AuditLogEvent{
			Time:   event.Time,
			Action: event.Action,
			Alias:  event.Alias,
			Host:   event.Host,
			User:   event.User,
			Result: event.Result,
			Error:  event.Error,
			Extra:  event.Extra,
		}
	}
	return rows
}
