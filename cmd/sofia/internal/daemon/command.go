package daemon

import (
	"github.com/spf13/cobra"
)

// NewDaemonCommand returns the `sofia daemon` subcommand tree.
func NewDaemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage Sofia as a background service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newInstallCommand(),
		newUninstallCommand(),
		newStatusCommand(),
	)

	return cmd
}
