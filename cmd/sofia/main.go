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

	"github.com/sipeed/sofia/cmd/sofia/internal"
	"github.com/sipeed/sofia/cmd/sofia/internal/agent"
	"github.com/sipeed/sofia/cmd/sofia/internal/auth"
	"github.com/sipeed/sofia/cmd/sofia/internal/cron"
	"github.com/sipeed/sofia/cmd/sofia/internal/gateway"
	"github.com/sipeed/sofia/cmd/sofia/internal/migrate"
	"github.com/sipeed/sofia/cmd/sofia/internal/onboard"
	"github.com/sipeed/sofia/cmd/sofia/internal/skills"
	"github.com/sipeed/sofia/cmd/sofia/internal/status"
	"github.com/sipeed/sofia/cmd/sofia/internal/version"
)

func NewSofiaCommand() *cobra.Command {
	short := fmt.Sprintf("%s sofia - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "sofia",
		Short:   short,
		Example: "sofia list",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

func main() {
	cmd := NewSofiaCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
