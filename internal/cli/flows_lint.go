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
	flowLintInvocationTypes  = map[string]bool{
		"string": true, "text": true, "number": true, "email": true, "password": true,
		"textarea": true, "boolean": true, "checkbox": true, "select": true,
		"date": true, "time": true, "datetime": true, "json": true, "file": true,
		"blob": true, "blob-ref": true, "resource": true, "secret": true,
	}
)

type unsupportedFlowFormRule struct {
	code   string
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
	// Keep this transform set aligned with the server flow SCI deny list.
	"map":          {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for for step orchestration, or move data transformation into a :function step."},
	"map-indexed":  {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Move indexed data transformation into a :function step."},
	"filter":       {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for with :when for step orchestration, or move data transformation into a :function step."},
	"reduce":       {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Move data aggregation into a :function step."},
	"mapv":         {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for for step orchestration, or move data transformation into a :function step."},
	"filterv":      {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for with :when for step orchestration, or move data transformation into a :function step."},
	"mapcat":       {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Move data transformation into a :function step."},
	"keep":         {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for with :when for step orchestration, or move data transformation into a :function step."},
	"keep-indexed": {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Move indexed data transformation into a :function step."},
	"remove":       {code: "prohibited_orchestration_transform", reason: "Flow orchestration cannot perform data transformations.", hint: "Use for with :when for step orchestration, or move data transformation into a :function step."},
}

var flowLintTransformReferenceHeads = map[string]bool{
	// true means every argument is a callable reference; false means only the
	// first argument is. apply, partial, and complement accept data arguments
	// after their function argument, so inspecting those would create false
	// positives for ordinary values named like prohibited transforms.
	"apply": false, "partial": false, "complement": false,
	"comp": true, "juxt": true, "some-fn": true, "every-pred": true,
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
					diagnostics = append(diagnostics, localAuthoringShapeDiagnostics(expandedLiteral, flowLiteral, pulledLegacyFunctionInputSteps(flowLiteral))...)
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
	for {
		unwrapped := false
		if next, offset := unwrapTopLevelReaderConditionalFlowSource(flowSource); offset > 0 {
			flowSource = next
			baseOffset += offset
			unwrapped = true
		}
		if next, offset := unwrapTopLevelMetadataFlowSource(flowSource); offset > 0 {
			flowSource = next
			baseOffset += offset
			unwrapped = true
		}
		if !unwrapped {
			break
		}
	}
	flowSource, unwrappedOffset := unwrapTopLevelQuotedFlowSource(flowSource)
	baseOffset += unwrappedOffset
	var diagnostics []flowLintDiagnostic
	for _, match := range unsupportedFlowFormMatches(flowSource, baseOffset) {
		rule := flowLintUnsupportedFlowForms[match.rule]
		code := rule.code
		if code == "" {
			code = "unsupported_visual_flow_form"
		}
		diag := lintDiagnostic(
			"error",
			code,
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
	rule   string
	offset int
}

func unsupportedFlowFormRuleKey(symbol string) (string, bool) {
	symbol = strings.TrimPrefix(symbol, "#'")
	if strings.HasPrefix(symbol, ":") {
		return "", false
	}
	if _, ok := flowLintUnsupportedFlowForms[symbol]; ok {
		return symbol, true
	}
	if slash := strings.LastIndex(symbol, "/"); slash >= 0 && slash+1 < len(symbol) {
		name := symbol[slash+1:]
		if rule, ok := flowLintUnsupportedFlowForms[name]; ok && rule.code == "prohibited_orchestration_transform" {
			return name, true
		}
	}
	return "", false
}

func transformReferenceHead(symbol string) (string, bool) {
	symbol = strings.TrimPrefix(symbol, "#'")
	if strings.HasPrefix(symbol, ":") {
		return "", false
	}
	if slash := strings.LastIndex(symbol, "/"); slash >= 0 && slash+1 < len(symbol) {
		symbol = symbol[slash+1:]
	}
	_, ok := flowLintTransformReferenceHeads[symbol]
	return symbol, ok
}

func readerDiscardedRegionEnd(src string, start int) int {
	i := start
	pending := 0
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return i
		}
		if strings.HasPrefix(src[i:], "#_") {
			markers := 0
			for strings.HasPrefix(src[i:], "#_") {
				markers++
				i = skipClojureWhitespaceCommaAndComments(src, i+2)
			}
			end, err := readClojureFormEnd(src, i)
			if err != nil || end <= i {
				return start + 2
			}
			i = end
			pending += markers - 1
			continue
		}
		if pending == 0 {
			return i
		}
		end, err := readClojureFormEnd(src, i)
		if err != nil || end <= i {
			return i
		}
		i = end
		pending--
	}
	return i
}

func unsupportedFlowFormMatches(src string, baseOffset int) []unsupportedFlowFormMatch {
	return unsupportedFlowFormMatchesScoped(src, baseOffset, nil, true)
}

func unsupportedFlowFormMatchesScoped(src string, baseOffset int, boundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
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
					matches = append(matches, unsupportedFlowFormMatchesScoped(src[formStart:formEnd], baseOffset+formStart, boundNames, flagBareReferences)...)
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
			i = readerDiscardedRegionEnd(src, i)
			continue
		}
		if src[i] == '^' || strings.HasPrefix(src[i:], "#^") {
			activeStart, activeEnd, formEnd, hasActive, err := clojureActiveFormSpan(src, i)
			if err != nil || formEnd <= i {
				i++
			} else {
				if hasActive {
					matches = append(matches, unsupportedFlowFormMatchesScoped(src[activeStart:activeEnd], baseOffset+activeStart, boundNames, flagBareReferences)...)
				}
				i = formEnd
			}
			continue
		}
		if taggedEnd, ok := clojureTaggedLiteralEnd(src, i); ok {
			i = taggedEnd
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
		case '\'':
			_, _, next, hasActive, err := clojureActiveFormSpan(src, i+1)
			if err != nil || !hasActive || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		case '`':
			matches = append(matches, unsupportedSyntaxQuoteUnquoteMatches(src, i, baseOffset, boundNames)...)
			_, _, next, hasActive, err := clojureActiveFormSpan(src, i+1)
			if err != nil || !hasActive || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		case '(':
			elements, listEnd, err := parseActiveClojureListElements(src, i)
			if err != nil || len(elements) == 0 {
				i++
				continue
			}
			symbol := clojureFormToken(src, elements[0])
			if symbol == "quote" || symbol == "clojure.core/quote" ||
				symbol == "clojure.core/comment" || (symbol == "comment" && !flowLintSymbolIsShadowed(symbol, boundNames)) {
				i = listEnd
				continue
			}
			if isFlowLintFnForm(symbol) {
				matches = append(matches, unsupportedFnFormMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol == "clojure.core/for" || symbol == "clojure.core/doseq" ||
				((symbol == "for" || symbol == "doseq") && !flowLintSymbolIsShadowed(symbol, boundNames)) {
				matches = append(matches, unsupportedComprehensionMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol == "clojure.core/letfn" || symbol == "letfn*" || (symbol == "letfn" && !flowLintSymbolIsShadowed(symbol, boundNames)) {
				matches = append(matches, unsupportedLetfnFormMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol == "clojure.core/binding" || (symbol == "binding" && !flowLintSymbolIsShadowed(symbol, boundNames)) {
				matches = append(matches, unsupportedDynamicBindingMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol == "clojure.core/case" || (symbol == "case" && !flowLintSymbolIsShadowed(symbol, boundNames)) {
				matches = append(matches, unsupportedCaseFormMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol == "try" {
				matches = append(matches, unsupportedTryFormMatches(src, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if flowLintBindingFormApplies(symbol, boundNames) {
				matches = append(matches, unsupportedBindingFormMatches(src, symbol, elements, baseOffset, boundNames, flagBareReferences)...)
				i = listEnd
				continue
			}
			if symbol != "" && !flagBareReferences && !flowLintSymbolIsShadowed(symbol, boundNames) {
				if rule, ok := unsupportedFlowFormRuleKey(symbol); ok {
					matches = append(matches, unsupportedFlowFormMatch{symbol: symbol, rule: rule, offset: baseOffset + elements[0].Start})
				}
			}
			if referenceHead, ok := transformReferenceHead(symbol); ok && !flagBareReferences && !flowLintSymbolIsShadowed(symbol, boundNames) {
				allArgumentsCallable := flowLintTransformReferenceHeads[referenceHead]
				for argumentIndex, element := range elements[1:] {
					if !allArgumentsCallable && argumentIndex > 0 {
						break
					}
					reference := clojureFormToken(src, element)
					if rule, ok := unsupportedFlowFormRuleKey(reference); ok && !flowLintSymbolIsShadowed(reference, boundNames) && flowLintUnsupportedFlowForms[rule].code == "prohibited_orchestration_transform" {
						matches = append(matches, unsupportedFlowFormMatch{symbol: reference, rule: rule, offset: baseOffset + element.Start})
					}
				}
			}
		}
		if flagBareReferences {
			if strings.HasPrefix(src[i:], "#'") {
				tokenEnd := readClojureTokenEnd(src, i+2)
				if tokenEnd > i+2 {
					symbol := src[i:tokenEnd]
					if rule, ok := unsupportedFlowFormRuleKey(symbol); ok {
						matches = append(matches, unsupportedFlowFormMatch{symbol: symbol, rule: rule, offset: baseOffset + i})
					}
					i = tokenEnd
					continue
				}
			}
			tokenEnd := readClojureTokenEnd(src, i)
			if tokenEnd > i {
				symbol := src[i:tokenEnd]
				if rule, ok := unsupportedFlowFormRuleKey(symbol); ok && !flowLintSymbolIsShadowed(symbol, boundNames) {
					matches = append(matches, unsupportedFlowFormMatch{symbol: symbol, rule: rule, offset: baseOffset + i})
				}
				i = tokenEnd
				continue
			}
		}
		i++
	}
	return matches
}

func isFlowLintFnForm(symbol string) bool {
	switch symbol {
	case "fn", "fn*", "clojure.core/fn":
		return true
	default:
		return false
	}
}

func isFlowLintBindingForm(symbol string) bool {
	switch symbol {
	case "let", "let*", "clojure.core/let", "loop", "loop*", "clojure.core/loop",
		"with-open", "clojure.core/with-open",
		"if-let", "clojure.core/if-let", "when-let", "clojure.core/when-let",
		"if-some", "clojure.core/if-some", "when-some", "clojure.core/when-some",
		"when-first", "clojure.core/when-first":
		return true
	default:
		return false
	}
}

func flowLintBindingFormApplies(symbol string, boundNames map[string]bool) bool {
	if !isFlowLintBindingForm(symbol) {
		return false
	}
	if strings.Contains(symbol, "/") {
		return true
	}
	switch symbol {
	case "let", "let*", "loop", "loop*":
		return true
	default:
		return !flowLintSymbolIsShadowed(symbol, boundNames)
	}
}

func flowLintSymbolIsShadowed(symbol string, boundNames map[string]bool) bool {
	if boundNames == nil || strings.HasPrefix(symbol, "#'") || strings.Contains(symbol, "/") {
		return false
	}
	return boundNames[symbol]
}

func cloneFlowLintBoundNames(boundNames map[string]bool) map[string]bool {
	cloned := map[string]bool{}
	for name := range boundNames {
		cloned[name] = true
	}
	return cloned
}

func flowLintBoundNamesWithPattern(src string, boundNames map[string]bool, pattern clojureFormSpan) map[string]bool {
	withPattern := cloneFlowLintBoundNames(boundNames)
	for name := range clojureBindingNames(src, pattern) {
		withPattern[name] = true
	}
	return withPattern
}

func unsupportedDynamicBindingMatches(src string, elements []clojureFormSpan, baseOffset int, boundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	bindings, _, err := parseActiveClojureVectorElements(src, elements[1].Start)
	if err != nil {
		return nil
	}
	var matches []unsupportedFlowFormMatch
	for bindingIndex := 1; bindingIndex < len(bindings); bindingIndex += 2 {
		value := bindings[bindingIndex]
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[value.Start:value.End], baseOffset+value.Start, boundNames, true)...)
	}
	for _, body := range elements[2:] {
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, boundNames, flagBareReferences)...)
	}
	return matches
}

func unsupportedBindingFormMatches(src, symbol string, elements []clojureFormSpan, baseOffset int, outerBoundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	bindings, _, err := parseActiveClojureVectorElements(src, elements[1].Start)
	if err != nil {
		return nil
	}
	boundNames := cloneFlowLintBoundNames(outerBoundNames)
	var matches []unsupportedFlowFormMatch
	for bindingIndex := 1; bindingIndex < len(bindings); bindingIndex += 2 {
		initializer := bindings[bindingIndex]
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[initializer.Start:initializer.End], baseOffset+initializer.Start, boundNames, true)...)
		patternBoundNames := flowLintBoundNamesWithPattern(src, boundNames, bindings[bindingIndex-1])
		for _, bindingDefault := range clojureBindingDefaults(src, bindings[bindingIndex-1]) {
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[bindingDefault.Span.Start:bindingDefault.Span.End], baseOffset+bindingDefault.Span.Start, boundNames, true)...)
		}
		boundNames = patternBoundNames
	}
	for bodyIndex, body := range elements[2:] {
		bodyBoundNames := boundNames
		if (symbol == "if-let" || symbol == "clojure.core/if-let" || symbol == "if-some" || symbol == "clojure.core/if-some") && bodyIndex == 1 {
			bodyBoundNames = outerBoundNames
		}
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, bodyBoundNames, flagBareReferences)...)
	}
	return matches
}

func unsupportedFnFormMatches(src string, elements []clojureFormSpan, baseOffset int, outerBoundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	boundNames := cloneFlowLintBoundNames(outerBoundNames)
	formIndex := 1
	if token, _, ok := clojureActiveBareToken(src, elements[formIndex]); ok {
		if token != "" && !strings.HasPrefix(token, ":") {
			boundNames[token] = true
			formIndex++
		}
	}
	if formIndex >= len(elements) {
		return nil
	}
	var matches []unsupportedFlowFormMatch
	if clojureFormStartsWith(src, elements[formIndex].Start, '[') {
		parameterMatches, parameterBoundNames := unsupportedFnParameterMatches(src, elements[formIndex], baseOffset, boundNames)
		matches = append(matches, parameterMatches...)
		boundNames = parameterBoundNames
		for _, body := range elements[formIndex+1:] {
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, boundNames, flagBareReferences)...)
		}
		return matches
	}
	for _, arity := range elements[formIndex:] {
		arityElements, _, err := parseActiveClojureListElements(src, arity.Start)
		if err != nil || len(arityElements) == 0 || !clojureFormStartsWith(src, arityElements[0].Start, '[') {
			continue
		}
		arityBoundNames := cloneFlowLintBoundNames(boundNames)
		parameterMatches, parameterBoundNames := unsupportedFnParameterMatches(src, arityElements[0], baseOffset, arityBoundNames)
		matches = append(matches, parameterMatches...)
		arityBoundNames = parameterBoundNames
		for _, body := range arityElements[1:] {
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, arityBoundNames, flagBareReferences)...)
		}
	}
	return matches
}

func unsupportedFnParameterMatches(src string, parameters clojureFormSpan, baseOffset int, outerBoundNames map[string]bool) ([]unsupportedFlowFormMatch, map[string]bool) {
	boundNames := cloneFlowLintBoundNames(outerBoundNames)
	elements, _, err := parseActiveClojureVectorElements(src, parameters.Start)
	if err != nil {
		return nil, boundNames
	}
	var matches []unsupportedFlowFormMatch
	for _, parameter := range elements {
		if token, _, ok := clojureActiveBareToken(src, parameter); ok && token == "&" {
			continue
		}
		parameterBoundNames := flowLintBoundNamesWithPattern(src, boundNames, parameter)
		for _, bindingDefault := range clojureBindingDefaults(src, parameter) {
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[bindingDefault.Span.Start:bindingDefault.Span.End], baseOffset+bindingDefault.Span.Start, boundNames, true)...)
		}
		boundNames = parameterBoundNames
	}
	return matches, boundNames
}

func unsupportedLetfnFormMatches(src string, elements []clojureFormSpan, baseOffset int, outerBoundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	definitions, _, err := parseActiveClojureVectorElements(src, elements[1].Start)
	if err != nil {
		return nil
	}
	boundNames := cloneFlowLintBoundNames(outerBoundNames)
	definitionElements := make([][]clojureFormSpan, 0, len(definitions))
	for _, definition := range definitions {
		parts, _, definitionErr := parseActiveClojureListElements(src, definition.Start)
		if definitionErr != nil || len(parts) < 2 {
			continue
		}
		definitionElements = append(definitionElements, parts)
		if name, _, ok := clojureActiveBareToken(src, parts[0]); ok {
			boundNames[name] = true
		}
	}
	var matches []unsupportedFlowFormMatch
	for _, parts := range definitionElements {
		if clojureFormStartsWith(src, parts[1].Start, '[') {
			parameterMatches, definitionBoundNames := unsupportedFnParameterMatches(src, parts[1], baseOffset, boundNames)
			matches = append(matches, parameterMatches...)
			for _, body := range parts[2:] {
				matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, definitionBoundNames, flagBareReferences)...)
			}
			continue
		}
		for _, arity := range parts[1:] {
			arityParts, _, arityErr := parseActiveClojureListElements(src, arity.Start)
			if arityErr != nil || len(arityParts) == 0 || !clojureFormStartsWith(src, arityParts[0].Start, '[') {
				continue
			}
			parameterMatches, definitionBoundNames := unsupportedFnParameterMatches(src, arityParts[0], baseOffset, boundNames)
			matches = append(matches, parameterMatches...)
			for _, body := range arityParts[1:] {
				matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, definitionBoundNames, flagBareReferences)...)
			}
		}
	}
	for _, body := range elements[2:] {
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, boundNames, flagBareReferences)...)
	}
	return matches
}

func unsupportedComprehensionMatches(src string, elements []clojureFormSpan, baseOffset int, outerBoundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	bindings, _, err := parseActiveClojureVectorElements(src, elements[1].Start)
	if err != nil {
		return nil
	}
	boundNames := cloneFlowLintBoundNames(outerBoundNames)
	var matches []unsupportedFlowFormMatch
	for bindingIndex := 0; bindingIndex < len(bindings); {
		token, _, tokenOK := clojureActiveBareToken(src, bindings[bindingIndex])
		if tokenOK && token == ":let" && bindingIndex+1 < len(bindings) {
			letBindings, _, letErr := parseActiveClojureVectorElements(src, bindings[bindingIndex+1].Start)
			if letErr == nil {
				for letIndex := 1; letIndex < len(letBindings); letIndex += 2 {
					initializer := letBindings[letIndex]
					matches = append(matches, unsupportedFlowFormMatchesScoped(src[initializer.Start:initializer.End], baseOffset+initializer.Start, boundNames, true)...)
					patternBoundNames := flowLintBoundNamesWithPattern(src, boundNames, letBindings[letIndex-1])
					for _, bindingDefault := range clojureBindingDefaults(src, letBindings[letIndex-1]) {
						matches = append(matches, unsupportedFlowFormMatchesScoped(src[bindingDefault.Span.Start:bindingDefault.Span.End], baseOffset+bindingDefault.Span.Start, boundNames, true)...)
					}
					boundNames = patternBoundNames
				}
			}
			bindingIndex += 2
			continue
		}
		if tokenOK && (token == ":when" || token == ":while") && bindingIndex+1 < len(bindings) {
			condition := bindings[bindingIndex+1]
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[condition.Start:condition.End], baseOffset+condition.Start, boundNames, true)...)
			bindingIndex += 2
			continue
		}
		if bindingIndex+1 >= len(bindings) {
			break
		}
		collection := bindings[bindingIndex+1]
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[collection.Start:collection.End], baseOffset+collection.Start, boundNames, true)...)
		patternBoundNames := flowLintBoundNamesWithPattern(src, boundNames, bindings[bindingIndex])
		for _, bindingDefault := range clojureBindingDefaults(src, bindings[bindingIndex]) {
			matches = append(matches, unsupportedFlowFormMatchesScoped(src[bindingDefault.Span.Start:bindingDefault.Span.End], baseOffset+bindingDefault.Span.Start, boundNames, true)...)
		}
		boundNames = patternBoundNames
		bindingIndex += 2
	}
	for _, body := range elements[2:] {
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, boundNames, flagBareReferences)...)
	}
	return matches
}

func unsupportedCaseFormMatches(src string, elements []clojureFormSpan, baseOffset int, boundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	if len(elements) < 2 {
		return nil
	}
	var matches []unsupportedFlowFormMatch
	test := elements[1]
	matches = append(matches, unsupportedFlowFormMatchesScoped(src[test.Start:test.End], baseOffset+test.Start, boundNames, flagBareReferences)...)
	remaining := elements[2:]
	for resultIndex := 1; resultIndex < len(remaining); resultIndex += 2 {
		result := remaining[resultIndex]
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[result.Start:result.End], baseOffset+result.Start, boundNames, flagBareReferences)...)
	}
	if len(remaining)%2 == 1 {
		fallback := remaining[len(remaining)-1]
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[fallback.Start:fallback.End], baseOffset+fallback.Start, boundNames, flagBareReferences)...)
	}
	return matches
}

func unsupportedTryFormMatches(src string, elements []clojureFormSpan, baseOffset int, boundNames map[string]bool, flagBareReferences bool) []unsupportedFlowFormMatch {
	var matches []unsupportedFlowFormMatch
	for _, element := range elements[1:] {
		parts, _, err := parseActiveClojureListElements(src, element.Start)
		if err == nil && len(parts) > 0 {
			head := clojureFormToken(src, parts[0])
			switch head {
			case "catch":
				if len(parts) < 3 {
					continue
				}
				catchBoundNames := cloneFlowLintBoundNames(boundNames)
				if name, _, ok := clojureActiveBareToken(src, parts[2]); ok {
					catchBoundNames[name] = true
				}
				for _, body := range parts[3:] {
					matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, catchBoundNames, flagBareReferences)...)
				}
				continue
			case "finally":
				for _, body := range parts[1:] {
					matches = append(matches, unsupportedFlowFormMatchesScoped(src[body.Start:body.End], baseOffset+body.Start, boundNames, flagBareReferences)...)
				}
				continue
			}
		}
		matches = append(matches, unsupportedFlowFormMatchesScoped(src[element.Start:element.End], baseOffset+element.Start, boundNames, flagBareReferences)...)
	}
	return matches
}

func clojureActiveBareToken(src string, span clojureFormSpan) (string, int, bool) {
	activeStart, activeEnd, _, hasActive, err := clojureActiveFormSpan(src, span.Start)
	if err != nil || !hasActive || activeEnd > span.End {
		return "", 0, false
	}
	token := strings.TrimSpace(src[activeStart:activeEnd])
	if token == "" {
		return "", 0, false
	}
	if strings.HasPrefix(token, "#'") {
		if readClojureTokenEnd(src, activeStart+2) != activeEnd {
			return "", 0, false
		}
		return token, activeStart, true
	}
	if readClojureTokenEnd(src, activeStart) != activeEnd {
		return "", 0, false
	}
	return token, activeStart, true
}

func clojureBindingNames(src string, span clojureFormSpan) map[string]bool {
	names := map[string]bool{}
	activeStart, activeEnd, _, hasActive, err := clojureActiveFormSpan(src, span.Start)
	if err != nil || !hasActive || activeEnd > span.End {
		return names
	}
	if token, _, ok := clojureActiveBareToken(src, span); ok {
		if token != "_" && token != "&" && token != ":as" && !strings.HasPrefix(token, ":") {
			if slash := strings.LastIndex(token, "/"); slash >= 0 && slash+1 < len(token) {
				token = token[slash+1:]
			}
			names[token] = true
		}
		return names
	}
	merge := func(found map[string]bool) {
		for name := range found {
			names[name] = true
		}
	}
	collectionStart := activeStart
	if mapStart, ok := clojureNamespacedMapStart(src, activeStart, activeEnd); ok {
		collectionStart = mapStart
	}
	switch src[collectionStart] {
	case '[':
		elements, _, vectorErr := parseActiveClojureVectorElements(src, collectionStart)
		if vectorErr == nil {
			for _, element := range elements {
				merge(clojureBindingNames(src, element))
			}
		}
	case '{':
		entries, _, mapErr := parseActiveClojureMapEntries(src, collectionStart)
		if mapErr == nil {
			for _, entry := range entries {
				switch {
				case entry.KeyToken == ":keys" || entry.KeyToken == ":syms" || entry.KeyToken == ":strs" ||
					entry.KeyToken == "::keys" || entry.KeyToken == "::syms" || entry.KeyToken == "::strs" ||
					strings.HasSuffix(entry.KeyToken, "/keys") || strings.HasSuffix(entry.KeyToken, "/syms") || strings.HasSuffix(entry.KeyToken, "/strs"):
					valueSpan := clojureFormSpan{Start: entry.ValueStart, End: entry.ValueEnd}
					valueStart, _, _, valueActive, valueErr := clojureActiveFormSpan(src, entry.ValueStart)
					if valueErr == nil && valueActive && valueStart < len(src) && src[valueStart] == '[' {
						elements, _, vectorErr := parseActiveClojureVectorElements(src, valueStart)
						if vectorErr == nil {
							for _, element := range elements {
								if token, _, ok := clojureActiveBareToken(src, element); ok {
									token = strings.TrimLeft(token, ":")
									if slash := strings.LastIndex(token, "/"); slash >= 0 && slash+1 < len(token) {
										token = token[slash+1:]
									}
									if token != "" {
										names[token] = true
									}
								}
							}
						}
					} else {
						merge(clojureBindingNames(src, valueSpan))
					}
				case entry.KeyToken == ":as":
					merge(clojureBindingNames(src, clojureFormSpan{Start: entry.ValueStart, End: entry.ValueEnd}))
				case !strings.HasPrefix(entry.KeyToken, ":"):
					merge(clojureBindingNames(src, clojureFormSpan{Start: entry.KeyStart, End: entry.KeyEnd}))
				}
			}
		}
	}
	return names
}

func clojureNamespacedMapStart(src string, start, end int) (int, bool) {
	if start < 0 || end > len(src) || start+2 >= end || !strings.HasPrefix(src[start:], "#:") {
		return start, false
	}
	i := start + 2
	if i < end && src[i] == ':' {
		i++
	}
	if i < end && src[i] == '{' {
		return i, true
	}
	i = readClojureTokenEnd(src, i)
	if i < end && src[i] == '{' {
		return i, true
	}
	return start, false
}

type clojureBindingDefault struct {
	Name string
	Span clojureFormSpan
}

func clojureBindingDefaults(src string, span clojureFormSpan) []clojureBindingDefault {
	activeStart, activeEnd, _, hasActive, err := clojureActiveFormSpan(src, span.Start)
	if err != nil || !hasActive || activeEnd > span.End {
		return nil
	}
	var defaults []clojureBindingDefault
	collectionStart := activeStart
	if mapStart, ok := clojureNamespacedMapStart(src, activeStart, activeEnd); ok {
		collectionStart = mapStart
	}
	switch src[collectionStart] {
	case '[':
		elements, _, vectorErr := parseActiveClojureVectorElements(src, collectionStart)
		if vectorErr == nil {
			for _, element := range elements {
				defaults = append(defaults, clojureBindingDefaults(src, element)...)
			}
		}
	case '{':
		entries, _, mapErr := parseActiveClojureMapEntries(src, collectionStart)
		if mapErr != nil {
			return defaults
		}
		for _, entry := range entries {
			if entry.KeyToken == ":or" {
				orEntries, _, orErr := parseActiveClojureMapEntries(src, entry.ValueStart)
				if orErr == nil {
					for _, defaultEntry := range orEntries {
						name := strings.TrimLeft(defaultEntry.KeyToken, ":")
						if slash := strings.LastIndex(name, "/"); slash >= 0 && slash+1 < len(name) {
							name = name[slash+1:]
						}
						defaults = append(defaults, clojureBindingDefault{
							Name: name,
							Span: clojureFormSpan{Start: defaultEntry.ValueStart, End: defaultEntry.ValueEnd},
						})
					}
				}
				continue
			}
			if !strings.HasPrefix(entry.KeyToken, ":") {
				defaults = append(defaults, clojureBindingDefaults(src, clojureFormSpan{Start: entry.KeyStart, End: entry.KeyEnd})...)
			}
		}
	}
	return defaults
}

func unsupportedBareTransformReferenceMatches(src string, span clojureFormSpan, baseOffset int, boundNames map[string]bool) []unsupportedFlowFormMatch {
	token, activeStart, ok := clojureActiveBareToken(src, span)
	if !ok || flowLintSymbolIsShadowed(token, boundNames) {
		return nil
	}
	if rule, ok := unsupportedFlowFormRuleKey(token); ok &&
		flowLintUnsupportedFlowForms[rule].code == "prohibited_orchestration_transform" {
		return []unsupportedFlowFormMatch{{symbol: token, rule: rule, offset: baseOffset + activeStart}}
	}
	return nil
}

func unsupportedTransformReferenceMatches(src string, span clojureFormSpan, baseOffset int, boundNames map[string]bool) []unsupportedFlowFormMatch {
	if matches := unsupportedBareTransformReferenceMatches(src, span, baseOffset, boundNames); len(matches) > 0 {
		return matches
	}
	return unsupportedFlowFormMatchesScoped(src[span.Start:span.End], baseOffset+span.Start, boundNames, true)
}

// A syntax quote produces data, except for its unquoted forms. Those forms are
// evaluated while the surrounding orchestration runs and therefore need the
// same transform checks as ordinary executable forms.
func unsupportedSyntaxQuoteUnquoteMatches(src string, start int, baseOffset int, boundNames map[string]bool) []unsupportedFlowFormMatch {
	end, err := readClojureFormEnd(src, start+1)
	if err != nil || end <= start+1 {
		return nil
	}
	return unsupportedSyntaxQuoteRangeMatches(src, start+1, end, baseOffset, boundNames)
}

func unsupportedSyntaxQuoteRangeMatches(src string, start, end, baseOffset int, boundNames map[string]bool) []unsupportedFlowFormMatch {
	return unsupportedSyntaxQuoteRangeMatchesAtDepth(src, start, end, baseOffset, boundNames, 1)
}

func unsupportedSyntaxQuoteRangeMatchesAtDepth(src string, start, end, baseOffset int, boundNames map[string]bool, quoteDepth int) []unsupportedFlowFormMatch {
	var matches []unsupportedFlowFormMatch
	for i := start; i < end; {
		if strings.HasPrefix(src[i:], `#"`) {
			next, regexErr := readClojureRegexTokenEnd(src, i+1)
			if regexErr != nil || next <= i+1 {
				i++
			} else {
				i = next
			}
			continue
		}
		if strings.HasPrefix(src[i:], "#_") {
			i = readerDiscardedRegionEnd(src, i)
			continue
		}
		if strings.HasPrefix(src[i:], "#?") {
			activeStart, activeEnd, next, ok := activeReaderConditionalForm(src, i)
			if ok {
				if activeStart >= 0 {
					matches = append(matches, unsupportedSyntaxQuoteRangeMatchesAtDepth(src, activeStart, activeEnd, baseOffset, boundNames, quoteDepth)...)
				}
				if next > i {
					i = next
				} else {
					i++
				}
				continue
			}
		}
		if taggedEnd, ok := clojureTaggedLiteralEnd(src, i); ok {
			i = taggedEnd
			continue
		}
		switch src[i] {
		case '"':
			_, _, next, stringErr := readClojureStringToken(src, i)
			if stringErr != nil || next <= i {
				i++
			} else {
				i = next
			}
			continue
		case ';':
			i = readCommentEnd(src, i)
			continue
		case '`':
			activeStart, activeEnd, formEnd, hasActive, formErr := clojureActiveFormSpan(src, i+1)
			if formErr == nil && hasActive && activeEnd <= end {
				matches = append(matches, unsupportedSyntaxQuoteRangeMatchesAtDepth(src, activeStart, activeEnd, baseOffset, boundNames, quoteDepth+1)...)
			}
			if formErr == nil && formEnd > i {
				i = formEnd
			} else {
				i++
			}
			continue
		case '~':
			formStart := i + 1
			if formStart < end && src[formStart] == '@' {
				formStart++
			}
			activeStart, activeEnd, formEnd, hasActive, formErr := clojureActiveFormSpan(src, formStart)
			if formErr == nil && hasActive && activeEnd <= end {
				if quoteDepth == 1 {
					matches = append(matches, unsupportedTransformReferenceMatches(src, clojureFormSpan{Start: activeStart, End: activeEnd}, baseOffset, boundNames)...)
				} else {
					matches = append(matches, unsupportedSyntaxQuoteRangeMatchesAtDepth(src, activeStart, activeEnd, baseOffset, boundNames, quoteDepth-1)...)
				}
			}
			if formErr == nil && formEnd > i {
				i = formEnd
			} else {
				i++
			}
			continue
		}
		i++
	}
	return matches
}

// parseActiveClojureListElements returns the forms the Clojure reader exposes
// in a list. Reader conditionals are reduced to their active branch, metadata
// prefixes are unwrapped, and discarded forms are omitted. Transform lint can
// then reason about the actual callable and arguments instead of raw tokens.
func parseActiveClojureListElements(src string, start int) ([]clojureFormSpan, int, error) {
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
		if strings.HasPrefix(src[i:], "#_") {
			discardEnd := readerDiscardedRegionEnd(src, i)
			if discardEnd <= i {
				return out, i, fmt.Errorf("could not advance past discarded list forms near byte %d", i)
			}
			i = discardEnd
			continue
		}
		activeStart, activeEnd, formEnd, hasActive, err := clojureActiveFormSpan(src, i)
		if err != nil {
			return out, formEnd, err
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
					spliced, branchEnd, branchErr = parseActiveClojureVectorElements(src, activeStart)
				case '(':
					spliced, branchEnd, branchErr = parseActiveClojureListElements(src, activeStart)
				default:
					return out, i, fmt.Errorf("splicing reader conditional must select a vector or list near byte %d", i)
				}
				if branchErr != nil {
					return out, i, branchErr
				}
				if branchEnd != activeEnd {
					return out, i, fmt.Errorf("splicing reader conditional branch did not consume its collection near byte %d", i)
				}
				out = append(out, spliced...)
			} else {
				out = append(out, clojureFormSpan{Start: activeStart, End: activeEnd})
			}
		}
		if formEnd <= i {
			return out, formEnd, fmt.Errorf("could not advance past list element near byte %d", i)
		}
		i = formEnd
	}
	return out, i, fmt.Errorf("unterminated list")
}

func parseActiveClojureVectorElements(src string, start int) ([]clojureFormSpan, int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) || src[i] != '[' {
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
		if strings.HasPrefix(src[i:], "#_") {
			discardEnd := readerDiscardedRegionEnd(src, i)
			if discardEnd <= i {
				return out, i, fmt.Errorf("could not advance past discarded vector forms near byte %d", i)
			}
			i = discardEnd
			continue
		}
		activeStart, activeEnd, formEnd, hasActive, err := clojureActiveFormSpan(src, i)
		if err != nil {
			return out, formEnd, err
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
					spliced, branchEnd, branchErr = parseActiveClojureVectorElements(src, activeStart)
				case '(':
					spliced, branchEnd, branchErr = parseActiveClojureListElements(src, activeStart)
				default:
					return out, i, fmt.Errorf("splicing reader conditional must select a vector or list near byte %d", i)
				}
				if branchErr != nil {
					return out, i, branchErr
				}
				if branchEnd != activeEnd {
					return out, i, fmt.Errorf("splicing reader conditional branch did not consume its collection near byte %d", i)
				}
				out = append(out, spliced...)
			} else {
				out = append(out, clojureFormSpan{Start: activeStart, End: activeEnd})
			}
		}
		if formEnd <= i {
			return out, formEnd, fmt.Errorf("could not advance past vector element near byte %d", i)
		}
		i = formEnd
	}
	return out, i, fmt.Errorf("unterminated vector")
}

func parseActiveClojureMapEntries(src string, start int) ([]clojureMapEntry, int, error) {
	i := skipClojureWhitespaceCommaAndComments(src, start)
	if i >= len(src) || src[i] != '{' {
		return nil, i, fmt.Errorf("expected map near byte %d", start)
	}
	i++
	var forms []clojureFormSpan
	for i < len(src) {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= len(src) {
			return nil, i, fmt.Errorf("unterminated map")
		}
		if src[i] == '}' {
			if len(forms)%2 != 0 {
				return nil, i, fmt.Errorf("map has an unmatched key near byte %d", forms[len(forms)-1].Start)
			}
			entries := make([]clojureMapEntry, 0, len(forms)/2)
			for formIndex := 0; formIndex < len(forms); formIndex += 2 {
				key := forms[formIndex]
				value := forms[formIndex+1]
				keyToken := strings.TrimSpace(src[key.Start:key.End])
				entries = append(entries, clojureMapEntry{
					KeyToken: keyToken, KeyName: clojureKeywordName(keyToken),
					KeyStart: key.Start, KeyEnd: key.End, ValueStart: value.Start, ValueEnd: value.End,
				})
			}
			return entries, i + 1, nil
		}
		if strings.HasPrefix(src[i:], "#_") {
			discardEnd := readerDiscardedRegionEnd(src, i)
			if discardEnd <= i {
				return nil, i, fmt.Errorf("could not advance past discarded map forms near byte %d", i)
			}
			i = discardEnd
			continue
		}
		activeStart, activeEnd, formEnd, hasActive, err := clojureActiveFormSpan(src, i)
		if err != nil {
			return nil, formEnd, err
		}
		if hasActive {
			if strings.HasPrefix(src[i:], "#?@") {
				var spliced []clojureFormSpan
				var branchEnd int
				var branchErr error
				if activeStart >= activeEnd || activeStart >= len(src) {
					return nil, i, fmt.Errorf("splicing reader conditional selected an empty branch near byte %d", i)
				}
				switch src[activeStart] {
				case '[':
					spliced, branchEnd, branchErr = parseActiveClojureVectorElements(src, activeStart)
				case '(':
					spliced, branchEnd, branchErr = parseActiveClojureListElements(src, activeStart)
				default:
					return nil, i, fmt.Errorf("splicing reader conditional must select a vector or list near byte %d", i)
				}
				if branchErr != nil {
					return nil, i, branchErr
				}
				if branchEnd != activeEnd {
					return nil, i, fmt.Errorf("splicing reader conditional branch did not consume its collection near byte %d", i)
				}
				forms = append(forms, spliced...)
			} else {
				forms = append(forms, clojureFormSpan{Start: activeStart, End: activeEnd})
			}
		}
		if formEnd <= i {
			return nil, formEnd, fmt.Errorf("could not advance past map form near byte %d", i)
		}
		i = formEnd
	}
	return nil, i, fmt.Errorf("unterminated map")
}

func clojureTaggedLiteralEnd(src string, start int) (int, bool) {
	if start < 0 || start+1 >= len(src) || src[start] != '#' {
		return start, false
	}
	switch src[start+1] {
	case '"', '?', '_', '{', '(', '\'', '^', '#', '=', ':':
		return start, false
	}
	tagEnd := readClojureTokenEnd(src, start+1)
	if tagEnd <= start+1 {
		return start, false
	}
	valueStart := skipClojureWhitespaceCommaAndComments(src, tagEnd)
	valueEnd, err := readClojureFormEnd(src, valueStart)
	if err != nil || valueEnd <= valueStart {
		return start, false
	}
	return valueEnd, true
}

func unwrapTopLevelQuotedFlowSource(src string) (string, int) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	if i < len(src) && (src[i] == '\'' || src[i] == '`') {
		activeStart, activeEnd, _, hasActive, err := clojureActiveFormSpan(src, i+1)
		if err == nil && hasActive {
			return src[activeStart:activeEnd], activeStart
		}
		return src, 0
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

func unwrapTopLevelMetadataFlowSource(src string) (string, int) {
	i := skipClojureWhitespaceCommaAndComments(src, 0)
	if i >= len(src) || (src[i] != '^' && !strings.HasPrefix(src[i:], "#^")) {
		return src, 0
	}
	metadataStart := i + 1
	if src[i] == '#' {
		metadataStart++
	}
	metadataEnd, err := readClojureFormEnd(src, metadataStart)
	if err != nil || metadataEnd <= metadataStart {
		return src, 0
	}
	formStart := skipClojureWhitespaceCommaAndComments(src, metadataEnd)
	if formStart >= len(src) {
		return src, 0
	}
	return src[formStart:], formStart
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
		keyStart, keyEnd, keyFormEnd, keyActive, err := clojureActiveFormSpan(src, i)
		if err != nil || !keyActive || keyEnd <= keyStart || keyFormEnd <= i {
			return "", 0, false
		}
		key := clojureKeywordName(src[keyStart:keyEnd])
		valueStart := skipClojureWhitespaceCommaAndComments(src, keyFormEnd)
		if valueStart >= len(src) {
			return "", 0, false
		}
		activeStart, activeEnd, valueFormEnd, valueActive, err := clojureActiveFormSpan(src, valueStart)
		if err != nil || !valueActive || activeEnd <= activeStart || valueFormEnd <= valueStart {
			return "", 0, false
		}
		if key == targetKey {
			return src[activeStart:activeEnd], activeStart, true
		}
		i = valueFormEnd
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

func localAuthoringShapeDiagnostics(flowLiteral, rootLiteral string, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
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
	diagnostics = append(diagnostics, localPackagedStepReferenceDiagnostics(flowLiteral, rootLiteral, stepsEntry, byKey["agents"])...)
	diagnostics = append(diagnostics, localFlowStepArityDiagnostics(flowLiteral, rootLiteral != "" && rootLiteral != flowLiteral)...)
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
			discardEnd := readerDiscardedRegionEnd(src, i)
			if discardEnd <= i {
				return i, i, i, false, fmt.Errorf("could not read discarded form near byte %d", i)
			}
			i = discardEnd
		case src[i] == '\'' || src[i] == '`':
			_, _, quotedEnd, quotedActive, quoteErr := clojureActiveFormSpan(src, i+1)
			if quoteErr != nil || !quotedActive || quotedEnd <= i+1 {
				if quoteErr == nil {
					quoteErr = fmt.Errorf("could not read quoted form near byte %d", i)
				}
				return i, i, i, false, quoteErr
			}
			return i, quotedEnd, quotedEnd, true, nil
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

// localFlowStepReference records one EXECUTABLE flow/step form found by the
// quote-aware body walker (nested quoted forms are treated as data and never
// collected). StepID is set only for the packaged (flow/step :ns/id ...) shape;
// PathID is a best-effort display id for diagnostics; ElementCount is the raw
// list arity including the flow/step head. TypeToken is the raw token in the
// type position; FirstArgKeyword/SecondArgKeyword record whether the
// first/second argument are keyword literals — the fact the server's
// step-call? analysis keys its push-time validation on — and SecondArgMap
// whether the second argument is a map literal. Plain marks forms whose every
// element is free of reader macros; shape diagnostics only fire on those.
type localFlowStepReference struct {
	StepID           string
	PathID           string
	TypeToken        string
	ByteOffset       int
	ElementCount     int
	FirstArgKeyword  bool
	SecondArgKeyword bool
	SecondArgMap     bool
	// SecondArgNeverStepID is true when the second argument is a literal that
	// can never evaluate to a keyword step id: a map, vector, string, number,
	// nil/boolean/character, or the empty list. ThirdArgNeverMap is the mirror
	// for the config position: a literal that can never evaluate to a map.
	// Symbols and call forms COULD evaluate to the required type at runtime,
	// so they stay ambiguous and never trigger shape warnings.
	SecondArgNeverStepID bool
	ThirdArgNeverMap     bool
	// FirstArgNeverStepType marks a fixed literal in the type position that
	// can never be a valid step type or packaged id.
	FirstArgNeverStepType bool
	Plain                 bool
}

// stripClojureStringLiterals returns src with the CONTENTS of double-quoted
// string literals blanked to spaces (length-preserving, \" escapes honored,
// char literals like \" skipped), so crude textual scans cannot misread
// reader-macro characters or tokens inside strings — URLs with #fragments,
// emails with @, descriptions containing "#_", and so on.
func stripClojureStringLiterals(src string) string {
	out := []byte(src)
	inString := false
	inComment := false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inComment {
			// Comment text is not syntax either: blank it so an unmatched
			// quote inside a ; comment cannot lock the scanner in string
			// mode and blank real forms on later lines.
			if c == '\n' {
				inComment = false
			} else {
				out[i] = ' '
			}
			continue
		}
		if !inString {
			if c == '\\' && i+1 < len(out) {
				i++ // char literal: the quoted character is not syntax
				continue
			}
			if c == ';' {
				inComment = true
				out[i] = ' '
				continue
			}
			if c == '"' {
				inString = true
			}
			continue
		}
		switch c {
		case '\\':
			out[i] = ' '
			if i+1 < len(out) {
				out[i+1] = ' '
				i++
			}
		case '"':
			inString = false
		default:
			out[i] = ' '
		}
	}
	return string(out)
}

// plainClojureForm reports whether a span's source text is completely free of
// reader macros: discards (#_), reader conditionals (#?), sets/fns/regexes/
// vars/tagged literals (any other #), syntax quotes and unquotes (` ~ @),
// metadata (^), and quote characters ('). String literal CONTENTS are blanked
// first, so {:url "https://x/#part"} stays plain; a regex #"..." still reads
// as non-plain via its # prefix outside the string. Beyond that the check is
// a dumb text scan: it can only produce false negatives, never false
// positives, which is the right direction for gating diagnostics.
func plainClojureForm(src string, span clojureFormSpan) bool {
	if span.Start < 0 || span.End > len(src) || span.End < span.Start {
		return false
	}
	return !strings.ContainsAny(stripClojureStringLiterals(src[span.Start:span.End]), "#`~^'@")
}

// clojureNeverKeywordLiteral reports whether the span holds a fixed literal
// that can never evaluate to a keyword: map/vector/string/char heads, number
// literals, nil/true/false, and the empty list. Symbols and non-empty call
// forms stay ambiguous.
func clojureNeverKeywordLiteral(src string, span clojureFormSpan) bool {
	start, ok := clojureActiveFormStart(src, span.Start)
	if !ok || start >= len(src) {
		return false
	}
	switch c := src[start]; {
	case c == '{' || c == '[' || c == '"' || c == '\\':
		return true
	case c >= '0' && c <= '9':
		return true
	case (c == '-' || c == '+') && start+1 < len(src) && src[start+1] >= '0' && src[start+1] <= '9':
		return true
	}
	switch clojureFormToken(src, span) {
	case "nil", "true", "false":
		return true
	}
	return clojureEmptyListForm(src, span)
}

// clojureEmptyListForm reports whether the span is an empty list literal —
// (), ( ), (,,), or parens around only comments — which evaluates to itself
// and can never be a keyword or a map.
func clojureEmptyListForm(src string, span clojureFormSpan) bool {
	i, ok := clojureActiveFormStart(src, span.Start)
	if !ok || i >= len(src) || src[i] != '(' {
		return false
	}
	j := skipClojureWhitespaceCommaAndComments(src, i+1)
	return j < len(src) && src[j] == ')'
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
	// The customary top-level quote is not an enclosing reader prefix, but a
	// top-level reader conditional is: forms below it get no shape diagnostics.
	return localFlowStepReferencesInRange(flowSource, 0, len(flowSource), baseOffset, readerOffset > 0)
}

func localFlowStepReferencesInRange(src string, start, end, baseOffset int, enclosed bool) ([]localFlowStepReference, error) {
	var spans []clojureFormSpan
	for i := skipClojureWhitespaceCommaAndComments(src, start); i < end; {
		formEnd, err := readClojureFormEnd(src, i)
		if err != nil {
			return nil, err
		}
		if formEnd <= i || formEnd > end {
			return nil, fmt.Errorf("could not read flow form near byte %d", i)
		}
		spans = append(spans, clojureFormSpan{Start: i, End: formEnd})
		i = skipClojureWhitespaceCommaAndComments(src, formEnd)
	}
	var references []localFlowStepReference
	err := forEachActiveSiblingSpan(src, spans, func(span clojureFormSpan) error {
		found, err := localFlowStepReferencesForForm(src, span, baseOffset, enclosed)
		if err != nil {
			return err
		}
		references = append(references, found...)
		return nil
	})
	return references, err
}

func localFlowStepReferencesForForm(src string, span clojureFormSpan, baseOffset int, enclosed bool) ([]localFlowStepReference, error) {
	return localFlowStepReferencesForFormAtDepth(src, span, baseOffset, 0, enclosed)
}

// localFlowStepReferencesForFormAtDepth walks one form. enclosed records that
// a reader prefix (metadata, reader conditional, deref, unquote) was stripped
// at THIS or ANY ancestor level on the way to the current form; references
// found below such a prefix are excluded from shape diagnostics (non-plain).
func localFlowStepReferencesForFormAtDepth(src string, span clojureFormSpan, baseOffset, syntaxQuoteDepth int, enclosed bool) ([]localFlowStepReference, error) {
	// The vector/set element parsers strip reader prefixes before handing out
	// spans: FormStart keeps the raw start, Start the active form. When they
	// differ, a prefix (metadata, reader conditional, discard chain) was
	// erased on the way to this form — everything below counts as enclosed.
	if span.FormStart > 0 && span.FormStart < span.Start {
		enclosed = true
	}
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
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: formStart, End: formEnd}, baseOffset, syntaxQuoteDepth, true)
	}
	if src[i] == '^' {
		metaEnd, err := readClojureFormEnd(src, i+1)
		if err != nil || metaEnd >= span.End {
			return nil, err
		}
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: metaEnd, End: span.End}, baseOffset, syntaxQuoteDepth, true)
	}
	switch src[i] {
	case '\'':
		return nil, nil
	case '`':
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: i + 1, End: span.End}, baseOffset, syntaxQuoteDepth+1, enclosed)
	case '@':
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: i + 1, End: span.End}, baseOffset, syntaxQuoteDepth, true)
	case '~':
		if syntaxQuoteDepth <= 0 {
			return nil, nil
		}
		next := i + 1
		if next < span.End && src[next] == '@' {
			next++
		}
		return localFlowStepReferencesForFormAtDepth(src, clojureFormSpan{Start: next, End: span.End}, baseOffset, syntaxQuoteDepth-1, true)
	case '"':
		return nil, nil
	case '#':
		if strings.HasPrefix(src[i:], "#(") {
			return localFlowStepReferencesInListAtDepth(src, i+1, baseOffset, syntaxQuoteDepth, enclosed)
		}
		if strings.HasPrefix(src[i:], "#{") {
			elements, setEnd, err := parseClojureSetElements(src, i)
			if err != nil {
				return nil, err
			}
			// The element parsers splice #?@ branches flat, losing the
			// enclosing marker on the spliced spans (FormStart == Start).
			// Crude, over-suppressing recovery: if the raw container text has
			// a reader conditional anywhere, treat every element as enclosed.
			childEnclosed := enclosed || strings.Contains(stripClojureStringLiterals(src[i:setEnd]), "#?")
			var references []localFlowStepReference
			err = forEachActiveSiblingSpan(src, elements, func(element clojureFormSpan) error {
				found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth, childEnclosed)
				if err != nil {
					return err
				}
				references = append(references, found...)
				return nil
			})
			return references, err
		}
		return nil, nil
	case '(':
		return localFlowStepReferencesInListAtDepth(src, i, baseOffset, syntaxQuoteDepth, enclosed)
	case '[':
		elements, vecEnd, err := parseClojureVectorElements(src, i)
		if err != nil {
			return nil, err
		}
		// Same crude recovery as the set case: spliced #?@ branches lose the
		// enclosing marker, so any reader conditional in the raw vector text
		// marks every element enclosed.
		childEnclosed := enclosed || strings.Contains(stripClojureStringLiterals(src[i:vecEnd]), "#?")
		var references []localFlowStepReference
		// The vector parser strips discard prefixes into FormStart/Start, so
		// chain debt must be recovered from the prefix regions: a consumed
		// element is neither diagnosed nor recorded as a reference.
		pending := 0
		for _, element := range elements {
			pending += discardDebtBefore(src, element)
			if pending > 0 {
				pending--
				continue
			}
			found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth, childEnclosed)
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
				found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth, enclosed)
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

func localFlowStepReferencesInListAtDepth(src string, listStart, baseOffset, syntaxQuoteDepth int, enclosed bool) ([]localFlowStepReference, error) {
	elements, _, err := parseClojureListElements(src, listStart)
	if err != nil {
		return nil, err
	}
	if len(elements) >= 2 {
		switch clojureFormToken(src, elements[0]) {
		case "quote", "clojure.core/quote":
			return nil, nil
		case "comment", "clojure.core/comment":
			// Clojure comment macros discard their bodies: no shape
			// diagnostics and no references from them. A flow/step token
			// inside a comment still trips the token-count mismatch and
			// suppresses the unreferenced warning — the accepted
			// over-suppression direction.
			return nil, nil
		}
	}
	var references []localFlowStepReference
	if syntaxQuoteDepth == 0 && len(elements) >= 1 && clojureFormToken(src, elements[0]) == "flow/step" {
		// Plain also requires that no enclosing reader prefix (metadata,
		// reader conditional, deref, unquote) was stripped on the way here:
		// ^:meta (flow/step ...) or #?(:clj (flow/step ...)) must bail even
		// though the form's own elements look plain.
		plain := !enclosed
		for _, element := range elements {
			if plain && !plainClojureForm(src, element) {
				plain = false
			}
		}
		reference := localFlowStepReference{
			ByteOffset:   baseOffset + elements[0].Start,
			ElementCount: len(elements),
			Plain:        plain,
		}
		if len(elements) >= 2 {
			reference.FirstArgKeyword = clojureFormStartsWith(src, elements[1].Start, ':')
			reference.TypeToken = clojureFormToken(src, elements[1])
			reference.FirstArgNeverStepType = clojureNeverKeywordLiteral(src, elements[1])
		}
		if len(elements) >= 3 {
			reference.SecondArgKeyword = clojureFormStartsWith(src, elements[2].Start, ':')
			reference.SecondArgMap = clojureFormStartsWith(src, elements[2].Start, '{')
			reference.SecondArgNeverStepID = clojureNeverKeywordLiteral(src, elements[2])
		}
		if len(elements) >= 4 {
			if start, ok := clojureActiveFormStart(src, elements[3].Start); ok && start < len(src) {
				switch c := src[start]; {
				case c == '[' || c == '"' || c == ':' || c == '\\':
					// Vector, string, keyword, or character literal — never a
					// map. Sets and other dispatch forms bail at the plain
					// gate; symbols and calls stay ambiguous.
					reference.ThirdArgNeverMap = true
				case c >= '0' && c <= '9':
					reference.ThirdArgNeverMap = true
				case (c == '-' || c == '+') && start+1 < len(src) && src[start+1] >= '0' && src[start+1] <= '9':
					reference.ThirdArgNeverMap = true
				default:
					switch clojureFormToken(src, elements[3]) {
					case "nil", "true", "false":
						reference.ThirdArgNeverMap = true
					default:
						// The empty list — (), ( ), (,,) — evaluates to
						// itself and can never be a map; non-empty list
						// forms stay ambiguous.
						if clojureEmptyListForm(src, elements[3]) {
							reference.ThirdArgNeverMap = true
						}
					}
				}
			}
		}
		if len(elements) >= 2 {
			if stepID, ok := localQualifiedStepIDFromForm(src, elements[1]); ok {
				reference.StepID = stepID
				reference.PathID = ":" + stepID
				reference.ByteOffset = baseOffset + elements[1].Start
			} else if len(elements) >= 3 {
				if id, ok := clojureIdentifierFromForm(src, elements[2].Start); ok {
					reference.PathID = ":" + id
				}
			}
		}
		references = append(references, reference)
	}
	err = forEachActiveSiblingSpan(src, elements, func(element clojureFormSpan) error {
		found, err := localFlowStepReferencesForFormAtDepth(src, element, baseOffset, syntaxQuoteDepth, enclosed)
		if err != nil {
			return err
		}
		references = append(references, found...)
		return nil
	})
	return references, err
}

// discardDebtBefore simulates the reader-discard forms the element parsers
// stripped into the [FormStart, Start) prefix region of a span: each folded
// `#_ ... form` group carries N markers around one inner object, leaving a
// debt of N-1 objects that the ACTIVE span (and its following siblings) must
// pay. Non-discard prefixes (metadata, reader conditionals) contribute no
// debt.
func discardDebtBefore(src string, span clojureFormSpan) int {
	if span.FormStart <= 0 || span.FormStart >= span.Start || span.Start > len(src) {
		return 0
	}
	debt := 0
	i := span.FormStart
	for i < span.Start {
		i = skipClojureWhitespaceCommaAndComments(src, i)
		if i >= span.Start {
			break
		}
		if strings.HasPrefix(src[i:], "#_") {
			markers := 0
			for i+1 < span.Start && src[i] == '#' && src[i+1] == '_' {
				markers++
				i = skipClojureWhitespaceCommaAndComments(src, i+2)
			}
			end, err := readClojureFormEnd(src, i)
			if err != nil || end <= i || end > span.Start {
				break
			}
			i = end
			debt += markers - 1
			continue
		}
		end, err := readClojureFormEnd(src, i)
		if err != nil || end <= i {
			break
		}
		i = end
	}
	return debt
}

// forEachActiveSiblingSpan invokes visit for each element span that survives
// reader-discard chain consumption, mirroring the Clojure reader: a folded
// `#_ ... form` span carries N markers around ONE inner object (which absorbs
// one discard immediately), so it raises the pending count by N-1; pending
// discards then consume following sibling objects — INCLUDING further
// discard-headed spans, whose own markers chain through. Verified against the
// reader: (do #_ #_ :a #_ #_ :b :c X) consumes :a, :b, :c, AND X, while
// (do #_ #_ :a #_ :b :c X) consumes :a, :b, :c and leaves X live. Consumed
// elements are neither diagnosed nor recorded as references.
func forEachActiveSiblingSpan(src string, elements []clojureFormSpan, visit func(clojureFormSpan) error) error {
	pending := 0
	for _, element := range elements {
		markers := 0
		j := element.Start
		for j+1 < element.End && j+1 < len(src) && src[j] == '#' && src[j+1] == '_' {
			markers++
			j = skipClojureWhitespaceCommaAndComments(src, j+2)
		}
		if markers > 0 {
			pending += markers - 1
			continue
		}
		if pending > 0 {
			pending--
			continue
		}
		if err := visit(element); err != nil {
			return err
		}
	}
	return nil
}

func localPackagedStepReferenceDiagnostics(src, rootSrc string, stepsEntry, agentsEntry clojureMapEntry) []flowLintDiagnostic {
	// Byte offsets are measured against the include-EXPANDED literal; when
	// expansion changed the source they would point into the wrong place in
	// the root file, so they are omitted and the message/hint (which names
	// the include provenance) carries the location instead. Offsets stay
	// exact for include-free files.
	sourceExpanded := rootSrc != "" && rootSrc != src
	// Over-suppression rule for reader discards in :steps: the shared vector
	// parser drops single discards span-wise but cannot honor #_ #_ chain
	// consumption, so ANY discard in the raw :steps text makes the declared
	// set unknowable — both missing_packaged_step_reference and
	// unreferenced_packaged_step are suppressed for the flow.
	stepsUnknowable := stepsEntry.ValueEnd > stepsEntry.ValueStart &&
		strings.Contains(stripClojureStringLiterals(src[stepsEntry.ValueStart:stepsEntry.ValueEnd]), "#_")
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
	type declaredStep struct {
		id         string
		byteOffset int
	}
	var declaredSteps []declaredStep
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
		id := strings.TrimPrefix(strings.TrimSpace(stepID), ":")
		declared[id] = true
		declaredSteps = append(declaredSteps, declaredStep{id: id, byteOffset: span.Start})
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
		// Typed forms like (flow/step :http :fetch {...}) carry no packaged
		// step id; only qualified (flow/step :ns/id ...) references matter here.
		if reference.StepID == "" {
			continue
		}
		referenced[reference.StepID] = true
		if stepsUnknowable || declared[reference.StepID] || seen[reference.StepID] {
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
		if !sourceExpanded {
			diag["byteOffset"] = reference.ByteOffset
		}
		diagnostics = append(diagnostics, diag)
	}
	// Inverse check: a packaged step that is neither referenced from the :flow
	// body nor exposed via any :tools {:steps [...]} vector is dead weight and
	// usually signals a forgotten wiring step. Warning only — the server
	// accepts such flows — and the tools scan deliberately over-suppresses
	// (see localToolsExposedStepIDs). An opaque :tools value anywhere makes
	// the exposure set unknowable, so the warning is suppressed entirely.
	if stepsUnknowable {
		return diagnostics
	}
	// The same over-suppression applies to DYNAMIC calls: a flow/step whose
	// first argument is not a literal keyword — (flow/step kind {}) — could
	// invoke any packaged step at runtime, so the usage set is unknowable and
	// the warning is suppressed for the whole flow.
	for _, reference := range references {
		if reference.ElementCount >= 2 && !reference.FirstArgKeyword {
			return diagnostics
		}
	}
	// Indirect invocations — (apply flow/step [...]) or flow/step bound to
	// another symbol — produce no direct-call reference at all. Cheapest
	// sound rule: if the executable body mentions the flow/step token more
	// (or less) often than the walker found direct call heads, some usage is
	// indirect (or quoted data inflates the count) and the usage set is
	// unknowable → suppress the unreferenced warning for the whole flow.
	if bodySource, ok := localFlowBodyExecutableSource(src); !ok || countFlowStepTokens(stripClojureStringLiterals(bodySource)) != len(references) {
		return diagnostics
	}
	toolsExposed, toolsKnown := localToolsExposedStepIDs(src)
	if !toolsKnown {
		return diagnostics
	}
	for _, step := range declaredSteps {
		if !localStepIDValid(step.id) || referenced[step.id] || toolsExposed[step.id] {
			continue
		}
		message := fmt.Sprintf("Packaged step :%s is defined but never referenced from :flow.", step.id)
		hint := fmt.Sprintf("Call it with (flow/step :%s :<step-id> {...}) in the :flow body, expose it via :tools {:steps [...]}, or remove it with `breyta flows steps remove ...`.", step.id)
		if localStepDefinedViaInclude(src, rootSrc, step.id) {
			// The step definition lives in a #flow/include file, not the root
			// flow source, so root-file edit commands would not find it.
			message = fmt.Sprintf("Packaged step :%s (defined in a #flow/include file) is never referenced from :flow.", step.id)
			hint = fmt.Sprintf("Call it with (flow/step :%s :<step-id> {...}) in the :flow body, expose it via :tools {:steps [...]}, or edit the included source file directly — `breyta flows steps remove` only rewrites the root flow file.", step.id)
		}
		diag := lintDiagnostic(
			"warning",
			"unreferenced_packaged_step",
			[]string{":steps", ":" + step.id},
			message,
			hint,
			"local",
		)
		if !sourceExpanded {
			diag["byteOffset"] = step.byteOffset
		}
		diagnostics = append(diagnostics, diag)
	}
	return diagnostics
}

// localFlowBodyExecutableSource extracts the executable :flow body source the
// same way the reference walker does (top-level reader conditional and quote
// unwrapped).
func localFlowBodyExecutableSource(flowLiteral string) (string, bool) {
	flowSource, _, ok := topLevelFlowValueSource(flowLiteral)
	if !ok {
		return "", false
	}
	flowSource, _ = unwrapTopLevelReaderConditionalFlowSource(flowSource)
	flowSource, _ = unwrapTopLevelQuotedFlowSource(flowSource)
	return flowSource, true
}

// countFlowStepTokens counts standalone occurrences of the flow/step token in
// the source text — a deliberately crude scan (strings, comments, and quoted
// data included) whose only job is to disagree with the walker's direct-call
// count whenever the token appears in a non-head position.
func countFlowStepTokens(src string) int {
	const token = "flow/step"
	count := 0
	for i := 0; ; {
		j := strings.Index(src[i:], token)
		if j < 0 {
			break
		}
		pos := i + j
		end := pos + len(token)
		prevOK := pos == 0 || !isFlowStepTokenChar(src[pos-1])
		nextOK := end >= len(src) || !isFlowStepTokenChar(src[end])
		if prevOK && nextOK {
			count++
		}
		i = end
	}
	return count
}

func isFlowStepTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.' || c == '/' || c == '*' || c == '?' || c == '!' || c == '+':
		return true
	}
	return false
}

// localStepDefinedViaInclude reports whether a packaged step id that exists in
// the EXPANDED source has no definition in the root source — meaning it was
// pulled in through #flow/include and root-file edit commands cannot reach it.
func localStepDefinedViaInclude(expandedSrc, rootSrc, stepID string) bool {
	if rootSrc == "" || rootSrc == expandedSrc {
		return false
	}
	_, _, index, err := localStepSpansForID(rootSrc, stepID)
	if err != nil {
		// The root :steps value could not be resolved as a vector — for
		// example the ENTIRE value is a tagged include
		// (:steps #flow/include "steps.edn"). Includes exist (the sources
		// differ), so never claim the step is root-defined.
		return true
	}
	return index < 0
}

// localFlowStepArityDiagnostics flags every EXECUTABLE flow/step form whose
// list arity cannot be right, reusing the quote-aware reference walker so
// nested quoted forms — literal data that never executes — are not flagged.
// Severity mirrors the server's push-time behavior:
//   - more than four elements: error — the server's shape check rejects the
//     form with "flow/step expects exactly three arguments".
//   - three elements where both arguments are keywords: error — the server's
//     step-call analysis picks the form up with a nil config and rejects it
//     with config "should be a map".
//   - two elements, or three elements the server's step-call analysis skips
//     (missing keyword id): warning only — push accepts the form as a plain
//     expression and it fails first at runtime.
//
// sourceExpanded marks that src is the include-EXPANDED literal: byte offsets
// would point into the wrong place in the root file, so they are omitted —
// same rule as the packaged-step reference diagnostics. Offsets stay exact
// for include-free files.
func localFlowStepArityDiagnostics(src string, sourceExpanded bool) []flowLintDiagnostic {
	references, err := localFlowStepReferences(src)
	if err != nil {
		// The reference scan already reports its own scan-incomplete warning.
		return nil
	}
	var diagnostics []flowLintDiagnostic
	appendDiag := func(reference localFlowStepReference, severity, code, message, hint string) {
		pathID := reference.PathID
		if pathID == "" {
			pathID = "<unknown>"
		}
		diag := lintDiagnostic(severity, code, []string{":flow", pathID}, message, hint, "local")
		if !sourceExpanded {
			diag["byteOffset"] = reference.ByteOffset
		}
		diagnostics = append(diagnostics, diag)
	}
	const shapeHint = "Typed forms take step type, step id, and config map; packaged forms take step id and config map."
	for _, reference := range references {
		// Function/code step shapes are owned by
		// localFunctionStepDiagnosticsForList (function_step_arity_invalid);
		// skip them here so a malformed form is not reported twice.
		if reference.TypeToken == ":function" || reference.TypeToken == ":code" {
			continue
		}
		// Diagnostics cover plain-literal forms only; forms using reader
		// macros (discards, conditionals, syntax quotes/unquotes, metadata,
		// tagged literals, quotes) are excluded by design — a lint warning
		// must have near-zero false positives and reader semantics belong to
		// the server.
		if !reference.Plain {
			continue
		}
		// More than four elements is invalid for BOTH the packaged and the
		// typed shape regardless of what any argument resolves to, so this
		// count-based, value-independent check fires before the dynamic-type
		// bail below.
		if reference.ElementCount > 4 {
			appendDiag(reference, "error", "flow_step_arity_invalid",
				"flow/step expects exactly three arguments: step type, step id, and config map.",
				"Merge extra arguments into the single config map.")
			continue
		}
		// Two list elements are invalid for BOTH shapes regardless of what
		// the first argument resolves to (packaged needs three, typed four) —
		// count-based and value-independent like the >4 check, so it fires
		// before the dynamic bail.
		if reference.ElementCount == 2 {
			appendDiag(reference, "warning", "flow_step_missing_config",
				"flow/step is missing its config map: typed forms take type, id, and config; packaged forms take id and config.",
				shapeHint)
			continue
		}
		// A fixed non-keyword literal in the TYPE position — (flow/step nil
		// :fetch {}) — can never be a valid step type or packaged id.
		if reference.ElementCount >= 2 && reference.FirstArgNeverStepType {
			appendDiag(reference, "warning", "flow_step_invalid_type",
				"flow/step type must be a keyword: typed forms take type, id, and config; packaged forms take id and config.",
				shapeHint)
			continue
		}
		// A non-keyword FIRST argument — (flow/step kind {config}) — could
		// resolve to any packaged or typed call at runtime; the shape is
		// unknowable, so bail from all remaining shape diagnostics. Dynamic
		// three-element forms could be valid packaged calls, so they stay
		// silent.
		if reference.ElementCount >= 2 && !reference.FirstArgKeyword {
			continue
		}
		packaged := reference.StepID != ""
		switch {
		case reference.ElementCount == 3 && reference.FirstArgKeyword && reference.SecondArgKeyword:
			// (flow/step :type :id) with no config: the server rejects this at
			// push with config "should be a map". A packaged (flow/step :ns/id
			// {config}) form has a map (not keyword) second argument, so the
			// legal packaged shape never lands here.
			appendDiag(reference, "error", "flow_step_missing_config",
				"flow/step is missing its config map; the server rejects this form with: config should be a map.",
				shapeHint)
		case packaged && reference.ElementCount == 4 && reference.SecondArgNeverStepID:
			// (flow/step :ns/id {config} extra): the second argument is a
			// literal that can never be a keyword step id, so this cannot be
			// the legal typed-style invocation — it is the two-argument
			// packaged form with a stray extra argument, which the server's
			// step-call analysis skips (push accepts it; it fails at runtime).
			// AMBIGUITY RULE: a symbol or call form in the second position
			// COULD evaluate to the legal keyword step id at runtime —
			// (flow/step :ns/id step-id cfg) is syntactically identical to
			// (flow/step :ns/id cfg extra) — so those forms get no warning.
			appendDiag(reference, "warning", "flow_step_packaged_extra_argument",
				"Packaged flow/step takes step id and config map; the extra argument is invalid at runtime.",
				"Use (flow/step :ns/id {config}), or the typed shape (flow/step :ns/id :step-id {config}) with a keyword step id.")
		case !packaged && reference.ElementCount == 4 && reference.SecondArgNeverStepID:
			// (flow/step :http nil {}) / (flow/step :http "fetch" {}): the id
			// position holds a literal that can never be a keyword step id.
			// Same ambiguity rule as the packaged case: symbols and calls
			// could evaluate to a keyword, so only never-a-step-id literals
			// warn.
			appendDiag(reference, "warning", "flow_step_missing_step_id",
				"flow/step step id must be a keyword: typed forms take type, id, and config.",
				shapeHint)
		case packaged && reference.ElementCount == 3 && reference.SecondArgNeverStepID && !reference.SecondArgMap:
			// (flow/step :ns/id nil) / (flow/step :ns/id []): in the packaged
			// shape the CONFIG is the second argument, and never-step-id
			// minus the map literal is exactly the never-map literal set
			// (keyword configs are caught by the both-keywords error above).
			// Symbols and calls stay ambiguous and silent.
			appendDiag(reference, "warning", "flow_step_missing_config",
				"flow/step config must be a map: packaged forms take id and config map.",
				shapeHint)
		case packaged && reference.ElementCount == 4 && reference.SecondArgKeyword && reference.ThirdArgNeverMap:
			// (flow/step :ns/id :run nil): the explicit-step-id packaged
			// shape puts the config in the FOURTH position.
			appendDiag(reference, "warning", "flow_step_missing_config",
				"flow/step config must be a map: packaged forms with an explicit step id take id, step id, and config map.",
				shapeHint)
		case !packaged && reference.ElementCount == 4 && reference.ThirdArgNeverMap:
			// (flow/step :http :fetch nil) / (flow/step :http :fetch []): the
			// config position holds a literal that can never be a map.
			appendDiag(reference, "warning", "flow_step_missing_config",
				"flow/step config must be a map: typed forms take type, id, and config map.",
				shapeHint)
		case reference.ElementCount == 3 && !packaged && reference.SecondArgMap:
			// (flow/step :type {config}): the config map is present — the
			// missing piece is the step id. The server's step-call analysis
			// skips the form (no keyword id), so warning severity.
			appendDiag(reference, "warning", "flow_step_missing_step_id",
				"flow/step is missing its step id: typed forms take type, id, and config.",
				shapeHint)
		case reference.ElementCount == 1 || (reference.ElementCount == 3 && !packaged):
			// Under-specified shapes the server's step-call analysis skips
			// (push accepts them as plain expressions; they fail at runtime):
			// bare (flow/step) and (flow/step :ns/id). Packaged three-element
			// forms with an expression config are legal and stay clean.
			appendDiag(reference, "warning", "flow_step_missing_config",
				"flow/step is missing its config map: typed forms take type, id, and config; packaged forms take id and config.",
				shapeHint)
		}
	}
	return diagnostics
}

// localToolsExposedStepIDs collects every qualified step id that appears inside
// ANY :tools {... :steps [...]} vector anywhere in the flow source — top-level
// :agents, the :flow body, packaged-step :defaults, quoted or not.
//
// Deliberate trade for this warning-severity dead-code lint: false positives
// cost more than false negatives, so quoting semantics are ignored and a
// tools-shaped map anywhere over-suppresses the unreferenced warning rather
// than risking a false warning (or a quoting-semantics arms race) here.
// localToolsExposedStepIDs returns the ids exposed via :tools vectors plus
// whether every :tools value in the source was fully understood. When any
// value was opaque (allKnown=false), the caller cannot know which steps it
// exposes and must suppress the unreferenced warning entirely.
func localToolsExposedStepIDs(src string) (map[string]bool, bool) {
	ids := map[string]bool{}
	allKnown := true
	collectToolsExposedStepIDs(src, 0, len(src), ids, &allKnown)
	return ids, allKnown
}

func collectToolsExposedStepIDs(src string, start, end int, ids map[string]bool, allKnown *bool) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= end || i >= len(src) {
		return
	}
	switch src[i] {
	case '\'', '`', '@':
		collectToolsExposedStepIDs(src, i+1, end, ids, allKnown)
		return
	case '~':
		next := i + 1
		if next < end && next < len(src) && src[next] == '@' {
			next++
		}
		collectToolsExposedStepIDs(src, next, end, ids, allKnown)
		return
	}
	if strings.HasPrefix(src[i:], "#(") {
		collectToolsExposedStepIDsInSpans(src, ids, parseListSpans(src, i+1), allKnown)
		return
	}
	if strings.HasPrefix(src[i:], "#{") {
		if spans, _, err := parseClojureSetElements(src, i); err == nil {
			collectToolsExposedStepIDsInSpans(src, ids, spans, allKnown)
		}
		return
	}
	if src[i] == '#' {
		// Any other dispatch form — tagged literals like #my/tag {...},
		// namespaced maps, regexes, var quotes — may hide a :tools entry the
		// collector cannot see: the exposure set is unknowable.
		*allKnown = false
		return
	}
	switch src[i] {
	case '{':
		entries, _, err := parseClojureMapEntries(src, i)
		if err != nil {
			// An unparseable map (for example a #?@ splice among its entries)
			// may hide a :tools entry: the exposure set is unknowable.
			*allKnown = false
			return
		}
		for _, entry := range entries {
			// Exact namespace-less match: :custom/tools is a different key
			// and must not count as tool exposure.
			if strings.TrimSpace(entry.KeyToken) == ":tools" {
				if !collectToolsStepsVectorIDs(src, entry, ids) {
					*allKnown = false
				}
			}
			collectToolsExposedStepIDs(src, entry.ValueStart, entry.ValueEnd, ids, allKnown)
		}
	case '[':
		if spans, _, err := parseClojureVectorElements(src, i); err == nil {
			collectToolsExposedStepIDsInSpans(src, ids, spans, allKnown)
		}
	case '(':
		collectToolsExposedStepIDsInSpans(src, ids, parseListSpans(src, i), allKnown)
	}
}

func collectToolsExposedStepIDsInSpans(src string, ids map[string]bool, spans []clojureFormSpan, allKnown *bool) {
	for _, span := range spans {
		collectToolsExposedStepIDs(src, span.Start, span.End, ids, allKnown)
	}
}

func parseListSpans(src string, start int) []clojureFormSpan {
	spans, _, err := parseClojureListElements(src, start)
	if err != nil {
		return nil
	}
	return spans
}

// collectToolsStepsVectorIDs reads one :tools entry and records its
// {:steps [...]} ids. It reports whether the value was fully understood: a
// value that is not a plain map/vector shape after ONE simple reader-quote
// unwrap is opaque, and callers must then treat every packaged step as
// potentially exposed (suppress the warning; over-suppression is the accepted
// direction for this warning-severity dead-code lint).
func collectToolsStepsVectorIDs(src string, toolsEntry clojureMapEntry, ids map[string]bool) bool {
	valueStart, ok := unwrapSingleReaderQuote(src, toolsEntry.ValueStart)
	if !ok || valueStart >= len(src) || src[valueStart] != '{' {
		return false
	}
	entries, _, err := parseClojureMapEntries(src, valueStart)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		// Exact namespace-less match: :custom/steps is a different key.
		if strings.TrimSpace(entry.KeyToken) != ":steps" {
			continue
		}
		stepsStart, ok := unwrapSingleReaderQuote(src, entry.ValueStart)
		if !ok || stepsStart >= len(src) || src[stepsStart] != '[' {
			return false
		}
		spans, stepsEnd, err := parseClojureVectorElements(src, stepsStart)
		if err != nil {
			return false
		}
		// Mirror of the stepsUnknowable rule: the shared parser cannot honor
		// #_ #_ chain consumption, so ANY discard in the raw vector text makes
		// the keyword set unknowable → opaque.
		if stepsEnd > stepsStart && stepsEnd <= len(src) && strings.Contains(stripClojureStringLiterals(src[stepsStart:stepsEnd]), "#_") {
			return false
		}
		for _, span := range spans {
			id, ok := localQualifiedStepIDFromForm(src, span)
			if !ok {
				// A symbol or call element could name any step at runtime:
				// the exposure set is incomplete → opaque.
				return false
			}
			ids[id] = true
		}
	}
	return true
}

// unwrapSingleReaderQuote performs the one simple unwrap the tools-value scan
// allows: a single leading reader quote (') or one explicit (quote ...) /
// (clojure.core/quote ...) wrapper — the two spellings must behave
// identically. Anything more exotic makes the value opaque for the caller.
func unwrapSingleReaderQuote(src string, start int) (int, bool) {
	i, ok := clojureActiveFormStart(src, start)
	if !ok || i >= len(src) {
		return i, ok
	}
	if src[i] == '\'' {
		return clojureActiveFormStart(src, i+1)
	}
	if src[i] == '(' {
		if elements := parseListSpans(src, i); len(elements) >= 2 {
			if head := clojureFormToken(src, elements[0]); head == "quote" || head == "clojure.core/quote" {
				return clojureActiveFormStart(src, elements[1].Start)
			}
		}
	}
	return i, ok
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
			if err == nil {
				diagnostics = append(diagnostics, localFunctionStepDiagnosticsForList(src, elements, i, allowBareInput, pulledLegacyInputSteps)...)
			}
		}
		i++
	}
	return diagnostics
}

func localFunctionStepDiagnosticsForList(src string, elements []clojureFormSpan, listStart int, allowBareInput bool, pulledLegacyInputSteps map[string]bool) []flowLintDiagnostic {
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
