package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func requireStepsAPI(cmd *cobra.Command, app *App) error {
	// Respect explicit `--api=` forcing mock mode.
	if apiFlagExplicit(cmd) && strings.TrimSpace(app.APIURL) == "" {
		return errors.New("steps requires API mode (set BREYTA_API_URL)")
	}
	return requireAPI(app)
}

func newStepsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "steps",
		Short: "Run and inspect individual steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newStepsRunCmd(app))
	return cmd
}

func newStepsRunCmd(app *App) *cobra.Command {
	var stepType string
	var stepID string
	var flowSlug string
	var paramsJSON string
	var traceID string
	var profileID string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a single step (API mode)",
		Long: strings.TrimSpace(`
Run a single step without executing an entire flow.

This is designed for fast iteration while authoring: provide an explicit step type,
step id, and step params; the server executes the step using the same runtime
dispatcher as normal flows.

Examples:
  breyta steps run --type http --id fetch --params '{"url":"https://api.example.com","method":"get"}'
  breyta steps run --type llm --id summarize --params '{"prompt":"Summarize this","model":"gpt-5.2"}'
`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			t := strings.TrimSpace(stepType)
			if t == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			id := strings.TrimSpace(stepID)
			if id == "" {
				return writeErr(cmd, errors.New("missing --id"))
			}

			params := map[string]any{}
			if strings.TrimSpace(paramsJSON) != "" {
				var v any
				if err := json.Unmarshal([]byte(paramsJSON), &v); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --params JSON: %w", err))
				}
				m, ok := v.(map[string]any)
				if !ok {
					return writeErr(cmd, errors.New("--params must be a JSON object"))
				}
				params = m
			}

			payload := map[string]any{
				"stepType": t,
				"stepId":   id,
				"params":   params,
			}
			if strings.TrimSpace(flowSlug) != "" {
				payload["flowSlug"] = strings.TrimSpace(flowSlug)
			}
			if strings.TrimSpace(traceID) != "" {
				payload["traceId"] = strings.TrimSpace(traceID)
			}
			if strings.TrimSpace(profileID) != "" {
				payload["profileId"] = strings.TrimSpace(profileID)
			}

			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.run", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&stepType, "type", "", "Step type (e.g. http, llm, code)")
	cmd.Flags().StringVar(&stepID, "id", "", "Step id (identifier within a flow)")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step params as JSON object")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Optional flow slug (for logging/templates)")
	cmd.Flags().StringVar(&traceID, "trace-id", "", "Optional trace id")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional profile id (for slot-based connections)")
	return cmd
}
