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

func TestFlowsInitCanSeedPackagedStepIntoLocalSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "fetch-order.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/fetch-order
 :type :http
 :description "Fetch an order"
 :input-schema [:map]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	body := executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-test"}),
		"order-sync", "--out", path, "--step-id", "tools/fetch-order", "--step-file", stepPath)
	if got, _ := body["ok"].(bool); !got {
		t.Fatalf("expected successful seeded init, got %#v", body)
	}
	if got, _ := body["data"].(map[string]any)["stepId"].(string); got != "tools/fetch-order" {
		t.Fatalf("expected seeded step id in result, got %#v", body)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded source: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		":id :tools/fetch-order",
		":type :http",
		":interfaces {:manual [{:id :run",
		":schedules []",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("seeded source missing %q:\n%s", want, text)
		}
	}
}

func TestFlowsInitSeedRunUsesCompleteLocalLiteral(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "fetch-order.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/fetch-order
 :type :function
 :description "Fetch an order"
 :input-schema [:map]}`), 0o644); err != nil {
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
			"data":        map[string]any{"stepId": "tools/fetch-order", "result": map[string]any{"orderId": "order-123"}},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	var out bytes.Buffer
	cmd := newFlowsInitCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"order-sync", "--out", path,
		"--step-id", "tools/fetch-order", "--step-file", stepPath,
		"--run", "--idempotency-key", "seed-proof-1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("seeded init --run failed: %v\n%s", err, out.String())
	}
	if got["command"] != "steps.run" {
		t.Fatalf("expected steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "order-sync" || args["stepId"] != "tools/fetch-order" {
		t.Fatalf("expected seeded step identity, got %#v", args)
	}
	if args["idempotencyKey"] != "seed-proof-1" {
		t.Fatalf("expected seeded proof idempotency key, got %#v", args["idempotencyKey"])
	}
	params, _ := args["params"].(map[string]any)
	if len(params) != 0 {
		t.Fatalf("expected no-input proof params by default, got %#v", params)
	}
	flowLiteral, _ := args["flowLiteral"].(string)
	if !strings.Contains(flowLiteral, ":id :tools/fetch-order") || !strings.Contains(flowLiteral, ":interfaces {:manual [{:id :run") {
		t.Fatalf("expected complete seeded local literal, got %s", flowLiteral)
	}
	if !strings.Contains(out.String(), `"run"`) {
		t.Fatalf("expected proof result in command output, got %s", out.String())
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

	executeLocalAuthoringJSON(t, newFlowsComposeCmd(app), "order-sync", "--flow-file", path, "--body", "(flow/input)")
	executeLocalAuthoringJSON(t, newFlowsStepsLocalRemoveCmd(app), "order-sync", "--flow-file", path, "tools/add-one")
	source, _ = os.ReadFile(path)
	if strings.Contains(string(source), ":id :tools/add-one") {
		t.Fatalf("expected removed step to be absent:\n%s", source)
	}
}

func TestLocalStepRemoveRejectsReferencedPackagedStep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	stepPath := filepath.Join(t.TempDir(), "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one" :input-schema [:map]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)
	executeLocalAuthoringJSON(t, newFlowsComposeCmd(app), "order-sync", "--flow-file", path, "--body", "(flow/step :tools/add-one :run {})")

	var out bytes.Buffer
	cmd := newFlowsStepsLocalRemoveCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected referenced step removal to fail\n%s", out.String())
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), ":id :tools/add-one") {
		t.Fatalf("failed removal should preserve step definition:\n%s", source)
	}
}

func TestLocalScheduleCRUDPreservesFlowDefinition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	updatedSchedulePath := filepath.Join(t.TempDir(), "daily-review.edn")
	if err := os.WriteFile(updatedSchedulePath, []byte(`{:id :daily-review
 :label "Daily review"
 :invocation :default
 :enabled false
 :cron "0 10 * * MON-FRI"
 :timezone "Europe/Oslo"
 :overlap-policy :queue}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	executeLocalAuthoringJSON(t, newFlowsSchedulesLocalAddCmd(app), "order-sync", "daily-review", "--flow-file", path, "--cron", "0 9 * * MON", "--timezone", "UTC", "--label", "Weekly review")

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{":id :daily-review", `:label "Weekly review"`, `:cron "0 9 * * MON"`, `:timezone "UTC"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("schedule add missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, ":interfaces {:manual [{:id :run") || !strings.Contains(text, ":invocations {:default") {
		t.Fatalf("schedule add rewrote unrelated definition surfaces:\n%s", text)
	}

	executeLocalAuthoringJSON(t, newFlowsSchedulesLocalUpdateCmd(app), "order-sync", "daily-review", "--flow-file", path, "--schedule-file", updatedSchedulePath)
	source, _ = os.ReadFile(path)
	text = string(source)
	for _, want := range []string{`:label "Daily review"`, `:enabled false`, `:cron "0 10 * * MON-FRI"`, `:timezone "Europe/Oslo"`, ":overlap-policy :queue"} {
		if !strings.Contains(text, want) {
			t.Fatalf("schedule update missing %q:\n%s", want, text)
		}
	}

	executeLocalAuthoringJSON(t, newFlowsSchedulesLocalRemoveCmd(app), "order-sync", "daily-review", "--flow-file", path)
	source, _ = os.ReadFile(path)
	if strings.Contains(string(source), ":id :daily-review") {
		t.Fatalf("expected removed schedule to be absent:\n%s", source)
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
