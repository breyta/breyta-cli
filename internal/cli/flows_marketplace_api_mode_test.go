package cli_test

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestFlowsMarketplaceUpdate_UsesAPICommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-Breyta-Workspace"); got != "ws-acme" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing workspace header",
				},
			})
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.marketplace.update" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
			return
		}
		args, _ := body["args"].(map[string]any)
		if got, _ := args["flowSlug"].(string); got != "market-flow" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing flowSlug",
				},
			})
			return
		}
		if got, ok := args["visible"].(bool); !ok || !got {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing visible=true",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"marketplace": map[string]any{"visible": true},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "marketplace", "update", "market-flow",
		"--visible=true",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows marketplace update failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["publicAppUrl"] != "https://breyta.ai/apps/market-flow" {
		t.Fatalf("expected public app URL hint, got %#v", meta["publicAppUrl"])
	}
}

func TestFlowsMarketplaceUpdate_ForwardsVisibleFalse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawVisibleFalse atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if v, ok := args["visible"].(bool); ok && !v {
			sawVisibleFalse.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"marketplace": map[string]any{"visible": false},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "marketplace", "update", "market-flow",
		"--visible=false",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows marketplace update failed: %v\n%s", err, stdout)
	}
	if !sawVisibleFalse.Load() {
		t.Fatalf("expected visible=false to be sent in command args")
	}
}
