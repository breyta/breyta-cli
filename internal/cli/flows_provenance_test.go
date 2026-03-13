package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func withAgentWorkspaceCwd(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "flows"), 0o755); err != nil {
		t.Fatalf("mkdir flows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(filepath.Join(root, "flows")); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return root
}

func decodeJSONOutput(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v\n%s", err, buf.String())
	}
	return out
}

func TestFlowsShow_RecordsConsultedFlowInAgentWorkspace(t *testing.T) {
	root := withAgentWorkspaceCwd(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			t.Fatalf("unexpected path: %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-1",
			"data": map[string]any{
				"flow": map[string]any{"flowSlug": "source-flow"},
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-1", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsShowCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"source-flow"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	refs, err := loadConsultedFlowRefsFromStart(root)
	if err != nil {
		t.Fatalf("load consulted refs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 consulted ref, got %d", len(refs))
	}
	if refs[0].WorkspaceID != "ws-1" || refs[0].FlowSlug != "source-flow" {
		t.Fatalf("unexpected consulted ref: %#v", refs[0])
	}
}

func TestFlowsCreate_AddsProvenanceHintsFromConsultedFlows(t *testing.T) {
	root := withAgentWorkspaceCwd(t)
	if err := saveConsultedFlowRefsFromStart(root, []provenanceSourceRef{
		{WorkspaceID: "ws-1", FlowSlug: "source-one"},
	}); err != nil {
		t.Fatalf("save consulted refs: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["command"] != "flows.put_draft" {
			t.Fatalf("unexpected command: %#v", body["command"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-1",
			"data": map[string]any{
				"flowSlug": "new-flow",
			},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-1", APIURL: srv.URL, Token: "t", TokenExplicit: true}
	cmd := newFlowsCreateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--slug", "new-flow"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	envelope := decodeJSONOutput(t, &out)
	hintsAny, ok := envelope["_hints"].([]any)
	if !ok || len(hintsAny) < 2 {
		t.Fatalf("expected _hints in output, got %#v", envelope["_hints"])
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %#v", envelope["meta"])
	}
	candidates, ok := meta["provenanceCandidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("expected one provenance candidate, got %#v", meta["provenanceCandidates"])
	}
	item, ok := candidates[0].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance candidate object, got %#v", candidates[0])
	}
	if item["workspaceId"] != "ws-1" || item["flowSlug"] != "source-one" {
		t.Fatalf("unexpected provenance candidate: %#v", item)
	}
}

func TestFlowsProvenanceSet_FromConsultedBuildsPayload(t *testing.T) {
	root := withAgentWorkspaceCwd(t)
	if err := saveConsultedFlowRefsFromStart(root, []provenanceSourceRef{
		{WorkspaceID: "ws-1", FlowSlug: "source-one"},
		{WorkspaceID: "ws-1", FlowSlug: "target-flow"},
	}); err != nil {
		t.Fatalf("save consulted refs: %v", err)
	}

	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotMethod string
	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		gotMethod = method
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-1", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsProvenanceSetCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"target-flow", "--from-consulted"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if gotMethod != "flows.provenance.set" {
		t.Fatalf("expected flows.provenance.set, got %q", gotMethod)
	}
	if gotPayload["flowSlug"] != "target-flow" {
		t.Fatalf("expected flowSlug target-flow, got %#v", gotPayload["flowSlug"])
	}
	sources, ok := gotPayload["sourceFlows"].([]map[string]any)
	if !ok {
		t.Fatalf("expected sourceFlows payload, got %#v", gotPayload["sourceFlows"])
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source flow after self-filtering, got %#v", sources)
	}
	if sources[0]["workspaceId"] != "ws-1" || sources[0]["flowSlug"] != "source-one" {
		t.Fatalf("unexpected source flow payload: %#v", sources[0])
	}
}

func TestFlowsProvenanceSet_FromConsultedFiltersSelfWithoutWorkspaceContext(t *testing.T) {
	root := withAgentWorkspaceCwd(t)
	if err := saveConsultedFlowRefsFromStart(root, []provenanceSourceRef{
		{WorkspaceID: "ws-1", FlowSlug: "source-one"},
		{WorkspaceID: "ws-1", FlowSlug: "target-flow"},
	}); err != nil {
		t.Fatalf("save consulted refs: %v", err)
	}

	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		if method != "flows.provenance.set" {
			t.Fatalf("expected flows.provenance.set, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsProvenanceSetCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"target-flow", "--from-consulted"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	sources, ok := gotPayload["sourceFlows"].([]map[string]any)
	if !ok {
		t.Fatalf("expected sourceFlows payload, got %#v", gotPayload["sourceFlows"])
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source flow after self-filtering, got %#v", sources)
	}
	if sources[0]["workspaceId"] != "ws-1" || sources[0]["flowSlug"] != "source-one" {
		t.Fatalf("unexpected source flow payload: %#v", sources[0])
	}
}

func TestFlowsProvenanceSet_ClearBuildsEmptyPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		if method != "flows.provenance.set" {
			t.Fatalf("expected flows.provenance.set, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-1", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsProvenanceSetCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"target-flow", "--clear"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	sources, ok := gotPayload["sourceFlows"].([]map[string]any)
	if !ok {
		t.Fatalf("expected empty sourceFlows slice, got %#v", gotPayload["sourceFlows"])
	}
	if len(sources) != 0 {
		t.Fatalf("expected clear payload to send empty sourceFlows, got %#v", sources)
	}
}
