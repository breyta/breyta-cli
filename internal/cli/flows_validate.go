package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func newFlowsValidateCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "validate <flow-slug>",
		Short: "Run read-only flow validation (no mutation)",
		Long: strings.TrimSpace(`
Validate is a read-only check you can run on demand.

Why use it if push/release already validate?
- push validates registration constraints while writing draft state
- release validates deploy-time constraints for released/lintable code
- validate gives an explicit check point for CI, troubleshooting, and target-specific verification without mutating flow state

Recommended release safety sequence:
- breyta flows configure check <flow-slug>
- breyta flows validate <flow-slug>
- breyta flows release <flow-slug>
- breyta flows show <flow-slug> --target live
- breyta flows run <flow-slug> --target live --wait
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetChanged := cmd.Flags().Changed("target")
			resolvedTarget := "draft"
			if targetChanged {
				if !isAPIMode(app) {
					return writeErr(cmd, errors.New("--target requires API mode"))
				}
				s, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				resolvedTarget = s
			}
			source := "current"
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0], "source": "draft"}
				if resolvedTarget == "live" {
					target, err := resolveLiveProfileTarget(cmd.Context(), app, args[0], true)
					if err != nil {
						return writeErr(cmd, err)
					}
					payload["source"] = "active"
					if target.Version > 0 {
						payload["version"] = target.Version
					}
				}
				return doAPICommand(cmd, app, "flows.validate", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			record, resolvedSource, version, err := flowRecordForSource(f, source)
			if err != nil {
				return writeErr(cmd, err)
			}
			warnings := []map[string]any{}
			seen := map[string]bool{}
			for _, s := range record.Steps {
				if s.ID == "" {
					warnings = append(warnings, map[string]any{"code": "missing_step_id", "message": "step has empty id"})
					continue
				}
				if seen[s.ID] {
					warnings = append(warnings, map[string]any{"code": "duplicate_step_id", "message": "duplicate step id", "stepId": s.ID})
				}
				seen[s.ID] = true
				if s.Type == "" {
					warnings = append(warnings, map[string]any{"code": "missing_step_type", "message": "step has empty type", "stepId": s.ID})
				}
			}
			out := map[string]any{"flowSlug": f.Slug, "valid": len(warnings) == 0, "warnings": warnings}
			if resolvedSource != "" {
				out["source"] = resolvedSource
			}
			if version > 0 {
				out["version"] = version
			}
			return writeData(cmd, app, nil, out)
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	return cmd
}

func newFlowsCompileCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile <flow-slug>",
		Short: "Compile a flow (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := "current"
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0], "source": "active"}
				return doAPICommand(cmd, app, "flows.compile", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			record, resolvedSource, version, err := flowRecordForSource(f, source)
			if err != nil {
				return writeErr(cmd, err)
			}
			plan := make([]map[string]any, 0, len(record.Steps))
			for idx, s := range record.Steps {
				plan = append(plan, map[string]any{"index": idx, "id": s.ID, "type": s.Type, "title": s.Title, "definition": s.Definition})
			}
			out := map[string]any{"flowSlug": f.Slug, "plan": plan}
			if resolvedSource != "" {
				out["source"] = resolvedSource
			}
			if version > 0 {
				out["version"] = version
			}
			return writeData(cmd, app, nil, out)
		},
	}
	return cmd
}

// --- helpers ----------------------------------------------------------------
