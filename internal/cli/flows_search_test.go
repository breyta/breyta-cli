package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
			"ok": true,
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsTemplatesSearchCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"vision", "--step-type", "llm", "--tool-name", "web_search", "--limit", "4", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotWorkspaceHeader != "" {
		t.Fatalf("expected no workspace header, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.search" {
		t.Fatalf("expected method flows.search, got %#v", gotBody["command"])
	}
	gotPayload, _ := gotBody["args"].(map[string]any)
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
		if r.URL.Path != "/api/global/commands" {
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
							"scope":               "template",
							"publish_description": longDescription,
							"steps_text":          longStepsText,
							"step_list":           steps,
							"matchedSurfaces":     []any{"definition", "tools"},
							"matchedPatterns":     []any{"web_search"},
							"matchPreviews": []any{
								map[string]any{"surface": "definition", "pattern": "web_search", "text": "...web_search..."},
							},
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
	if hint, _ := meta["hint"].(string); !strings.Contains(hint, "Flow search results are compact") {
		t.Fatalf("expected generic compact search hint, got %#v", meta["hint"])
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
	if hit["hitRef"] != "template:template-agent" || hit["nextCommand"] != "breyta flows templates search 'template-agent' --full" {
		t.Fatalf("expected compact hit ref and next command, got %#v", hit)
	}
	if hit["duplicateCommand"] != "breyta flows templates duplicate 'template-agent'" {
		t.Fatalf("expected duplicate command, got %#v", hit["duplicateCommand"])
	}
	if hit["inspectCommand"] != "breyta flows templates search 'template-agent' --full" {
		t.Fatalf("expected inspect command, got %#v", hit["inspectCommand"])
	}
	if surfaces, _ := hit["matchedSurfaces"].([]any); len(surfaces) != 2 {
		t.Fatalf("expected matched surfaces to survive compaction, got %#v", hit["matchedSurfaces"])
	}
	if previews, _ := hit["matchPreviews"].([]any); len(previews) != 1 {
		t.Fatalf("expected match previews to survive compaction, got %#v", hit["matchPreviews"])
	}
}

func TestFlowsTemplatesDuplicate_BuildsPayload(t *testing.T) {
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
	cmd := newFlowsTemplatesDuplicateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"ad-creative-studio-2", "--slug", "ad-creative-studio-copy", "--name", "Ad Creative Studio Copy", "--description", "Draft copy", "--catalog-scope", "workspace", "--replace"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.templates.duplicate" {
		t.Fatalf("expected method flows.templates.duplicate, got %q", gotMethod)
	}
	if gotPayload["templateSlug"] != "ad-creative-studio-2" {
		t.Fatalf("expected templateSlug, got %#v", gotPayload)
	}
	if gotPayload["targetSlug"] != "ad-creative-studio-copy" || gotPayload["name"] != "Ad Creative Studio Copy" || gotPayload["description"] != "Draft copy" {
		t.Fatalf("unexpected duplicate payload: %#v", gotPayload)
	}
	if gotPayload["scope"] != "workspace" || gotPayload["replace"] != true {
		t.Fatalf("unexpected duplicate scope/replace: %#v", gotPayload)
	}
}

func TestFlowsTemplatesDuplicate_RequiresWorkspace(t *testing.T) {
	app := &App{APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsTemplatesDuplicateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"ad-creative-studio-2"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "requires --workspace or BREYTA_WORKSPACE") {
		t.Fatalf("unexpected error: %v", err)
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
	cmd.SetArgs([]string{"web_search", "--or", "web_search_preview", "--step-type", "agent", "--tool-name", "web_search", "--connection", "openai", "--surface", "definition,tools", "--target", "draft", "--limit", "3"})

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
	matchSurfaces, _ := gotPayload["matchSurfaces"].([]string)
	if len(matchSurfaces) != 2 || matchSurfaces[0] != "definition" || matchSurfaces[1] != "tools" {
		t.Fatalf("unexpected matchSurfaces: %#v", gotPayload["matchSurfaces"])
	}
}

func TestFlowsTemplatesGrep_BuildsTemplateDefinitionSearchPayload(t *testing.T) {
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
			"ok": true,
			"data": map[string]any{
				"result": map[string]any{"hits": []any{}},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsTemplatesGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"image/*", "--or", ":multiple true", "--step-type", "llm", "--surface", "definition", "--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotWorkspaceHeader != "" {
		t.Fatalf("expected no workspace header, got %q", gotWorkspaceHeader)
	}
	if gotBody["command"] != "flows.search" {
		t.Fatalf("expected method flows.search, got %#v", gotBody["command"])
	}
	gotPayload, _ := gotBody["args"].(map[string]any)
	if gotPayload["definitionSearch"] != true || gotPayload["query"] != "image/*" || gotPayload["scope"] != "all" || gotPayload["surface"] != "templates" || gotPayload["includeDefinition"] != true {
		t.Fatalf("unexpected template grep payload: %#v", gotPayload)
	}
	if gotPayload["rawDefinition"] != false {
		t.Fatalf("expected rawDefinition=false, got %#v", gotPayload["rawDefinition"])
	}
	matchSurfaces := sliceAny(gotPayload["matchSurfaces"])
	if len(matchSurfaces) != 1 || matchSurfaces[0] != "definition" {
		t.Fatalf("unexpected matchSurfaces: %#v", gotPayload["matchSurfaces"])
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

func TestFlowsGrep_RejectsInvalidMatchSurface(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"web_search", "--surface", "providers"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--surface must be one of definition, steps, tools, connections, description") {
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

func TestFlowsGrep_FullIncludesWorkspaceDefinition(t *testing.T) {
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
	cmd.SetArgs([]string{"web_search", "--full"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.workspace.search" {
		t.Fatalf("expected method flows.workspace.search, got %q", gotMethod)
	}
	if gotPayload["definitionSearch"] != true || gotPayload["includeDefinition"] != true {
		t.Fatalf("expected definition source preview enabled, got %#v", gotPayload)
	}
	if gotPayload["rawDefinition"] != false {
		t.Fatalf("expected rawDefinition=false without --raw-definition, got %#v", gotPayload["rawDefinition"])
	}
}

func TestFlowsGrep_TemplateScopeFullIncludesRawDefinition(t *testing.T) {
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
	cmd.SetArgs([]string{"web_search", "--scope", "templates", "--full", "--raw-definition"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.search" {
		t.Fatalf("expected method flows.search, got %q", gotMethod)
	}
	if gotPayload["surface"] != "templates" || gotPayload["includeDefinition"] != true || gotPayload["rawDefinition"] != true {
		t.Fatalf("expected template source preview with raw definition, got %#v", gotPayload)
	}
}

func TestFlowsGrep_RejectsRawDefinitionWithoutFull(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsGrepCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"web_search", "--raw-definition"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error, got success")
	}
	if !strings.Contains(err.Error(), "--raw-definition requires --full") {
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

func TestFlowsReadinessAggregatesDoctorConfigureAndPublicPreflight(t *testing.T) {
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
		args := mapStringAny(body["args"])
		seenCommands = append(seenCommands, command)
		switch command {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "public-flow",
						"target":   "live",
						"ready":    true,
						"summary":  map[string]any{"activeVersion": 3, "latestVersion": 4},
						"checks":   []map[string]any{{"id": "definition", "label": "Flow definition", "pass": true}},
					},
				},
				"meta": map[string]any{
					"webUrl":       "https://localhost:33156/ws-test/flows/public-flow",
					"nextCommands": []string{"breyta flows run public-flow --target live --wait"},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"ready": true, "flowSlug": "public-flow", "target": "live"},
			})
		case "flows.public.preflight":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"preflight": map[string]any{
						"flowSlug": "public-flow",
						"ready":    false,
						"public":   map[string]any{"discoverPublic": false, "marketplaceVisible": true},
						"pricing":  map[string]any{"type": "subscription", "amount": "19.99"},
						"checks":   []map[string]any{{"id": "discover-public", "label": "Discover visibility", "pass": false}},
					},
				},
				"meta": map[string]any{"nextCommands": []string{"breyta flows public preflight public-flow"}},
			})
		case "flows.invocations.metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"items": []map[string]any{{
						"installationId":   "inst-1",
						"entrypointId":     "manual",
						"invocationKind":   "manual",
						"interfaceScope":   "installation",
						"lastWorkflowId":   "wf-installed-1",
						"lastCalledAt":     "2026-05-18T10:00:00Z",
						"lastStatus":       "completed",
						"lastStatusBucket": "2xx",
						"requestCount":     3,
						"successCount":     2,
						"errorCount":       1,
					}},
				},
			})
		case "flows.diff":
			if args["from"] != "live" || args["to"] != "draft" || args["view"] != "summary" {
				t.Fatalf("unexpected readiness diff args: %#v", args)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug": "public-flow",
					"from":     map[string]any{"source": "live", "version": 3},
					"to":       map[string]any{"source": "draft", "version": 4},
					"changed":  true,
					"stat":     map[string]any{"additions": 2, "deletions": 1, "hunks": 1},
					"changedSections": []string{
						"@@ -10,7 +10,8 @@",
					},
				},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsReadinessCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"public-flow", "--target", "live", "--public"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("readiness execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check,flows.public.preflight,flows.invocations.metrics,flows.diff" {
		t.Fatalf("unexpected commands: %s", got)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	readiness := mapStringAny(mapStringAny(envelope["data"])["readiness"])
	if readiness["ready"] != false {
		t.Fatalf("expected readiness ready=false, got %#v", readiness["ready"])
	}
	if readiness["configurationReady"] != true {
		t.Fatalf("expected configurationReady=true, got %#v", readiness["configurationReady"])
	}
	if mapStringAny(readiness["pricing"])["amount"] != "19.99" {
		t.Fatalf("expected pricing in readiness, got %#v", readiness["pricing"])
	}
	if _, ok := readiness["doctor"]; ok {
		t.Fatalf("expected compact readiness to omit raw doctor by default, got %#v", readiness["doctor"])
	}
	if _, ok := readiness["publicPreflight"]; ok {
		t.Fatalf("expected compact readiness to omit raw public preflight by default, got %#v", readiness["publicPreflight"])
	}
	if got := mapStringAny(envelope["meta"])["webUrl"]; got != "http://localhost:33156/ws-test/flows/public-flow" {
		t.Fatalf("expected local webUrl to use http, got %#v", got)
	}
	if len(sliceAny(readiness["blockers"])) != 1 {
		t.Fatalf("expected one blocker, got %#v", readiness["blockers"])
	}
	blocker := mapStringAny(sliceAny(readiness["blockers"])[0])
	if blocker["status"] != "fail" {
		t.Fatalf("expected blocking public check status=fail, got %#v", blocker)
	}
	if blocker["fixCommand"] != "breyta flows discover update public-flow --public=true" {
		t.Fatalf("expected discover fix command, got %#v", blocker["fixCommand"])
	}
	if blocker["openUrl"] != srv.URL+"/ws-test/discover" {
		t.Fatalf("expected discover blocker open URL, got %#v", blocker["openUrl"])
	}
	latestInstalled := mapStringAny(readiness["latestInstalledRun"])
	if latestInstalled["workflowId"] != "wf-installed-1" || latestInstalled["installationId"] != "inst-1" {
		t.Fatalf("expected latest installed run context, got %#v", latestInstalled)
	}
	diff := mapStringAny(readiness["diff"])
	if diff["changed"] != true {
		t.Fatalf("expected compact changed diff, got %#v", diff)
	}
	diffStat := mapStringAny(diff["stat"])
	if diffStat["additions"] != float64(2) || diffStat["deletions"] != float64(1) || diffStat["hunks"] != float64(1) {
		t.Fatalf("expected compact diff stat, got %#v", diffStat)
	}
	draftLive := mapStringAny(readiness["draftLive"])
	if draftLive["changed"] != true || mapStringAny(draftLive["stat"])["additions"] != float64(2) {
		t.Fatalf("expected draftLive diff summary, got %#v", draftLive)
	}
	urls := mapStringAny(readiness["urls"])
	if urls["flow"] != srv.URL+"/ws-test/flows/public-flow" {
		t.Fatalf("expected flow URL, got %#v", urls["flow"])
	}
	if urls["publicApp"] != srv.URL+"/apps/public-flow" {
		t.Fatalf("expected public app URL, got %#v", urls["publicApp"])
	}
	if urls["install"] != srv.URL+"/ws-test/flows/public-flow/installations" {
		t.Fatalf("expected install URL, got %#v", urls["install"])
	}
	if urls["installation"] != srv.URL+"/ws-test/flows/public-flow/installations/inst-1" {
		t.Fatalf("expected latest installation URL, got %#v", urls["installation"])
	}
	if urls["installationSetup"] != srv.URL+"/ws-test/flows/public-flow/installations/inst-1?configure=setup" {
		t.Fatalf("expected latest installation setup URL, got %#v", urls["installationSetup"])
	}
	if urls["latestRun"] != srv.URL+"/ws-test/runs/public-flow/wf-installed-1" {
		t.Fatalf("expected latest run URL, got %#v", urls["latestRun"])
	}
	nextCommands := stringSlice(mapStringAny(envelope["meta"])["nextCommands"])
	if !slices.Contains(nextCommands, "breyta flows discover update public-flow --public=true") {
		t.Fatalf("expected blocker fix command in nextCommands, got %#v", nextCommands)
	}
	if !slices.Contains(nextCommands, "breyta flows diff public-flow") {
		t.Fatalf("expected changed diff command in nextCommands, got %#v", nextCommands)
	}
	nextActions := sliceAny(mapStringAny(envelope["meta"])["nextActions"])
	if !hasReadinessNextAction(nextActions, "open-public-app", srv.URL+"/apps/public-flow") {
		t.Fatalf("expected public app next action, got %#v", nextActions)
	}
	if !hasReadinessNextAction(nextActions, "configure-latest-installation", srv.URL+"/ws-test/flows/public-flow/installations/inst-1?configure=setup") {
		t.Fatalf("expected configure latest installation next action, got %#v", nextActions)
	}
	if !hasReadinessNextAction(nextActions, "open-latest-run", srv.URL+"/ws-test/runs/public-flow/wf-installed-1") {
		t.Fatalf("expected latest run next action, got %#v", nextActions)
	}
}

func hasReadinessNextAction(actions []any, id string, rawURL string) bool {
	for _, item := range actions {
		action := mapStringAny(item)
		if action != nil && action["id"] == id && action["url"] == rawURL {
			return true
		}
	}
	return false
}

func TestFlowsReleaseCheckNoLiveFiltersInvalidLiveRunCommand(t *testing.T) {
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
						"flowSlug": "draft-only-flow",
						"target":   "live",
						"ready":    false,
						"summary": map[string]any{
							"activeVersion":       nil,
							"latestVersion":       2,
							"stepCount":           0,
							"interfaceCount":      0,
							"draftStepCount":      2,
							"draftInterfaceCount": 1,
							"liveUnavailable":     true,
							"countSource":         "live-unavailable",
						},
						"checks": []map[string]any{{"id": "live-version", "label": "Live version", "pass": false}},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{"breyta flows run draft-only-flow --target live --wait"},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"ready": true, "flowSlug": "draft-only-flow", "target": "live"},
			})
		case "flows.public.preflight":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"preflight": map[string]any{
						"flowSlug": "draft-only-flow",
						"ready":    false,
						"public":   map[string]any{"discoverPublic": false, "marketplaceVisible": false},
						"checks":   []map[string]any{{"id": "released", "label": "Released version", "pass": false}},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{"breyta flows run draft-only-flow --target live --wait"},
				},
			})
		case "flows.invocations.metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"items": []any{}},
			})
		case "flows.diff":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"changed": false},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsReleaseCheckCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"draft-only-flow", "--public", "--marketplace"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("release-check execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check,flows.public.preflight,flows.invocations.metrics,flows.diff" {
		t.Fatalf("unexpected commands: %s", got)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	readiness := mapStringAny(mapStringAny(envelope["data"])["readiness"])
	if readiness["liveUnavailable"] != true {
		t.Fatalf("expected liveUnavailable=true, got %#v", readiness["liveUnavailable"])
	}
	nextJoined := strings.Join(stringSlice(mapStringAny(envelope["meta"])["nextCommands"]), "\n")
	if strings.Contains(nextJoined, "flows run draft-only-flow --target live") {
		t.Fatalf("release-check should not suggest known-invalid live run:\n%s", nextJoined)
	}
	if !strings.Contains(nextJoined, "breyta flows release draft-only-flow") {
		t.Fatalf("expected release command before live run proof:\n%s", nextJoined)
	}
}

func TestFlowsReleaseCheckConfiguredLiveWithoutActiveFiltersInvalidLiveRunCommand(t *testing.T) {
	seenCommands := []string{}
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		command, _ := body["command"].(string)
		seenCommands = append(seenCommands, command)
		switch command {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "draft-configured-flow",
						"target":   "live",
						"ready":    false,
						"summary": map[string]any{
							"activeVersion": nil,
							"latestVersion": 2,
							"stepCount":     0,
						},
						"checks": []map[string]any{{"id": "live-version", "label": "Live version", "pass": false}},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{"breyta flows run draft-configured-flow --target live --wait"},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug": "draft-configured-flow",
					"target":   "live",
					"version":  2,
					"ready":    true,
				},
			})
		case "flows.public.preflight":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"preflight": map[string]any{
						"flowSlug": "draft-configured-flow",
						"target":   "live",
						"ready":    false,
						"public":   map[string]any{"discoverPublic": false, "marketplaceVisible": false},
						"checks":   []map[string]any{{"id": "released", "label": "Released version", "pass": false}},
					},
				},
				"meta": map[string]any{
					"nextCommands": []string{"breyta flows run draft-configured-flow --target live --wait"},
				},
			})
		case "flows.invocations.metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"items": []any{}},
			})
		case "flows.diff":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"changed": false},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsReleaseCheckCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"draft-configured-flow", "--public"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("release-check execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check,flows.public.preflight,flows.invocations.metrics,flows.diff" {
		t.Fatalf("unexpected commands: %s", got)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	readiness := mapStringAny(mapStringAny(envelope["data"])["readiness"])
	if readiness["liveUnavailable"] != true {
		t.Fatalf("expected liveUnavailable=true, got %#v", readiness["liveUnavailable"])
	}
	summary := mapStringAny(readiness["summary"])
	if got := fmt.Sprint(summary["liveConfiguredVersion"]); got != "2" {
		t.Fatalf("expected liveConfiguredVersion=2, got %#v", summary["liveConfiguredVersion"])
	}
	nextJoined := strings.Join(stringSlice(mapStringAny(envelope["meta"])["nextCommands"]), "\n")
	if strings.Contains(nextJoined, "flows run draft-configured-flow --target live") {
		t.Fatalf("release-check should not suggest live run before release:\n%s", nextJoined)
	}
	if !strings.Contains(nextJoined, "breyta flows release draft-configured-flow") {
		t.Fatalf("expected release command before live run proof:\n%s", nextJoined)
	}
}

func TestFlowsReadinessFullIncludesRawPayloads(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["command"] {
		case "flows.doctor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "public-flow",
						"target":   "draft",
						"ready":    true,
						"summary":  map[string]any{"activeVersion": 1, "latestVersion": 1},
						"checks":   []map[string]any{{"id": "definition", "label": "Flow definition", "pass": true}},
					},
				},
			})
		case "flows.configure.check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"ready": true, "flowSlug": "public-flow", "target": "draft"},
			})
		case "flows.public.preflight":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"preflight": map[string]any{
						"flowSlug": "public-flow",
						"ready":    true,
						"public":   map[string]any{"discoverPublic": false, "marketplaceVisible": false},
						"checks":   []map[string]any{{"id": "discover-public", "label": "Discover visibility", "pass": true}},
					},
				},
			})
		case "flows.invocations.metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"items": []map[string]any{}},
			})
		case "flows.diff":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug": "public-flow",
					"from":     map[string]any{"source": "live", "version": 1},
					"to":       map[string]any{"source": "draft", "version": 1},
					"changed":  false,
					"stat":     map[string]any{"additions": 0, "deletions": 0, "hunks": 0},
				},
			})
		default:
			t.Fatalf("unexpected command: %s", body["command"])
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsReadinessCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"public-flow", "--full"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("readiness execute: %v\n%s", err, out.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	readiness := mapStringAny(mapStringAny(envelope["data"])["readiness"])
	if mapStringAny(readiness["doctor"]) == nil {
		t.Fatalf("expected --full readiness to include raw doctor, got %#v", readiness)
	}
	if mapStringAny(readiness["publicPreflight"]) == nil {
		t.Fatalf("expected --full readiness to include raw public preflight, got %#v", readiness)
	}
}

func TestFlowsReadinessDefaultKeepsPublicSnapshotNonBlocking(t *testing.T) {
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
		args := mapStringAny(body["args"])
		seenCommands = append(seenCommands, command)
		switch command {
		case "flows.doctor":
			if args["target"] != "draft" {
				t.Fatalf("expected default readiness target draft, got %#v", args["target"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"doctor": map[string]any{
						"flowSlug": "private-flow",
						"target":   "draft",
						"ready":    true,
						"summary":  map[string]any{"activeVersion": 1, "latestVersion": 1},
						"checks":   []map[string]any{{"id": "definition", "label": "Flow definition", "pass": true}},
					},
				},
				"meta": map[string]any{"nextCommands": []string{"breyta flows run private-flow --target draft --wait"}},
			})
		case "flows.configure.check":
			if args["target"] != "draft" {
				t.Fatalf("expected default readiness configure target draft, got %#v", args["target"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"ready": true, "flowSlug": "private-flow", "target": "draft"},
			})
		case "flows.public.preflight":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"preflight": map[string]any{
						"flowSlug": "private-flow",
						"ready":    false,
						"public":   map[string]any{"discoverPublic": false, "marketplaceVisible": false},
						"checks":   []map[string]any{{"id": "discover-public", "label": "Discover visibility", "pass": false}},
					},
				},
				"meta": map[string]any{"nextCommands": []string{"breyta flows public preflight private-flow"}},
			})
		case "flows.invocations.metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data":        map[string]any{"items": []map[string]any{}},
			})
		case "flows.diff":
			if args["from"] != "live" || args["to"] != "draft" || args["view"] != "summary" {
				t.Fatalf("unexpected readiness diff args: %#v", args)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":          true,
				"workspaceId": "ws-test",
				"data": map[string]any{
					"flowSlug": "private-flow",
					"from":     map[string]any{"source": "live", "version": 1},
					"to":       map[string]any{"source": "draft", "version": 1},
					"changed":  false,
					"stat":     map[string]any{"additions": 0, "deletions": 0, "hunks": 0},
				},
			})
		default:
			t.Fatalf("unexpected command: %s", command)
		}
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-test", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsReadinessCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"private-flow"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("readiness execute: %v\n%s", err, out.String())
	}
	if got := strings.Join(seenCommands, ","); got != "flows.doctor,flows.configure.check,flows.public.preflight,flows.invocations.metrics,flows.diff" {
		t.Fatalf("unexpected commands: %s", got)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	readiness := mapStringAny(mapStringAny(envelope["data"])["readiness"])
	if readiness["ready"] != true {
		t.Fatalf("expected readiness ready=true, got %#v", readiness["ready"])
	}
	if readiness["publicIncluded"] != true || readiness["publicRequired"] != false {
		t.Fatalf("expected included-but-not-required public snapshot, got %#v", readiness)
	}
	if readiness["marketplaceRequired"] != false {
		t.Fatalf("expected marketplaceRequired=false, got %#v", readiness["marketplaceRequired"])
	}
	if len(sliceAny(readiness["blockers"])) != 0 {
		t.Fatalf("expected no blockers, got %#v", readiness["blockers"])
	}
	var publicCheck map[string]any
	for _, item := range sliceAny(readiness["checks"]) {
		check := mapStringAny(item)
		if check != nil && check["id"] == "discover-public" {
			publicCheck = check
			break
		}
	}
	if publicCheck == nil || publicCheck["status"] != "warn" {
		t.Fatalf("expected non-required failed public check to be status=warn, got %#v", publicCheck)
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
