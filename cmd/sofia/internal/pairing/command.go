package pairing

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/pkg/channels"
)

// sofiaHome returns the Sofia home directory (~/.sofia).
func sofiaHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sofia")
}

// NewPairingCommand returns the top-level "pairing" cobra command.
func NewPairingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pairing",
		Short: "Manage DM pairing for unknown senders",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newListCommand(), newApproveCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List pending pairing requests",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			pm := channels.NewPairingManager(sofiaHome())
			pending := pm.ListPending()
			if len(pending) == 0 {
				fmt.Println("No pending pairing requests.")
				return nil
			}
			for _, req := range pending {
				fmt.Printf(
					"Code: %s  Channel: %s  Sender: %s  Expires: %s\n",
					req.Code, req.Channel, req.SenderID,
					req.Expires.Format("15:04:05"),
				)
			}
			return nil
		},
	}
}

func newApproveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <code>",
		Short: "Approve a pairing request by code",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			pm := channels.NewPairingManager(sofiaHome())
			ch, sender, err := pm.Approve(args[0])
			if err != nil {
				return fmt.Errorf("pairing approve failed: %w", err)
			}
			fmt.Printf("Approved sender %s on channel %s\n", sender, ch)
			return nil
		},
	}
}
