package cli

import (
        "errors"
        "time"

        "breyta-cli/internal/state"

        "github.com/spf13/cobra"
)

func newRunsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "run", Aliases: []string{"runs"}, Short: "Inspect and control runs (mock)"}
        cmd.AddCommand(newRunsListCmd(app))
        cmd.AddCommand(newRunsShowCmd(app))
        cmd.AddCommand(newRunsStartCmd(app))
        cmd.AddCommand(newRunsReplayCmd(app))
        cmd.AddCommand(newRunsStepCmd(app))
        return cmd
}

func newRunsListCmd(app *App) *cobra.Command {
        var flow string
        var limit int
        var includeSteps bool
        cmd := &cobra.Command{
                Use:   "list [flow-slug]",
                Short: "List runs (mock)",
                Args:  cobra.MaximumNArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if len(args) == 1 && flow == "" {
                                flow = args[0]
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
                                        "workflowId":    r.WorkflowID,
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
                                meta["hint"] = "List returns summaries. Use `breyta run show <run-id>` for full detail, or pass --include-steps."
                        }

                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "flowSlug":    flow,
                                "meta":        meta,
                                "items":       items,
                        })
                },
        }
        cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
        cmd.Flags().IntVar(&limit, "limit", 25, "Limit results (0 = all)")
        cmd.Flags().BoolVar(&includeSteps, "include-steps", false, "Include step arrays in list results")
        return cmd
}

func newRunsShowCmd(app *App) *cobra.Command {
        var steps int
        cmd := &cobra.Command{
                Use:   "show <workflow-id>",
                Short: "Show run detail (mock)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
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

                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "meta":        meta,
                                "run":         &out,
                        })
                },
        }
        cmd.Flags().IntVar(&steps, "steps", 20, "Number of steps to include (0 = all)")
        return cmd
}

func newRunsStartCmd(app *App) *cobra.Command {
        var flow string
        var version int
        cmd := &cobra.Command{
                Use:   "start",
                Short: "Start a new run (mock)",
                RunE: func(cmd *cobra.Command, args []string) error {
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
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "run":         r,
                        })
                },
        }
        cmd.Flags().StringVar(&flow, "flow", "", "Flow slug")
        cmd.Flags().IntVar(&version, "version", 0, "Version (default active)")
        _ = cmd.MarkFlagRequired("flow")
        return cmd
}

func newRunsReplayCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "replay <run-id>",
                Short: "Replay a run (mock)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
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
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId":   app.WorkspaceID,
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
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "runId":       runID,
                                "flowSlug":    r.FlowSlug,
                                "stepId":      stepID,
                                "input":       execStep.InputPreview,
                                "output":      execStep.ResultPreview,
                                "execution":   execStep,
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
