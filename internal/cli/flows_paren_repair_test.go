package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func buildFakeParinferBinary(t *testing.T, output string, exitCode int) string {
	t.Helper()

	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "main.go")
	bin := filepath.Join(tmpDir, "parinfer-rust")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	prog := `package main

import (
  "fmt"
  "io"
  "os"
)

func main() {
  _, _ = io.ReadAll(os.Stdin)
  fmt.Print(` + "`" + output + "`" + `)
  os.Exit(` + itoaForTest(exitCode) + `)
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake parinfer: %v\n%s", err, string(out))
	}
	return bin
}

func itoaForTest(i int) string {
	if i == 0 {
		return "0"
	}
	return "1"
}

func TestFlowsParenRepair_UsesParinferWhenAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "flow.clj")
	orig := "(defn f [x]\n  (+ x 1)\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	// Pretend parinfer produces a repaired string.
	fake := buildFakeParinferBinary(t, `{"text":"(defn f [x]\n  (+ x 1))\n","success":true,"error":null}`, 0)
	t.Setenv("BREYTA_PARINFER_RUST", fake)

	app := &App{WorkspaceID: "ws-test"}
	cmd := newFlowsParenRepairCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--write=true", path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("parse output json: %v\n%s", err, out.String())
	}
	data := envelope["data"].(map[string]any)
	results := data["results"].([]any)
	r0 := results[0].(map[string]any)
	if r0["engine"] != "parinfer-rust" {
		t.Fatalf("expected engine parinfer-rust, got %v", r0["engine"])
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) == orig {
		t.Fatalf("expected file to change")
	}
}

func TestFlowsParenRepair_FallsBackWhenParinferMissing(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "flow.clj")
	orig := "(defn f [x]\n  (+ x 1)\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}

	// Ensure we don't accidentally pick up a user-installed parinfer-rust from PATH.
	t.Setenv("PATH", "")
	t.Setenv("BREYTA_PARINFER_RUST", "")

	app := &App{WorkspaceID: "ws-test"}
	cmd := newFlowsParenRepairCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--write=true", path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("parse output json: %v\n%s", err, out.String())
	}
	data := envelope["data"].(map[string]any)
	results := data["results"].([]any)
	r0 := results[0].(map[string]any)
	if r0["engine"] != "fallback" {
		t.Fatalf("expected engine fallback, got %v", r0["engine"])
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) == orig {
		t.Fatalf("expected file to change")
	}
}
