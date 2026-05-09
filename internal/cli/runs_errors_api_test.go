package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRunsShowErrors_FiltersFailedSteps(t *testing.T) {
	var gotArgs map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["command"] != "runs.get" {
			t.Fatalf("expected runs.get, got %#v", body["command"])
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-1",
					"status":     "failed",
					"error":      map[string]any{"message": "Run failed"},
					"steps": []any{
						map[string]any{"stepId": "ok", "status": "completed"},
						map[string]any{"stepId": "bad", "status": "failed", "error": map[string]any{"message": "Boom"}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "show", "wf-1",
		"--errors",
	)
	if err != nil {
		t.Fatalf("runs show --errors failed: %v\n%s", err, stdout)
	}
	if gotArgs["includeSteps"] != true {
		t.Fatalf("expected includeSteps=true, got %#v", gotArgs["includeSteps"])
	}
	if gotArgs["includeResult"] != false {
		t.Fatalf("expected includeResult=false, got %#v", gotArgs["includeResult"])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	steps, _ := run["steps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("expected one failed step, got %#v", steps)
	}
	step, _ := steps[0].(map[string]any)
	if step["stepId"] != "bad" {
		t.Fatalf("unexpected failed step: %#v", step)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["errorsOnly"] != true {
		t.Fatalf("expected errorsOnly meta, got %#v", meta)
	}
}
