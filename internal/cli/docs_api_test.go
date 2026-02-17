package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDocsFind_PrintsTSVWithSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start Here", "source": "flows-api"},
						{"slug": "reference-flow-definition", "title": "Reference: Flow Definition", "source": "flows-api"},
					},
				},
			})
		case "/api/docs/pages/start-here":
			_, _ = w.Write([]byte("# Start Here\n\nRun your first flow end-to-end.\n"))
		case "/api/docs/pages/reference-flow-definition":
			_, _ = w.Write([]byte("# Reference: Flow Definition\n\nCanonical shape for flow definitions.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsFindCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "tsv"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "slug\ttitle\tdescription\n") {
		t.Fatalf("expected tsv header, got: %q", got)
	}
	if !strings.Contains(got, "start-here\tStart Here\tRun your first flow end-to-end.\n") {
		t.Fatalf("expected start page row, got: %q", got)
	}
	if !strings.Contains(got, "reference-flow-definition\tReference: Flow Definition\tCanonical shape for flow definitions.\n") {
		t.Fatalf("expected reference page row, got: %q", got)
	}
}

func TestDocsFind_WithoutSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsFindCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--with-summary=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "start-here\tStart Here\t\n") {
		t.Fatalf("expected page row without summary, got: %q", got)
	}
}

func TestDocsFind_ForwardsSearchOptions(t *testing.T) {
	t.Parallel()

	sawQuery := false
	sawPageFetch := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			sawQuery = true
			q := r.URL.Query()
			if got := q.Get("query"); got != "source:cli release" {
				t.Fatalf("expected query=source:cli release, got %q", got)
			}
			if got := q.Get("q"); got != "source:cli release" {
				t.Fatalf("expected q=source:cli release, got %q", got)
			}
			if got := q.Get("limit"); got != "25" {
				t.Fatalf("expected limit=25, got %q", got)
			}
			if got := q.Get("offset"); got != "10" {
				t.Fatalf("expected offset=10, got %q", got)
			}
			if got := q.Get("with-snippets"); got != "true" {
				t.Fatalf("expected with-snippets=true, got %q", got)
			}
			if got := q.Get("explain"); got != "true" {
				t.Fatalf("expected explain=true, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "reference-cli-commands", "title": "Reference: CLI Commands", "source": "cli"},
					},
				},
			})
		case "/api/docs/pages/reference-cli-commands":
			sawPageFetch = true
			_, _ = w.Write([]byte("# Reference: CLI Commands\n\nCommand catalog.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsFindCmd(&App{APIURL: srv.URL})
	cmd.SetArgs([]string{
		"source:cli release",
		"--limit", "25",
		"--offset", "10",
		"--with-snippets",
		"--explain",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !sawQuery {
		t.Fatalf("expected /api/docs/pages to be requested")
	}
	if !sawPageFetch {
		t.Fatalf("expected page markdown fetch for summary")
	}
}

func TestDocsFind_UsesPerRequestTimeoutForSummaries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-here", "title": "Start Here"},
						{"slug": "reference-flow-definition", "title": "Reference: Flow Definition"},
					},
				},
			})
		case "/api/docs/pages/start-here", "/api/docs/pages/reference-flow-definition":
			if r.URL.Query().Get("format") != "markdown" {
				t.Fatalf("expected format=markdown, got %q", r.URL.Query().Get("format"))
			}
			time.Sleep(700 * time.Millisecond)
			_, _ = w.Write([]byte("# Title\n\nSummary line.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsFindCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--timeout-seconds", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "start-here\tStart Here\tSummary line.\n") {
		t.Fatalf("expected start summary row, got: %q", got)
	}
	if !strings.Contains(got, "reference-flow-definition\tReference: Flow Definition\tSummary line.\n") {
		t.Fatalf("expected reference summary row, got: %q", got)
	}
}

func TestDocsShow_PrintsMarkdown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/start-here" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "markdown" {
			t.Fatalf("expected format=markdown, got %q", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte("# Start\n"))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"start-here"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "# Start\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestDocsShow_PrintsHTML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/start-here" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "html" {
			t.Fatalf("expected format=html, got %q", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte("<h1>Start</h1>"))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"start-here", "--format", "html"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "<h1>Start</h1>\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
