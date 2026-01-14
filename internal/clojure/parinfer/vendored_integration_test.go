package parinfer

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunner_WithVendoredParinferRustBinary(t *testing.T) {
	if os.Getenv("BREYTA_TEST_VENDORED_PARINFER") != "1" {
		t.Skip("set BREYTA_TEST_VENDORED_PARINFER=1 to run vendored parinfer integration test")
	}

	root, ok := findRepoRoot(t)
	if !ok {
		t.Skip("repo root not found (no go.mod)")
	}

	exe := "parinfer-rust"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	path := filepath.Join(root, "tools", "parinfer-rust", runtime.GOOS, runtime.GOARCH, exe)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("vendored parinfer-rust not present at %s: %v", path, err)
	}

	r := Runner{BinaryPath: path}
	// A simple, stable mismatch-close input that parinfer can fix without requiring change metadata.
	out, ans, err := r.RepairIndent("(let [x 1]\n  x]\n")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ans.Success {
		t.Fatalf("expected success answer: %+v", ans)
	}
	if out == "" {
		t.Fatalf("expected non-empty output")
	}
	if out == "(let [x 1]\n  x]\n" {
		t.Fatalf("expected output to change")
	}
}

func findRepoRoot(t *testing.T) (string, bool) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
