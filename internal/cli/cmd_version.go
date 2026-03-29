package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(version, commit, date string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "onessh %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "commit: %s\n", commit)
			fmt.Fprintf(cmd.OutOrStdout(), "date: %s\n", date)
		},
	}
	return cmd
}
