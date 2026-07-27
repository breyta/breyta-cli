package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestLocalStepScaffoldIncludesExecutableDefaults(t *testing.T) {
	tests := []struct {
		stepType string
		wants    []string
	}{
		{"http", []string{":defaults", ":method :get", `:url "https://example.com"`}},
		{"function", []string{":defaults", ":code '(fn [input] input)"}},
		{"llm", []string{":defaults", ":connection nil", `:model "gpt-5.4"`, ":prompt"}},
	}
	for _, tc := range tests {
		t.Run(tc.stepType, func(t *testing.T) {
			literal := localStepScaffold("tools/example", tc.stepType, "Example")
			for _, want := range tc.wants {
				if !strings.Contains(literal, want) {
					t.Fatalf("%s scaffold missing %q:\n%s", tc.stepType, want, literal)
				}
			}
		})
	}
}

func TestFlowsInitSeedsInvocationInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input-flow.clj")
	executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-test"}),
		"input-flow", "--out", path,
		"--input", "site-url:text:required:Site URL",
		"--input", "notes:textarea:optional:Notes")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		`:name :site-url :type :text :label "Site URL" :required true`,
		`:name :notes :type :textarea :label "Notes" :required false`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("initialized source missing %q:\n%s", want, text)
		}
	}
}

func TestLocalInvocationInputsRejectDuplicateNames(t *testing.T) {
	if _, err := localInvocationInputsLiteral([]string{"site-url:text", "site-url:textarea"}); err == nil ||
		!strings.Contains(err.Error(), "duplicate invocation input name") {
		t.Fatalf("expected duplicate input rejection, got %v", err)
	}
}

func TestLocalStepRunCommandsDefaultToFlowWaitTimeout(t *testing.T) {
	for name, cmd := range map[string]*cobra.Command{
		"init":   newFlowsInitCmd(&App{}),
		"create": newFlowsStepsLocalCreateCmd(&App{}),
		"update": newFlowsStepsLocalUpdateCmd(&App{}),
		"run":    newFlowsStepsLocalRunCmd(&App{}),
	} {
		flag := cmd.Flags().Lookup("timeout")
		if flag == nil {
			t.Fatalf("%s missing --timeout", name)
		}
		if got := flag.DefValue; got != defaultFlowRunWaitTimeout.String() {
			t.Fatalf("%s timeout default = %s, want %s", name, got, defaultFlowRunWaitTimeout)
		}
	}
}

func TestFlowsStepsRunRoutesInlineStepIDsToRunStep(t *testing.T) {
	cmd := newFlowsStepsLocalRunCmd(&App{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"pitch-studio", "build-copy-request"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unqualified packaged-step id rejection")
	}
	if got := out.String(); !strings.Contains(got, "flows run-step pitch-studio build-copy-request") ||
		!strings.Contains(got, "bound :llm") {
		t.Fatalf("missing canonical inline-step guidance:\n%s", got)
	}
}

func TestFlowRunStepHelpExplainsCanonicalDraftProbe(t *testing.T) {
	help := newFlowsRunStepCmd(&App{}).Long
	for _, want := range []string{"canonical authoring-time probe", "draft-bound connection slots", "do not wrap it"} {
		if !strings.Contains(help, want) {
			t.Fatalf("run-step help missing %q:\n%s", want, help)
		}
	}
}

func TestFlowsStepsCreatePushFailureReportsPersistedLocalStep(t *testing.T) {
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	path := filepath.Join(t.TempDir(), "order-sync.clj")
	executeLocalAuthoringJSON(t, newFlowsInitCmd(&App{WorkspaceID: "ws-test"}), "order-sync", "--out", path)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": map[string]any{"message": "invalid packaged step"},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "user-dev", DevMode: true}
	cmd := newFlowsStepsLocalCreateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"order-sync", "tools/fetch", "--flow-file", path, "--type", "http", "--push"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected rejected push")
	}
	var envelope map[string]any
	if err := json.NewDecoder(strings.NewReader(out.String())).Decode(&envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	meta := mapStringAny(envelope["meta"])
	if meta["saved"] != true || meta["path"] != path {
		t.Fatalf("expected persisted-local metadata, got %#v", meta)
	}
	if !strings.Contains(firstNonBlankString(meta["hint"]), "step saved locally") {
		t.Fatalf("expected explicit saved-local hint, got %#v", meta)
	}
	hint := firstNonBlankString(meta["hint"])
	if strings.Contains(hint, "<step-file>") || !strings.Contains(hint, path) {
		t.Fatalf("expected recovery command to preserve the saved path without placeholders: %q", hint)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), ":id :tools/fetch") {
		t.Fatalf("rejected step was not retained locally:\n%s", source)
	}
}

func TestSavedRunTransportFailureRequiresReconciliation(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := writeSavedLocalAuthoringFailure(cmd, &App{}, nil, 0, errors.New("timeout"),
		"/tmp/custom flow.clj", "orders", "tools/send", "run", "stable-key")
	if err == nil || !strings.Contains(err.Error(), "Reconcile before retrying") ||
		!strings.Contains(err.Error(), "--idempotency-key") ||
		strings.Contains(err.Error(), "server rejected") {
		t.Fatalf("expected ambiguity-safe recovery guidance, got %v", err)
	}
}

func TestSavedRunServerFailureRequiresReconciliation(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	apiOut := map[string]any{
		"ok":    false,
		"error": map[string]any{"message": "internal error"},
	}
	err := writeSavedLocalAuthoringFailure(cmd, &App{}, apiOut, 500, nil,
		"/tmp/flow.clj", "orders", "tools/send", "run", "stable-key")
	if err == nil {
		t.Fatal("expected server failure")
	}
	if hint := firstNonBlankString(mapStringAny(apiOut["meta"])["hint"]); !strings.Contains(hint, "Reconcile before retrying") {
		t.Fatalf("expected ambiguity-safe 5xx hint, got %q", hint)
	}
}

func TestSavedRunNilServerFailureDoesNotPanic(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := writeSavedLocalAuthoringFailure(cmd, &App{}, nil, 500, nil,
		"/tmp/flow.clj", "orders", "tools/send", "run", "stable-key")
	if err == nil || !strings.Contains(err.Error(), "Reconcile before retrying") {
		t.Fatalf("expected safe nil-envelope failure, got %v", err)
	}
}

func TestSavedInitPushFailureOmitsStepRunWithoutSeededStep(t *testing.T) {
	cmd := &cobra.Command{}
	apiOut := map[string]any{
		"ok":    false,
		"error": map[string]any{"message": "invalid flow"},
	}
	_ = writeSavedLocalAuthoringFailure(cmd, &App{}, apiOut, 400, nil,
		"/tmp/flow.clj", "orders", "", "push", "")
	commands, _ := mapStringAny(apiOut["meta"])["nextCommands"].([]any)
	if len(commands) != 1 || firstNonBlankString(commands[0]) != "breyta flows push --file /tmp/flow.clj" {
		t.Fatalf("expected only the valid flow push retry, got %#v", commands)
	}
}

func TestSavedRunRecoveryPreservesChangedFlags(t *testing.T) {
	cmd := newFlowsStepsLocalCreateCmd(&App{})
	_ = cmd.Flags().Set("params", `{"orderId":"123"}`)
	_ = cmd.Flags().Set("profile-id", "profile-1")
	_ = cmd.Flags().Set("idempotency-key", "stable-key")
	_ = cmd.Flags().Set("timeout", "20m")
	apiOut := map[string]any{"ok": false, "error": map[string]any{"message": "invalid"}}
	_ = writeSavedLocalAuthoringFailure(cmd, &App{}, apiOut, 400, nil,
		"/tmp/custom flow.clj", "orders", "tools/send", "run", "stable-key")
	hint := firstNonBlankString(mapStringAny(apiOut["meta"])["hint"])
	for _, want := range []string{"--profile-id", "profile-1", "--params", "orderId", "--idempotency-key", "stable-key", "--timeout", "20m"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("recovery hint missing %q: %s", want, hint)
		}
	}
}

func TestShellQuotePathQuotesMetacharacters(t *testing.T) {
	for _, path := range []string{"flows/order;backup.clj", "flows/a&b.clj", "flows/*.clj", "flows/`cmd`.clj"} {
		quoted := shellQuotePath(path)
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Fatalf("unsafe path was not quoted: %q -> %q", path, quoted)
		}
	}
}

func TestRunLocalFlowStepRejectsNonPositiveTimeout(t *testing.T) {
	_, _, err := runLocalFlowStep(nil, &App{Token: "token", APIURL: "https://example.invalid"}, "flow", "flow.clj", "{}", "tools/x", nil, "", "", 0*time.Second)
	if err == nil || !strings.Contains(err.Error(), "--timeout must be > 0") {
		t.Fatalf("expected timeout validation, got %v", err)
	}
}
