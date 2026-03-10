package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsSearch_BuildsPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotMethod string
	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		gotMethod = method
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"stripe", "--catalog-scope", "workspace", "--provider", "stripe", "--limit", "5", "--from", "10", "--full=true"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["query"] != "stripe" {
		t.Fatalf("expected query=stripe, got %#v", gotPayload["query"])
	}
	if gotPayload["scope"] != "workspace" {
		t.Fatalf("expected scope=workspace, got %#v", gotPayload["scope"])
	}
	if gotPayload["provider"] != "stripe" {
		t.Fatalf("expected provider=stripe, got %#v", gotPayload["provider"])
	}
	if gotPayload["limit"] != 5 {
		t.Fatalf("expected limit=5, got %#v", gotPayload["limit"])
	}
	if gotPayload["from"] != 10 {
		t.Fatalf("expected from=10, got %#v", gotPayload["from"])
	}
	if gotPayload["includeDefinition"] != true {
		t.Fatalf("expected includeDefinition=true, got %#v", gotPayload["includeDefinition"])
	}
}

func TestFlowsSearch_BrowseWithoutQuery(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotMethod string
	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		gotMethod = method
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--provider", "stripe", "--limit", "25"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if _, ok := gotPayload["query"]; ok {
		t.Fatalf("expected query omitted for browse, got %#v", gotPayload["query"])
	}
	if gotPayload["provider"] != "stripe" {
		t.Fatalf("expected provider=stripe, got %#v", gotPayload["provider"])
	}
	if gotPayload["limit"] != 25 {
		t.Fatalf("expected limit=25, got %#v", gotPayload["limit"])
	}
}

func TestFlowsSearch_UsesGlobalCommandWithoutWorkspace(t *testing.T) {
	var gotWorkspaceHeader string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/global/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotWorkspaceHeader = r.Header.Get("X-Breyta-Workspace")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "",
			"data": map[string]any{
				"result": map[string]any{
					"hits": []any{},
				},
			},
		})
	}))
	defer srv.Close()

	app := &App{APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"stripe"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotWorkspaceHeader != "" {
		t.Fatalf("expected no workspace header, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.search" {
		t.Fatalf("expected flows.search, got %#v", gotBody["command"])
	}
	args, _ := gotBody["args"].(map[string]any)
	if args["scope"] != "all" {
		t.Fatalf("expected global scope, got %#v", args["scope"])
	}
}

func TestFlowsSearch_WorkspaceScopeRequiresWorkspaceLocally(t *testing.T) {
	app := &App{APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"stripe", "--catalog-scope", "workspace"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "workspace-scoped catalog search requires --workspace or BREYTA_WORKSPACE") {
		t.Fatalf("unexpected error: %v", err)
	}
}
