package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunsCancel_ResolvesShortIDWithFlowFilter(t *testing.T) {
	listCalls := 0
	cancelCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch cmd {
		case "runs.list":
			listCalls++
			if args["flowSlug"] != "fiken-email-receipts" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "missing flowSlug"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"items": []any{
						map[string]any{"workflowId": "flow-fiken-email-receipts-ws-acme-r34", "flowSlug": "fiken-email-receipts"},
					},
				},
				"meta": map[string]any{"hasMore": false},
			})
		case "runs.cancel":
			cancelCalls++
			if args["workflowId"] != "flow-fiken-email-receipts-ws-acme-r34" {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data":        map[string]any{"workflowId": args["workflowId"], "cancelled": true},
			})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "cancel", "r34",
		"--flow", "fiken-email-receipts",
		"--reason", "user requested cancellation",
	)
	if err != nil {
		t.Fatalf("runs cancel failed: %v\n%s", err, stdout)
	}
	if listCalls != 1 {
		t.Fatalf("expected 1 runs.list call, got %d", listCalls)
	}
	if cancelCalls != 1 {
		t.Fatalf("expected 1 runs.cancel call, got %d", cancelCalls)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["resolvedFrom"] != "r34" {
		t.Fatalf("expected meta.resolvedFrom=r34, got: %+v", meta)
	}
}

func TestRunsCancel_ShortIDAmbiguousReturnsError(t *testing.T) {
	cancelCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		switch cmd {
		case "runs.list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-acme",
				"data": map[string]any{
					"items": []any{
						map[string]any{"workflowId": "flow-a-ws-acme-r34"},
						map[string]any{"workflowId": "flow-b-ws-acme-r34"},
					},
				},
				"meta": map[string]any{"hasMore": false},
			})
		case "runs.cancel":
			cancelCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme"})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	_, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "cancel", "r34",
	)
	if err == nil {
		t.Fatalf("expected runs cancel to fail for ambiguous short id")
	}
	if cancelCalls != 0 {
		t.Fatalf("expected no runs.cancel call when id is ambiguous, got %d", cancelCalls)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Fatalf("expected ambiguous error, got stderr:\n%s", stderr)
	}
}

func TestRunsCancel_FullWorkflowIDSkipsResolution(t *testing.T) {
	listCalls := 0
	cancelCalls := 0
	fullID := "flow-fiken-email-receipts-ws-acme-r34"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		cmd, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch cmd {
		case "runs.list":
			listCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme"})
		case "runs.cancel":
			cancelCalls++
			if args["workflowId"] != fullID {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected workflowId"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "workspaceId": "ws-acme", "data": map[string]any{"cancelled": true}})
		default:
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "cancel", fullID,
	)
	if err != nil {
		t.Fatalf("runs cancel failed: %v\n%s", err, stdout)
	}
	if listCalls != 0 {
		t.Fatalf("expected no runs.list calls for full workflow id, got %d", listCalls)
	}
	if cancelCalls != 1 {
		t.Fatalf("expected 1 runs.cancel call, got %d", cancelCalls)
	}
}
