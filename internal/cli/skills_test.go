package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestSkillBundleServer(t *testing.T, skill []byte) *httptest.Server {
	t.Helper()
	return newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "test",
					"minCliVersion": "0.0.0",
					"keyId":         "test",
					"signature":     "",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": "", "bytes": 0, "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write(skill)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestSkillsInstallWarnsOnDuplicateBreytaSkillName(t *testing.T) {
	homeDir := t.TempDir()
	duplicatePath := filepath.Join(homeDir, ".codex", "skills", "legacy-breyta", "SKILL.md")
	duplicateContent := []byte("---\nname: breyta\n---\n# Legacy Breyta skill\n")
	if err := os.MkdirAll(filepath.Dir(duplicatePath), 0o755); err != nil {
		t.Fatalf("mkdir duplicate skill dir: %v", err)
	}
	if err := os.WriteFile(duplicatePath, duplicateContent, 0o644); err != nil {
		t.Fatalf("seed duplicate skill file: %v", err)
	}

	srv := newTestSkillBundleServer(t, []byte("---\nname: breyta\n---\n# Breyta Skill\n"))
	defer srv.Close()

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "install", "--provider", "codex")
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Installed skill in") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
	if !strings.Contains(stderr, "frontmatter name \"breyta\"") ||
		!strings.Contains(stderr, duplicatePath) ||
		!strings.Contains(stderr, "left it untouched") {
		t.Fatalf("expected duplicate skill warning, got stderr: %s", stderr)
	}

	gotContent, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("read duplicate skill file: %v", err)
	}
	if string(gotContent) != string(duplicateContent) {
		t.Fatalf("duplicate skill file was modified:\nwant %q\ngot  %q", string(duplicateContent), string(gotContent))
	}
}
