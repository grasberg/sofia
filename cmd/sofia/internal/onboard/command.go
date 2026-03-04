package onboard

import (
	"embed"

	"github.com/spf13/cobra"
)

//go:generate cp -r ../../../../workspace .
//go:generate cp -r ../../../../third_party/antigravity-kit .
//go:embed workspace all:antigravity-kit
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
