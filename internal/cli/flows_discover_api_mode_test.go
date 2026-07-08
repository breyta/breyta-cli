package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFlowsDiscoverList_UsesAPICommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawIncludeOwn atomic.Value
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.discover.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
			return
		}
		args, _ := body["args"].(map[string]any)
		if got, _ := args["limit"].(float64); got != 5 {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    "bad_request",
					"message": "expected compact default limit",
				},
			})
			return
		}
		if includeOwn, _ := args["includeOwn"].(bool); includeOwn {
			sawIncludeOwn.Store(includeOwn)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "discover", "list",
		"--include-own",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows discover list failed: %v\n%s", err, stdout)
	}
	if got, _ := sawIncludeOwn.Load().(bool); !got {
		t.Fatalf("expected discover list includeOwn to be forwarded")
	}
}

func TestDiscoverListAlias_UsesAPICommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawIncludeOwn atomic.Value
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.discover.list" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
			return
		}
		args, _ := body["args"].(map[string]any)
		if includeOwn, _ := args["includeOwn"].(bool); includeOwn {
			sawIncludeOwn.Store(includeOwn)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"discover", "list",
		"--include-own",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("discover list alias failed: %v\n%s", err, stdout)
	}
	if got, _ := sawIncludeOwn.Load().(bool); !got {
		t.Fatalf("expected discover list alias includeOwn to be forwarded")
	}
}

func TestDiscoverListAlias_RequiresAPIModeUsesAliasPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	_, stderr, err := runCLIArgs(t,
		"--api", "",
		"--api-key", "bsa_sak-test_secret",
		"discover", "list",
	)
	if err == nil {
		t.Fatalf("expected discover list alias without API mode to fail")
	}
	if !strings.Contains(stderr, "discover list requires API mode") {
		t.Fatalf("expected discover alias API-mode error, got:\n%s", stderr)
	}
	if strings.Contains(stderr, "flows discover list requires API mode") {
		t.Fatalf("discover alias error should not use nested command path, got:\n%s", stderr)
	}
}

func TestFlowsDiscoverSearch_UsesAPICommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawQuery atomic.Value
	var sawIncludeOwn atomic.Value
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.discover.search" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    "bad_request",
					"message": "unexpected command",
				},
			})
			return
		}
		args, _ := body["args"].(map[string]any)
		if got, _ := args["limit"].(float64); got != 5 {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": false,
				"error": map[string]any{
					"code":    "bad_request",
					"message": "expected compact default limit",
				},
			})
			return
		}
		if q, _ := args["query"].(string); q != "" {
			sawQuery.Store(q)
		}
		if includeOwn, _ := args["includeOwn"].(bool); includeOwn {
			sawIncludeOwn.Store(includeOwn)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "discover", "search", "reverse",
		"--include-own",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows discover search failed: %v\n%s", err, stdout)
	}
	if got, _ := sawQuery.Load().(string); got != "reverse" {
		t.Fatalf("expected discover query to be forwarded, got %q", got)
	}
	if got, _ := sawIncludeOwn.Load().(bool); !got {
		t.Fatalf("expected discover search includeOwn to be forwarded")
	}
}

func TestFlowsDiscoverUpdate_UsesAPICommand(t *testing.T) {
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
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.discover.update" {
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
		if got, _ := args["flowSlug"].(string); got != "discover-flow" {
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
		if got, ok := args["public"].(bool); !ok || !got {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-acme",
				"error": map[string]any{
					"code":    "bad_request",
					"message": "missing public=true",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"discover": map[string]any{"public": true},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "discover", "update", "discover-flow",
		"--public=true",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows discover update failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["publicAppUrl"] != srv.URL+"/apps/discover-flow" {
		t.Fatalf("expected public app URL hint, got %#v", meta["publicAppUrl"])
	}
}

func TestFlowsDiscoverUpdate_ForwardsPublicFalse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawPublicFalse atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if v, ok := args["public"].(bool); ok && !v {
			sawPublicFalse.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"discover": map[string]any{"public": false},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--dev",
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--token", "user-dev",
		"flows", "discover", "update", "discover-flow",
		"--public=false",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows discover update failed: %v\n%s", err, stdout)
	}
	if !sawPublicFalse.Load() {
		t.Fatalf("expected public=false to be sent in command args")
	}
}

func TestFlowsSearchHelp_ClarifiesWorkspaceAndTemplateSearch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, _, err := runCLIArgs(t, "flows", "search", "--help")
	if err != nil {
		t.Fatalf("flows search --help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{"workspace", "flows grep", "flows templates search", "approved-template"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected flows search help to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestFlowsDiscoverHelp_IncludesPublicFlowChecklist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, _, err := runCLIArgs(t, "flows", "discover", "--help")
	if err != nil {
		t.Fatalf("flows discover --help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{
		"show up in Discover",
		":discover {:public true}",
		"public flow",
		"Release/promote it",
		"--include-own",
		"from another workspace",
		"Discover install dialog",
		"only proves owner setup",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected flows discover help to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestDiscoverHelp_IncludesTopLevelCommands(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, _, err := runCLIArgs(t, "discover", "--help")
	if err != nil {
		t.Fatalf("discover --help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{
		"Usage:\n  breyta discover [command]",
		"breyta discover list",
		"breyta discover search <query>",
		"breyta discover update <slug> --public=true",
		"list",
		"search",
		"update",
		"More: breyta docs show playbook-public-and-marketplace",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected discover help to include %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "breyta flows discover list") {
		t.Fatalf("top-level discover help should prefer the top-level command path, got:\n%s", stdout)
	}

	updateHelp, _, err := runCLIArgs(t, "discover", "update", "--help")
	if err != nil {
		t.Fatalf("discover update --help failed: %v\n%s", err, updateHelp)
	}
	if !strings.Contains(updateHelp, "breyta discover list") {
		t.Fatalf("expected discover update help to use top-level list path, got:\n%s", updateHelp)
	}
	if strings.Contains(updateHelp, "breyta flows discover list") {
		t.Fatalf("discover update help should not use nested list path, got:\n%s", updateHelp)
	}
}
