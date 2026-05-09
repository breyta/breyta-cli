package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
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
	cmd.SetArgs([]string{"stripe", "--catalog-scope", "workspace", "--provider", "stripe", "--step-type", "http", "--limit", "5", "--from", "10", "--full=true"})

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
	if gotPayload["stepType"] != "http" {
		t.Fatalf("expected stepType=http, got %#v", gotPayload["stepType"])
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

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFlowsExamplesStep_BuildsWorkspacePayload(t *testing.T) {
	var gotWorkspaceHeader string
	var gotBody map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotWorkspaceHeader = r.Header.Get("X-Breyta-Workspace")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"examples": map[string]any{"items": []any{}},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsExamplesStepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"search", "customer research", "--catalog-scope", "workspace", "--limit", "3", "--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotWorkspaceHeader != "ws-test" {
		t.Fatalf("expected workspace header ws-test, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.examples.step" {
		t.Fatalf("expected flows.examples.step, got %#v", gotBody["command"])
	}
	args, _ := gotBody["args"].(map[string]any)
	if args["stepType"] != "search" || args["query"] != "customer research" || args["scope"] != "workspace" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if args["limit"] != float64(3) {
		t.Fatalf("expected limit=3, got %#v", args["limit"])
	}
	if args["full"] != true {
		t.Fatalf("expected full=true, got %#v", args["full"])
	}
}

func TestFlowsWorkspaceSearch_BuildsWorkspacePayload(t *testing.T) {
	var gotWorkspaceHeader string
	var gotBody map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotWorkspaceHeader = r.Header.Get("X-Breyta-Workspace")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsWorkspaceSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--step-type", "http", "--flow", "gmail-support-agent", "--target", "draft", "--limit", "5", "--from", "2", "--include-archived"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotWorkspaceHeader != "ws-test" {
		t.Fatalf("expected workspace header ws-test, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.workspace.search" {
		t.Fatalf("expected flows.workspace.search, got %#v", gotBody["command"])
	}
	args, _ := gotBody["args"].(map[string]any)
	if args["query"] != "gmail" || args["stepType"] != "http" || args["flowSlug"] != "gmail-support-agent" || args["target"] != "draft" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if args["limit"] != float64(5) || args["from"] != float64(2) || args["includeArchived"] != true {
		t.Fatalf("unexpected pagination/archive args: %#v", args)
	}
}

func TestFlowsWorkspaceSearch_RejectsFullFlag(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsWorkspaceSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--full"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "unknown flag: --full") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsWorkspaceExamplesStep_BuildsWorkspacePayload(t *testing.T) {
	var gotWorkspaceHeader string
	var gotBody map[string]any

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		gotWorkspaceHeader = r.Header.Get("X-Breyta-Workspace")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"examples": map[string]any{"items": []any{}},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsWorkspaceExamplesStepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"http", "gmail", "--flow", "gmail-support-agent", "--target", "live", "--limit", "3", "--full", "--include-archived"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotWorkspaceHeader != "ws-test" {
		t.Fatalf("expected workspace header ws-test, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.workspace.examples.step" {
		t.Fatalf("expected flows.workspace.examples.step, got %#v", gotBody["command"])
	}
	args, _ := gotBody["args"].(map[string]any)
	if args["stepType"] != "http" || args["query"] != "gmail" || args["flowSlug"] != "gmail-support-agent" || args["target"] != "live" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if args["limit"] != float64(3) || args["full"] != true || args["includeArchived"] != true {
		t.Fatalf("unexpected full/archive args: %#v", args)
	}
}

func TestFlowsWorkspaceHelp_SeparatesPrivateSearchFromApprovedCatalog(t *testing.T) {
	cmd := newFlowsWorkspaceCmd(&App{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	stdout := out.String()
	if err != nil {
		t.Fatalf("flows workspace --help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{"private", "actual flows", "breyta flows search", "approved"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected flows workspace help to include %q, got:\n%s", want, stdout)
		}
	}
}

func TestFlowsDoctorAndPublicPreflightCommands(t *testing.T) {
	seenCommands := []string{}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		seenCommands = append(seenCommands, body["command"].(string))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data":        map[string]any{},
			"meta":        map[string]any{"nextCommands": []string{}},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{name: "doctor", cmd: newFlowsDoctorCmd(app), args: []string{"my-flow", "--target", "live"}},
		{name: "public preflight", cmd: newFlowsPublicPreflightCmd(app), args: []string{"my-flow"}},
	} {
		var out bytes.Buffer
		tc.cmd.SetOut(&out)
		tc.cmd.SetErr(&out)
		tc.cmd.SetArgs(tc.args)
		if err := tc.cmd.Execute(); err != nil {
			t.Fatalf("%s execute: %v\n%s", tc.name, err, out.String())
		}
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.public.preflight" {
		t.Fatalf("unexpected commands: %s", got)
	}
}
