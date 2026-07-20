package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

func TestInstallationIDCandidatesFromLinkedRunWorkflowID(t *testing.T) {
	tests := []struct {
		name       string
		workflowID string
		want       []string
	}{
		{
			name:       "canonical linked installation run",
			workflowID: installationRunWorkflowID,
			want:       []string{"hqfZ7l2dvkFCxMJU8rOz"},
		},
		{
			name:       "canonical linked child run",
			workflowID: "flow-demo-ws-install-inst-with-dashes-v7-pdeadbeef-c-r3",
			want:       []string{"inst", "inst-with", "inst-with-dashes"},
		},
		{
			name:       "keyed installation run",
			workflowID: "flow-demo-ws-install-inst-with-dashes-user-42-v7-pdeadbeef-r3",
			want:       []string{"inst", "inst-with", "inst-with-dashes", "inst-with-dashes-user", "inst-with-dashes-user-42"},
		},
		{
			name:       "keyed coexist installation run",
			workflowID: "flow-demo-ws-install-inst-with-dashes-v7-user-42-pdeadbeef-r3",
			want:       []string{"inst", "inst-with", "inst-with-dashes", "inst-with-dashes-v7", "inst-with-dashes-v7-user", "inst-with-dashes-v7-user-42"},
		},
		{
			name:       "installation id containing marker",
			workflowID: "flow-demo-ws-install-acme-install-prod-v7-pdeadbeef-r3",
			want:       []string{"acme", "acme-install", "acme-install-prod", "prod"},
		},
		{
			name:       "noncanonical installation marker",
			workflowID: "flow-demo-ws-install-inst-r3",
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := installationIDCandidatesFromLinkedRunWorkflowID(tc.workflowID)
			if len(got) != len(tc.want) {
				t.Fatalf("installationIDCandidatesFromLinkedRunWorkflowID(%q) = %#v, want %#v", tc.workflowID, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("installationIDCandidatesFromLinkedRunWorkflowID(%q) = %#v, want %#v", tc.workflowID, got, tc.want)
				}
			}
		})
	}
}

func TestRunsInspectGetWithCommand(t *testing.T) {
	t.Run("workspace run containing marker stays unscoped", func(t *testing.T) {
		calls := 0
		_, status, installationID, err := runsInspectGetWithCommand(
			func(payload map[string]any) (map[string]any, int, error) {
				calls++
				if _, ok := payload["installationId"]; ok {
					t.Fatalf("unexpected installation scope: %#v", payload)
				}
				return map[string]any{"ok": true}, http.StatusOK, nil
			},
			"flow-end-user-install-demo-ws-acme-v1-pdeadbeef-r1",
			"",
			map[string]any{"workflowId": "flow-end-user-install-demo-ws-acme-v1-pdeadbeef-r1"},
		)
		if err != nil || status != http.StatusOK || installationID != "" || calls != 1 {
			t.Fatalf("got status=%d installationID=%q calls=%d err=%v", status, installationID, calls, err)
		}
	})

	t.Run("keyed run retries installation prefixes", func(t *testing.T) {
		const workflowID = "flow-demo-ws-install-inst-with-dashes-user-42-v7-pdeadbeef-r3"
		const wantInstallationID = "inst-with-dashes"
		var attempts []string
		_, status, installationID, err := runsInspectGetWithCommand(
			func(payload map[string]any) (map[string]any, int, error) {
				candidate := firstNonBlankString(payload["installationId"])
				attempts = append(attempts, candidate)
				if candidate != wantInstallationID {
					return map[string]any{"ok": false}, http.StatusNotFound, nil
				}
				return map[string]any{"ok": true}, http.StatusOK, nil
			},
			workflowID,
			"",
			map[string]any{"workflowId": workflowID},
		)
		wantAttempts := []string{"", "inst", "inst-with", wantInstallationID}
		if err != nil || status != http.StatusOK || installationID != wantInstallationID {
			t.Fatalf("got status=%d installationID=%q err=%v", status, installationID, err)
		}
		if len(attempts) != len(wantAttempts) {
			t.Fatalf("installation attempts = %#v, want %#v", attempts, wantAttempts)
		}
		for i := range wantAttempts {
			if attempts[i] != wantAttempts[i] {
				t.Fatalf("installation attempts = %#v, want %#v", attempts, wantAttempts)
			}
		}
	})

	t.Run("installation id containing marker retries the full id", func(t *testing.T) {
		const workflowID = "flow-demo-ws-install-acme-install-prod-v7-pdeadbeef-r3"
		const wantInstallationID = "acme-install-prod"
		var attempts []string
		_, status, installationID, err := runsInspectGetWithCommand(
			func(payload map[string]any) (map[string]any, int, error) {
				candidate := firstNonBlankString(payload["installationId"])
				attempts = append(attempts, candidate)
				if candidate != wantInstallationID {
					return map[string]any{"ok": false}, http.StatusNotFound, nil
				}
				return map[string]any{"ok": true}, http.StatusOK, nil
			},
			workflowID,
			"",
			map[string]any{"workflowId": workflowID},
		)
		wantAttempts := []string{"", "acme", "acme-install", wantInstallationID}
		if err != nil || status != http.StatusOK || installationID != wantInstallationID {
			t.Fatalf("got status=%d installationID=%q err=%v", status, installationID, err)
		}
		if len(attempts) != len(wantAttempts) {
			t.Fatalf("installation attempts = %#v, want %#v", attempts, wantAttempts)
		}
		for i := range wantAttempts {
			if attempts[i] != wantAttempts[i] {
				t.Fatalf("installation attempts = %#v, want %#v", attempts, wantAttempts)
			}
		}
	})

	t.Run("explicit installation id bypasses inference", func(t *testing.T) {
		calls := 0
		_, status, installationID, err := runsInspectGetWithCommand(
			func(payload map[string]any) (map[string]any, int, error) {
				calls++
				if payload["installationId"] != "inst-explicit" {
					t.Fatalf("explicit installation id missing: %#v", payload)
				}
				return map[string]any{"ok": false}, http.StatusNotFound, nil
			},
			installationRunWorkflowID,
			" inst-explicit ",
			map[string]any{"workflowId": installationRunWorkflowID},
		)
		if err != nil || status != http.StatusNotFound || installationID != "inst-explicit" || calls != 1 {
			t.Fatalf("got status=%d installationID=%q calls=%d err=%v", status, installationID, calls, err)
		}
	})
}

func TestRunsInspect_InfersInstallationIDFromCanonicalWorkflowID(t *testing.T) {
	var capturedGetArgs map[string]any
	getCalls := 0

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			getCalls++
			capturedGetArgs = args
			if args["installationId"] != "hqfZ7l2dvkFCxMJU8rOz" {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
				return
			}
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
	if getCalls != 2 {
		t.Fatalf("expected unscoped lookup and one installation fallback, got %d calls", getCalls)
	}
}

func TestRunsInspectStep_UsesInferredInstallationIDForRunAndEvents(t *testing.T) {
	var capturedGetArgs map[string]any
	var capturedEventsArgs map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if args["installationId"] != "hqfZ7l2dvkFCxMJU8rOz" {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false})
				return
			}
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
