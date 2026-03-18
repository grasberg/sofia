package doctor

import (
	"github.com/spf13/cobra"
)

// NewDoctorCommand returns the `sofia doctor` subcommand.
func NewDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Sofia's configuration and environment",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDoctor()
		},
	}
	return cmd
}
