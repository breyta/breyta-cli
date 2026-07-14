package cli_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runFlowsAgentsCapture(t *testing.T, responseData map[string]any, args ...string) (map[string]any, string, string, error) {
	t.Helper()
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")

	var got map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		if responseData == nil {
			responseData = map[string]any{
				"flowSlug": "my-flow",
				"source":   "draft",
				"stepId":   "agents/reviewer",
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        responseData,
		})
	}))
	defer srv.Close()

	baseArgs := []string{
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
	}
	stdout, stderr, err := runCLIArgs(t, append(baseArgs, args...)...)
	return got, stdout, stderr, err
}

func TestFlowsAgentsCreateSendsAgentStepCreatePayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "create", "my-flow", "agents/reviewer",
		"--title", "Review changes",
		"--description", "Review pull request changes",
	)
	if err != nil {
		t.Fatalf("flows agents create failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.create" {
		t.Fatalf("expected flows.steps.create, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "agents/reviewer" {
		t.Fatalf("expected flow-scoped agent create args, got %#v", args)
	}
	if args["type"] != "agent" || args["title"] != "Review changes" || args["description"] != "Review pull request changes" {
		t.Fatalf("expected agent scaffold payload, got %#v", args)
	}
	if _, exists := args["expectedStepType"]; exists {
		t.Fatalf("expected create payload not to require an existing agent step, got %#v", args)
	}
}

func TestFlowsAgentsUpdateSendsCanonicalStepUpdatePayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "update", "my-flow", "agents/reviewer",
		"defaults.model", "gpt-5.4",
		"defaults.maxIterations", "8",
	)
	if err != nil {
		t.Fatalf("flows agents update failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["expectedStepType"] != "agent" {
		t.Fatalf("expected agent update precondition, got %#v", args)
	}
	edits, _ := args["edits"].([]any)
	if len(edits) != 2 {
		t.Fatalf("expected two edits, got %#v", args["edits"])
	}
	first, _ := edits[0].(map[string]any)
	second, _ := edits[1].(map[string]any)
	if first["path"] != "defaults.model" || first["value"] != "gpt-5.4" {
		t.Fatalf("expected first edit payload, got %#v", first)
	}
	if second["path"] != "defaults.maxIterations" || second["value"] != "8" {
		t.Fatalf("expected second edit payload, got %#v", second)
	}
}

func TestFlowsAgentsUpdateIncludesResultFunctionFileContents(t *testing.T) {
	fnFile := filepath.Join(t.TempDir(), "projection.clj")
	if err := os.WriteFile(fnFile, []byte(`(fn [result] {:summary (:summary result)})`), 0o600); err != nil {
		t.Fatalf("write function file: %v", err)
	}

	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "update", "my-flow", "agents/reviewer",
		"result.fnFile", fnFile,
	)
	if err != nil {
		t.Fatalf("flows agents update failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	edits, _ := args["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", args["edits"])
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "result.fnFile" || edit["value"] != fnFile {
		t.Fatalf("expected result.fnFile path/value edit, got %#v", edit)
	}
	if edit["fileContents"] != "(fn [result] {:summary (:summary result)})" {
		t.Fatalf("expected function file contents, got %#v", edit["fileContents"])
	}
}

func TestFlowsAgentsUpdateRejectsConflictingOrBlankFileMode(t *testing.T) {
	emptyFile := filepath.Join(t.TempDir(), "empty-agent.edn")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write empty agent file: %v", err)
	}

	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "blank file with edits",
			args:    []string{"flows", "agents", "update", "my-flow", "agents/reviewer", "--file", "", "defaults.model", "gpt-5.4"},
			wantErr: "--file cannot be combined with path/value edits",
		},
		{
			name:    "blank explicit file",
			args:    []string{"flows", "agents", "update", "my-flow", "agents/reviewer", "--file", ""},
			wantErr: "--file requires a non-empty path",
		},
		{
			name:    "empty file contents",
			args:    []string{"flows", "agents", "update", "my-flow", "agents/reviewer", "--file", emptyFile},
			wantErr: "--file must contain a non-empty packaged agent step literal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, stdout, stderr, err := runFlowsAgentsCapture(t, nil, tc.args...)
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, err=%v\nstdout=%s\nstderr=%s", tc.wantErr, err, stdout, stderr)
			}
			if got != nil {
				t.Fatalf("expected validation before API request, got %#v", got)
			}
		})
	}
}

func TestFlowsAgentsToolsSetSendsCanonicalToolsetEdit(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "tools", "set", "my-flow", "agents/reviewer",
		"--step", "team.github/open-pr",
		"--step", "search",
	)
	if err != nil {
		t.Fatalf("flows agents tools set failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["expectedStepType"] != "agent" {
		t.Fatalf("expected agent tools precondition, got %#v", args)
	}
	edits, _ := args["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", args["edits"])
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "defaults.tools.steps" || edit["valueLiteral"] != "[:team.github/open-pr :search]" {
		t.Fatalf("expected toolset valueLiteral edit, got %#v", edit)
	}
}

func TestFlowsAgentsToolsSetClearRevokesAllTools(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "tools", "set", "my-flow", "agents/reviewer",
		"--clear",
	)
	if err != nil {
		t.Fatalf("flows agents tools set failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	args, _ := got["args"].(map[string]any)
	if args["expectedStepType"] != "agent" {
		t.Fatalf("expected agent tools precondition, got %#v", args)
	}
	edits, _ := args["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", args["edits"])
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "defaults.tools.steps" || edit["valueLiteral"] != "[]" {
		t.Fatalf("expected empty toolset valueLiteral edit, got %#v", edit)
	}
}

func TestFlowsAgentsToolsSetRequiresStepsOrClear(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "tools", "set", "my-flow", "agents/reviewer",
	)
	if err == nil {
		t.Fatalf("expected missing --step/--clear to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if got != nil {
		t.Fatalf("expected validation before the API request, got %#v", got)
	}
}

func TestFlowsAgentsRunSendsCanonicalStepRunPayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t,
		map[string]any{
			"flowSlug": "my-flow",
			"source":   "draft",
			"stepId":   "agents/reviewer",
			"stepType": "agent",
			"result":   map[string]any{"answer": "ok"},
		},
		"flows", "agents", "run", "my-flow", "agents/reviewer",
		"--source", "draft",
		"--params", `{"task":"review"}`,
		"--idempotency-key", "review-123",
	)
	if err != nil {
		t.Fatalf("flows agents run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.run" {
		t.Fatalf("expected flows.steps.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	params, _ := args["params"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "agents/reviewer" {
		t.Fatalf("expected flow-scoped run args, got %#v", args)
	}
	if params["task"] != "review" {
		t.Fatalf("expected params to be forwarded, got %#v", params)
	}
	if args["idempotencyKey"] != "review-123" {
		t.Fatalf("expected idempotency key to be forwarded, got %#v", args)
	}
	if args["expectedStepType"] != "agent" {
		t.Fatalf("expected agent run precondition, got %#v", args)
	}
}

func TestFlowsAgentsChecksRunSendsTargetPayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t,
		map[string]any{
			"flowSlug": "my-flow",
			"source":   "draft",
			"ready":    true,
			"target": map[string]any{
				"kind":   "agent",
				"stepId": "agents/reviewer",
			},
		},
		"flows", "agents", "checks", "run", "my-flow", "agents/reviewer",
		"--category", "security",
	)
	if err != nil {
		t.Fatalf("flows agents checks run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.agents.checks.run" {
		t.Fatalf("expected flows.agents.checks.run, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "agents/reviewer" || args["category"] != "security" {
		t.Fatalf("expected agent target check args, got %#v", args)
	}
}

func TestFlowsAgentsRemoveSendsCanonicalStepRemovePayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t,
		map[string]any{
			"flowSlug": "my-flow",
			"source":   "draft",
			"stepId":   "agents/reviewer",
			"removed":  true,
		},
		"flows", "agents", "remove", "my-flow", "agents/reviewer",
	)
	if err != nil {
		t.Fatalf("flows agents remove failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.remove" {
		t.Fatalf("expected flows.steps.remove, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	if args["flowSlug"] != "my-flow" || args["source"] != "draft" || args["stepId"] != "agents/reviewer" {
		t.Fatalf("expected flow-scoped remove args, got %#v", args)
	}
	if args["expectedStepType"] != "agent" {
		t.Fatalf("expected agent remove precondition, got %#v", args)
	}
}
