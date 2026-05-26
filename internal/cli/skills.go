package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/internal/skillsync"
	"github.com/breyta/breyta-cli/skills"

	"github.com/spf13/cobra"
)

func newSkillsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Agent skill utilities",
	}

	cmd.AddCommand(newSkillsInstallCmd(app))
	cmd.AddCommand(newSkillsStatusCmd(app))
	return cmd
}

func newSkillsInstallCmd(app *App) *cobra.Command {
	var provider string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the breyta agent skill bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			provider = strings.TrimSpace(provider)

			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			var providers []skills.Provider
			if strings.EqualFold(provider, "all") {
				providers = skillsync.AllProviders()
			} else {
				p := skills.Provider(provider)
				if _, err := skills.Target(home, p); err != nil {
					return err
				}
				providers = []skills.Provider{p}
			}

			_, files, err := skilldocs.FetchBundle(context.Background(), nil, app.APIURL, app.Token, skills.BreytaSkillSlug)
			if err != nil {
				return err
			}
			files = skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)
			allPaths := []string{}
			skillsync.ClearCachedStatusWarnings()
			for _, p := range providers {
				target, err := skills.Target(home, p)
				if err != nil {
					return err
				}
				paths, err := skills.InstallBreytaSkillFiles(home, p, files)
				if err != nil {
					return err
				}
				allPaths = append(allPaths, paths...)
				fmt.Fprintf(cmd.OutOrStdout(), "Installed skill in %s (%s)\n", target.Dir, p)
				warnDuplicateBreytaSkills(cmd, home, p)
			}
			if verbose {
				for _, installedPath := range allPaths {
					fmt.Fprintln(cmd.OutOrStdout(), "installed:", installedPath)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", string(skills.ProviderCodex), "Install location (all|codex|cursor|claude|gemini)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print every installed file path")
	return cmd
}

func newSkillsStatusCmd(app *App) *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check installed Breyta skill freshness",
		Long: `Check installed Breyta skill freshness.

The command compares installed agent skill files against the current Breyta docs
API bundle. It warns when a local skill is stale or missing bundle files and
prints the command to refresh it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			provider = strings.TrimSpace(provider)
			var providers []skills.Provider
			if strings.EqualFold(provider, "all") {
				providers = skillsync.AllProviders()
			} else if provider != "" {
				p := skills.Provider(provider)
				if _, err := skills.Target(".", p); err != nil {
					return writeErr(cmd, err)
				}
				providers = []skills.Provider{p}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			res, err := skillsync.StatusInstalled(ctx, app.APIURL, app.Token, providers)
			if err != nil {
				return writeErr(cmd, err)
			}
			for _, warning := range res.Warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), warning)
			}
			if res.Hint != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+res.Hint)
			}
			return writeData(cmd, app, nil, res)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "all", "Provider to check (all|codex|cursor|claude|gemini)")
	return cmd
}

func warnDuplicateBreytaSkills(cmd *cobra.Command, home string, provider skills.Provider) {
	duplicates, err := skills.FindDuplicateBreytaSkills(home, provider)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: duplicate skill check failed (%v)\n", err)
		return
	}
	if warning := skills.DuplicateBreytaSkillWarning(provider, duplicates); warning != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), warning)
	}
}
