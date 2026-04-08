package onboard

import (
	"embed"

	"github.com/spf13/cobra"
)

//go:generate sh -c "rm -rf workspace && cp -r ../../../../workspace ."
//go:embed all:workspace
var embeddedFiles embed.FS

func NewOnboardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "onboard",
		Aliases: []string{"o"},
		Short:   "Initialize sofia configuration and workspace",
		Run: func(cmd *cobra.Command, args []string) {
			onboard()
		},
	}

	return cmd
}
