package cli_test

import (
	"encoding/json"
	"net/http"
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
}

func TestFlowsAgentsUpdateSendsCanonicalStepUpdatePayload(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "update", "my-flow", "agents/reviewer",
		"runner.model", "gpt-5.4",
		"limits.maxIterations", "8",
	)
	if err != nil {
		t.Fatalf("flows agents update failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	edits, _ := args["edits"].([]any)
	if len(edits) != 2 {
		t.Fatalf("expected two edits, got %#v", args["edits"])
	}
	first, _ := edits[0].(map[string]any)
	second, _ := edits[1].(map[string]any)
	if first["path"] != "runner.model" || first["value"] != "gpt-5.4" {
		t.Fatalf("expected first edit payload, got %#v", first)
	}
	if second["path"] != "limits.maxIterations" || second["value"] != "8" {
		t.Fatalf("expected second edit payload, got %#v", second)
	}
}

func TestFlowsAgentsToolsSetSendsCanonicalToolsetEdit(t *testing.T) {
	got, stdout, stderr, err := runFlowsAgentsCapture(t, nil,
		"flows", "agents", "tools", "set", "my-flow", "agents/reviewer",
		"--step", "tools/search",
		"--step", "tools/load",
	)
	if err != nil {
		t.Fatalf("flows agents tools set failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if got["command"] != "flows.steps.update" {
		t.Fatalf("expected flows.steps.update, got %#v", got["command"])
	}
	args, _ := got["args"].(map[string]any)
	edits, _ := args["edits"].([]any)
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", args["edits"])
	}
	edit, _ := edits[0].(map[string]any)
	if edit["path"] != "defaults.tools.steps" || edit["valueLiteral"] != "[:tools/search :tools/load]" {
		t.Fatalf("expected toolset valueLiteral edit, got %#v", edit)
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
}
