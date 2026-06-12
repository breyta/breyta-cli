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
	if !strings.Contains(string(agents), "`breyta skills install --provider all`") {
		t.Fatalf("unexpected agents content (missing all-provider skill install guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "CLI warns that the skill is missing or stale") {
		t.Fatalf("unexpected agents content (missing missing-skill drift guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Read `SKILL.md` first; load only the `playbooks/` or `references/` file named for the touched surface.") {
		t.Fatalf("unexpected agents content (missing persistent playbook/reference guidance): %s", string(agents))
	}
	if strings.Contains(string(agents), "(Not installed)") {
		t.Fatalf("unexpected agents content (expected installed): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Start with `README.md` in this folder") {
		t.Fatalf("unexpected agents content (missing README pointer): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Pick task mode: existing-flow edit, new flow, primitive/step edit") {
		t.Fatalf("unexpected agents content (missing task mode router guidance): %s", string(agents))
	}
	for _, want := range []string{
		"flow files are a Breyta DSL with Clojure/EDN syntax, not normal Clojure programs",
		"`breyta flows search \"<query>\" --limit 5`",
		"`breyta flows grep \"<literal>\" --limit 5`",
		"`breyta flows templates search \"<query>\" --limit 5`",
		"`breyta resources search \"<query>\" --limit 5`",
		"Build in small slices: contract -> manual interface -> one boundary -> lint -> push -> configure-check -> run -> inspect output.",
		"Persist large or unknown payloads with `:persist`",
		"`:tier :ephemeral` on streaming `:http` steps",
		"Keep functions map-oriented",
		"Do not run `breyta connections test --all`",
		"After two failed edit/run cycles, stop and re-plan.",
	} {
		if !strings.Contains(string(agents), want) {
			t.Fatalf("unexpected agents content (missing %q): %s", want, string(agents))
		}
	}
	if !strings.Contains(string(agents), "Draft/live and release") {
		t.Fatalf("unexpected agents content (missing draft/live section): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`draft` is staging/current authoring; `live` is released/runtime.") {
		t.Fatalf("unexpected agents content (missing draft/live distinction): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Say `draft verified` when only draft was exercised.") {
		t.Fatalf("unexpected agents content (missing draft verified wording): %s", string(agents))
	}
	if !strings.Contains(string(agents), "`breyta flows run-step <slug> <step-id> --target live --input '{...}' --wait`") {
		t.Fatalf("unexpected agents content (missing focused run-step proof guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "verify live/install-shaped behavior or report `web UI not verified`") {
		t.Fatalf("unexpected agents content (missing web UI risk wording): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Include workspace/template queries run, chosen/rejected snippets or templates") {
		t.Fatalf("unexpected agents content (missing discovery proof guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Include full Breyta URLs from CLI JSON") {
		t.Fatalf("unexpected agents content (missing URL proof guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Prefer recovery URLs from failures: `error.actions[].url`, then `meta.webUrl`.") {
		t.Fatalf("unexpected agents content (missing recovery URL priority guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Submit `breyta feedback send --agent` when flow development hits significant authoring friction") {
		t.Fatalf("unexpected agents content (missing authoring friction feedback guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "source flow, live version, activation/setup, Discover install, marketplace visibility") {
		t.Fatalf("unexpected agents content (missing public surface separation): %s", string(agents))
	}
	if strings.Contains(string(agents), "## Reliability checklist (required)") ||
		strings.Contains(string(agents), "## Scale-aware defaults") ||
		strings.Contains(string(agents), "## Provenance for derived flows") {
		t.Fatalf("unexpected agents content (AGENTS.md should stay compact): %s", string(agents))
	}
	if !strings.Contains(string(agents), "For n8n workflow JSON imports, use `breyta flows import n8n <workflow.json>` first") {
		t.Fatalf("unexpected agents content (missing n8n import CLI guidance): %s", string(agents))
	}
	if strings.Contains(string(agents), "## Stop gate") {
		t.Fatalf("unexpected agents content (AGENTS.md should stay evergreen): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Do not run `breyta connections test --all`") {
		t.Fatalf("unexpected agents content (missing targeted connection-test guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "For paid apps, author pricing in source under `:marketplace {:app ... :monetization {:plans [...]}}`") {
		t.Fatalf("unexpected agents content (missing paid app source-authored pricing guidance): %s", string(agents))
	}
	if !strings.Contains(string(agents), "Seat-based pricing is not implemented") {
		t.Fatalf("unexpected agents content (missing paid app seat-pricing guardrail): %s", string(agents))
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
	if !strings.Contains(string(readme), "`breyta docs find \"<idea or primitive>\" --limit 5 --format json`") {
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
	if !strings.Contains(string(readme), "`breyta resources read <uri>` defaults to compact blob previews and bounded table row/cell previews") {
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
	if !strings.Contains(string(readme), "`breyta flows run-step <slug> <step-id> --target live --input '{...}' --wait`") {
		t.Fatalf("unexpected readme content (missing focused run-step proof guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows run <slug> --input-file ./input.json`") ||
		!strings.Contains(string(readme), "shell or OS argument limits") {
		t.Fatalf("unexpected readme content (missing input-file payload guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`breyta flows lint --file ./flows/<slug>.clj`") {
		t.Fatalf("unexpected readme content (missing flow lint guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "`--timeout <duration>` when server lint needs a longer bound") {
		t.Fatalf("unexpected readme content (missing lint timeout guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Do not call a public/end-user flow \"ready for UI\" from draft CLI proof alone") {
		t.Fatalf("unexpected readme content (missing ready-for-UI guardrail): %s", string(readme))
	}
	if !strings.Contains(string(readme), "For installable/public flows, do not stop at activation") {
		t.Fatalf("unexpected readme content (missing activation-vs-install guardrail): %s", string(readme))
	}
	if !strings.Contains(string(readme), "verify Discover install plus an installed run") {
		t.Fatalf("unexpected readme content (missing Discover installed-run proof): %s", string(readme))
	}
	if !strings.Contains(string(readme), "breyta flows run <slug> --buyer-test --installation-id <installation-id> --wait") {
		t.Fatalf("unexpected readme content (missing Buyer Test installation-run proof): %s", string(readme))
	}
	if !strings.Contains(string(readme), "OpenAI connection default: `:http-api` requirement, backend `openai`, base URL `https://api.openai.com/v1`") {
		t.Fatalf("unexpected readme content (missing OpenAI connection default): %s", string(readme))
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
	if !strings.Contains(string(readme), "set curated media with `breyta flows update <slug> --publish-media-type image --publish-media-source-file ./screenshot.png`") {
		t.Fatalf("unexpected readme content (missing discover card media workflow): %s", string(readme))
	}
	if !strings.Contains(string(readme), "HTTPS media sources must be publicly reachable safe media URLs") {
		t.Fatalf("unexpected readme content (missing HTTPS discover media constraints): %s", string(readme))
	}
	if !strings.Contains(string(readme), "For paid apps, author pricing in source under `:marketplace {:app ... :monetization {:plans [...]}}`") {
		t.Fatalf("unexpected readme content (missing paid app source-authored pricing guidance): %s", string(readme))
	}
	if !strings.Contains(string(readme), "Seat-based pricing is not implemented; do not describe a paid app plan as N seats or N installs") {
		t.Fatalf("unexpected readme content (missing paid app seat-pricing restriction): %s", string(readme))
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
	if !strings.Contains(stdout, "Search docs: breyta docs find \"<idea or primitive>\" --limit 5 --format json") {
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
