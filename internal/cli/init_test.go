package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/breyta/breyta-cli/internal/cli"
)

func runInit(t *testing.T, homeDir string, args ...string) (string, string, error) {
	t.Helper()

	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("BREYTA_NO_SKILL_SYNC", "1")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	cmd := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestInit_Default_CreatesWorkspaceAndInstallsSkill(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "breyta-workspace")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = w.Write([]byte("# Breyta Skill\n\nUse `breyta docs`.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--provider", "codex", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	// Workspace layout
	for _, p := range []string{
		filepath.Join(wsDir, "AGENTS.md"),
		filepath.Join(wsDir, "README.md"),
		filepath.Join(wsDir, ".gitignore"),
		filepath.Join(wsDir, "flows"),
		filepath.Join(wsDir, "tmp", "flows"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	agents, err := os.ReadFile(filepath.Join(wsDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agents), "- Breyta skill bundle:") {
		t.Fatalf("unexpected agents content (missing skill bundle line): %s", string(agents))
	}
	if strings.Contains(string(agents), "(Not installed)") {
		t.Fatalf("unexpected agents content (expected installed): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Verify live install target: `breyta flows show <slug> --target live`") {
		t.Fatalf("unexpected agents content (missing live verify show step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Smoke-run live target: `breyta flows run <slug> --target live --wait`") {
		t.Fatalf("unexpected agents content (missing live verify run step): %s", string(agents))
	}
	// Skill install (Codex)
	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	b, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected skill file to exist: %s: %v", skillPath, err)
	}
	if !strings.Contains(string(b), "Breyta Skill") {
		t.Fatalf("unexpected skill file content: %s", string(b))
	}
}

func TestInit_GeminiProvider_InstallsSkill(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = w.Write([]byte("# Breyta Skill\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--no-workspace", "--provider", "gemini", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if _, err := os.Stat(wsDir); err == nil {
		t.Fatalf("expected workspace dir to not be created: %s", wsDir)
	}

	skillPath := filepath.Join(homeDir, ".gemini", "skills", "breyta", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected skill file to exist: %s: %v", skillPath, err)
	}
}

func TestInit_SkillInstallFailure_RendersNotInstalledInAgents(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate docs API failure.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	}))
	defer srv.Close()

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--provider", "codex", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	agents, err := os.ReadFile(filepath.Join(wsDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agents), "(Not installed)") {
		t.Fatalf("expected AGENTS.md to mention skill not installed, got: %s", string(agents))
	}

	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if _, err := os.Stat(skillPath); err == nil {
		t.Fatalf("expected skill file to not exist, but found: %s", skillPath)
	}
}

func TestInit_NoSkill_SkipsSkillInstall(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	_, _, err := runInit(t, homeDir, "init", "--no-skill", "--provider", "codex", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wsDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected workspace AGENTS.md to exist: %v", err)
	}

	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if _, err := os.Stat(skillPath); err == nil {
		t.Fatalf("expected skill file to not exist, but found: %s", skillPath)
	}
}

func TestInit_NoSkill_AllowsUnknownProvider(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	_, _, err := runInit(t, homeDir, "init", "--no-skill", "--provider", "not-a-provider", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wsDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected workspace AGENTS.md to exist: %v", err)
	}

	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if _, err := os.Stat(skillPath); err == nil {
		t.Fatalf("expected skill file to not exist, but found: %s", skillPath)
	}
}

func TestInit_NoWorkspace_SkipsWorkspaceFiles(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = w.Write([]byte("# Breyta Skill\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	_, _, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--no-workspace", "--provider", "codex", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if _, err := os.Stat(wsDir); err == nil {
		t.Fatalf("expected workspace dir to not be created: %s", wsDir)
	}

	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("expected skill file to exist: %s: %v", skillPath, err)
	}
}

func TestInit_NothingToDo_IsError(t *testing.T) {
	homeDir := t.TempDir()

	_, _, err := runInit(t, homeDir, "init", "--no-skill", "--no-workspace")
	if err == nil {
		t.Fatalf("expected error")
	}
}
