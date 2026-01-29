package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

func newRunsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "runs", Aliases: []string{"run"}, Short: "Inspect and control runs"}
	cmd.AddCommand(newRunsListCmd(app))
	cmd.AddCommand(newRunsShowCmd(app))
	cmd.AddCommand(newRunsStartCmd(app))
	cmd.AddCommand(newRunsReplayCmd(app))
	cmd.AddCommand(newRunsStepCmd(app))
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
	return ""
}

func newRunsListCmd(app *App) *cobra.Command {
	var flow string
	var profileID string
	var status string
	var limit int
	var cursor string
	var includeSteps bool
	cmd := &cobra.Command{
		Use:   "list [flow-slug]",
		Short: "List runs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && flow == "" {
				flow = args[0]
			}
			if isAPIMode(app) {
				payload := map[string]any{}
				if strings.TrimSpace(flow) != "" {
					payload["flowSlug"] = strings.TrimSpace(flow)
				}
				if strings.TrimSpace(profileID) != "" {
					payload["profileId"] = strings.TrimSpace(profileID)
				}
				if strings.TrimSpace(status) != "" {
					payload["status"] = strings.TrimSpace(status)
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
				return doAPICommand(cmd, app, "runs.list", payload)
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
				meta["hint"] = "List returns summaries. Use `breyta runs show <workflow-id>` for full detail, or pass --include-steps."
			}

			return writeData(cmd, app, meta, map[string]any{
				"flowSlug": flow,
				"items":    items,
			})
		},
	}
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Filter by profile id (API mode only)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Limit results (0 = all)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	cmd.Flags().BoolVar(&includeSteps, "include-steps", false, "Include step arrays in list results")
	return cmd
}

func newRunsShowCmd(app *App) *cobra.Command {
	var steps int
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
				return doAPICommand(cmd, app, "runs.get", map[string]any{"workflowId": args[0]})
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
	return cmd
}

func newRunsStartCmd(app *App) *cobra.Command {
	var flow string
	var profileID string
	var version int
	var source string
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				payload := map[string]any{"flowSlug": flow}
				if version > 0 {
					payload["version"] = version
				}
				if strings.TrimSpace(profileID) != "" {
					payload["profileId"] = strings.TrimSpace(profileID)
				}
				if strings.TrimSpace(source) != "" && source != "active" {
					payload["source"] = source
				}
				if strings.TrimSpace(profileID) == "" && strings.TrimSpace(source) == "" && app.DevMode {
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

				deadline := time.Now().Add(timeout)
				for {
					execResp, execStatus, err := client.DoCommand(context.Background(), "runs.get", map[string]any{"workflowId": workflowID})
					if err != nil {
						return writeErr(cmd, err)
					}
					// The execution store may lag slightly after runs.start returns.
					// Treat a transient 404 as "not visible yet" and retry until timeout.
					if execStatus == 404 {
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
					statusStr, _ := run["status"].(string)

					if statusStr == "completed" || statusStr == "failed" || statusStr == "cancelled" || statusStr == "canceled" || statusStr == "terminated" || statusStr == "timed-out" || statusStr == "timed_out" {
						return writeAPIResult(cmd, app, execResp, execStatus)
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
								"hint":     "The run may still be in progress. Use `breyta runs show <workflow-id>` to check status, or `breyta waits list --workflow <workflow-id>` if the run is waiting for human input.",
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
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Profile id (instance) to run under (API mode only)")
	cmd.Flags().IntVar(&version, "version", 0, "Version (default active)")
	cmd.Flags().StringVar(&source, "source", "active", "Source (active|draft|latest)")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input (API mode only)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run to complete (API mode only)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Wait timeout (API mode only)")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting (API mode only)")
	must(cmd.MarkFlagRequired("flow"))
	return cmd
}

func newRunsReplayCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <run-id>",
		Short: "Replay a run (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (API replay is not implemented).")
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
			// Create a new run that represents “replay” of the original.
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
	cmd := &cobra.Command{
		Use:   "step <run-id> <step-id>",
		Short: "Show step detail for a run (mock)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (use `breyta runs show <workflow-id>` in API mode).")
			}
			runID := args[0]
			stepID := args[1]
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
				if f.Steps[i].ID == stepID {
					flowStep = &f.Steps[i]
					break
				}
			}
			var execStep *state.StepExecution
			for i := range r.Steps {
				if r.Steps[i].StepID == stepID {
					execStep = &r.Steps[i]
					break
				}
			}
			if execStep == nil {
				return writeErr(cmd, errors.New("step execution not found"))
			}

			// For "execution truth" we surface concrete input/output from the run.
			// (Schemas live on the flow step and can be returned separately if needed.)
			return writeData(cmd, app, nil, map[string]any{
				"runId":     runID,
				"flowSlug":  r.FlowSlug,
				"stepId":    stepID,
				"input":     execStep.InputPreview,
				"output":    execStep.ResultPreview,
				"execution": execStep,
				"flowStep": map[string]any{
					"id":    stepID,
					"type":  flowStepType(flowStep),
					"title": flowStepTitle(flowStep),
				},
			})
		},
	}
	return cmd
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
	cmd := &cobra.Command{
		Use:   "cancel <workflow-id>",
		Short: "Cancel a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"workflowId": args[0]}
				if strings.TrimSpace(reason) != "" {
					payload["reason"] = strings.TrimSpace(reason)
				}
				if force {
					payload["force"] = true
				}
				return doAPICommand(cmd, app, "runs.cancel", payload)
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
	return cmd
}

func newRunsRetryCmd(app *App) *cobra.Command {
	var stepID string
	cmd := &cobra.Command{
		Use:   "retry <run-id>",
		Short: "Retry a run (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (use `breyta runs start` to re-run in API mode).")
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
	cmd := &cobra.Command{
		Use:   "events <run-id>",
		Short: "Show run event timeline (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (API events stream is not implemented).")
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			r, err := store.GetRun(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			events := deriveRunEvents(r)
			meta := map[string]any{"count": len(events)}
			return writeData(cmd, app, meta, map[string]any{"runId": r.WorkflowID, "items": events})
		},
	}
	return cmd
}

func newRunsLogsCmd(app *App) *cobra.Command {
	var stepID string
	cmd := &cobra.Command{
		Use:   "logs <run-id>",
		Short: "Show run logs (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (API logs are not implemented).")
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
			meta := map[string]any{"hint": "Logs are planned (per-step, per-attempt). Use `runs events` for a timeline today."}
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
