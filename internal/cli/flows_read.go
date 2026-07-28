package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/state"
	"github.com/spf13/cobra"
)

func newFlowsListCmd(app *App) *cobra.Command {
	var limit int
	var pageSize int
	var includeArchived bool
	var cursor string
	var outFormat string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateJSONOnlyFormat(outFormat, "flows list"); err != nil {
				return writeErr(cmd, err)
			}
			if isAPIMode(app) {
				if limit < 0 {
					return writeErr(cmd, fmt.Errorf("invalid --limit: must be >= 0"))
				}
				if pageSize <= 0 {
					pageSize = 100
				}
				if pageSize > 100 {
					pageSize = 100
				}

				client := apiClient(app)
				cur := strings.TrimSpace(cursor)
				wantAll := limit == 0
				remaining := limit

				allItems := make([]any, 0, 128)
				var nextCursor string
				hasMore := false
				seenCursors := map[string]bool{}
				var localWorkspaceBootstrap any

				for {
					reqLimit := pageSize
					if !wantAll && remaining > 0 && remaining < reqLimit {
						reqLimit = remaining
					}
					payload := map[string]any{
						"limit": reqLimit,
					}
					if includeArchived {
						payload["includeArchived"] = true
					}
					if cur != "" {
						payload["cursor"] = cur
					}

					out, status, err := client.DoCommand(context.Background(), "flows.list", payload)
					if err != nil {
						return writeErr(cmd, err)
					}
					if status >= 400 {
						return writeAPIResult(cmd, app, out, status)
					}
					if okAny, ok := out["ok"]; ok {
						if okb, ok := okAny.(bool); ok && !okb {
							return writeAPIResult(cmd, app, out, status)
						}
					}

					data, _ := out["data"].(map[string]any)
					pageItems, _ := data["items"].([]any)
					allItems = append(allItems, pageItems...)

					meta, _ := out["meta"].(map[string]any)
					if localWorkspaceBootstrap == nil && meta != nil {
						localWorkspaceBootstrap = meta["localWorkspaceBootstrap"]
					}
					if hm, ok := meta["hasMore"].(bool); ok {
						hasMore = hm
					} else {
						hasMore = false
					}
					if nc, ok := meta["nextCursor"].(string); ok {
						nextCursor = strings.TrimSpace(nc)
					} else {
						nextCursor = ""
					}

					if !wantAll {
						remaining -= len(pageItems)
						if remaining <= 0 {
							break
						}
					}

					if !hasMore || nextCursor == "" {
						break
					}
					if seenCursors[nextCursor] {
						return writeErr(cmd, fmt.Errorf("pagination cursor did not advance (nextCursor=%q)", nextCursor))
					}
					seenCursors[nextCursor] = true
					cur = nextCursor
				}

				metaOut := map[string]any{
					"shown":      len(allItems),
					"hasMore":    hasMore,
					"nextCursor": nextCursor,
				}
				if hasMore && nextCursor != "" {
					metaOut["hint"] = "More available. Continue with: breyta flows list --cursor " + nextCursor + " --limit " + fmt.Sprintf("%d", limit)
					if wantAll {
						metaOut["hint"] = "More available. Continue with: breyta flows list --cursor " + nextCursor + " --limit 0"
					}
				}
				if localWorkspaceBootstrap != nil {
					metaOut["localWorkspaceBootstrap"] = localWorkspaceBootstrap
				}

				out := map[string]any{
					"ok":          true,
					"workspaceId": app.WorkspaceID,
					"meta":        metaOut,
					"data": map[string]any{
						"items": allItems,
					},
				}
				return writeAPIResult(cmd, app, out, 200)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			flows, err := store.ListFlows(st)
			if err != nil {
				return writeErr(cmd, err)
			}
			total := len(flows)
			truncated := false
			if limit > 0 && limit < len(flows) {
				flows = flows[:limit]
				truncated = true
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
				item := map[string]any{
					"flowSlug":       f.Slug,
					"name":           f.Name,
					"description":    f.Description,
					"tags":           f.Tags,
					"activeVersion":  f.ActiveVersion,
					"updatedAt":      f.UpdatedAt,
					"activeCount":    activeCount[f.Slug],
					"lastStatus":     lastStatus[f.Slug],
					"lastWorkflowId": lastWorkflow[f.Slug],
				}
				appendFlowMutableMetadata(item, f)
				items = append(items, item)
			}

			meta := map[string]any{"total": total, "shown": len(items), "truncated": truncated}
			if truncated {
				meta["hint"] = "Use --limit 0 to show all flows"
			}

			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Limit results (0 = all)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size for API pagination (1-100)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived flows")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (start after this flow slug)")
	cmd.Flags().StringVar(&outFormat, "format", "json", "Output format (json)")
	return cmd
}

func newFlowsShowCmd(app *App) *cobra.Command {
	var include string
	var target string
	var version int
	var full bool
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Show a flow",
		Long: strings.TrimSpace(`
Show a flow definition for a specific source target.

- Default (no --target): workspace current (draft) source
- --target live: resolves the live installation profile and fetches its active version
- --version N: fetches an immutable historical version

Use --target live when verifying what production/live runs are executing.
`),
		Example: strings.TrimSpace(`
breyta flows show order-ingest
breyta flows show order-ingest --target live
breyta flows show order-ingest --version 6
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetChanged := cmd.Flags().Changed("target")
			payload := map[string]any{
				"flowSlug": args[0],
				"source":   "draft",
			}
			if targetChanged {
				if !isAPIMode(app) {
					return writeErr(cmd, errors.New("--target requires API mode"))
				}
				s, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				if s == "live" && version > 0 {
					return writeErr(cmd, errors.New("--target cannot be combined with --version"))
				}
				if s == "live" {
					target, err := resolveLiveProfileTarget(cmd.Context(), app, args[0], true)
					if err != nil {
						return writeErr(cmd, err)
					}
					payload["source"] = "active"
					if target.Version > 0 {
						payload["version"] = target.Version
					}
				}
			}

			if isAPIMode(app) {
				if version > 0 {
					payload["source"] = "version"
					payload["version"] = version
				}
				applyFlowsGetVerbosityPayload(payload, full, include)
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.get", payload)
				}
				out, status, err := runAPICommand(app, "flows.get", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				if status < 400 && isOK(out) {
					_ = recordConsultedFlow(provenanceSourceRef{
						WorkspaceID: workspaceIDFromEnvelope(out, app.WorkspaceID),
						FlowSlug:    args[0],
					})
				}
				if err := writeAPIResult(cmd, app, out, status); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}
			st, _, err := appStore(app)
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
			if full {
				include = "schemas,definition,spine,versions"
			}
			inc := parseCSV(include)

			out := map[string]any{
				"slug":          f.Slug,
				"name":          f.Name,
				"description":   f.Description,
				"tags":          f.Tags,
				"activeVersion": f.ActiveVersion,
				"updatedAt":     f.UpdatedAt,
			}
			appendFlowMutableMetadata(out, f)

			// Default: lightweight step list.
			steps := make([]map[string]any, 0, len(f.Steps))
			for _, s := range f.Steps {
				steps = append(steps, map[string]any{"id": s.ID, "type": s.Type, "title": s.Title})
			}
			out["steps"] = steps
			if groupKey := normalizeOptionalText(f.GroupKey); groupKey != "" {
				out["groupFlows"] = localGroupFlows(ws, f.Slug, groupKey)
			}

			if inc["spine"] {
				out["spine"] = f.Spine
			}
			if inc["schemas"] || inc["definition"] {
				detailed := make([]state.FlowStep, 0, len(f.Steps))
				for _, s := range f.Steps {
					ss := s
					if !inc["schemas"] {
						ss.InputSchema = ""
						ss.OutputSchema = ""
					}
					if !inc["definition"] {
						ss.Definition = ""
					}
					detailed = append(detailed, ss)
				}
				out["steps"] = detailed
			}

			meta := map[string]any{"hint": "Use --include schemas,definition,spine to fetch heavier fields"}
			if include != "" {
				delete(meta, "hint")
			}
			return writeData(cmd, app, meta, map[string]any{"flow": out})
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Comma-separated include list (templates,functions,flow-literal,definition; local also supports schemas,spine,versions)")
	cmd.Flags().BoolVar(&full, "full", false, "Include full flow definition, templates, functions, and source literal")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Specific version for API mode (0 = default)")
	return cmd
}

func newFlowsCreateCmd(app *App) *cobra.Command {
	var slug, name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			if slug == "" {
				return writeErr(cmd, errors.New("missing --slug"))
			}
			if isAPIMode(app) && !isAPIValidFlowSlug(slug) {
				return writeErr(cmd, fmt.Errorf("invalid --slug %q (must start with a letter; allowed: letters, digits, hyphen (-), underscore (_); max 128 chars)", slug))
			}
			if name == "" {
				name = slug
			}
			if isAPIMode(app) {
				// Create a minimal draft (version) on the server.
				// Users/agents can then pull/edit/push and deploy explicitly.
				flowLiteral := fmt.Sprintf("{:slug :%s\n :name %q\n :description %q\n :tags [\"draft\"]\n :concurrency {:type :singleton :on-new-version :supersede}\n :requires nil\n :templates nil\n :functions nil\n :invocations {:default {:label \"Run\" :inputs []}}\n :interfaces {:manual [{:id :run :label \"Run\" :invocation :default}]}\n :schedules nil\n :flow '(let [input (flow/input)]\n          input)}\n", slug, name, description)
				payload := map[string]any{"flowLiteral": flowLiteral}
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.put_draft", payload)
				}
				out, status, err := runAPICommand(app, "flows.put_draft", payload)
				if err != nil {
					return writeErr(cmd, err)
				}
				if status < 400 && isOK(out) {
					_ = appendProvenanceHints(out, workspaceIDFromEnvelope(out, app.WorkspaceID), slug)
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
			f := &state.Flow{Slug: slug, Name: name, Description: description, Tags: []string{"draft"}, ActiveVersion: 1, UpdatedAt: now, Steps: []state.FlowStep{}}
			ws.Flows[slug] = f
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "Flow slug")
	cmd.Flags().StringVar(&name, "name", "", "Flow display name")
	cmd.Flags().StringVar(&description, "description", "", "Flow description")
	must(cmd.MarkFlagRequired("slug"))
	return cmd
}
