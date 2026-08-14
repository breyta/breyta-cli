package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceExportAndImportUseEngineRESTContract(t *testing.T) {
	bundle := map[string]any{"format": "breyta.workspace", "version": float64(1), "flows": []any{}}
	var imported map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Breyta-Workspace") != "ws-target" {
			t.Errorf("missing workspace header: %q", request.Header.Get("X-Breyta-Workspace"))
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing bearer token: %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/workspace/export":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"bundle": bundle}})
		case "/api/workspace/import":
			if err := json.NewDecoder(request.Body).Decode(&imported); err != nil {
				t.Errorf("decode import: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"unresolved": []any{}}})
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	app := &App{APIURL: server.URL, WorkspaceID: "ws-target", Token: "token", TokenExplicit: true, HTTP: server.Client()}
	outputPath := filepath.Join(t.TempDir(), "workspace.json")
	export := newWorkspaceExportCmd(app)
	export.SetArgs([]string{"--out", outputPath})
	if err := export.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat export: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export permissions = %o, want 600", info.Mode().Perm())
	}

	connectionsPath := filepath.Join(t.TempDir(), "connections.json")
	if err := os.WriteFile(connectionsPath, []byte(`{"source":"destination"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	resourcesPath := filepath.Join(t.TempDir(), "resources.json")
	if err := os.WriteFile(resourcesPath, []byte(`{"res://source":{"status":"intentionally-empty"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	importCommand := newWorkspaceImportCmd(app)
	importCommand.SetArgs([]string{"--file", outputPath, "--connection-mappings", connectionsPath, "--resource-mappings", resourcesPath, "--reviewed-dependencies", "--enable-triggers"})
	if err := importCommand.Execute(); err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported["reviewedDependencies"] != true || imported["enableTriggers"] != true {
		t.Fatalf("operator confirmations not forwarded: %#v", imported)
	}
	if got := imported["bundle"].(map[string]any)["format"]; got != "breyta.workspace" {
		t.Fatalf("imported bundle format = %v", got)
	}
}

func TestWorkspaceExportRefusesToOverwriteWithoutForce(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(outputPath, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	cmd := newWorkspaceExportCmd(&App{APIURL: "https://example.invalid", WorkspaceID: "ws-test", Token: "token", TokenExplicit: true})
	cmd.SetArgs([]string{"--out", outputPath})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read existing output: %v", err)
	}
	if string(contents) != "keep me" {
		t.Fatalf("existing output changed: %q", contents)
	}
}
