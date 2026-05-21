package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsUpdate_BuildsGroupingPayload(t *testing.T) {
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
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--group-key", "billing-core",
		"--group-name", "Billing Core",
		"--group-description", "Shared billing flows",
		"--group-order", "20",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if gotMethod != "flows.update" {
		t.Fatalf("expected method flows.update, got %q", gotMethod)
	}
	if gotPayload["flowSlug"] != "demo-flow" {
		t.Fatalf("expected flowSlug=demo-flow, got %#v", gotPayload["flowSlug"])
	}
	if gotPayload["groupKey"] != "billing-core" {
		t.Fatalf("expected groupKey=billing-core, got %#v", gotPayload["groupKey"])
	}
	if gotPayload["groupName"] != "Billing Core" {
		t.Fatalf("expected groupName=Billing Core, got %#v", gotPayload["groupName"])
	}
	if gotPayload["groupDescription"] != "Shared billing flows" {
		t.Fatalf("expected groupDescription=Shared billing flows, got %#v", gotPayload["groupDescription"])
	}
	if gotPayload["groupOrder"] != 20 {
		t.Fatalf("expected groupOrder=20, got %#v", gotPayload["groupOrder"])
	}
}

func TestFlowsUpdate_BuildsGroupClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--group-key", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["groupKey"]
	if !ok {
		t.Fatalf("expected groupKey to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected groupKey to be empty string for explicit clear, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsGroupOrderClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--group-order", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["groupOrder"]
	if !ok {
		t.Fatalf("expected groupOrder to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected groupOrder to be empty string for explicit clear, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPublishDescriptionPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--publish-description", "## Install\n\nUse this flow."})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishDescription"]
	if !ok {
		t.Fatalf("expected publishDescription to be present in payload")
	}
	if value != "## Install\n\nUse this flow." {
		t.Fatalf("expected publishDescription markdown, got %#v", value)
	}
}

func TestFlowsUpdate_AddsPublicAppURLHint(t *testing.T) {
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["command"] != "flows.update" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]any{"message": "unexpected command"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"workspaceId": "ws-acme",
			"meta": map[string]any{
				"nextActions": []map[string]any{{
					"id":    "open-public-app",
					"label": "Open old public app",
					"url":   "https://old.example/apps/market-flow",
				}},
			},
			"data": map[string]any{"flowSlug": "market-flow"},
		})
	}))
	defer srv.Close()

	app := &App{WorkspaceID: "ws-acme", APIURL: srv.URL, Token: "user-dev", TokenExplicit: true, DevMode: true, PrettyJSON: true}
	cmd := newFlowsUpdateCmd(app)
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{
		"market-flow",
		"--publish-description", "## Install\n\nUse this flow.",
	})
	err := cmd.Execute()
	stdout := outBuf.String()
	if err != nil {
		t.Fatalf("flows update failed: %v\n%s", err, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid json output: %v\n---\n%s", err, stdout)
	}
	meta, _ := out["meta"].(map[string]any)
	if meta["publicAppUrl"] != "https://breyta.ai/apps/market-flow" {
		t.Fatalf("expected public app URL hint, got %#v", meta["publicAppUrl"])
	}
	count := 0
	for _, item := range sliceAny(meta["nextActions"]) {
		action := mapStringAny(item)
		if action["id"] == "open-public-app" {
			count++
			if action["url"] != "https://breyta.ai/apps/market-flow" {
				t.Fatalf("expected public app action URL to be replaced, got %#v", action)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one public app next action, got %d in %#v", count, meta["nextActions"])
	}
}

func TestFlowsUpdate_PreservesPublishDescriptionMarkdownWhitespace(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	rawMarkdown := "    code block\nline with hard break  "

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--publish-description", rawMarkdown})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishDescription"]
	if !ok {
		t.Fatalf("expected publishDescription to be present in payload")
	}
	if value != rawMarkdown {
		t.Fatalf("expected publishDescription markdown to preserve whitespace, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPublishDescriptionClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--publish-description", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishDescription"]
	if !ok {
		t.Fatalf("expected publishDescription to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected publishDescription to be empty string for explicit clear, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPublishDescriptionFromFilePayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "publish-description.md")
	if err := os.WriteFile(path, []byte("## Install\n\nFrom file."), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--publish-description-file", path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishDescription"]
	if !ok {
		t.Fatalf("expected publishDescription to be present in payload")
	}
	if value != "## Install\n\nFrom file." {
		t.Fatalf("expected publishDescription markdown from file, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPublishMediaPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--publish-media-type", "video",
		"--publish-media-source-kind", "https-url",
		"--publish-media-source", "https://cdn.example.com/hero.mp4",
		"--publish-media-poster-kind", "flow-resource",
		"--publish-media-poster", "res://v1/ws/ws-test/result/run/run-1/step/poster",
		"--publish-media-alt", "Generated hero",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishMedia"]
	if !ok {
		t.Fatalf("expected publishMedia to be present in payload")
	}
	media, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected publishMedia object, got %T", value)
	}
	if media["type"] != "video" {
		t.Fatalf("expected publishMedia.type=video, got %#v", media["type"])
	}
	source, ok := media["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected publishMedia.source object, got %T", media["source"])
	}
	if source["kind"] != "https-url" || source["url"] != "https://cdn.example.com/hero.mp4" {
		t.Fatalf("unexpected publishMedia.source: %#v", source)
	}
	poster, ok := media["posterSource"].(map[string]any)
	if !ok {
		t.Fatalf("expected publishMedia.posterSource object, got %T", media["posterSource"])
	}
	if poster["kind"] != "flow-resource" || poster["uri"] != "res://v1/ws/ws-test/result/run/run-1/step/poster" {
		t.Fatalf("unexpected publishMedia.posterSource: %#v", poster)
	}
	if media["alt"] != "Generated hero" {
		t.Fatalf("expected publishMedia.alt=Generated hero, got %#v", media["alt"])
	}
}

func TestFlowsUpdate_BuildsPublishMediaPayloadFromSourceFile(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	origUpload := publishMediaUploadFileResource
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
		publishMediaUploadFileResource = origUpload
	})

	var gotPayload map[string]any
	var uploadedPath string
	publishMediaUploadFileResource = func(_ context.Context, _ *App, path string, filename string, contentType string) (map[string]any, error) {
		uploadedPath = path
		if filename != "hero.png" {
			t.Fatalf("expected upload filename hero.png, got %q", filename)
		}
		if contentType != "" {
			t.Fatalf("expected inferred content type, got %q", contentType)
		}
		return map[string]any{"resourceUri": "res://v1/ws/ws-test/file/uploaded-hero"}, nil
	}
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	tmp := t.TempDir()
	path := filepath.Join(tmp, "hero.png")
	if err := os.WriteFile(path, []byte("fake image"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--publish-media-type", "image",
		"--publish-media-source-file", path,
		"--publish-media-alt", "Uploaded hero",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if uploadedPath != path {
		t.Fatalf("expected upload path %q, got %q", path, uploadedPath)
	}
	media, ok := gotPayload["publishMedia"].(map[string]any)
	if !ok {
		t.Fatalf("expected publishMedia object, got %T", gotPayload["publishMedia"])
	}
	source, ok := media["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected publishMedia.source object, got %T", media["source"])
	}
	if source["kind"] != "flow-resource" || source["uri"] != "res://v1/ws/ws-test/file/uploaded-hero" {
		t.Fatalf("unexpected publishMedia.source: %#v", source)
	}
}

func TestFlowsUpdate_ValidatesUpdateBeforePublishMediaSourceFileUpload(t *testing.T) {
	origUpload := publishMediaUploadFileResource
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		publishMediaUploadFileResource = origUpload
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	doAPICommandFn = func(_ *cobra.Command, _ *App, _ string, _ map[string]any) error {
		t.Fatalf("flows.update should not run after validation fails")
		return nil
	}
	useDoAPICommandFn = true

	tmp := t.TempDir()
	sourcePath := filepath.Join(tmp, "hero.png")
	if err := os.WriteFile(sourcePath, []byte("fake image"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		args        []string
		wantMessage string
	}{
		{
			name: "group order",
			args: []string{
				"demo-flow",
				"--publish-media-type", "image",
				"--publish-media-source-file", sourcePath,
				"--group-order", "bad",
			},
			wantMessage: "invalid --group-order",
		},
		{
			name: "publish description file",
			args: []string{
				"demo-flow",
				"--publish-media-type", "image",
				"--publish-media-source-file", sourcePath,
				"--publish-description-file", filepath.Join(tmp, "missing.md"),
			},
			wantMessage: "read --publish-description-file",
		},
		{
			name: "poster media",
			args: []string{
				"demo-flow",
				"--publish-media-type", "image",
				"--publish-media-source-file", sourcePath,
				"--publish-media-poster-kind", "https-url",
				"--publish-media-poster", "https://cdn.example.com/poster.jpg",
			},
			wantMessage: "poster media is only supported for video publish media",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uploadCalled := false
			publishMediaUploadFileResource = func(_ context.Context, _ *App, _ string, _ string, _ string) (map[string]any, error) {
				uploadCalled = true
				return map[string]any{"resourceUri": "res://v1/ws/ws-test/file/should-not-exist"}, nil
			}

			app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
			cmd := newFlowsUpdateCmd(app)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected execute to fail")
			}
			if uploadCalled {
				t.Fatalf("upload should not run before update validation succeeds")
			}
			if !strings.Contains(out.String(), tc.wantMessage) {
				t.Fatalf("expected error to contain %q, got %q", tc.wantMessage, out.String())
			}
		})
	}
}

func TestFlowsUpdate_RejectsPublishMediaSourceFileInExplicitMockMode(t *testing.T) {
	origUpload := publishMediaUploadFileResource
	t.Cleanup(func() {
		publishMediaUploadFileResource = origUpload
	})
	publishMediaUploadFileResource = func(_ context.Context, _ *App, _ string, _ string, _ string) (map[string]any, error) {
		t.Fatalf("upload should not run in explicit mock mode")
		return nil, nil
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "hero.png")
	if err := os.WriteFile(path, []byte("fake image"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{WorkspaceID: "ws-test", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	cmd.Flags().StringVar(&app.APIURL, "api", "", "API base URL")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--api=",
		"--publish-media-type", "image",
		"--publish-media-source-file", path,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected explicit mock mode to reject publish media file upload")
	}
	if !strings.Contains(out.String(), "--publish-media-source-file requires API mode") {
		t.Fatalf("expected API mode error, got %q", out.String())
	}
	if strings.TrimSpace(app.APIURL) != "" {
		t.Fatalf("explicit mock mode should not be replaced by configured API URL, got %q", app.APIURL)
	}
}

func TestFlowsUpdate_BuildsPublishMediaClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--clear-publish-media"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishMedia"]
	if !ok {
		t.Fatalf("expected publishMedia to be present in payload")
	}
	if value != nil {
		t.Fatalf("expected publishMedia to be nil for explicit clear, got %#v", value)
	}
}

func TestFlowsUpdate_RejectsPublishMediaWhenIncomplete(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--publish-media-type", "image",
		"--publish-media-source", "https://cdn.example.com/hero.png",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected execute to fail for incomplete publish media flags")
	}
	if !strings.Contains(out.String(), "--publish-media-source-kind") {
		t.Fatalf("expected error to mention missing source kind, got %q", out.String())
	}
}

func TestFlowsUpdate_RejectsPublishMediaClearMixedWithSetFlags(t *testing.T) {
	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"demo-flow",
		"--clear-publish-media",
		"--publish-media-type", "image",
		"--publish-media-source-kind", "https-url",
		"--publish-media-source", "https://cdn.example.com/hero.png",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected execute to fail when clear flag is mixed with publish media setters")
	}
	if !strings.Contains(out.String(), "--clear-publish-media cannot be combined") {
		t.Fatalf("expected clear/set conflict error, got %q", out.String())
	}
}

func TestFlowsUpdate_PreservesPublishDescriptionFileWhitespace(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	rawMarkdown := "    code block\nline with hard break  \n"
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "publish-description.md")
	if err := os.WriteFile(path, []byte(rawMarkdown), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--publish-description-file", path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["publishDescription"]
	if !ok {
		t.Fatalf("expected publishDescription to be present in payload")
	}
	if value != rawMarkdown {
		t.Fatalf("expected publishDescription file markdown to preserve whitespace, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPrimaryDisplayConnectionSlotPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--primary-display-connection-slot", "crm_main"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["primaryDisplayConnectionSlot"]
	if !ok {
		t.Fatalf("expected primaryDisplayConnectionSlot to be present in payload")
	}
	if value != "crm_main" {
		t.Fatalf("expected primaryDisplayConnectionSlot=crm_main, got %#v", value)
	}
}

func TestFlowsUpdate_BuildsPrimaryDisplayConnectionSlotClearPayload(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotPayload map[string]any
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		if method != "flows.update" {
			t.Fatalf("expected method flows.update, got %q", method)
		}
		gotPayload = payload
		return nil
	}
	useDoAPICommandFn = true

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsUpdateCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"demo-flow", "--primary-display-connection-slot", ""})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	value, ok := gotPayload["primaryDisplayConnectionSlot"]
	if !ok {
		t.Fatalf("expected primaryDisplayConnectionSlot to be present in payload")
	}
	if value != "" {
		t.Fatalf("expected primaryDisplayConnectionSlot to be empty string for explicit clear, got %#v", value)
	}
}
