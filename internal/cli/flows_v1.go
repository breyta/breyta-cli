package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/breyta/breyta-cli/internal/clojure/parinfer"
	"github.com/breyta/breyta-cli/internal/state"
	"github.com/breyta/breyta-cli/internal/tools"
	"github.com/spf13/cobra"
)

var apiValidFlowSlugRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)

func isAPISafeIdentifier(s string) bool {
	return apiValidFlowSlugRe.MatchString(strings.TrimSpace(s))
}

func isAPIValidFlowSlug(s string) bool {
	return isAPISafeIdentifier(s)
}

func normalizeOptionalText(s string) string {
	return strings.TrimSpace(s)
}

func appendFlowMutableMetadata(out map[string]any, flow *state.Flow) {
	if flow == nil {
		return
	}
	appendGroupMetadata(out, flow.GroupKey, flow.GroupName, flow.GroupDescription, flow.GroupOrder)
	if selector := normalizeOptionalText(flow.PrimaryDisplayConnectionSlot); selector != "" {
		out["primaryDisplayConnectionSlot"] = selector
	}
}

func appendGroupMetadata(out map[string]any, groupKey, groupName, groupDescription string, groupOrder *int) {
	if out == nil {
		return
	}
	if groupKey = normalizeOptionalText(groupKey); groupKey != "" {
		out["groupKey"] = groupKey
	}
	if groupName = normalizeOptionalText(groupName); groupName != "" {
		out["groupName"] = groupName
	}
	if groupDescription = normalizeOptionalText(groupDescription); groupDescription != "" {
		out["groupDescription"] = groupDescription
	}
	if groupOrder != nil {
		out["groupOrder"] = *groupOrder
	}
}

func parseOptionalGroupOrder(raw string) (*int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid --group-order %q (must be a non-negative integer or empty string to clear it)", raw)
	}
	return &n, nil
}

func parseOptionalDisplayConnectionSlot(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if !isAPISafeIdentifier(value) {
		return "", fmt.Errorf("invalid --primary-display-connection-slot %q (must start with a letter; allowed: letters, digits, hyphen (-), underscore (_); max 128 chars)", raw)
	}
	return value, nil
}

func localGroupFlows(ws *state.Workspace, currentSlug, groupKey string) []map[string]any {
	groupKey = normalizeOptionalText(groupKey)
	if ws == nil || groupKey == "" {
		return nil
	}

	members := make([]*state.Flow, 0, len(ws.Flows))
	for _, candidate := range ws.Flows {
		if candidate == nil || candidate.Slug == currentSlug {
			continue
		}
		if normalizeOptionalText(candidate.GroupKey) == groupKey {
			members = append(members, candidate)
		}
	}

	ordered := false
	for _, member := range members {
		if member != nil && member.GroupOrder != nil {
			ordered = true
			break
		}
	}

	sort.Slice(members, func(i, j int) bool {
		if ordered {
			leftOrder := int(^uint(0) >> 1)
			rightOrder := int(^uint(0) >> 1)
			if members[i].GroupOrder != nil {
				leftOrder = *members[i].GroupOrder
			}
			if members[j].GroupOrder != nil {
				rightOrder = *members[j].GroupOrder
			}
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
		}
		leftName := strings.ToLower(normalizeOptionalText(members[i].Name))
		rightName := strings.ToLower(normalizeOptionalText(members[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return members[i].Slug < members[j].Slug
	})

	items := make([]map[string]any, 0, len(members))
	for _, member := range members {
		item := map[string]any{
			"flowSlug":    member.Slug,
			"name":        member.Name,
			"description": member.Description,
		}
		appendFlowMutableMetadata(item, member)
		items = append(items, item)
	}
	return items
}

func resolveLocalFlowGroupUpdate(cmd *cobra.Command, flow *state.Flow, groupKey, groupName, groupDescription, groupOrder string) (string, string, string, *int, bool, error) {
	groupKeyProvided := cmd.Flags().Changed("group-key")
	groupNameProvided := cmd.Flags().Changed("group-name")
	groupDescriptionProvided := cmd.Flags().Changed("group-description")
	groupOrderProvided := cmd.Flags().Changed("group-order")
	if !groupKeyProvided && !groupNameProvided && !groupDescriptionProvided && !groupOrderProvided {
		return "", "", "", nil, false, nil
	}

	currentGroupKey := normalizeOptionalText(flow.GroupKey)
	currentGroupName := normalizeOptionalText(flow.GroupName)
	currentGroupDescription := normalizeOptionalText(flow.GroupDescription)
	currentGroupOrder := flow.GroupOrder
	requestedGroupKey := normalizeOptionalText(groupKey)
	requestedGroupName := normalizeOptionalText(groupName)
	requestedGroupDescription := normalizeOptionalText(groupDescription)
	requestedGroupOrder, err := parseOptionalGroupOrder(groupOrder)
	if err != nil {
		return "", "", "", nil, false, err
	}
	clearGroup := groupKeyProvided && requestedGroupKey == ""

	finalGroupKey := currentGroupKey
	if clearGroup {
		finalGroupKey = ""
	} else if groupKeyProvided {
		finalGroupKey = requestedGroupKey
	}

	finalGroupName := currentGroupName
	if clearGroup {
		finalGroupName = ""
	} else if groupNameProvided {
		finalGroupName = requestedGroupName
	}

	finalGroupDescription := currentGroupDescription
	if clearGroup {
		finalGroupDescription = ""
	} else if groupDescriptionProvided {
		finalGroupDescription = requestedGroupDescription
	}

	finalGroupOrder := currentGroupOrder
	if clearGroup {
		finalGroupOrder = nil
	} else if groupOrderProvided {
		finalGroupOrder = requestedGroupOrder
	}

	if groupKeyProvided && requestedGroupKey != "" && !isAPIValidFlowSlug(requestedGroupKey) {
		return "", "", "", nil, false, fmt.Errorf("invalid --group-key %q (must start with a letter; allowed: letters, digits, hyphen (-), underscore (_); max 128 chars)", requestedGroupKey)
	}
	if (groupNameProvided || groupDescriptionProvided || groupOrderProvided) && finalGroupKey == "" {
		return "", "", "", nil, false, errors.New("groupKey is required")
	}
	if finalGroupKey != "" && finalGroupName == "" {
		return "", "", "", nil, false, errors.New("groupName is required")
	}

	return finalGroupKey, finalGroupName, finalGroupDescription, finalGroupOrder, true, nil
}

// doAPICommandFn is a test hook to stub API calls in command unit tests.
var doAPICommandFn = doAPICommand
var useDoAPICommandFn bool

func newFlowsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "flows",
		Aliases: []string{"flow"},
		Short:   "Inspect and edit flows",
		Long: strings.TrimSpace(`
Flow authoring uses a file workflow:
1) pull a flow to a local .clj file
2) edit the file (Clojure map literal + DSL)
3) push -> updates working copy (and validates by default)
4) diff -> inspect draft changes against live or a released version
5) release -> activates the latest pushed version and promotes live + installations in current workspace
6) run -> verifies behavior in your workspace

Optional explicit check:
- validate -> read-only verification for CI, troubleshooting, or explicit target checks

Advanced rollout workflow (optional):
- release -> activates the latest pushed version + live/installations promotion in current workspace
- promote -> updates live target and installations to a released version
- installations ... -> installation-id scoped management

Quick commands:
- breyta flows list
- breyta flows pull <slug> --out ./tmp/flows/<slug>.clj
- breyta flows push --file ./tmp/flows/<slug>.clj
- breyta flows update <slug> --group-order 10
- breyta flows diff <slug>
- breyta flows configure <slug> --set api.conn=conn-...
- breyta flows configure check <slug>
- breyta flows release <slug> --release-note-file ./release-note.md
- breyta flows promote <slug> --version <n>
- breyta flows show <slug> --target live
- breyta flows run <slug> --target live --wait
- breyta flows run <slug> --wait

Flow file format (minimal):
{:slug :my-flow
 :name "My Flow"
 :description "..."
 :tags ["example"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :triggers nil
 :flow '(let [input (flow/input)]
          (flow/step :function :do {:code '(fn [x] x)} :input input))}

Notes:
- The server reads the file with *read-eval* disabled.
- :flow should be a quoted form. (quote ...) is also accepted.
- Use flow/input for inputs and flow/step for steps.
- Grouping metadata is mutable workspace metadata, not part of the pulled flow source file.
  - inspect grouped flows: breyta flows list --pretty
  - verify ordered siblings: breyta flows show <slug> --pretty
  - clear only ordering: breyta flows update <slug> --group-order ""
- Release notes are markdown attached to published versions.
  - draft vs live diff: breyta flows diff <slug>
  - set on release: breyta flows release <slug> --release-note-file ./release-note.md
  - edit later: breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md
- activeVersion is the currently activated released version. Live runtime can resolve to a different installation version
  - verify live with: breyta flows show <slug> --target live
  - smoke-run live with: breyta flows run <slug> --target live --wait
- Concurrency guidance:
  - Reconciler/sweeper/scheduled cleanup flows should use :on-new-version :supersede so fixes take effect immediately
  - Use :on-new-version :drain only when in-flight runs must finish on the old version

Advanced install lifecycle:
- Release the latest pushed version with default live + installations promotion: breyta flows release <slug>
- Release the latest pushed version while skipping end-user installation promotion: breyta flows release <slug> --skip-promote-installations
- Promote released version to live explicitly (also rollback to known-good): breyta flows promote <slug> --version <n>
- Configure installation inputs: breyta flows installations configure <installation-id> --input '{...}'
- List installation triggers: breyta flows installations triggers <installation-id>
		`),
	}

	cmd.AddCommand(newFlowsListCmd(app))
	cmd.AddCommand(newFlowsSearchCmd(app))
	cmd.AddCommand(newFlowsShowCmd(app))
	cmd.AddCommand(newFlowsDiffCmd(app))
	cmd.AddCommand(newFlowsCreateCmd(app))
	cmd.AddCommand(newFlowsConfigureCmd(app))
	cmd.AddCommand(newFlowsBindingsCmd(app))
	cmd.AddCommand(newFlowsReleaseCmd(app))
	cmd.AddCommand(newFlowsPromoteCmd(app))
	cmd.AddCommand(newFlowsRunCmd(app))
	cmd.AddCommand(newFlowsActivateCmd(app))
	cmd.AddCommand(newFlowsInstallationsCmd(app))
	cmd.AddCommand(newFlowsMarketplaceCmd(app))
	cmd.AddCommand(newFlowsDraftCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsURLCmd(app))
	cmd.AddCommand(newFlowsPullCmd(app))
	cmd.AddCommand(newFlowsPushCmd(app))
	cmd.AddCommand(newFlowsParenRepairCmd(app))
	cmd.AddCommand(newFlowsParenCheckCmd(app))
	cmd.AddCommand(newFlowsDeployCmd(app))
	cmd.AddCommand(newFlowsUpdateCmd(app))
	cmd.AddCommand(newFlowsProvenanceCmd(app))
	cmd.AddCommand(newFlowsArchiveCmd(app))
	cmd.AddCommand(newFlowsDeleteCmd(app))
	cmd.AddCommand(newFlowsSpineCmd(app))
	cmd.AddCommand(newFlowsCompileCmd(app))

	steps := &cobra.Command{Use: "steps", Short: "Manage flow steps"}
	steps.AddCommand(newFlowsStepsListCmd(app))
	steps.AddCommand(newFlowsStepsShowCmd(app))
	cmd.AddCommand(steps)

	versions := &cobra.Command{Use: "versions", Short: "Manage flow versions"}
	versions.AddCommand(newFlowsVersionsListCmd(app))
	versions.AddCommand(newFlowsVersionsPublishCmd(app))
	versions.AddCommand(newFlowsVersionsUpdateCmd(app))
	versions.AddCommand(newFlowsVersionsActivateCmd(app))
	versions.AddCommand(newFlowsVersionsDiffCmd(app))
	cmd.AddCommand(versions)

	cmd.AddCommand(newFlowsValidateCmd(app))

	return cmd
}

func newFlowsParenRepairCmd(app *App) *cobra.Command {
	var write bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "paren-repair <files...>",
		Short: "Repair unbalanced Clojure delimiters in flow files (local)",
		Long: strings.TrimSpace(`
Repair unbalanced delimiters in one or more local .clj flow files.

This is intended as an escape hatch when LLM edits introduce delimiter errors.
`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results := make([]map[string]any, 0, len(args))
			changedAny := false

			parinferPath := tools.FindParinferRust()
			parinferRunner := parinfer.Runner{BinaryPath: parinferPath}

			for _, path := range args {
				b, err := os.ReadFile(path)
				if err != nil {
					return writeFailure(cmd, app, "read_failed", err, "Check the path and permissions.", map[string]any{"path": path})
				}

				orig := string(b)
				engine := "fallback"
				repaired := orig
				var report any

				if parinferPath != "" {
					engine = "parinfer-rust"
					if out, ans, err := parinferRunner.RepairIndent(orig); err == nil {
						repaired = out
						report = ans
					} else {
						engine = "fallback"
					}
				}

				if engine == "fallback" {
					out, rep, err := parenrepair.Repair(orig, verbose)
					if err != nil {
						return writeFailure(cmd, app, "clojure_paren_repair_failed", err, "Fix the underlying syntax issue (e.g. unterminated string), then retry.", map[string]any{"path": path, "report": rep})
					}
					repaired = out
					report = rep
				}

				changed := repaired != orig
				if changed {
					changedAny = true
				}

				if write && changed {
					if err := atomicWriteFile(path, []byte(repaired), 0o644); err != nil {
						return writeFailure(cmd, app, "write_failed", err, "Check the path and permissions.", map[string]any{"path": path})
					}
				}

				r := map[string]any{
					"path":    path,
					"changed": changed,
					"written": write && changed,
					"engine":  engine,
					"report":  report,
				}
				results = append(results, r)
			}

			return writeData(cmd, app, nil, map[string]any{
				"changed": changedAny,
				"results": results,
			})
		},
	}

	cmd.Flags().BoolVar(&write, "write", true, "Write changes to files (in-place)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Include per-fix details in output (fallback engine only)")
	return cmd
}

func newFlowsParenCheckCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paren-check <file>",
		Short: "Check that a flow file has balanced delimiters (local)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			b, err := os.ReadFile(path)
			if err != nil {
				return writeFailure(cmd, app, "read_failed", err, "Check the path and permissions.", map[string]any{"path": path})
			}
			if err := parenrepair.Check(string(b)); err != nil {
				return writeFailure(cmd, app, "clojure_delimiters_invalid", err, "Run: breyta flows paren-repair --write <file>", map[string]any{"path": path})
			}
			return writeData(cmd, app, nil, map[string]any{"path": path, "ok": true})
		},
	}
	return cmd
}

func atomicWriteFile(path string, data []byte, defaultPerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	perm := defaultPerm
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".breyta.tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

func newFlowsActivateCmd(app *App) *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:    "activate <flow-slug>",
		Short:  "Enable the prod profile for a flow",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows activate requires API mode"))
			}
			body := map[string]any{
				"flowSlug": args[0],
				"version":  strings.TrimSpace(version),
			}
			return doAPICommand(cmd, app, "profiles.activate", body)
		},
	}
	cmd.Flags().StringVar(&version, "version", "latest", "Flow version to activate (number or latest)")
	return cmd
}

func newFlowsDraftBindingsURLCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft-bindings-url <flow-slug>",
		Short: "Print the draft bindings URL for working-copy runs",
		Long: strings.TrimSpace(`
Working-copy runs use a user-scoped draft profile. Bind credentials here:
- Draft bindings: http://localhost:8090/<workspace>/flows/<slug>/draft-bindings
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := strings.TrimRight(app.APIURL, "/")
			if strings.TrimSpace(base) == "" {
				base = "http://localhost:8090"
			}
			url := fmt.Sprintf("%s/%s/flows/%s/draft-bindings", base, app.WorkspaceID, args[0])
			return writeData(cmd, app, nil, map[string]any{
				"workspaceId":      app.WorkspaceID,
				"flowSlug":         args[0],
				"draftBindingsUrl": url,
			})
		},
	}
	return cmd
}

func newFlowsSpineCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spine <flow-slug>",
		Short: "Show a flow spine (textual structure)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Mock-only command (use `breyta flows show` in API mode).")
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "spine": f.Spine})
		},
	}
	return cmd
}

func newFlowsListCmd(app *App) *cobra.Command {
	var limit int
	var pageSize int
	var includeArchived bool
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List flows",
		RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.Flags().IntVar(&limit, "limit", 25, "Limit results (0 = all)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "Page size for API pagination (1-100)")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived flows")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (start after this flow slug)")
	return cmd
}

func newFlowsShowCmd(app *App) *cobra.Command {
	var include string
	var target string
	var version int
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Show a flow",
		Long: strings.TrimSpace(`
Show a flow definition for a specific source target.

- Default (no --target): workspace current (draft) source
- --target live: resolves the live installation profile and fetches its active version

Use --target live when verifying what production/live runs are executing.
`),
		Example: strings.TrimSpace(`
breyta flows show order-ingest
breyta flows show order-ingest --target live
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
					payload["version"] = version
				}
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
	cmd.Flags().StringVar(&include, "include", "", "Comma-separated include list (schemas,definition,spine,versions)")
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
				flowLiteral := fmt.Sprintf("{:slug :%s\n :name %q\n :description %q\n :tags [\"draft\"]\n :concurrency {:type :singleton :on-new-version :supersede}\n :requires nil\n :templates nil\n :functions nil\n :triggers [{:type :manual :label \"Run\" :enabled true :config {}}]\n :flow '(let [input (flow/input)]\n          input)}\n", slug, name, description)
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

func newFlowsPullCmd(app *App) *cobra.Command {
	var out string
	var target string
	var version int
	cmd := &cobra.Command{
		Use:   "pull <flow-slug>",
		Short: "Pull a flow to a local .clj file for editing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Pull requires --api/BREYTA_API_URL")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			slug := args[0]
			path := out
			if strings.TrimSpace(path) == "" {
				path = filepath.Join("tmp", "flows", slug+".clj")
			}
			targetChanged := cmd.Flags().Changed("target")
			resolvedTarget := "draft"
			if targetChanged {
				s, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				resolvedTarget = s
				if resolvedTarget == "live" && version > 0 {
					return writeErr(cmd, errors.New("--target cannot be combined with --version"))
				}
			}

			payload := map[string]any{"flowSlug": slug}
			if resolvedTarget == "live" {
				target, err := resolveLiveProfileTarget(cmd.Context(), app, slug, true)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["source"] = "active"
				if target.Version > 0 {
					payload["version"] = target.Version
				}
			} else {
				payload["source"] = "draft"
				if version > 0 {
					payload["version"] = version
				}
			}

			resp, status, err := runAPICommandWithContext(cmd.Context(), app, "flows.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(resp) {
				return writeAPIResult(cmd, app, resp, status)
			}

			dataAny, ok := resp["data"]
			if !ok {
				return writeErr(cmd, errors.New("missing data in response"))
			}
			data, ok := dataAny.(map[string]any)
			if !ok {
				return writeErr(cmd, errors.New("invalid data in response"))
			}
			flowLiteral, ok := data["flowLiteral"].(string)
			if !ok || strings.TrimSpace(flowLiteral) == "" {
				return writeErr(cmd, errors.New("missing data.flowLiteral in response"))
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return writeErr(cmd, err)
			}
			if err := os.WriteFile(path, []byte(flowLiteral+"\n"), 0o644); err != nil {
				return writeErr(cmd, err)
			}
			_ = recordConsultedFlow(provenanceSourceRef{
				WorkspaceID: workspaceIDFromEnvelope(resp, app.WorkspaceID),
				FlowSlug:    slug,
			})
			result := map[string]any{"saved": true, "path": path, "flowSlug": slug}
			if targetChanged {
				result["target"] = resolvedTarget
			}
			trackCLIEvent(app, "cli_flow_pulled", nil, app.Token, map[string]any{
				"product":   "flows",
				"channel":   "cli",
				"api_host":  apiHostname(app.APIURL),
				"flow_slug": slug,
				"target":    resolvedTarget,
			})
			return writeData(cmd, app, nil, result)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default: tmp/flows/<slug>.clj)")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = default)")
	return cmd
}

func newFlowsPushCmd(app *App) *cobra.Command {
	var file string
	var target string
	var repairDelimiters bool
	var noRepairWriteback bool
	var validate bool
	var deployKey string
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push a local .clj flow file as an updated working copy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Push requires --api/BREYTA_API_URL")
			}
			if cmd.Flags().Changed("target") {
				resolvedTarget, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				if resolvedTarget == "live" {
					return writeErr(cmd, errors.New("--target live is not supported for flows push; push always updates workspace current. Use `breyta flows release <slug>` to publish/install live, or `breyta flows promote <slug>` to retarget live"))
				}
			}
			if strings.TrimSpace(file) == "" {
				return writeErr(cmd, errors.New("missing --file"))
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return writeErr(cmd, err)
			}

			orig := string(b)
			flowLiteral := orig
			if repairDelimiters {
				parinferPath := tools.FindParinferRust()
				if parinferPath != "" {
					if repaired, _, err := (parinfer.Runner{BinaryPath: parinferPath}).RepairIndent(flowLiteral); err == nil {
						flowLiteral = repaired
					}
				}
				// Fallback best-effort repair (always runs if parinfer isn't available or fails).
				if repaired, _, err := parenrepair.Repair(flowLiteral, false); err == nil {
					flowLiteral = repaired
				}
			}

			repairWriteback := !noRepairWriteback
			if repairWriteback && flowLiteral != orig {
				if err := atomicWriteFile(file, []byte(flowLiteral), 0o644); err != nil {
					return writeErr(cmd, err)
				}
			}

			if useDoAPICommandFn {
				payload := map[string]any{"flowLiteral": flowLiteral}
				resolvedDeployKey := strings.TrimSpace(deployKey)
				if resolvedDeployKey == "" {
					resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
				}
				if resolvedDeployKey != "" {
					payload["deploy-key"] = resolvedDeployKey
				}
				return doAPICommandFn(cmd, app, "flows.put_draft", payload)
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{"flowLiteral": flowLiteral}
			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deploy-key"] = resolvedDeployKey
			}
			out, status, err := runAPICommand(app, "flows.put_draft", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 || !isOK(out) {
				return writeAPIResult(cmd, app, out, status)
			}
			flowSlug := ""
			if dataAny, ok := out["data"]; ok {
				if data, ok := dataAny.(map[string]any); ok {
					if slug, _ := data["flowSlug"].(string); strings.TrimSpace(slug) != "" {
						flowSlug = strings.TrimSpace(slug)
					}
				}
			}
			if !validate {
				if flowSlug != "" {
					_ = appendProvenanceHints(out, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug)
				}
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":   "flows",
					"channel":   "cli",
					"api_host":  apiHostname(app.APIURL),
					"flow_slug": flowSlug,
					"validated": false,
				})
				return writeAPIResult(cmd, app, out, status)
			}
			if flowSlug == "" {
				meta := ensureMeta(out)
				if meta != nil {
					meta["hint"] = "Draft pushed, but flowSlug missing for validation. Run: breyta flows validate <slug>"
				}
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":   "flows",
					"channel":   "cli",
					"api_host":  apiHostname(app.APIURL),
					"validated": false,
				})
				return writeAPIResult(cmd, app, out, status)
			}

			client := apiClient(app)
			validateOut, validateStatus, err := client.DoCommand(context.Background(), "flows.validate", map[string]any{
				"flowSlug": flowSlug,
				"source":   "draft",
			})
			if err != nil {
				return writeErr(cmd, err)
			}
			if validateStatus >= 400 || !isOK(validateOut) {
				_ = appendProvenanceHints(validateOut, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug)
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":         "flows",
					"channel":         "cli",
					"api_host":        apiHostname(app.APIURL),
					"flow_slug":       flowSlug,
					"validated":       false,
					"validate_source": "draft",
				})
				return writeAPIResult(cmd, app, validateOut, validateStatus)
			}
			meta := ensureMeta(out)
			if meta != nil {
				meta["validated"] = true
				meta["validateSource"] = "draft"
			}
			_ = appendProvenanceHints(out, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug)
			trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
				"product":         "flows",
				"channel":         "cli",
				"api_host":        apiHostname(app.APIURL),
				"flow_slug":       flowSlug,
				"validated":       true,
				"validate_source": "draft",
			})
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Path to a flow .clj file")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live). live is not valid for push")
	cmd.Flags().BoolVar(&repairDelimiters, "repair-delimiters", true, "Attempt best-effort delimiter repair before uploading")
	cmd.Flags().BoolVar(&noRepairWriteback, "no-repair-writeback", false, "Do not write repaired content back to --file (default: write back when changed)")
	cmd.Flags().BoolVar(&validate, "validate", true, "Validate the working copy after pushing")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key for guarded flows (default: BREYTA_FLOW_DEPLOY_KEY)")
	must(cmd.MarkFlagRequired("file"))
	return cmd
}

func newFlowsDeployCmd(app *App) *cobra.Command {
	var version int
	var deployKey string
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	cmd := &cobra.Command{
		Use:   "deploy <flow-slug>",
		Short: "Deploy a flow version (make it active)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Deploy requires --api/BREYTA_API_URL")
			}
			payload := map[string]any{"flowSlug": args[0]}
			if version > 0 {
				payload["version"] = version
			}
			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deployKey"] = resolvedDeployKey
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(resolvedReleaseNote) != "" {
				payload["releaseNote"] = resolvedReleaseNote
			}
			return doAPICommand(cmd, app, "flows.deploy", payload)
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = latest)")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key (default: BREYTA_FLOW_DEPLOY_KEY)")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note to attach to the deployed version")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsUpdateCmd(app *App) *cobra.Command {
	var name, description, tags, primaryDisplayConnectionSlot string
	var groupKey, groupName, groupDescription, groupOrder string
	cmd := &cobra.Command{
		Use:   "update <flow-slug>",
		Short: "Update flow metadata",
		Long: strings.TrimSpace(`
Update mutable flow metadata such as name, description, tags, grouping, and display icon selection.

Grouping and display icon metadata are workspace metadata. They do not round-trip through
` + "`breyta flows pull`" + ` / ` + "`breyta flows push`" + ` source files.

Common grouped-flow loop:
- inspect current grouping with ` + "`breyta flows list --pretty`" + ` or ` + "`breyta flows show <slug> --pretty`" + `
- set or change grouping with ` + "`breyta flows update <slug> --group-key ... --group-name ... --group-order ...`" + `
- verify sibling order again with ` + "`breyta flows show <slug> --pretty`" + `

Display icon loop:
- inspect current display icon selector with ` + "`breyta flows show <slug> --pretty`" + `
- set or clear it with ` + "`breyta flows update <slug> --primary-display-connection-slot <selector>`" + `
		`),
		Example: strings.TrimSpace(`
breyta flows update invoice-start \
  --group-key invoice-pipeline \
  --group-name "Invoice Pipeline" \
  --group-description "Flows that run in sequence for invoice processing" \
  --group-order 10

breyta flows update invoice-reconcile --group-order 20

breyta flows show invoice-start --pretty

breyta flows update invoice-reconcile --group-order ""
breyta flows update invoice-start --group-key ""

breyta flows update customer-support --primary-display-connection-slot crm
breyta flows update customer-support --primary-display-connection-slot ""
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(name) != "" {
					payload["name"] = name
				}
				if strings.TrimSpace(description) != "" {
					payload["description"] = description
				}
				if strings.TrimSpace(tags) != "" {
					payload["tags"] = tags
				}
				if cmd.Flags().Changed("group-key") {
					payload["groupKey"] = normalizeOptionalText(groupKey)
				}
				if cmd.Flags().Changed("group-name") {
					payload["groupName"] = normalizeOptionalText(groupName)
				}
				if cmd.Flags().Changed("group-description") {
					payload["groupDescription"] = normalizeOptionalText(groupDescription)
				}
				if cmd.Flags().Changed("group-order") {
					resolvedGroupOrder, err := parseOptionalGroupOrder(groupOrder)
					if err != nil {
						return writeErr(cmd, err)
					}
					if resolvedGroupOrder == nil {
						payload["groupOrder"] = ""
					} else {
						payload["groupOrder"] = *resolvedGroupOrder
					}
				}
				if cmd.Flags().Changed("primary-display-connection-slot") {
					resolvedSelector, err := parseOptionalDisplayConnectionSlot(primaryDisplayConnectionSlot)
					if err != nil {
						return writeErr(cmd, err)
					}
					payload["primaryDisplayConnectionSlot"] = resolvedSelector
				}
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.update", payload)
				}
				return doAPICommand(cmd, app, "flows.update", payload)
			}
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
			if name != "" {
				f.Name = name
			}
			if description != "" {
				f.Description = description
			}
			if tags != "" {
				f.Tags = splitNonEmpty(tags)
			}
			resolvedGroupKey, resolvedGroupName, resolvedGroupDescription, resolvedGroupOrder, groupChanged, err := resolveLocalFlowGroupUpdate(cmd, f, groupKey, groupName, groupDescription, groupOrder)
			if err != nil {
				return writeErr(cmd, err)
			}
			if groupChanged {
				f.GroupKey = resolvedGroupKey
				f.GroupName = resolvedGroupName
				f.GroupDescription = resolvedGroupDescription
				f.GroupOrder = resolvedGroupOrder
			}
			if cmd.Flags().Changed("primary-display-connection-slot") {
				resolvedSelector, err := parseOptionalDisplayConnectionSlot(primaryDisplayConnectionSlot)
				if err != nil {
					return writeErr(cmd, err)
				}
				f.PrimaryDisplayConnectionSlot = resolvedSelector
			}
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&groupKey, "group-key", "", "Group key (safe identifier; empty string clears grouping)")
	cmd.Flags().StringVar(&groupName, "group-name", "", "Group name (required whenever group key is set)")
	cmd.Flags().StringVar(&groupDescription, "group-description", "", "Group description")
	cmd.Flags().StringVar(&groupOrder, "group-order", "", "Group order (lower numbers sort first; empty string clears it)")
	cmd.Flags().StringVar(&primaryDisplayConnectionSlot, "primary-display-connection-slot", "", "Display icon selector (empty string clears it)")
	return cmd
}

func newFlowsProvenanceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provenance",
		Short: "Manage flow provenance metadata",
	}
	cmd.AddCommand(newFlowsProvenanceSetCmd(app))
	return cmd
}

func newFlowsProvenanceSetCmd(app *App) *cobra.Command {
	var sources []string
	var templates []string
	var fromConsulted bool
	var clear bool

	cmd := &cobra.Command{
		Use:   "set <flow-slug>",
		Short: "Replace flow provenance metadata",
		Long: strings.TrimSpace(`
Replace the full set of source-flow provenance refs for a flow.

Use --from-consulted to persist the flows previously opened with ` + "`breyta flows show`" + `
or ` + "`breyta flows pull`" + ` in this agent workspace. Use --source for workspace flows,
--template for public templates, and --clear to explicitly remove all provenance.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeErr(cmd, errors.New("flows provenance set requires API mode"))
			}
			if clear && (fromConsulted || len(sources) > 0 || len(templates) > 0) {
				return writeErr(cmd, errors.New("--clear cannot be combined with --source, --template, or --from-consulted"))
			}

			flowSlug := strings.TrimSpace(args[0])
			payload := map[string]any{"flowSlug": flowSlug}

			if clear {
				payload["sourceFlows"] = []map[string]any{}
				if useDoAPICommandFn {
					return doAPICommandFn(cmd, app, "flows.provenance.set", payload)
				}
				return doAPICommand(cmd, app, "flows.provenance.set", payload)
			}

			refs := make([]provenanceSourceRef, 0, len(sources)+len(templates))
			for _, raw := range sources {
				ref, err := parseProvenanceSourceRef(raw, app.WorkspaceID)
				if err != nil {
					return writeErr(cmd, err)
				}
				refs = append(refs, ref)
			}
			for _, raw := range templates {
				ref, err := parseProvenanceTemplateRef(raw)
				if err != nil {
					return writeErr(cmd, err)
				}
				refs = append(refs, ref)
			}
			if fromConsulted {
				consulted, err := currentProvenanceCandidates(app.WorkspaceID, flowSlug)
				if err != nil {
					return writeErr(cmd, err)
				}
				if len(consulted) == 0 && len(refs) == 0 {
					return writeErr(cmd, errors.New("no consulted flows found; use `breyta flows show` or `breyta flows pull` first, or pass --source"))
				}
				refs = append(refs, consulted...)
			}
			refs = dedupeProvenanceSourceRefs(refs)
			if len(refs) == 0 {
				return writeErr(cmd, errors.New("provide --source, --template, --from-consulted, or --clear"))
			}

			payload["sourceFlows"] = provenanceSourceFlowPayloadItems(refs)
			if useDoAPICommandFn {
				return doAPICommandFn(cmd, app, "flows.provenance.set", payload)
			}
			return doAPICommand(cmd, app, "flows.provenance.set", payload)
		},
	}

	cmd.Flags().StringArrayVar(&sources, "source", nil, "Source flow ref (<flow-slug> or <workspace-id>/<flow-slug>); repeatable")
	cmd.Flags().StringArrayVar(&templates, "template", nil, "Public template source slug (<template-slug>); repeatable")
	cmd.Flags().BoolVar(&fromConsulted, "from-consulted", false, "Use consulted flows tracked in this agent workspace")
	cmd.Flags().BoolVar(&clear, "clear", false, "Clear all provenance for this flow")
	return cmd
}

func newFlowsDeleteCmd(app *App) *cobra.Command {
	var yes bool
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <flow-slug>",
		Short: "Delete a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if yes {
					payload["yes"] = true
				}
				if force {
					payload["force"] = true
				}
				return doAPICommand(cmd, app, "flows.delete", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws.Flows[args[0]] == nil {
				return writeErr(cmd, errors.New("flow not found"))
			}
			delete(ws.Flows, args[0])
			ws.UpdatedAt = time.Now().UTC()
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"deleted": true, "flowSlug": args[0]})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm delete")
	cmd.Flags().BoolVar(&force, "force", false, "Force delete (cancel runs, delete installations)")
	return cmd
}

func newFlowsArchiveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "archive <flow-slug>",
		Short: "Archive a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				return doAPICommand(cmd, app, "flows.archive", payload)
			}
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
			f.Tags = append(f.Tags, "archived")
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"archived": true, "flowSlug": args[0]})
		},
	}
	return cmd
}

// --- Steps ------------------------------------------------------------------

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

func newFlowsStepsSetCmd(app *App) *cobra.Command {
	var (
		stepType     string
		title        string
		inputSchema  string
		outputSchema string
		definition   string
	)
	cmd := &cobra.Command{
		Use:   "set <flow-slug> <step-id>",
		Short: "Create or update a step",
		Args:  cobra.ExactArgs(2),
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
			if stepType == "" {
				stepType = "function"
			}
			if title == "" {
				title = args[1]
			}
			s := state.FlowStep{ID: args[1], Type: stepType, Title: title, InputSchema: inputSchema, OutputSchema: outputSchema, Definition: definition}
			upsertStep(f, s)
			f.Spine = buildSpine(f)
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().StringVar(&stepType, "type", "", "Step type (http|function|code|wait|notify|llm)")
	cmd.Flags().StringVar(&title, "title", "", "Step title")
	cmd.Flags().StringVar(&inputSchema, "input-schema", "", "Input schema")
	cmd.Flags().StringVar(&outputSchema, "output-schema", "", "Output schema")
	cmd.Flags().StringVar(&definition, "definition", "", "Definition")
	return cmd
}

func newFlowsStepsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <flow-slug> <step-id>",
		Short: "Delete a step",
		Args:  cobra.ExactArgs(2),
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
			if !deleteStep(f, args[1]) {
				return writeErr(cmd, errors.New("step not found"))
			}
			f.Spine = buildSpine(f)
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"deleted": true, "flowSlug": f.Slug, "stepId": args[1]})
		},
	}
	return cmd
}

func newFlowsStepsMoveCmd(app *App) *cobra.Command {
	var before, after string
	cmd := &cobra.Command{
		Use:   "move <flow-slug> <step-id>",
		Short: "Move a step",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if before == "" && after == "" {
				return writeErr(cmd, errors.New("missing --before or --after"))
			}
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
			if err := moveStep(f, args[1], before, after); err != nil {
				return writeErr(cmd, err)
			}
			f.Spine = buildSpine(f)
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().StringVar(&before, "before", "", "Move step before this step-id")
	cmd.Flags().StringVar(&after, "after", "", "Move step after this step-id")
	return cmd
}

// --- Versions ----------------------------------------------------------------

func newFlowsVersionsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List published versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doAPICommand(cmd, app, "flows.versions.list", map[string]any{"flowSlug": args[0]})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]map[string]any, 0, len(f.Versions))
			for _, v := range f.Versions {
				item := map[string]any{
					"version":     v.Version,
					"publishedAt": v.PublishedAt,
					"note":        v.Note,
				}
				if strings.TrimSpace(v.Note) != "" {
					item["releaseNote"] = v.Note
				}
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i]["version"].(int) > items[j]["version"].(int) })
			meta := map[string]any{"activeVersion": f.ActiveVersion}
			return writeData(cmd, app, meta, map[string]any{"flowSlug": f.Slug, "items": items})
		},
	}
	return cmd
}

func newFlowsVersionsPublishCmd(app *App) *cobra.Command {
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	cmd := &cobra.Command{
		Use:   "publish <flow-slug>",
		Short: "Publish a new immutable version",
		Long: strings.TrimSpace(`
Publish a new immutable version.

Attach a markdown release note when you know what changed:
- breyta flows versions publish my-flow --release-note 'Added retry guard'
- breyta flows versions publish my-flow --release-note-file ./release-note.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(resolvedReleaseNote) != "" {
					payload["releaseNote"] = resolvedReleaseNote
				}
				return doAPICommand(cmd, app, "flows.versions.publish", payload)
			}
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
			now := time.Now().UTC()
			next := maxVersion(f) + 1
			fv := state.FlowVersion{
				Version:     next,
				PublishedAt: now,
				Note:        resolvedReleaseNote,
				Flow: state.FlowRecord{
					Name:        f.Name,
					Description: f.Description,
					Tags:        append([]string{}, f.Tags...),
					Spine:       append([]string{}, f.Spine...),
					Steps:       append([]state.FlowStep{}, f.Steps...),
				},
			}
			f.Versions = append(f.Versions, fv)
			f.ActiveVersion = next
			f.UpdatedAt = now
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "publishedVersion": next})
		},
	}
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsVersionsUpdateCmd(app *App) *cobra.Command {
	var version int
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string
	var clearReleaseNote bool

	cmd := &cobra.Command{
		Use:   "update <flow-slug>",
		Short: "Update version metadata such as the release note",
		Long: strings.TrimSpace(`
Update version metadata without publishing a new version.

Examples:
- breyta flows versions update my-flow --version 7 --release-note-file ./release-note.md
- breyta flows versions update my-flow --version 7 --clear-release-note
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version <= 0 {
				return writeErr(cmd, errors.New("missing --version"))
			}
			hasReleaseNoteInput := strings.TrimSpace(releaseNote) != "" ||
				strings.TrimSpace(legacyNote) != "" ||
				strings.TrimSpace(releaseNoteFile) != ""
			if clearReleaseNote && hasReleaseNoteInput {
				return writeErr(cmd, errors.New("--clear-release-note cannot be combined with --release-note/--release-note-file"))
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if !clearReleaseNote && strings.TrimSpace(resolvedReleaseNote) == "" {
				return writeErr(cmd, errors.New("missing --release-note/--release-note-file or --clear-release-note"))
			}

			if isAPIMode(app) {
				payload := map[string]any{
					"flowSlug": args[0],
					"version":  version,
				}
				if clearReleaseNote {
					payload["clearReleaseNote"] = true
				} else {
					payload["releaseNote"] = resolvedReleaseNote
				}
				return doAPICommand(cmd, app, "flows.versions.update", payload)
			}

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
			for i := range f.Versions {
				if f.Versions[i].Version != version {
					continue
				}
				if clearReleaseNote {
					f.Versions[i].Note = ""
				} else {
					f.Versions[i].Note = resolvedReleaseNote
				}
				f.UpdatedAt = time.Now().UTC()
				ws.UpdatedAt = f.UpdatedAt
				if err := store.Save(st); err != nil {
					return writeErr(cmd, err)
				}
				versionOut := map[string]any{
					"version": version,
					"note":    f.Versions[i].Note,
				}
				if strings.TrimSpace(f.Versions[i].Note) != "" {
					versionOut["releaseNote"] = f.Versions[i].Note
				}
				return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "version": versionOut})
			}
			return writeErr(cmd, errors.New("version not found"))
		},
	}

	cmd.Flags().IntVar(&version, "version", 0, "Version to update")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	cmd.Flags().BoolVar(&clearReleaseNote, "clear-release-note", false, "Clear the release note for this version")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsVersionsActivateCmd(app *App) *cobra.Command {
	var version int
	var deployKey string
	cmd := &cobra.Command{
		Use:   "activate <flow-slug>",
		Short: "Activate a published version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == 0 {
				return writeErr(cmd, errors.New("missing --version"))
			}
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0], "version": version}
				resolvedDeployKey := strings.TrimSpace(deployKey)
				if resolvedDeployKey == "" {
					resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
				}
				if resolvedDeployKey != "" {
					payload["deployKey"] = resolvedDeployKey
				}
				return doAPICommand(cmd, app, "flows.versions.activate", payload)
			}
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
			v, ok := findVersion(f, version)
			if !ok {
				return writeErr(cmd, errors.New("version not found"))
			}
			// Mock behavior: activation also swaps current draft to that snapshot.
			f.ActiveVersion = v.Version
			f.Name = v.Flow.Name
			f.Description = v.Flow.Description
			f.Tags = append([]string{}, v.Flow.Tags...)
			f.Spine = append([]string{}, v.Flow.Spine...)
			f.Steps = append([]state.FlowStep{}, v.Flow.Steps...)
			f.Spine = buildSpine(f)
			f.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = f.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"flow": f})
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "Version")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key (default: BREYTA_FLOW_DEPLOY_KEY)")
	return cmd
}

func newFlowsVersionsDiffCmd(app *App) *cobra.Command {
	var from, to int
	cmd := &cobra.Command{
		Use:   "diff <flow-slug>",
		Short: "Diff two versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if from == 0 || to == 0 {
					return writeErr(cmd, errors.New("missing --from and/or --to"))
				}
				return doAPICommand(cmd, app, "flows.diff", map[string]any{
					"flowSlug":    args[0],
					"from":        "version",
					"fromVersion": from,
					"to":          "version",
					"toVersion":   to,
				})
			}
			if from == 0 || to == 0 {
				return writeErr(cmd, errors.New("missing --from and/or --to"))
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			vf, ok := findVersion(f, from)
			if !ok {
				return writeErr(cmd, errors.New("from version not found"))
			}
			vt, ok := findVersion(f, to)
			if !ok {
				return writeErr(cmd, errors.New("to version not found"))
			}
			d := diffSteps(vf.Flow.Steps, vt.Flow.Steps)
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "from": from, "to": to, "diff": d})
		},
	}
	cmd.Flags().IntVar(&from, "from", 0, "From version")
	cmd.Flags().IntVar(&to, "to", 0, "To version")
	return cmd
}

// --- Validate ----------------------------------------------------------------

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

func parseCSV(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range splitNonEmpty(s) {
		out[p] = true
	}
	return out
}

func splitNonEmpty(s string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func findStep(f *state.Flow, id string) (state.FlowStep, bool) {
	for _, s := range f.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return state.FlowStep{}, false
}

func upsertStep(f *state.Flow, s state.FlowStep) {
	for i := range f.Steps {
		if f.Steps[i].ID == s.ID {
			f.Steps[i] = s
			return
		}
	}
	f.Steps = append(f.Steps, s)
}

func deleteStep(f *state.Flow, id string) bool {
	for i := range f.Steps {
		if f.Steps[i].ID == id {
			f.Steps = append(f.Steps[:i], f.Steps[i+1:]...)
			return true
		}
	}
	return false
}

func moveStep(f *state.Flow, stepID, before, after string) error {
	if before != "" && after != "" {
		return errors.New("cannot set both --before and --after")
	}
	idx := -1
	var step state.FlowStep
	for i := range f.Steps {
		if f.Steps[i].ID == stepID {
			idx = i
			step = f.Steps[i]
			break
		}
	}
	if idx == -1 {
		return errors.New("step not found")
	}
	// remove
	f.Steps = append(f.Steps[:idx], f.Steps[idx+1:]...)

	ref := before
	insertAfter := false
	if after != "" {
		ref = after
		insertAfter = true
	}
	pos := -1
	for i := range f.Steps {
		if f.Steps[i].ID == ref {
			pos = i
			break
		}
	}
	if pos == -1 {
		return errors.New("reference step not found")
	}
	if insertAfter {
		pos++
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(f.Steps) {
		pos = len(f.Steps)
	}
	f.Steps = append(f.Steps[:pos], append([]state.FlowStep{step}, f.Steps[pos:]...)...)
	return nil
}

func maxVersion(f *state.Flow) int {
	m := 0
	for _, v := range f.Versions {
		if v.Version > m {
			m = v.Version
		}
	}
	if f.ActiveVersion > m {
		m = f.ActiveVersion
	}
	return m
}

func flowRecordForSource(f *state.Flow, source string) (state.FlowRecord, string, int, error) {
	switch source {
	case "draft":
		return flowRecordFromFlow(f), "draft", 0, nil
	case "current":
		fallthrough
	case "active":
		if f.ActiveVersion > 0 {
			if v, ok := findVersion(f, f.ActiveVersion); ok {
				if source == "current" {
					return v.Flow, "current", v.Version, nil
				}
				return v.Flow, "active", v.Version, nil
			}
		}
		if len(f.Versions) == 0 {
			return flowRecordFromFlow(f), "draft", 0, nil
		}
		return state.FlowRecord{}, "", 0, errors.New("current/active version not found; push then release first")
	case "latest":
		if len(f.Versions) == 0 {
			return flowRecordFromFlow(f), "draft", 0, nil
		}
		latest := f.Versions[0]
		for _, v := range f.Versions[1:] {
			if v.Version > latest.Version {
				latest = v
			}
		}
		return latest.Flow, "latest", latest.Version, nil
	default:
		return state.FlowRecord{}, "", 0, fmt.Errorf("invalid source %q (expected current or latest)", source)
	}
}

func flowRecordFromFlow(f *state.Flow) state.FlowRecord {
	return state.FlowRecord{
		Name:        f.Name,
		Description: f.Description,
		Tags:        f.Tags,
		Spine:       f.Spine,
		Steps:       f.Steps,
	}
}

func findVersion(f *state.Flow, version int) (state.FlowVersion, bool) {
	for _, v := range f.Versions {
		if v.Version == version {
			return v, true
		}
	}
	return state.FlowVersion{}, false
}

func diffSteps(a, b []state.FlowStep) map[string]any {
	am := map[string]state.FlowStep{}
	bm := map[string]state.FlowStep{}
	for _, s := range a {
		am[s.ID] = s
	}
	for _, s := range b {
		bm[s.ID] = s
	}
	added := []string{}
	removed := []string{}
	changed := []map[string]any{}
	for id, s := range bm {
		if _, ok := am[id]; !ok {
			added = append(added, id)
			continue
		}
		sa := am[id]
		if sa.Type != s.Type || sa.Title != s.Title {
			changed = append(changed, map[string]any{"id": id, "from": map[string]any{"type": sa.Type, "title": sa.Title}, "to": map[string]any{"type": s.Type, "title": s.Title}})
		}
	}
	for id := range am {
		if _, ok := bm[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return map[string]any{"added": added, "removed": removed, "changed": changed}
}

func buildSpine(f *state.Flow) []string {
	lines := make([]string, 0, len(f.Steps))
	for _, s := range f.Steps {
		lines = append(lines, fmt.Sprintf("%s (%s) %s", s.ID, s.Type, s.Title))
	}
	return lines
}
