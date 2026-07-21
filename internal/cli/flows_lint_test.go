package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/authstore"
)

func runFlowLintLocalOnlyForLiteral(t *testing.T, flowLiteral string) (map[string]any, error, string) {
	t.Helper()
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}
	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})
	err := cmd.Execute()
	var body map[string]any
	if decodeErr := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); decodeErr != nil {
		t.Fatalf("decode output: %v\n%s", decodeErr, out.String())
	}
	return body, err, out.String()
}

func requireFlowLintDiagnosticCodes(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	codes := map[string]bool{}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if code, _ := item["code"].(string); code != "" {
			codes[code] = true
		}
	}
	for _, code := range want {
		if !codes[code] {
			t.Fatalf("expected diagnostic code %q, got items=%#v", code, items)
		}
	}
}

func rejectFlowLintDiagnosticCodes(t *testing.T, body map[string]any, reject ...string) {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	rejected := map[string]bool{}
	for _, code := range reject {
		rejected[code] = true
	}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if code, _ := item["code"].(string); rejected[code] {
			t.Fatalf("unexpected diagnostic code %q, got item=%#v all=%#v", code, item, items)
		}
	}
}

func TestFlowsLintLocalOnlyReportsMissingPackagedStepReference(t *testing.T) {
	flowLiteral := `{:slug :missing-step-reference
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps [{:id :tools/declared :type :function :description "Declared"}]
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(let [declared (flow/step :tools/declared :run {})
              missing (flow/step :tools/missing :run {})]
          [declared missing])}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected missing packaged step lint error\n%s", output)
	}
	requireFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyAcceptsDeclaredPackagedStepReference(t *testing.T) {
	flowLiteral := `{:slug :declared-step-reference
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps [{:id :tools/declared :type :function :description "Declared"}]
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(flow/step :tools/declared :run {})}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("declared packaged step should lint successfully: %v\n%s", err, output)
	}
	rejectFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyAcceptsDeclaredAgentReference(t *testing.T) {
	flowLiteral := `{:slug :declared-agent-reference
 :concurrency {:type :singleton :on-new-version :coexist}
 :agents [{:id :review/security :description "Security reviewer"}]
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(flow/step :review/security :run {})}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("declared agent should satisfy a flow/step reference: %v\n%s", err, output)
	}
	rejectFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyIgnoresNestedQuotedStepData(t *testing.T) {
	flowLiteral := `{:slug :nested-quoted-step-data
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps [{:id :tools/declared :type :function :description "Declared"}]
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(let [literal '(flow/step :tools/missing :run {})]
          (flow/step :tools/declared :run {}))}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("nested quoted step data should not be treated as executable: %v\n%s", err, output)
	}
	rejectFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyFindsStepReferencesInAnonymousFunctions(t *testing.T) {
	flowLiteral := `{:slug :anonymous-step-reference
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps []
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(map #(flow/step :tools/missing :run %) [1])}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected anonymous-function step reference to be reported\n%s", output)
	}
	requireFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyFindsStepReferencesInSetLiterals(t *testing.T) {
	flowLiteral := `{:slug :set-step-reference
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps []
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '#{(flow/step :tools/missing :run {})}}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected set-literal step reference to be reported\n%s", output)
	}
	requireFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyIgnoresExplicitQuoteForms(t *testing.T) {
	flowLiteral := `{:slug :explicit-quote-step-data
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps []
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(let [literal (quote (flow/step :tools/missing :run {}))]
          (flow/input))}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("explicitly quoted step data should not be treated as executable: %v\n%s", err, output)
	}
	rejectFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyFindsStepReferencesInTopLevelExplicitQuote(t *testing.T) {
	for _, quoteForm := range []string{"quote", "clojure.core/quote"} {
		flowLiteral := fmt.Sprintf(`{:slug :top-level-explicit-quote-%s
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps []
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow (%s (flow/step :tools/missing :run {}))}
`, strings.ReplaceAll(quoteForm, ".", "-"), quoteForm)
		body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
		if err == nil {
			t.Fatalf("expected top-level %s step reference to be reported\n%s", quoteForm, output)
		}
		requireFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
	}
}

func TestFlowsLintLocalOnlyReportsMissingPackagedStepWhenStepsKeyIsAbsent(t *testing.T) {
	flowLiteral := `{:slug :missing-step-key
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :schedules []
 :flow '(flow/step :tools/missing :run {})}
`
	body, err, output := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected missing packaged step lint error when :steps is absent\n%s", output)
	}
	requireFlowLintDiagnosticCodes(t, body, "missing_packaged_step_reference")
}

func TestFlowsLintLocalOnlyReportsDelimiterErrors(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	if err := os.WriteFile(flowFile, []byte("{:slug :bad\n :flow '(identity 1)\n"), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}

	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %#v", body)
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected diagnostics, got %#v", body)
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["code"].(string); got != "clojure_delimiters_invalid" {
		t.Fatalf("expected delimiter diagnostic, got %#v", first)
	}
}

func TestFlowsLintLocalOnlyReportsMismatchedDelimiterErrors(t *testing.T) {
	flowLiteral := `{:slug :mismatched-delimiter
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(identity 1)]
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected lint error for mismatched delimiter\n%s", stdout)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %#v", body)
	}
	requireFlowLintDiagnosticCodes(t, body, "clojure_delimiters_invalid")
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	first, _ := items[0].(map[string]any)
	message, _ := first["message"].(string)
	if !strings.Contains(message, "replaced=1") {
		t.Fatalf("expected one mismatched delimiter replacement, got %#v", first)
	}
	hint, _ := first["hint"].(string)
	if !strings.Contains(hint, "flows paren-repair --write") {
		t.Fatalf("expected paren-repair hint, got %#v", first)
	}
}

func TestFlowsLintLocalOnlyReportsReaderShapeErrors(t *testing.T) {
	flowLiteral := `{:slug :bad
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected lint error\n%s", stdout)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %#v", body)
	}
	data, _ := body["data"].(map[string]any)
	if valid, _ := data["valid"].(bool); valid {
		t.Fatalf("expected valid=false, got %#v", data)
	}
	requireFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid")
	items, _ := data["diagnostics"].([]any)
	first, _ := items[0].(map[string]any)
	if message, _ := first["message"].(string); !strings.Contains(message, "missing map value") {
		t.Fatalf("expected missing map value message, got %#v", first)
	}
	meta, _ := body["meta"].(map[string]any)
	if _, ok := meta["nextCommands"]; ok {
		t.Fatalf("malformed source should not suggest nextCommands, got %#v", meta)
	}
}

func TestFlowsLintLocalOnlyAcceptsMultilineClojureStrings(t *testing.T) {
	flowLiteral := `{:slug :multiline
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(fn [] "line one
line two")}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("valid multiline Clojure string should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid", "clojure_syntax_invalid")
}

func TestFlowsLintLocalOnlyAcceptsReaderConditionalSpliceInMap(t *testing.T) {
	flowLiteral := `{:slug :reader-conditional-splice
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(fn [] {:base true #?@(:clj [:extra 1])})}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("valid reader-conditional splice should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid", "clojure_syntax_invalid")
}

func TestFlowsLintLocalOnlyPreservesMapParityAroundReaderConditionalSplice(t *testing.T) {
	flowLiteral := `{:slug :reader-conditional-splice-odd
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(fn [] {:base true #?@(:clj [:extra 1]) :unmatched})}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected odd map after reader-conditional splice to fail\n%s", stdout)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %#v", body)
	}
	requireFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid")
}

func TestFlowsLintLocalOnlyExpandsIncludesBeforeReaderShapeValidation(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	includeFile := filepath.Join(tmpDir, "common-fields.edn")
	if err := os.WriteFile(includeFile, []byte(`:slug :included-reader-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}`), 0o644); err != nil {
		t.Fatalf("write include file: %v", err)
	}
	flowLiteral := `{:flow '(identity) #flow/include "common-fields.edn"}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("valid included flow should pass local lint: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid", "clojure_syntax_invalid")
}

func TestFlowsLintLocalOnlyDoesNotSuggestRootRepairForUnbalancedInclude(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	includeFile := filepath.Join(tmpDir, "broken-fields.edn")
	if err := os.WriteFile(includeFile, []byte(":slug :broken\n :concurrency {"), 0o644); err != nil {
		t.Fatalf("write include file: %v", err)
	}
	flowLiteral := `{:flow '(identity) #flow/include "broken-fields.edn"}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected unbalanced included source to fail\n%s", out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected included-source diagnostic, got %#v", body)
	}
	first, _ := items[0].(map[string]any)
	hint, _ := first["hint"].(string)
	if !strings.Contains(hint, "included source") {
		t.Fatalf("expected included-source hint, got %#v", first)
	}
	if strings.Contains(hint, "paren-repair --write --file "+flowFile) {
		t.Fatalf("must not suggest repairing the root file for an included-source error: %#v", first)
	}
}

func TestFlowsLintLocalOnlyAcceptsDelimiterCharacterLiteral(t *testing.T) {
	flowLiteral := `{:slug :delimiter-character
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(fn [] \])}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("valid delimiter character literal should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid", "clojure_syntax_invalid")
}

func TestFlowsParenRepairDryRunDoesNotWriteByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	original := "{:slug :bad\n :flow '(identity 1)\n"
	if err := os.WriteFile(flowFile, []byte(original), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsParenRepairCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("paren-repair dry run failed: %v\n%s", err, out.String())
	}
	after, err := os.ReadFile(flowFile)
	if err != nil {
		t.Fatalf("read flow file: %v", err)
	}
	if string(after) != original {
		t.Fatalf("dry run rewrote file: %q", string(after))
	}

	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected one repair result, got %#v", data)
	}
	first, _ := results[0].(map[string]any)
	if first["changed"] != true || first["written"] != false {
		t.Fatalf("expected changed=true and written=false, got %#v", first)
	}
}

func TestFlowsLintLocalOnlyWarnsOnUnboundedRange(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :range-risk
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(take 5 (range))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint returned error for warning-only diagnostics: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	meta, _ := body["meta"].(map[string]any)
	nextCommands, _ := meta["nextCommands"].([]any)
	if len(nextCommands) == 0 {
		t.Fatalf("expected warning-only lint to include next commands, got meta=%#v", meta)
	}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "sandbox_unbounded_range" && item["severity"] == "warning" {
			return
		}
	}
	t.Fatalf("expected sandbox_unbounded_range warning, got %#v", items)
}

func TestFlowsLintLocalOnlyRejectsUnsupportedVisualThreading(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :visual-threading-risk
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)
              payload (cond-> {:path (:path input)}
                        (:content input) (assoc :content (:content input)))]
          payload)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" && item["form"] == "cond->" {
			if item["severity"] != "error" {
				t.Fatalf("expected error diagnostic, got %#v", item)
			}
			return
		}
	}
	t.Fatalf("expected unsupported_visual_flow_form diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlySkipsReaderDiscardedUnsupportedVisualThreading(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :discarded-visual-threading
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)
              #_(cond-> {:path (:path input)}
                  (:content input) (assoc :content (:content input)))
              payload {:path (:path input)}]
          payload)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should ignore reader-discarded visual threading form: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" {
			t.Fatalf("reader-discarded cond-> produced visual-flow diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyRejectsUnsupportedVisualThreadingInSyntaxQuotedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := strings.Join([]string{
		"{:slug :syntax-quoted-visual-threading-risk",
		" :concurrency {:type :singleton :on-new-version :coexist}",
		" :invocations {:default {:inputs []}}",
		" :interfaces {:manual [{:id :run :label \"Run\" :invocation :default}]}",
		" :flow `(let [input (flow/input)",
		"              payload (cond-> {:path (:path input)}",
		"                        (:content input) (assoc :content (:content input)))]",
		"          payload)}",
		"",
	}, "\n")
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" && item["form"] == "cond->" {
			return
		}
	}
	t.Fatalf("expected syntax-quoted unsupported_visual_flow_form diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyRejectsUnsupportedVisualThreadingInIncludedFlow(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	includedFlow := filepath.Join(tmpDir, "flow-body.clj")
	if err := os.WriteFile(includedFlow, []byte(`'(let [input (flow/input)
       payload (cond-> {:path (:path input)}
                 (:content input) (assoc :content (:content input)))]
   payload)`), 0o644); err != nil {
		t.Fatalf("write included flow file: %v", err)
	}
	flowLiteral := `{:slug :included-visual-threading-risk
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow #flow/include "flow-body.clj"}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" && item["form"] == "cond->" {
			return
		}
	}
	t.Fatalf("expected included unsupported_visual_flow_form diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyRejectsUnsupportedVisualThreadingInReaderConditionalFlow(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-conditional-visual-threading-risk
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow #?(:cljs '(let [input (flow/input)]
                   (-> input
                       (assoc :cljs true)))
          :clj '(let [input (flow/input)
                      payload (cond-> {:path (:path input)}
                                (:content input) (assoc :content (:content input)))]
                  payload))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" && item["form"] == "cond->" {
			return
		}
	}
	t.Fatalf("expected reader-conditional unsupported_visual_flow_form diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlySkipsInactiveReaderConditionalFlowBranch(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-conditional-inactive-visual-threading
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)
              payload #?(:cljs (cond-> {}
                                  true (assoc :x 1))
                          :clj {})]
          (assoc payload :path (:path input)))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should ignore inactive reader-conditional visual threading form: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" {
			t.Fatalf("inactive reader-conditional branch produced visual-flow diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyAllowsUnsupportedVisualFormsInsideQuotedFunctionCode(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :function-threading-ok
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)
              payload (flow/step :function :build-payload
                                 {:input input
                                  :code '(fn [input]
                                           (cond-> {:path (:path input)}
                                             (:content input) (assoc :content (:content input))))})]
          payload)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should not reject quoted function code cond->: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" {
			t.Fatalf("function :code cond-> produced visual-flow diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyAllowsUnsupportedVisualFormsInsideSyntaxQuotedFunctionCode(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := strings.Join([]string{
		"{:slug :function-syntax-quote-threading-ok",
		" :concurrency {:type :singleton :on-new-version :coexist}",
		" :invocations {:default {:inputs []}}",
		" :interfaces {:manual [{:id :run :label \"Run\" :invocation :default}]}",
		" :flow '(let [input (flow/input)",
		"              payload (flow/step :function :build-payload",
		"                                 {:input input",
		"                                  :code `(fn [input]",
		"                                           (cond-> {:path (:path input)}",
		"                                             (:content input) (assoc :content (:content input))))})]",
		"          payload)}",
		"",
	}, "\n")
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should not reject syntax-quoted function code cond->: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "unsupported_visual_flow_form" {
			t.Fatalf("syntax-quoted function :code cond-> produced visual-flow diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyRejectsNilConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :nil-concurrency
 :concurrency nil
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "invalid_required_field" && item["severity"] == "error" {
			path, _ := item["path"].([]any)
			if len(path) != 1 || path[0] != ":concurrency" {
				t.Fatalf("expected :concurrency path, got %#v", item)
			}
			return
		}
	}
	t.Fatalf("expected invalid_required_field diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyAllowsNilConcurrencyInsideFlowCode(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :nested-nil-concurrency
 :description "Example text mentioning :concurrency nil without changing the top-level field."
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)]
          ;; Internal payload normalization may clear a nested field.
          (assoc input :concurrency nil))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint returned error for valid top-level concurrency: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "invalid_required_field" {
			t.Fatalf("nested :concurrency nil produced invalid_required_field diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyRejectsInvalidInvocationAndInterfaceShapes(t *testing.T) {
	flowLiteral := `{:slug :bad-authoring-shapes
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations [{:id :default :inputs [{:id :repo}]}]
 :interfaces {:manual {:id :run :invocation :default}}
 :flow '(let [input (flow/input)] input)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected lint error\n%s", stdout)
	}
	requireFlowLintDiagnosticCodes(t, body,
		"invalid_invocations_shape",
		"invalid_interface_category_shape")
}

func TestFlowsLintLocalOnlyAcceptsNilInvocationsAndInterfaces(t *testing.T) {
	flowLiteral := `{:slug :minimal-authoring-shapes
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations nil
 :interfaces nil
 :flow '(let [input (flow/input)] input)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("lint should accept documented nil invocation/interface shape: %v\n%s", err, stdout)
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "invalid_invocations_shape" {
			t.Fatalf("unexpected nil invocations diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyAcceptsDocumentedMinimalFunctionStepShape(t *testing.T) {
	flowLiteral := `{:slug :my-flow
 :name "My Flow"
 :description "..."
 :tags ["example"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :requires nil
 :templates nil
 :functions nil
 :invocations nil
 :interfaces nil
 :schedules nil
 :flow '(let [input (flow/input)]
          (flow/step :function :do {:code '(fn [input] input)
                                     :input {:input input}}))}
`
	if _, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral); err != nil {
		t.Fatalf("lint should accept documented minimal function step shape: %v\n%s", err, stdout)
	}
}

func TestFlowsLintLocalOnlyRejectsUnknownInterfaceInvocation(t *testing.T) {
	flowLiteral := `{:slug :missing-interface-invocation
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :invocation :missing}]}
 :flow '(let [input (flow/input)] input)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected lint error\n%s", stdout)
	}
	requireFlowLintDiagnosticCodes(t, body, "unknown_interface_invocation")
}

func TestFlowsLintLocalOnlyAcceptsNonIdentifierMcpToolName(t *testing.T) {
	flowLiteral := `{:slug :mcp-tool-name
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:mcp [{:tool-name "github.tree.commit/v1" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("lint should accept nonblank MCP tool names: %v\n%s", err, stdout)
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "invalid_interface_id" {
			t.Fatalf("unexpected MCP tool-name diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyRejectsFunctionStepAuthoringShapes(t *testing.T) {
	flowLiteral := `{:slug :bad-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :shape :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :shape {:ref :shape :input [input]}
                     :code '(fn [_] nil)))}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("expected lint error\n%s", stdout)
	}
	requireFlowLintDiagnosticCodes(t, body,
		"function_step_arity_invalid",
		"function_step_input_shape_invalid")
}

// A bare symbol :input (a runtime value that resolves to a map at execution
// time) is valid and accepted by the server, so local lint must accept it too.
// Regression for the false positive where existing, server-valid flows like
// youtube-video-search-scraper (:normalize-input, :assert-valid-input) were
// flagged with function_step_input_shape_invalid on a clean pull+push.
func TestFlowsLintLocalOnlyAcceptsNewBareReferencedFunctionStepInput(t *testing.T) {
	flowLiteral := `{:slug :new-bare-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize-input-payload
              :language :clojure
              :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input-payload
                     {:ref :normalize-input-payload
                      :input input}))}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("bare symbol function input should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
}

// A runtime expression :input (for example a call that returns a map) is also a
// valid runtime value and must be accepted.
func TestFlowsLintLocalOnlyAcceptsExpressionFunctionStepInput(t *testing.T) {
	flowLiteral := `{:slug :expression-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :assert-valid :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :assert-valid-input
                     {:ref :assert-valid
                      :input (select-keys input [:id])}))}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("expression function input should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
}

// A literal that can never be a map (vector, string, set, char, keyword, or a
// self-evaluating number/nil/boolean) is still flagged so the obvious authoring
// mistake is caught before the runtime map-of schema rejects it.
func TestFlowsLintLocalOnlyRejectsNonMapLiteralFunctionStepInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"vector", "[1 2 3]"},
		{"string", "\"text\""},
		{"set", "#{1 2 3}"},
		{"regex", "#\"x\""},
		{"anon-fn", "#(identity %)"},
		{"symbolic", "##Inf"},
		{"var-quote", "#'input"},
		{"number", "42"},
		{"negative-number", "-5"},
		{"decimal", "3.14"},
		{"nil", "nil"},
		{"boolean", "true"},
		{"keyword", ":id"},
		{"char", "\\a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flowLiteral := `{:slug :non-map-literal-function-input
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input {:ref :normalize :input ` + tc.input + `}))}
`
			body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
			if err == nil {
				t.Fatalf("expected lint error for non-map literal :input\n%s", stdout)
			}
			requireFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
		})
	}
}

// Symbols (including sign-prefixed names) and call forms are runtime values that
// may resolve to a map, so they must NOT be flagged as non-map literals.
func TestFlowsLintLocalOnlyAcceptsSymbolLikeFunctionStepInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"plain-symbol", "input"},
		{"dash-prefixed-symbol", "-input"},
		{"plus-prefixed-symbol", "+config"},
		{"namespaced-symbol", "my.ns/data"},
		{"call-form", "(select-keys input [:id])"},
		{"map-literal", "{:rows input}"},
		{"deref", "@input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flowLiteral := `{:slug :symbol-like-function-input
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input {:ref :normalize :input ` + tc.input + `}))}
`
			body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
			if err != nil {
				t.Fatalf("symbol/expression :input %q should pass local lint: %v\n%s", tc.input, err, stdout)
			}
			rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
		})
	}
}

// Under a quote the value is literal data, so anything but a map literal is
// provably not a map; quoting a constant must not smuggle a non-map :input past
// local lint. A quoted map literal stays accepted.
func TestFlowsLintLocalOnlyRejectsQuotedNonMapLiteralFunctionStepInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		flagged bool
	}{
		{"quoted-vector", "'[input]", true},
		{"quoted-nil", "'nil", true},
		{"quoted-keyword", "':id", true},
		{"quoted-symbol", "'input", true},
		{"quoted-list", "'(hash-map :a 1)", true},
		{"quoted-number", "'42", true},
		{"syntax-quoted-vector", "`[input]", true},
		{"quote-form", "(quote [input])", true},
		{"quoted-map", "'{:rows input}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flowLiteral := `{:slug :quoted-literal-function-input
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input {:ref :normalize :input ` + tc.input + `}))}
`
			body, _, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
			if tc.flagged {
				requireFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
			} else {
				rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
				_ = stdout
			}
		})
	}
}

// Metadata (legacy #^meta and modern ^meta) is stripped by the reader, so the
// underlying value must be classified: a metadata-wrapped non-map literal is
// flagged, a metadata-wrapped map or symbol is not.
func TestFlowsLintLocalOnlyClassifiesMetadataWrappedFunctionStepInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		flagged bool
	}{
		{"legacy-meta-vector", "#^String [input]", true},
		{"modern-meta-vector", "^String [input]", true},
		{"legacy-meta-map", "#^Foo {:rows input}", false},
		{"modern-meta-symbol", "^:tag input", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flowLiteral := `{:slug :metadata-wrapped-function-input
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input {:ref :normalize :input ` + tc.input + `}))}
`
			body, _, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
			if tc.flagged {
				requireFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
			} else {
				rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
				_ = stdout
			}
		})
	}
}

// Tagged reader literals run a data reader that could yield a map, so local lint
// defers them rather than flagging (unlike non-tagged reader literals such as
// #"..." or #(...), which have fixed non-map semantics and are rejected above).
func TestFlowsLintLocalOnlyDefersTaggedLiteralFunctionStepInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"inst", `#inst "2020-01-01"`},
		{"uuid", `#uuid "00000000-0000-0000-0000-000000000000"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flowLiteral := `{:slug :tagged-literal-function-input
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input {:ref :normalize :input ` + tc.input + `}))}
`
			body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
			if err != nil {
				t.Fatalf("tagged literal :input %q should pass local lint: %v\n%s", tc.input, err, stdout)
			}
			rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
		})
	}
}

// Pulled source that carries a non-map literal :input on a recorded legacy step
// is downgraded to a non-blocking warning instead of a hard error.
func TestFlowsLintLocalOnlyWarnsForPulledLegacyFunctionStepInputShape(t *testing.T) {
	flowLiteral := markPulledFlowSource(`{:slug :pulled-legacy-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize-input-payload
              :language :clojure
              :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input-payload
                     {:ref :normalize-input-payload
                      :input [input]}))}
`)
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("pulled legacy function input should warn without blocking local lint: %v\n%s", err, stdout)
	}
	requireFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_step_input_shape_invalid" && item["severity"] != "warning" {
			t.Fatalf("expected compatibility diagnostic to be a warning: %#v", item)
		}
	}
}

// A pulled source whose function step uses a bare symbol :input is simply valid:
// it is accepted with no diagnostic at all (not even the compatibility warning).
func TestFlowsLintLocalOnlyAcceptsPulledBareSymbolFunctionStepInput(t *testing.T) {
	flowLiteral := markPulledFlowSource(`{:slug :pulled-bare-symbol-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize-input-payload
              :language :clojure
              :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :normalize-input-payload
                     {:ref :normalize-input-payload
                      :input input}))}
`)
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("pulled bare symbol function input should pass local lint: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid")
}

func TestFlowsLintLocalOnlyRejectsNewBareInputStepInPulledSource(t *testing.T) {
	flowLiteral := markPulledFlowSource(`{:slug :edited-pulled-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :legacy :language :clojure :code '(fn [input] input)}
             {:id :new-step :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)]
          (flow/step :function :legacy {:ref :legacy :input [input]}))}
`)
	flowLiteral = strings.Replace(flowLiteral,
		`(flow/step :function :legacy {:ref :legacy :input [input]}))}`,
		`(flow/step :function :legacy {:ref :legacy :input [input]})
          (flow/step :function :new-step {:ref :new-step :input [input]}))}`,
		1,
	)

	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("new bare input step in pulled source should fail local lint\n%s", stdout)
	}

	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	severities := map[string]string{}
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] != "function_step_input_shape_invalid" {
			continue
		}
		path, _ := item["path"].([]any)
		if len(path) >= 2 {
			step, _ := path[1].(string)
			severities[step], _ = item["severity"].(string)
		}
	}
	if severities[":legacy"] != "warning" || severities[":new-step"] != "error" {
		t.Fatalf("expected only the recorded legacy step to warn, got %#v in %#v", severities, items)
	}
}

func TestFlowsLintLocalOnlyIgnoresReaderDiscardedFunctionStepAuthoringShapes(t *testing.T) {
	flowLiteral := `{:slug :discarded-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :active :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)
              #_(flow/step :function :old {:ref :old :input input}
                           :code '(fn [_] nil))]
          (flow/step :function :active {:ref :active :input {:input input}}))}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("lint should ignore reader-discarded function step shape: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body,
		"function_step_arity_invalid",
		"function_step_input_shape_invalid")
}

func TestFlowsLintLocalOnlyIgnoresInactiveReaderConditionalFunctionStepAuthoringShapes(t *testing.T) {
	flowLiteral := `{:slug :inactive-reader-conditional-function-step-shape
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :active :language :clojure :code '(fn [input] input)}]
 :flow '(let [input (flow/input)
              result #?(:cljs (flow/step :function :old {:ref :old :input input}
                                          :code '(fn [_] nil))
                        :clj (flow/step :function :active {:ref :active :input {:input input}}))]
          result)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("lint should ignore inactive reader-conditional function step shape: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body,
		"function_step_arity_invalid",
		"function_step_input_shape_invalid")
}

func TestFlowsLintLocalOnlyReportsFunctionCodeStringSyntaxErrors(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :bad-function-code
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code "(fn [input]\n  (assoc input :ok true)"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_invalid" && item["severity"] == "error" {
			path, _ := item["path"].([]any)
			if len(path) < 3 || path[1] != ":build-plan" {
				t.Fatalf("expected function id in path, got %#v", item)
			}
			return
		}
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyReportsFunctionCodeStringSyntaxErrorsAfterTopLevelReaderPrefixes(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#_ {:ignored true}
^:breyta/flow
{:slug :bad-function-code-with-prefixes
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code "(fn [input]\n  (assoc input :ok true)"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_invalid":
			path, _ := item["path"].([]any)
			if len(path) < 3 || path[1] != ":build-plan" {
				t.Fatalf("expected function id in path, got %#v", item)
			}
			return
		case "function_code_string_scan_incomplete":
			t.Fatalf("did not expect fallback scanner warning, got %#v", item)
		}
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyBestEffortScansCodeStringsInTopLevelReaderConditional(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#?(:clj
   {:slug :bad-function-code-with-reader-conditional
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :functions [{:id :build-plan
                 :code "(fn [input]\n  (assoc input :ok true)"}]
    :flow '(let [input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning, sawCodeError bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_scan_incomplete" && item["severity"] == "warning" {
			sawFallbackWarning = true
		}
		if item["code"] == "function_code_string_invalid" && item["severity"] == "error" {
			sawCodeError = true
		}
	}
	if !sawFallbackWarning || !sawCodeError {
		t.Fatalf("expected fallback warning and code-string error, got %#v", items)
	}
}

func TestFlowsLintLocalOnlyBestEffortIgnoresNonFunctionCodeStrings(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#?(:clj
   {:slug :reader-conditional-with-config-code
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :metadata {:code "(fn [input]\n  (assoc input :not-a-function true)"}
    :functions [{:id :build-plan
                 :code "(fn [input]\n  (assoc input :ok true))"}]
    :flow '(let [input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should not fail on non-function :code string in fallback mode: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_scan_incomplete":
			if item["severity"] == "warning" {
				sawFallbackWarning = true
			}
		case "function_code_string_invalid":
			t.Fatalf("non-function :code string produced lint-blocking diagnostic: %#v", item)
		}
	}
	if !sawFallbackWarning {
		t.Fatalf("expected fallback warning, got %#v", items)
	}
}

func TestFlowsLintLocalOnlyBestEffortSkipsInactiveReaderForms(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#_ {:functions [{:id :discarded
                  :code "(fn [input]\n  (assoc input :discarded true)"}]}
#?(:cljs
   {:slug :inactive-reader-branch
    :functions [{:id :cljs-only
                 :code "(fn [input]\n  (assoc input :cljs true)"}]
    :flow '(identity {})}
   :clj
   {:slug :reader-conditional-with-inactive-forms
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :functions [{:id :build-plan
                 :code "(fn [input]\n  (assoc input :ok true))"}]
    :flow '(let [input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should not fail on inactive reader forms in fallback mode: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_scan_incomplete":
			if item["severity"] == "warning" {
				sawFallbackWarning = true
			}
		case "function_code_string_invalid":
			t.Fatalf("inactive reader form produced lint-blocking diagnostic: %#v", item)
		}
	}
	if !sawFallbackWarning {
		t.Fatalf("expected fallback warning, got %#v", items)
	}
}

func TestFlowsLintLocalOnlyReadsReaderConditionalFunctionsValue(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-conditional-functions
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions #?(:cljs [{:id :cljs-only
                       :code "(fn [input]\n  (assoc input :cljs true))"}]
               :clj [{:id :build-plan
                      :code "(fn [input]\n  (assoc input :ok true)"}])
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_invalid":
			path, _ := item["path"].([]any)
			if len(path) < 3 || path[1] != ":build-plan" {
				t.Fatalf("expected active :clj function id in path, got %#v", item)
			}
			return
		case "function_code_string_scan_incomplete":
			t.Fatalf("did not expect fallback scanner warning, got %#v", item)
		}
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyBestEffortReadsReaderConditionalFunctionsValue(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#?(:clj
   {:slug :top-level-reader-conditional-functions
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :functions #?(:cljs [{:id :cljs-only
                          :code "(fn [input]\n  (assoc input :cljs true))"}]
                  :clj [{:id :build-plan
                         :code "(fn [input]\n  (assoc input :ok true)"}])
    :flow '(let [input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning, sawCodeError bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_scan_incomplete" && item["severity"] == "warning" {
			sawFallbackWarning = true
		}
		if item["code"] == "function_code_string_invalid" && item["severity"] == "error" {
			path, _ := item["path"].([]any)
			if len(path) < 3 || path[1] != ":build-plan" {
				t.Fatalf("expected active :clj function id in path, got %#v", item)
			}
			sawCodeError = true
		}
	}
	if !sawFallbackWarning || !sawCodeError {
		t.Fatalf("expected fallback warning and code-string error, got %#v", items)
	}
}

func TestFlowsLintLocalOnlyReadsReaderConditionalFunctionEntries(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-conditional-function-entries
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [#?(:cljs {:id :cljs-only
                       :code "(fn [input]\n  (assoc input :cljs true)"}
                :clj {:id :build-plan
                      :code "(fn [input]\n  (assoc input :ok true))"})]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should ignore inactive reader-conditional function entry: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_invalid" || item["code"] == "function_code_string_scan_incomplete" {
			t.Fatalf("unexpected reader-conditional function diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyReportsReaderConditionalCodeValue(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-conditional-code-value
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code #?(:cljs "(fn [input]\n  (assoc input :cljs true))"
                       :clj "(fn [input]\n  (assoc input :ok true)")}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_invalid":
			path, _ := item["path"].([]any)
			if len(path) < 3 || path[1] != ":build-plan" {
				t.Fatalf("expected active code value path, got %#v", item)
			}
			return
		case "function_code_string_scan_incomplete":
			t.Fatalf("did not expect fallback scanner warning, got %#v", item)
		}
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyAcceptsVarQuoteInFunctionCodeStrings(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :var-quote-function-code
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code "(fn [input]\n  {:handler #'my.ns/f\n   :input input})"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint returned error for valid var-quote code string: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_invalid" {
			t.Fatalf("unexpected function_code_string_invalid diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyAcceptsLegacyMetadataAndSymbolicValuesInFunctionCodeStrings(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :legacy-reader-function-code
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code "(fn [input]\n  {:typed #^String (:name input)\n   :nan ##NaN\n   :input input})"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint returned error for valid legacy reader forms in code string: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_invalid" {
			t.Fatalf("unexpected function_code_string_invalid diagnostic: %#v", item)
		}
	}
}

func TestFlowsLintLocalOnlyAcceptsRegexEscapesInPulledFunctionCodeStrings(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `;; breyta: pulled-source
{:slug :pulled-function-regex
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :blank-text?
              :code "(fn [input]\n  (boolean (re-matches #\"\\s+\" (:text input))))"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :check-text {:ref :blank-text? :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint returned error for valid pulled regex code string: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_invalid", "function_code_string_invalid")
}

func TestFlowsLintLocalOnlyIgnoresCodeLikeTextInRegexLiterals(t *testing.T) {
	flowLiteral := `{:slug :regex-code-like-text
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [pattern #"\s+#=(->> fake)"
              input (flow/input)]
          {:pattern pattern :input input})}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err != nil {
		t.Fatalf("code-like regex contents should not produce lint errors: %v\n%s", err, stdout)
	}
	rejectFlowLintDiagnosticCodes(t, body, "clojure_reader_eval_disabled", "unsupported_visual_flow_form")
}

func TestFlowsLintLocalOnlyContinuesScanningAfterRegexLiterals(t *testing.T) {
	flowLiteral := `{:slug :regex-before-invalid-forms
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :normalize :language :clojure :code '(fn [input] input)}]
 :flow '(let [pattern #"\s+"
              input (flow/input)
              normalized (flow/step :function :normalize {:ref :normalize :input [input]})]
          (->> normalized identity))}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("invalid forms after a regex should fail local lint\n%s", stdout)
	}
	requireFlowLintDiagnosticCodes(t, body, "function_step_input_shape_invalid", "unsupported_visual_flow_form")
}

func TestFlowsLintLocalOnlyFindsReaderEvalAfterRegexLiteral(t *testing.T) {
	flowLiteral := `{:slug :regex-before-reader-eval
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :pattern #"\s+"
 :unsafe #=(identity true)
 :flow '(flow/input)}
`
	body, err, stdout := runFlowLintLocalOnlyForLiteral(t, flowLiteral)
	if err == nil {
		t.Fatalf("reader eval after a regex should fail local lint\n%s", stdout)
	}
	requireFlowLintDiagnosticCodes(t, body, "clojure_reader_eval_disabled")
}

func TestFlowsLintLocalOnlyRejectsReaderEvalInFunctionCodeStrings(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-eval-function-code
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :code "(fn [input]\n  #=(identity input))"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_invalid" && item["severity"] == "error" {
			return
		}
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyRejectsReaderEvalInFlowSource(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :reader-eval-source
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :helper #=(identity :unsafe)
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "clojure_reader_eval_disabled" && item["severity"] == "error" {
			return
		}
	}
	t.Fatalf("expected clojure_reader_eval_disabled diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyRejectsReaderEvalInIncludedFlowSource(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	includeFile := filepath.Join(tmpDir, "unsafe.edn")
	if err := os.WriteFile(includeFile, []byte(`#=(identity :unsafe)`), 0o644); err != nil {
		t.Fatalf("write include file: %v", err)
	}
	flowLiteral := `{:slug :reader-eval-source-include
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :helper #flow/include "unsafe.edn"
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "clojure_reader_eval_disabled" && item["severity"] == "error" {
			return
		}
	}
	t.Fatalf("expected clojure_reader_eval_disabled diagnostic, got %#v", items)
}

func TestFlowsLintLocalOnlyBestEffortScansCodeStringsAfterExtractionError(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#?(:clj
   {:slug :bad-function-code-with-reader-conditional
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :functions [{:id :build-plan
                 :code "(fn [input]\n  (assoc input :ok true)"}]
    :flow '(let [input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected lint error")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning, sawCodeError bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] == "function_code_string_scan_incomplete" && item["severity"] == "warning" {
			sawFallbackWarning = true
		}
		if item["code"] == "function_code_string_invalid" && item["severity"] == "error" {
			sawCodeError = true
		}
	}
	if !sawFallbackWarning || !sawCodeError {
		t.Fatalf("expected fallback warning and code-string error, got %#v", items)
	}
}

func TestFlowsLintLocalOnlyBestEffortIgnoresNestedFunctionsInFlow(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `#?(:clj
   {:slug :fallback-nested-functions
    :concurrency {:type :singleton :on-new-version :coexist}
    :invocations {:default {:inputs []}}
    :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
    :functions [{:id :build-plan
                 :code "(fn [input]\n  (assoc input :ok true))"}]
    :flow '(let [shadow {:functions [{:id :shadow
                                      :code "(fn [input]\n  (assoc input :shadow true)"}]}
                 input (flow/input)]
             (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))})
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	app := &App{WorkspaceID: "ws-acme"}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--local-only"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint should not fail on nested :functions literals in fallback mode: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	var sawFallbackWarning bool
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		switch item["code"] {
		case "function_code_string_scan_incomplete":
			if item["severity"] == "warning" {
				sawFallbackWarning = true
			}
		case "function_code_string_invalid":
			t.Fatalf("nested :flow :functions produced lint-blocking diagnostic: %#v", item)
		}
	}
	if !sawFallbackWarning {
		t.Fatalf("expected fallback warning, got %#v", items)
	}
}

func TestFlowsLintLocalOnlySkipsAutomaticSkillNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)
	t.Setenv("LOCALAPPDATA", tmpDir)
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")

	skillPath := filepath.Join(tmpDir, ".codex", "skills", "breyta", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: breyta\n---\n# Old Breyta Skill\n"), 0o644); err != nil {
		t.Fatalf("seed installed skill: %v", err)
	}

	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :local-lint
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	var requestCount atomic.Int32
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{
		"--dev",
		"--api", srv.URL,
		"--token", "dev-user",
		"flows", "lint",
		"--file", flowFile,
		"--local-only",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("flows lint --local-only failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	time.Sleep(50 * time.Millisecond)
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("expected no API requests for --local-only lint, got %d; stderr=%s stdout=%s", got, errOut.String(), out.String())
	}
	if strings.Contains(errOut.String(), "Breyta skill") {
		t.Fatalf("expected no skill drift warning for --local-only lint, got stderr=%s", errOut.String())
	}
}

func TestFlowsLintLocalOnlySkipsStoredTokenRefresh(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)
	t.Setenv("LOCALAPPDATA", tmpDir)
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")

	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :local-lint
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	var apiRequests atomic.Int32
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiRequests.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	storePath := filepath.Join(tmpDir, "auth.json")
	st := &authstore.Store{}
	st.SetRecord(srv.URL, authstore.Record{
		Token:        "tok-stale",
		RefreshToken: "ref-stale",
		ExpiresAt:    time.Now().UTC().Add(30 * time.Second),
	})
	if err := authstore.SaveAtomic(storePath, st); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	t.Setenv("BREYTA_AUTH_STORE", storePath)

	var refreshCalls atomic.Int32
	authRefreshHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			refreshCalls.Add(1)
			return httpJSON(200, map[string]any{
				"success":      true,
				"token":        "tok-refreshed",
				"refreshToken": "ref-refreshed",
				"expiresIn":    3600,
			})
		}),
	}
	t.Cleanup(func() { authRefreshHTTPClient = nil })

	root := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{
		"--dev",
		"--api", srv.URL,
		"flows", "lint",
		"--file", flowFile,
		"--local-only",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("flows lint --local-only failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	time.Sleep(50 * time.Millisecond)
	if got := refreshCalls.Load(); got != 0 {
		t.Fatalf("expected no auth refresh for --local-only lint, got %d; stderr=%s stdout=%s", got, errOut.String(), out.String())
	}
	if got := apiRequests.Load(); got != 0 {
		t.Fatalf("expected no API requests for --local-only lint, got %d; stderr=%s stdout=%s", got, errOut.String(), out.String())
	}
}

func TestFlowsLintServerSendsCandidateLiteral(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :linted-flow
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :language :clojure
              :code "(fn [input]\n  (assoc input :ok true))"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	var gotCommand string
	var gotLiteral string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotCommand, _ = body["command"].(string)
		args, _ := body["args"].(map[string]any)
		gotLiteral, _ = args["flowLiteral"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"meta":        map[string]any{"stages": []string{"server"}},
			"data": map[string]any{
				"valid":       true,
				"flowSlug":    "linted-flow",
				"diagnostics": []any{},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--server"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("lint failed: %v\n%s", err, out.String())
	}
	if gotCommand != "flows.lint" {
		t.Fatalf("expected flows.lint command, got %q", gotCommand)
	}
	if gotLiteral != flowLiteral {
		t.Fatalf("expected flow literal to be sent unchanged")
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	meta, _ := body["meta"].(map[string]any)
	stages, _ := meta["stages"].([]any)
	if len(stages) != 2 || stages[0] != "local" || stages[1] != "server" {
		t.Fatalf("expected local+server stages, got %#v", meta["stages"])
	}
}

func TestFlowsLintServerRejectsMalformedTopLevelFunctionCodeBeforeAPI(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :malformed-function-code
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :functions [{:id :build-plan
              :language :clojure
              :code "(fn [input]\n  (assoc input :ok true)"}]
 :flow '(let [input (flow/input)]
          (flow/step :function :build-plan {:ref :build-plan :input {:input input}}))}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	var requestCount atomic.Int32
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		http.Error(w, "server lint must not run for malformed local source", http.StatusInternalServerError)
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--server"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected server lint to fail on malformed function code\n%s", out.String())
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("expected local diagnostics to prevent server lint request, got %d", got)
	}

	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("expected ok=false, got %#v", body)
	}
	data, _ := body["data"].(map[string]any)
	items, _ := data["diagnostics"].([]any)
	for _, itemAny := range items {
		item, _ := itemAny.(map[string]any)
		if item["code"] != "function_code_string_invalid" || item["severity"] != "error" {
			continue
		}
		path, _ := item["path"].([]any)
		if len(path) < 3 || path[1] != ":build-plan" {
			t.Fatalf("expected function id in path, got %#v", item)
		}
		hint, _ := item["hint"].(string)
		if !strings.Contains(hint, "directly quoted form") {
			t.Fatalf("expected actionable function-code hint, got %#v", item)
		}
		return
	}
	t.Fatalf("expected function_code_string_invalid diagnostic, got %#v", items)
}

func TestFlowsLintServerTimeoutBoundsRequiredServerLint(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :linted-flow
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--server", "--timeout", "20ms"})

	start := time.Now()
	err := cmd.Execute()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected server lint timeout")
	}
	if elapsed > time.Second {
		t.Fatalf("expected timeout to bound lint quickly, elapsed=%s", elapsed)
	}
	if !strings.Contains(out.String(), "flows lint server timed out after 20ms") {
		t.Fatalf("expected actionable timeout error, got %q", out.String())
	}
}

func TestFlowsLintOptionalServerTimeoutKeepsLocalResult(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :linted-flow
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--timeout", "20ms"})

	start := time.Now()
	err := cmd.Execute()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("optional server lint timeout should keep local result: %v\n%s", err, out.String())
	}
	if elapsed > time.Second {
		t.Fatalf("expected timeout to bound lint quickly, elapsed=%s", elapsed)
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from local lint fallback, got %#v", body)
	}
	meta, _ := body["meta"].(map[string]any)
	if got, _ := meta["serverSkipped"].(string); got != "api_error" {
		t.Fatalf("expected api_error skip reason, got %#v", meta)
	}
	if serverErr, _ := meta["serverError"].(string); !strings.Contains(serverErr, "flows lint server timed out after 20ms") {
		t.Fatalf("expected actionable timeout serverError, got %#v", meta)
	}
	stages, _ := meta["stages"].([]any)
	if len(stages) != 1 || stages[0] != "local" {
		t.Fatalf("expected local-only stages after optional server timeout, got %#v", meta["stages"])
	}
}

func TestFlowsLintOptionalServerFailureKeepsLocalResult(t *testing.T) {
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	flowLiteral := `{:slug :linted-flow
 :concurrency {:type :singleton :on-new-version :coexist}
 :invocations {:default {:inputs []}}
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :flow '(let [input (flow/input)] input)}
`
	if err := os.WriteFile(flowFile, []byte(flowLiteral), 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"error":{"message":"server unavailable"}}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsLintCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("optional server lint should not fail clean local lint: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.NewDecoder(bytes.NewReader(out.Bytes())).Decode(&body); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok=true from local lint fallback, got %#v", body)
	}
	meta, _ := body["meta"].(map[string]any)
	if got, _ := meta["serverSkipped"].(string); got != "api_status_503" {
		t.Fatalf("expected api_status_503 skip reason, got %#v", meta)
	}
	stages, _ := meta["stages"].([]any)
	if len(stages) != 1 || stages[0] != "local" {
		t.Fatalf("expected local-only stages after optional server failure, got %#v", meta["stages"])
	}
}
