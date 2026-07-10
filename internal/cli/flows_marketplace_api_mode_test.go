package cli_test

import (
	"encoding/json"
	"net/http"
	"strings"
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
		"--visible",
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
	if meta["publicAppUrl"] != srv.URL+"/apps/market-flow" {
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

func TestFlowsMarketplaceUpdate_AcceptsSpaceSeparatedVisibleFalse(t *testing.T) {
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
		"--visible", "false",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows marketplace update failed: %v\n%s", err, stdout)
	}
	if !sawVisibleFalse.Load() {
		t.Fatalf("expected visible=false to be sent in command args")
	}
}

func TestFlowsMarketplaceUpdate_RejectsAmbiguousBooleanLikeSlugAndSpaceSeparatedValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var calledAPI atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledAPI.Store(true)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "marketplace", "update",
		"--visible", "true", "false",
		"--pretty",
	)
	if err == nil {
		t.Fatalf("expected ambiguous boolean-like slug command to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if calledAPI.Load() {
		t.Fatalf("ambiguous flag/slug command should not reach API")
	}
	if !strings.Contains(stderr, "ambiguous --visible value and flow slug") &&
		!strings.Contains(stdout, "ambiguous --visible value and flow slug") {
		t.Fatalf("expected ambiguous slug/value error, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestFlowsMarketplaceUpdate_RejectsExplicitVisibleFalseWithExtraBooleanValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var calledAPI atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledAPI.Store(true)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "marketplace", "update", "market-flow",
		"--visible=false", "true",
		"--pretty",
	)
	if err == nil {
		t.Fatalf("expected duplicate boolean command to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if calledAPI.Load() {
		t.Fatalf("duplicate flag value command should not reach API")
	}
	if !strings.Contains(stderr, "extra boolean argument") &&
		!strings.Contains(stdout, "extra boolean argument") {
		t.Fatalf("expected extra boolean error, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestFlowsMarketplaceUpdate_RejectsExplicitVisibleTrueWithExtraBooleanValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var calledAPI atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledAPI.Store(true)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	stdout, stderr, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "marketplace", "update", "market-flow",
		"--visible=true", "false",
		"--pretty",
	)
	if err == nil {
		t.Fatalf("expected duplicate boolean command to fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if calledAPI.Load() {
		t.Fatalf("duplicate flag value command should not reach API")
	}
	if !strings.Contains(stderr, "extra boolean argument") &&
		!strings.Contains(stdout, "extra boolean argument") {
		t.Fatalf("expected extra boolean error, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestFlowsMarketplaceUpdate_AllowsSingleLetterSlugWithSpaceSeparatedVisibleFalse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawPayload atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if slug, _ := args["flowSlug"].(string); slug == "t" {
			if v, ok := args["visible"].(bool); ok && !v {
				sawPayload.Store(true)
			}
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
		"flows", "marketplace", "update", "t",
		"--visible", "false",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows marketplace update failed: %v\n%s", err, stdout)
	}
	if !sawPayload.Load() {
		t.Fatalf("expected flowSlug=t and visible=false to be sent in command args")
	}
}

func TestFlowsMarketplaceUpdate_AcceptsBooleanLikeSlugWithExplicitVisibleValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawPayload atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		args, _ := body["args"].(map[string]any)
		if slug, _ := args["flowSlug"].(string); slug == "false" {
			if v, ok := args["visible"].(bool); ok && v {
				sawPayload.Store(true)
			}
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
		"flows", "marketplace", "update", "false",
		"--visible=true",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows marketplace update failed: %v\n%s", err, stdout)
	}
	if !sawPayload.Load() {
		t.Fatalf("expected flowSlug=false and visible=true to be sent in command args")
	}
}

func TestFlowsPublicCommand_VisibleInDefaultHelp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	stdout, _, err := runCLIArgs(t, "flows", "--help")
	if err != nil {
		t.Fatalf("flows help failed: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, "public") ||
		!strings.Contains(stdout, "Inspect and manage public-flow visibility") {
		t.Fatalf("expected flows help to expose public command, got:\n%s", stdout)
	}
}

func TestPublicVisibilityHelpHidesBareFlagSentinelAndShowsPlaybook(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	for _, args := range [][]string{
		{"flows", "discover", "update", "--help"},
		{"flows", "marketplace", "update", "--help"},
		{"flows", "public", "publish", "--help"},
	} {
		stdout, _, err := runCLIArgs(t, args...)
		if err != nil {
			t.Fatalf("help failed for %v: %v\n%s", args, err, stdout)
		}
		if strings.Contains(stdout, "__breyta_bare_true__") {
			t.Fatalf("help for %v leaked bare flag sentinel:\n%s", args, stdout)
		}
		if args[1] != "public" && !strings.Contains(stdout, "true|false") {
			t.Fatalf("expected true|false guidance for %v:\n%s", args, stdout)
		}
		if !strings.Contains(stdout, "playbook-public-and-marketplace") {
			t.Fatalf("expected public playbook hint for %v:\n%s", args, stdout)
		}
	}
}

func TestFlowsPublicDelist_UpdatesAllPublicSurfaces(t *testing.T) {
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
		if body["command"] != "flows.public.update" {
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
		if v, ok := args["public"].(bool); ok && !v {
			sawPublicFalse.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"public": map[string]any{
					"discoverPublic":     false,
					"marketplaceVisible": false,
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "public", "delist", "market-flow",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows public delist failed: %v\n%s", err, stdout)
	}
	if !sawPublicFalse.Load() {
		t.Fatalf("expected public=false to be sent in command args")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta != nil && meta["publicAppUrl"] != nil {
		t.Fatalf("did not expect public app URL hint while delisting, got %#v", meta)
	}
}

func TestFlowsPublicDelist_FailsOnPartialUpdateWarning(t *testing.T) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"public": map[string]any{
					"discoverPublic":     false,
					"marketplaceVisible": false,
				},
			},
			"meta": map[string]any{
				"publicCatalog": map[string]any{
					"ok":      false,
					"warning": "public_catalog_refresh_failed",
					"action":  "sync",
				},
				"publish": map[string]any{
					"ok":        false,
					"warning":   "publish_failed",
					"eventName": "flow.marketplace.published",
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "public", "delist", "market-flow",
		"--pretty",
	)
	if err == nil {
		t.Fatalf("expected partial public update warning to fail\n%s", stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	if out["ok"] != false {
		t.Fatalf("expected transformed ok=false, got %#v", out["ok"])
	}
	errMap, _ := out["error"].(map[string]any)
	if errMap["code"] != "partial_public_update_failed" {
		t.Fatalf("expected partial failure error, got %#v", errMap)
	}
	details, _ := errMap["details"].(map[string]any)
	failed, _ := details["failed"].([]any)
	if len(failed) != 2 || failed[0] != "publicCatalog" || failed[1] != "publish" {
		t.Fatalf("expected publicCatalog and publish failed details, got %#v", failed)
	}
}

func TestFlowsPublicPublish_UpdatesAllPublicSurfaces(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	t.Setenv("LOCALAPPDATA", tmp)

	var sawPublicTrue atomic.Bool
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.public.update" {
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
		if v, ok := args["public"].(bool); ok && v {
			sawPublicTrue.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"data": map[string]any{
				"public": map[string]any{
					"discoverPublic":     true,
					"marketplaceVisible": true,
				},
			},
		})
	}))
	defer srv.Close()

	stdout, _, err := runCLIArgs(t,
		"--workspace", "ws-acme",
		"--api", srv.URL,
		"--api-key", "sa-dev",
		"flows", "public", "publish", "market-flow",
		"--pretty",
	)
	if err != nil {
		t.Fatalf("flows public publish failed: %v\n%s", err, stdout)
	}
	if !sawPublicTrue.Load() {
		t.Fatalf("expected public=true to be sent in command args")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["publicAppUrl"] != srv.URL+"/apps/market-flow" {
		t.Fatalf("expected public app URL hint, got %#v", meta["publicAppUrl"])
	}
}
