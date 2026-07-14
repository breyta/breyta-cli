package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var flowAgentStepIDRe = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9_-]{0,127}|[a-zA-Z][a-zA-Z0-9_-]{0,127}(?:\.[a-zA-Z][a-zA-Z0-9_-]{0,127})*/[a-zA-Z][a-zA-Z0-9_-]{0,127})$`)

func newFlowsAgentsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage flow-local agent steps",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newFlowsAgentsCreateCmd(app))
	cmd.AddCommand(newFlowsAgentsUpdateCmd(app))
	cmd.AddCommand(newFlowsAgentsToolsCmd(app))
	cmd.AddCommand(newFlowsAgentsRunCmd(app))
	cmd.AddCommand(newFlowsAgentsChecksCmd(app))
	cmd.AddCommand(newFlowsAgentsRemoveCmd(app))
	return cmd
}

func requireFlowsAgentsAPI(cmd *cobra.Command, app *App, action string) error {
	if !isAPIMode(app) {
		return writeErr(cmd, fmt.Errorf("flows agents %s requires API mode", action))
	}
	return requireAPI(app)
}

func flowAgentBasePayload(flowSlug string, agentStepID string, source string) map[string]any {
	return map[string]any{
		"flowSlug": strings.TrimSpace(flowSlug),
		"stepId":   strings.TrimSpace(agentStepID),
		"source":   strings.TrimSpace(source),
	}
}

func flowAgentMutationPayload(flowSlug string, agentStepID string, source string) map[string]any {
	payload := flowAgentBasePayload(flowSlug, agentStepID, source)
	payload["expectedStepType"] = "agent"
	return payload
}

func newFlowsAgentsCreateCmd(app *App) *cobra.Command {
	var source string
	var title string
	var description string

	cmd := &cobra.Command{
		Use:   "create <flow-slug> <agent-step-id>",
		Short: "Create a draft flow-local agent step",
		Long: strings.TrimSpace(`
Create a draft flow-local agent step through the canonical flows.steps.create
API. The stored step remains a normal packaged step with type agent.

Examples:
  breyta flows agents create my-flow agents/reviewer --title "Review changes"
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAgentsAPI(cmd, app, "create")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(title) == "" {
				return writeErr(cmd, errors.New("missing --title"))
			}
			payload := flowAgentBasePayload(args[0], args[1], source)
			payload["type"] = "agent"
			payload["title"] = strings.TrimSpace(title)
			if strings.TrimSpace(description) != "" {
				payload["description"] = strings.TrimSpace(description)
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.steps.create", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&title, "title", "", "Agent step title")
	cmd.Flags().StringVar(&description, "description", "", "Agent step description")
	return cmd
}

func newFlowsAgentsUpdateCmd(app *App) *cobra.Command {
	var source string
	var stepFile string

	cmd := &cobra.Command{
		Use:   "update <flow-slug> <agent-step-id> [path value]...",
		Short: "Update a draft flow-local agent step",
		Long: strings.TrimSpace(`
Update a draft flow-local agent step through the canonical flows.steps.update
API. Pass a full step definition with --file, or provide dotted path/value
pairs.

Examples:
  breyta flows agents update my-flow agents/reviewer defaults.model gpt-5.4 defaults.maxIterations 8
  breyta flows agents update my-flow agents/reviewer --file ./steps/reviewer.edn
`),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("accepts at least 2 arg(s), received %d", len(args))
			}
			if cmd.Flags().Changed("file") {
				if len(args) != 2 {
					return errors.New("--file cannot be combined with path/value edits")
				}
				if strings.TrimSpace(stepFile) == "" {
					return errors.New("--file requires a non-empty path")
				}
				return nil
			}
			editArgCount := len(args) - 2
			if editArgCount == 0 {
				return errors.New("missing path/value edits or --file")
			}
			if editArgCount%2 != 0 {
				return errors.New("path/value edits must be provided in pairs")
			}
			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAgentsAPI(cmd, app, "update")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := flowAgentMutationPayload(args[0], args[1], source)
			if cmd.Flags().Changed("file") {
				b, err := readExplicitFile(strings.TrimSpace(stepFile))
				if err != nil {
					return writeErr(cmd, fmt.Errorf("read --file: %w", err))
				}
				stepLiteral := strings.TrimSpace(string(b))
				if stepLiteral == "" {
					return writeErr(cmd, errors.New("--file must contain a non-empty packaged agent step literal"))
				}
				payload["stepLiteral"] = stepLiteral
			} else {
				edits, err := buildFlowStepEdits(args, 2)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["edits"] = edits
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.steps.update", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringVar(&stepFile, "file", "", "Read a full packaged agent step EDN literal from file")
	return cmd
}

func newFlowsAgentsToolsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Manage flow-local agent tool access",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newFlowsAgentsToolsSetCmd(app))
	return cmd
}

func newFlowsAgentsToolsSetCmd(app *App) *cobra.Command {
	var source string
	var stepIDs []string
	var clear bool

	cmd := &cobra.Command{
		Use:   "set <flow-slug> <agent-step-id>",
		Short: "Set verified flow-local steps available to an agent step",
		Long: strings.TrimSpace(`
Set the flow-local steps available to an agent step through the canonical
flows.steps.update API.

Examples:
  breyta flows agents tools set my-flow agents/reviewer --step tools/search --step tools/load
  breyta flows agents tools set my-flow agents/reviewer --clear
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAgentsAPI(cmd, app, "tools set")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clear && len(stepIDs) > 0 {
				return writeErr(cmd, errors.New("--clear cannot be combined with --step"))
			}
			if !clear && len(stepIDs) == 0 {
				return writeErr(cmd, errors.New("pass at least one --step, or use --clear to revoke all tools"))
			}
			valueLiteral, err := flowAgentToolStepsLiteral(stepIDs)
			if err != nil {
				return writeErr(cmd, err)
			}
			payload := flowAgentMutationPayload(args[0], args[1], source)
			payload["edits"] = []any{
				map[string]any{
					"path":         "defaults.tools.steps",
					"valueLiteral": valueLiteral,
				},
			}
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.steps.update", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	cmd.Flags().StringArrayVar(&stepIDs, "step", nil, "Flow-local step id to make available; repeat for multiple steps")
	cmd.Flags().BoolVar(&clear, "clear", false, "Revoke all flow-local tools")
	return cmd
}

func flowAgentToolStepsLiteral(stepIDs []string) (string, error) {
	if len(stepIDs) == 0 {
		return "[]", nil
	}
	tokens := make([]string, 0, len(stepIDs))
	for _, raw := range stepIDs {
		stepID := strings.TrimPrefix(strings.TrimSpace(raw), ":")
		if stepID == "" {
			return "", errors.New("--step cannot be empty")
		}
		if !flowAgentStepIDRe.MatchString(stepID) {
			return "", fmt.Errorf("invalid --step %q (use safe ids such as search or tools/search)", raw)
		}
		tokens = append(tokens, ":"+stepID)
	}
	return "[" + strings.Join(tokens, " ") + "]", nil
}

func newFlowsAgentsRunCmd(app *App) *cobra.Command {
	var source string
	var version int
	var paramsJSON string
	var paramsFile string
	var traceID string
	var idempotencyKey string
	var installationID string
	var legacyProfileID string
	var previewOpts stepResultPreviewOptions

	cmd := &cobra.Command{
		Use:   "run <flow-slug> <agent-step-id>",
		Short: "Run a registered flow-local agent step",
		Long: strings.TrimSpace(`
Run a registered agent step from a selected flow definition without restating
the agent step type.

Examples:
  breyta flows agents run my-flow agents/reviewer --source draft
  breyta flows agents run my-flow agents/reviewer --params-file ./params.json --result-path answer
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireStepsAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStepsRunCommand(cmd, app, stepsRunInvocation{
				CommandName:       "flows.steps.run",
				StepID:            args[1],
				FlowSlug:          args[0],
				Source:            source,
				Version:           version,
				ParamsJSON:        paramsJSON,
				ParamsFile:        paramsFile,
				TraceID:           traceID,
				IdempotencyKey:    idempotencyKey,
				ExpectedStepType:  "agent",
				InstallationID:    installationID,
				LegacyProfileID:   legacyProfileID,
				Preview:           previewOpts,
				RequireFlowForRun: true,
			})
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow definition source (draft|latest|active)")
	cmd.Flags().IntVar(&version, "version", 0, "Specific flow version")
	cmd.Flags().StringVar(&paramsJSON, "params", "", "Agent step params as JSON object; overrides authored defaults")
	cmd.Flags().StringVar(&paramsFile, "params-file", "", "Read agent step params JSON from file (overrides --params)")
	cmd.Flags().StringVar(&traceID, "trace-id", "", "Optional trace id")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for safely retrying a side-effectful agent step run")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Optional installation id for slot-based connections")
	cmd.Flags().StringVar(&legacyProfileID, "profile-id", "", "Deprecated alias for --installation-id")
	_ = cmd.Flags().MarkHidden("profile-id")
	cmd.Flags().BoolVar(&previewOpts.Full, "full", false, "Include full data.result instead of the default compact resultPreview")
	cmd.Flags().StringVar(&previewOpts.Path, "result-path", "", "Preview only one result branch (dot path or EDN-style vector path, e.g. rows.0 or [:rows 0])")
	cmd.Flags().StringVar(&previewOpts.ResultFile, "result-file", "", "Write full data.result JSON to a local file while keeping terminal output compact")
	cmd.Flags().IntVar(&previewOpts.MaxDepth, "preview-depth", stepResultPreviewDefaultDepth, "Max nested depth for resultPreview")
	cmd.Flags().IntVar(&previewOpts.MaxItems, "preview-items", stepResultPreviewDefaultItems, "Max map entries or vector items per resultPreview level")
	cmd.Flags().IntVar(&previewOpts.MaxRunes, "preview-runes", stepResultPreviewDefaultRunes, "Max runes for resultPreview.value")
	return cmd
}

func newFlowsAgentsRemoveCmd(app *App) *cobra.Command {
	var source string

	cmd := &cobra.Command{
		Use:   "remove <flow-slug> <agent-step-id>",
		Short: "Remove a draft flow-local agent step",
		Long: strings.TrimSpace(`
Remove a flow-local agent step through the canonical flows.steps.remove API.
The backend rejects removal while the flow literal or another step toolset still
references the agent step.

Examples:
  breyta flows agents remove my-flow agents/reviewer --source draft
`),
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireFlowsAgentsAPI(cmd, app, "remove")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := flowAgentMutationPayload(args[0], args[1], source)
			out, status, err := apiClient(app).DoCommand(cmd.Context(), "flows.steps.remove", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}

	cmd.Flags().StringVar(&source, "source", "draft", "Flow source; only draft is currently supported")
	return cmd
}
