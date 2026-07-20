package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const installationRunWorkflowID = "flow-ai-blog-post-generator-ws-consumer-install-hqfZ7l2dvkFCxMJU8rOz-v45-p5189c667-r1"

func runCLIForRunInstallTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestEffectiveRunInstallationID(t *testing.T) {
	tests := []struct {
		name       string
		workflowID string
		explicit   string
		want       string
	}{
		{
			name:       "canonical linked installation run",
			workflowID: installationRunWorkflowID,
			want:       "hqfZ7l2dvkFCxMJU8rOz",
		},
		{
			name:       "canonical linked child run",
			workflowID: "flow-demo-ws-install-inst-with-dashes-v7-pdeadbeef-c-r3",
			want:       "inst-with-dashes",
		},
		{
			name:       "explicit target wins",
			workflowID: installationRunWorkflowID,
			explicit:   "inst-explicit",
			want:       "inst-explicit",
		},
		{
			name:       "ordinary workspace run",
			workflowID: "flow-demo-ws-v7-pdeadbeef-r3",
			want:       "",
		},
		{
			name:       "noncanonical installation marker",
			workflowID: "flow-demo-ws-install-inst-r3",
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveRunInstallationID(tc.workflowID, tc.explicit); got != tc.want {
				t.Fatalf("effectiveRunInstallationID(%q, %q) = %q, want %q", tc.workflowID, tc.explicit, got, tc.want)
			}
		})
	}
}

func TestRunsInspect_InfersInstallationIDFromCanonicalWorkflowID(t *testing.T) {
	var capturedGetArgs map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "runs.get":
			capturedGetArgs = args
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": installationRunWorkflowID,
						"flowSlug":   "ai-blog-post-generator",
						"status":     "completed",
						"steps":      []any{},
					},
				},
			})
		case "runs.events":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"items": []any{}},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIForRunInstallTest(t,
		"--dev",
		"--workspace", "ws-consumer",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "inspect", installationRunWorkflowID,
		"--full",
	)
	if err != nil {
		t.Fatalf("runs inspect failed: %v\n%s", err, stdout)
	}
	if capturedGetArgs["installationId"] != "hqfZ7l2dvkFCxMJU8rOz" {
		t.Fatalf("expected inferred installationId, got %#v", capturedGetArgs)
	}
}

func TestRunsInspectStep_UsesInferredInstallationIDForRunAndEvents(t *testing.T) {
	var capturedGetArgs map[string]any
	var capturedEventsArgs map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		switch command {
		case "runs.get":
			capturedGetArgs = args
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"run": map[string]any{
						"workflowId": installationRunWorkflowID,
						"flowSlug":   "ai-blog-post-generator",
						"steps": []map[string]any{
							{"stepId": "draft", "stepType": "function", "status": "completed"},
						},
					},
				},
			})
		case "runs.events":
			capturedEventsArgs = args
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"data": map[string]any{"items": []any{}},
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
		}
	}))
	defer srv.Close()

	stdout, _, err := runCLIForRunInstallTest(t,
		"--dev",
		"--workspace", "ws-consumer",
		"--api", srv.URL,
		"--token", "user-dev",
		"runs", "inspect", installationRunWorkflowID,
		"--step", "draft",
	)
	if err != nil {
		t.Fatalf("runs inspect --step failed: %v\n%s", err, stdout)
	}
	for label, args := range map[string]map[string]any{
		"runs.get":    capturedGetArgs,
		"runs.events": capturedEventsArgs,
	} {
		if args["installationId"] != "hqfZ7l2dvkFCxMJU8rOz" {
			t.Fatalf("expected inferred installationId in %s, got %#v", label, args)
		}
	}
}
