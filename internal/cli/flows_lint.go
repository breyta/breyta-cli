package cli

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/spf13/cobra"
)

type flowLintDiagnostic map[string]any

var (
	flowLintWorkspaceIDRe    = regexp.MustCompile(`\bws-[A-Za-z0-9_-]+\b`)
	flowLintUnboundedRangeRe = regexp.MustCompile(`\(\s*range\s*\)`)
)

func newFlowsLintCmd(app *App) *cobra.Command {
	var file string
	var server bool
	var localOnly bool

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint a local flow file before pushing",
		Long: strings.TrimSpace(`
Lint checks a candidate source file before it is written to Breyta.

Two stages are supported:
- local lint always runs first and never requires auth or network
- server lint sends the candidate flow literal for canonical, non-mutating API checks

Use ` + "`flows validate <slug>`" + ` after push to validate stored draft/live state.
`),
		Example: strings.TrimSpace(`
breyta flows lint --file ./flows/order-ingest.clj
breyta flows lint --file ./flows/order-ingest.clj --server
breyta flows lint --file ./flows/order-ingest.clj --local-only
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return writeErr(cmd, errors.New("missing --file"))
			}
			if server && localOnly {
				return writeErr(cmd, errors.New("--server cannot be combined with --local-only"))
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return writeErr(cmd, err)
			}

			flowLiteral := string(b)
			diagnostics := localFlowLintDiagnostics(file, flowLiteral)
			expandedLiteral := flowLiteral
			if expanded, err := expandFlowSourceIncludes(file, flowLiteral); err != nil {
				diagnostics = append(diagnostics, lintDiagnostic("error", "flow_include_invalid", []string{":flow"}, err.Error(), "Fix #flow/include paths before linting or pushing.", "local"))
			} else {
				expandedLiteral = expanded
			}

			meta := map[string]any{
				"stages": []string{"local"},
			}
			serverRequested := server
			serverCanRun := false
			if !localOnly && !lintHasErrors(diagnostics) {
				if serverRequested {
					if err := requireAPI(app); err != nil {
						return writeErr(cmd, err)
					}
					serverCanRun = true
				} else if lintServerContextAvailable(app) {
					serverCanRun = true
				}
			}
			if !localOnly && lintHasErrors(diagnostics) {
				meta["serverSkipped"] = "local_errors"
			}
			if localOnly {
				meta["serverSkipped"] = "local_only"
			}
			if !localOnly && !serverCanRun && !serverRequested {
				meta["serverSkipped"] = "no_api_context"
			}

			flowSlug := ""
			if serverCanRun {
				out, status, err := runAPICommand(app, "flows.lint", map[string]any{"flowLiteral": expandedLiteral})
				if err != nil {
					if serverRequested {
						return writeErr(cmd, err)
					}
					meta["serverSkipped"] = "api_error"
					meta["serverError"] = err.Error()
				} else if status >= 400 {
					if serverRequested {
						return writeAPIResult(cmd, app, out, status)
					}
					meta["serverSkipped"] = "api_status_" + fmt.Sprintf("%d", status)
					meta["serverError"] = formatAPIError(out)
				} else {
					meta["stages"] = []string{"local", "server"}
					if serverMeta, ok := out["meta"].(map[string]any); ok {
						if next, exists := serverMeta["nextCommands"]; exists {
							meta["nextCommands"] = next
						}
					}
					if data, ok := out["data"].(map[string]any); ok {
						if slug, _ := data["flowSlug"].(string); strings.TrimSpace(slug) != "" {
							flowSlug = strings.TrimSpace(slug)
						}
						diagnostics = append(diagnostics, serverFlowLintDiagnostics(data)...)
					}
				}
			}

			if !lintHasErrors(diagnostics) {
				meta["nextCommands"] = []string{"breyta flows push --file " + file}
				if flowSlug != "" {
					meta["nextCommands"] = append(meta["nextCommands"].([]string), "breyta flows validate "+flowSlug)
				}
			}

			return writeFlowLintResult(cmd, app, meta, flowSlug, diagnostics)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Path to local .clj flow source")
	cmd.Flags().BoolVar(&server, "server", false, "Require canonical server lint after local lint")
	cmd.Flags().BoolVar(&localOnly, "local-only", false, "Run only local lint checks; never call the API")
	return cmd
}

func lintDiagnostic(severity string, code string, path []string, message string, hint string, stage string) flowLintDiagnostic {
	out := flowLintDiagnostic{
		"severity": severity,
		"code":     code,
		"message":  message,
		"stage":    stage,
	}
	if len(path) > 0 {
		out["path"] = path
	}
	if strings.TrimSpace(hint) != "" {
		out["hint"] = strings.TrimSpace(hint)
	}
	return out
}

func localFlowLintDiagnostics(file string, flowLiteral string) []flowLintDiagnostic {
	var diagnostics []flowLintDiagnostic
	if err := parenrepair.Check(flowLiteral); err != nil {
		code := "clojure_syntax_invalid"
		hint := "Fix malformed Clojure/EDN before pushing."
		if errors.Is(err, parenrepair.ErrUnbalancedDelimiters) {
			code = "clojure_delimiters_invalid"
			hint = "Run: breyta flows paren-repair --write --file " + file
		}
		diagnostics = append(diagnostics, lintDiagnostic("error", code, []string{":flow"}, err.Error(), hint, "local"))
		return diagnostics
	}

	for _, key := range []string{":slug", ":concurrency", ":flow"} {
		if !strings.Contains(flowLiteral, key) {
			diagnostics = append(diagnostics, lintDiagnostic("error", "missing_required_field", []string{key}, "Missing required field "+key, "Add "+key+" before pushing.", "local"))
		}
	}
	if strings.Contains(flowLiteral, ":triggers") && strings.Contains(flowLiteral, ":manual") {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "deprecated_manual_trigger", []string{":triggers"}, "Manual triggers are legacy for new flow source.", "Use :interfaces {:manual [...]} with :invocations.", "local"))
	}
	if !strings.Contains(flowLiteral, ":interfaces") {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "missing_interfaces", []string{":interfaces"}, "Callable flows should expose user entrypoints with :interfaces.", "Add one manual interface; use invocation inputs such as mode for alternate manual paths.", "local"))
	}
	if !strings.Contains(flowLiteral, ":invocations") {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "missing_invocations", []string{":invocations"}, "Callable flows should declare run input contracts with :invocations.", "Move per-run fields into :invocations instead of trigger config fields.", "local"))
	}
	if flowLintWorkspaceIDRe.MatchString(flowLiteral) {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "hardcoded_workspace_id", []string{":flow"}, "Flow source appears to contain a hardcoded workspace id.", "Move workspace-specific ids into :requires, setup, or run input.", "local"))
	}
	if containsLongQuotedString(flowLiteral, 4000) {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "large_inline_string", []string{":flow"}, "Flow source contains a large inline string.", "Prefer :persist, templates, files, or resource refs for large payloads.", "local"))
	}
	if flowLintUnboundedRangeRe.MatchString(flowLiteral) {
		diagnostics = append(diagnostics, lintDiagnostic("warning", "sandbox_unbounded_range", []string{":flow"}, "Flow source calls unbounded (range), which is rejected by the runtime sandbox.", "Use a bounded range such as (range n), take from a finite collection, or derive limits from invocation inputs.", "local"))
	}
	return diagnostics
}

func containsLongQuotedString(s string, minLen int) bool {
	inString := false
	escaped := false
	currentLen := 0
	for _, r := range s {
		if !inString {
			if r == '"' {
				inString = true
				currentLen = 0
			}
			continue
		}
		if escaped {
			escaped = false
			currentLen++
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"':
			if currentLen >= minLen {
				return true
			}
			inString = false
		case '\n', '\r':
			inString = false
		default:
			currentLen++
			if currentLen >= minLen {
				return true
			}
		}
	}
	return inString && currentLen >= minLen
}

func lintServerContextAvailable(app *App) bool {
	if app == nil {
		return false
	}
	resolveAPIToken(app)
	return strings.TrimSpace(app.APIURL) != "" && strings.TrimSpace(app.Token) != ""
}

func serverFlowLintDiagnostics(data map[string]any) []flowLintDiagnostic {
	raw, _ := data["diagnostics"].([]any)
	out := make([]flowLintDiagnostic, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := m["stage"]; !exists {
			m["stage"] = "server"
		}
		out = append(out, flowLintDiagnostic(m))
	}
	return out
}

func lintHasErrors(diagnostics []flowLintDiagnostic) bool {
	for _, d := range diagnostics {
		if sev, _ := d["severity"].(string); strings.EqualFold(sev, "error") {
			return true
		}
	}
	return false
}

func writeFlowLintResult(cmd *cobra.Command, app *App, meta map[string]any, flowSlug string, diagnostics []flowLintDiagnostic) error {
	valid := !lintHasErrors(diagnostics)
	data := map[string]any{
		"valid":       valid,
		"diagnostics": diagnostics,
	}
	if strings.TrimSpace(flowSlug) != "" {
		data["flowSlug"] = strings.TrimSpace(flowSlug)
	}
	out := map[string]any{
		"ok":          valid,
		"workspaceId": app.WorkspaceID,
		"meta":        meta,
		"data":        data,
	}
	enrichEnvelopeWebLinks(app, out)
	if err := writeOut(cmd, app, out); err != nil {
		return err
	}
	if !valid {
		return guidedCLIErrorForCommand(cmd, "flow lint found errors", nil)
	}
	return nil
}
