package cli_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
)

func TestFlowsRunRecordsProactiveActivity(t *testing.T) {
	var mu sync.Mutex
	var activity map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.run" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["flowSlug"] != "customer-import" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"run": map[string]any{"workflowId": "wf-customer-import", "status": "running"}},
			})
		case "/api/proactive-agent/activity":
			if got := r.Header.Get("X-Breyta-Workspace"); got != "ws-acme" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "missing workspace header"})
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			activity = body
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "customer-import",
	)
	if err != nil {
		t.Fatalf("flows run failed: %v\n%s", err, stdout)
	}

	mu.Lock()
	defer mu.Unlock()
	if activity == nil {
		t.Fatal("expected proactive activity request")
	}
	if activity["source"] != "cli" || activity["kind"] != "flow-run" || activity["flowSlug"] != "customer-import" {
		t.Fatalf("unexpected activity payload: %#v", activity)
	}
	if activity["workflowId"] != "wf-customer-import" {
		t.Fatalf("expected workflow id in activity payload, got %#v", activity)
	}
}

func TestFlowsArchiveIgnoresProactiveActivityFailure(t *testing.T) {
	var activityCalls int

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["command"] != "flows.archive" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
				return
			}
			args, _ := body["args"].(map[string]any)
			if args["flowSlug"] != "customer-import" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"archived": true, "flowSlug": "customer-import"},
			})
		case "/api/proactive-agent/activity":
			activityCalls++
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "activity unavailable"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "archive", "customer-import",
	)
	if err != nil {
		t.Fatalf("flows archive should ignore proactive activity failure: %v\n%s", err, stdout)
	}
	if activityCalls != 1 {
		t.Fatalf("expected one proactive activity attempt, got %d", activityCalls)
	}
}
