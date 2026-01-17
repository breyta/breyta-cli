package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func addStepSidecarHint(out map[string]any, flowSlug string, stepID string) {
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if _, exists := meta["hint"]; exists {
		return
	}

	fs := strings.TrimSpace(flowSlug)
	if fs == "" {
		fs = "<flow-slug>"
	}
	sid := strings.TrimSpace(stepID)
	if sid == "" {
		sid = "<step-id>"
	}

	meta["hint"] = "Save step intent + examples: breyta steps docs set " + fs + " " + sid + " --markdown '...'; breyta steps examples add " + fs + " " + sid + " --input '{...}' --output '{...}'; breyta steps tests add " + fs + " " + sid + " --name '...' --input '{...}' --expected '{...}'"
}

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
	cmd.AddCommand(newStepsShowCmd(app))
	cmd.AddCommand(newStepsDocsCmd(app))
	cmd.AddCommand(newStepsExamplesCmd(app))
	cmd.AddCommand(newStepsTestsCmd(app))
	return cmd
}

func newStepsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <flow-slug> <step-id>",
		Short: "Show docs/examples/tests for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.artifacts.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}

			// Back-compat: servers predating steps.artifacts.get.
			if status == 400 && !isOK(out) {
				if errAny, ok := out["error"].(map[string]any); ok {
					if code, _ := errAny["code"].(string); code == "unknown_command" {
						docsOut, docsStatus, docsErr := client.DoCommand(context.Background(), "steps.docs.get", payload)
						if docsErr != nil {
							return writeErr(cmd, docsErr)
						}
						if docsStatus >= 400 || !isOK(docsOut) {
							return writeAPIResult(cmd, app, docsOut, docsStatus)
						}

						exOut, exStatus, exErr := client.DoCommand(context.Background(), "steps.examples.list", payload)
						if exErr != nil {
							return writeErr(cmd, exErr)
						}
						if exStatus >= 400 || !isOK(exOut) {
							return writeAPIResult(cmd, app, exOut, exStatus)
						}

						testsOut, testsStatus, testsErr := client.DoCommand(context.Background(), "steps.tests.list", payload)
						if testsErr != nil {
							return writeErr(cmd, testsErr)
						}
						if testsStatus >= 400 || !isOK(testsOut) {
							return writeAPIResult(cmd, app, testsOut, testsStatus)
						}

						wsid := ""
						if s, _ := docsOut["workspaceId"].(string); strings.TrimSpace(s) != "" {
							wsid = strings.TrimSpace(s)
						}
						if wsid == "" {
							wsid = strings.TrimSpace(app.WorkspaceID)
						}

						data := map[string]any{
							"flowSlug": args[0],
							"stepId":   args[1],
							"docs":     nil,
							"examples": map[string]any{"items": []any{}, "count": 0},
							"tests":    map[string]any{"items": []any{}, "count": 0},
						}

						if d, ok := docsOut["data"].(map[string]any); ok {
							if docs, ok := d["docs"]; ok {
								data["docs"] = docs
							}
						}
						if d, ok := exOut["data"].(map[string]any); ok {
							if items, ok := d["items"].([]any); ok {
								data["examples"] = map[string]any{"items": items, "count": len(items)}
							}
						}
						if d, ok := testsOut["data"].(map[string]any); ok {
							if items, ok := d["items"].([]any); ok {
								data["tests"] = map[string]any{"items": items, "count": len(items)}
							}
						}

						out = map[string]any{
							"ok":          true,
							"workspaceId": wsid,
							"data":        data,
						}
						addStepSidecarHint(out, args[0], args[1])
						status = 200
					}
				}
			}

			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	return cmd
}

func newStepsDocsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Manage per-step docs (API mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newStepsDocsGetCmd(app))
	cmd.AddCommand(newStepsDocsSetCmd(app))
	return cmd
}

func newStepsDocsGetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <flow-slug> <step-id>",
		Short: "Get docs for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.docs.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	return cmd
}

func newStepsDocsSetCmd(app *App) *cobra.Command {
	var markdown string
	var filePath string

	cmd := &cobra.Command{
		Use:   "set <flow-slug> <step-id>",
		Short: "Set docs for a step (API mode)",
		Long: strings.TrimSpace(`
Set (upsert) documentation for a step without publishing a new flow version.

Examples:
  breyta steps docs set my-flow make-output --markdown '# Notes\n\nThis step ...'
  breyta steps docs set my-flow make-output --file ./notes.md
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			md := strings.TrimSpace(markdown)
			if strings.TrimSpace(filePath) != "" {
				b, err := os.ReadFile(strings.TrimSpace(filePath))
				if err != nil {
					return writeErr(cmd, fmt.Errorf("read --file: %w", err))
				}
				md = string(b)
			}
			if strings.TrimSpace(md) == "" {
				return writeErr(cmd, errors.New("missing --markdown (or --file)"))
			}

			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
				"markdown": md,
				"docs":     md, // fallback alias for older servers
			}
			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.docs.put", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&markdown, "markdown", "", "Markdown content")
	cmd.Flags().StringVar(&filePath, "file", "", "Read markdown from file")
	return cmd
}

func newStepsExamplesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "examples",
		Short: "Manage per-step examples (API mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newStepsExamplesAddCmd(app))
	cmd.AddCommand(newStepsExamplesListCmd(app))
	return cmd
}

func newStepsExamplesAddCmd(app *App) *cobra.Command {
	var inputJSON string
	var outputJSON string
	var note string

	cmd := &cobra.Command{
		Use:   "add <flow-slug> <step-id>",
		Short: "Add an input/output example for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var input any
			if strings.TrimSpace(inputJSON) != "" {
				if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
			}
			var output any
			if strings.TrimSpace(outputJSON) != "" {
				if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --output JSON: %w", err))
				}
			}

			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
				"input":    input,
				"output":   output,
			}
			if strings.TrimSpace(note) != "" {
				payload["note"] = strings.TrimSpace(note)
			}

			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.examples.add", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&inputJSON, "input", "", "Example input JSON (any value)")
	cmd.Flags().StringVar(&outputJSON, "output", "", "Example output JSON (any value)")
	cmd.Flags().StringVar(&note, "note", "", "Optional note")
	return cmd
}

func newStepsExamplesListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug> <step-id>",
		Short: "List examples for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.examples.list", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	return cmd
}

func newStepsTestsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tests",
		Short: "Manage per-step test cases (API mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newStepsTestsAddCmd(app))
	cmd.AddCommand(newStepsTestsListCmd(app))
	cmd.AddCommand(newStepsTestsVerifyCmd(app))
	return cmd
}

func newStepsTestsAddCmd(app *App) *cobra.Command {
	var stepType string
	var name string
	var inputJSON string
	var expectedJSON string
	var note string

	cmd := &cobra.Command{
		Use:   "add <flow-slug> <step-id>",
		Short: "Add a test case for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var input any
			if strings.TrimSpace(inputJSON) != "" {
				if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
			}
			var expected any
			if strings.TrimSpace(expectedJSON) != "" {
				if err := json.Unmarshal([]byte(expectedJSON), &expected); err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --expected JSON: %w", err))
				}
			}

			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
				"name":     strings.TrimSpace(name),
				"input":    input,
				"expected": expected,
				"note":     strings.TrimSpace(note),
			}
			if strings.TrimSpace(stepType) != "" {
				payload["stepType"] = strings.TrimSpace(stepType)
			}

			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.tests.add", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&stepType, "type", "", "Step type for executing this test (e.g. http, llm, code)")
	cmd.Flags().StringVar(&name, "name", "", "Optional test case name")
	cmd.Flags().StringVar(&inputJSON, "input", "", "Test input JSON (any value)")
	cmd.Flags().StringVar(&expectedJSON, "expected", "", "Expected output JSON (any value)")
	cmd.Flags().StringVar(&note, "note", "", "Optional note")
	return cmd
}

func newStepsTestsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug> <step-id>",
		Short: "List test cases for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.tests.list", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	return cmd
}

func newStepsTestsVerifyCmd(app *App) *cobra.Command {
	var stepType string

	cmd := &cobra.Command{
		Use:   "verify <flow-slug> <step-id>",
		Short: "Run stored test cases for a step (API mode)",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			if strings.TrimSpace(stepType) != "" {
				payload["stepType"] = strings.TrimSpace(stepType)
			}

			client := apiClient(app)
			out, status, err := client.DoCommand(context.Background(), "steps.tests.verify", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isOK(out) {
				addStepSidecarHint(out, args[0], args[1])
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&stepType, "type", "", "Step type (required if tests were saved without one)")
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
			if isOK(out) {
				addStepSidecarHint(out, flowSlug, id)
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
