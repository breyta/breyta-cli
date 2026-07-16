package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsInitCmd(app *App) *cobra.Command {
	var name string
	var description string
	var empty bool
	var withManualInterface bool
	var stepID string
	var stepFile string
	var runAfterSeed bool
	var runIdempotencyKey string

	cmd := &cobra.Command{
		Use:   "init <flow-slug>",
		Short: "Create an empty draft flow for step-first authoring",
		Long: strings.TrimSpace(`
Create a minimal draft flow that can receive interfaces, schedules, steps,
checks, and later full source edits.
The empty draft and a simple no-input manual run interface are the defaults, so
--empty and --with-manual-interface are retained only for compatibility.
To create a runnable first literal in one command, pass a qualified --step-id
and a packaged step EDN file with --step-file. Add --run to prove that seeded
step immediately with its authored defaults.

Examples:
  breyta flows init company-profile --name "Company profile"
  breyta flows init company-profile --name "Company profile" --description "Marketing profile builder"
  breyta flows init company-profile --step-id tools/add-one --step-file ./steps/add-one.edn --run
`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows init requires API mode"))
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("empty") && !empty {
				return writeErr(cmd, errors.New("flows init only creates an empty draft; omit --empty or pass --empty=true"))
			}
			stepFileChanged := cmd.Flags().Changed("step-file")
			stepIDChanged := cmd.Flags().Changed("step-id")
			if stepFileChanged != stepIDChanged {
				return writeErr(cmd, errors.New("--step-id and --step-file must be provided together"))
			}
			if (runAfterSeed || cmd.Flags().Changed("run-idempotency-key")) && !stepFileChanged {
				return writeErr(cmd, errors.New("--run and --run-idempotency-key require --step-id with --step-file"))
			}
			if cmd.Flags().Changed("run-idempotency-key") && strings.TrimSpace(runIdempotencyKey) == "" {
				return writeErr(cmd, errors.New("--run-idempotency-key cannot be empty"))
			}
			payload := map[string]any{
				"flowSlug": strings.TrimSpace(args[0]),
				"empty":    true,
			}
			if strings.TrimSpace(name) != "" {
				payload["name"] = strings.TrimSpace(name)
			}
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			if withManualInterface {
				payload["withManualInterface"] = true
			}
			if stepFileChanged {
				if strings.TrimSpace(stepID) == "" {
					return writeErr(cmd, errors.New("--step-id cannot be empty"))
				}
				b, err := readExplicitFile(strings.TrimSpace(stepFile))
				if err != nil {
					return writeErr(cmd, fmt.Errorf("read --step-file: %w", err))
				}
				stepLiteral := strings.TrimSpace(string(b))
				if stepLiteral == "" {
					return writeErr(cmd, errors.New("--step-file must contain a non-empty packaged step literal"))
				}
				payload["stepId"] = strings.TrimSpace(stepID)
				payload["stepLiteral"] = stepLiteral
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.init", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if runAfterSeed {
				return runFlowStepAfterWrite(cmd, app, args[0], stepID, "draft", runIdempotencyKey, out, status)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().BoolVar(&empty, "empty", true, "Compatibility flag; initialization is empty by default")
	cmd.Flags().BoolVar(&withManualInterface, "with-manual-interface", false, "Compatibility flag; the enabled no-input manual run interface is now added by default")
	cmd.Flags().StringVar(&name, "name", "", "Flow display name")
	cmd.Flags().StringVar(&description, "description", "", "Flow description")
	cmd.Flags().StringVar(&stepID, "step-id", "", "Qualified first packaged step id to seed into the new literal")
	cmd.Flags().StringVar(&stepFile, "step-file", "", "Read the first packaged step EDN literal to seed into the new literal")
	cmd.Flags().BoolVar(&runAfterSeed, "run", false, "Run the seeded draft step with authored defaults and include the proof result")
	cmd.Flags().StringVar(&runIdempotencyKey, "run-idempotency-key", "", "Stable key for retrying an optional seeded-step proof run")
	return cmd
}
