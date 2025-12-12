package cli

import (
        "errors"
        "time"

        "breyta-cli/internal/state"

        "github.com/spf13/cobra"
)

func newFlowsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "flow", Aliases: []string{"flows"}, Short: "Inspect and edit flows (mock)"}
        cmd.AddCommand(newFlowsListCmd(app))
        cmd.AddCommand(newFlowsShowCmd(app))
        cmd.AddCommand(newFlowsSpineCmd(app))
        cmd.AddCommand(newFlowCreateCmd(app))
        cmd.AddCommand(newFlowDeployCmd(app))
        cmd.AddCommand(newFlowStepSetCmd(app))
        return cmd
}

func newFlowsListCmd(app *App) *cobra.Command {
        var limit int
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List flows (mock)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        flows, err := store.ListFlows(st)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if limit > 0 && limit < len(flows) {
                                flows = flows[:limit]
                        }

                        // Include simple aggregates based on runs.
                        runs, _ := store.ListRuns(st, "")
                        activeCount := map[string]int{}
                        lastStatus := map[string]string{}
                        lastWorkflow := map[string]string{}
                        for _, r := range runs {
                                if r.Status == "running" {
                                        activeCount[r.FlowSlug]++
                                }
                                if _, ok := lastStatus[r.FlowSlug]; !ok {
                                        lastStatus[r.FlowSlug] = r.Status
                                        lastWorkflow[r.FlowSlug] = r.WorkflowID
                                }
                        }

                        items := make([]map[string]any, 0, len(flows))
                        for _, f := range flows {
                                items = append(items, map[string]any{
                                        "flowSlug":       f.Slug,
                                        "name":           f.Name,
                                        "activeVersion":  f.ActiveVersion,
                                        "activeCount":    activeCount[f.Slug],
                                        "lastStatus":     lastStatus[f.Slug],
                                        "lastWorkflowId": lastWorkflow[f.Slug],
                                })
                        }

                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "items":       items,
                        })
                },
        }
        cmd.Flags().IntVar(&limit, "limit", 0, "Limit results")
        return cmd
}

func newFlowsShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <flow-slug>",
                Short: "Show flow details (mock)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        f, err := store.GetFlow(st, args[0])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "flow":        f,
                        })
                },
        }
        return cmd
}

func newFlowsSpineCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "spine <flow-slug>",
                Short: "Render a textual spine for a flow (mock)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        f, err := store.GetFlow(st, args[0])
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        lines := []string{"Flow: " + f.Name, ""}
                        for i, s := range f.Steps {
                                prefix := "├─"
                                if i == len(f.Steps)-1 {
                                        prefix = "└─"
                                }
                                lines = append(lines, prefix+" "+s.ID+"  ("+s.Type+")  "+s.Title)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "flowSlug":    f.Slug,
                                "spine":       lines,
                        })
                },
        }
        return cmd
}

func newFlowCreateCmd(app *App) *cobra.Command {
        var slug, name, description string
        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create a new flow (mock)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        if slug == "" {
                                return writeErr(cmd, errors.New("missing --slug"))
                        }
                        if name == "" {
                                name = slug
                        }
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        ws, err := getWorkspace(st, app.WorkspaceID)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if ws.Flows == nil {
                                ws.Flows = map[string]*state.Flow{}
                        }
                        if _, exists := ws.Flows[slug]; exists {
                                return writeErr(cmd, errors.New("flow already exists"))
                        }
                        now := time.Now().UTC()
                        f := &state.Flow{
                                Slug:          slug,
                                Name:          name,
                                Description:   description,
                                Tags:          []string{"draft"},
                                ActiveVersion: 1,
                                UpdatedAt:     now,
                                Spine:         []string{"(empty)"},
                                Steps:         []state.FlowStep{},
                        }
                        ws.Flows[slug] = f
                        ws.UpdatedAt = now
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"workspaceId": app.WorkspaceID, "flow": f})
                },
        }
        cmd.Flags().StringVar(&slug, "slug", "", "Flow slug")
        cmd.Flags().StringVar(&name, "name", "", "Flow display name")
        cmd.Flags().StringVar(&description, "description", "", "Flow description")
        _ = cmd.MarkFlagRequired("slug")
        return cmd
}

func newFlowDeployCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "deploy <flow-slug>",
                Short: "Bump version and mark as deployed (mock)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        ws, err := getWorkspace(st, app.WorkspaceID)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        f := ws.Flows[args[0]]
                        if f == nil {
                                return writeErr(cmd, errors.New("flow not found"))
                        }
                        f.ActiveVersion++
                        f.UpdatedAt = time.Now().UTC()
                        ws.UpdatedAt = f.UpdatedAt
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"workspaceId": app.WorkspaceID, "flow": f})
                },
        }
        return cmd
}

func newFlowStepSetCmd(app *App) *cobra.Command {
        var (
                stepType     string
                title        string
                inputSchema  string
                outputSchema string
                definition   string
        )
        cmd := &cobra.Command{
                Use:   "step-set <flow-slug> <step-id>",
                Short: "Create or update a step (mock)",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        flowSlug := args[0]
                        stepID := args[1]

                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        ws, err := getWorkspace(st, app.WorkspaceID)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        f := ws.Flows[flowSlug]
                        if f == nil {
                                return writeErr(cmd, errors.New("flow not found"))
                        }
                        if stepType == "" {
                                stepType = "code"
                        }
                        if title == "" {
                                title = stepID
                        }
                        step := state.FlowStep{
                                ID:           stepID,
                                Type:         stepType,
                                Title:        title,
                                InputSchema:  inputSchema,
                                OutputSchema: outputSchema,
                                Definition:   definition,
                        }
                        found := false
                        for i := range f.Steps {
                                if f.Steps[i].ID == stepID {
                                        f.Steps[i] = step
                                        found = true
                                        break
                                }
                        }
                        if !found {
                                f.Steps = append(f.Steps, step)
                        }
                        // Recompute simple spine
                        lines := []string{"Flow: " + f.Name, ""}
                        for i, s := range f.Steps {
                                prefix := "├─"
                                if i == len(f.Steps)-1 {
                                        prefix = "└─"
                                }
                                lines = append(lines, prefix+" "+s.ID+"  ("+s.Type+")  "+s.Title)
                        }
                        f.Spine = lines
                        f.UpdatedAt = time.Now().UTC()
                        ws.UpdatedAt = f.UpdatedAt

                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"workspaceId": app.WorkspaceID, "flow": f})
                },
        }
        cmd.Flags().StringVar(&stepType, "type", "", "Step type (http|code|wait|notify|llm)")
        cmd.Flags().StringVar(&title, "title", "", "Step title")
        cmd.Flags().StringVar(&inputSchema, "input-schema", "", "Input schema (string)")
        cmd.Flags().StringVar(&outputSchema, "output-schema", "", "Output schema (string)")
        cmd.Flags().StringVar(&definition, "definition", "", "Step definition (string)")
        return cmd
}
