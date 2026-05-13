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
	cmd.SetArgs([]string{"stripe", "--catalog-scope", "workspace", "--provider", "stripe", "--step-type", "http", "--limit", "5", "--from", "10", "--full=true", "--raw-definition"})

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
	if gotPayload["rawDefinition"] != true {
		t.Fatalf("expected rawDefinition=true, got %#v", gotPayload["rawDefinition"])
	}
}

func TestFlowsSearch_DefaultsToWorkspaceMetadataSearch(t *testing.T) {
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
	cmd.SetArgs([]string{"gmail", "--provider", "google", "--step-type", "http", "--tool-name", "web_search", "--connection", "gmail", "--flow", "gmail-support-agent", "--limit", "25"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.workspace.search" {
		t.Fatalf("expected method flows.workspace.search, got %q", gotMethod)
	}
	if gotPayload["query"] != "gmail" {
		t.Fatalf("expected query=gmail, got %#v", gotPayload["query"])
	}
	if gotPayload["provider"] != "google" || gotPayload["stepType"] != "http" || gotPayload["toolName"] != "web_search" || gotPayload["connection"] != "gmail" || gotPayload["flowSlug"] != "gmail-support-agent" {
		t.Fatalf("unexpected filters: %#v", gotPayload)
	}
	if gotPayload["limit"] != 25 {
		t.Fatalf("expected limit=25, got %#v", gotPayload["limit"])
	}
	if gotPayload["target"] != "latest" || gotPayload["includeArchived"] != false {
		t.Fatalf("unexpected workspace defaults: %#v", gotPayload)
	}
}

func TestFlowsSearch_FullFalseStaysOnWorkspaceSearch(t *testing.T) {
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
	cmd.SetArgs([]string{"gmail", "--full=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.workspace.search" {
		t.Fatalf("expected method flows.workspace.search, got %q", gotMethod)
	}
	if gotPayload["query"] != "gmail" {
		t.Fatalf("expected query=gmail, got %#v", gotPayload["query"])
	}
	if _, ok := gotPayload["includeDefinition"]; ok {
		t.Fatalf("did not expect template includeDefinition in workspace payload: %#v", gotPayload)
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
	if !strings.Contains(err.Error(), "workspace-scoped template search requires --workspace or BREYTA_WORKSPACE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsSearch_RejectsFlowFilterOnTemplateFallback(t *testing.T) {
	app := &App{APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--flow", "gmail-support-agent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--flow only applies to workspace search") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsSearch_RejectsFlowFilterWithFullTemplateMode(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--flow", "gmail-support-agent", "--full"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--flow only applies to workspace search") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsSearch_RejectsRawDefinitionWithoutFull(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--raw-definition"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--raw-definition requires --full") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsTemplatesSearch_BuildsApprovedTemplatePayload(t *testing.T) {
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
	cmd := newFlowsTemplatesSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"vision", "--step-type", "llm", "--tool-name", "web_search", "--limit", "4"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["query"] != "vision" || gotPayload["scope"] != "all" || gotPayload["surface"] != "templates" || gotPayload["stepType"] != "llm" || gotPayload["toolName"] != "web_search" {
		t.Fatalf("unexpected payload: %#v", gotPayload)
	}
}

func TestFlowsTemplatesSearch_CompactsDefaultOutput(t *testing.T) {
	var gotArgs map[string]any
	longDescription := strings.Repeat("approved template description ", 40)
	longStepsText := strings.Repeat("source step summary ", 70)
	steps := []any{
		map[string]any{"id": "s1"},
		map[string]any{"id": "s2"},
		map[string]any{"id": "s3"},
		map[string]any{"id": "s4"},
		map[string]any{"id": "s5"},
		map[string]any{"id": "s6"},
		map[string]any{"id": "s7"},
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["command"] != "flows.search" {
			t.Fatalf("expected flows.search, got %#v", body["command"])
		}
		gotArgs, _ = body["args"].(map[string]any)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"result": map[string]any{
					"hits": []any{
						map[string]any{
							"flow_slug":           "template-agent",
							"publish_description": longDescription,
							"steps_text":          longStepsText,
							"step_list":           steps,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsTemplatesSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"agent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotArgs["limit"] != float64(5) || gotArgs["includeDefinition"] != false {
		t.Fatalf("unexpected compact default args: %#v", gotArgs)
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	meta, _ := envelope["meta"].(map[string]any)
	if meta["outputView"] != "compact" {
		t.Fatalf("expected compact outputView, got %#v", meta)
	}
	data, _ := envelope["data"].(map[string]any)
	result, _ := data["result"].(map[string]any)
	hits, _ := result["hits"].([]any)
	hit, _ := hits[0].(map[string]any)
	if _, ok := hit["publish_description"]; ok {
		t.Fatalf("expected publish_description to be compacted: %#v", hit)
	}
	if _, ok := hit["steps_text"]; ok {
		t.Fatalf("expected steps_text to be compacted: %#v", hit)
	}
	if got, _ := hit["publishDescriptionPreview"].(string); got == "" || len(got) >= len(longDescription) {
		t.Fatalf("expected compact publishDescriptionPreview, got %#v", hit["publishDescriptionPreview"])
	}
	if got, _ := hit["stepsTextPreview"].(string); got == "" || len(got) >= len(longStepsText) {
		t.Fatalf("expected compact stepsTextPreview, got %#v", hit["stepsTextPreview"])
	}
	stepList, _ := hit["step_list"].([]any)
	if len(stepList) != 5 || hit["stepListOmitted"] != float64(2) {
		t.Fatalf("expected truncated step list, got step_list=%#v omitted=%#v", stepList, hit["stepListOmitted"])
	}
}

func TestFlowsTemplatesSearch_TableFormat(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["command"] != "flows.search" {
			t.Fatalf("expected flows.search, got %#v", body["command"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-test",
			"data": map[string]any{
				"result": map[string]any{
					"hits": []any{
						map[string]any{
							"flow_slug":  "gmail-support-agent",
							"name":       "AI Gmail Support Agent",
							"step_types": []any{"function", "http", "llm"},
							"step_count": 12,
							"providers":  []any{"google", "openai"},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsTemplatesSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail", "--format", "table"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	got := out.String()
	if strings.Contains(got, "\t") {
		t.Fatalf("expected table output to expand tabs, got:\n%s", got)
	}
	for _, want := range []string{"slug", "name", "steps", "gmail-support-agent", "AI Gmail Support Agent", "function,http,llm", "google,openai"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestFlowsGrep_BuildsWorkspaceDefinitionSearchPayload(t *testing.T) {
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
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"web_search", "--or", "web_search_preview", "--step-type", "agent", "--tool-name", "web_search", "--connection", "openai", "--target", "draft", "--limit", "3"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.workspace.search" {
		t.Fatalf("expected method flows.workspace.search, got %q", gotMethod)
	}
	if gotPayload["definitionSearch"] != true || gotPayload["query"] != "web_search" || gotPayload["target"] != "draft" {
		t.Fatalf("unexpected grep payload: %#v", gotPayload)
	}
	patterns, _ := gotPayload["patterns"].([]string)
	if len(patterns) != 1 || patterns[0] != "web_search_preview" {
		t.Fatalf("unexpected patterns: %#v", gotPayload["patterns"])
	}
	if gotPayload["stepType"] != "agent" || gotPayload["toolName"] != "web_search" || gotPayload["connection"] != "openai" {
		t.Fatalf("unexpected filters: %#v", gotPayload)
	}
}

func TestFlowsTemplatesGrep_BuildsTemplateDefinitionSearchPayload(t *testing.T) {
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
	cmd := newFlowsTemplatesGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"image/*", "--or", ":multiple true", "--step-type", "llm", "--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["definitionSearch"] != true || gotPayload["query"] != "image/*" || gotPayload["scope"] != "all" || gotPayload["surface"] != "templates" || gotPayload["includeDefinition"] != true {
		t.Fatalf("unexpected template grep payload: %#v", gotPayload)
	}
	if gotPayload["rawDefinition"] != false {
		t.Fatalf("expected rawDefinition=false, got %#v", gotPayload["rawDefinition"])
	}
}

func TestFlowsGrep_RejectsNoPatternOrFilter(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "provide a grep pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsGrep_RejectsFlowFilterOutsideWorkspaceScope(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"web_search", "--scope", "templates", "--flow", "gmail-support-agent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--flow only applies to workspace grep") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFlowsGrep_TemplateScopeMarksTemplateSurface(t *testing.T) {
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
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"image/*", "--scope", "templates", "--step-type", "llm"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["definitionSearch"] != true || gotPayload["query"] != "image/*" || gotPayload["surface"] != "templates" {
		t.Fatalf("unexpected template grep payload: %#v", gotPayload)
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
	cmd.SetArgs([]string{"gmail", "--step-type", "http", "--flow", "gmail-support-agent", "--target", "draft", "--from", "2", "--include-archived"})

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

func TestFlowsWorkspaceHelp_PointsToNewSearchTaxonomy(t *testing.T) {
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
	for _, want := range []string{"actual flows", "breyta flows search", "breyta flows grep", "templates"} {
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
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check,flows.public.preflight" {
		t.Fatalf("unexpected commands: %s", got)
	}
}

func TestFlowsDoctorIncludesConfigureCheckReadiness(t *testing.T) {
	seenCommands := []string{}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		command, _ := body["command"].(string)
		args, _ := body["args"].(map[string]any)
		seenCommands = append(seenCommands, command)
		if args["flowSlug"] != "gmail-memory-draft-reply-agent" {
			t.Fatalf("unexpected flowSlug for %s: %#v", command, args["flowSlug"])
		}
		if args["target"] != "draft" {
			t.Fatalf("unexpected target for %s: %#v", command, args["target"])
		}
		switch command {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "gmail-memory-draft-reply-agent",
						"target":   "draft",
						"ready":    true,
						"checks": []map[string]any{
							{"id": "definition", "label": "Flow definition", "pass": true},
						},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{
						"breyta flows validate gmail-memory-draft-reply-agent",
						"breyta flows run gmail-memory-draft-reply-agent --target draft --wait",
						"breyta flows diff gmail-memory-draft-reply-agent",
					},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug":                    "gmail-memory-draft-reply-agent",
					"target":                      "draft",
					"ready":                       false,
					"requiredConnectionSlots":     []string{"gmail", "ai"},
					"configuredConnectionSlots":   []string{"gmail", "ai"},
					"missingConnectionSlots":      []string{},
					"missingActivationInputs":     []string{},
					"invalidConnectionBindings":   []map[string]any{{"slot": "gmail", "connectionId": "conn-gmail", "error": "OAuth requirements not met"}},
					"unhealthyConnectionBindings": []string{},
				},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsDoctorCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"gmail-memory-draft-reply-agent", "--target", "draft"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check" {
		t.Fatalf("unexpected commands: %s", got)
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	doctor := mapStringAny(mapStringAny(envelope["data"])["doctor"])
	if doctor["ready"] != false {
		t.Fatalf("expected doctor ready=false when configure check blocks, got %#v", doctor["ready"])
	}
	if doctor["definitionReady"] != true {
		t.Fatalf("expected definitionReady=true, got %#v", doctor["definitionReady"])
	}
	if doctor["configurationReady"] != false {
		t.Fatalf("expected configurationReady=false, got %#v", doctor["configurationReady"])
	}
	config := mapStringAny(doctor["configuration"])
	if config["ready"] != false {
		t.Fatalf("expected configuration.ready=false, got %#v", config["ready"])
	}
	invalidBindings := sliceAny(config["invalidConnectionBindings"])
	if len(invalidBindings) != 1 || mapStringAny(invalidBindings[0])["slot"] != "gmail" {
		t.Fatalf("expected gmail invalid binding, got %#v", invalidBindings)
	}
	meta := mapStringAny(envelope["meta"])
	nextCommands := sliceAny(meta["nextCommands"])
	nextJoined := strings.TrimSpace(out.String())
	if len(nextCommands) == 0 {
		t.Fatalf("expected blocked next commands, got %#v", meta["nextCommands"])
	}
	if strings.Contains(nextJoined, "flows run gmail-memory-draft-reply-agent") {
		t.Fatalf("doctor should not suggest run while configure check is blocked:\n%s", out.String())
	}
	if !strings.Contains(nextJoined, "flows configure check gmail-memory-draft-reply-agent --target draft") {
		t.Fatalf("expected configure check recovery command:\n%s", out.String())
	}
	if !strings.Contains(nextJoined, "flows configure suggest gmail-memory-draft-reply-agent --target draft") {
		t.Fatalf("expected configure suggest recovery command:\n%s", out.String())
	}
}

func TestFlowsDoctorPreservesReadinessWhenConfigureCheckUnsupported(t *testing.T) {
	seenCommands := []string{}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		command, _ := body["command"].(string)
		seenCommands = append(seenCommands, command)
		switch command {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "legacy-flow",
						"target":   "draft",
						"ready":    true,
						"checks": []map[string]any{
							{"id": "definition", "label": "Flow definition", "pass": true},
						},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{
						"breyta flows validate legacy-flow",
						"breyta flows run legacy-flow --target draft --wait",
						"breyta flows diff legacy-flow",
					},
				},
			})
		case "flows.configure.check":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"workspaceId": "ws-test",
				"error": map[string]any{
					"code":    "unknown_command",
					"message": "unexpected command",
				},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsDoctorCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"legacy-flow", "--target", "draft"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check" {
		t.Fatalf("unexpected commands: %s", got)
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	doctor := mapStringAny(mapStringAny(envelope["data"])["doctor"])
	if doctor["ready"] != true {
		t.Fatalf("expected original doctor ready=true, got %#v", doctor["ready"])
	}
	if _, ok := doctor["configuration"]; ok {
		t.Fatalf("expected unsupported configure check to leave doctor unchanged, got:\n%s", out.String())
	}
	nextJoined := strings.TrimSpace(out.String())
	if !strings.Contains(nextJoined, "flows run legacy-flow --target draft --wait") {
		t.Fatalf("expected original run command to remain:\n%s", out.String())
	}
	if strings.Contains(nextJoined, "flows configure suggest legacy-flow") {
		t.Fatalf("did not expect configure remediation for unsupported check:\n%s", out.String())
	}
}

func TestFlowsDoctorPreservesDefinitionGuidanceWhenConfigurationBlocked(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		command, _ := body["command"].(string)
		switch command {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "broken-flow",
						"target":   "draft",
						"ready":    false,
						"checks": []map[string]any{
							{"id": "definition", "label": "Flow definition", "pass": false},
						},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{
						"breyta flows validate broken-flow",
						"breyta flows diff broken-flow",
					},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug":                  "broken-flow",
					"target":                    "draft",
					"ready":                     false,
					"missingConnectionSlots":    []string{"gmail"},
					"invalidConnectionBindings": []string{},
				},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsDoctorCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"broken-flow", "--target", "draft"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor execute: %v\n%s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	doctor := mapStringAny(mapStringAny(envelope["data"])["doctor"])
	if doctor["ready"] != false {
		t.Fatalf("expected doctor ready=false, got %#v", doctor["ready"])
	}
	if doctor["definitionReady"] != false {
		t.Fatalf("expected definitionReady=false, got %#v", doctor["definitionReady"])
	}
	if doctor["configurationReady"] != false {
		t.Fatalf("expected configurationReady=false, got %#v", doctor["configurationReady"])
	}
	nextJoined := strings.TrimSpace(out.String())
	for _, want := range []string{
		"breyta flows validate broken-flow",
		"breyta flows diff broken-flow",
		"breyta flows configure check broken-flow --target draft",
		"breyta flows configure suggest broken-flow --target draft",
	} {
		if !strings.Contains(nextJoined, want) {
			t.Fatalf("expected nextCommands to include %q:\n%s", want, out.String())
		}
	}
}
