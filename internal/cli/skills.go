package cli

import (
	"fmt"
	"os"
	"strings"

	"breyta-cli/skills"

	"github.com/spf13/cobra"
)

func newSkillsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Agent skill utilities",
	}

	cmd.AddCommand(newSkillsInstallCmd(app))
	return cmd
}

func newSkillsInstallCmd(app *App) *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the breyta-flows-cli agent skill bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := skills.Provider(strings.TrimSpace(provider))

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			paths, err := skills.InstallBreytaFlowsCLI(home, p)
			if err != nil {
				return err
			}
			for _, p := range paths {
				fmt.Fprintln(cmd.OutOrStdout(), "installed:", p)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", string(skills.ProviderCodex), "Install location (codex|cursor|claude)")
	return cmd
}
