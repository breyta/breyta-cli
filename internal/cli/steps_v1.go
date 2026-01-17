package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/mattn/go-isatty"
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

	meta["hint"] = "Save step intent + examples: breyta steps docs set " + fs + " " + sid + " --markdown '...'; breyta steps record --flow " + fs + " --type <type> --id " + sid + " --params '{...}'; breyta steps examples add " + fs + " " + sid + " --input '{...}' --output '{...}'; breyta steps tests add " + fs + " " + sid + " --name '...' --input '{...}' --expected '{...}'"
}

func shouldWriteHumanNextActions(app *App, cmd *cobra.Command) bool {
	if app == nil || cmd == nil {
		return false
	}
	// Only add extra human guidance when the caller explicitly opted into "pretty"
	// or the user is in an interactive terminal.
	if app.PrettyJSON {
		return true
	}
	if f, ok := cmd.ErrOrStderr().(*os.File); ok {
		return isatty.IsTerminal(f.Fd())
	}
	return false
}

func extractHints(out map[string]any) []string {
	if out == nil {
		return nil
	}
	// Prefer explicit progressive-disclosure hints.
	if hsAny, ok := out["_hints"]; ok {
		if hs, ok := hsAny.([]any); ok {
			var hints []string
			for _, h := range hs {
				if s, ok := h.(string); ok && strings.TrimSpace(s) != "" {
					hints = append(hints, strings.TrimSpace(s))
				}
			}
			if len(hints) > 0 {
				return hints
			}
		}
	}
	// Fall back to a single meta.hint if that's all we have.
	if metaAny, ok := out["meta"]; ok {
		if meta, ok := metaAny.(map[string]any); ok {
			if s, _ := meta["hint"].(string); strings.TrimSpace(s) != "" {
				return []string{strings.TrimSpace(s)}
			}
		}
	}
	return nil
}

func renderNextActionsBlock(out map[string]any, max int) string {
	hints := extractHints(out)
	if len(hints) == 0 {
		return ""
	}
	if max > 0 && len(hints) > max {
		hints = hints[:max]
	}

	var b strings.Builder
	b.WriteString("Next actions:\n")
	for _, h := range hints {
		b.WriteString("  - ")
		b.WriteString(h)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeNextActionsIfHelpful(app *App, cmd *cobra.Command, out map[string]any) {
	if !shouldWriteHumanNextActions(app, cmd) {
		return
	}
	block := renderNextActionsBlock(out, 4)
	if strings.TrimSpace(block) == "" {
		return
	}
	_, _ = io.WriteString(cmd.ErrOrStderr(), block+"\n")
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
	cmd.AddCommand(newStepsRecordCmd(app))
	cmd.AddCommand(newStepsShowCmd(app))
	cmd.AddCommand(newStepsDocsCmd(app))
	cmd.AddCommand(newStepsExamplesCmd(app))
	cmd.AddCommand(newStepsTestsCmd(app))
	return cmd
}

func extractStepsRunResult(out map[string]any) any {
	if out == nil {
		return nil
	}
	if dataAny, ok := out["data"]; ok {
		if data, ok := dataAny.(map[string]any); ok {
			return data["result"]
		}
	}
	return nil
}

func recordStepSidecars(client api.Client, out map[string]any, flowSlug string, stepID string, stepType string, params map[string]any, resultAny any, note string, testName string, traceID string, profileID string, recordExample bool, recordTest bool) {
	if !isOK(out) || (!recordExample && !recordTest) {
		return
	}

	meta := ensureMeta(out)

	if recordExample {
		exPayload := map[string]any{
			"flowSlug": flowSlug,
			"stepId":   stepID,
			"input":    params,
			"output":   resultAny,
		}
		if strings.TrimSpace(note) != "" {
			exPayload["note"] = strings.TrimSpace(note)
		}
		exOut, _, exErr := client.DoCommand(context.Background(), "steps.examples.add", exPayload)
		if exErr != nil {
			if meta != nil {
				meta["recordExampleSaved"] = false
				meta["recordExampleError"] = exErr.Error()
			}
		} else if isOK(exOut) {
			if meta != nil {
				meta["recordExampleSaved"] = true
			}
		} else if meta != nil {
			meta["recordExampleSaved"] = false
			meta["recordExampleError"] = formatAPIError(exOut)
		}
	}

	if recordTest {
		testPayload := map[string]any{
			"flowSlug":  flowSlug,
			"stepId":    stepID,
			"stepType":  stepType,
			"name":      strings.TrimSpace(testName),
			"input":     params,
			"expected":  resultAny,
			"note":      strings.TrimSpace(note),
			"traceId":   strings.TrimSpace(traceID),
			"profileId": strings.TrimSpace(profileID),
		}
		for _, k := range []string{"name", "note", "traceId", "profileId"} {
			if sv, ok := testPayload[k].(string); ok && strings.TrimSpace(sv) == "" {
				delete(testPayload, k)
			}
		}
		testOut, _, testErr := client.DoCommand(context.Background(), "steps.tests.add", testPayload)
		if testErr != nil {
			if meta != nil {
				meta["recordTestSaved"] = false
				meta["recordTestError"] = testErr.Error()
			}
		} else if isOK(testOut) {
			if meta != nil {
				meta["recordTestSaved"] = true
			}
		} else if meta != nil {
			meta["recordTestSaved"] = false
			meta["recordTestError"] = formatAPIError(testOut)
		}
	}
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
			writeNextActionsIfHelpful(app, cmd, out)
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
	var recordExample bool
	var recordTest bool
	var recordNote string
	var recordTestName string

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
			fs := strings.TrimSpace(flowSlug)
			if (recordExample || recordTest) && fs == "" {
				return writeErr(cmd, errors.New("missing --flow (required for --record-example/--record-test)"))
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
			recordStepSidecars(client, out, fs, id, t, params, extractStepsRunResult(out), recordNote, recordTestName, traceID, profileID, recordExample, recordTest)
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
	cmd.Flags().BoolVar(&recordExample, "record-example", false, "After a successful run, store the observed input/output as a step example (requires --flow)")
	cmd.Flags().BoolVar(&recordTest, "record-test", false, "After a successful run, store a snapshot test case with expected=result (requires --flow)")
	cmd.Flags().StringVar(&recordNote, "record-note", "", "Optional note for --record-example/--record-test")
	cmd.Flags().StringVar(&recordTestName, "record-test-name", "", "Optional test name for --record-test")
	return cmd
}

func newStepsRecordCmd(app *App) *cobra.Command {
	var stepType string
	var stepID string
	var flowSlug string
	var paramsJSON string
	var traceID string
	var profileID string
	var note string
	var testName string
	var noExample bool
	var noTest bool

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Run a step and record examples/tests (API mode)",
		Long: strings.TrimSpace(`
Run a single step and persist the observed input/output as step sidecars:
- Example: input=params, output=result
- Snapshot test: input=params, expected=result

This is a convenience wrapper around steps run + steps examples add + steps tests add.

Examples:
  breyta steps record --flow my-flow --type code --id make-output --params '{"input":{"n":2},"code":"(fn [input] {:nPlusOne (inc (:n input))})"}'
  breyta steps record --flow my-flow --type http --id fetch --params '{"url":"https://api.example.com","method":"get"}' --note 'happy path'
`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fs := strings.TrimSpace(flowSlug)
			if fs == "" {
				return writeErr(cmd, errors.New("missing --flow"))
			}
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
				"flowSlug": fs,
				"stepType": t,
				"stepId":   id,
				"params":   params,
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
			recordStepSidecars(client, out, fs, id, t, params, extractStepsRunResult(out), note, testName, traceID, profileID, !noExample, !noTest)
			if isOK(out) {
				addStepSidecarHint(out, fs, id)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&flowSlug, "flow", "", "Flow slug (required)")
	cmd.Flags().StringVar(&stepType, "type", "", "Step type (e.g. http, llm, code)")
	cmd.Flags().StringVar(&stepID, "id", "", "Step id (identifier within a flow)")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Step params as JSON object")
	cmd.Flags().StringVar(&traceID, "trace-id", "", "Optional trace id")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Optional profile id (for slot-based connections)")
	cmd.Flags().StringVar(&note, "note", "", "Optional note for the recorded example/test")
	cmd.Flags().StringVar(&testName, "test-name", "", "Optional test name for the recorded snapshot test")
	cmd.Flags().BoolVar(&noExample, "no-example", false, "Do not record an example")
	cmd.Flags().BoolVar(&noTest, "no-test", false, "Do not record a snapshot test")
	return cmd
}
