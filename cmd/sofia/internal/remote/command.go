package remote

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/pkg/remote"
)

// NewRemoteCommand returns the "remote" subcommand tree.
func NewRemoteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote access via Tailscale",
	}

	cmd.AddCommand(
		newEnableCommand(),
		newDisableCommand(),
		newStatusCommand(),
	)

	return cmd
}

func newEnableCommand() *cobra.Command {
	var funnel bool
	var port int

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable remote access to Sofia's web UI",
		RunE: func(_ *cobra.Command, _ []string) error {
			tm := remote.NewTailscaleManager()
			if !tm.IsAvailable() {
				return fmt.Errorf("tailscale CLI not found in PATH — install Tailscale first")
			}

			if funnel {
				fmt.Printf("Enabling Tailscale Funnel on port %d (public HTTPS)...\n", port)
				if err := tm.EnableFunnel(port); err != nil {
					return err
				}
				fmt.Println("Funnel enabled. Your Sofia is now publicly accessible.")
			} else {
				fmt.Printf("Enabling Tailscale Serve on port %d (tailnet-only HTTPS)...\n", port)
				if err := tm.EnableServe(port); err != nil {
					return err
				}
				fmt.Println("Serve enabled. Your Sofia is accessible within your tailnet.")
			}

			// Show the DNS name
			status, err := tm.Status()
			if err == nil && status.Self.DNSName != "" {
				dnsName := strings.TrimSuffix(status.Self.DNSName, ".")
				fmt.Printf("Access at: https://%s\n", dnsName)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&funnel, "funnel", false, "Use Tailscale Funnel (public access)")
	cmd.Flags().IntVar(&port, "port", 3000, "Local port to expose")

	return cmd
}

func newDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable remote access",
		RunE: func(_ *cobra.Command, _ []string) error {
			tm := remote.NewTailscaleManager()
			if err := tm.Disable(); err != nil {
				return err
			}
			fmt.Println("Remote access disabled.")
			return nil
		},
	}
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show remote access status",
		RunE: func(_ *cobra.Command, _ []string) error {
			tm := remote.NewTailscaleManager()
			if !tm.IsAvailable() {
				fmt.Println("Tailscale is not installed.")
				return nil
			}

			status, err := tm.Status()
			if err != nil {
				return err
			}

			dnsName := strings.TrimSuffix(status.Self.DNSName, ".")
			fmt.Printf("Tailscale state: %s\n", status.BackendState)
			fmt.Printf("DNS name: %s\n", dnsName)
			if len(status.Self.TailscaleIPs) > 0 {
				fmt.Printf("IPs: %s\n", strings.Join(status.Self.TailscaleIPs, ", "))
			}

			return nil
		},
	}
}
