package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/breyta/breyta-cli/internal/authstore"
)

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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
          (flow/step :function :build-plan {:ref :build-plan :input input}))}
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
             (flow/step :function :build-plan {:ref :build-plan :input input}))})
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
 :flow '(let [input (flow/input)] input)}
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
