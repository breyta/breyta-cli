package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
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

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						{"path": "references/authoring-loop.md", "sha256": "", "bytes": 0, "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write([]byte("# Breyta Skill\n\nUse `breyta docs`.\n"))
		case "/api/docs/skills/breyta/files/references/authoring-loop.md":
			_, _ = w.Write([]byte("# Authoring Loop\n"))
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
	if !strings.Contains(string(agents), "Before meaningful Breyta flow work, state the loaded Breyta skill path and the bundled reference files read for the task.") {
		t.Fatalf("unexpected agents content (missing skill path proof requirement): %s", string(agents))
	}
	if !strings.Contains(string(agents), "The bundle may include `SKILL.md` plus `references/`. Read `SKILL.md` first") {
		t.Fatalf("unexpected agents content (missing skill references guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "load the relevant bundled `references/` file named by `SKILL.md` before creating or editing flows") {
		t.Fatalf("unexpected agents content (missing persistent references guidance): %s", string(agents))
	}
	if strings.Contains(string(agents), "(Not installed)") {
		t.Fatalf("unexpected agents content (expected installed): %s", string(agents))
	}
	if !strings.Contains(string(agents), "start with `README.md` in this folder") {
		t.Fatalf("unexpected agents content (missing README pointer): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Pick a task mode before running commands: existing-flow edit, new flow, primitive/step edit") {
		t.Fatalf("unexpected agents content (missing task mode router guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Start new work by inspecting the smallest current state needed, then use workspace search/grep, docs, and approved templates at the primitive level") {
		t.Fatalf("unexpected agents content (missing ordered discovery guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Primitive-first reuse (required for create/edit)") {
		t.Fatalf("unexpected agents content (missing primitive-first reuse section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "New flow sequence: `breyta flows search \"<integration or problem query>\" --limit 5`") {
		t.Fatalf("unexpected agents content (missing new-flow example sequence): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Existing flow sequence: `breyta flows show <slug>` or `breyta flows pull <slug>` -> workspace search/grep only for nearby patterns -> docs search snippets") {
		t.Fatalf("unexpected agents content (missing edit-flow primitive-first sequence): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Do not use `breyta flows list` for pattern discovery") {
		t.Fatalf("unexpected agents content (missing workspace search list guard): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Do not pull a full template for a primitive/step edit unless snippet context") {
		t.Fatalf("unexpected agents content (missing full-template escalation guard): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Command budget") {
		t.Fatalf("unexpected agents content (missing command budget): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Do not run `breyta connections test --all` by default") {
		t.Fatalf("unexpected agents content (missing targeted connection-test guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Final handoff must include workspace/template queries run, chosen/rejected snippets or templates") {
		t.Fatalf("unexpected agents content (missing example reporting requirement): %s", string(agents))
	}
	if !strings.Contains(string(agents), "check against current official provider docs/API references or model-list endpoints") {
		t.Fatalf("unexpected agents content (missing provider/API freshness guidance): %s", string(agents))
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
	if !strings.Contains(string(agents), "Do not tell the user a public/end-user flow is \"ready for UI\" from draft proof alone") {
		t.Fatalf("unexpected agents content (missing ready-for-UI guardrail): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`web UI not verified` in the risk ledger") {
		t.Fatalf("unexpected agents content (missing web UI risk ledger wording): %s", string(agents))
	}
	if !strings.Contains(string(agents), "## Authoring standard (required before editing)") {
		t.Fatalf("unexpected agents content (missing authoring standard section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "For public/end-user flows, classify every user value before adding fields:") {
		t.Fatalf("unexpected agents content (missing setup/run field planning): %s", string(agents))
	}
	if !strings.Contains(string(agents), "run-each-time values like prompt, file, CSV, resource picker selection") {
		t.Fatalf("unexpected agents content (missing run field examples): %s", string(agents))
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
	if !strings.Contains(string(agents), "Search workspace patterns and docs snippets, inspect private snippets with `breyta flows workspace examples step <type> \"<query>\"`") {
		t.Fatalf("unexpected agents content (missing authoring loop primitive-first step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "If configure check reports missing required config, stop before draft/live runs unless the task is static validation only") {
		t.Fatalf("unexpected agents content (missing configure-check run gate): %s", string(agents))
	}
	if !strings.Contains(string(agents), "set explicit order: `breyta flows update <slug> --group-order <n>`") {
		t.Fatalf("unexpected agents content (missing group ordering guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "confirm ordered siblings with `breyta flows show <slug>`") {
		t.Fatalf("unexpected agents content (missing ordered siblings verification guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "set curated media with `breyta flows update <slug> --publish-media-type image --publish-media-source-kind https-url --publish-media-source https://...`") {
		t.Fatalf("unexpected agents content (missing discover card media guidance): %s", string(agents))
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
	if !strings.Contains(string(agents), "inspect installation setup/config and run with `breyta flows run <slug> --installation-id <installation-id> --wait`") {
		t.Fatalf("unexpected agents content (missing installation-targeted proof step): %s", string(agents))
	}
	if !strings.Contains(string(agents), "test the actual setup page, run form fields, upload CSV or file flow, resource picker, and output page") {
		t.Fatalf("unexpected agents content (missing UI verification ladder): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Prefer exact recovery URLs from failures: `error.actions[].url` first, then `meta.webUrl`.") {
		t.Fatalf("unexpected agents content (missing recovery URL priority guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "include the exact recovery URL in runtime proof instead of generic \"go to billing/setup\" text") {
		t.Fatalf("unexpected agents content (missing runtime proof recovery guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Skill references: read `SKILL.md` first, then load the bundled `references/` file named for the task surface before creating or editing flows.") {
		t.Fatalf("unexpected agents content (missing docs skill reference guidance): %s", string(agents))
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
	if !strings.Contains(string(readme), "`breyta flows search \"<integration or problem query>\" --limit 5`") {
		t.Fatalf("unexpected readme content (missing workspace flow search step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5`") {
		t.Fatalf("unexpected readme content (missing workspace source grep step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows workspace examples step <type> \"<query>\" --limit 3`") {
		t.Fatalf("unexpected readme content (missing workspace snippets step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta docs find \"<idea or primitive>\"`") {
		t.Fatalf("unexpected readme content (missing docs search step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows templates search \"<problem or integration query>\" --limit 5`") {
		t.Fatalf("unexpected readme content (missing query-shaped flows search step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "use primitive snippets and referenced dependencies before a full template") {
		t.Fatalf("unexpected readme content (missing primitive-first reuse step): %s", string(readme))
	}
	if !strings.Contains(string(readme), "inspect one full template only when architecture-level reuse is needed") {
		t.Fatalf("unexpected readme content (missing full-template escalation guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "compare the touched surface against the closest local or approved template example before changing structure") {
		t.Fatalf("unexpected readme content (missing example comparison guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Authoring reads are compact by default. Use `--full` on `flows show`, `flows diff`, or `runs show`") {
		t.Fatalf("unexpected readme content (missing compact authoring default guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta resources read <uri>` defaults to bounded table row and cell previews") {
		t.Fatalf("unexpected readme content (missing bounded resource-read guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Treat `--pretty` as formatting only") {
		t.Fatalf("unexpected readme content (missing pretty formatting-only guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "persist the full body as a resource") {
		t.Fatalf("unexpected readme content (missing large artifact resource guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Treat failed configure checks as a hard stop before draft/live runs unless the task is static validation only") {
		t.Fatalf("unexpected readme content (missing configure-check run gate): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Do not call a public/end-user flow \"ready for UI\" from draft CLI proof alone") {
		t.Fatalf("unexpected readme content (missing ready-for-UI guardrail): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`web UI not verified` in the risk ledger") {
		t.Fatalf("unexpected readme content (missing web UI risk ledger guidance): %s", string(readme))
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
	if !strings.Contains(string(readme), "External provider/API truth: check current official provider docs/API references or model-list endpoints") {
		t.Fatalf("unexpected readme content (missing provider/API freshness guidance): %s", string(readme))
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
	if !strings.Contains(string(readme), "set explicit order with `breyta flows update <slug> --group-order <n>` and verify ordered siblings with `breyta flows show <slug>`") {
		t.Fatalf("unexpected readme content (missing group ordering workflow): %s", string(readme))
	}
	if !strings.Contains(string(readme), "set curated media with `breyta flows update <slug> --publish-media-type image --publish-media-source-kind https-url --publish-media-source https://...`") {
		t.Fatalf("unexpected readme content (missing discover card media workflow): %s", string(readme))
	}
	if !strings.Contains(stdout, "Verify identity + workspace summary: breyta auth whoami") {
		t.Fatalf("unexpected init stdout (missing whoami next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Search nearby workspace flow patterns: breyta flows search \"<integration or problem query>\" --limit 5") {
		t.Fatalf("unexpected init stdout (missing workspace search next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Search workspace source/config literals: breyta flows grep \"<literal>\" --or \"<variant>\" --limit 5") {
		t.Fatalf("unexpected init stdout (missing workspace grep next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Search docs: breyta docs find \"<idea or primitive>\"") {
		t.Fatalf("unexpected init stdout (missing docs search next step): %s", stdout)
	}
	if !strings.Contains(stdout, "Search approved templates: breyta flows templates search \"<problem or integration query>\" --limit 5") {
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
	refPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "references", "authoring-loop.md")
	ref, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("expected skill reference file to exist: %s: %v", refPath, err)
	}
	if !strings.Contains(string(ref), "Authoring Loop") {
		t.Fatalf("unexpected reference file content: %s", string(ref))
	}
}

func TestInit_GeminiProvider_InstallsSkill(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestInitWarnsOnDuplicateBreytaSkillName(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")
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

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "init", "--provider", "codex", "--dir", wsDir)
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
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

func TestInit_SkillInstallFailure_RendersNotInstalledInAgents(t *testing.T) {
	homeDir := t.TempDir()
	wsDir := filepath.Join(t.TempDir(), "ws")

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
