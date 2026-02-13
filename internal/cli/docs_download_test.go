package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocsSync_WritesPages(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start"},
						{"slug": "build-flow-basics", "title": "Build"},
					},
				},
			})
		case "/api/docs/pages/start-here":
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
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "docs-dump")
	app := &App{APIURL: srv.URL, Token: "test-token"}
	cmd := newDocsSyncCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--out", outDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	for _, p := range []string{
		filepath.Join(outDir, "pages", "start-here.md"),
		filepath.Join(outDir, "pages", "build-flow-basics.md"),
		filepath.Join(outDir, "pages-index.json"),
		filepath.Join(outDir, ".breyta-docs-marker"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}

	if _, err := os.Stat(filepath.Join(outDir, "skills")); !os.IsNotExist(err) {
		t.Fatalf("expected no skills directory, err=%v", err)
	}

	if !strings.Contains(out.String(), "Downloaded 2 docs pages") {
		t.Fatalf("expected summary output, got: %s", out.String())
	}
}

func TestDocsSync_CleanRemovesExistingFiles(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{{"slug": "start-here", "title": "Start"}},
				},
			})
		case "/api/docs/pages/start-here":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Start\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "docs-dump")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := filepath.Join(outDir, "stale.txt")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	app := &App{APIURL: srv.URL}
	cmd := newDocsSyncCmd(app)
	cmd.SetArgs([]string{"--out", outDir, "--clean"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed by --clean, err=%v", err)
	}
}

func TestDocsSync_CleanRejectsDangerousPath(t *testing.T) {
	t.Parallel()

	app := &App{APIURL: "http://example.test"}
	cmd := newDocsSyncCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--out", ".", "--clean"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for dangerous clean path")
	}
	if !strings.Contains(err.Error(), "refusing to clean dangerous output path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDocsSync_UsesPerRequestTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start"},
						{"slug": "build-flow-basics", "title": "Build"},
					},
				},
			})
		case "/api/docs/pages/start-here", "/api/docs/pages/build-flow-basics":
			if r.URL.Query().Get("format") != "markdown" {
				t.Fatalf("expected format=markdown, got %q", r.URL.Query().Get("format"))
			}
			time.Sleep(700 * time.Millisecond)
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# Page\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := filepath.Join(t.TempDir(), "docs-dump")
	app := &App{APIURL: srv.URL}
	cmd := newDocsSyncCmd(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--out", outDir, "--timeout-seconds", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(outDir, "pages", "start-here.md")); err != nil {
		t.Fatalf("expected start page output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "pages", "build-flow-basics.md")); err != nil {
		t.Fatalf("expected build page output: %v", err)
	}
}
