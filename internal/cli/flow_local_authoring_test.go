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
		`:invocations {:default {:label "Run" :inputs []}}`,
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

func TestFlowsInitSeedsRepeatedInvocationInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-audit.clj")
	executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-test"}),
		"site-audit", "--out", path,
		"--input", "site-url:text:required:Site URL",
		"--input", "depth:number:optional:Depth: in pages",
		"--input", "notes:textarea")

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initialized source: %v", err)
	}
	text := string(source)
	for _, want := range []string{
		`:inputs [{:name :site-url :type :text :required true :label "Site URL"}`,
		`{:name :depth :type :number :required false :label "Depth: in pages"}`,
		`{:name :notes :type :textarea}]`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("initialized source missing %q:\n%s", want, text)
		}
	}
}

func TestFlowsInitRegistersRepeatableInputFlag(t *testing.T) {
	flag := newFlowsInitCmd(&App{WorkspaceID: "ws-test"}).Flags().Lookup("input")
	if flag == nil {
		t.Fatal("expected flows init to register --input")
	}
	if flag.Value.Type() != "stringArray" {
		t.Fatalf("expected repeatable stringArray --input, got %s", flag.Value.Type())
	}
}

func TestFlowsInitRejectsInvalidInvocationInputsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing type", args: []string{"--input", "site-url"}, want: "expected name:type"},
		{name: "unsafe name", args: []string{"--input", "site/url:text"}, want: "invalid --input name"},
		{name: "unsupported type", args: []string{"--input", "site-url:object"}, want: "invalid --input type"},
		{name: "invalid requirement", args: []string{"--input", "site-url:text:yes"}, want: "must be required or optional"},
		{name: "duplicate name", args: []string{"--input", "site-url:text", "--input", "site-url:text"}, want: "duplicate --input name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "site-audit.clj")
			cmd := newFlowsInitCmd(&App{WorkspaceID: "ws-test"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := append([]string{"site-audit", "--out", path}, tc.args...)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("expected invalid input to fail\n%s", out.String())
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("expected %q, got:\n%s", tc.want, out.String())
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid input wrote local source: %v", err)
			}
		})
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
	cmd.SetArgs([]string{
		"order-sync", "--out", path,
		"--step-id", "tools/add-one", "--step-file", stepPath,
		"--run", "--params", `{"orderId":"order 123"}`,
		"--idempotency-key", "seed-proof-1",
		"--profile-id", "profile-1",
		"--timeout", "22m",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("seeded init --run failure returned nil")
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("seeded init --run failure did not emit API failure envelope: %s", out.String())
	}
	for _, want := range []string{
		`"savedLocally":true`,
		`"localPath":"` + path + `"`,
		"breyta flows steps run order-sync tools/add-one --flow-file " + path,
		`--params '{\"orderId\":\"order 123\"}'`,
		"--idempotency-key seed-proof-1",
		"--profile-id profile-1",
		"--timeout 22m0s",
		"instead of rerunning flows init",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("seeded init --run failure missing recovery %q: %s", want, out.String())
		}
	}
}

func TestFlowsInitPushFailureReportsSavedLocalRecovery(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	requests := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error":       map[string]any{"message": "draft rejected"},
		})
	}))
	defer srv.Close()

	dir := filepath.Join(t.TempDir(), "flow sources")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	cmd := newFlowsInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"order-sync", "--out", path,
		"--step-id", "tools/add-one", "--step-file", stepPath,
		"--push", "--run",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("seeded init --push failure returned nil\n%s", out.String())
	}
	if requests != 1 {
		t.Fatalf("failed push must abort the pending run; got %d requests", requests)
	}
	for _, want := range []string{
		`"savedLocally":true`,
		`"localPath":"` + path + `"`,
		"breyta flows push --file " + shellSingleQuote(path),
		"The requested --run did NOT happen.",
		"instead of rerunning flows init",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("seeded init --push failure missing recovery %q: %s", want, out.String())
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected initialized source to remain saved: %v", err)
	}
}

func TestFlowsInitPostPushValidationFailureDoesNotRecommendRepush(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		command, _ := request["command"].(string)
		commands = append(commands, command)
		if command == "flows.validate" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": map[string]any{"message": "invalid invocation"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "order-sync.clj")
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	cmd := newFlowsInitCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "--out", path, "--push"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("post-push validation failure returned nil\n%s", out.String())
	}
	if strings.Join(commands, ",") != "flows.put_draft,flows.validate" {
		t.Fatalf("expected draft write followed by validation, got %v", commands)
	}
	text := out.String()
	for _, want := range []string{
		`"draftSaved":true`,
		`"savedLocally":true`,
		"breyta flows validate order-sync",
		"the draft was pushed, but validation failed",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("post-push validation failure missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "breyta flows push --file") {
		t.Fatalf("post-push validation failure must not recommend repeating the successful push: %s", text)
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

func TestLocalVectorEditingFlattensSplicedReaderConditionalVectors(t *testing.T) {
	source := `{:slug :order-sync
 :steps [#?@(:clj [{:id :tools/first :type :function}
                  {:id :tools/second :type :function}])]
 :schedules [#?@(:clj [{:id :daily :cron "0 9 * * MON"}])]
}
`
	_, stepSpans, stepIndex, err := localStepSpansForID(source, "tools/second")
	if err != nil {
		t.Fatalf("localStepSpansForID() spliced vector error = %v", err)
	}
	if len(stepSpans) != 2 || stepIndex != 1 {
		t.Fatalf("expected two spliced steps and second index, got spans=%#v index=%d", stepSpans, stepIndex)
	}
	replacement := `{:id :tools/second :type :http}`
	updated, err := replaceLocalStep(source, "tools/second", replacement)
	if err != nil {
		t.Fatalf("replaceLocalStep() spliced vector error = %v", err)
	}
	if !strings.Contains(updated, "#?@(:clj") || !strings.Contains(updated, ":type :http") || !strings.Contains(updated, ":id :tools/first") {
		t.Fatalf("replaceLocalStep() did not preserve spliced vector: %s", updated)
	}
	removed, err := removeLocalStep(source, "tools/second")
	if err != nil {
		t.Fatalf("removeLocalStep() spliced vector error = %v", err)
	}
	if !strings.Contains(removed, ":id :tools/first") || strings.Contains(removed, ":id :tools/second") || !strings.Contains(removed, "#?@(:clj") {
		t.Fatalf("removeLocalStep() did not preserve remaining spliced vector: %s", removed)
	}
	_, scheduleSpans, scheduleIndex, err := localScheduleSpansForID(source, "daily")
	if err != nil {
		t.Fatalf("localScheduleSpansForID() spliced vector error = %v", err)
	}
	if len(scheduleSpans) != 1 || scheduleIndex != 0 {
		t.Fatalf("expected one spliced schedule, got spans=%#v index=%d", scheduleSpans, scheduleIndex)
	}
	removedSchedule, err := removeLocalSchedule(source, "daily")
	if err != nil {
		t.Fatalf("removeLocalSchedule() spliced vector error = %v", err)
	}
	if !strings.Contains(removedSchedule, "#?@(:clj []") || strings.Contains(removedSchedule, ":id :daily") {
		t.Fatalf("removeLocalSchedule() did not preserve empty spliced vector: %s", removedSchedule)
	}

	listSource := `{:slug :order-sync
 :steps [#?@(:clj ({:id :tools/list-branch :type :function}))]}
`
	_, listSpans, listIndex, err := localStepSpansForID(listSource, "tools/list-branch")
	if err != nil {
		t.Fatalf("localStepSpansForID() spliced list error = %v", err)
	}
	if len(listSpans) != 1 || listIndex != 0 {
		t.Fatalf("expected one spliced list step, got spans=%#v index=%d", listSpans, listIndex)
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

func TestLocalStepRunCommandsRegisterTimeoutFlag(t *testing.T) {
	app := &App{WorkspaceID: "ws-test"}
	for name, cmd := range map[string]*cobra.Command{
		"flows steps run":    newFlowsStepsLocalRunCmd(app),
		"flows steps create": newFlowsStepsLocalCreateCmd(app),
		"flows steps update": newFlowsStepsLocalUpdateCmd(app),
		"flows init":         newFlowsInitCmd(app),
	} {
		flag := cmd.Flags().Lookup("timeout")
		if flag == nil {
			t.Fatalf("expected %s to register --timeout", name)
		}
		if flag.DefValue != "15m0s" {
			t.Fatalf("expected %s --timeout default of 15m0s (defaultFlowRunWaitTimeout), got %s", name, flag.DefValue)
		}
	}
}

func TestLocalStepRunHonorsTimeoutFlag(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	release := make(chan struct{})
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{}})
	}))
	defer srv.Close()
	// Unblock the handler before srv.Close (defers run LIFO) so shutdown does
	// not wait on the still-parked request.
	defer close(release)

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalRunCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--timeout", "100ms"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected a 100ms --timeout to abort the slow server call\n%s", out.String())
	}
	if !strings.Contains(out.String(), "timed out after 100ms") {
		t.Fatalf("expected a timeout error naming the --timeout bound, got:\n%s", out.String())
	}
}

func TestLocalStepRunRejectsNonPositiveTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	app := &App{WorkspaceID: "ws-acme", APIURL: "http://127.0.0.1:1", Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-acme"}), "order-sync", "--out", path)
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(&App{WorkspaceID: "ws-acme"}), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalRunCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--timeout", "0s"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected --timeout 0s to be rejected\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--timeout must be > 0") {
		t.Fatalf("expected --timeout validation error, got:\n%s", out.String())
	}
}

func TestLocalStepCreateFailureAfterSaveExplainsRecovery(t *testing.T) {
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

	for _, mode := range []string{"--push", "--run"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "order-sync.clj")
		stepPath := filepath.Join(dir, "add-one.edn")
		if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
		executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

		var out bytes.Buffer
		cmd := newFlowsStepsLocalCreateCmd(app)
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, mode})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("create %s failure returned nil\n%s", mode, out.String())
		}
		text := out.String()
		if !strings.Contains(text, `"savedLocally":true`) {
			t.Fatalf("create %s failure must report the step as saved locally: %s", mode, text)
		}
		if !strings.Contains(text, path) {
			t.Fatalf("create %s failure must include the local flow path: %s", mode, text)
		}
		// The flow file lives outside the default flows/<slug>.clj, so every
		// recovery command must carry --flow-file with the saved path. (JSON
		// escapes < and >, so match around the <step.edn> placeholder.)
		if !strings.Contains(text, "breyta flows steps update order-sync tools/add-one --step-file") || !strings.Contains(text, "--flow-file "+path) {
			t.Fatalf("create %s failure must point at flows steps update with --flow-file: %s", mode, text)
		}
		if !strings.Contains(text, "'steps update' in place of 'steps create'") {
			t.Fatalf("create %s failure hint must point at the shell-history retry: %s", mode, text)
		}
		// The step must actually be on disk so the recovery guidance is true.
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(saved), ":id :tools/add-one") {
			t.Fatalf("expected the step literal saved locally despite %s failure:\n%s", mode, saved)
		}
	}
}

func TestLocalStepUpdateRunFailureAfterSaveExplainsRecovery(t *testing.T) {
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
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalUpdateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", updatedStepPath, "--run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("update --run failure returned nil\n%s", out.String())
	}
	text := out.String()
	if !strings.Contains(text, `"savedLocally":true`) {
		t.Fatalf("update --run failure must report the step as saved locally: %s", text)
	}
	if !strings.Contains(text, path) {
		t.Fatalf("update --run failure must include the local flow path: %s", text)
	}
}

func TestLocalStepScaffoldIncludesPerTypeDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	app := &App{WorkspaceID: "ws-test"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/fetch", "--flow-file", path, "--type", "http")
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/transform", "--flow-file", path, "--type", "function")
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/compute", "--flow-file", path, "--type", "code")
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/summarize", "--flow-file", path, "--type", "llm")

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `:defaults {:method :get :url "https://example.com"}`) {
		t.Fatalf("http scaffold missing runnable :defaults:\n%s", text)
	}
	// The emitted quotes must be plain double quotes, never backslash-escaped:
	// a literal \" in the source file would make the scaffold malformed.
	if strings.Contains(text, `\"`) {
		t.Fatalf("scaffolded source must not contain backslash-escaped quotes:\n%s", text)
	}
	// Both function and its supported alias code get the identity-function defaults.
	if got := strings.Count(text, ":defaults {:code '(fn [input] input)}"); got != 2 {
		t.Fatalf("expected function AND code scaffolds to carry identity :defaults (got %d):\n%s", got, text)
	}
	if !strings.Contains(text, ":defaults {}") {
		t.Fatalf("generic scaffold missing explicit empty :defaults:\n%s", text)
	}
	if !strings.Contains(text, "fill :defaults with the step config") {
		t.Fatalf("generic scaffold description missing the fill-in note:\n%s", text)
	}

	// The scaffolded flow must survive the real scaffold-then-lint path: the
	// local linter parses the complete literal, so a malformed :defaults map
	// (for example escaped quotes) would surface as delimiter/parse errors.
	lintCmd := newFlowsLintCmd(app)
	var lintOut bytes.Buffer
	lintCmd.SetOut(&lintOut)
	lintCmd.SetErr(&lintOut)
	lintCmd.SetArgs([]string{"--file", path, "--local-only"})
	if err := lintCmd.Execute(); err != nil {
		t.Fatalf("scaffolded flow must pass local lint: %v\n%s", err, lintOut.String())
	}
}

func TestStepSaveFailureRecoveryFlowFileHandling(t *testing.T) {
	defaultPath := filepath.Join("flows", "order-sync.clj")
	rec := stepSaveFailureRecovery(true, false, false, defaultPath, "order-sync", "tools/add-one")
	for _, cmdText := range rec.nextCommands {
		if strings.Contains(cmdText, "--flow-file") {
			t.Fatalf("default flow path must not add --flow-file, got %#v", rec.nextCommands)
		}
	}
	if !strings.Contains(rec.hint, "'steps update' in place of 'steps create'") {
		t.Fatalf("create recovery hint must point at the shell-history retry, got %q", rec.hint)
	}

	customPath := "/tmp/elsewhere/order.clj"
	rec = stepSaveFailureRecovery(true, false, false, customPath, "order-sync", "tools/add-one")
	wantUpdate := "breyta flows steps update order-sync tools/add-one --step-file <step.edn> --flow-file " + customPath
	if len(rec.nextCommands) != 1 || rec.nextCommands[0] != wantUpdate {
		t.Fatalf("custom flow path must carry --flow-file on the single recovery command, got %#v", rec.nextCommands)
	}
	if rec.path != customPath {
		t.Fatalf("recovery must carry the saved path for the envelope, got %q", rec.path)
	}

	rec = stepSaveFailureRecovery(false, false, false, customPath, "order-sync", "tools/add-one")
	if len(rec.nextCommands) != 1 || rec.nextCommands[0] != wantUpdate {
		t.Fatalf("update recovery must keep the single update command, got %#v", rec.nextCommands)
	}
}

func TestLocalStepSaveTransportFailureEmitsStructuredEnvelope(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	for _, mode := range []string{"--push", "--run"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "order-sync.clj")
		stepPath := filepath.Join(dir, "add-one.edn")
		if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		// Nothing listens on port 1: the --push/--run call fails at the
		// transport layer (connection refused), after the local save.
		app := &App{WorkspaceID: "ws-acme", APIURL: "http://127.0.0.1:1", Token: "user-dev", DevMode: true}
		executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-acme"}), "order-sync", "--out", path)

		var out bytes.Buffer
		cmd := newFlowsStepsLocalCreateCmd(app)
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, mode})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("create %s transport failure returned nil\n%s", mode, out.String())
		}
		text := out.String()
		if !strings.Contains(text, `"ok":false`) {
			t.Fatalf("create %s transport failure must emit a structured failure envelope: %s", mode, text)
		}
		if !strings.Contains(text, `"savedLocally":true`) {
			t.Fatalf("create %s transport failure must report the step as saved locally: %s", mode, text)
		}
		if !strings.Contains(text, `"nextCommands"`) ||
			!strings.Contains(text, "breyta flows steps update order-sync tools/add-one --step-file") ||
			!strings.Contains(text, "--flow-file "+path) {
			t.Fatalf("create %s transport failure must carry recovery nextCommands: %s", mode, text)
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(saved), ":id :tools/add-one") {
			t.Fatalf("expected the step literal saved locally despite %s transport failure:\n%s", mode, saved)
		}
	}
}

func TestStepSaveFailureRecoveryShellQuotesSpacedPaths(t *testing.T) {
	spacedPath := "/tmp/My Flows/order sync.clj"
	rec := stepSaveFailureRecovery(true, false, false, spacedPath, "order-sync", "tools/add-one")
	wantUpdate := "breyta flows steps update order-sync tools/add-one --step-file <step.edn> --flow-file '/tmp/My Flows/order sync.clj'"
	if len(rec.nextCommands) != 1 || rec.nextCommands[0] != wantUpdate {
		t.Fatalf("spaced path must be shell-quoted in the recovery command, got %#v", rec.nextCommands)
	}

	// A path with an embedded single quote must stay a valid shell word.
	quoted := stepSaveFailureRecovery(true, false, false, "/tmp/it's here.clj", "order-sync", "tools/add-one")
	if want := `--flow-file '/tmp/it'"'"'s here.clj'`; !strings.Contains(quoted.nextCommands[0], want) {
		t.Fatalf("single quotes in the path must be escaped, got %#v", quoted.nextCommands)
	}
}

func TestLocalStepCreateRejectsMalformedRunParamsBeforeSaving(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, "--run", "--params", "{not json"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("malformed --params must fail the create\n%s", out.String())
	}
	if !strings.Contains(out.String(), "the local flow file was not modified") {
		t.Fatalf("expected pre-save params validation error, got:\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("malformed --params must not write the step to the flow file:\n%s", current)
	}

	// Same guarantee for update: the previous step body must stay untouched.
	executeLocalAuthoringJSON(t, newFlowsStepsLocalCreateCmd(app), "order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath)
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updatedStepPath := filepath.Join(dir, "add-two.edn")
	if err := os.WriteFile(updatedStepPath, []byte(`{:id :tools/add-one :type :function :description "Add two"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	cmd = newFlowsStepsLocalUpdateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", updatedStepPath, "--run", "--params", "{not json"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("malformed --params must fail the update\n%s", out.String())
	}
	current, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(saved) {
		t.Fatalf("malformed --params must not rewrite the flow file on update:\n%s", current)
	}

	// And for init --run: no flow file may be created at all.
	initPath := filepath.Join(dir, "fresh-flow.clj")
	out.Reset()
	initCmd := newFlowsInitCmd(app)
	initCmd.SetOut(&out)
	initCmd.SetErr(&out)
	initCmd.SetArgs([]string{"fresh-flow", "--out", initPath, "--step-id", "tools/add-one", "--step-file", stepPath, "--run", "--params", "{not json"})
	if err := initCmd.Execute(); err == nil {
		t.Fatalf("malformed --params must fail init --run\n%s", out.String())
	}
	if _, err := os.Stat(initPath); !os.IsNotExist(err) {
		t.Fatalf("malformed --params must not create the flow file (stat err=%v)", err)
	}
}

func TestShellQuoteIfNeededQuotesHistoryExpansion(t *testing.T) {
	if got := shellQuoteIfNeeded("/tmp/wow!echo.clj"); got != "'/tmp/wow!echo.clj'" {
		t.Fatalf("'!' must trigger quoting (bash history expansion), got %q", got)
	}
	if got := shellQuoteIfNeeded("/tmp/plain/order-sync_v2.clj"); got != "/tmp/plain/order-sync_v2.clj" {
		t.Fatalf("allowlisted plain path must stay unquoted, got %q", got)
	}
	rec := stepSaveFailureRecovery(true, false, false, "/tmp/wow!echo.clj", "order-sync", "tools/add-one")
	if want := "--flow-file '/tmp/wow!echo.clj'"; !strings.Contains(rec.nextCommands[0], want) {
		t.Fatalf("recovery command must quote '!' paths, got %#v", rec.nextCommands)
	}
}

func TestLocalStepCreateRejectsNonPositiveTimeoutBeforeSaving(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, "--run", "--timeout", "0s"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("--run --timeout 0s must fail the create\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--timeout must be > 0; the local flow file was not modified") {
		t.Fatalf("expected pre-save timeout validation error, got:\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("bad --timeout must not write the step to the flow file:\n%s", current)
	}
}

func TestFlowsInitForceRejectsNonPositiveTimeoutBeforeOverwriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme"}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path, "--description", "keep me")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newFlowsInitCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "--out", path, "--force", "--step-id", "tools/add-one", "--step-file", stepPath, "--run", "--timeout", "-5s"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("init --force --run with negative --timeout must fail\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--timeout must be > 0; no local flow file was written") {
		t.Fatalf("expected pre-write timeout validation error, got:\n%s", out.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != string(original) {
		t.Fatalf("bad --timeout must leave the existing flow file untouched under --force:\n%s", current)
	}
}

func TestLocalStepSaveFailureMergesServerHint(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error":       map[string]any{"message": "step validation failed"},
			"meta":        map[string]any{"hint": "Server-side setup is incomplete."},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, "--run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected --run failure\n%s", out.String())
	}
	text := out.String()
	if !strings.Contains(text, "Server-side setup is incomplete.") {
		t.Fatalf("server hint must be retained: %s", text)
	}
	if !strings.Contains(text, "'steps update' in place of 'steps create'") {
		t.Fatalf("local-save recovery hint must be merged alongside the server hint: %s", text)
	}
}

func TestLocalStepPushFailureWithPendingRunSaysRunDidNotHappen(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	var commands []string
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		command, _ := got["command"].(string)
		commands = append(commands, command)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"workspaceId": "ws-acme",
			"error":       map[string]any{"message": "draft rejected"},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, "--push", "--run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected --push failure\n%s", out.String())
	}
	if !strings.Contains(out.String(), "The requested --run did NOT happen.") {
		t.Fatalf("push failure with a pending --run must state the run never happened: %s", out.String())
	}
	for _, command := range commands {
		if command == "steps.run" {
			t.Fatalf("the run must not execute after a failed push, got commands %#v", commands)
		}
	}
}

func TestLocalStepPushNullEnvelopeStillCarriesRecoveryMetadata(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	stepPath := filepath.Join(dir, "add-one.edn")
	if err := os.WriteFile(stepPath, []byte(`{:id :tools/add-one :type :function :description "Add one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

	for _, mode := range []string{"--push", "--run"} {
		var out bytes.Buffer
		cmd := newFlowsStepsLocalCreateCmd(app)
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"order-sync", "tools/add-one", "--flow-file", path, "--step-file", stepPath, mode})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("null envelope %s failure returned nil\n%s", mode, out.String())
		}
		text := out.String()
		if !strings.Contains(text, `"savedLocally":true`) || !strings.Contains(text, `"nextCommands"`) || !strings.Contains(text, `"localPath"`) {
			t.Fatalf("null envelope %s failure must still carry recovery metadata: %s", mode, text)
		}
		// Reset the flow file for the second mode: remove the created step.
		executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-acme"}), "order-sync", "--out", path, "--force")
	}
}

func TestLocalStepScaffoldCreateFailureSuggestsStepsRun(t *testing.T) {
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

	dir := t.TempDir()
	path := filepath.Join(dir, "order-sync.clj")
	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	executeLocalAuthoringJSON(t, newFlowsInitCmd(app), "order-sync", "--out", path)

	var out bytes.Buffer
	cmd := newFlowsStepsLocalCreateCmd(app)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"order-sync", "tools/fetch", "--flow-file", path, "--type", "http", "--run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("scaffold create --run failure returned nil\n%s", out.String())
	}
	text := out.String()
	// A create-to-update substitution would carry create-only flags that
	// update rejects; scaffolded creates get edit-then-run guidance instead.
	if !strings.Contains(text, "breyta flows steps run order-sync tools/fetch --flow-file "+path) {
		t.Fatalf("scaffold recovery must suggest flows steps run: %s", text)
	}
	if !strings.Contains(text, "scaffolded step") || !strings.Contains(text, "edit it in the flow file") {
		t.Fatalf("scaffold recovery hint must point at editing the flow file: %s", text)
	}
	if strings.Contains(text, "steps update' in place of") {
		t.Fatalf("scaffold recovery must not suggest the create-to-update substitution: %s", text)
	}
}
