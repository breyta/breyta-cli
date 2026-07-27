package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	flowLintFlowQuoteRe      = regexp.MustCompile(`:flow\s*$`)
	flowLintInvocationTypes  = map[string]bool{
		"string": true, "text": true, "number": true, "email": true, "password": true,
		"textarea": true, "boolean": true, "checkbox": true, "select": true,
		"date": true, "time": true, "datetime": true, "json": true, "file": true,
		"blob": true, "blob-ref": true, "resource": true, "secret": true,
	}
)

type unsupportedFlowFormRule struct {
	reason string
	hint   string
}

var flowLintUnsupportedFlowForms = map[string]unsupportedFlowFormRule{
	"->>":     {reason: "Visual renderer only supports -> (thread-first) for Data Pipeline view.", hint: "Rewrite with -> or explicit let bindings."},
	"as->":    {reason: "Visual renderer only supports -> (thread-first).", hint: "Rewrite with explicit let bindings for named intermediates."},
	"some->":  {reason: "Visual renderer cannot display nil-short-circuiting.", hint: "Use explicit if/when branching before the pipeline."},
	"some->>": {reason: "Visual renderer cannot display nil-short-circuiting.", hint: "Use explicit if/when branching before the pipeline."},
	"cond->":  {reason: "Visual renderer cannot display conditional threading.", hint: "Use explicit conditionals and let bindings."},
	"cond->>": {reason: "Visual renderer cannot display conditional threading.", hint: "Use explicit conditionals and let bindings."},
}

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
			diagnostics := localFlowLintPreExpansionDiagnostics(file, flowLiteral)
			expandedLiteral := flowLiteral
			if !lintHasErrors(diagnostics) {
				if expanded, err := expandFlowSourceIncludes(file, flowLiteral); err != nil {
					diagnostics = append(diagnostics, lintDiagnostic("error", "flow_include_invalid", []string{":flow"}, err.Error(), "Fix #flow/include paths before linting or pushing.", "local"))
				} else {
					expandedLiteral = expanded
					diagnostics = append(diagnostics, localFlowLintDiagnostics(file, expandedLiteral, expandedLiteral != flowLiteral)...)
					diagnostics = append(diagnostics, localUnsupportedFlowFormDiagnostics(expandedLiteral)...)
					diagnostics = append(diagnostics, localAuthoringShapeDiagnostics(expandedLiteral, pulledLegacyFunctionInputSteps(flowLiteral))...)
					diagnostics = append(diagnostics, localFunctionCodeStringDiagnostics(expandedLiteral)...)
				}
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

func localFlowLintPreExpansionDiagnostics(file string, flowLiteral string) []flowLintDiagnostic {
	if err := parenrepair.Check(flowLiteral); err != nil {
		code := "clojure_syntax_invalid"
		hint := "Fix malformed Clojure/EDN before pushing."
		if errors.Is(err, parenrepair.ErrUnbalancedDelimiters) {
			code = "clojure_delimiters_invalid"
			hint = "Run: breyta flows paren-repair --write --file " + file
		}
		return []flowLintDiagnostic{lintDiagnostic("error", code, []string{":flow"}, err.Error(), hint, "local")}
	}
	return nil
}

func localFlowLintDiagnostics(file string, flowLiteral string, expandedFromInclude bool) []flowLintDiagnostic {
	var diagnostics []flowLintDiagnostic
	if err := parenrepair.Check(flowLiteral); err != nil {
		code := "clojure_syntax_invalid"
		hint := "Fix malformed Clojure/EDN before pushing."
		if errors.Is(err, parenrepair.ErrUnbalancedDelimiters) {
			code = "clojure_delimiters_invalid"
			if expandedFromInclude {
				hint = "Fix the unbalanced included source before linting or pushing."
			} else {
				hint = "Run: breyta flows paren-repair --write --file " + file
			}
		}
		diagnostics = append(diagnostics, lintDiagnostic("error", code, []string{":flow"}, err.Error(), hint, "local"))
		return diagnostics
	}
	if readerEvalDiagnostics := localReaderEvalDiagnostics(flowLiteral); lintHasErrors(readerEvalDiagnostics) {
		diagnostics = append(diagnostics, readerEvalDiagnostics...)
		return diagnostics
	}
	if err := validateLocalClojureReaderShape(flowLiteral); err != nil {
		diagnostics = append(diagnostics, lintDiagnostic(
			"error",
			"clojure_reader_invalid",
			[]string{":flow"},
			"Flow source is not readable Clojure/EDN: "+err.Error(),
			"Fix malformed source before pushing. For delimiter repairs, try `breyta flows paren-repair --write --file "+file+"`.",
			"local",
		))
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

func localUnsupportedFlowFormDiagnostics(flowLiteral string) []flowLintDiagnostic {
	flowSource, baseOffset, ok := topLevelFlowValueSource(flowLiteral)
	if !ok {
		return nil
	}
	flowSource, readerOffset := unwrapTopLevelReaderConditionalFlowSource(flowSource)
	baseOffset += readerOffset
	flowSource, unwrappedOffset := unwrapTopLevelQuotedFlowSource(flowSource)
	baseOffset += unwrappedOffset
	var diagnostics []flowLintDiagnostic
	for _, match := range unsupportedFlowFormMatches(flowSource, baseOffset) {
		rule := flowLintUnsupportedFlowForms[match.symbol]
		diag := lintDiagnostic(
			"error",
			"unsupported_visual_flow_form",
			[]string{":flow"},
			fmt.Sprintf("Flow source uses %s. %s", match.symbol, rule.reason),
			rule.hint,
			"local",
		)
		diag["form"] = match.symbol
		diag["byteOffset"] = match.offset
		diagnostics = append(diagnostics, diag)
	}
	return diagnostics
}

type unsupportedFlowFormMatch struct {
	symbol string
	offset int
}

func unsupportedFlowFormMatches(src string, baseOffset int) []unsupportedFlowFormMatch {
	var matches []unsupportedFlowFormMatch
	for i := 0; i < len(src); {
		if strings.HasPrefix(src[i:], `#"`) {
			next, err := readClojureRegexTokenEnd(src, i+1)
			if err != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		}
		if strings.HasPrefix(src[i:], "#?") {
			formStart, formEnd, next, ok := activeReaderConditionalForm(src, i)
			if ok {
				if formStart >= 0 {
					matches = append(matches, unsupportedFlowFormMatches(src[formStart:formEnd], baseOffset+formStart)...)
				}
				if next <= i {
					i++
				} else {
					i = next
				}
				continue
			}
		}
		if strings.HasPrefix(src[i:], "#_") {
			next, err := readClojureFormEnd(src, i)
			if err != nil || next <= i {
				i++
			} else {
				i = next
			}
			continue
		}
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
		case '\'', '`':
			next, err := readClojureFormEnd(src, i+1)
			if err != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		case '(':
			j := skipClojureWhitespaceCommaAndComments(src, i+1)
			tokenEnd := readClojureTokenEnd(src, j)
			if tokenEnd > j {
				symbol := src[j:tokenEnd]
				if _, ok := flowLintUnsupportedFlowForms[symbol]; ok {
					matches = append(matches, unsupportedFlowFormMatch{symbol: symbol, offset: baseOffset + j})
				}
			}
		}
		i++
	}
	return matches
}

func unwrapTopLevelQuotedFlowSource(src string) (string, int) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	if i < len(src) && (src[i] == '\'' || src[i] == '`') {
		return src[i+1:], i + 1
	}
	if i < len(src) && src[i] == '(' {
		elements, _, err := parseClojureListElements(src, i)
		if err == nil && len(elements) >= 2 {
			head := clojureFormToken(src, elements[0])
			if head == "quote" || head == "clojure.core/quote" {
				return src[elements[1].Start:elements[1].End], elements[1].Start
			}
		}
	}
	return src, 0
}

func unwrapTopLevelReaderConditionalFlowSource(src string) (string, int) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	if i >= len(src) || !strings.HasPrefix(src[i:], "#?") {
		return src, 0
	}
	formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
	if !ok || formStart < 0 {
		return src, 0
	}
	return src[formStart:formEnd], formStart
}

func localReaderEvalDiagnostics(flowLiteral string) []flowLintDiagnostic {
	for i := 0; i < len(flowLiteral); {
		if strings.HasPrefix(flowLiteral[i:], `#"`) {
			next, err := readClojureRegexTokenEnd(flowLiteral, i+1)
			if err != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		}
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

func validateLocalClojureReaderShape(src string) error {
	start := skipClojureWhitespaceCommaAndComments(src, 0)
	if start >= len(src) {
		return fmt.Errorf("expected Clojure form")
	}
	end, err := validateLocalClojureReaderForm(src, start)
	if err != nil {
		return err
	}
	next := skipClojureWhitespaceCommaAndComments(src, end)
	if next < len(src) {
		return fmt.Errorf("unexpected trailing form near byte %d", next)
	}
	return nil
}

func validateLocalClojureReaderForm(src string, start int) (int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	for strings.HasPrefix(src[i:], "#_") {
		discardEnd, err := validateLocalClojureReaderForm(src, i+2)
		if err != nil {
			return 0, fmt.Errorf("parse discarded form near byte %d: %w", i, err)
		}
		i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
	}
	if i >= len(src) {
		return 0, fmt.Errorf("expected Clojure form")
	}

	switch src[i] {
	case '\\':
		return readClojureCharLiteralEnd(src, i)
	case '"':
		_, _, next, err := readClojureStringToken(src, i)
		return next, err
	case '(':
		return validateLocalClojureDelimitedReaderForms(src, i, ')', false)
	case '[':
		return validateLocalClojureDelimitedReaderForms(src, i, ']', false)
	case '{':
		return validateLocalClojureDelimitedReaderForms(src, i, '}', true)
	case '\'', '`', '@':
		return validateLocalClojureReaderForm(src, i+1)
	case '~':
		if i+1 < len(src) && src[i+1] == '@' {
			return validateLocalClojureReaderForm(src, i+2)
		}
		return validateLocalClojureReaderForm(src, i+1)
	case '^':
		metaEnd, err := validateLocalClojureReaderForm(src, i+1)
		if err != nil {
			return 0, err
		}
		return validateLocalClojureReaderForm(src, metaEnd)
	case '#':
		if i+1 >= len(src) {
			return 0, fmt.Errorf("incomplete reader macro near byte %d", i)
		}
		switch src[i+1] {
		case '\'':
			return validateLocalClojureReaderForm(src, i+2)
		case '^':
			metaEnd, err := validateLocalClojureReaderForm(src, i+2)
			if err != nil {
				return 0, err
			}
			return validateLocalClojureReaderForm(src, metaEnd)
		case '#':
			next := readClojureTokenEnd(src, i)
			switch src[i:next] {
			case "##Inf", "##-Inf", "##NaN":
				return next, nil
			default:
				return 0, fmt.Errorf("unsupported symbolic value near byte %d", i)
			}
		case '=':
			return 0, fmt.Errorf("reader eval is not supported near byte %d", i)
		case '{':
			return validateLocalClojureDelimitedReaderForms(src, i+1, '}', false)
		case '(':
			return validateLocalClojureDelimitedReaderForms(src, i+1, ')', false)
		case '"':
			return readClojureRegexTokenEnd(src, i+1)
		case '?':
			formStart, formEnd, next, ok := activeReaderConditionalForm(src, i)
			if !ok {
				return 0, fmt.Errorf("invalid reader conditional near byte %d", i)
			}
			if formStart >= 0 {
				parsedEnd, err := validateLocalClojureReaderForm(src, formStart)
				if err != nil {
					return 0, err
				}
				if parsedEnd != formEnd {
					return 0, fmt.Errorf("invalid reader conditional form near byte %d", formStart)
				}
			}
			return next, nil
		default:
			tagEnd := readClojureTokenEnd(src, i+1)
			if tagEnd == i+1 {
				return 0, fmt.Errorf("unsupported reader macro near byte %d", i)
			}
			return validateLocalClojureReaderForm(src, tagEnd)
		}
	default:
		next := readClojureTokenEnd(src, i)
		if next <= i {
			return 0, fmt.Errorf("could not read form near byte %d", i)
		}
		return next, nil
	}
}

func validateLocalClojureDelimitedReaderForms(src string, start int, closeCh byte, requireEvenForms bool) (int, error) {
	i := start + 1
	formCount := 0
	lastFormStart := -1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return 0, fmt.Errorf("unterminated collection near byte %d", start)
		}
		if src[i] == closeCh {
			if requireEvenForms && formCount%2 != 0 {
				return 0, fmt.Errorf("missing map value for form near byte %d", lastFormStart)
			}
			return i + 1, nil
		}
		if strings.HasPrefix(src[i:], "#_") {
			next, err := validateLocalClojureReaderForm(src, i+2)
			if err != nil {
				return 0, fmt.Errorf("parse discarded form near byte %d: %w", i, err)
			}
			i = next
			continue
		}
		lastFormStart = i
		next, err := validateLocalClojureReaderForm(src, i)
		if err != nil {
			return 0, err
		}
		formsAdded := 1
		if strings.HasPrefix(src[i:], "#?@") {
			formsAdded, err = readerConditionalSpliceFormCount(src, i)
			if err != nil {
				return 0, err
			}
		}
		formCount += formsAdded
		i = next
	}
	return 0, fmt.Errorf("unterminated collection near byte %d", start)
}

func readerConditionalSpliceFormCount(src string, start int) (int, error) {
	formStart, formEnd, _, ok := activeReaderConditionalForm(src, start)
	if !ok {
		return 0, fmt.Errorf("invalid reader conditional splice near byte %d", start)
	}
	if formStart < 0 {
		return 0, nil
	}
	if formStart >= formEnd || formStart >= len(src) {
		return 0, fmt.Errorf("invalid reader conditional splice form near byte %d", start)
	}
	closeCh := byte(0)
	switch src[formStart] {
	case '(':
		closeCh = ')'
	case '[':
		closeCh = ']'
	case '{':
		closeCh = '}'
	default:
		return 1, nil
	}

	count := 0
	for i := formStart + 1; i < formEnd; {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= formEnd {
			break
		}
		if src[i] == closeCh {
			return count, nil
		}
		if strings.HasPrefix(src[i:], "#_") {
			next, err := validateLocalClojureReaderForm(src, i+2)
			if err != nil {
				return 0, err
			}
			i = next
			continue
		}
		next, err := validateLocalClojureReaderForm(src, i)
		if err != nil {
			return 0, err
		}
		count++
		i = next
	}
	return count, nil
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

func topLevelFlowValueSource(src string) (string, int, bool) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	for i < len(src) {
		switch {
		case src[i] == '{':
			return topLevelMapValueSource(src, i, "flow")
		case src[i] == '^':
			metaEnd, err := readClojureFormEnd(src, i+1)
			if err != nil || metaEnd <= i+1 {
				return "", 0, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardEnd, err := readClojureFormEnd(src, i+2)
			if err != nil || discardEnd <= i+2 {
				return "", 0, false
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		case strings.HasPrefix(src[i:], "#?"):
			formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
			if !ok || formStart < 0 {
				return "", 0, false
			}
			flowSource, offset, ok := topLevelFlowValueSource(src[formStart:formEnd])
			return flowSource, formStart + offset, ok
		default:
			return "", 0, false
		}
	}
	return "", 0, false
}

func topLevelMapValueSource(src string, start int, targetKey string) (string, int, bool) {
	i := start + 1
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) || src[i] == '}' {
			return "", 0, false
		}
		keyStart := i
		keyEnd, err := readClojureFormEnd(src, i)
		if err != nil || keyEnd <= keyStart {
			return "", 0, false
		}
		key := clojureKeywordName(src[keyStart:keyEnd])
		valueStart := skipClojureWhitespaceCommaAndComments(src, keyEnd)
		if valueStart >= len(src) {
			return "", 0, false
		}
		valueEnd, err := readClojureFormEnd(src, valueStart)
		if err != nil || valueEnd <= valueStart {
			return "", 0, false
		}
		if key == targetKey {
			return src[valueStart:valueEnd], valueStart, true
		}
		i = valueEnd
	}
	return "", 0, false
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
	Start     int
	End       int
	FormStart int
	FormEnd   int
}

type clojureMapEntry struct {
	KeyToken   string
	KeyName    string
	KeyStart   int
	KeyEnd     int
	ValueStart int
	ValueEnd   int
}

func pulledLegacyFunctionInputSteps(flowLiteral string) map[string]bool {
	steps := map[string]bool{}
	pulledSource := false
	for _, line := range strings.Split(flowLiteral, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ";") {
			break
		}
		if trimmed == pulledFlowSourceMarker {
			pulledSource = true
			continue
		}
		if !strings.HasPrefix(trimmed, pulledLegacyFunctionInputMarker) {
			continue
		}
		step, err := strconv.Unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, pulledLegacyFunctionInputMarker)))
		if err == nil && step != "" {
			steps[step] = true
		}
	}
	if !pulledSource {
		return map[string]bool{}
	}
	return steps
}

func localAuthoringShapeDiagnostics(flowLiteral string, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
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
	stepsEntry := byKey["steps"]
	diagnostics = append(diagnostics, localPackagedStepReferenceDiagnostics(flowLiteral, stepsEntry, byKey["agents"])...)
	diagnostics = append(diagnostics, localFunctionStepShapeDiagnostics(flowLiteral, localFlowHasTag(flowLiteral, byKey["tags"], "n8n-import"), pulledLegacyInputSteps)...)
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
		for strings.HasPrefix(src[i:], "#_") {
			discardEnd, err := readClojureDiscardedFormEnd(src, i+2)
			if err != nil || discardEnd <= i+2 {
				if err == nil {
					err = fmt.Errorf("could not read discarded map form near byte %d", i)
				}
				return entries, discardEnd, err
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		}
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
		for strings.HasPrefix(src[valueStart:], "#_") {
			discardEnd, err := readClojureDiscardedFormEnd(src, valueStart+2)
			if err != nil || discardEnd <= valueStart+2 {
				if err == nil {
					err = fmt.Errorf("could not read discarded map value near byte %d", valueStart)
				}
				return entries, discardEnd, err
			}
			valueStart = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		}
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
		activeStart, activeEnd, formEnd, hasActive, err := clojureActiveFormSpan(src, i)
		if err != nil {
			return out, i, err
		}
		if hasActive {
			if strings.HasPrefix(src[i:], "#?@") {
				var spliced []clojureFormSpan
				var branchEnd int
				var branchErr error
				if activeStart >= activeEnd || activeStart >= len(src) {
					return out, i, fmt.Errorf("splicing reader conditional selected an empty branch near byte %d", i)
				}
				switch src[activeStart] {
				case '[':
					spliced, branchEnd, branchErr = parseClojureVectorElements(src, activeStart)
				case '(':
					spliced, branchEnd, branchErr = parseClojureListElements(src, activeStart)
				default:
					return out, i, fmt.Errorf("splicing reader conditional must select a vector or list near byte %d", i)
				}
				if branchErr != nil {
					return out, i, branchErr
				}
				if branchEnd != activeEnd {
					return out, i, fmt.Errorf("splicing reader conditional branch did not consume its vector near byte %d", i)
				}
				out = append(out, spliced...)
			} else {
				out = append(out, clojureFormSpan{
					Start:     activeStart,
					End:       activeEnd,
					FormStart: i,
					FormEnd:   formEnd,
				})
			}
		} else if formEnd == i && i < len(src) && src[i] == ']' {
			// A discarded form may be the only form left before the vector
			// closes. The discard prefix has already been consumed, so treat
			// the closing delimiter as the collection boundary rather than
			// trying to parse it as another element.
			return out, i + 1, nil
		}
		if formEnd <= i {
			return out, i, fmt.Errorf("could not advance past vector element near byte %d", i)
		}
		i = formEnd
	}
	return out, i, fmt.Errorf("unterminated vector")
}

// clojureActiveFormSpan returns the active form's span and the end of the
// complete reader form that occupied the vector slot. The spans used for
// editing intentionally exclude metadata/discard prefixes, while the cursor
// advances over reader conditionals as a whole.
func clojureActiveFormSpan(src string, start int) (activeStart, activeEnd, formEnd int, hasActive bool, err error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	for i < len(src) {
		if src[i] == ')' || src[i] == ']' || src[i] == '}' {
			return -1, -1, i, false, nil
		}
		switch {
		case strings.HasPrefix(src[i:], "#?"):
			branchStart, branchEnd, next, ok := activeReaderConditionalForm(src, i)
			if !ok {
				return i, i, i, false, fmt.Errorf("could not read reader conditional near byte %d", i)
			}
			if branchStart < 0 {
				return -1, -1, next, false, nil
			}
			activeStart, activeEnd, _, hasActive, err := clojureActiveFormSpan(src, branchStart)
			if err != nil {
				return i, i, i, false, err
			}
			if !hasActive {
				return -1, -1, next, false, nil
			}
			if activeEnd > branchEnd {
				return i, i, i, false, fmt.Errorf("reader conditional branch extends past its form near byte %d", branchStart)
			}
			return activeStart, activeEnd, next, true, nil
		case src[i] == '^' || strings.HasPrefix(src[i:], "#^"):
			metaValueStart := i + 1
			if src[i] == '#' {
				metaValueStart++
			}
			metaEnd, metaErr := readClojureFormEnd(src, metaValueStart)
			if metaErr != nil || metaEnd <= metaValueStart {
				if metaErr == nil {
					metaErr = fmt.Errorf("could not read metadata near byte %d", i)
				}
				return i, i, i, false, metaErr
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardEnd, discardErr := readClojureFormEnd(src, i+2)
			if discardErr != nil || discardEnd <= i+2 {
				if discardErr == nil {
					discardErr = fmt.Errorf("could not read discarded form near byte %d", i)
				}
				return i, i, i, false, discardErr
			}
			i = skipClojureWhitespaceCommaAndComments(src, discardEnd)
		default:
			end, formErr := readClojureFormEnd(src, i)
			if formErr != nil || end <= i {
				if formErr == nil {
					formErr = fmt.Errorf("could not read vector element near byte %d", i)
				}
				return i, i, i, false, formErr
			}
			return i, end, end, true, nil
		}
	}
	return i, i, i, false, fmt.Errorf("could not locate active vector element near byte %d", start)
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

func parseClojureSetElements(src string, start int) ([]clojureFormSpan, int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i+1 >= len(src) || src[i] != '#' || src[i+1] != '{' {
		return nil, i, fmt.Errorf("expected set near byte %d", start)
	}
	i += 2
	var out []clojureFormSpan
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return out, i, fmt.Errorf("unterminated set")
		}
		if src[i] == '}' {
			return out, i + 1, nil
		}
		end, err := readClojureFormEnd(src, i)
		if err != nil || end <= i {
			if err == nil {
				err = fmt.Errorf("could not read set element near byte %d", i)
			}
			return out, end, err
		}
		out = append(out, clojureFormSpan{Start: i, End: end})
		i = end
	}
	return out, i, fmt.Errorf("unterminated set")
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
		case src[i] == '^' || strings.HasPrefix(src[i:], "#^"):
			metaValueStart := i + 1
			if src[i] == '#' {
				metaValueStart++
			}
			metaEnd, err := readClojureFormEnd(src, metaValueStart)
			if err != nil || metaEnd <= metaValueStart {
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

type localFlowStepReference struct {
	StepID     string
	ByteOffset int
}

func localQualifiedStepIDFromForm(src string, span clojureFormSpan) (string, bool) {
	i, ok := clojureActiveFormStart(src, span.Start)
	if !ok || i >= span.End || i >= len(src) || src[i] != ':' {
		return "", false
	}
	end, err := readClojureFormEnd(src, i)
	if err != nil || end > span.End || end <= i {
		return "", false
	}
	stepID := strings.TrimPrefix(strings.TrimSpace(src[i:end]), ":")
	return stepID, localStepIDValid(stepID)
}

func localFlowStepReferences(flowLiteral string) ([]localFlowStepReference, error) {
	flowSource, baseOffset, ok := topLevelFlowValueSource(flowLiteral)
	if !ok {
		return nil, errors.New("top-level :flow value could not be located")
	}
	flowSource, readerOffset := unwrapTopLevelReaderConditionalFlowSource(flowSource)
	baseOffset += readerOffset
	flowSource, quotedOffset := unwrapTopLevelQuotedFlowSource(flowSource)
	baseOffset += quotedOffset
	return localFlowStepReferencesInRange(flowSource, 0, len(flowSource), baseOffset)
}

func localFlowStepReferencesInRange(src string, start, end, baseOffset int) ([]localFlowStepReference, error) {
	var references []localFlowStepReference
	for i := skipClojureWhitespaceCommaAndComments(src, start); i < end; {
		formEnd, err := readClojureFormEnd(src, i)
		if err != nil {
			return references, err
		}
		if formEnd <= i || formEnd > end {
			return references, fmt.Errorf("could not read flow form near byte %d", i)
		}
		found, err := localFlowStepReferencesForForm(src, clojureFormSpan{Start: i, End: formEnd}, baseOffset)
		if err != nil {
			return references, err
		}
		references = append(references, found...)
		i = skipClojureWhitespaceCommaAndComments(src, formEnd)
	}
	return references, nil
}

func localFlowStepReferencesForForm(src string, span clojureFormSpan, baseOffset int) ([]localFlowStepReference, error) {
	return localFlowStepReferencesForFormAtDepth(src, span, baseOffset, 0)
}

func localFlowStepReferencesForFormAtDepth(src string, span clojureFormSpan, baseOffset, syntaxQuoteDepth int) ([]localFlowStepReference, error) {
	i := skipClojureWhitespaceCommaAndComments(src, span.Start)
	if i >= span.End || i >= len(src) {
		return nil, nil
	}
	if strings.HasPrefix(src[i:], "#_") {
		return nil, nil
	}
	if strings.HasPrefix(src[i:], "#?") {
		formStart, formEnd, _, ok := activeReaderConditionalForm(src, i)
		if !ok {
			return nil, fmt.Errorf("could not read reader conditional near byte %d", i)
		}
		if formStart < 0 {
			return nil, nil
		}
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: formStart, End: formEnd}, baseOffset, syntaxQuoteDepth)
	}
	if src[i] == '^' {
		metaEnd, err := readClojureFormEnd(src, i+1)
		if err != nil || metaEnd >= span.End {
			return nil, err
		}
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: metaEnd, End: span.End}, baseOffset, syntaxQuoteDepth)
	}
	switch src[i] {
	case '\'':
		return nil, nil
	case '`':
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: i + 1, End: span.End}, baseOffset, syntaxQuoteDepth+1)
	case '@':
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: i + 1, End: span.End}, baseOffset, syntaxQuoteDepth)
	case '~':
		if syntaxQuoteDepth <= 0 {
			return nil, nil
		}
		next := i + 1
		if next < span.End && src[next] == '@' {
			next++
		}
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: next, End: span.End}, baseOffset, syntaxQuoteDepth-1)
	case '"':
		return nil, nil
	case '#':
		if strings.HasPrefix(src[i:], "#(") {
			return localFlowStepReferencesInListAtDepth(src, i+1, baseOffset, syntaxQuoteDepth)
		}
		if strings.HasPrefix(src[i:], "#{") {
			elements, _, err := parseClojureSetElements(src, i)
			if err != nil {
				return nil, err
			}
			var references []localFlowStepReference
			for _, element := range elements {
				found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth)
				if err != nil {
					return references, err
				}
				references = append(references, found...)
			}
			return references, nil
		}
		return nil, nil
	case '(':
		return localFlowStepReferencesInListAtDepth(src, i, baseOffset, syntaxQuoteDepth)
	case '[':
		elements, _, err := parseClojureVectorElements(src, i)
		if err != nil {
			return nil, err
		}
		var references []localFlowStepReference
		for _, element := range elements {
			found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth)
			if err != nil {
				return references, err
			}
			references = append(references, found...)
		}
		return references, nil
	case '{':
		entries, _, err := parseClojureMapEntries(src, i)
		if err != nil {
			return nil, err
		}
		var references []localFlowStepReference
		for _, entry := range entries {
			for _, element := range []clojureFormSpan{{Start: entry.KeyStart, End: entry.KeyEnd}, {Start: entry.ValueStart, End: entry.ValueEnd}} {
				found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth)
				if err != nil {
					return references, err
				}
				references = append(references, found...)
			}
		}
		return references, nil
	default:
		return nil, nil
	}
}

func localFlowStepReferencesInListAtDepth(src string, listStart, baseOffset, syntaxQuoteDepth int) ([]localFlowStepReference, error) {
	elements, _, err := parseClojureListElements(src, listStart)
	if err != nil {
		return nil, err
	}
	if len(elements) >= 2 {
		head := clojureFormToken(src, elements[0])
		if head == "quote" || head == "clojure.core/quote" {
			return nil, nil
		}
	}
	var references []localFlowStepReference
	if syntaxQuoteDepth == 0 && len(elements) >= 2 && clojureFormToken(src, elements[0]) == "flow/step" {
		if stepID, ok := localQualifiedStepIDFromForm(src, elements[1]); ok {
			references = append(references, localFlowStepReference{StepID: stepID, ByteOffset: baseOffset + elements[1].Start})
		}
	}
	for _, element := range elements {
		found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth)
		if err != nil {
			return references, err
		}
		references = append(references, found...)
	}
	return references, nil
}

func localPackagedStepReferenceDiagnostics(src string, stepsEntry, agentsEntry clojureMapEntry) []flowLintDiagnostic {
	var spans []clojureFormSpan
	if stepsEntry.ValueEnd > stepsEntry.ValueStart {
		var err error
		spans, err = localFlowStepVector(src, stepsEntry)
		if err != nil {
			return []flowLintDiagnostic{lintDiagnostic(
				"warning",
				"step_reference_scan_incomplete",
				[]string{":steps"},
				fmt.Sprintf("Local lint could not scan packaged steps: %v", err),
				"Use a top-level :steps vector or nil, then run `breyta flows lint --server` for canonical validation.",
				"local",
			)}
		}
	}
	declared := map[string]bool{}
	declaredPackaged := map[string]bool{}
	var diagnostics []flowLintDiagnostic
	for _, span := range spans {
		stepID, idErr := localStepIDFromMap(src, span)
		if idErr != nil {
			diagnostics = append(diagnostics, lintDiagnostic(
				"warning",
				"step_reference_scan_incomplete",
				[]string{":steps"},
				fmt.Sprintf("Local lint could not read one packaged step: %v", idErr),
				"Fix the packaged step map or run `breyta flows lint --server` for canonical validation.",
				"local",
			))
			continue
		}
		normalizedStepID := strings.TrimPrefix(strings.TrimSpace(stepID), ":")
		declared[normalizedStepID] = true
		declaredPackaged[normalizedStepID] = true
		if entries, _, parseErr := parseClojureMapEntries(src, span.Start); parseErr == nil {
			defaults, hasDefaults := mapEntryByKey(entries, "defaults")
			prepare, hasPrepare := mapEntryByKey(entries, "prepare")
			hasDefaults = hasDefaults && !clojureFormIsNil(src, defaults.ValueStart)
			hasPrepare = hasPrepare && !clojureFormIsNil(src, prepare.ValueStart)
			if !hasDefaults && !hasPrepare {
				diagnostics = append(diagnostics, lintDiagnostic(
					"warning",
					"packaged_step_missing_executable_config",
					[]string{":steps", ":" + normalizedStepID},
					fmt.Sprintf("Packaged step :%s has no :defaults or :prepare executable configuration.", normalizedStepID),
					"Add the wrapped step's required config under :defaults, or use :prepare to build it from invocation input.",
					"local",
				))
			}
		}
	}
	for _, agentID := range localDeclaredQualifiedIDs(src, agentsEntry) {
		declared[agentID] = true
	}
	references, scanErr := localFlowStepReferences(src)
	if scanErr != nil {
		return append(diagnostics, lintDiagnostic(
			"warning",
			"step_reference_scan_incomplete",
			[]string{":flow"},
			fmt.Sprintf("Local lint could not scan packaged step references: %v", scanErr),
			"Fix the flow form or run `breyta flows lint --server` for canonical validation.",
			"local",
		))
	}
	seen := map[string]bool{}
	referenced := map[string]bool{}
	for _, reference := range references {
		referenced[reference.StepID] = true
		if declared[reference.StepID] || seen[reference.StepID] {
			continue
		}
		seen[reference.StepID] = true
		diag := lintDiagnostic(
			"error",
			"missing_packaged_step_reference",
			[]string{":flow", ":" + reference.StepID},
			fmt.Sprintf("Flow body references packaged step :%s, but no matching top-level :steps entry exists.", reference.StepID),
			"Add it with `breyta flows steps create ...` or update the flow body to use a declared step.",
			"local",
		)
		diag["byteOffset"] = reference.ByteOffset
		diagnostics = append(diagnostics, diag)
	}
	for stepID := range declaredPackaged {
		if referenced[stepID] || clojureEntryContainsQualifiedID(src, agentsEntry, stepID) {
			continue
		}
		diagnostics = append(diagnostics, lintDiagnostic(
			"warning",
			"unreferenced_packaged_step",
			[]string{":steps", ":" + stepID},
			fmt.Sprintf("Packaged step :%s is defined but never referenced from :flow.", stepID),
			"Invoke it with `flow/step` from :flow, or remove the unused definition.",
			"local",
		))
	}
	return diagnostics
}

func clojureEntryContainsQualifiedID(src string, entry clojureMapEntry, id string) bool {
	if entry.ValueEnd <= entry.ValueStart {
		return false
	}
	value := src[entry.ValueStart:entry.ValueEnd]
	pattern := regexp.MustCompile(`(^|[\s\[\]{}()'"])` + regexp.QuoteMeta(":"+id) + `($|[\s\[\]{}()'"])`)
	return pattern.MatchString(value)
}

func localDeclaredQualifiedIDs(src string, entry clojureMapEntry) []string {
	if entry.ValueEnd <= entry.ValueStart || clojureFormIsNil(src, entry.ValueStart) {
		return nil
	}
	spans, _, err := parseClojureVectorElements(src, entry.ValueStart)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(spans))
	for _, span := range spans {
		if localFlowVectorSpanIsInclude(src, span) {
			continue
		}
		id, idErr := localStepIDFromMap(src, span)
		if idErr != nil {
			continue
		}
		id = strings.TrimPrefix(strings.TrimSpace(id), ":")
		if localStepIDValid(id) {
			ids = append(ids, id)
		}
	}
	return ids
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
	wanted := ":" + strings.TrimPrefix(strings.TrimSpace(key), ":")
	for _, entry := range entries {
		if strings.TrimSpace(entry.KeyToken) == wanted {
			return entry, true
		}
	}
	return clojureMapEntry{}, false
}

func localFlowHasTag(src string, entry clojureMapEntry, tag string) bool {
	if entry.ValueStart <= 0 && entry.ValueEnd <= 0 {
		return false
	}
	if !clojureFormStartsWith(src, entry.ValueStart, '[') {
		return false
	}
	items, _, err := parseClojureVectorElements(src, entry.ValueStart)
	if err != nil {
		return false
	}
	want := ":" + tag
	for _, item := range items {
		if clojureFormToken(src, item) == want {
			return true
		}
	}
	return false
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

func localFunctionStepShapeDiagnostics(src string, allowBareInput bool, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
	return localFunctionStepShapeDiagnosticsInRange(src, 0, len(src), allowBareInput, pulledLegacyInputSteps)
}

func localFunctionStepShapeDiagnosticsInRange(src string, start int, end int, allowBareInput bool, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
	var diagnostics []flowLintDiagnostic
	for i := start; i < end && i < len(src); {
		if strings.HasPrefix(src[i:], `#"`) {
			next, err := readClojureRegexTokenEnd(src, i+1)
			if err != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		}
		if strings.HasPrefix(src[i:], "#_") {
			next, err := readClojureFormEnd(src, i)
			if err != nil || next <= i {
				i++
			} else {
				i = next
			}
			continue
		}
		if strings.HasPrefix(src[i:], "#?") {
			formStart, formEnd, next, ok := activeReaderConditionalForm(src, i)
			if ok {
				if formStart >= 0 && formEnd >= formStart {
					diagnostics = append(diagnostics, localFunctionStepShapeDiagnosticsInRange(src, formStart, formEnd, allowBareInput, pulledLegacyInputSteps)...)
				}
				i = next
				continue
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
			continue
		case ';':
			i = readCommentEnd(src, i)
			continue
		case '\'', '`':
			formStart := skipClojureWhitespaceCommaAndComments(src, i+1)
			if formStart < len(src) && src[formStart] == '(' {
				elements, _, err := parseClojureListElements(src, formStart)
				if err == nil && len(elements) > 0 {
					switch clojureFormToken(src, elements[0]) {
					case "fn", "fn*":
						next, err := readClojureFormEnd(src, i)
						if err == nil && next > i {
							i = next
							continue
						}
					}
				}
			}
		case '(':
			elements, _, err := parseClojureListElements(src, i)
			if err == nil && !clojureListDirectlyQuoted(src, i) {
				diagnostics = append(diagnostics, localFunctionStepDiagnosticsForList(src, elements, i, allowBareInput, pulledLegacyInputSteps)...)
			}
		}
		i++
	}
	return diagnostics
}

func clojureListDirectlyQuoted(src string, listStart int) bool {
	for i := listStart - 1; i >= 0; i-- {
		switch src[i] {
		case ' ', '\t', '\r', '\n', ',':
			continue
		default:
			if src[i] != '\'' {
				return false
			}
			// The flow DSL convention quotes the top-level :flow form, but
			// nested quotes represent literal data and must not be linted as
			// executable calls.
			prefix := src[:i]
			return !flowLintFlowQuoteRe.MatchString(prefix)
		}
	}
	return false
}

func localFunctionStepDiagnosticsForList(src string, elements []clojureFormSpan, listStart int, allowBareInput bool, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
	if len(elements) == 0 || clojureFormToken(src, elements[0]) != "flow/step" {
		return nil
	}
	if len(elements) < 2 {
		return nil
	}
	stepType := clojureFormToken(src, elements[1])
	if len(elements) > 4 && stepType != ":function" && stepType != ":code" {
		stepID := "<missing>"
		if len(elements) >= 3 {
			if id, ok := clojureIdentifierFromForm(src, elements[2].Start); ok {
				stepID = ":" + id
			} else {
				stepID = strings.TrimSpace(src[elements[2].Start:elements[2].End])
			}
		}
		diag := lintDiagnostic(
			"error",
			"flow_step_arity_invalid",
			[]string{":flow", stepID},
			"flow/step expects exactly three arguments: step type, step id, and config map.",
			"Put :on-error, :retry, :timeout, :persist, and related controls inside the single config map.",
			"local",
		)
		diag["byteOffset"] = listStart
		return []flowLintDiagnostic{diag}
	}
	if stepType != ":function" && stepType != ":code" {
		return nil
	}
	stepID := "<missing>"
	stepMarker := ""
	if len(elements) >= 3 {
		stepMarker = strings.TrimSpace(src[elements[2].Start:elements[2].End])
		if id, ok := clojureIdentifierFromForm(src, elements[2].Start); ok {
			stepID = ":" + id
		} else {
			stepID = stepMarker
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
	if input, ok := mapEntryByKey(entries, "input"); ok && hasRef && !allowBareInput && functionStepInputProvablyNonMap(src, input.ValueStart) {
		severity := "error"
		message := "Function step :input must resolve to a map; a vector, string, or set literal never can."
		hint := "Use a map literal like :input {:rows rows}, or a symbol or expression that resolves to a map such as :input input or :input (select-keys input [:id])."
		if pulledLegacyInputSteps[stepID] {
			severity = "warning"
			message = "Referenced function step :input uses a non-map literal value."
			hint = "Pulled legacy source can keep this compatibility shape. For new steps, use a map literal such as :input {:rows rows} or an expression that resolves to a map, then confirm compatibility with server lint."
		}
		diag := lintDiagnostic(
			severity,
			"function_step_input_shape_invalid",
			append(path, ":input"),
			message,
			hint,
			"local",
		)
		diag["byteOffset"] = input.ValueStart
		diagnostics = append(diagnostics, diag)
	}
	return diagnostics
}

// functionStepInputProvablyNonMap reports whether the :input value form at start
// can never resolve to a map at runtime. Unquoted symbols and function/macro-call
// forms are accepted because they may resolve to a map when the server evaluates
// :input at execution time (server FunctionStepParams :input is [:map-of ...]);
// map literals are accepted because they already are maps. Local lint must not
// reject function-step source that the server lint accepts, so only forms that
// are provably not maps are flagged.
func functionStepInputProvablyNonMap(src string, start int) bool {
	return functionStepFormProvablyNonMap(src, start, quoteNone)
}

// functionStepQuoteMode records the quoting context of a value form: no quote, an
// ordinary quote ('X or (quote X)), or a syntax-quote (`X). It matters only for
// unquote (~), which escapes a syntax-quote back to a runtime value but is plain
// (unquote x) list data under an ordinary quote.
type functionStepQuoteMode int

const (
	quoteNone functionStepQuoteMode = iota
	quoteOrdinary
	quoteSyntax
)

// functionStepFormProvablyNonMap classifies the value form at start. Under a quote
// the datum is taken literally, so anything but a map literal is provably not a
// map (a quoted symbol, list, vector, keyword, etc. is data, never a map). When
// unquoted, symbols and call forms stay accepted because they may resolve to a map
// at runtime.
func functionStepFormProvablyNonMap(src string, start int, mode functionStepQuoteMode) bool {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) {
		return false
	}
	switch src[i] {
	case '\'':
		if mode != quoteNone {
			// A quote nested inside another quote is not transparent: 'X becomes
			// literal (quote X) list data, which is never a map.
			return true
		}
		// Ordinary quote: the following form is literal data.
		return functionStepFormProvablyNonMap(src, i+1, quoteOrdinary)
	case '`':
		if mode != quoteNone {
			// A syntax-quote nested inside another quote is literal list data.
			return true
		}
		// Syntax-quote: literal data, but a top-level unquote escapes it.
		return functionStepFormProvablyNonMap(src, i+1, quoteSyntax)
	case '~':
		// Unquote / unquote-splice escapes a syntax-quote back to a runtime value
		// (defer). Under an ordinary quote it is literal (unquote x) list data,
		// never a map. Outside any quote it is invalid, where deferring is safe.
		return mode == quoteOrdinary
	case '@':
		// Unquoted, @x derefs to a runtime value that may be a map (defer). Under
		// any quote, @x is literal (deref x) list data, which is never a map.
		return mode != quoteNone
	case '{':
		// Map literal (quoted or not) — this is a map.
		return false
	case '[', '"', '\\', ':':
		// Vector, string, char, or keyword literal — none can ever be a map.
		return true
	case '#':
		if i+1 >= len(src) {
			return false
		}
		if src[i+1] == '^' {
			// Legacy #^meta reader macro (equivalent to modern ^meta, which
			// clojureActiveFormStart already unwraps): the reader strips the
			// metadata wrapper, so classify the underlying value form.
			metaEnd, err := readClojureFormEnd(src, i+2)
			if err != nil || metaEnd <= i+2 {
				return false
			}
			return functionStepFormProvablyNonMap(src, metaEnd, mode)
		}
		// Non-tagged reader literals have fixed, provably non-map semantics:
		//   #{...} set, #"..." regex, #(...) fn, #'x var, ##Inf/##NaN symbolic.
		// Tagged literals (#inst, #uuid, #my/tag ...) run a data reader that may
		// yield a map, so defer those to runtime validation.
		switch src[i+1] {
		case '{', '"', '(', '#', '\'':
			return true
		default:
			return false
		}
	case '(':
		if mode != quoteNone {
			// A quoted list is literal data, never a map.
			return true
		}
		elements, _, err := parseClojureListElements(src, i)
		if err != nil {
			// Unparseable — defer rather than risk a false positive.
			return false
		}
		if len(elements) == 0 {
			// The empty list () evaluates to itself, not a callable, so it can
			// never be a map.
			return true
		}
		// (quote X) / (clojure.core/quote X) yields the literal datum X.
		if len(elements) >= 2 {
			if head := clojureFormToken(src, elements[0]); head == "quote" || head == "clojure.core/quote" {
				return functionStepFormProvablyNonMap(src, elements[1].Start, quoteOrdinary)
			}
		}
		// Any other call form may resolve to a map.
		return false
	default:
		if mode != quoteNone {
			// A quoted symbol or scalar is literal data, never a map.
			return true
		}
		// A bare token: either a self-evaluating literal (number/nil/boolean,
		// never a map) or a symbol (a runtime value that may resolve to a map).
		// Flag only the literals; accept symbols.
		return tokenIsScalarNonMapLiteral(src, i)
	}
}

// tokenIsScalarNonMapLiteral reports whether the bare token starting at i is a
// numeric, nil, or boolean literal — self-evaluating values that can never be a
// map. Symbols (including sign-prefixed names like -main or +config) return
// false so they stay valid runtime values.
func tokenIsScalarNonMapLiteral(src string, i int) bool {
	end := readClojureTokenEnd(src, i)
	if end <= i {
		return false
	}
	token := src[i:end]
	switch token {
	case "nil", "true", "false":
		return true
	}
	// Clojure number rule: a token is numeric if it starts with a digit, or with
	// a sign/dot immediately followed by a digit.
	c := token[0]
	if c >= '0' && c <= '9' {
		return true
	}
	if (c == '+' || c == '-' || c == '.') && len(token) >= 2 {
		if token[1] >= '0' && token[1] <= '9' {
			return true
		}
		if token[1] == '.' && len(token) >= 3 && token[2] >= '0' && token[2] <= '9' {
			return true
		}
	}
	return false
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
		if strings.HasPrefix(src[i:], `#"`) {
			next, err := readClojureRegexTokenEnd(src, i+1)
			if err != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			consumed = true
			continue
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
		case src[i] == '^' || strings.HasPrefix(src[i:], "#^"):
			metaStart := i
			metaValueStart := i + 1
			if src[i] == '#' {
				metaValueStart++
			}
			metaEnd, err := readClojureFormEnd(src, metaValueStart)
			if err != nil {
				return -1, err
			}
			if metaEnd <= metaValueStart {
				return -1, fmt.Errorf("could not read metadata before top-level map near byte %d", metaStart)
			}
			i = skipClojureWhitespaceCommaAndComments(src, metaEnd)
		case strings.HasPrefix(src[i:], "#_"):
			discardStart := i
			discardEnd, err := readClojureDiscardedFormEnd(src, i+2)
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
