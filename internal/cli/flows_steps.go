package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

func newFlowsStepsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				client := apiClient(app)
				resp, status, err := client.DoCommand(context.Background(), "flows.compile", map[string]any{"flowSlug": args[0]})
				if err != nil {
					return writeErr(cmd, err)
				}
				if status >= 400 {
					return writeAPIResult(cmd, app, resp, status)
				}
				data, _ := resp["data"].(map[string]any)
				analysis, _ := data["analysis"].(map[string]any)
				rawSteps, _ := analysis["steps"].([]any)
				items := make([]map[string]any, 0, len(rawSteps))
				for _, raw := range rawSteps {
					step, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					id, _ := step["id"].(string)
					typ, _ := step["type"].(string)
					if id == "" && typ == "" {
						continue
					}
					items = append(items, map[string]any{"id": id, "type": typ})
				}
				return writeData(cmd, app, nil, map[string]any{"flowSlug": args[0], "items": items})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]map[string]any, 0, len(f.Steps))
			for i, s := range f.Steps {
				items = append(items, map[string]any{"index": i, "id": s.ID, "type": s.Type, "title": s.Title})
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "items": items})
		},
	}
	return cmd
}

func newFlowsStepsShowCmd(app *App) *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:   "show <flow-slug> <step-id>",
		Short: "Show step",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				client := apiClient(app)
				resp, status, err := client.DoCommand(context.Background(), "flows.compile", map[string]any{"flowSlug": args[0]})
				if err != nil {
					return writeErr(cmd, err)
				}
				if status >= 400 {
					return writeAPIResult(cmd, app, resp, status)
				}
				data, _ := resp["data"].(map[string]any)
				analysis, _ := data["analysis"].(map[string]any)
				rawSteps, _ := analysis["steps"].([]any)
				var matched map[string]any
				for _, raw := range rawSteps {
					step, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					id, _ := step["id"].(string)
					if id == args[1] {
						matched = step
						break
					}
				}
				if matched == nil {
					return writeErr(cmd, errors.New("step not found"))
				}
				out := map[string]any{
					"id":   matched["id"],
					"type": matched["type"],
				}
				inc := parseCSV(include)
				if include != "" || inc["definition"] || inc["schemas"] {
					out["config"] = matched["config"]
					out["hasRetry"] = matched["hasRetry"]
					out["hasErrorHandling"] = matched["hasErrorHandling"]
					out["hasPersist"] = matched["hasPersist"]
				}
				meta := map[string]any{"hint": "Use --include definition to show config"}
				if include != "" {
					delete(meta, "hint")
				}
				return writeData(cmd, app, meta, map[string]any{"flowSlug": args[0], "step": out})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			step, ok := findStep(f, args[1])
			if !ok {
				return writeErr(cmd, errors.New("step not found"))
			}
			inc := parseCSV(include)
			out := map[string]any{"id": step.ID, "type": step.Type, "title": step.Title}
			if inc["schema"] || inc["schemas"] {
				out["inputSchema"] = step.InputSchema
				out["outputSchema"] = step.OutputSchema
			}
			if inc["definition"] {
				out["definition"] = step.Definition
			}
			meta := map[string]any{"hint": "Use --include schemas,definition"}
			if include != "" {
				delete(meta, "hint")
			}
			return writeData(cmd, app, meta, map[string]any{"flowSlug": f.Slug, "step": out})
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Comma-separated include list (schemas,definition)")
	return cmd
}

// --- Versions ----------------------------------------------------------------
