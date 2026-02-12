package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsIndex_PrintsTSVWithSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-quickstart", "title": "Start: Quickstart", "source": "flows-api"},
						{"slug": "reference-flow-definition", "title": "Reference: Flow Definition", "source": "flows-api"},
					},
				},
			})
		case "/api/docs/pages/start-quickstart":
			_, _ = w.Write([]byte("# Start: Quickstart\n\nRun your first flow end-to-end.\n"))
		case "/api/docs/pages/reference-flow-definition":
			_, _ = w.Write([]byte("# Reference: Flow Definition\n\nCanonical shape for flow definitions.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsIndexCmd(&App{APIURL: srv.URL})
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
	if !strings.Contains(got, "start-quickstart\tStart: Quickstart\tRun your first flow end-to-end.\n") {
		t.Fatalf("expected start page row, got: %q", got)
	}
	if !strings.Contains(got, "reference-flow-definition\tReference: Flow Definition\tCanonical shape for flow definitions.\n") {
		t.Fatalf("expected reference page row, got: %q", got)
	}
}

func TestDocsIndex_WithoutSummary(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{"slug": "start-quickstart", "title": "Start: Quickstart", "source": "flows-api"},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsIndexCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--with-summary=false"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "start-quickstart\tStart: Quickstart\t\n") {
		t.Fatalf("expected page row without summary, got: %q", got)
	}
}

func TestDocsPage_PrintsMarkdown(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/start-quickstart" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "markdown" {
			t.Fatalf("expected format=markdown, got %q", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte("# Start\n"))
	}))
	defer srv.Close()

	cmd := newDocsPageCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"start-quickstart"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "# Start\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestDocsPage_PrintsHTML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/start-quickstart" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("format") != "html" {
			t.Fatalf("expected format=html, got %q", r.URL.Query().Get("format"))
		}
		_, _ = w.Write([]byte("<h1>Start</h1>"))
	}))
	defer srv.Close()

	cmd := newDocsPageCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"start-quickstart", "--format", "html"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "<h1>Start</h1>\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
