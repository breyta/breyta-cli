package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsDownload_WritesPagesAndSkillBundle(t *testing.T) {
	t.Parallel()

	skillMain := []byte("---\nname: breyta\n---\n")
	skillRef := []byte("# Reference\n")
	shaMain := sha256.Sum256(skillMain)
	shaRef := sha256.Sum256(skillRef)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-quickstart", "title": "Start"},
						{"slug": "build-flow-basics", "title": "Build"},
					},
				},
			})
		case "/api/docs/pages/start-quickstart":
			if r.URL.Query().Get("format") != "markdown" {
				t.Fatalf("expected format=markdown for start page, got %q", r.URL.Query().Get("format"))
			}
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Start\n"))
		case "/api/docs/pages/build-flow-basics":
			if r.URL.Query().Get("format") != "markdown" {
				t.Fatalf("expected format=markdown for build page, got %q", r.URL.Query().Get("format"))
			}
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Build\n"))
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
						{
							"path":        "SKILL.md",
							"sha256":      hex.EncodeToString(shaMain[:]),
							"bytes":       len(skillMain),
							"contentType": "text/markdown",
						},
						{
							"path":        "references/ref.md",
							"sha256":      hex.EncodeToString(shaRef[:]),
							"bytes":       len(skillRef),
							"contentType": "text/markdown",
						},
					},
				},
			})
		case "/api/docs/skills/breyta/files/SKILL.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write(skillMain)
		case "/api/docs/skills/breyta/files/references/ref.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write(skillRef)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "docs-dump")
	app := &App{APIURL: srv.URL, Token: "test-token"}
	cmd := newDocsDownloadCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--out", outDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	for _, p := range []string{
		filepath.Join(outDir, "pages", "start-quickstart.md"),
		filepath.Join(outDir, "pages", "build-flow-basics.md"),
		filepath.Join(outDir, "pages-index.json"),
		filepath.Join(outDir, "skills", "breyta", "manifest.json"),
		filepath.Join(outDir, "skills", "breyta", "files", "SKILL.md"),
		filepath.Join(outDir, "skills", "breyta", "files", "references", "ref.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	if !strings.Contains(out.String(), "Downloaded 2 docs pages") {
		t.Fatalf("expected summary output, got: %s", out.String())
	}
}

func TestDocsDownload_SkipSkillBundle(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-quickstart", "title": "Start"},
					},
				},
			})
		case "/api/docs/pages/start-quickstart":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Start\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "docs-dump")
	app := &App{APIURL: srv.URL}
	cmd := newDocsDownloadCmd(app)
	cmd.SetArgs([]string{"--out", outDir, "--include-skill=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "pages", "start-quickstart.md")); err != nil {
		t.Fatalf("expected page file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "skills")); !os.IsNotExist(err) {
		t.Fatalf("expected no skills directory when --include-skill=false, err=%v", err)
	}
}
