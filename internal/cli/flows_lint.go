package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/spf13/cobra"
)

type flowLintDiagnostic map[string]any

var (
	flowLintWorkspaceIDRe    = regexp.MustCompile(`\bws-[A-Za-z0-9_-]+\b`)
	flowLintUnboundedRangeRe = regexp.MustCompile(`\(\s*range\s*\)`)
	flowLintInvocationTypes  = map[string]bool{
		"string": true, "text": true, "number": true, "email": true, "password": true,
		"textarea": true, "boolean": true, "checkbox": true, "select": true,
		"date": true, "time": true, "datetime": true, "json": true, "file": true,
		"blob": true, "blob-ref": true, "resource": true, "secret": true,
	}
)

const defaultFlowLintServerTimeout = 30 * time.Second

func newFlowsLintCmd(app *App) *cobra.Command {
	var file string
	var server bool
	var localOnly bool
	var serverTimeout time.Duration

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
breyta flows lint --file ./flows/order-ingest.clj --server --timeout 2m
breyta flows lint --file ./flows/order-ingest.clj --local-only
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return writeErr(cmd, errors.New("missing --file"))
			}
			if server && localOnly {
				return writeErr(cmd, errors.New("--server cannot be combined with --local-only"))
			}
			if serverTimeout <= 0 {
				return writeErr(cmd, errors.New("--timeout must be > 0"))
			}
			b, err := readExplicitFile(file)
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
				diagnostics = append(diagnostics, localReaderEvalDiagnostics(expandedLiteral)...)
				diagnostics = append(diagnostics, localAuthoringShapeDiagnostics(expandedLiteral)...)
				diagnostics = append(diagnostics, localFunctionCodeStringDiagnostics(expandedLiteral)...)
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
				out, status, err := runAPICommandWithContextAndTimeout(cmd.Context(), app, "flows.lint", map[string]any{"flowLiteral": expandedLiteral}, serverTimeout)
				if err != nil {
					err = flowLintServerError(err, serverTimeout)
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
	cmd.Flags().DurationVar(&serverTimeout, "timeout", defaultFlowLintServerTimeout, "Server lint request timeout")
	return cmd
}

func flowLintServerError(err error, timeout time.Duration) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("flows lint server timed out after %s; rerun with --local-only or increase --timeout", timeout)
	}
	return err
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
	if topLevelConcurrencyValueIsNil(flowLiteral) {
		diagnostics = append(diagnostics, lintDiagnostic("error", "invalid_required_field", []string{":concurrency"}, ":concurrency cannot be nil.", "Use a concurrency map such as {:type :singleton :on-new-version :coexist} before pushing.", "local"))
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

func localReaderEvalDiagnostics(flowLiteral string) []flowLintDiagnostic {
	for i := 0; i < len(flowLiteral); {
		switch flowLiteral[i] {
		case '"':
			_, _, next, err := readClojureStringToken(flowLiteral, i)
			if err != nil || next <= i {
				i++
			} else {
				i = next
			}
			continue
		case ';':
			i = readCommentEnd(flowLiteral, i)
			continue
		}
		if strings.HasPrefix(flowLiteral[i:], "#=") {
			diag := lintDiagnostic(
				"error",
				"clojure_reader_eval_disabled",
				[]string{":flow"},
				"Flow source uses reader eval (#=), which is not allowed during safe Clojure reading.",
				"Replace reader-eval forms with ordinary data or runtime code that does not execute while the source is read.",
				"local",
			)
			diag["byteOffset"] = i
			return []flowLintDiagnostic{diag}
		}
		i++
	}
	return nil
}

func topLevelConcurrencyValueIsNil(src string) bool {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	for i < len(src) {
		switch {
		case src[i] == '{':
			return topLevelMapValueIsNil(src, i, "concurrency")
		case src[i] == '^':
			metaEnd, err := readClojureFormEnd(src, i+1)
			if err != nil || metaEnd <= i+1 {
				return false
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardEnd, err := readClojureFormEnd(src, i+2)
			if err != nil || discardEnd <= i+2 {
				return false
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		case strings.HasPrefix(src[i:], "#?"):
			formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
			if !ok || formStart < 0 {
				return false
			}
			return topLevelConcurrencyValueIsNil(src[formStart:formEnd])
		default:
			return false
		}
	}
	return false
}

func topLevelMapValueIsNil(src string, start int, targetKey string) bool {
	i := start + 1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) || src[i] == '}' {
			return false
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil || keyEnd <= keyStart {
			return false
		}
		key := clojureKeywordName(src[keyStart:keyEnd])
		valueStart := skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if valueStart >= len(src) {
			return false
		}
		if key == targetKey {
			return clojureFormIsNil(src, valueStart)
		}
		valueEnd, err := readClojureFormEnd(src, valueStart)
		if err != nil || valueEnd <= valueStart {
			return false
		}
		i = valueEnd
	}
	return false
}

func clojureFormIsNil(src string, start int) bool {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) {
		return false
	}
	if strings.HasPrefix(src[i:], "#?") {
		formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
		if !ok || formStart < 0 {
			return false
		}
		return clojureFormIsNil(src[formStart:formEnd], 0)
	}
	if src[i] == '^' {
		metaEnd, err := readClojureFormEnd(src, i+1)
		if err != nil || metaEnd <= i+1 {
			return false
		}
		return clojureFormIsNil(src, metaEnd)
	}
	if strings.HasPrefix(src[i:], "#_") {
		discardEnd, err := readClojureFormEnd(src, i+2)
		if err != nil || discardEnd <= i+2 {
			return false
		}
		return clojureFormIsNil(src, discardEnd)
	}
	end := readClojureTokenEnd(src, i)
	return end > i && src[i:end] == "nil"
}

type clojureFormSpan struct {
	Start int
	End   int
}

type clojureMapEntry struct {
	KeyToken   string
	KeyName    string
	KeyStart   int
	KeyEnd     int
	ValueStart int
	ValueEnd   int
}

func localAuthoringShapeDiagnostics(flowLiteral string) []flowLintDiagnostic {
	entries, err := extractTopLevelMapEntries(flowLiteral)
	if err != nil {
		return []flowLintDiagnostic{lintDiagnostic(
			"warning",
			"authoring_shape_scan_incomplete",
			[]string{":flow"},
			fmt.Sprintf("Local authoring shape validation could not scan the top-level flow map: %v", err),
			"Run `breyta flows lint --server` before pushing for canonical schema validation.",
			"local",
		)}
	}
	var diagnostics []flowLintDiagnostic
	byKey := map[string]clojureMapEntry{}
	for _, entry := range entries {
		if entry.KeyName != "" {
			byKey[entry.KeyName] = entry
		}
	}
	invocationIDs, foundInvocations, invocationDiagnostics := localInvocationShapeDiagnostics(flowLiteral, byKey["invocations"])
	diagnostics = append(diagnostics, invocationDiagnostics...)
	diagnostics = append(diagnostics, localInterfaceShapeDiagnostics(flowLiteral, byKey["interfaces"], invocationIDs, foundInvocations)...)
	diagnostics = append(diagnostics, localFunctionStepShapeDiagnostics(flowLiteral)...)
	return diagnostics
}

func extractTopLevelMapEntries(src string) ([]clojureMapEntry, error) {
	start, err := topLevelFlowMapStart(src)
	if err != nil || start < 0 {
		return nil, err
	}
	entries, _, err := parseClojureMapEntries(src, start)
	return entries, err
}

func parseClojureMapEntries(src string, start int) ([]clojureMapEntry, int, error) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) || src[i] != '{' {
		return nil, i, fmt.Errorf("expected map near byte %d", start)
	}
	i++
	var entries []clojureMapEntry
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return entries, i, fmt.Errorf("unterminated map")
		}
		if src[i] == '}' {
			return entries, i + 1, nil
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, keyStart)
		if err != nil || keyEnd <= keyStart {
			if err == nil {
				err = fmt.Errorf("could not read map key near byte %d", keyStart)
			}
			return entries, keyEnd, err
		}
		valueStart := skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if valueStart >= len(src) || src[valueStart] == '}' {
			return entries, valueStart, fmt.Errorf("missing map value for key %s near byte %d", src[keyStart:keyEnd], keyStart)
		}
		valueEnd, err := readClojureFormEnd(src, valueStart)
		if err != nil || valueEnd <= valueStart {
			if err == nil {
				err = fmt.Errorf("could not read map value for key %s near byte %d", src[keyStart:keyEnd], valueStart)
			}
			return entries, valueEnd, err
		}
		keyToken := strings.TrimSpace(src[keyStart:keyEnd])
		entries = append(entries, clojureMapEntry{
			KeyToken:   keyToken,
			KeyName:    clojureKeywordName(keyToken),
			KeyStart:   keyStart,
			KeyEnd:     keyEnd,
			ValueStart: valueStart,
			ValueEnd:   valueEnd,
		})
		i = valueEnd
	}
	return entries, i, fmt.Errorf("unterminated map")
}

func parseClojureVectorElements(src string, start int) ([]clojureFormSpan, int, error) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) || src[i] != '[' {
		return nil, i, fmt.Errorf("expected vector near byte %d", start)
	}
	i++
	var out []clojureFormSpan
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, i, fmt.Errorf("unterminated vector")
		}
		if src[i] == ']' {
			return out, i + 1, nil
		}
		end, err := readClojureFormEnd(src, i)
		if err != nil || end <= i {
			if err == nil {
				err = fmt.Errorf("could not read vector element near byte %d", i)
			}
			return out, end, err
		}
		out = append(out, clojureFormSpan{Start: i, End: end})
		i = end
	}
	return out, i, fmt.Errorf("unterminated vector")
}

func parseClojureListElements(src string, start int) ([]clojureFormSpan, int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) || src[i] != '(' {
		return nil, i, fmt.Errorf("expected list near byte %d", start)
	}
	i++
	var out []clojureFormSpan
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, i, fmt.Errorf("unterminated list")
		}
		if src[i] == ')' {
			return out, i + 1, nil
		}
		end, err := readClojureFormEnd(src, i)
		if err != nil || end <= i {
			if err == nil {
				err = fmt.Errorf("could not read list element near byte %d", i)
			}
			return out, end, err
		}
		out = append(out, clojureFormSpan{Start: i, End: end})
		i = end
	}
	return out, i, fmt.Errorf("unterminated list")
}

func clojureActiveFormStart(src string, start int) (int, bool) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	for i < len(src) {
		switch {
		case strings.HasPrefix(src[i:], "#?"):
			formStart, _, _, ok := activeReaderConditionalForm(src, i)
			if !ok || formStart < 0 {
				return i, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, formStart)
		case src[i] == '^':
			metaEnd, err := readClojureFormEnd(src, i+1)
			if err != nil || metaEnd <= i+1 {
				return i, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardEnd, err := readClojureFormEnd(src, i+2)
			if err != nil || discardEnd <= i+2 {
				return i, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		default:
			return i, true
		}
	}
	return i, false
}

func clojureFormStartsWith(src string, start int, ch byte) bool {
	i, ok := clojureActiveFormStart(src, start)
	return ok && i < len(src) && src[i] == ch
}

func clojureFormToken(src string, span clojureFormSpan) string {
	if span.Start < 0 || span.End > len(src) || span.End <= span.Start {
		return ""
	}
	return strings.TrimSpace(src[span.Start:span.End])
}

func clojureIdentifierFromForm(src string, start int) (string, bool) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) {
		return "", false
	}
	if src[i] == '"' {
		_, value, _, err := readClojureStringToken(src, i)
		if err != nil {
			return "", false
		}
		value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
		return value, validFlowLintSafeIdentifier(value)
	}
	end, err := readClojureFormEnd(src, i)
	if err != nil || end <= i {
		return "", false
	}
	token := strings.TrimSpace(src[i:end])
	if !strings.HasPrefix(token, ":") || strings.Contains(token, "/") {
		return "", false
	}
	name := strings.TrimPrefix(token, ":")
	return name, validFlowLintSafeIdentifier(name)
}

func clojureNonBlankStringFromForm(src string, start int) (string, bool) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) || src[i] != '"' {
		return "", false
	}
	_, value, _, err := readClojureStringToken(src, i)
	if err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func validFlowLintSafeIdentifier(s string) bool {
	if s == "" || len([]rune(s)) > 128 {
		return false
	}
	for idx, r := range s {
		if idx == 0 {
			if !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func mapEntryByKey(entries []clojureMapEntry, key string) (clojureMapEntry, bool) {
	for _, entry := range entries {
		if entry.KeyName == key {
			return entry, true
		}
	}
	return clojureMapEntry{}, false
}

func localInvocationShapeDiagnostics(src string, entry clojureMapEntry) (map[string]bool, bool, []flowLintDiagnostic) {
	invocationIDs := map[string]bool{}
	if entry.ValueStart <= 0 && entry.ValueEnd <= 0 {
		return invocationIDs, false, nil
	}
	if clojureFormIsNil(src, entry.ValueStart) {
		return invocationIDs, false, nil
	}
	if !clojureFormStartsWith(src, entry.ValueStart, '{') {
		return invocationIDs, true, []flowLintDiagnostic{lintDiagnostic(
			"error",
			"invalid_invocations_shape",
			[]string{":invocations"},
			":invocations must be a map keyed by invocation id.",
			"Use a shape such as :invocations {:default {:inputs [...]}}.",
			"local",
		)}
	}
	entries, _, err := parseClojureMapEntries(src, entry.ValueStart)
	if err != nil {
		return invocationIDs, true, []flowLintDiagnostic{lintDiagnostic(
			"warning",
			"invocations_shape_scan_incomplete",
			[]string{":invocations"},
			fmt.Sprintf("Local lint could not scan :invocations: %v", err),
			"Run `breyta flows lint --server` before pushing for canonical schema validation.",
			"local",
		)}
	}
	var diagnostics []flowLintDiagnostic
	for _, inv := range entries {
		id := ""
		if strings.HasPrefix(inv.KeyToken, ":") && !strings.Contains(inv.KeyToken, "/") {
			id = strings.TrimPrefix(inv.KeyToken, ":")
			if validFlowLintSafeIdentifier(id) {
				invocationIDs[id] = true
			}
		}
		if id == "" || !validFlowLintSafeIdentifier(id) {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"invalid_invocation_id",
				[]string{":invocations"},
				fmt.Sprintf("Invocation id %s must be an unqualified safe keyword.", strings.TrimSpace(inv.KeyToken)),
				"Use ids like :default or :run, not strings, namespaced keywords, or arbitrary forms.",
				"local",
			))
		}
		if !clojureFormStartsWith(src, inv.ValueStart, '{') {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"invalid_invocation_shape",
				[]string{":invocations", inv.KeyToken},
				"Each invocation value must be a map.",
				"Use a shape such as :default {:inputs [{:name :query :type :text}]}",
				"local",
			))
			continue
		}
		invEntries, _, err := parseClojureMapEntries(src, inv.ValueStart)
		if err != nil {
			continue
		}
		if inputs, ok := mapEntryByKey(invEntries, "inputs"); ok {
			diagnostics = append(diagnostics, localInvocationInputsDiagnostics(src, inv.KeyToken, inputs)...)
		}
	}
	return invocationIDs, true, diagnostics
}

func localInvocationInputsDiagnostics(src string, invocationToken string, inputs clojureMapEntry) []flowLintDiagnostic {
	if !clojureFormStartsWith(src, inputs.ValueStart, '[') {
		return []flowLintDiagnostic{lintDiagnostic(
			"error",
			"invalid_invocation_inputs_shape",
			[]string{":invocations", invocationToken, ":inputs"},
			"Invocation :inputs must be a vector of input maps.",
			"Use :inputs [{:name :query :type :text :required true}].",
			"local",
		)}
	}
	items, _, err := parseClojureVectorElements(src, inputs.ValueStart)
	if err != nil {
		return []flowLintDiagnostic{lintDiagnostic(
			"warning",
			"invocation_inputs_scan_incomplete",
			[]string{":invocations", invocationToken, ":inputs"},
			fmt.Sprintf("Local lint could not scan invocation inputs: %v", err),
			"Run `breyta flows lint --server` before pushing for canonical schema validation.",
			"local",
		)}
	}
	var diagnostics []flowLintDiagnostic
	names := map[string]bool{}
	for idx, item := range items {
		path := []string{":invocations", invocationToken, ":inputs", fmt.Sprintf("[%d]", idx)}
		if !clojureFormStartsWith(src, item.Start, '{') {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"invalid_invocation_input_shape",
				path,
				"Each invocation input must be a map.",
				"Use an input map such as {:name :query :type :text}.",
				"local",
			))
			continue
		}
		entries, _, err := parseClojureMapEntries(src, item.Start)
		if err != nil {
			continue
		}
		nameEntry, hasName := mapEntryByKey(entries, "name")
		if !hasName {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"missing_invocation_input_name",
				append(path, ":name"),
				"Invocation input is missing required :name.",
				"Add :name with a safe keyword or string such as :query.",
				"local",
			))
		} else if name, ok := clojureIdentifierFromForm(src, nameEntry.ValueStart); !ok {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"invalid_invocation_input_name",
				append(path, ":name"),
				"Invocation input :name must be a safe identifier.",
				"Use a keyword or string like :query, :repo, or :branch.",
				"local",
			))
		} else if names[name] {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"duplicate_invocation_input_name",
				append(path, ":name"),
				fmt.Sprintf("Invocation input name %q is duplicated.", name),
				"Keep input names unique within each invocation.",
				"local",
			))
		} else {
			names[name] = true
		}
		if typeEntry, hasType := mapEntryByKey(entries, "type"); hasType {
			typeName, ok := clojureIdentifierFromForm(src, typeEntry.ValueStart)
			if !ok || !flowLintInvocationTypes[typeName] {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"invalid_invocation_input_type",
					append(path, ":type"),
					"Invocation input :type is not a supported input type.",
					"Use types such as :text, :string, :number, :boolean, :json, :file, :resource, or :secret.",
					"local",
				))
			}
		}
	}
	return diagnostics
}

func localInterfaceShapeDiagnostics(src string, entry clojureMapEntry, invocationIDs map[string]bool, foundInvocations bool) []flowLintDiagnostic {
	if entry.ValueStart <= 0 && entry.ValueEnd <= 0 {
		return nil
	}
	if clojureFormIsNil(src, entry.ValueStart) {
		return nil
	}
	if !clojureFormStartsWith(src, entry.ValueStart, '{') {
		return []flowLintDiagnostic{lintDiagnostic(
			"error",
			"invalid_interfaces_shape",
			[]string{":interfaces"},
			":interfaces must be a map of interface categories.",
			"Use a shape such as :interfaces {:manual [{:id :run :invocation :default}]}",
			"local",
		)}
	}
	entries, _, err := parseClojureMapEntries(src, entry.ValueStart)
	if err != nil {
		return []flowLintDiagnostic{lintDiagnostic(
			"warning",
			"interfaces_shape_scan_incomplete",
			[]string{":interfaces"},
			fmt.Sprintf("Local lint could not scan :interfaces: %v", err),
			"Run `breyta flows lint --server` before pushing for canonical schema validation.",
			"local",
		)}
	}
	var diagnostics []flowLintDiagnostic
	identifiers := map[string]string{}
	for _, category := range entries {
		switch category.KeyName {
		case "manual", "http", "webhook", "mcp":
		default:
			continue
		}
		path := []string{":interfaces", ":" + category.KeyName}
		if !clojureFormStartsWith(src, category.ValueStart, '[') {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"invalid_interface_category_shape",
				path,
				fmt.Sprintf(":interfaces :%s must be a vector of interface maps.", category.KeyName),
				"Use vectors, for example :manual [{:id :run :invocation :default}].",
				"local",
			))
			continue
		}
		items, _, err := parseClojureVectorElements(src, category.ValueStart)
		if err != nil {
			continue
		}
		if category.KeyName != "mcp" && len(items) > 1 {
			diagnostics = append(diagnostics, lintDiagnostic(
				"error",
				"too_many_interfaces",
				path,
				fmt.Sprintf(":interfaces :%s supports at most one entry.", category.KeyName),
				"Keep a single manual, HTTP, or webhook interface per flow for this source shape.",
				"local",
			))
		}
		for idx, item := range items {
			itemPath := append(path, fmt.Sprintf("[%d]", idx))
			if !clojureFormStartsWith(src, item.Start, '{') {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"invalid_interface_shape",
					itemPath,
					"Each interface entry must be a map.",
					"Use an interface map with :id or :tool-name plus :invocation.",
					"local",
				))
				continue
			}
			itemEntries, _, err := parseClojureMapEntries(src, item.Start)
			if err != nil {
				continue
			}
			idKey := "id"
			if category.KeyName == "mcp" {
				idKey = "tool-name"
			}
			idEntry, hasID := mapEntryByKey(itemEntries, idKey)
			if !hasID {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"missing_interface_id",
					append(itemPath, ":"+idKey),
					fmt.Sprintf(":%s interface entry is missing required :%s.", category.KeyName, idKey),
					"Add a stable interface identifier.",
					"local",
				))
			} else {
				var id string
				var ok bool
				if category.KeyName == "mcp" {
					id, ok = clojureNonBlankStringFromForm(src, idEntry.ValueStart)
				} else {
					id, ok = clojureIdentifierFromForm(src, idEntry.ValueStart)
				}
				if !ok {
					message := fmt.Sprintf("Interface :%s must be a safe identifier.", idKey)
					hint := "Use values like :run, :enrich, or \"enrich_company\"."
					if category.KeyName == "mcp" {
						message = "MCP interface :tool-name must be a nonblank string."
						hint = "Use a string tool name, for example :tool-name \"enrich_company\"."
					}
					diagnostics = append(diagnostics, lintDiagnostic(
						"error",
						"invalid_interface_id",
						append(itemPath, ":"+idKey),
						message,
						hint,
						"local",
					))
					continue
				}
				if prev, exists := identifiers[id]; exists {
					diagnostics = append(diagnostics, lintDiagnostic(
						"error",
						"duplicate_interface_id",
						append(itemPath, ":"+idKey),
						fmt.Sprintf("Interface identifier %q is duplicated with %s.", id, prev),
						"Keep interface ids and MCP tool names unique.",
						"local",
					))
				} else {
					identifiers[id] = strings.Join(itemPath, " ")
				}
			}
			invEntry, hasInvocation := mapEntryByKey(itemEntries, "invocation")
			if !hasInvocation {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"missing_interface_invocation",
					append(itemPath, ":invocation"),
					"Interface entry is missing required :invocation.",
					"Reference a declared invocation id, for example :invocation :default.",
					"local",
				))
				continue
			}
			invocationName, ok := clojureIdentifierFromForm(src, invEntry.ValueStart)
			if !ok {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"invalid_interface_invocation",
					append(itemPath, ":invocation"),
					"Interface :invocation must be a safe identifier.",
					"Use a keyword or string matching a key in :invocations.",
					"local",
				))
				continue
			}
			if !foundInvocations || !invocationIDs[invocationName] {
				diagnostics = append(diagnostics, lintDiagnostic(
					"error",
					"unknown_interface_invocation",
					append(itemPath, ":invocation"),
					fmt.Sprintf("Interface references unknown invocation %q.", invocationName),
					"Declare the invocation under :invocations, for example :invocations {:default {:inputs [...]}}.",
					"local",
				))
			}
		}
	}
	return diagnostics
}

func localFunctionStepShapeDiagnostics(src string) []flowLintDiagnostic {
	var diagnostics []flowLintDiagnostic
	for i := 0; i < len(src); {
		switch src[i] {
		case '"':
			_, _, next, err := readClojureStringToken(src, i)
			if err != nil || next <= i {
				i++
			} else {
				i = next
			}
			continue
		case ';':
			i = readCommentEnd(src, i)
			continue
		case '(':
			elements, _, err := parseClojureListElements(src, i)
			if err == nil {
				diagnostics = append(diagnostics, localFunctionStepDiagnosticsForList(src, elements, i)...)
			}
		}
		i++
	}
	return diagnostics
}

func localFunctionStepDiagnosticsForList(src string, elements []clojureFormSpan, listStart int) []flowLintDiagnostic {
	if len(elements) == 0 || clojureFormToken(src, elements[0]) != "flow/step" {
		return nil
	}
	if len(elements) < 2 {
		return nil
	}
	stepType := clojureFormToken(src, elements[1])
	if stepType != ":function" && stepType != ":code" {
		return nil
	}
	stepID := "<missing>"
	if len(elements) >= 3 {
		if id, ok := clojureIdentifierFromForm(src, elements[2].Start); ok {
			stepID = ":" + id
		} else {
			stepID = strings.TrimSpace(src[elements[2].Start:elements[2].End])
		}
	}
	path := []string{":flow", stepID}
	var diagnostics []flowLintDiagnostic
	if len(elements) != 4 {
		diag := lintDiagnostic(
			"error",
			"function_step_arity_invalid",
			path,
			"Function steps must use exactly three arguments after flow/step: step type, step id, and config map.",
			"Put :code, :ref, :input, :persist, and related fields inside the single config map.",
			"local",
		)
		diag["byteOffset"] = listStart
		diagnostics = append(diagnostics, diag)
	}
	if len(elements) < 4 {
		return diagnostics
	}
	config := elements[3]
	if !clojureFormStartsWith(src, config.Start, '{') {
		diag := lintDiagnostic(
			"error",
			"function_step_config_invalid",
			path,
			"Function step config must be a map.",
			"Use (flow/step :function :step-id {:input {...} :code '(fn [input] ...)}).",
			"local",
		)
		diag["byteOffset"] = config.Start
		diagnostics = append(diagnostics, diag)
		return diagnostics
	}
	entries, _, err := parseClojureMapEntries(src, config.Start)
	if err != nil {
		return diagnostics
	}
	hasCode := false
	hasRef := false
	if _, ok := mapEntryByKey(entries, "code"); ok {
		hasCode = true
	}
	if _, ok := mapEntryByKey(entries, "ref"); ok {
		hasRef = true
	}
	if hasCode && hasRef {
		diagnostics = append(diagnostics, lintDiagnostic(
			"error",
			"function_step_code_ref_conflict",
			path,
			"Function step config cannot include both :code and :ref.",
			"Use inline :code or reference one top-level :functions entry with :ref, not both.",
			"local",
		))
	} else if !hasCode && !hasRef {
		diagnostics = append(diagnostics, lintDiagnostic(
			"error",
			"function_step_missing_code_or_ref",
			path,
			"Function step config requires either :code or :ref.",
			"Add inline :code or reference a top-level function with :ref.",
			"local",
		))
	}
	if input, ok := mapEntryByKey(entries, "input"); ok && !clojureFormStartsWith(src, input.ValueStart, '{') {
		diag := lintDiagnostic(
			"error",
			"function_step_input_shape_invalid",
			append(path, ":input"),
			"Function step :input must be a map when present.",
			"Wrap runtime values in an input map, for example :input {:input input} or :input {:rows rows}.",
			"local",
		)
		diag["byteOffset"] = input.ValueStart
		diagnostics = append(diagnostics, diag)
	}
	return diagnostics
}

type functionCodeString struct {
	Code       string
	Path       []string
	ByteOffset int
}

func localFunctionCodeStringDiagnostics(flowLiteral string) []flowLintDiagnostic {
	codes, err := extractTopLevelFunctionCodeStrings(flowLiteral)
	diagnostics := make([]flowLintDiagnostic, 0)
	if err != nil {
		diagnostics = append(diagnostics, lintDiagnostic(
			"warning",
			"function_code_string_scan_incomplete",
			[]string{":functions"},
			fmt.Sprintf("Function :code string validation fell back to a best-effort scan: %v", err),
			"Remove unsupported reader syntax from the top-level flow source or use directly quoted function forms so local lint can validate every function.",
			"local",
		))
		codes = bestEffortFunctionCodeStrings(flowLiteral)
	}
	for _, code := range codes {
		if err := validateFunctionCodeString(code.Code); err != nil {
			diag := lintDiagnostic(
				"error",
				"function_code_string_invalid",
				code.Path,
				fmt.Sprintf("Function :code string is not readable: %v", err),
				"Fix the string code or use a directly quoted form, for example :code '(fn [input] ...).",
				"local",
			)
			diag["byteOffset"] = code.ByteOffset
			diagnostics = append(diagnostics, diag)
		}
	}
	return diagnostics
}

func validateFunctionCodeString(code string) error {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return errors.New("empty function code")
	}
	if err := parenrepair.Check(trimmed); err != nil {
		return err
	}
	start := skipClojureWhitespaceCommaAndComments(trimmed, 0)
	next, err := readClojureFormEnd(trimmed, start)
	if err != nil {
		return err
	}
	if next <= start {
		return errors.New("could not read function code form")
	}
	end := skipClojureWhitespaceCommaAndComments(trimmed, next)
	if end < len(trimmed) {
		return errors.New("trailing content after function code form")
	}
	return nil
}

func bestEffortFunctionCodeStrings(src string) []functionCodeString {
	codes, _ := bestEffortTopLevelFunctionCodeStrings(src, 0)
	return codes
}

func bestEffortTopLevelFunctionCodeStrings(src string, baseOffset int) ([]functionCodeString, bool) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	for i < len(src) {
		switch {
		case src[i] == '{':
			return bestEffortFunctionCodeStringsInTopLevelMap(src, i, baseOffset), true
		case src[i] == '^':
			metaStart := i
			metaEnd, err := readClojureFormEnd(src, i+1)
			if err != nil || metaEnd <= i+1 {
				return nil, false
			}
			if metaEnd <= metaStart {
				return nil, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardStart := i
			discardEnd, err := readClojureFormEnd(src, i+2)
			if err != nil || discardEnd <= i+2 {
				return nil, false
			}
			if discardEnd <= discardStart {
				return nil, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		case strings.HasPrefix(src[i:], "#?"):
			formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
			if !ok {
				return nil, false
			}
			if formStart < 0 {
				return nil, true
			}
			return bestEffortTopLevelFunctionCodeStrings(src[formStart:formEnd], baseOffset+formStart)
		default:
			return nil, false
		}
	}
	return nil, true
}

func bestEffortFunctionCodeStringsInTopLevelMap(src string, start int, baseOffset int) []functionCodeString {
	var out []functionCodeString
	i := start + 1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) || src[i] == '}' {
			return out
		}

		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil || keyEnd <= keyStart {
			next := skipTopLevelMapValueBestEffort(src, i)
			if next <= i {
				i++
			} else {
				i = next
			}
			continue
		}

		key := clojureKeywordName(src[keyStart:keyEnd])
		valueStart := skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if valueStart >= len(src) {
			return out
		}
		if key == "functions" {
			codes, next, err := extractFunctionsValueCodeStrings(src, valueStart)
			offsetFunctionCodeStrings(codes, baseOffset)
			out = append(out, codes...)
			if err == nil && next > valueStart {
				i = next
			} else {
				i = skipTopLevelMapValueBestEffort(src, valueStart)
				if i <= valueStart {
					return out
				}
			}
			continue
		}

		next, err := readClojureFormEnd(src, valueStart)
		if err == nil && next > valueStart {
			i = next
			continue
		}
		next = skipTopLevelMapValueBestEffort(src, valueStart)
		if next <= valueStart {
			i = valueStart + 1
		} else {
			i = next
		}
	}
	return out
}

func skipTopLevelMapValueBestEffort(src string, start int) int {
	i := start
	depth := 0
	consumed := false
	for i < len(src) {
		if depth == 0 && consumed {
			next := skipClojureWhitespaceCommaAndComments(src, i)
			if next != i {
				i = next
				if i >= len(src) || src[i] == '}' || isProbableTopLevelMapKey(src, i) {
					return i
				}
				continue
			}
			if src[i] == '}' || isProbableTopLevelMapKey(src, i) {
				return i
			}
		}

		switch src[i] {
		case '"':
			_, _, next, err := readClojureStringToken(src, i)
			if err != nil || next <= i {
				i++
			} else {
				i = next
			}
			consumed = true
		case ';':
			i = readCommentEnd(src, i)
		case '(', '[', '{':
			depth++
			i++
			consumed = true
		case ')', ']':
			if depth > 0 {
				depth--
			}
			i++
			consumed = true
		case '}':
			if depth == 0 {
				return i
			}
			depth--
			i++
			consumed = true
		default:
			if isClojureWhitespaceOrComma(src[i]) {
				i++
				continue
			}
			next := readClojureTokenEnd(src, i)
			if next <= i {
				i++
			} else {
				i = next
			}
			consumed = true
		}
	}
	return i
}

func isProbableTopLevelMapKey(src string, start int) bool {
	if start < 0 || start >= len(src) || src[start] != ':' {
		return false
	}
	if start > 0 && !isClojureTokenDelimiter(src[start-1]) && !isClojureWhitespaceOrComma(src[start-1]) {
		return false
	}
	next := readClojureTokenEnd(src, start)
	return next > start+1
}

func activeReaderConditionalForm(src string, start int) (int, int, int, bool) {
	if !strings.HasPrefix(src[start:], "#?") {
		return -1, -1, start, false
	}
	i := start + 2
	if i < len(src) && src[i] == '@' {
		i++
	}
	i = skipClojureWhitespaceCommaAndComments(src, i)
	if i >= len(src) || src[i] != '(' {
		return -1, -1, start, false
	}
	i++
	selectedStart := -1
	selectedEnd := -1
	selected := false
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return -1, -1, start, false
		}
		if src[i] == ')' {
			return selectedStart, selectedEnd, i + 1, true
		}
		featureStart := i
		featureEnd, err := readClojureFormEnd(src, i)
		if err != nil || featureEnd <= featureStart {
			return -1, -1, start, false
		}
		active := !selected && readerConditionalFeatureActive(src[featureStart:featureEnd])
		i = skipClojureWhitespaceCommaAndComments(src, featureEnd)
		if i >= len(src) {
			return -1, -1, start, false
		}
		formStart := i
		formEnd, err := readClojureFormEnd(src, i)
		if err != nil || formEnd <= formStart {
			return -1, -1, start, false
		}
		if active {
			selectedStart = formStart
			selectedEnd = formEnd
			selected = true
		}
		i = formEnd
	}
	return -1, -1, start, false
}

func readerConditionalFeatureActive(feature string) bool {
	switch strings.TrimSpace(feature) {
	case ":clj", ":default":
		return true
	default:
		return false
	}
}

func offsetFunctionCodeStrings(codes []functionCodeString, offset int) {
	if offset == 0 {
		return
	}
	for i := range codes {
		codes[i].ByteOffset += offset
	}
}

func topLevelFlowMapStart(src string) (int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	for i < len(src) {
		switch {
		case src[i] == '{':
			return i, nil
		case src[i] == '^':
			metaStart := i
			metaEnd, err := readClojureFormEnd(src, i+1)
			if err != nil {
				return -1, err
			}
			if metaEnd <= i+1 {
				return -1, fmt.Errorf("could not read metadata before top-level map near byte %d", metaStart)
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardStart := i
			discardEnd, err := readClojureFormEnd(src, i+2)
			if err != nil {
				return -1, err
			}
			if discardEnd <= i+2 {
				return -1, fmt.Errorf("could not read discard form before top-level map near byte %d", discardStart)
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		default:
			return -1, fmt.Errorf("top-level flow form is not a map near byte %d", i)
		}
	}
	return -1, nil
}

func extractTopLevelFunctionCodeStrings(src string) ([]functionCodeString, error) {
	i, err := topLevelFlowMapStart(src)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		return nil, nil
	}
	var out []functionCodeString
	i++
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, fmt.Errorf("unterminated top-level map")
		}
		if src[i] == '}' {
			return out, nil
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil {
			return out, err
		}
		if keyEnd <= keyStart {
			return out, fmt.Errorf("could not read top-level key near byte %d", keyStart)
		}
		key := clojureKeywordName(src[keyStart:keyEnd])
		i = skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if i >= len(src) {
			return out, fmt.Errorf("missing value for top-level key near byte %d", keyStart)
		}
		if key == "functions" {
			codes, next, err := extractFunctionsValueCodeStrings(src, i)
			if err != nil {
				return out, err
			}
			out = append(out, codes...)
			i = next
			continue
		}
		next, err := readClojureFormEnd(src, i)
		if err != nil {
			return out, err
		}
		if next <= i {
			return out, fmt.Errorf("could not read value for key %s near byte %d", src[keyStart:keyEnd], i)
		}
		i = next
	}
	return out, fmt.Errorf("unterminated top-level map")
}

func extractFunctionsValueCodeStrings(src string, start int) ([]functionCodeString, int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) {
		return nil, i, fmt.Errorf("missing :functions value")
	}
	if strings.HasPrefix(src[i:], "#?") {
		formStart, _, next, ok := activeReaderConditionalForm(src, i)
		if !ok {
			next, err := readClojureFormEnd(src, i)
			return nil, next, err
		}
		if formStart < 0 {
			return nil, next, nil
		}
		codes, _, err := extractFunctionsValueCodeStrings(src, formStart)
		return codes, next, err
	}
	switch src[i] {
	case '[':
		return extractFunctionVectorCodeStrings(src, i)
	case '{':
		return extractFunctionMapCodeStrings(src, i)
	default:
		next, err := readClojureFormEnd(src, i)
		return nil, next, err
	}
}

func extractFunctionVectorCodeStrings(src string, start int) ([]functionCodeString, int, error) {
	var out []functionCodeString
	i := start + 1
	index := 0
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, i, fmt.Errorf("unterminated :functions vector")
		}
		if src[i] == ']' {
			return out, i + 1, nil
		}
		if strings.HasPrefix(src[i:], "#?") {
			codes, next, ok, err := extractReaderConditionalFunctionEntryCodeStrings(src, i, fmt.Sprintf("[%d]", index))
			if ok {
				if err != nil {
					return out, next, err
				}
				out = append(out, codes...)
				i = next
				index++
				continue
			}
		}
		if src[i] == '{' {
			codes, next, err := extractFunctionEntryCodeStrings(src, i, fmt.Sprintf("[%d]", index))
			if err != nil {
				return out, next, err
			}
			out = append(out, codes...)
			i = next
		} else {
			next, err := readClojureFormEnd(src, i)
			if err != nil {
				return out, next, err
			}
			if next <= i {
				return out, next, fmt.Errorf("could not read :functions entry near byte %d", i)
			}
			i = next
		}
		index++
	}
	return out, i, fmt.Errorf("unterminated :functions vector")
}

func extractReaderConditionalFunctionEntryCodeStrings(src string, start int, fallbackLabel string) ([]functionCodeString, int, bool, error) {
	formStart, _, next, ok := activeReaderConditionalForm(src, start)
	if !ok {
		return nil, start, false, nil
	}
	if formStart < 0 {
		return nil, next, true, nil
	}
	switch src[formStart] {
	case '{':
		codes, _, err := extractFunctionEntryCodeStrings(src, formStart, fallbackLabel)
		return codes, next, true, err
	case '[':
		codes, _, err := extractFunctionVectorCodeStrings(src, formStart)
		return codes, next, true, err
	default:
		return nil, next, true, nil
	}
}

func extractFunctionMapCodeStrings(src string, start int) ([]functionCodeString, int, error) {
	var out []functionCodeString
	i := start + 1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, i, fmt.Errorf("unterminated :functions map")
		}
		if src[i] == '}' {
			return out, i + 1, nil
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil {
			return out, keyEnd, err
		}
		if keyEnd <= keyStart {
			return out, keyEnd, fmt.Errorf("could not read :functions map key near byte %d", keyStart)
		}
		label := functionLabelFromToken(src[keyStart:keyEnd], "")
		i = skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if i >= len(src) {
			return out, i, fmt.Errorf("missing :functions map value")
		}
		if strings.HasPrefix(src[i:], "#?") {
			value, valueStart, next, ok, err := readActiveReaderConditionalStringToken(src, i)
			if err != nil {
				return out, next, err
			}
			if ok {
				if valueStart >= 0 {
					out = append(out, functionCodeString{
						Code:       value,
						Path:       []string{":functions", label, ":code"},
						ByteOffset: valueStart,
					})
				}
				i = next
				continue
			}
		}
		if src[i] == '"' {
			_, value, next, err := readClojureStringToken(src, i)
			if err != nil {
				return out, next, err
			}
			out = append(out, functionCodeString{
				Code:       value,
				Path:       []string{":functions", label, ":code"},
				ByteOffset: i,
			})
			i = next
			continue
		}
		next, err := readClojureFormEnd(src, i)
		if err != nil {
			return out, next, err
		}
		if next <= i {
			return out, next, fmt.Errorf("could not read :functions map value near byte %d", i)
		}
		i = next
	}
	return out, i, fmt.Errorf("unterminated :functions map")
}

func extractFunctionEntryCodeStrings(src string, start int, fallbackLabel string) ([]functionCodeString, int, error) {
	var local []functionCodeString
	label := fallbackLabel
	i := start + 1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return local, i, fmt.Errorf("unterminated function map")
		}
		if src[i] == '}' {
			for idx := range local {
				local[idx].Path = []string{":functions", label, ":code"}
			}
			return local, i + 1, nil
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil {
			return local, keyEnd, err
		}
		if keyEnd <= keyStart {
			return local, keyEnd, fmt.Errorf("could not read function map key near byte %d", keyStart)
		}
		key := clojureKeywordName(src[keyStart:keyEnd])
		i = skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if i >= len(src) {
			return local, i, fmt.Errorf("missing function map value")
		}
		switch key {
		case "id", "name":
			label = readFunctionLabel(src, i, fallbackLabel)
			next, err := readClojureFormEnd(src, i)
			if err != nil {
				return local, next, err
			}
			if next <= i {
				return local, next, fmt.Errorf("could not read function label near byte %d", i)
			}
			i = next
		case "code":
			if strings.HasPrefix(src[i:], "#?") {
				value, valueStart, next, ok, err := readActiveReaderConditionalStringToken(src, i)
				if err != nil {
					return local, next, err
				}
				if ok {
					if valueStart >= 0 {
						local = append(local, functionCodeString{
							Code:       value,
							ByteOffset: valueStart,
						})
					}
					i = next
				} else {
					i++
				}
			} else if src[i] == '"' {
				_, value, next, err := readClojureStringToken(src, i)
				if err != nil {
					return local, next, err
				}
				local = append(local, functionCodeString{
					Code:       value,
					ByteOffset: i,
				})
				i = next
			} else {
				next, err := readClojureFormEnd(src, i)
				if err != nil {
					return local, next, err
				}
				if next <= i {
					return local, next, fmt.Errorf("could not read function :code near byte %d", i)
				}
				i = next
			}
		default:
			next, err := readClojureFormEnd(src, i)
			if err != nil {
				return local, next, err
			}
			if next <= i {
				return local, next, fmt.Errorf("could not read function map value near byte %d", i)
			}
			i = next
		}
	}
	return local, i, fmt.Errorf("unterminated function map")
}

func readActiveReaderConditionalStringToken(src string, start int) (string, int, int, bool, error) {
	formStart, _, next, ok := activeReaderConditionalForm(src, start)
	if !ok {
		return "", -1, start, false, nil
	}
	if formStart < 0 {
		return "", -1, next, true, nil
	}
	if src[formStart] != '"' {
		return "", -1, next, true, nil
	}
	_, value, _, err := readClojureStringToken(src, formStart)
	if err != nil {
		return "", formStart, next, true, err
	}
	return value, formStart, next, true, nil
}

func clojureKeywordName(token string) string {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, ":") {
		return ""
	}
	token = strings.TrimPrefix(token, ":")
	if slash := strings.LastIndex(token, "/"); slash >= 0 && slash+1 < len(token) {
		token = token[slash+1:]
	}
	return token
}

func functionLabelFromToken(token string, fallback string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return fallback
	}
	if strings.HasPrefix(token, ":") {
		return token
	}
	return strings.Trim(token, `"`)
}

func readFunctionLabel(src string, start int, fallback string) string {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) {
		return fallback
	}
	if src[i] == '"' {
		_, value, _, err := readClojureStringToken(src, i)
		if err == nil && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	next, err := readClojureFormEnd(src, i)
	if err != nil || next <= i {
		return fallback
	}
	return functionLabelFromToken(src[i:next], fallback)
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
