package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func FindParinferRust() string {
	if v := strings.TrimSpace(os.Getenv("BREYTA_PARINFER_RUST")); v != "" {
		if p := resolveExecutablePath(v); p != "" {
			return p
		}
	}

	// Prefer a sibling binary next to `breyta` (bundled in release archives / brew).
	if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
		dir := filepath.Dir(exe)
		if p := resolveExecutablePath(filepath.Join(dir, "parinfer-rust")); p != "" {
			return p
		}
	}

	// Fall back to PATH (useful for local builds where users install via `cargo install parinfer-rust`).
	if p, err := exec.LookPath("parinfer-rust"); err == nil {
		return p
	}

	return ""
}

func resolveExecutablePath(path string) string {
	candidates := []string{path}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		candidates = append([]string{path + ".exe"}, candidates...)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}
