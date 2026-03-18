// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/cmd/sofia/internal/agent"
	"github.com/grasberg/sofia/cmd/sofia/internal/cron"
	"github.com/grasberg/sofia/cmd/sofia/internal/daemon"
	"github.com/grasberg/sofia/cmd/sofia/internal/doctor"
	"github.com/grasberg/sofia/cmd/sofia/internal/eval"
	"github.com/grasberg/sofia/cmd/sofia/internal/export"
	"github.com/grasberg/sofia/cmd/sofia/internal/gateway"
	"github.com/grasberg/sofia/cmd/sofia/internal/mcpserver"
	"github.com/grasberg/sofia/cmd/sofia/internal/onboard"
	"github.com/grasberg/sofia/cmd/sofia/internal/pairing"
	"github.com/grasberg/sofia/cmd/sofia/internal/remote"
	"github.com/grasberg/sofia/cmd/sofia/internal/version"
)

func NewSofiaCommand() *cobra.Command {
	short := fmt.Sprintf("%s sofia - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "sofia",
		Short:   short,
		Example: "sofia gateway",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		gateway.NewGatewayCommand(),
		cron.NewCronCommand(),
		daemon.NewDaemonCommand(),
		eval.NewEvalCommand(),
		export.NewDataCommand(),
		pairing.NewPairingCommand(),
		mcpserver.NewMCPServerCommand(),
		remote.NewRemoteCommand(),
		doctor.NewDoctorCommand(),
		version.NewVersionCommand(),
	)

	cmd.Root().CompletionOptions.DisableDefaultCmd = true

	return cmd
}

func main() {
	cmd := NewSofiaCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
