package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
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
						map[string]any{"stepId": "canceled-ok", "status": "canceled"},
						map[string]any{"stepId": "canceled-bad", "status": "canceled", "errorMessage": "Canceled after upstream failure"},
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
	if len(steps) != 2 {
		t.Fatalf("expected failed/error-bearing steps only, got %#v", steps)
	}
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	if first["stepId"] != "bad" || second["stepId"] != "canceled-bad" {
		t.Fatalf("unexpected error steps: %#v", steps)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["errorsOnly"] != true {
		t.Fatalf("expected errorsOnly meta, got %#v", meta)
	}
	nextCommands, _ := meta["nextCommands"].([]any)
	var nextCommandStrings []string
	for _, item := range nextCommands {
		if s, ok := item.(string); ok {
			nextCommandStrings = append(nextCommandStrings, s)
		}
	}
	joined := strings.Join(nextCommandStrings, "\n")
	if !strings.Contains(joined, "breyta resources workflow list wf-1") {
		t.Fatalf("expected canonical workflow resource list command, got %q", joined)
	}
	if strings.Contains(joined, "breyta resources workflow wf-1") {
		t.Fatalf("expected nextCommands not to omit workflow list subcommand, got %q", joined)
	}
}

func TestRunsInspect_CompactsLargeHTMLErrorBody(t *testing.T) {
	largeHTML := "<html>" + strings.Repeat("challenge", 300) + "</html>"
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"run": map[string]any{
					"workflowId": "wf-1",
					"status":     "failed",
					"error":      map[string]any{"message": largeHTML, "status": 403, "contentType": "text/html"},
					"steps": []any{
						map[string]any{"stepId": "fetch", "status": "failed", "error": map[string]any{"message": largeHTML, "status": 403, "contentType": "text/html"}},
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
		"runs", "inspect", "wf-1",
	)
	if err != nil {
		t.Fatalf("runs inspect failed: %v\n%s", err, stdout)
	}
	if strings.Contains(stdout, largeHTML) {
		t.Fatalf("expected large HTML error body to be summarized, got:\n%s", stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	run, _ := data["run"].(map[string]any)
	errMap, _ := run["error"].(map[string]any)
	message, _ := errMap["message"].(map[string]any)
	if message["truncated"] != true {
		t.Fatalf("expected compact truncated run error, got %#v", errMap)
	}
}
