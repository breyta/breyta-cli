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

func TestLocalAuthoringRejectsMismatchedFlowSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-b.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-b", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newFlowsStepsLocalCreateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-a", "tools/add-one", "--flow-file", path, "--step-file", stepPath})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("local authoring accepted a flow file with a mismatched slug\n%s", out.String())
	}
	if !strings.Contains(out.String(), `does not match requested flow "order-a"`) {
		t.Fatalf("expected slug mismatch error, got:\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("slug mismatch changed the source:\n%s", current)
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

func TestLocalRunFailureIsNotReportedAsSuccessfulAuthoring(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error":       map[string]any{"message": "step validation failed"},
		})
	}))
	defer srv.Close()

	for _, update := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "order-sync.clj")
		stepPath := filepath.Join(dir, "add-one.edn")
		updatedStepPath := filepath.Join(dir, "add-two.edn")
		if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(updatedStepPath, []byte(`{:id :tools/add-one :type :function :description "Add two"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
		executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

		var cmd *cobra.Command
		var args []string
		if update {
			executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)
			cmd = newFlowsStepsLocalUpdateCmd(app)
			args = []string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", updatedStepPath, "--run"}
		} else {
			cmd = newFlowsStepsLocalCreateCmd(app)
			args = []string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, "--run"}
		}

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("--run failure for update=%t returned nil", update)
		}
		if !strings.Contains(out.String(), `"ok":false`) {
			t.Fatalf("--run failure for update=%t did not emit API failure envelope: %s", update, out.String())
		}
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newFlowsInitCmd(&App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "--out", path, "--step-id", "tools/add-one", "--step-file", stepPath, "--run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("seeded init --run failure returned nil")
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("seeded init --run failure did not emit API failure envelope: %s", out.String())
	}
}

func TestLocalStepEditingSkipsDiscardedVectorElements(t *testing.T) {
	source := `{:slug :order-sync
 :steps [#_{:id :tools/old :type :function}
          {:id :tools/add-one :type :function :description "Add one"}]}
`
	replacement := `{:id :tools/add-one :type :function :description "Add two"}`
	updated, err := replaceLocalStep(source, "tools/add-one", replacement)
	if err != nil {
		t.Fatalf("replaceLocalStep() error = %v", err)
	}
	if !strings.Contains(updated, `#_{:id :tools/old :type :function}`) || !strings.Contains(updated, `:description "Add two"`) {
		t.Fatalf("replaceLocalStep() rewrote the wrong vector element:\n%s", updated)
	}
	if strings.Contains(updated, `:description "Add one"`) {
		t.Fatalf("replaceLocalStep() left the active old step in place:\n%s", updated)
	}

	removed, err := removeLocalStep(source, "tools/add-one")
	if err != nil {
		t.Fatalf("removeLocalStep() error = %v", err)
	}
	if strings.Contains(removed, `#_{:id :tools/old :type :function}`) || strings.Contains(removed, ":id :tools/add-one") {
		t.Fatalf("removeLocalStep() left the complete discarded vector slot behind:\n%s", removed)
	}
}

func TestLocalVectorEditingAllowsOnlyDiscardedForms(t *testing.T) {
	source := `{:slug :order-sync
 :steps [#_{:id :tools/old :type :function}]
 :schedules [#_{:id :daily :cron "0 9 * * MON"}]}
`
	_, stepSpans, stepIndex, err := localStepSpansForID(source, "tools/old")
	if err != nil {
		t.Fatalf("localStepSpansForID() returned an error for an only-discarded vector: %v", err)
	}
	if len(stepSpans) != 0 || stepIndex != -1 {
		t.Fatalf("expected no active steps, got spans=%#v index=%d", stepSpans, stepIndex)
	}

	_, scheduleSpans, scheduleIndex, err := localScheduleSpansForID(source, "daily")
	if err != nil {
		t.Fatalf("localScheduleSpansForID() returned an error for an only-discarded vector: %v", err)
	}
	if len(scheduleSpans) != 0 || scheduleIndex != -1 {
		t.Fatalf("expected no active schedules, got spans=%#v index=%d", scheduleSpans, scheduleIndex)
	}
}

func TestLocalAuthoringPushValidatesDraft(t *testing.T) {
	flowFile := filepath.Join(t.TempDir(), "order-sync.clj")
	if err := os.WriteFile(flowFile, []byte(`{:slug :order-sync :flow '(identity 1)}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		command, _ := body["command"].(string)
		commands = append(commands, command)
		switch command {
		case "flows.put_draft":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"flowSlug": "order-sync", "saved": true},
			})
		case "flows.validate":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error":       map[string]any{"message": "canonical validation failed"},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "unexpected command"},
			})
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "token", TokenExplicit: true}
	cmd := newFlowsComposeCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "--flow-file", flowFile, "--body", "(flow/input)", "--push"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("local --push reported success without canonical validation\n%s", out.String())
	}
	if strings.Join(commands, ",") != "flows.put_draft,flows.validate" {
		t.Fatalf("expected put_draft followed by validate, got %v", commands)
	}
}

func TestLocalStepEditingAdvancesPastReaderConditionalElements(t *testing.T) {
	source := `{:slug :order-sync
 :steps [#?(:clj {:id :tools/add-one :type :function :description "Add one"}
             :cljs {:id :tools/cljs-only :type :function})
          {:id :tools/second :type :function :description "Second"}]}
`
	replacement := `{:id :tools/add-one :type :function :description "Add two"}`
	updated, err := replaceLocalStep(source, "tools/add-one", replacement)
	if err != nil {
		t.Fatalf("replaceLocalStep() with reader conditional error = %v", err)
	}
	if !strings.Contains(updated, `:description "Add two"`) || !strings.Contains(updated, `:id :tools/cljs-only`) || !strings.Contains(updated, `:id :tools/second`) {
		t.Fatalf("replaceLocalStep() corrupted reader conditional vector:\n%s", updated)
	}

	removed, err := removeLocalStep(source, "tools/second")
	if err != nil {
		t.Fatalf("removeLocalStep() after reader conditional error = %v", err)
	}
	if strings.Contains(removed, `:id :tools/second`) || !strings.Contains(removed, `:id :tools/add-one`) {
		t.Fatalf("removeLocalStep() did not advance past reader conditional:\n%s", removed)
	}

	conditionalRemoved, err := removeLocalStep(source, "tools/add-one")
	if err != nil {
		t.Fatalf("removeLocalStep() reader conditional error = %v", err)
	}
	if strings.Contains(conditionalRemoved, "#?") || strings.Contains(conditionalRemoved, `:id :tools/add-one`) || !strings.Contains(conditionalRemoved, `:id :tools/second`) {
		t.Fatalf("removeLocalStep() left a malformed reader conditional:\n%s", conditionalRemoved)
	}

	metadataSource := `{:slug :order-sync
 :steps [^{:tag :old} {:id :tools/metadata :type :function}
          {:id :tools/next :type :function}]}
`
	metadataRemoved, err := removeLocalStep(metadataSource, "tools/metadata")
	if err != nil {
		t.Fatalf("removeLocalStep() metadata-prefixed error = %v", err)
	}
	if strings.Contains(metadataRemoved, ":tag :old") || strings.Contains(metadataRemoved, ":id :tools/metadata") || !strings.Contains(metadataRemoved, ":id :tools/next") {
		t.Fatalf("removeLocalStep() left metadata attached to the next form:\n%s", metadataRemoved)
	}

	scheduleSource := `{:slug :order-sync
 :schedules [#?(:clj {:id :daily :cron "0 9 * * MON"}
                  :cljs {:id :cljs-daily :cron "0 10 * * MON"})
              {:id :later :cron "0 11 * * MON"}]}
`
	scheduleRemoved, err := removeLocalSchedule(scheduleSource, "daily")
	if err != nil {
		t.Fatalf("removeLocalSchedule() reader conditional error = %v", err)
	}
	if strings.Contains(scheduleRemoved, "#?") || strings.Contains(scheduleRemoved, ":id :daily") || !strings.Contains(scheduleRemoved, ":id :later") {
		t.Fatalf("removeLocalSchedule() left a malformed reader conditional:\n%s", scheduleRemoved)
	}
}

func TestLocalStepLiteralIDRequiresOneCompleteTopLevelMap(t *testing.T) {
	for _, literal := range []string{
		`{:id :tools/one :type :function} {:id :tools/two :type :function}`,
		`{:id :tools/one :type :function} [1]`,
		`{:id :tools/one :type :function} )`,
	} {
		if _, err := localStepLiteralID(literal); err == nil {
			t.Fatalf("localStepLiteralID(%q) returned nil error for invalid literal", literal)
		}
	}

	if got, err := localStepLiteralID("{:id :tools/one :type :function}\n; trailing comment\n"); err != nil || got != "tools/one" {
		t.Fatalf("localStepLiteralID() valid literal = %q, %v; want tools/one", got, err)
	}
	for _, literal := range []string{
		`{:custom/id :tools/wrong :type :function}`,
		`{:custom/id :tools/wrong :type :function :id :tools/right}`,
	} {
		got, err := localStepLiteralID(literal)
		if strings.Contains(literal, ":id :tools/right") {
			if err != nil || got != "tools/right" {
				t.Fatalf("localStepLiteralID() ignored exact :id in %q: %q, %v", literal, got, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("localStepLiteralID() accepted namespaced :custom/id as packaged id: %q", literal)
		}
	}
}

func TestLocalComposeRejectsInvalidBodyWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{"(let [value 1)", "(flow/input) (flow/input)"} {
		cmd := newFlowsComposeCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"order-sync", "--flow-file", path, "--body", body})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("compose accepted invalid body %q", body)
		}
		current, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(current) != string(original) {
			t.Fatalf("compose overwrote source after rejecting body %q:\n%s", body, current)
		}
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

func TestLocalStepCRUDSkipsIncludedVectorEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	includePath := filepath.Join(dir, "existing.edn")
	newStepPath := filepath.Join(dir, "new-step.edn")
	updatedStepPath := filepath.Join(dir, "direct-updated.edn")
	if err := os.WriteFile(includePath, []byte(`{:id :tools/included :type :function :description "Included"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newStepPath, []byte(`{:id :tools/new :type :function :description "New"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updatedStepPath, []byte(`{:id :tools/direct :type :function :description "Updated direct"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	flowSource := `{:slug :order-sync
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps [#flow/include "existing.edn"
         {:id :tools/direct :type :function :description "Direct"}]
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :invocations {:default {:inputs []}}
 :schedules []
 :flow '(flow/input)}
`
	if err := os.WriteFile(path, []byte(flowSource), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}

	var duplicateOut bytes.Buffer
	duplicateCmd := newFlowsStepsLocalCreateCmd(app)
	duplicateCmd.SetOut(&duplicateOut)
	duplicateCmd.SetErr(&duplicateOut)
	duplicateCmd.SetArgs([]string{"order-sync", "tools/included", "--flow-file", path, "--step-file", includePath})
	if err := duplicateCmd.Execute(); err == nil {
		t.Fatalf("expected duplicate included step to be rejected\n%s", duplicateOut.String())
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != flowSource {
		t.Fatalf("duplicate included step changed source:\n%s", unchanged)
	}

	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/new", "--flow-file", path, "--step-file", newStepPath)
	executeLocalAuthoringJSON(t, newFlowsStepsLocalUpdateCmd(app), "order-sync", "tools/direct", "--flow-file", path, "--step-file", updatedStepPath, "--run=false")
	executeLocalAuthoringJSON(t, newFlowsStepsLocalRemoveCmd(app), "order-sync", "tools/new", "--flow-file", path)

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, `#flow/include "existing.edn"`) || !strings.Contains(text, `:description "Updated direct"`) {
		t.Fatalf("local CRUD should preserve included entries and edit direct entries:\n%s", text)
	}
	if strings.Contains(text, ":id :tools/new") {
		t.Fatalf("removed direct step is still present:\n%s", text)
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

func TestLocalStepRemoveRejectsReferencesInIncludedFlowBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	bodyPath := filepath.Join(dir, "body.clj")
	flowSource := `{:slug :order-sync
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps [{:id :tools/add-one :type :function :description "Add one"}]
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :invocations {:default {:inputs []}}
 :schedules []
 :flow #flow/include "body.clj"}
`
	if err := os.WriteFile(path, []byte(flowSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bodyPath, []byte(`'(flow/step :tools/add-one :run {})`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	var out bytes.Buffer
	cmd := newFlowsStepsLocalRemoveCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected included flow body reference to block removal\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != flowSource {
		t.Fatalf("failed removal should preserve source:\n%s", current)
	}
}

func TestLocalStepScaffoldRejectsUnsafeTypeWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/unsafe", "--flow-file", path, "--type", "http}"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected unsafe scaffold type to be rejected\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("unsafe scaffold type changed source:\n%s", current)
	}
}

func TestLocalFlowEditingMatchesExactTopLevelKeys(t *testing.T) {
	source := `{:slug :order-sync
 :custom/steps [{:id :custom/step}]
 :steps []
 :custom/flow '(custom/body)
 :flow '(flow/input)}
`
	updated, err := appendLocalStep(source, `{:id :tools/direct :type :function}`)
	if err != nil {
		t.Fatalf("appendLocalStep() failed: %v", err)
	}
	if !strings.Contains(updated, ":custom/steps [{:id :custom/step}]") || !strings.Contains(updated, ":steps [") {
		t.Fatalf("appendLocalStep() touched a namespaced extension key:\n%s", updated)
	}
	composed, err := composeLocalFlowBody(updated, "(flow/input)")
	if err != nil {
		t.Fatalf("composeLocalFlowBody() failed: %v", err)
	}
	if !strings.Contains(composed, ":custom/flow '(custom/body)") || !strings.Contains(composed, ":flow '(flow/input)") {
		t.Fatalf("composeLocalFlowBody() touched a namespaced extension key:\n%s", composed)
	}
}

func TestLocalComposeAcceptsOnlyExactQuoteForm(t *testing.T) {
	source := `{:slug :order-sync
 :flow '(flow/input)}
`
	updated, err := composeLocalFlowBody(source, "(quote-string value)")
	if err != nil {
		t.Fatalf("composeLocalFlowBody() rejected valid quoted body: %v", err)
	}
	if !strings.Contains(updated, ":flow '(quote-string value)") {
		t.Fatalf("composeLocalFlowBody() should add reader quote to non-quote form:\n%s", updated)
	}
	if got, err := composeLocalFlowBody(source, "(quote (flow/input))"); err != nil || !strings.Contains(got, ":flow (quote (flow/input))") {
		t.Fatalf("composeLocalFlowBody() should preserve exact quote form: %v\n%s", err, got)
	}
	if got, err := composeLocalFlowBody(source, "`(flow/input)"); err != nil || !strings.Contains(got, ":flow `(flow/input)") {
		t.Fatalf("composeLocalFlowBody() should preserve syntax quote: %v\n%s", err, got)
	}
}

func TestLocalStepUpdateRegistersResultPreviewFlags(t *testing.T) {
	cmd := newFlowsStepsLocalUpdateCmd(&App{WorkspaceID: "ws-test"})
	for _, name := range []string{"full", "result-path", "result-file", "preview-depth", "preview-items", "preview-runes"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected local step update to register --%s", name)
		}
	}
}

func TestLocalScheduleAddRejectsIncludedDuplicate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	includePath := filepath.Join(dir, "daily.edn")
	flowSource := `{:slug :order-sync
 :concurrency {:type :singleton :on-new-version :coexist}
 :steps []
 :interfaces {:manual [{:id :run :label "Run" :invocation :default}]}
 :invocations {:default {:inputs []}}
 :schedules [#flow/include "daily.edn"]
 :flow '(flow/input)}
`
	if err := os.WriteFile(path, []byte(flowSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(includePath, []byte(`{:id :daily :cron "0 9 * * MON"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newFlowsSchedulesLocalAddCmd(&App{WorkspaceID: "ws-test"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "daily", "--flow-file", path, "--cron", "0 10 * * MON"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected duplicate included schedule to be rejected\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != flowSource {
		t.Fatalf("duplicate included schedule changed source:\n%s", current)
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

func TestLocalScheduleLiteralRequiresOneCompleteTopLevelMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	badSchedulePath := filepath.Join(t.TempDir(), "bad-schedule.edn")
	if err := os.WriteFile(badSchedulePath, []byte(`{:id :daily-review :cron "0 9 * * MON"} {:id :hidden :cron "0 10 * * MON"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newFlowsSchedulesLocalAddCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "daily-review", "--flow-file", path, "--schedule-file", badSchedulePath})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("schedule add accepted multiple top-level maps\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("schedule add overwrote source after rejecting literal:\n%s", current)
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
			"data":        map[string]any{"stepId": "tools/add-one", "result": map[string]any{"answer": 3, "nested": map[string]any{"items": []any{1, 2, 3}}}},
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
	var body map[string]any
	if err := json.Unmarshal(out.Bytes(), &body); err != nil {
		t.Fatalf("decode compact local run output: %v\n%s", err, out.String())
	}
	data, _ := body["data"].(map[string]any)
	if _, exists := data["result"]; exists {
		t.Fatalf("local run should compact data.result by default: %#v", data)
	}
	if _, exists := data["resultPreview"]; !exists {
		t.Fatalf("local run should include data.resultPreview by default: %#v", data)
	}

	fullCmd := newFlowsStepsLocalRunCmd(app)
	var fullOut bytes.Buffer
	fullCmd.SetOut(&fullOut)
	fullCmd.SetErr(&fullOut)
	fullCmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--params", `{"n":2}`, "--full"})
	if err := fullCmd.Execute(); err != nil {
		t.Fatalf("full local flow step run failed: %v\noutput=%s", err, fullOut.String())
	}
	var fullBody map[string]any
	if err := json.Unmarshal(fullOut.Bytes(), &fullBody); err != nil {
		t.Fatalf("decode full local run output: %v\n%s", err, fullOut.String())
	}
	fullData, _ := fullBody["data"].(map[string]any)
	if _, exists := fullData["result"]; !exists {
		t.Fatalf("--full should preserve data.result: %#v", fullData)
	}
}
