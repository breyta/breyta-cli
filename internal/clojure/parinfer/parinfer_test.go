package parinfer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func buildFakeParinfer(t *testing.T, output string, exitCode int) string {
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
  os.Exit(` + itoa(exitCode) + `)
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

func itoa(i int) string {
	// tiny local helper to avoid fmt in generated source.
	if i == 0 {
		return "0"
	}
	if i == 1 {
		return "1"
	}
	if i == 2 {
		return "2"
	}
	if i == 127 {
		return "127"
	}
	// good enough for tests we use.
	return "1"
}

func TestRunner_NotAvailable(t *testing.T) {
	_, _, err := (Runner{}).RepairIndent("(+ 1 2)")
	if err != ErrNotAvailable {
		t.Fatalf("expected ErrNotAvailable, got %v", err)
	}
}

func TestRunner_SuccessJSON(t *testing.T) {
	fake := buildFakeParinfer(t, `{"text":"(ok)","success":true,"error":null}`, 0)
	out, ans, err := (Runner{BinaryPath: fake}).RepairIndent("(broken")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != "(ok)" {
		t.Fatalf("unexpected out: %q", out)
	}
	if !ans.Success || ans.Text != "(ok)" {
		t.Fatalf("unexpected answer: %+v", ans)
	}
}

func TestRunner_FailureJSON(t *testing.T) {
	fake := buildFakeParinfer(t, `{"text":"","success":false,"error":{"name":"unclosed-quote","message":"bad","lineNo":0,"x":0}}`, 1)
	_, _, err := (Runner{BinaryPath: fake}).RepairIndent("(def s \"oops)")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestRunner_NonJSONOutput(t *testing.T) {
	fake := buildFakeParinfer(t, "not-json", 0)
	_, _, err := (Runner{BinaryPath: fake}).RepairIndent("(+ 1 2)")
	if err == nil {
		t.Fatalf("expected err")
	}
}
