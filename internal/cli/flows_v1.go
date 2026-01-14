package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/breyta/breyta-cli/internal/clojure/parinfer"
	"github.com/breyta/breyta-cli/internal/state"
	"github.com/breyta/breyta-cli/internal/tools"

	"github.com/spf13/cobra"
)

var apiValidFlowSlugRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,127}$`)

func isAPIValidFlowSlug(s string) bool {
	return apiValidFlowSlugRe.MatchString(strings.TrimSpace(s))
}

// doAPICommandFn is a test hook to stub API calls in command unit tests.
var doAPICommandFn = doAPICommand

func newFlowsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "flows",
		Aliases: []string{"flow"},
		Short:   "Inspect and edit flows",
		Long: strings.TrimSpace(`
Flow authoring uses a file workflow:
1) pull a flow to a local .clj file
2) edit the file (Clojure map literal + DSL)
3) push -> creates a new draft version
4) deploy -> promotes latest version to active

Quick commands:
- breyta flows list
- breyta flows pull <slug> --out ./tmp/flows/<slug>.clj
- breyta flows push --file ./tmp/flows/<slug>.clj
- breyta flows deploy <slug>

Activation (credentials for :requires):
- If your flow declares :requires slots (e.g. :http-api with :auth/:oauth), activate it once to create a profile and bind credentials.
- Print activation URLs from the CLI: breyta flows activate-url <slug> and breyta flows draft-bindings-url <slug>
`),
	}

	cmd.AddCommand(newFlowsListCmd(app))
	cmd.AddCommand(newFlowsShowCmd(app))
	cmd.AddCommand(newFlowsCreateCmd(app))
	cmd.AddCommand(newFlowsActivateURLCmd(app))
	cmd.AddCommand(newFlowsDraftBindingsURLCmd(app))
	cmd.AddCommand(newFlowsPullCmd(app))
	cmd.AddCommand(newFlowsPushCmd(app))
	cmd.AddCommand(newFlowsParenRepairCmd(app))
	cmd.AddCommand(newFlowsParenCheckCmd(app))
	cmd.AddCommand(newFlowsDeployCmd(app))
	cmd.AddCommand(newFlowsUpdateCmd(app))
	cmd.AddCommand(newFlowsDeleteCmd(app))
	cmd.AddCommand(newFlowsSpineCmd(app))

	steps := &cobra.Command{Use: "steps", Short: "Manage flow steps"}
	steps.AddCommand(newFlowsStepsListCmd(app))
	steps.AddCommand(newFlowsStepsShowCmd(app))
	steps.AddCommand(newFlowsStepsSetCmd(app))
	steps.AddCommand(newFlowsStepsDeleteCmd(app))
	steps.AddCommand(newFlowsStepsMoveCmd(app))
	cmd.AddCommand(steps)

	versions := &cobra.Command{Use: "versions", Short: "Manage flow versions"}
	versions.AddCommand(newFlowsVersionsListCmd(app))
	versions.AddCommand(newFlowsVersionsPublishCmd(app))
	versions.AddCommand(newFlowsVersionsActivateCmd(app))
	versions.AddCommand(newFlowsVersionsDiffCmd(app))
	cmd.AddCommand(versions)

	cmd.AddCommand(newFlowsValidateCmd(app))
	cmd.AddCommand(newFlowsCompileCmd(app))

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

func newFlowsActivateURLCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate-url <flow-slug>",
		Short: "Print the activation URL for a flow",
		Long: strings.TrimSpace(`
Activation is where users provide credentials for :requires slots (including :llm-provider).

Example:
- Sign in: http://localhost:8090/login → Sign in with Google → Dev User
- Activate: http://localhost:8090/<workspace>/flows/<slug>/activate
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			base := strings.TrimRight(app.APIURL, "/")
			if strings.TrimSpace(base) == "" {
				// Default to local flows-api URL if API mode wasn't configured.
				base = "http://localhost:8090"
			}
			url := fmt.Sprintf("%s/%s/flows/%s/activate", base, app.WorkspaceID, args[0])
			return writeData(cmd, app, nil, map[string]any{
				"workspaceId":   app.WorkspaceID,
				"flowSlug":      args[0],
				"activationUrl": url,
			})
		},
	}
	return cmd
}

func newFlowsDraftBindingsURLCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "draft-bindings-url <flow-slug>",
		Short: "Print the draft bindings URL for preview runs",
		Long: strings.TrimSpace(`
Draft preview runs use a user-scoped draft profile. Bind credentials here:
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				// Server-side pagination defaults apply for now.
				// Keep --limit for mock mode; we can add it to the API later.
				return doAPICommand(cmd, app, "flows.list", map[string]any{})
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
				items = append(items, map[string]any{
					"flowSlug":       f.Slug,
					"name":           f.Name,
					"description":    f.Description,
					"tags":           f.Tags,
					"activeVersion":  f.ActiveVersion,
					"updatedAt":      f.UpdatedAt,
					"activeCount":    activeCount[f.Slug],
					"lastStatus":     lastStatus[f.Slug],
					"lastWorkflowId": lastWorkflow[f.Slug],
				})
			}

			meta := map[string]any{"total": total, "shown": len(items), "truncated": truncated}
			if truncated {
				meta["hint"] = "Use --limit 0 to show all flows"
			}

			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "Limit results (0 = all)")
	return cmd
}

func newFlowsShowCmd(app *App) *cobra.Command {
	var include string
	var source string
	var version int
	cmd := &cobra.Command{
		Use:   "show <flow-slug>",
		Short: "Show a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(source) != "" && source != "active" {
					payload["source"] = source
				}
				if version > 0 {
					payload["version"] = version
				}
				return doAPICommand(cmd, app, "flows.get", payload)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
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

			// Default: lightweight step list.
			steps := make([]map[string]any, 0, len(f.Steps))
			for _, s := range f.Steps {
				steps = append(steps, map[string]any{"id": s.ID, "type": s.Type, "title": s.Title})
			}
			out["steps"] = steps

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
	cmd.Flags().StringVar(&source, "source", "active", "Fetch source for API mode (active|latest|draft)")
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
				flowLiteral := fmt.Sprintf("{:slug :%s\n :name %q\n :description %q\n :tags [\"draft\"]\n :concurrency-config {:concurrency :singleton :on-new-version :supersede}\n :requires nil\n :templates nil\n :functions nil\n :triggers [{:type :manual :label \"Run\" :enabled true :config {}}]\n :definition '(defflow [input]\n              input)}\n", slug, name, description)
				return doAPICommand(cmd, app, "flows.put_draft", map[string]any{"flowLiteral": flowLiteral})
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
	var source string
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

			payload := map[string]any{"flowSlug": slug}
			if strings.TrimSpace(source) != "" && source != "active" {
				payload["source"] = source
			}
			if version > 0 {
				payload["version"] = version
			}

			client := apiClient(app)
			resp, status, err := client.DoCommand(context.Background(), "flows.get", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 {
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
			return writeData(cmd, app, nil, map[string]any{"saved": true, "path": path, "flowSlug": slug})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Output path (default: tmp/flows/<slug>.clj)")
	cmd.Flags().StringVar(&source, "source", "active", "Source (active|latest|draft)")
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = default)")
	return cmd
}

func newFlowsPushCmd(app *App) *cobra.Command {
	var file string
	var repairDelimiters bool
	var noRepairWriteback bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push a local .clj flow file as a new draft version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Push requires --api/BREYTA_API_URL")
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

			return doAPICommandFn(cmd, app, "flows.put_draft", map[string]any{"flowLiteral": flowLiteral})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Path to a flow .clj file")
	cmd.Flags().BoolVar(&repairDelimiters, "repair-delimiters", true, "Attempt best-effort delimiter repair before uploading")
	cmd.Flags().BoolVar(&noRepairWriteback, "no-repair-writeback", false, "Do not write repaired content back to --file (default: write back when changed)")
	must(cmd.MarkFlagRequired("file"))
	return cmd
}

func newFlowsDeployCmd(app *App) *cobra.Command {
	var version int
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
			return doAPICommand(cmd, app, "flows.deploy", payload)
		},
	}
	cmd.Flags().IntVar(&version, "version", 0, "Version (0 = latest)")
	return cmd
}

func newFlowsUpdateCmd(app *App) *cobra.Command {
	var name, description, tags string
	cmd := &cobra.Command{
		Use:   "update <flow-slug>",
		Short: "Update flow metadata",
		Args:  cobra.ExactArgs(1),
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
	return cmd
}

func newFlowsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <flow-slug>",
		Short: "Delete a flow",
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
	return cmd
}

// --- Steps ------------------------------------------------------------------

func newFlowsStepsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <flow-slug>",
		Short: "List steps",
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
				items = append(items, map[string]any{"version": v.Version, "publishedAt": v.PublishedAt, "note": v.Note})
			}
			sort.Slice(items, func(i, j int) bool { return items[i]["version"].(int) > items[j]["version"].(int) })
			meta := map[string]any{"activeVersion": f.ActiveVersion}
			return writeData(cmd, app, meta, map[string]any{"flowSlug": f.Slug, "items": items})
		},
	}
	return cmd
}

func newFlowsVersionsPublishCmd(app *App) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   "publish <flow-slug>",
		Short: "Publish a new immutable version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				payload := map[string]any{"flowSlug": args[0]}
				if strings.TrimSpace(note) != "" {
					payload["note"] = note
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
				Note:        note,
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
	cmd.Flags().StringVar(&note, "note", "", "Release note")
	return cmd
}

func newFlowsVersionsActivateCmd(app *App) *cobra.Command {
	var version int
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
				return writeNotImplemented(cmd, app, "Diff requires local store (API not implemented)")
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

// --- Validate/compile --------------------------------------------------------

func newFlowsValidateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <flow-slug>",
		Short: "Validate a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doAPICommand(cmd, app, "flows.validate", map[string]any{"flowSlug": args[0]})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			warnings := []map[string]any{}
			seen := map[string]bool{}
			for _, s := range f.Steps {
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
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "valid": len(warnings) == 0, "warnings": warnings})
		},
	}
	return cmd
}

func newFlowsCompileCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compile <flow-slug>",
		Short: "Compile a flow (mock)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				return doAPICommand(cmd, app, "flows.compile", map[string]any{"flowSlug": args[0]})
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			f, err := store.GetFlow(st, args[0])
			if err != nil {
				return writeErr(cmd, err)
			}
			plan := make([]map[string]any, 0, len(f.Steps))
			for idx, s := range f.Steps {
				plan = append(plan, map[string]any{"index": idx, "id": s.ID, "type": s.Type, "title": s.Title, "definition": s.Definition})
			}
			return writeData(cmd, app, nil, map[string]any{"flowSlug": f.Slug, "plan": plan})
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
