package cli_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestResourcesWorkflowListAndDirectAliasUseWorkflowEndpoint(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "canonical list subcommand",
			args: []string{"resources", "workflow", "list", "wf-123", "--limit", "4"},
		},
		{
			name: "direct workflow id compatibility",
			args: []string{"resources", "workflow", "wf-123", "--limit", "4"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawWorkflowRequest bool
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/resources/workflow/wf-123" {
					http.NotFound(w, r)
					return
				}
				sawWorkflowRequest = true
				if got := r.URL.Query().Get("limit"); got != "4" {
					t.Fatalf("expected limit=4, got %q", got)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{
						{
							"uri":        "res://v1/ws/ws-acme/result/run/wf-123/flow-output",
							"workflowId": "wf-123",
						},
					},
				})
			}))
			defer srv.Close()

			args := append([]string{
				"--dev",
				"--workspace", "ws-acme",
				"--api", srv.URL,
				"--token", "user-dev",
			}, tc.args...)
			stdout, _, err := runCLIArgs(t, args...)
			if err != nil {
				t.Fatalf("resources workflow failed: %v\n%s", err, stdout)
			}
			if !sawWorkflowRequest {
				t.Fatalf("expected workflow resources request")
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
			}
			if ok, _ := out["ok"].(bool); !ok {
				t.Fatalf("expected ok=true, got %#v", out)
			}
		})
	}
}
