package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestFlowsDiscoverShow_UsesAPICommand(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "slash ref", args: []string{"ws-source/lead-research"}},
		{name: "colon ref", args: []string{"ws-source:lead-research"}},
		{name: "catalog id ref", args: []string{"ws-source__lead-research"}},
		{name: "two args", args: []string{"ws-source", "lead-research"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotArgs map[string]any
			srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/commands" {
					t.Fatalf("unexpected path: %q", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["command"] != "flows.discover.get" {
					t.Fatalf("expected flows.discover.get, got %#v", body["command"])
				}
				gotArgs, _ = body["args"].(map[string]any)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"ok":          true,
					"workspaceId": "ws-consumer",
					"data": map[string]any{
						"app": map[string]any{
							"flow_slug":    "lead-research",
							"workspace_id": "ws-source",
						},
					},
				})
			}))
			defer srv.Close()

			app := &App{WorkspaceID: "ws-consumer", APIURL: srv.URL, Token: "t", TokenExplicit: true}
			cmd := newFlowsDiscoverShowCmd(app)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v\n%s", err, out.String())
			}
			if gotArgs["sourceWorkspaceId"] != "ws-source" || gotArgs["flowSlug"] != "lead-research" {
				t.Fatalf("unexpected args: %#v", gotArgs)
			}
		})
	}
}

func TestFlowsDiscoverShow_RejectsUnsplittableRef(t *testing.T) {
	app := &App{WorkspaceID: "ws-consumer", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsDiscoverShowCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lead-research"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for ref without workspace id, got output: %s", out.String())
	}
}

func TestFlowsDiscoverSearch_CompactHitsCarryDiscoverRefs(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-consumer",
			"data": map[string]any{
				"result": map[string]any{
					"hits": []any{
						map[string]any{
							"id":               "ws-source:lead-research",
							"catalog_id":       "ws-source__lead-research",
							"workspace_id":     "ws-source",
							"flow_slug":        "lead-research",
							"name":             "Lead Research",
							"description":      "Research leads",
							"discover_visible": true,
							"flow_web_url":     "https://flows.example/ws-consumer/discover/flows/ws-source/lead-research",
							"discover_web_url": "https://flows.example/ws-consumer/discover/flows/ws-source/lead-research",
							"public_app_url":   "https://site.example/apps/lead-research",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-consumer", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsDiscoverSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"lead research"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	data, _ := envelope["data"].(map[string]any)
	result, _ := data["result"].(map[string]any)
	hits, _ := result["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %#v", hits)
	}
	hit, _ := hits[0].(map[string]any)
	if hit["workspace_id"] != "ws-source" {
		t.Fatalf("expected workspace_id passthrough, got %#v", hit)
	}
	if hit["discover_web_url"] != "https://flows.example/ws-consumer/discover/flows/ws-source/lead-research" {
		t.Fatalf("expected discover_web_url passthrough, got %#v", hit)
	}
	if hit["public_app_url"] != "https://site.example/apps/lead-research" {
		t.Fatalf("expected public_app_url passthrough, got %#v", hit)
	}
	if hit["hitRef"] != "discover:ws-source/lead-research" {
		t.Fatalf("expected discover hitRef, got %#v", hit["hitRef"])
	}
	if hit["nextCommand"] != "breyta flows discover show 'ws-source/lead-research'" {
		t.Fatalf("expected discover show next command, got %#v", hit["nextCommand"])
	}
}

func TestEnrichCommandHints_FlowsGetMembershipForbidden(t *testing.T) {
	app := &App{WorkspaceID: "ws-source"}
	envelope := map[string]any{"error": "Access denied: not a workspace member"}
	enrichCommandHints(app, "flows.get", map[string]any{"flowSlug": "lead-research"}, http.StatusForbidden, envelope)

	meta, _ := envelope["meta"].(map[string]any)
	hint, _ := meta["hint"].(string)
	if !strings.Contains(hint, "flows show cannot open it") {
		t.Fatalf("expected discover inspect hint for flows.get 403, got %#v", meta)
	}
	next, _ := meta["nextCommands"].([]any)
	joined := ""
	for _, n := range next {
		s, _ := n.(string)
		joined += s + "\n"
	}
	if !strings.Contains(joined, "breyta flows discover show 'ws-source/lead-research'") {
		t.Fatalf("expected concrete discover show next command, got %#v", next)
	}

	// Unrelated commands must not get the Discover suggestion.
	other := map[string]any{"error": "Access denied: not a workspace member"}
	enrichCommandHints(app, "flows.put_draft", map[string]any{"flowSlug": "lead-research"}, http.StatusForbidden, other)
	if otherMeta, ok := other["meta"].(map[string]any); ok {
		if hint, _ := otherMeta["hint"].(string); strings.Contains(hint, "Discover") {
			t.Fatalf("discover hint must be gated to flows.get, got %#v", otherMeta)
		}
	}
}

func TestWriteAPIResult_MembershipForbiddenHint(t *testing.T) {
	t.Run("non-dev gives generic workspace-selection hint", func(t *testing.T) {
		app := &App{WorkspaceID: "ws-consumer"}
		cmd := newFlowsDiscoverShowCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)

		envelope := map[string]any{"error": "Access denied: not a workspace member"}
		if err := writeAPIResult(cmd, app, envelope, http.StatusForbidden); err == nil {
			t.Fatalf("expected guided error for 403")
		}
		var body map[string]any
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatalf("invalid json output: %v\n%s", err, out.String())
		}
		meta, _ := body["meta"].(map[string]any)
		hint, _ := meta["hint"].(string)
		if !strings.Contains(hint, "not a member of the addressed workspace") {
			t.Fatalf("expected generic membership hint, got %#v", meta)
		}
		next, _ := meta["nextCommands"].([]any)
		joined := ""
		for _, n := range next {
			s, _ := n.(string)
			joined += s + "\n"
		}
		if !strings.Contains(joined, "breyta workspaces list") {
			t.Fatalf("expected workspaces list next command, got %#v", next)
		}
		if strings.Contains(joined, "workspaces bootstrap") {
			t.Fatalf("bootstrap hint must stay dev-only, got %#v", next)
		}
	})

	t.Run("command-aware hint from enrich wins over the generic one", func(t *testing.T) {
		app := &App{WorkspaceID: "ws-source"}
		cmd := newFlowsDiscoverShowCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)

		envelope := map[string]any{"error": "Access denied: not a workspace member"}
		enrichCommandHints(app, "flows.get", map[string]any{"flowSlug": "lead-research"}, http.StatusForbidden, envelope)
		if err := writeAPIResult(cmd, app, envelope, http.StatusForbidden); err == nil {
			t.Fatalf("expected guided error for 403")
		}
		var body map[string]any
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatalf("invalid json output: %v\n%s", err, out.String())
		}
		meta, _ := body["meta"].(map[string]any)
		if hint, _ := meta["hint"].(string); !strings.Contains(hint, "flows show cannot open it") {
			t.Fatalf("expected discover inspect hint to survive writeAPIResult, got %#v", meta)
		}
	})

	t.Run("dev keeps bootstrap hint", func(t *testing.T) {
		app := &App{WorkspaceID: "ws-local", DevMode: true}
		cmd := newFlowsDiscoverShowCmd(app)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)

		envelope := map[string]any{"error": "Access denied: not a workspace member"}
		if err := writeAPIResult(cmd, app, envelope, http.StatusForbidden); err == nil {
			t.Fatalf("expected guided error for 403")
		}
		var body map[string]any
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatalf("invalid json output: %v\n%s", err, out.String())
		}
		meta, _ := body["meta"].(map[string]any)
		next, _ := meta["nextCommands"].([]any)
		joined := ""
		for _, n := range next {
			s, _ := n.(string)
			joined += s + "\n"
		}
		if !strings.Contains(joined, "breyta workspaces bootstrap ws-local") {
			t.Fatalf("expected bootstrap next command in dev, got %#v", next)
		}
	})
}
