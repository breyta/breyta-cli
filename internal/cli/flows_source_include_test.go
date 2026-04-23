package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExpandFlowSourceIncludes_RecursiveAndCommentSafe(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "flow.clj")
	funcPath := filepath.Join(tmpDir, "functions", "normalize.edn")
	codePath := filepath.Join(tmpDir, "code", "normalize-code.clj")

	if err := os.MkdirAll(filepath.Dir(funcPath), 0o755); err != nil {
		t.Fatalf("mkdir functions: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}

	if err := os.WriteFile(codePath, []byte(`"(fn [x] {:ok x})"`), 0o644); err != nil {
		t.Fatalf("write code include: %v", err)
	}
	if err := os.WriteFile(funcPath, []byte(`{:id :normalize-config
 :language :clojure
 :code #flow/include "../code/normalize-code.clj"}`), 0o644); err != nil {
		t.Fatalf("write function include: %v", err)
	}

	source := `{:slug :flow-include
 :name "Flow Include"
 :functions [#flow/include "functions/normalize.edn"]
 :concurrency {:type :singleton :on-new-version :supersede}
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 ; #flow/include "ignored-comment.edn"
 :flow '(let [example "#flow/include \"ignored-string.edn\""]
          example)}`

	expanded, err := expandFlowSourceIncludes(root, source)
	if err != nil {
		t.Fatalf("expand includes: %v", err)
	}
	if strings.Contains(expanded, `#flow/include "functions/normalize.edn"`) {
		t.Fatalf("expected include tag to be expanded")
	}
	if !strings.Contains(expanded, `:id :normalize-config`) {
		t.Fatalf("expected included function map in expanded source:\n%s", expanded)
	}
	if !strings.Contains(expanded, `"(fn [x] {:ok x})"`) {
		t.Fatalf("expected nested include to resolve into function code:\n%s", expanded)
	}
	if !strings.Contains(expanded, `; #flow/include "ignored-comment.edn"`) {
		t.Fatalf("expected comment include text to remain untouched")
	}
	if !strings.Contains(expanded, `"#flow/include \"ignored-string.edn\""`) {
		t.Fatalf("expected string literal include text to remain untouched")
	}
}

func TestExpandFlowSourceIncludes_DetectsCycles(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "flow.clj")
	aPath := filepath.Join(tmpDir, "a.edn")
	bPath := filepath.Join(tmpDir, "b.edn")

	if err := os.WriteFile(aPath, []byte(`#flow/include "b.edn"`), 0o644); err != nil {
		t.Fatalf("write a.edn: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(`#flow/include "a.edn"`), 0o644); err != nil {
		t.Fatalf("write b.edn: %v", err)
	}

	_, err := expandFlowSourceIncludes(root, `{:templates [#flow/include "a.edn"]}`)
	if err == nil {
		t.Fatalf("expected include cycle error")
	}
	if !strings.Contains(err.Error(), "flow source include cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestExpandFlowSourceIncludes_HonorsReaderDiscardOnDirectInclude(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "flow.clj")

	expanded, err := expandFlowSourceIncludes(root, `{:templates [#_ #flow/include "tmp/debug.edn"]}`)
	if err != nil {
		t.Fatalf("expand discarded include: %v", err)
	}
	if !strings.Contains(expanded, `#_ #flow/include "tmp/debug.edn"`) {
		t.Fatalf("expected discarded include form to remain untouched, got:\n%s", expanded)
	}
}

func TestExpandFlowSourceIncludes_HonorsReaderDiscardAcrossNestedForms(t *testing.T) {
	tmpDir := t.TempDir()
	root := filepath.Join(tmpDir, "flow.clj")

	expanded, err := expandFlowSourceIncludes(root, `{:templates [#_ {:debug #flow/include "tmp/debug.edn"}]}`)
	if err != nil {
		t.Fatalf("expand discarded nested form: %v", err)
	}
	if !strings.Contains(expanded, `#_ {:debug #flow/include "tmp/debug.edn"}`) {
		t.Fatalf("expected discarded nested form to remain untouched, got:\n%s", expanded)
	}
}

func TestFlowsPush_LocalIncludeExpandsPayloadWithoutRewritingSource(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotLiteral string
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		_ = method
		gotLiteral, _ = payload["flowLiteral"].(string)
		return nil
	}
	useDoAPICommandFn = true

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.clj")
	templateFile := filepath.Join(tmpDir, "templates", "reviewer.edn")
	schemaFile := filepath.Join(tmpDir, "review-schema.edn")

	if err := os.MkdirAll(filepath.Dir(templateFile), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(templateFile, []byte(`{:id :security-reviewer
 :type :llm-prompt
 :system "You are concise."
 :prompt "Review {{objective}}"}`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(schemaFile, []byte(`{"type" "object" "required" ["summary"]}`), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	flowSource := `{:slug :flow-include
 :name "Flow Include"
 :concurrency {:type :singleton :on-new-version :supersede}
 :templates [#flow/include "templates/reviewer.edn"]
 :triggers [{:type :manual :label "Run" :enabled true :config {}}]
 :flow '(let [schema #flow/include "review-schema.edn"]
          schema)}`
	if err := os.WriteFile(flowFile, []byte(flowSource), 0o644); err != nil {
		t.Fatalf("write flow source: %v", err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsPushCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", flowFile, "--repair-delimiters=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotLiteral == "" {
		t.Fatalf("expected flowLiteral payload to be captured")
	}
	if strings.Contains(gotLiteral, "#flow/include") {
		t.Fatalf("expected payload to be expanded, got:\n%s", gotLiteral)
	}
	if !strings.Contains(gotLiteral, `:id :security-reviewer`) {
		t.Fatalf("expected included template in payload:\n%s", gotLiteral)
	}
	if !strings.Contains(gotLiteral, `{"type" "object" "required" ["summary"]}`) {
		t.Fatalf("expected included schema in payload:\n%s", gotLiteral)
	}

	after, err := os.ReadFile(flowFile)
	if err != nil {
		t.Fatalf("read after push: %v", err)
	}
	if !strings.Contains(string(after), `#flow/include "templates/reviewer.edn"`) {
		t.Fatalf("expected authored source file to retain include tags, got:\n%s", string(after))
	}
}
