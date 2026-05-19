package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRunsReplayAPIModeCallsRunsReplay(t *testing.T) {
	replayCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		if cmd != "runs.replay" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		replayCalls++
		if args["workflowId"] != "wf-old" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"workflowId":         "wf-new",
				"originalWorkflowId": "wf-old",
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "replay", "wf-old",
	)
	if err != nil {
		t.Fatalf("runs replay failed: %v\n%s", err, stdout)
	}
	if replayCalls != 1 {
		t.Fatalf("expected 1 runs.replay call, got %d", replayCalls)
	}
	if strings.Contains(stdout, "not_implemented") {
		t.Fatalf("runs replay still returned not_implemented:\n%s", stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	data, _ := out["data"].(map[string]any)
	if data["workflowId"] != "wf-new" || data["originalWorkflowId"] != "wf-old" {
		t.Fatalf("unexpected replay output data: %+v", data)
	}
}

func TestRunsRetryAPIModeUsesReplayForWholeRun(t *testing.T) {
	replayCalls := 0
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		if cmd != "runs.replay" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		replayCalls++
		if args["workflowId"] != "wf-old" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data":        map[string]any{"workflowId": "wf-new"},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "retry", "wf-old",
	)
	if err != nil {
		t.Fatalf("runs retry failed: %v\n%s", err, stdout)
	}
	if replayCalls != 1 {
		t.Fatalf("expected 1 runs.replay call, got %d", replayCalls)
	}
	if strings.Contains(stdout, "not_implemented") {
		t.Fatalf("runs retry still returned not_implemented:\n%s", stdout)
	}
}
