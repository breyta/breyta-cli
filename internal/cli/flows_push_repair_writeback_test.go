package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestFlowsPush_RepairWriteback_WritesFileAndUploadsRepairedLiteral(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	var gotLiteral string
	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = method
		_ = cmd
		_ = app
		gotLiteral, _ = payload["flowLiteral"].(string)
		return nil
	}
	useDoAPICommandFn = true

	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "flow.clj")
	content := "(defn f [x]\n  (+ x 1)\n" // missing two closes
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsPushCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", file, "--repair-delimiters=true"})

	// Since our doAPICommandFn stub doesn't write output, we don't care about stdout here.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) == content {
		t.Fatalf("expected file to be rewritten")
	}
	if gotLiteral == "" {
		t.Fatalf("expected flowLiteral payload to be captured")
	}
	if gotLiteral == content {
		t.Fatalf("expected uploaded literal to be repaired")
	}
}

func TestFlowsPush_NoWriteback_DoesNotTouchFile(t *testing.T) {
	origDo := doAPICommandFn
	origUse := useDoAPICommandFn
	t.Cleanup(func() {
		doAPICommandFn = origDo
		useDoAPICommandFn = origUse
	})

	doAPICommandFn = func(cmd *cobra.Command, app *App, method string, payload map[string]any) error {
		_ = cmd
		_ = app
		_ = method
		_ = payload
		return nil
	}
	useDoAPICommandFn = true

	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "flow.clj")
	content := "(defn f [x]\n  (+ x 1)\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	app := &App{WorkspaceID: "ws-test", APIURL: "https://example.invalid", Token: "t", TokenExplicit: true}
	cmd := newFlowsPushCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--file", file, "--repair-delimiters=true", "--no-repair-writeback"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != content {
		t.Fatalf("expected file unchanged")
	}
}
