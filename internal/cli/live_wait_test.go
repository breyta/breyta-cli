package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFlowsRunLiveBootstrapsRealtimeAndRendersProgress(t *testing.T) {
	var baseURL string
	var bootstrapCalls int64
	var snapshotCalls int64
	var streamCalls int64

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/commands":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			command, _ := body["command"].(string)
			switch command {
			case "flows.run":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"run": map[string]any{
							"workflowId": "wf-live",
							"status":     "running",
						},
					},
				})
			case "runs.live.bootstrap":
				atomic.AddInt64(&bootstrapCalls, 1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"enabled":         true,
						"workspaceId":     "ws-acme",
						"workflowId":      "wf-live",
						"baseUrl":         baseURL,
						"snapshotUrl":     baseURL + "/workspaces/ws-acme/live",
						"signalsUrl":      baseURL + "/workspaces/ws-acme/signals",
						"streamUrl":       baseURL + "/workspaces/ws-acme/stream",
						"pollMs":          1,
						"refreshBeforeMs": 60000,
						"auth": map[string]any{
							"type":      "bearer",
							"token":     "rt-token",
							"expiresAt": time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano),
						},
					},
				})
			case "runs.live.graph":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"workflowId": "wf-live",
						"flowSlug":   "root-flow",
						"version":    1,
						"source":     "run-version",
						"graph": map[string]any{
							"schemaVersion": 1,
							"rootId":        "flow:root-flow",
							"nodes": []map[string]any{
								{"id": "flow:root-flow", "kind": "flow", "label": "root-flow", "order": 1},
								{"id": "step:prepare", "kind": "step", "label": "Prepare", "stepId": "prepare", "stepType": "sleep", "parentId": "flow:root-flow", "order": 2},
								{"id": "step:fanout-customers", "kind": "step", "label": "Fan out customers", "stepId": "fanout-customers", "stepType": "fanout", "parentId": "flow:root-flow", "order": 3},
							},
							"edges": []map[string]any{},
						},
					},
				})
			case "runs.get":
				args, _ := body["args"].(map[string]any)
				if args["includeSteps"] == true {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ok":          true,
						"workspaceId": "ws-acme",
						"data": map[string]any{
							"run": map[string]any{
								"workflowId": "wf-live",
								"status":     "completed",
								"steps":      []map[string]any{{"stepId": "fanout-customers", "stepType": "fanout", "status": "completed"}},
							},
						},
					})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-acme",
					"data": map[string]any{
						"run": map[string]any{
							"workflowId": "wf-live",
							"status":     "completed",
						},
					},
				})
			default:
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command " + command}})
			}
		case "/workspaces/ws-acme/stream":
			atomic.AddInt64(&streamCalls, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer rt-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad auth"})
				return
			}
			now := time.Now().UTC()
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: workspace_snapshot\n"))
			_, _ = w.Write([]byte("data: "))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":         "workspace_snapshot",
				"workspace_id": "ws-acme",
				"snapshot": map[string]any{
					"workspace": map[string]any{
						"workspace_id":     "ws-acme",
						"active_run_count": 1,
						"steps_running":    1,
						"updated_at":       now.Format(time.RFC3339Nano),
					},
					"runs": []map[string]any{
						{
							"workspace_id":     "ws-acme",
							"workflow_id":      "wf-live",
							"root_workflow_id": "wf-live",
							"flow_slug":        "root-flow",
							"status":           "running",
							"active":           true,
							"updated_at":       now.Format(time.RFC3339Nano),
						},
					},
				},
			})
			_, _ = w.Write([]byte("\n"))
		case "/workspaces/ws-acme/live":
			atomic.AddInt64(&snapshotCalls, 1)
			if got := r.Header.Get("Authorization"); got != "Bearer rt-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad auth"})
				return
			}
			now := time.Now().UTC()
			started := now.Add(-3 * time.Second)
			branch := 1
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workspace": map[string]any{
					"workspace_id":           "ws-acme",
					"active_run_count":       2,
					"active_child_run_count": 1,
					"steps_running":          2,
					"updated_at":             now.Format(time.RFC3339Nano),
				},
				"runs": []map[string]any{
					{
						"workspace_id":        "ws-acme",
						"workflow_id":         "wf-live",
						"root_workflow_id":    "wf-live",
						"flow_slug":           "root-flow",
						"status":              "running",
						"active":              true,
						"current_step_id":     "fanout-customers",
						"current_step_name":   "Fan out customers",
						"current_step_type":   "fanout",
						"current_step_status": "running",
						"steps_started":       2,
						"steps_completed":     1,
						"steps_running":       1,
						"started_at":          started.Format(time.RFC3339Nano),
						"last_event_at":       now.Format(time.RFC3339Nano),
						"updated_at":          now.Format(time.RFC3339Nano),
					},
					{
						"workspace_id":        "ws-acme",
						"workflow_id":         "wf-child",
						"root_workflow_id":    "wf-live",
						"parent_workflow_id":  "wf-live",
						"parent_step_id":      "fanout-customers",
						"relation_kind":       "child_flow",
						"flow_slug":           "customer-agent",
						"status":              "running",
						"active":              true,
						"fanout_branch_index": branch,
						"last_event_at":       now.Format(time.RFC3339Nano),
						"updated_at":          now.Format(time.RFC3339Nano),
					},
				},
				"relations": []map[string]any{
					{
						"workspace_id":        "ws-acme",
						"root_workflow_id":    "wf-live",
						"parent_workflow_id":  "wf-live",
						"child_workflow_id":   "wf-child",
						"parent_step_id":      "fanout-customers",
						"relation_kind":       "child_flow",
						"flow_slug":           "customer-agent",
						"active":              true,
						"status":              "running",
						"fanout_branch_index": branch,
						"created_at":          started.Format(time.RFC3339Nano),
						"updated_at":          now.Format(time.RFC3339Nano),
					},
				},
				"nodes": []map[string]any{
					{
						"workspace_id":     "ws-acme",
						"workflow_id":      "wf-live",
						"root_workflow_id": "wf-live",
						"node_id":          "step:fanout",
						"node_kind":        "step",
						"node_type":        "fanout",
						"node_name":        "Fan out customers",
						"status":           "running",
						"active":           true,
						"step_id":          "fanout-customers",
						"started_at":       started.Format(time.RFC3339Nano),
						"updated_at":       now.Format(time.RFC3339Nano),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	stdout, stderr, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "run", "root-flow",
		"--live",
		"--poll", "1ms",
		"--timeout", "2s",
	)
	if err != nil {
		t.Fatalf("flows run --live failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if atomic.LoadInt64(&bootstrapCalls) == 0 {
		t.Fatalf("expected live bootstrap call")
	}
	if atomic.LoadInt64(&snapshotCalls) == 0 {
		t.Fatalf("expected realtime snapshot call")
	}
	if atomic.LoadInt64(&streamCalls) == 0 {
		t.Fatalf("expected realtime stream call")
	}
	for _, want := range []string{"wf-live", "Fan out customers", "customer-agent", "[b1]"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("expected live stderr to contain %q\n--- stderr ---\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "Live") {
		t.Fatalf("live renderer should not include Live header label:\n%s", stderr)
	}
	if strings.Contains(stderr, "rt-token") {
		t.Fatalf("live renderer leaked realtime token in stderr:\n%s", stderr)
	}
	var out map[string]any
	if err := json.NewDecoder(bytes.NewBufferString(stdout)).Decode(&out); err != nil {
		t.Fatalf("expected JSON stdout, got error %v\n%s", err, stdout)
	}
	if out["ok"] != true {
		t.Fatalf("expected ok stdout, got %#v", out)
	}
}
