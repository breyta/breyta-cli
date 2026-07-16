package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func executeLocalAuthoringJSON(t *testing.T, cmd *cobra.Command, args ...string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v\n%s", err, out.String())
	}
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("decode command output: %v\n%s", err, out.String())
	}
	return body
}

func TestFlowsInitCreatesLocalCanonicalSourceWithManualInterface(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	body := executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-test"}),
		"order-sync", "--out", path, "--name", "Order sync")

	if got, _ := body["ok"].(bool); !got {
		t.Fatalf("expected successful init, got %#v", body)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initialized source: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		":slug :order-sync",
		":steps []",
		":interfaces {:manual [{:id :run",
		":schedules []",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("initialized source missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, ":triggers") {
		t.Fatalf("initialized source should use the interface surface without deprecated :triggers:\n%s", text)
	}
}

func TestLocalStepCRUDAndComposeOnlyRewriteOwnedSourceSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	stepPath := filepath.Join(t.TempDir(), "add-one.edn")
	updatedStepPath := filepath.Join(t.TempDir(), "add-two.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one" :input-schema [:map]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updatedStepPath, []byte(`{:id :tools/add-one :type :function :description "Add two" :input-schema [:map]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)

	source, _ := os.ReadFile(path)
	if !strings.Contains(string(source), ":id :tools/add-one") {
		t.Fatalf("expected created step in source:\n%s", source)
	}

	executeLocalAuthoringJSON(t, newFlowsComposeCmd(app), "order-sync", "--flow-file", path, "--body", "(let [step (flow/step :tools/add-one :run {})] step)")
	source, _ = os.ReadFile(path)
	if !strings.Contains(string(source), "(let [step (flow/step :tools/add-one :run {})] step)") {
		t.Fatalf("expected compose to replace :flow body:\n%s", source)
	}
	if !strings.Contains(string(source), ":interfaces {:manual [{:id :run") || !strings.Contains(string(source), ":id :tools/add-one") {
		t.Fatalf("compose should preserve interfaces and steps:\n%s", source)
	}

	executeLocalAuthoringJSON(t, newFlowsStepsLocalUpdateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", updatedStepPath, "--run=false")
	source, _ = os.ReadFile(path)
	if !strings.Contains(string(source), `:description "Add two"`) {
		t.Fatalf("expected updated step in source:\n%s", source)
	}

	executeLocalAuthoringJSON(t, newFlowsStepsLocalRemoveCmd(app), "order-sync", "--flow-file", path, "tools/add-one")
	source, _ = os.ReadFile(path)
	if strings.Contains(string(source), ":id :tools/add-one") {
		t.Fatalf("expected removed step to be absent:\n%s", source)
	}
}

func TestFlowsStepsRunSendsFullLocalLiteralToEphemeralAPI(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	flowSource := `{:slug :order-sync
 :name "Order sync"
 :description ""
 :concurrency {:type :singleton :on-new-version :supersede}
 :steps [{:id :tools/add-one :type :function :description "Add one" :input-schema [:map] :defaults {:ref :add-one}}]
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :invocations {:default {:label "Run" :inputs []}}
 :schedules []
 :flow '(flow/input)}
`
	if err := os.WriteFile(path, []byte(flowSource), 0o644); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"stepId": "tools/add-one", "result": map[string]any{"answer": 3}},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	cmd := newFlowsStepsLocalRunCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--params", `{"n":2}`})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("local flow step run failed: %v\noutput=%s", err, out.String())
	}
	if got["command"] != "steps.run" {
		t.Fatalf("expected steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowLiteral"] != flowSource {
		t.Fatalf("expected complete local literal, got %#v", args["flowLiteral"])
	}
	if args["stepId"] != "tools/add-one" || args["flowSlug"] != "order-sync" {
		t.Fatalf("expected local step identity, got %#v", args)
	}
	if _, exists := args["source"]; exists {
		t.Fatalf("local literal run should not send a stored source selector: %#v", args)
	}
}
