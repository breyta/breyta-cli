package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsSchedulesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedules",
		Short: "Manage draft flow schedules backed by invocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsSchedulesUpsertCmd(app))
	cmd.AddCommand(newFlowsSchedulesValidateCmd(app))
	cmd.AddCommand(newFlowsSchedulesRemoveCmd(app))
	return cmd
}

func newFlowsSchedulesUpsertCmd(app *App) *cobra.Command {
	var source string
	var cronExpr string
	var timezone string
	var invocation string
	var label string
	var description string
	var enabled bool
	var inputSchemaFile string
	var inputSchemaLiteral string
	var responseFile string
	var responseLiteral string
	var clearFields []string

	cmd := &cobra.Command{
		Use:   "upsert <flow-slug> <schedule-id>",
		Short: "Upsert a draft flow schedule and backing invocation",
		Long: strings.TrimSpace(`
Upsert a draft schedule and generated invocation contract.

Example:
  breyta flows schedules upsert my-flow weekday --cron "0 9 * * 1-5" --timezone Europe/Oslo --input-schema ./inputs.edn
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows schedules upsert")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			clearFields, err := authoringClearFields(cmd, clearFields, map[string][]string{
				"timezone":     {"timezone"},
				"label":        {"label"},
				"description":  {"description"},
				"invocation":   {"invocation"},
				"input-schema": {"input-schema", "input-schema-literal"},
				"response":     {"response", "response-literal"},
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := pruneEmptyStrings(map[string]any{
				"flowSlug":    strings.TrimSpace(args[0]),
				"scheduleId":  strings.TrimSpace(args[1]),
				"source":      strings.TrimSpace(source),
				"cron":        strings.TrimSpace(cronExpr),
				"timezone":    strings.TrimSpace(timezone),
				"invocation":  strings.TrimSpace(invocation),
				"label":       strings.TrimSpace(label),
				"description": strings.TrimSpace(description),
			})
			if cmd.Flags().Changed("enabled") {
				payload["enabled"] = enabled
			}
			if len(clearFields) > 0 {
				payload["clearFields"] = clearFields
			}
			if err := applyLiteralOrFile(cmd, payload, "inputSchema", inputSchemaLiteral, inputSchemaFile, "--input-schema-literal", "--input-schema"); err != nil {
				return writeErr(cmd, err)
			}
			if err := applyLiteralOrFile(cmd, payload, "responseLiteral", responseLiteral, responseFile, "--response-literal", "--response"); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.schedules.upsert", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "Cron expression")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone")
	cmd.Flags().StringVar(&invocation, "invocation", "", "Backing invocation id; defaults to schedule id")
	cmd.Flags().StringVar(&label, "label", "", "Display label")
	cmd.Flags().StringVar(&description, "description", "", "Schedule and invocation description")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "Enable the schedule")
	cmd.Flags().StringVar(&inputSchemaFile, "input-schema", "", "Read invocation input schema EDN from file")
	cmd.Flags().StringVar(&inputSchemaLiteral, "input-schema-literal", "", "Invocation input schema EDN literal")
	cmd.Flags().StringVar(&responseFile, "response", "", "Read invocation response EDN from file")
	cmd.Flags().StringVar(&responseLiteral, "response-literal", "", "Invocation response EDN literal")
	cmd.Flags().StringSliceVar(&clearFields, "clear", nil, "Clear optional fields (timezone, label, description, invocation, input-schema, response); repeat as needed")
	return cmd
}

func newFlowsSchedulesValidateCmd(app *App) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "validate <flow-slug> <schedule-id>",
		Short: "Validate a draft flow schedule",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows schedules validate")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug":   strings.TrimSpace(args[0]),
				"scheduleId": strings.TrimSpace(args[1]),
				"source":     strings.TrimSpace(source),
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.schedules.validate", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	return cmd
}

func newFlowsSchedulesRemoveCmd(app *App) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <schedule-id>",
		Short: "Remove a draft flow schedule",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAuthoringAPI(cmd, app, "flows schedules remove")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug":   strings.TrimSpace(args[0]),
				"scheduleId": strings.TrimSpace(args[1]),
				"source":     strings.TrimSpace(source),
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.schedules.remove", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	return cmd
}
