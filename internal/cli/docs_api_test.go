package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDocsFind_PrintsTSVWithSummary(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestDocsFields_PrintsStepConfigOverview(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-http" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step HTTP\n\n## Canonical Shape\n\nCore fields:\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:connection` | keyword/string | Yes* | Slot or connection id |\n| `:response-as` | keyword | No | `:auto`, `:json`, or `:text` |\n| `:persist` | map | No | Persist large responses |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"http"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "field\ttype\trequired\tnotes\n") {
		t.Fatalf("expected tsv header, got: %q", got)
	}
	if !strings.Contains(got, ":connection\tkeyword/string\tYes*\tSlot or connection id\n") ||
		!strings.Contains(got, ":response-as\tkeyword\tNo\t:auto, :json, or :text\n") ||
		!strings.Contains(got, ":persist\tmap\tNo\tPersist large responses\n") {
		t.Fatalf("expected compact field rows, got: %q", got)
	}
}

func TestDocsFields_SelectsMultipleFieldsAsJSON(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-http" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step HTTP\n\n## Canonical Shape\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:connection` | keyword/string | Yes* | Slot or connection id |\n| `:response-as` | keyword | No | Response parser |\n| `:persist` | map | No | Persist response refs |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"http", "response-as", "persist", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	var payload struct {
		Data struct {
			Slug            string   `json:"slug"`
			AvailableFields []string `json:"availableFields"`
			Fields          []struct {
				Field   string   `json:"field"`
				Aliases []string `json:"aliases"`
			} `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if payload.Data.Slug != "reference-step-http" {
		t.Fatalf("unexpected slug: %q", payload.Data.Slug)
	}
	if len(payload.Data.Fields) != 2 {
		t.Fatalf("expected selected fields only, got %+v", payload.Data.Fields)
	}
	if payload.Data.Fields[0].Field != ":response-as" || payload.Data.Fields[1].Field != ":persist" {
		t.Fatalf("unexpected selected fields: %+v", payload.Data.Fields)
	}
	if strings.Join(payload.Data.AvailableFields, ",") != "connection,response-as,persist" {
		t.Fatalf("unexpected available fields: %+v", payload.Data.AvailableFields)
	}
}

func TestDocsFields_MissingFieldListsAvailableFields(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-llm" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step LLM\n\n## Canonical Shape\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:model` | string | No | Model override |\n| `:output` / `:response-format` | map/keyword | No | Structured output config |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{":llm", "missing-field"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing field error")
	}
	got := out.String()
	if !strings.Contains(got, "field(s) not found in reference-step-llm: missing-field") ||
		!strings.Contains(got, "available fields: model, output, response-format") {
		t.Fatalf("expected available field hint, got: %q", got)
	}
}

func TestDocsShow_CompactsLongMarkdownByDefault(t *testing.T) {
	t.Parallel()

	longBody := "# Long Doc\n\n" +
		"Summary line for the long doc.\n\n" +
		"## Setup\n\n" + strings.Repeat("setup detail ", 80) + "\n\n" +
		"## Runtime\n\n" + strings.Repeat("runtime detail ", 80) + "\n\n" +
		"FINAL_SENTINEL_TOKEN\n"
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/long-doc" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"long-doc", "--max-chars", "240"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Compact docs preview") {
		t.Fatalf("expected compact docs hint, got: %q", got)
	}
	if strings.Contains(got, "FINAL_SENTINEL_TOKEN") {
		t.Fatalf("expected default docs show to omit tail sentinel, got: %q", got)
	}

	cmd = newDocsShowCmd(&App{APIURL: srv.URL})
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"long-doc", "--full"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute full: %v", err)
	}
	if !strings.Contains(out.String(), "FINAL_SENTINEL_TOKEN") {
		t.Fatalf("expected --full docs show to include full markdown, got: %q", out.String())
	}
}

func TestDocsShow_SectionNarrowsMarkdown(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/long-doc" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Long Doc\n\n## Setup\n\nSetup details.\n\n## Runtime\n\nRuntime details.\n"))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"long-doc", "--section", "runtime"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "## Runtime") || strings.Contains(got, "Setup details") {
		t.Fatalf("expected focused runtime section, got: %q", got)
	}
}

func TestDocsShow_SectionMissReturnsHeadings(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/long-doc" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Long Doc\n\n## Setup\n\nSetup details.\n\n## Runtime\n\nRuntime details.\n"))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"long-doc", "--section", "resources"})

	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected missing section error")
	}
	got := out.String()
	if !strings.Contains(got, `section "resources" not found in long-doc`) {
		t.Fatalf("expected missing section message, got: %q", got)
	}
	if !strings.Contains(got, "Setup") || !strings.Contains(got, "Runtime") {
		t.Fatalf("expected heading suggestions, got: %q", got)
	}
	if strings.Contains(got, "Setup details") || strings.Contains(got, "Runtime details") {
		t.Fatalf("expected headings without page body, got: %q", got)
	}
}

func TestDocsShow_PrintsHTML(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
