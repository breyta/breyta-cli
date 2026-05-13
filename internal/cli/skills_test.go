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

func TestSkillsInstallAllProviders(t *testing.T) {
	homeDir := t.TempDir()
	srv := newTestSkillBundleServer(t, []byte("---\nname: breyta\n---\n# Breyta Skill\n"))
	defer srv.Close()

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "install", "--provider", "all")
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, rel := range []string{
		filepath.Join(".codex", "skills", "breyta", "SKILL.md"),
		filepath.Join(".cursor", "rules", "breyta", "RULE.md"),
		filepath.Join(".claude", "skills", "breyta", "SKILL.md"),
		filepath.Join(".gemini", "skills", "breyta", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(homeDir, rel)); err != nil {
			t.Fatalf("expected installed skill file %s: %v\nstdout=%s\nstderr=%s", rel, err, stdout, stderr)
		}
	}
	if count := strings.Count(stdout, "Installed skill in"); count != 4 {
		t.Fatalf("expected four install messages, got %d:\n%s", count, stdout)
	}
}

func TestSkillsStatusWarnsWhenInstalledSkillIsOutdated(t *testing.T) {
	homeDir := t.TempDir()
	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: breyta\n---\n# Old Breyta Skill\n"), 0o644); err != nil {
		t.Fatalf("seed stale skill: %v", err)
	}

	srv := newTestSkillBundleServer(t, []byte("---\nname: breyta\n---\n# Current Breyta Skill\n"))
	defer srv.Close()

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "status", "--provider", "codex", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning: installed codex Breyta skill is outdated") ||
		!strings.Contains(stderr, "SKILL.md") ||
		!strings.Contains(stderr, "breyta skills install --provider codex") {
		t.Fatalf("expected outdated skill warning with update command, got stderr: %s", stderr)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, stdout)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", envelope["data"])
	}
	providers, ok := data["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected one provider status, got %#v", data["providers"])
	}
	status, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid provider status: %#v", providers[0])
	}
	if got, _ := status["outdated"].(bool); !got {
		t.Fatalf("expected outdated=true, got %#v", status)
	}
	if got, _ := status["updateCommand"].(string); got != "breyta skills install --provider codex" {
		t.Fatalf("unexpected update command: %q", got)
	}
}

func TestSkillsStatusAllReportsMissingProviders(t *testing.T) {
	homeDir := t.TempDir()
	srv := newTestSkillBundleServer(t, []byte("---\nname: breyta\n---\n# Current Breyta Skill\n"))
	defer srv.Close()

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "status", "--provider", "all", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, provider := range []string{"codex", "cursor", "claude", "gemini"} {
		if !strings.Contains(stderr, "warning: "+provider+" Breyta skill is not installed") {
			t.Fatalf("expected missing warning for %s, got stderr: %s", provider, stderr)
		}
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, stdout)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", envelope["data"])
	}
	providers, ok := data["providers"].([]any)
	if !ok || len(providers) != 4 {
		t.Fatalf("expected four provider statuses, got %#v", data["providers"])
	}
}

func TestSkillsStatusReportsInstalledSkillUpToDate(t *testing.T) {
	homeDir := t.TempDir()
	srv := newTestSkillBundleServer(t, []byte("---\nname: breyta\n---\n# Current Breyta Skill\n"))
	defer srv.Close()

	if stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "install", "--provider", "codex"); err != nil {
		t.Fatalf("install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	stdout, stderr, err := runInit(t, homeDir, "--dev", "--api", srv.URL, "skills", "status", "--provider", "codex", "--pretty")
	if err != nil {
		t.Fatalf("expected success, got error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if strings.Contains(stderr, "outdated") {
		t.Fatalf("did not expect outdated warning, got stderr: %s", stderr)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("parse stdout: %v\n%s", err, stdout)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", envelope["data"])
	}
	providers, ok := data["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("expected one provider status, got %#v", data["providers"])
	}
	status, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid provider status: %#v", providers[0])
	}
	if got, _ := status["upToDate"].(bool); !got {
		t.Fatalf("expected upToDate=true, got %#v", status)
	}
}

func TestRootWarnsAutomaticallyWhenNoBreytaSkillIsInstalled(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start Here", "source": "flows-api"},
					},
				},
			})
		case "/api/docs/pages/start-here":
			_, _ = w.Write([]byte("# Start Here\n\nRun your first flow end-to-end.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"--dev", "--api", srv.URL, "docs", "find", "flows", "--limit", "1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("docs find failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "warning: Breyta agent skill is not installed for any supported agent") ||
		!strings.Contains(stderr, "breyta skills install --provider all") ||
		!strings.Contains(stderr, "breyta init --agents-md") {
		t.Fatalf("expected automatic missing skill warning, got stderr: %s", stderr)
	}
	if !strings.Contains(out.String(), "start-here\tStart Here\tRun your first flow end-to-end.") {
		t.Fatalf("expected docs output to still be written, got stdout: %s", out.String())
	}
}

func TestRootWarnsAutomaticallyWhenInstalledSkillIsOutdated(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("BREYTA_NO_UPDATE_CHECK", "1")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	skillPath := filepath.Join(homeDir, ".codex", "skills", "breyta", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: breyta\n---\n# Old Breyta Skill\n"), 0o644); err != nil {
		t.Fatalf("seed stale skill: %v", err)
	}

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/skills/breyta/manifest":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schemaVersion": 1,
					"skillSlug":     "breyta",
					"version":       "test",
					"files": []map[string]any{
						{"path": "SKILL.md", "sha256": "", "bytes": 0, "contentType": "text/markdown"},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			_, _ = w.Write([]byte("---\nname: breyta\n---\n# Current Breyta Skill\n"))
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start Here", "source": "flows-api"},
					},
				},
			})
		case "/api/docs/pages/start-here":
			_, _ = w.Write([]byte("# Start Here\n\nRun your first flow end-to-end.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := cli.NewRootCmd()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs([]string{"--dev", "--api", srv.URL, "--token", "dev-user", "docs", "find", "flows", "--limit", "1"})

	if err := root.Execute(); err != nil {
		t.Fatalf("docs find failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "warning: installed codex Breyta skill is outdated") ||
		!strings.Contains(stderr, "breyta skills install --provider codex") {
		t.Fatalf("expected automatic stale skill warning, got stderr: %s", stderr)
	}
	if !strings.Contains(out.String(), "start-here\tStart Here\tRun your first flow end-to-end.") {
		t.Fatalf("expected docs output to still be written, got stdout: %s", out.String())
	}
}
