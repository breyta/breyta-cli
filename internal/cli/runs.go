package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

var shortRunIDPattern = regexp.MustCompile(`^r[0-9]+$`)

type apiCommandRunner interface {
	DoCommand(ctx context.Context, command string, args map[string]any) (map[string]any, int, error)
}

func newRunsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "runs", Aliases: []string{"run"}, Short: "Inspect and control runs"}
	cmd.AddCommand(newRunsListCmd(app))
	cmd.AddCommand(newRunsShowCmd(app))
	cmd.AddCommand(newRunsStartCmd(app))
	cmd.AddCommand(newRunsReplayCmd(app))
	cmd.AddCommand(newRunsStepCmd(app))
	cmd.AddCommand(newRunsInspectCmd(app))
	cmd.AddCommand(newRunsContinueCmd(app))
	cmd.AddCommand(newRunsCancelCmd(app))
	cmd.AddCommand(newRunsRetryCmd(app))
	cmd.AddCommand(newRunsEventsCmd(app))
	cmd.AddCommand(newRunsLogsCmd(app))
	return cmd
}

func workflowIDFromRunData(data map[string]any) string {
	if data == nil {
		return ""
	}
	if wf, _ := data["workflowId"].(string); strings.TrimSpace(wf) != "" {
		return wf
	}
	if runData, _ := data["run"].(map[string]any); runData != nil {
		if wf, _ := runData["workflowId"].(string); strings.TrimSpace(wf) != "" {
			return wf
		}
	}
	return ""
}

func installationIDFromRunData(data map[string]any) string {
	if data == nil {
		return ""
	}
	if installationID := firstNonBlankString(data["installationId"], data["installation-id"], data["profileId"], data["profile-id"]); installationID != "" {
		return installationID
	}
	if runData, _ := data["run"].(map[string]any); runData != nil {
		return firstNonBlankString(runData["installationId"], runData["installation-id"], runData["profileId"], runData["profile-id"])
	}
	return ""
}

func newRunsListCmd(app *App) *cobra.Command {
	var flow string
	var installationID string
	var profileID string
	var query string
	var status string
	var version int
	var limit int
	var cursor string
	var includeSteps bool
	cmd := &cobra.Command{
		Use:   "list [flow-slug]",
		Short: "List runs",
		Long: `List runs.

In API mode, prefer the structured query syntax used by the web runs list:
  breyta runs list --query 'status:failed flow:my-flow installation:prof-1 version:7'

Supported query tokens:
  - status:<running|completed|failed|waiting>
  - flow:<slug>
  - installation:<installation-id>
  - version:<n>

Legacy discrete flags remain available and override matching --query tokens.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && strings.TrimSpace(flow) == "" {
				flow = args[0]
			}
			if isAPIMode(app) {
				queryFilters, err := parseRunsListQuery(query)
				if err != nil {
					return writeErr(cmd, err)
				}
				effectiveFlow := strings.TrimSpace(flow)
				if effectiveFlow == "" {
					effectiveFlow = queryFilters.Flow
				}
				effectiveInstallationID := strings.TrimSpace(installationID)
				legacyProfileID := strings.TrimSpace(profileID)
				if effectiveInstallationID == "" {
					effectiveInstallationID = legacyProfileID
				}
				if effectiveInstallationID == "" {
					effectiveInstallationID = strings.TrimSpace(queryFilters.InstallationID)
				}
				effectiveStatus := strings.TrimSpace(status)
				if effectiveStatus == "" {
					effectiveStatus = strings.TrimSpace(queryFilters.Status)
				}
				effectiveVersion := queryFilters.Version
				hasVersion := queryFilters.HasVersion
				if cmd.Flags().Changed("version") {
					effectiveVersion = version
					hasVersion = true
				}
				payload := map[string]any{}
				if effectiveFlow != "" {
					payload["flowSlug"] = effectiveFlow
				}
				if effectiveInstallationID != "" {
					payload["profileId"] = effectiveInstallationID
				}
				if effectiveStatus != "" {
					payload["status"] = effectiveStatus
				}
				if hasVersion {
					payload["version"] = effectiveVersion
				}
				if strings.TrimSpace(cursor) != "" {
					payload["cursor"] = strings.TrimSpace(cursor)
				}
				if limit > 0 {
					payload["limit"] = limit
				}
				if includeSteps {
					return writeNotImplemented(cmd, app, "--include-steps is not supported in API mode yet (use `runs show <workflow-id>`)")
				}
				if structuredQuery := buildRunsListQuery(runsListFilters{
					Flow:           effectiveFlow,
					InstallationID: effectiveInstallationID,
					Status:         effectiveStatus,
					Version:        effectiveVersion,
					HasVersion:     hasVersion,
				}); structuredQuery != "" {
					payload["query"] = structuredQuery
				}
				return doAPICommand(cmd, app, "runs.list", payload)
			}
			if strings.TrimSpace(query) != "" {
				return writeNotImplemented(cmd, app, "--query is supported in API mode only")
			}
			if cmd.Flags().Changed("version") {
				return writeNotImplemented(cmd, app, "--version filtering is supported in API mode only")
			}

			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			runs, err := store.ListRuns(st, flow)
			if err != nil {
				return writeErr(cmd, err)
			}
			total := len(runs)
			truncated := false
			if limit > 0 && limit < len(runs) {
				runs = runs[:limit]
				truncated = true
			}
			meta := map[string]any{
				"total":     total,
				"shown":     len(runs),
				"truncated": truncated,
			}
			if truncated {
				meta["hint"] = "Use --limit 0 to show all runs"
			}

			// Progressive disclosure: list returns summaries by default.
			items := make([]any, 0, len(runs))
			for _, r := range runs {
				if includeSteps {
					items = append(items, r)
					continue
				}
				items = append(items, map[string]any{
					"runId":         r.WorkflowID,
					"flowSlug":      r.FlowSlug,
					"version":       r.Version,
					"status":        r.Status,
					"triggeredBy":   r.TriggeredBy,
					"startedAt":     r.StartedAt,
					"updatedAt":     r.UpdatedAt,
					"completedAt":   r.CompletedAt,
					"currentStep":   r.CurrentStep,
					"error":         r.Error,
					"resultPreview": r.ResultPreview,
				})
			}
			if !includeSteps {
				meta["hint"] = "List returns summaries. Use runs show for details."
			}

			return writeData(cmd, app, meta, map[string]any{
				"flowSlug": flow,
				"items":    items,
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Structured runs filter query (API mode only), e.g. 'status:failed flow:my-flow'")
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Filter by installation id (API mode only)")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Deprecated alias for --installation-id")
	_ = cmd.Flags().MarkHidden("profile-id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (API mode only)")
	cmd.Flags().IntVar(&version, "version", 0, "Filter by flow version active when the run started (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Limit results (0 = all)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	cmd.Flags().BoolVar(&includeSteps, "include-steps", false, "Include step arrays in list results")
	return cmd
}

func newRunsShowCmd(app *App) *cobra.Command {
	var steps int
	var installationID string
	var profileID string
	var includeSteps bool
	var includeResult bool
	var full bool
	var errorsOnly bool
	cmd := &cobra.Command{
		Use:   "show <workflow-id>",
		Short: "Show run detail",
		Long: `Show run detail.

To access run resources, use the resources command:
  breyta resources workflow list <workflow-id>  # List all resources for workflow
  breyta resources read <resource-uri>          # Read resource content
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"workflowId": args[0]}
				effectiveInstallationID := strings.TrimSpace(installationID)
				if effectiveInstallationID == "" {
					effectiveInstallationID = strings.TrimSpace(profileID)
				}
				if effectiveInstallationID != "" {
					payload["installationId"] = effectiveInstallationID
				}
				if errorsOnly {
					payload["includeSteps"] = true
					payload["includeResult"] = full
				} else if full {
					payload["includeSteps"] = true
					payload["includeResult"] = true
				} else {
					payload["includeSteps"] = includeSteps
					payload["includeResult"] = includeResult
				}
				out, status, err := runAPICommand(app, "runs.get", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				if errorsOnly {
					filterRunErrorsOnly(out, args[0])
				} else {
					annotateRunsShowDefaultOutput(out, args[0], includeSteps || includeResult || full)
				}
				if err := writeAPIResult(cmd, app, out, status); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}

			// Progressive disclosure: truncate steps unless caller requests all.
			out := *r
			meta := map[string]any{
				"stepsTotal": len(r.Steps),
				"stepsShown": len(r.Steps),
				"truncated":  false,
			}
			if steps > 0 && steps < len(r.Steps) {
				out.Steps = out.Steps[:steps]
				meta["stepsShown"] = steps
				meta["truncated"] = true
				meta["hint"] = "Use --steps 0 to show all steps"
			}

			return writeData(cmd, app, meta, map[string]any{"run": &out})
		},
	}
	cmd.Flags().IntVar(&steps, "steps", 20, "Number of steps to include (0 = all)")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Advanced: lookup run using a specific installation id (API mode only)")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Deprecated alias for --installation-id")
	cmd.Flags().BoolVar(&includeSteps, "include-steps", false, "Include step arrays in API mode")
	cmd.Flags().BoolVar(&includeResult, "include-result", false, "Include full result payload in API mode")
	cmd.Flags().BoolVar(&full, "full", false, "Include full steps and result payload in API mode")
	cmd.Flags().BoolVar(&errorsOnly, "errors", false, "Show only run-level and failed step errors in API mode")
	_ = cmd.Flags().MarkHidden("profile-id")
	return cmd
}

func annotateRunsShowDefaultOutput(out map[string]any, workflowID string, expanded bool) {
	if out == nil || expanded {
		return
	}
	data := mapStringAny(out["data"])
	if data == nil {
		return
	}
	run := mapStringAny(data["run"])
	if run == nil {
		return
	}
	if steps, ok := run["steps"]; ok {
		if len(sliceAny(steps)) == 0 {
			delete(run, "steps")
			run["stepsOmitted"] = true
		}
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["stepsIncluded"] = false
	meta["stepsOmitted"] = true
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Run detail is compact; use nextCommands for steps/resources."
	}
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		workflowID = "<workflow-id>"
	}
	appendMetaNextCommands(meta,
		"breyta runs show "+workflowID+" --include-steps",
		"breyta runs inspect "+workflowID,
		"breyta resources workflow list "+workflowID)
}

func filterRunErrorsOnly(out map[string]any, workflowID string) {
	if out == nil {
		return
	}
	data := mapStringAny(out["data"])
	if data == nil {
		return
	}
	run := mapStringAny(data["run"])
	if run == nil {
		return
	}
	steps := sliceAny(run["steps"])
	errorSteps := make([]any, 0, len(steps))
	for _, item := range steps {
		step := mapStringAny(item)
		if runStepHasError(step) {
			errorSteps = append(errorSteps, item)
		}
	}
	filteredRun := make(map[string]any, len(run)+2)
	for key, value := range run {
		filteredRun[key] = value
	}
	filteredRun["steps"] = errorSteps
	filteredRun["errorStepsCount"] = len(errorSteps)
	data["run"] = filteredRun
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["errorsOnly"] = true
	meta["nextCommands"] = []string{
		"breyta runs show " + strings.TrimSpace(workflowID) + " --full",
		"breyta resources workflow list " + strings.TrimSpace(workflowID),
	}
}

func runStepHasError(step map[string]any) bool {
	if step == nil {
		return false
	}
	status := strings.ToLower(firstNonBlankString(step["status"]))
	if strings.Contains(status, "fail") || strings.Contains(status, "error") {
		return true
	}
	if errMap := mapStringAny(step["error"]); len(errMap) > 0 {
		return true
	}
	return firstNonBlankString(step["error"], step["errorMessage"], step["error-message"]) != ""
}

func newRunsStartCmd(app *App) *cobra.Command {
	var flow string
	var installationID string
	var profileID string
	var source string
	var version int
	var invocation string
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Advanced/legacy: start a new run",
		Long: strings.TrimSpace(`
Legacy compatibility command.

Prefer:
- breyta flows run <flow-slug> [--input '{...}'] [--wait]

Use runs start only when integrating with older scripts.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				payload := map[string]any{"flowSlug": flow}
				if strings.TrimSpace(source) != "" {
					payload["source"] = strings.TrimSpace(source)
				}
				if version > 0 {
					payload["version"] = version
				}
				if strings.TrimSpace(invocation) != "" {
					payload["invocation"] = strings.TrimSpace(invocation)
				}
				effectiveInstallationID := strings.TrimSpace(installationID)
				legacyProfileID := strings.TrimSpace(profileID)
				if effectiveInstallationID == "" {
					effectiveInstallationID = legacyProfileID
				}
				if effectiveInstallationID != "" {
					payload["profileId"] = effectiveInstallationID
				}
				if effectiveInstallationID == "" && app.DevMode {
					if runConfigID := loadRunConfigID(app); strings.TrimSpace(runConfigID) != "" {
						payload["profileId"] = strings.TrimSpace(runConfigID)
					}
				}
				if strings.TrimSpace(inputJSON) != "" {
					var v any
					if err := json.Unmarshal([]byte(inputJSON), &v); err != nil {
						return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
					}
					m, ok := v.(map[string]any)
					if !ok {
						return writeErr(cmd, errors.New("--input must be a JSON object"))
					}
					payload["input"] = m
				}

				client := apiClient(app)
				startResp, status, err := client.DoCommand(context.Background(), "runs.start", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				trackCommandTelemetry(app, "runs.start", payload, status, status < 400 && isOK(startResp))
				enrichCommandHints(app, "runs.start", payload, status, startResp)
				if !wait {
					return writeAPIResult(cmd, app, startResp, status)
				}
				if status >= 400 {
					return writeAPIResult(cmd, app, startResp, status)
				}

				dataAny := startResp["data"]
				data, _ := dataAny.(map[string]any)
				workflowID := workflowIDFromRunData(data)
				if strings.TrimSpace(workflowID) == "" {
					return writeErr(cmd, errors.New("missing data.workflowId in runs.start response"))
				}
				waitInstallationID := installationIDFromRunData(data)

				deadline := time.Now().Add(timeout)
				polls := 0
				var nextTerminalFallback time.Time
				for {
					pollPayload := compactRunsGetPayload(workflowID)
					if waitInstallationID != "" {
						pollPayload["installationId"] = waitInstallationID
					}
					execResp, execStatus, err := client.DoCommand(context.Background(), "runs.get", pollPayload)
					if err != nil {
						return writeErr(cmd, err)
					}
					// The execution store may lag slightly after runs.start returns.
					// Treat a transient 404 as "not visible yet" and retry until timeout.
					if execStatus == 404 {
						polls++
						if shouldCheckTerminalWaitFallback(polls, nextTerminalFallback) {
							nextTerminalFallback = time.Now().Add(terminalWaitFallbackInterval(poll))
							if finalResp, finalStatus, _, ok, err := terminalRunFallback(client, workflowID, waitInstallationID); err == nil && ok {
								return writeAPIResult(cmd, app, finalResp, finalStatus)
							}
						}
						if time.Now().After(deadline) {
							return writeAPIResult(cmd, app, execResp, execStatus)
						}
						time.Sleep(poll)
						continue
					}
					if execStatus >= 400 {
						return writeAPIResult(cmd, app, execResp, execStatus)
					}
					execDataAny := execResp["data"]
					execData, _ := execDataAny.(map[string]any)
					runAny := execData["run"]
					run, _ := runAny.(map[string]any)
					statusStr := canonicalRunStatus(run["status"])

					if isTerminalRunStatus(statusStr) {
						finalResp, finalStatus, err := hydrateTerminalWaitRun(client, workflowID, waitInstallationID)
						if err != nil {
							return writeErr(cmd, err)
						}
						if finalStatus >= 400 {
							finalResp = execResp
							finalStatus = execStatus
						}
						return writeAPIResult(cmd, app, finalResp, finalStatus)
					}
					polls++
					if shouldCheckTerminalWaitFallback(polls, nextTerminalFallback) {
						nextTerminalFallback = time.Now().Add(terminalWaitFallbackInterval(poll))
						if finalResp, finalStatus, _, ok, err := terminalRunFallback(client, workflowID, waitInstallationID); err == nil && ok {
							return writeAPIResult(cmd, app, finalResp, finalStatus)
						}
					}
					if time.Now().After(deadline) {
						// Important UX: still return the workflowId so callers can continue
						// with `breyta runs show <workflow-id>` or inspect waits.
						timeoutOut := map[string]any{
							"ok": false,
							"error": map[string]any{
								"message": fmt.Sprintf("timed out waiting for run completion (workflowId=%s)", workflowID),
								"details": map[string]any{
									"workflowId": workflowID,
									"timeoutMs":  timeout.Milliseconds(),
									"pollMs":     poll.Milliseconds(),
								},
							},
							"meta": map[string]any{
								"timedOut": true,
								"hint":     "Run may still be active. Check runs show or waits list.",
							},
							"data": map[string]any{
								"workflowId": workflowID,
								"start":      startResp,
								"lastPoll":   execResp,
							},
						}
						return writeAPIResult(cmd, app, timeoutOut, 200)
					}
					time.Sleep(poll)
				}
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.StartRun(st, flow, version)
			if err != nil {
				return writeErr(cmd, err)
			}
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"run": r})
		},
	}
	cmd.Flags().StringVar(&flow, "flow", "", "Flow slug")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Installation id to run under (API mode only)")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Deprecated alias for --installation-id")
	_ = cmd.Flags().MarkHidden("profile-id")
	cmd.Flags().StringVar(&source, "source", "", "Deprecated source selector (API mode only)")
	_ = cmd.Flags().MarkHidden("source")
	cmd.Flags().IntVar(&version, "version", 0, "Version (default active)")
	cmd.Flags().StringVar(&invocation, "invocation", "", "Named invocation input contract (API mode only)")
	cmd.Flags().StringVar(&invocation, "invocation-id", "", "Named invocation input contract (API mode only)")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input (API mode only)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run to complete (API mode only)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout (API mode only)")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting (API mode only)")
	must(cmd.MarkFlagRequired("flow"))
	cmd.Hidden = true
	return cmd
}

func doRunsReplayAPI(cmd *cobra.Command, app *App, runID string) error {
	workflowID := strings.TrimSpace(runID)
	if workflowID == "" {
		return writeErr(cmd, errors.New("missing run id"))
	}
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	out, status, err := runAPICommand(app, "runs.replay", map[string]any{"workflowId": workflowID})
	if err != nil {
		return writeErr(cmd, err)
	}
	return writeAPIResult(cmd, app, out, status)
}

func newRunsReplayCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <run-id>",
		Short: "Replay a failed webhook run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doRunsReplayAPI(cmd, app, args[0])
			}
			runID := args[0]
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			orig, err := store.GetRun(st, runID)
			if err != nil {
				return writeErr(cmd, err)
			}
			// Create a new run that represents "replay" of the original.
			now := time.Now().UTC()
			newID := "replay-" + runID + "-" + now.Format("150405")
			replayed := *orig
			replayed.WorkflowID = newID
			replayed.StartedAt = now
			replayed.UpdatedAt = now
			replayed.TriggeredBy = "replay"
			// Mark as completed immediately (mock).
			replayed.Status = "completed"
			replayed.CurrentStep = ""
			replayed.CompletedAt = ptrTime(now)
			ws, _ := getWorkspace(st, app.WorkspaceID)
			if ws != nil {
				ws.Runs[newID] = &replayed
			}
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{
				"originalRunId": runID,
				"replayRunId":   newID,
				"run":           &replayed,
			})
		},
	}
	return cmd
}

func newRunsStepCmd(app *App) *cobra.Command {
	var installationID string
	var full bool
	cmd := &cobra.Command{
		Use:   "step <run-id> <step-id>",
		Short: "Show compact step input/output detail for a run",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doRunsStepInspect(cmd, app, args[0], args[1], installationID, full)
			}
			return writeLocalRunStepInspect(cmd, app, args[0], args[1])
		},
	}
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Lookup run using a specific installation id (API mode only)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full captured step output when available (API mode only)")
	return cmd
}

func newRunsInspectCmd(app *App) *cobra.Command {
	var stepID string
	var installationID string
	var full bool
	cmd := &cobra.Command{
		Use:   "inspect <workflow-id>",
		Short: "Inspect a run or one step's compact I/O",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(stepID) != "" {
				if !isAPIMode(app) {
					return writeLocalRunStepInspect(cmd, app, args[0], stepID)
				}
				return doRunsStepInspect(cmd, app, args[0], stepID, installationID, full)
			}
			if !isAPIMode(app) {
				return writeLocalRunInspect(cmd, app, args[0])
			}
			payload := map[string]any{
				"workflowId":     strings.TrimSpace(args[0]),
				"includeSteps":   true,
				"includeResult":  false,
				"compactInspect": true,
			}
			if strings.TrimSpace(installationID) != "" {
				payload["installationId"] = strings.TrimSpace(installationID)
			}
			out, status, err := runAPICommand(app, "runs.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status < 400 && isOK(out) {
				compactRunInspectOutput(out, args[0])
				reconcileRunResponseWithTerminalEvents(apiClient(app), out, strings.TrimSpace(args[0]), installationID)
			}
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&stepID, "step", "", "Show compact I/O for one step id/title")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Lookup run using a specific installation id (API mode only)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full captured step output with --step in API mode")
	return cmd
}

func compactRunInspectOutput(out map[string]any, workflowID string) {
	data := mapStringAny(out["data"])
	run := mapStringAny(data["run"])
	if run == nil {
		return
	}
	steps := sliceAny(run["steps"])
	if steps != nil {
		compactSteps := make([]any, 0, len(steps))
		for _, item := range steps {
			if step := mapStringAny(item); step != nil {
				compactSteps = append(compactSteps, compactAPIRunStepExecution(step))
			}
		}
		run["steps"] = compactSteps
	}
	if firstPresent(run, "inputPreview", "input-preview", "input", "paramsPreview", "params-preview") != nil {
		run["hasInput"] = true
	}
	if firstPresent(run, "resultPreview", "result-preview", "outputPreview", "output-preview", "output", "result") != nil {
		run["hasResult"] = true
	}
	if errValue := firstPresent(run, "error", "errorMessage", "error-message"); errValue != nil {
		run["error"] = compactRunErrorValue(errValue)
	}
	for _, key := range []string{"input", "params", "result", "output"} {
		delete(run, key)
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["workflowId"] = strings.TrimSpace(workflowID)
	meta["outputView"] = "compact"
	meta["compactInspect"] = true
	meta["stepsTotal"] = len(steps)
	if _, ok := meta["hint"]; !ok {
		meta["hint"] = "Run inspection is compact. Use --full only for full payloads."
	}
}

func writeLocalRunInspect(cmd *cobra.Command, app *App, runID string) error {
	st, store, err := appStore(app)
	if err != nil {
		return writeErr(cmd, err)
	}
	r, err := store.GetRun(st, runID)
	if err != nil {
		return writeErr(cmd, err)
	}
	steps := make([]map[string]any, 0, len(r.Steps))
	for i := range r.Steps {
		steps = append(steps, compactLocalStepExecution(&r.Steps[i]))
	}
	meta := map[string]any{
		"workflowId": strings.TrimSpace(runID),
		"stepsTotal": len(r.Steps),
		"compact":    true,
		"nextCommands": []string{
			"breyta runs show " + strings.TrimSpace(runID) + " --steps 0",
		},
	}
	if len(r.Steps) > 0 {
		meta["nextCommands"] = append(meta["nextCommands"].([]string),
			"breyta runs inspect "+strings.TrimSpace(runID)+" --step "+r.Steps[0].StepID)
	}
	return writeData(cmd, app, meta, map[string]any{
		"workflowId":  r.WorkflowID,
		"flowSlug":    r.FlowSlug,
		"version":     r.Version,
		"status":      r.Status,
		"currentStep": r.CurrentStep,
		"hasInput":    r.InputPreview != nil,
		"hasResult":   r.ResultPreview != nil,
		"error":       r.Error,
		"steps":       steps,
	})
}

func writeLocalRunStepInspect(cmd *cobra.Command, app *App, runID string, stepID string) error {
	st, store, err := appStore(app)
	if err != nil {
		return writeErr(cmd, err)
	}
	r, err := store.GetRun(st, runID)
	if err != nil {
		return writeErr(cmd, err)
	}
	ws, err := getWorkspace(st, app.WorkspaceID)
	if err != nil {
		return writeErr(cmd, err)
	}
	f := ws.Flows[r.FlowSlug]
	if f == nil {
		return writeErr(cmd, errors.New("flow not found for run"))
	}
	var flowStep *state.FlowStep
	for i := range f.Steps {
		if localRunStepMatches(f.Steps[i].ID, f.Steps[i].Title, stepID) {
			flowStep = &f.Steps[i]
			break
		}
	}
	var execStep *state.StepExecution
	for i := range r.Steps {
		if localRunStepMatches(r.Steps[i].StepID, r.Steps[i].Title, stepID) {
			execStep = &r.Steps[i]
			break
		}
	}
	if execStep == nil {
		return writeErr(cmd, errors.New("step execution not found"))
	}

	return writeData(cmd, app, nil, map[string]any{
		"runId":      runID,
		"flowSlug":   r.FlowSlug,
		"stepId":     execStep.StepID,
		"status":     execStep.Status,
		"type":       execStep.StepType,
		"title":      execStep.Title,
		"durationMs": execStep.DurationMs,
		"input":      execStep.InputPreview,
		"output":     execStep.ResultPreview,
		"error":      execStep.Error,
		"execution":  compactLocalStepExecution(execStep),
		"flowStep": map[string]any{
			"id":    flowStepID(flowStep, execStep.StepID),
			"type":  flowStepType(flowStep),
			"title": flowStepTitle(flowStep),
		},
	})
}

func localRunStepMatches(stepID string, title string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, candidate := range []string{stepID, title} {
		if strings.ToLower(strings.TrimSpace(candidate)) == needle {
			return true
		}
	}
	return false
}

func flowStepID(s *state.FlowStep, fallback string) string {
	if s == nil || strings.TrimSpace(s.ID) == "" {
		return fallback
	}
	return s.ID
}

func compactLocalStepExecution(execStep *state.StepExecution) map[string]any {
	if execStep == nil {
		return nil
	}
	return map[string]any{
		"stepId":     execStep.StepID,
		"type":       execStep.StepType,
		"title":      execStep.Title,
		"status":     execStep.Status,
		"attempt":    execStep.Attempt,
		"durationMs": execStep.DurationMs,
		"hasInput":   execStep.InputPreview != nil,
		"hasOutput":  execStep.ResultPreview != nil,
		"error":      execStep.Error,
	}
}

func doRunsStepInspect(cmd *cobra.Command, app *App, workflowID string, stepID string, installationID string, full bool) error {
	if strings.TrimSpace(stepID) == "" {
		return writeErr(cmd, errors.New("missing step id"))
	}
	payload := map[string]any{
		"workflowId":         strings.TrimSpace(workflowID),
		"includeSteps":       true,
		"includeResult":      false,
		"includeStepResults": full,
		"stepId":             strings.TrimSpace(stepID),
	}
	if strings.TrimSpace(installationID) != "" {
		payload["installationId"] = strings.TrimSpace(installationID)
	}
	out, status, err := runAPICommand(app, "runs.get", payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	if status >= 400 || !isOK(out) {
		return writeAPIResult(cmd, app, out, status)
	}
	run := mapStringAny(mapStringAny(out["data"])["run"])
	step := findRunStep(run, stepID)
	if step == nil {
		return writeErr(cmd, fmt.Errorf("step %q not found in run %s", stepID, workflowID))
	}
	flowSlug := firstNonBlankString(run["flowSlug"], run["flow-slug"])
	stepIDOut := firstNonBlankString(step["stepId"], step["step-id"], step["id"], stepID)
	meta := map[string]any{
		"workflowId":    strings.TrimSpace(workflowID),
		"stepId":        stepIDOut,
		"sourceCommand": "runs.get",
		"outputView":    map[bool]string{true: "full", false: "compact"}[full],
		"nextCommands": []string{
			"breyta runs show " + strings.TrimSpace(workflowID) + " --include-steps",
			"breyta runs events " + strings.TrimSpace(workflowID) + " --step " + shellSingleQuote(stepIDOut),
			"breyta runs step " + strings.TrimSpace(workflowID) + " " + shellSingleQuote(stepIDOut) + " --full",
		},
	}
	data := map[string]any{
		"workflowId":     strings.TrimSpace(workflowID),
		"flowSlug":       flowSlug,
		"stepId":         stepIDOut,
		"status":         firstNonBlankString(step["status"]),
		"type":           firstNonBlankString(step["stepType"], step["step-type"], step["type"]),
		"title":          firstNonBlankString(step["title"], step["label"]),
		"durationMs":     firstPresent(step, "durationMs", "duration-ms"),
		"input":          firstPresent(step, "inputPreview", "input-preview", "input", "paramsPreview", "params-preview"),
		"output":         firstPresent(step, "output", "result", "resultPreview", "result-preview", "outputPreview", "output-preview"),
		"outputResource": firstPresent(step, "outputResource", "output-resource"),
		"errorResource":  firstPresent(step, "errorResource", "error-resource"),
		"error":          firstPresent(step, "error", "errorMessage", "error-message"),
		"cost":           firstPresent(step, "cost", "usage", "counters", "metering"),
		"execution":      compactAPIRunStepExecution(step),
	}
	if events, eventsMeta := fetchAPIRunEvents(app, strings.TrimSpace(workflowID), stepIDOut, installationID, 50); events != nil {
		data["events"] = events
		meta["eventCount"] = len(events)
		meta["eventsSourceCommand"] = "runs.events"
		if count, ok := eventsMeta["count"]; ok {
			meta["eventsTotal"] = count
		}
	}
	return writeData(cmd, app, meta, data)
}

func fetchAPIRunEvents(app *App, workflowID string, stepID string, installationID string, limit int) ([]any, map[string]any) {
	payload := map[string]any{
		"workflowId": strings.TrimSpace(workflowID),
	}
	if strings.TrimSpace(stepID) != "" {
		payload["stepId"] = strings.TrimSpace(stepID)
	}
	if strings.TrimSpace(installationID) != "" {
		payload["installationId"] = strings.TrimSpace(installationID)
	}
	if limit > 0 {
		payload["limit"] = limit
	}
	out, status, err := runAPICommand(app, "runs.events", payload)
	if err != nil || status >= 400 || !isOK(out) {
		return nil, nil
	}
	data := mapStringAny(out["data"])
	items := sliceAny(data["items"])
	if items == nil {
		items = []any{}
	}
	return items, mapStringAny(out["meta"])
}

func compactAPIRunStepExecution(step map[string]any) map[string]any {
	if step == nil {
		return nil
	}
	return compactNonEmptyFields(map[string]any{
		"stepId":     firstNonBlankString(step["stepId"], step["step-id"], step["id"]),
		"type":       firstNonBlankString(step["stepType"], step["step-type"], step["type"]),
		"title":      firstNonBlankString(step["title"], step["label"]),
		"status":     firstNonBlankString(step["status"]),
		"attempt":    firstPresent(step, "attempt", "attemptNumber", "attempt-number"),
		"durationMs": firstPresent(step, "durationMs", "duration-ms"),
		"hasInput":   firstPresent(step, "inputPreview", "input-preview", "input", "paramsPreview", "params-preview") != nil,
		"hasOutput":  firstPresent(step, "resultPreview", "result-preview", "outputPreview", "output-preview", "output", "result") != nil,
		"error":      compactRunErrorValue(firstPresent(step, "error", "errorMessage", "error-message")),
		"cost":       firstPresent(step, "cost", "usage", "counters", "metering"),
	})
}

func compactRunErrorValue(value any) any {
	if value == nil {
		return nil
	}
	if m := mapStringAny(value); m != nil {
		out := map[string]any{}
		for _, key := range []string{"type", "errorType", "error-type", "code", "status", "url", "contentType", "content-type", "message"} {
			if v := firstPresentAny(m[key]); v != nil {
				out[key] = compactRunErrorScalar(v)
			}
		}
		if ctx := mapStringAny(m["context"]); ctx != nil {
			out["context"] = compactNonEmptyFields(map[string]any{
				"status": firstPresentAny(ctx["status"]),
				"url":    firstNonBlankString(ctx["url"]),
			})
		}
		if len(out) == 0 {
			out["message"] = compactRunErrorScalar(value)
		}
		return out
	}
	return compactRunErrorScalar(value)
}

func compactRunErrorScalar(value any) any {
	s := scalarString(value)
	if s == "" {
		return value
	}
	if len([]rune(s)) <= 500 {
		return s
	}
	preview, _ := truncateRunesWithFlag(s, 500)
	return map[string]any{
		"preview":   preview,
		"truncated": true,
		"runes":     len([]rune(s)),
	}
}

func findRunStep(run map[string]any, stepID string) map[string]any {
	needle := strings.ToLower(strings.TrimSpace(stepID))
	for _, item := range sliceAny(run["steps"]) {
		step := mapStringAny(item)
		if step == nil {
			continue
		}
		for _, value := range []any{step["stepId"], step["step-id"], step["id"], step["name"], step["title"]} {
			if strings.ToLower(strings.TrimSpace(scalarString(value))) == needle {
				return step
			}
		}
	}
	return nil
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

func newRunsContinueCmd(app *App) *cobra.Command {
	var approveLatestWait bool
	var limit int
	cmd := &cobra.Command{
		Use:   "continue <workflow-id>",
		Short: "Continue a waiting run",
		Long: strings.TrimSpace(`
Continue a waiting run by applying an explicit wait action. The first supported
action is --approve-latest-wait, which finds the most recent active approval
wait for the workflow and approves it through the wait API.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !approveLatestWait {
				return writeErr(cmd, errors.New("choose a continue action, e.g. --approve-latest-wait"))
			}
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "runs continue is supported in API mode only")
			}
			return doRunsContinueApproveLatestWait(cmd, app, strings.TrimSpace(args[0]), limit)
		},
	}
	cmd.Flags().BoolVar(&approveLatestWait, "approve-latest-wait", false, "Approve the most recent active approval wait for this workflow")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max active waits to inspect while selecting the latest approval wait")
	return cmd
}

func doRunsContinueApproveLatestWait(cmd *cobra.Command, app *App, workflowID string, limit int) error {
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	if workflowID == "" {
		return writeErr(cmd, errors.New("missing workflow id"))
	}
	if limit <= 0 {
		limit = 25
	}
	q := url.Values{}
	q.Set("workflowId", workflowID)
	q.Set("limit", fmt.Sprintf("%d", limit))
	listOut, listStatus, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/waits", q, nil)
	if err != nil {
		return writeErr(cmd, err)
	}
	if listStatus >= 400 {
		return writeREST(cmd, app, listStatus, listOut)
	}
	items := waitItems(listOut)
	items = filterWaitItemsForWorkflow(items, workflowID)
	if len(items) == 0 {
		return writeErr(cmd, fmt.Errorf("no active waits found for workflow %s", workflowID))
	}
	selected := latestApprovableWait(items)
	if selected == nil {
		return writeFailure(
			cmd,
			app,
			"no_approval_wait",
			fmt.Errorf("no active approval wait found for workflow %s", workflowID),
			"--approve-latest-wait requires an active wait with approval metadata or an approve action. Use `breyta waits list --workflow-id "+workflowID+"` to inspect the wait shape.",
			map[string]any{
				"workflowId":         workflowID,
				"activeWaitCount":    len(items),
				"requiredWaitShape":  "wait.approval or wait.actions must contain approve",
				"ineligibleWaits":    waitIneligibilityDetails(items),
				"nextCommand":        "breyta waits list --workflow-id " + workflowID,
				"manualInspect":      "breyta runs show " + workflowID + " --include-steps",
				"continueActionFlag": "--approve-latest-wait",
			},
		)
	}
	waitID := waitIDValue(selected)
	if waitID == "" {
		return writeErr(cmd, errors.New("selected wait is missing waitId"))
	}
	approveOut, approveStatus, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/approve", nil, nil)
	if err != nil {
		return writeErr(cmd, err)
	}
	if approveStatus >= 400 {
		return writeREST(cmd, app, approveStatus, approveOut)
	}
	return writeData(cmd, app, map[string]any{
		"workflowId": workflowID,
		"waitId":     waitID,
		"action":     "approve",
		"nextCommands": []string{
			"breyta runs show " + workflowID,
			"breyta waits list --workflow-id " + workflowID,
		},
	}, map[string]any{
		"workflowId": workflowID,
		"continued":  true,
		"action":     "approve",
		"wait":       selected,
		"approval":   approveOut,
	})
}

func filterWaitItemsForWorkflow(items []map[string]any, workflowID string) []map[string]any {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return items
	}
	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		itemWorkflowID := firstNonBlankString(item["workflowId"], item["workflow-id"], item["runId"], item["run-id"])
		if itemWorkflowID == "" || itemWorkflowID == workflowID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func waitItems(out any) []map[string]any {
	root := mapStringAny(out)
	if root == nil {
		return nil
	}
	data := mapStringAny(root["data"])
	var raw []any
	switch {
	case data != nil && len(sliceAny(data["items"])) > 0:
		raw = sliceAny(data["items"])
	case data != nil && len(sliceAny(data["waits"])) > 0:
		raw = sliceAny(data["waits"])
	case len(sliceAny(root["items"])) > 0:
		raw = sliceAny(root["items"])
	case len(sliceAny(root["waits"])) > 0:
		raw = sliceAny(root["waits"])
	}
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m := mapStringAny(item); m != nil {
			items = append(items, m)
		}
	}
	return items
}

func latestApprovableWait(items []map[string]any) map[string]any {
	var selected map[string]any
	var selectedTime time.Time
	for i, item := range items {
		if !waitLooksApprovable(item) {
			continue
		}
		t := waitSortTime(item)
		if selected == nil || t.After(selectedTime) || (t.IsZero() && selectedTime.IsZero() && i == 0) {
			selected = item
			selectedTime = t
		}
	}
	return selected
}

func waitLooksApprovable(wait map[string]any) bool {
	if wait == nil {
		return false
	}
	switch strings.ToLower(firstNonBlankString(wait["status"], wait["state"])) {
	case "completed", "complete", "cancelled", "canceled", "rejected", "expired", "failed":
		return false
	}
	if approval := mapStringAny(wait["approval"]); approval != nil {
		if actions := stringSlice(approval["actions"]); len(actions) > 0 {
			return containsFold(actions, "approve")
		}
		return true
	}
	if actions := stringSlice(wait["actions"]); len(actions) > 0 {
		return containsFold(actions, "approve")
	}
	notify := firstPresent(wait, "notify", "notification")
	if notify == nil {
		return false
	}
	encoded, err := json.Marshal(notify)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(encoded)), "approve")
}

func waitIneligibilityDetails(items []map[string]any) []map[string]any {
	details := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil || waitLooksApprovable(item) {
			continue
		}
		status := strings.ToLower(firstNonBlankString(item["status"], item["state"]))
		reason := "missing approval metadata or approve action"
		switch status {
		case "completed", "complete", "cancelled", "canceled", "rejected", "expired", "failed":
			reason = "wait is not active"
		}
		detail := map[string]any{
			"waitId": waitIDValue(item),
			"status": status,
			"reason": reason,
		}
		if stepID := firstNonBlankString(item["stepId"], item["step-id"], item["step"], item["currentStep"]); stepID != "" {
			detail["stepId"] = stepID
		}
		if actions := stringSlice(item["actions"]); len(actions) > 0 {
			detail["actions"] = actions
		}
		if approval := mapStringAny(item["approval"]); approval != nil {
			detail["approvalActions"] = stringSlice(approval["actions"])
		}
		details = append(details, detail)
	}
	return details
}

func containsFold(items []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == needle {
			return true
		}
	}
	return false
}

func waitSortTime(wait map[string]any) time.Time {
	for _, key := range []string{"registeredAt", "registered-at", "createdAt", "created-at", "updatedAt", "updated-at", "expiresAt", "expires-at"} {
		raw := firstNonBlankString(wait[key])
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func waitIDValue(wait map[string]any) string {
	return firstNonBlankString(wait["waitId"], wait["wait-id"], wait["id"])
}

func ptrTime(t time.Time) *time.Time { return &t }

func flowStepType(s *state.FlowStep) string {
	if s == nil {
		return ""
	}
	return s.Type
}

func flowStepTitle(s *state.FlowStep) string {
	if s == nil {
		return ""
	}
	return s.Title
}

func newRunsCancelCmd(app *App) *cobra.Command {
	var reason string
	var force bool
	var flow string
	cmd := &cobra.Command{
		Use:   "cancel <workflow-id>",
		Short: "Cancel a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				workflowID := strings.TrimSpace(args[0])
				if looksLikeShortRunID(workflowID) {
					if err := requireAPI(app); err != nil {
						return writeErr(cmd, err)
					}
					client := apiClient(app)
					resolved, err := resolveRunIDForCancel(context.Background(), client, workflowID, flow)
					if err != nil {
						return writeErr(cmd, err)
					}
					workflowID = resolved
				}
				payload := map[string]any{"workflowId": workflowID}
				if strings.TrimSpace(reason) != "" {
					payload["reason"] = strings.TrimSpace(reason)
				}
				if force {
					payload["force"] = true
				}
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				client := apiClient(app)
				out, status, err := client.DoCommand(context.Background(), "runs.cancel", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				if workflowID != strings.TrimSpace(args[0]) {
					meta := ensureMeta(out)
					if meta != nil {
						meta["resolvedFrom"] = strings.TrimSpace(args[0])
						meta["workflowId"] = workflowID
					}
				}
				if status == http.StatusConflict && isAlreadyCompletedRun(out) {
					meta := ensureMeta(out)
					if meta != nil {
						meta["hint"] = "Run is already completed"
					}
					out["ok"] = true
					status = http.StatusOK
				}
				return writeAPIResult(cmd, app, out, status)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			if r.Status == "completed" || r.Status == "failed" || r.Status == "cancelled" || r.Status == "terminated" {
				return writeData(cmd, app, map[string]any{"hint": "Run is already terminal"}, map[string]any{"run": r})
			}
			now := time.Now().UTC()
			if force {
				r.Status = "terminated"
			} else {
				r.Status = "cancelled"
			}
			r.Error = reason
			r.CurrentStep = ""
			r.CompletedAt = &now
			r.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"run": r})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")
	cmd.Flags().BoolVar(&force, "force", false, "Terminate the run immediately")
	cmd.Flags().StringVar(&flow, "flow", "", "Flow slug to narrow short run-id resolution (API mode only)")
	return cmd
}

func looksLikeShortRunID(v string) bool {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "-")
	return shortRunIDPattern.MatchString(s)
}

func resolveRunIDForCancel(ctx context.Context, client apiCommandRunner, shortID string, flow string) (string, error) {
	shortID = strings.TrimSpace(shortID)
	normalized := strings.TrimPrefix(shortID, "-")
	suffix := "-" + normalized
	seen := map[string]struct{}{}
	matches := []string{}
	cursor := ""

	for {
		payload := map[string]any{"limit": 100}
		if strings.TrimSpace(flow) != "" {
			payload["flowSlug"] = strings.TrimSpace(flow)
		}
		if strings.TrimSpace(cursor) != "" {
			payload["cursor"] = strings.TrimSpace(cursor)
		}
		out, status, err := client.DoCommand(ctx, "runs.list", payload)
		if err != nil {
			return "", err
		}
		if status >= http.StatusBadRequest {
			return "", fmt.Errorf("api error (status=%d): %s", status, formatAPIError(out))
		}

		data, _ := out["data"].(map[string]any)
		items, _ := data["items"].([]any)
		for _, item := range items {
			row, _ := item.(map[string]any)
			workflowID, _ := row["workflowId"].(string)
			if strings.TrimSpace(workflowID) == "" {
				continue
			}
			if workflowID == shortID || strings.HasSuffix(workflowID, suffix) {
				if _, ok := seen[workflowID]; ok {
					continue
				}
				seen[workflowID] = struct{}{}
				matches = append(matches, workflowID)
			}
		}

		meta, _ := out["meta"].(map[string]any)
		hasMore, _ := meta["hasMore"].(bool)
		nextCursor, _ := meta["nextCursor"].(string)
		if !hasMore || strings.TrimSpace(nextCursor) == "" {
			break
		}
		cursor = nextCursor
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("run %q not found; pass full workflow-id or list runs with --flow and --cursor to locate it", shortID)
	}
	if len(matches) > 1 {
		preview := strings.Join(matches, ", ")
		if len(preview) > 240 {
			preview = preview[:240] + "..."
		}
		return "", fmt.Errorf("run id %q is ambiguous (%d matches); pass full workflow-id or add --flow <slug>. matches: %s", shortID, len(matches), preview)
	}
	return matches[0], nil
}

func isAlreadyCompletedRun(out map[string]any) bool {
	if out == nil {
		return false
	}
	errAny, ok := out["error"]
	if !ok {
		return false
	}
	errMap, ok := errAny.(map[string]any)
	if !ok {
		return false
	}
	code, _ := errMap["code"].(string)
	return strings.TrimSpace(code) == "already_completed"
}

func newRunsRetryCmd(app *App) *cobra.Command {
	var stepID string
	cmd := &cobra.Command{
		Use:   "retry <run-id>",
		Short: "Retry a failed webhook run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if strings.TrimSpace(stepID) != "" {
					return writeFailure(cmd, app, "not_implemented", errors.New("partial step retry is not implemented"), "Use `breyta runs replay <run-id>` to replay the whole failed webhook run.", nil)
				}
				return doRunsReplayAPI(cmd, app, args[0])
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			orig, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.StartRun(st, orig.FlowSlug, orig.Version)
			if err != nil {
				return writeErr(cmd, err)
			}
			r.TriggeredBy = "retry"
			if stepID != "" {
				// v1 placeholder: we don't yet support partial-step retries in the mock executor.
				// Keep it as metadata + hint.
			}
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			meta := map[string]any{"hint": "Partial step retries are planned; currently retries restart from step 1"}
			if stepID == "" {
				meta = nil
			}
			return writeData(cmd, app, meta, map[string]any{"originalRunId": orig.WorkflowID, "run": r})
		},
	}
	cmd.Flags().StringVar(&stepID, "step", "", "Retry from this step-id (planned)")
	return cmd
}

func newRunsEventsCmd(app *App) *cobra.Command {
	var stepID string
	var installationID string
	var limit int
	cmd := &cobra.Command{
		Use:   "events <run-id>",
		Short: "Show run event timeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("limit") && limit <= 0 {
				return writeErr(cmd, errors.New("--limit must be > 0"))
			}
			if isAPIMode(app) {
				payload := map[string]any{
					"workflowId": strings.TrimSpace(args[0]),
				}
				if strings.TrimSpace(stepID) != "" {
					payload["stepId"] = strings.TrimSpace(stepID)
				}
				if strings.TrimSpace(installationID) != "" {
					payload["installationId"] = strings.TrimSpace(installationID)
				}
				if limit > 0 {
					payload["limit"] = limit
				}
				out, status, err := runAPICommand(app, "runs.events", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				return writeAPIResult(cmd, app, out, status)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			events := filterLocalRunEvents(deriveRunEvents(r), stepID)
			if limit > 0 && len(events) > limit {
				events = events[:limit]
			}
			meta := map[string]any{"count": len(events)}
			return writeData(cmd, app, meta, map[string]any{"runId": r.WorkflowID, "items": events})
		},
	}
	cmd.Flags().StringVar(&stepID, "step", "", "Filter events by step id/title")
	cmd.Flags().StringVar(&installationID, "installation-id", "", "Lookup run using a specific installation id (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of events to return")
	return cmd
}

func filterLocalRunEvents(events []map[string]any, stepID string) []map[string]any {
	if strings.TrimSpace(stepID) == "" {
		return events
	}
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		if localRunStepMatches(scalarString(event["stepId"]), scalarString(event["title"]), stepID) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func newRunsLogsCmd(app *App) *cobra.Command {
	var stepID string
	cmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show run logs (mock only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				runID := strings.TrimSpace(args[0])
				return writeNotImplemented(cmd, app, "API run logs are not available yet. Use `breyta runs inspect "+runID+"` or `breyta runs step "+runID+" STEP_ID --full`.")
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			// We don't have real logs yet; return a structured placeholder.
			meta := map[string]any{"hint": "Logs are planned. Use runs events for a timeline today."}
			return writeData(cmd, app, meta, map[string]any{"runId": r.WorkflowID, "stepId": stepID, "items": []any{}})
		},
	}
	cmd.Flags().StringVar(&stepID, "step", "", "Filter logs by step-id")
	return cmd
}

func deriveRunEvents(r *state.Run) []map[string]any {
	items := []map[string]any{
		{"at": r.StartedAt, "type": "run_started", "runId": r.WorkflowID, "flowSlug": r.FlowSlug, "version": r.Version, "triggeredBy": r.TriggeredBy},
	}
	for _, s := range r.Steps {
		if !s.StartedAt.IsZero() {
			items = append(items, map[string]any{"at": s.StartedAt, "type": "step_started", "stepId": s.StepID, "stepType": s.StepType, "title": s.Title, "input": s.InputPreview})
		}
		if s.CompletedAt != nil && !s.CompletedAt.IsZero() {
			t := *s.CompletedAt
			ev := map[string]any{"at": t, "type": "step_completed", "stepId": s.StepID, "status": s.Status, "output": s.ResultPreview}
			if s.Error != "" {
				ev["error"] = s.Error
			}
			items = append(items, ev)
		}
	}
	if r.CompletedAt != nil && !r.CompletedAt.IsZero() {
		items = append(items, map[string]any{"at": *r.CompletedAt, "type": "run_completed", "status": r.Status, "error": r.Error, "result": r.ResultPreview})
	}
	sort.Slice(items, func(i, j int) bool {
		ti := items[i]["at"].(time.Time)
		tj := items[j]["at"].(time.Time)
		return ti.Before(tj)
	})
	return items
}
