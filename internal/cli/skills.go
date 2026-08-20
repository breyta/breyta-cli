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
	cmd := &cobra.Command{Use: "skills", Short: "Manage agent guidance for this engine"}
	cmd.AddCommand(newSkillsInstallCmd(app), newSkillsStatusCmd(app))
	return cmd
}

func newSkillsInstallCmd(app *App) *cobra.Command {
	var provider string
	var verbose bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the Breyta agent skill from the configured engine",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return writeErr(cmd, err)
			}
			provider = strings.TrimSpace(provider)
			var providers []skills.Provider
			if strings.EqualFold(provider, "all") {
				providers = skillsync.AllProviders()
			} else {
				p := skills.Provider(provider)
				if _, err := skills.Target(home, p); err != nil {
					return writeErr(cmd, err)
				}
				providers = []skills.Provider{p}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			_, files, err := skilldocs.FetchBundle(ctx, nil, app.APIURL, app.Token, skills.BreytaSkillSlug)
			if err != nil {
				return writeErr(cmd, err)
			}
			files = skilldocs.ApplyCLIOverrides(skills.BreytaSkillSlug, files)
			skillsync.ClearCachedStatusWarnings()
			for _, p := range providers {
				target, err := skills.Target(home, p)
				if err != nil {
					return writeErr(cmd, err)
				}
				paths, err := skillsync.InstallProviderFiles(home, p, files)
				if err != nil {
					return writeErr(cmd, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Installed skill in %s (%s)\n", target.Dir, p)
				if verbose {
					for _, path := range paths {
						fmt.Fprintln(cmd.OutOrStdout(), "installed:", path)
					}
				}
				warnDuplicateBreytaSkills(cmd, home, p)
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var providers []skills.Provider
			provider = strings.TrimSpace(provider)
			if strings.EqualFold(provider, "all") {
				providers = skillsync.AllProviders()
			} else {
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
