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

	stdout, _, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--provider", "codex", "--dir", wsDir)
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
	if !strings.Contains(string(agents), "start with `README.md` in this folder") {
		t.Fatalf("unexpected agents content (missing README pointer): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Start new work with approved template discovery: `breyta flows search <query>`") {
		t.Fatalf("unexpected agents content (missing search-first guidance): %s", string(agents))
	}
	if strings.Contains(string(agents), "## Stop gate") {
		t.Fatalf("unexpected agents content (AGENTS.md should stay evergreen): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Verify live install target: `breyta flows show <slug> --target live`") {
		t.Fatalf("unexpected agents content (missing live verify show step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Smoke-run live target and capture proof: `breyta flows run <slug> --target live --wait`") {
		t.Fatalf("unexpected agents content (missing live verify run step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Release hygiene (required)") {
		t.Fatalf("unexpected agents content (missing release hygiene section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Authoring standard (required before editing)") {
		t.Fatalf("unexpected agents content (missing authoring standard section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Reliability checklist (required)") {
		t.Fatalf("unexpected agents content (missing reliability checklist): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Scale-aware defaults") {
		t.Fatalf("unexpected agents content (missing scale-aware defaults): %s", string(agents))
	}
	if !strings.Contains(string(agents), "idempotency key or dedupe strategy") {
		t.Fatalf("unexpected agents content (missing duplicate protection guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Choose concurrency mode intentionally before draft runs") {
		t.Fatalf("unexpected agents content (missing concurrency planning guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`sequential` for ordered work, shared state, large artifacts, or fragile APIs") {
		t.Fatalf("unexpected agents content (missing sequential mode guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`fanout` only for independent bounded items") {
		t.Fatalf("unexpected agents content (missing fanout guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`keyed` when work must serialize per entity") {
		t.Fatalf("unexpected agents content (missing keyed guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "pass signed URLs/blob refs for large files") {
		t.Fatalf("unexpected agents content (missing large file reference guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Cursors/checkpoints do not advance past failed work.") {
		t.Fatalf("unexpected agents content (missing cursor safety guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "The chosen concurrency mode is justified in plain language.") {
		t.Fatalf("unexpected agents content (missing concurrency justification guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Shared state and side effects that must not overlap are named explicitly.") {
		t.Fatalf("unexpected agents content (missing overlap guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Prefer sequential mode when uncertain; concurrency is opt-in, not the default.") {
		t.Fatalf("unexpected agents content (missing sequential default guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Run draft target and wait for output: `breyta flows run <slug> --input '{\"n\":41}' --wait`") {
		t.Fatalf("unexpected agents content (missing draft run step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "set explicit order: `breyta flows update <slug> --group-order <n>`") {
		t.Fatalf("unexpected agents content (missing group ordering guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "confirm ordered siblings with `breyta flows show <slug> --pretty`") {
		t.Fatalf("unexpected agents content (missing ordered siblings verification guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Run at least one failure/no-op/replay check when feasible before release") {
		t.Fatalf("unexpected agents content (missing failure/no-op/replay step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "If using concurrency, verify no skipped, duplicated, or overlapped work in draft output") {
		t.Fatalf("unexpected agents content (missing concurrency verification step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Provenance for derived flows") {
		t.Fatalf("unexpected agents content (missing provenance section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "breyta flows provenance set <slug> --from-consulted") {
		t.Fatalf("unexpected agents content (missing provenance set command): %s", string(agents))
	}
	if !strings.Contains(string(agents), "breyta flows provenance set <slug> --template <template-slug>") {
		t.Fatalf("unexpected agents content (missing template provenance command): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Only flows actually opened with `breyta flows show` or `breyta flows pull` become consulted provenance candidates") {
		t.Fatalf("unexpected agents content (missing consulted-flow guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Inspect draft vs live before release: `breyta flows diff <slug>`") {
		t.Fatalf("unexpected agents content (missing diff-before-release step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Release once (after explicit sign-off) with a markdown note: `breyta flows release <slug> --release-note-file ./release-note.md`") {
		t.Fatalf("unexpected agents content (missing sign-off-gated release step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Use `breyta flows archive <slug>` when the flow should stop appearing in the normal active surface") {
		t.Fatalf("unexpected agents content (missing archive guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Use `breyta flows delete <slug> --yes` only for permanent removal; add `--force` when runs/installations must also be cleaned up.") {
		t.Fatalf("unexpected agents content (missing delete guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Edit the note later if needed: `breyta flows versions update <slug> --version <n> --release-note-file ./release-note.md`") {
		t.Fatalf("unexpected agents content (missing version note update step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Smoke-run live target and capture proof: `breyta flows run <slug> --target live --wait`") {
		t.Fatalf("unexpected agents content (missing live proof step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Prefer exact recovery URLs from failures: `error.actions[].url` first, then `meta.webUrl`.") {
		t.Fatalf("unexpected agents content (missing recovery URL priority guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "include the exact recovery URL in runtime proof instead of generic \"go to billing/setup\" text") {
		t.Fatalf("unexpected agents content (missing runtime proof recovery guidance): %s", string(agents))
	}
	readme, err := os.ReadFile(filepath.Join(wsDir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	if !strings.Contains(string(readme), "## Recommended first session") {
		t.Fatalf("unexpected readme content (missing first-session section): %s", string(readme))
	}
	if strings.Contains(string(readme), "`breyta-workspace/`") {
		t.Fatalf("unexpected readme content (should not hardcode workspace directory name): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta auth whoami`") {
		t.Fatalf("unexpected readme content (missing whoami step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows search \"<idea>\"`") {
		t.Fatalf("unexpected readme content (missing flows search step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "## Stop gate") {
		t.Fatalf("unexpected readme content (missing stop gate): %s", string(readme))
	}
	if !strings.Contains(string(readme), "https://flows.breyta.ai/signup") {
		t.Fatalf("unexpected readme content (missing signup fallback): %s", string(readme))
	}
	if !strings.Contains(string(readme), "When a command fails, prefer the exact page from `error.actions[].url` first, then `meta.webUrl`.") {
		t.Fatalf("unexpected readme content (missing recovery URL guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Release once to live after draft is verified and approved, using `breyta flows release <slug> --release-note-file ./release-note.md`") {
		t.Fatalf("unexpected readme content (missing release-note workflow): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Archive flows you want to retire without removing their history: `breyta flows archive <slug>`") {
		t.Fatalf("unexpected readme content (missing archive workflow): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Delete flows only for permanent cleanup: `breyta flows delete <slug> --yes`") {
		t.Fatalf("unexpected readme content (missing delete workflow): %s", string(readme))
	}
	if !strings.Contains(string(readme), "set explicit order with `breyta flows update <slug> --group-order <n>` and verify ordered siblings with `breyta flows show <slug> --pretty`") {
		t.Fatalf("unexpected readme content (missing group ordering workflow): %s", string(readme))
	}
	if !strings.Contains(stdout, "Verify identity + workspace summary: breyta auth whoami") {
		t.Fatalf("unexpected init stdout (missing whoami next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Discover approved templates: breyta flows search \"<idea>\"") {
		t.Fatalf("unexpected init stdout (missing flows search next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Stop after idea exploration unless you intentionally want to continue now") {
		t.Fatalf("unexpected init stdout (missing stop gate): %s", stdout)
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
