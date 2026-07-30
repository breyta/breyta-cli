package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const (
	agentDashboardDefaultHistoryLimit = 20
	agentDashboardMaxHistoryLimit     = 50
)

func newAgentDashboardCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Manage the workspace Marketing Hub dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAgentDashboardShowCmd(app))
	cmd.AddCommand(newAgentDashboardCatalogCmd(app))
	cmd.AddCommand(newAgentDashboardApplyCmd(app))
	cmd.AddCommand(newAgentDashboardHistoryCmd(app))
	cmd.AddCommand(newAgentDashboardRestoreCmd(app))
	return cmd
}

func newAgentDashboardShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Aliases: []string{"get"},
		Short:   "Show the active Marketing Hub dashboard",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dispatchAgentAPICommand(cmd, app, "overview.dashboard.get", map[string]any{})
		},
	}
}

func newAgentDashboardCatalogCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "Show the governed Marketing Hub component catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dispatchAgentAPICommand(cmd, app, "overview.dashboard.catalog", map[string]any{})
		},
	}
}

func parseAgentDashboardManifest(raw string, file string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	file = strings.TrimSpace(file)
	if raw == "" && file == "" {
		return nil, errors.New("provide exactly one of --manifest or --manifest-file")
	}
	if raw != "" && file != "" {
		return nil, errors.New("--manifest and --manifest-file cannot be combined")
	}
	if file != "" {
		content, err := readExplicitFile(file)
		if err != nil {
			return nil, fmt.Errorf("read --manifest-file: %w", err)
		}
		raw = strings.TrimSpace(string(content))
		if raw == "" {
			return nil, errors.New("--manifest-file is empty; write a JSON object")
		}
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("invalid Marketing Hub manifest JSON: %w", err)
	}
	manifest, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("Marketing Hub manifest must be a JSON object")
	}
	return manifest, nil
}

func newAgentDashboardApplyCmd(app *App) *cobra.Command {
	var expectedRevision int
	var manifestJSON string
	var manifestFile string
	var changeSummary string

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a complete Marketing Hub manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return writeErr(cmd, errors.New("--expected-revision must be a positive integer"))
			}
			manifestSet := cmd.Flags().Changed("manifest")
			manifestFileSet := cmd.Flags().Changed("manifest-file")
			if !manifestSet && !manifestFileSet {
				return writeErr(cmd, errors.New("provide exactly one of --manifest or --manifest-file"))
			}
			if manifestSet && manifestFileSet {
				return writeErr(cmd, errors.New("--manifest and --manifest-file cannot be combined"))
			}
			manifest, err := parseAgentDashboardManifest(manifestJSON, manifestFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{
				"expectedRevision": expectedRevision,
				"manifest":         manifest,
			}
			if changeSummary = strings.TrimSpace(changeSummary); changeSummary != "" {
				payload["changeSummary"] = changeSummary
			}
			return dispatchAgentAPICommand(cmd, app, "overview.dashboard.apply", payload)
		},
	}
	cmd.Flags().IntVar(&expectedRevision, "expected-revision", 0, "Current dashboard revision used for conflict protection")
	cmd.Flags().StringVar(&manifestJSON, "manifest", "", "Complete Marketing Hub manifest as a JSON object")
	cmd.Flags().StringVar(&manifestFile, "manifest-file", "", "Read the complete Marketing Hub manifest from a JSON file")
	cmd.Flags().StringVar(&changeSummary, "change-summary", "", "Optional short description of the dashboard change")
	return cmd
}

func newAgentDashboardHistoryCmd(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show retained Marketing Hub dashboard revisions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > agentDashboardMaxHistoryLimit {
				return writeErr(cmd, errors.New("--limit must be between 1 and 50"))
			}
			return dispatchAgentAPICommand(cmd, app, "overview.dashboard.history", map[string]any{
				"limit": limit,
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", agentDashboardDefaultHistoryLimit, "Maximum revisions to return (1-50)")
	return cmd
}

func newAgentDashboardRestoreCmd(app *App) *cobra.Command {
	var expectedRevision int
	var revision int
	var changeSummary string

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore a retained Marketing Hub revision as a new revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return writeErr(cmd, errors.New("--expected-revision must be a positive integer"))
			}
			if revision < 1 {
				return writeErr(cmd, errors.New("--revision must be a positive integer"))
			}
			payload := map[string]any{
				"expectedRevision": expectedRevision,
				"revision":         revision,
			}
			if changeSummary = strings.TrimSpace(changeSummary); changeSummary != "" {
				payload["changeSummary"] = changeSummary
			}
			return dispatchAgentAPICommand(cmd, app, "overview.dashboard.restore", payload)
		},
	}
	cmd.Flags().IntVar(&expectedRevision, "expected-revision", 0, "Current dashboard revision used for conflict protection")
	cmd.Flags().IntVar(&revision, "revision", 0, "Retained revision to restore")
	cmd.Flags().StringVar(&changeSummary, "change-summary", "", "Optional short description of the restore")
	return cmd
}
