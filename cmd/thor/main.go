// Thor ⚡ the Termux Titan - Ultra-powerful personal AI agent
// Originally inspired by nanobot, reforged as Thor.
// License: MIT
//
// Copyright (c) 2026 Thor contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"thor/cmd/thor/internal"
	"thor/cmd/thor/internal/agent"
	"thor/cmd/thor/internal/auth"
	"thor/cmd/thor/internal/cron"
	"thor/cmd/thor/internal/gateway"
	"thor/cmd/thor/internal/migrate"
	"thor/cmd/thor/internal/onboard"
	"thor/cmd/thor/internal/skills"
	"thor/cmd/thor/internal/status"
	"thor/cmd/thor/internal/version"
)

func NewThorCommand() *cobra.Command {
	short := fmt.Sprintf("%s Thor ⚡ the Termux Titan v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "thor",
		Short:   short,
		Example: "thor list",
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
	cmd := NewThorCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
