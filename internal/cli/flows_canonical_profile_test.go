package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestWaitRetryCommandUsesCanonicalProfileFlag(t *testing.T) {
	got := waitRetryCommand("flows.run", "example", map[string]any{"installationId": "profile-1"}, nil)
	if got != "breyta flows run example --profile-id profile-1 --wait --timeout 15m" {
		t.Fatalf("unexpected retry command: %q", got)
	}
}

func TestWaitForRunCompletionCarriesPayloadProfileIntoPolling(t *testing.T) {
	var pollArgs []map[string]any
	server := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode command request: %v", err)
		}
		if request["command"] != "runs.get" {
			t.Fatalf("expected runs.get, got %#v", request["command"])
		}
		args, _ := request["args"].(map[string]any)
		pollArgs = append(pollArgs, args)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{"run": map[string]any{
				"workflowId": "wf-1",
				"status":     "completed",
			}},
		})
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := waitForRunCompletion(cmd, &App{APIURL: server.URL, WorkspaceID: "ws-1", Token: "token"},
		map[string]any{"ok": true, "data": map[string]any{"workflowId": "wf-1", "status": "running"}},
		"example", "flows.run", map[string]any{"profileId": "profile-1"}, time.Second, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if len(pollArgs) == 0 {
		t.Fatal("expected at least one runs.get poll")
	}
	for _, args := range pollArgs {
		if args["installationId"] != "profile-1" {
			t.Fatalf("expected profile scope on poll, got %#v", args)
		}
	}
}
