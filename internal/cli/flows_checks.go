package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsChecksCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Manage step-first authoring checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsChecksCreateCmd(app))
	cmd.AddCommand(newFlowsChecksRunCmd(app))
	cmd.AddCommand(newFlowsChecksStatusCmd(app))
	return cmd
}

func newFlowsStepsChecksCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Run checks for flow-local steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsTargetChecksRunCmd(app,
		"run <flow-slug> <step-id>",
		"Run draft checks for a flow-local step",
		"flows.steps.checks.run",
		"flows steps checks run",
	))
	return cmd
}

func newFlowsAgentsChecksCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Run checks for flow-local agent steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsTargetChecksRunCmd(app,
		"run <flow-slug> <agent-step-id>",
		"Run draft checks for a flow-local agent step",
		"flows.agents.checks.run",
		"flows agents checks run",
	))
	return cmd
}

func newFlowsTargetChecksRunCmd(app *App, use string, short string, commandName string, apiModeName string) *cobra.Command {
	var source string
	var category string

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New(apiModeName+" requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"stepId":   strings.TrimSpace(args[1]),
				"source":   strings.TrimSpace(source),
			}
			if strings.TrimSpace(category) != "" {
				payload["category"] = strings.TrimSpace(category)
			}
			out, status, err := apiClient(app).DoCommand(context.Background(), commandName, payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft)")
	cmd.Flags().StringVar(&category, "category", "", "Check category (eval|security|reliability|release)")
	return cmd
}

func newFlowsChecksCreateCmd(app *App) *cobra.Command {
	var source string
	var category string
	var file string
	var description string
	var enabled bool

	cmd := &cobra.Command{
		Use:   "create <flow-slug> <check-id>",
		Short: "Create or update a draft authoring check",
		Long: strings.TrimSpace(`
Store an eval, security, reliability, or release check as draft flow authoring
state.

Examples:
  breyta flows checks create my-flow definition-of-done --file checks/dod.edn
  breyta flows checks create my-flow security-policy --category security --file checks/security.edn
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows checks create requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"checkId":  strings.TrimSpace(args[1]),
				"source":   strings.TrimSpace(source),
				"enabled":  enabled,
			}
			if strings.TrimSpace(category) != "" {
				payload["category"] = strings.TrimSpace(category)
			}
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			if strings.TrimSpace(file) != "" {
				b, err := readExplicitFile(strings.TrimSpace(file))
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["checkLiteral"] = string(b)
			}
			out, status, err := apiClient(app).DoCommand(context.Background(), "flows.checks.create", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft)")
	cmd.Flags().StringVar(&category, "category", "", "Check category (eval|security|reliability|release)")
	cmd.Flags().StringVar(&file, "file", "", "EDN check definition file")
	cmd.Flags().StringVar(&description, "description", "", "Check description")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Whether the check is enabled")
	return cmd
}

func newFlowsChecksRunCmd(app *App) *cobra.Command {
	var source string
	var category string

	cmd := &cobra.Command{
		Use:   "run <flow-slug>",
		Short: "Run draft authoring checks",
		Long: strings.TrimSpace(`
Run bounded authoring checks for a draft flow and persist compact latest-run
summaries on authored checks.

Examples:
  breyta flows checks run my-flow --source draft
  breyta flows checks run my-flow --source draft --category security
`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows checks run requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"source":   strings.TrimSpace(source),
			}
			if strings.TrimSpace(category) != "" {
				payload["category"] = strings.TrimSpace(category)
			}
			out, status, err := apiClient(app).DoCommand(context.Background(), "flows.checks.run", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft)")
	cmd.Flags().StringVar(&category, "category", "", "Check category (eval|security|reliability|release)")
	return cmd
}

func newFlowsChecksStatusCmd(app *App) *cobra.Command {
	var source string
	var category string

	cmd := &cobra.Command{
		Use:   "status <flow-slug>",
		Short: "Show draft authoring check status",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows checks status requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"source":   strings.TrimSpace(source),
			}
			if strings.TrimSpace(category) != "" {
				payload["category"] = strings.TrimSpace(category)
			}
			out, status, err := apiClient(app).DoCommand(context.Background(), "flows.checks.status", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source (draft)")
	cmd.Flags().StringVar(&category, "category", "", "Check category (eval|security|reliability|release)")
	return cmd
}
