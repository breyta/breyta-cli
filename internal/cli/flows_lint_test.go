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
