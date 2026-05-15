package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDocsFind_PrintsTSVWithSummaryWhenRequested(t *testing.T) {
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
	cmd.SetArgs([]string{"--format", "tsv", "--with-summary"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "ref\tsource\tmatched\ttitle\tsnippet\tnext\n") {
		t.Fatalf("expected tsv header, got: %q", got)
	}
	if !strings.Contains(got, "docs:start-here\tflows-api\t\tStart Here\tRun your first flow end-to-end.\tbreyta docs show start-here\n") {
		t.Fatalf("expected start page row, got: %q", got)
	}
	if !strings.Contains(got, "docs:reference-flow-definition\tflows-api\t\tReference: Flow Definition\tCanonical shape for flow definitions.\tbreyta docs show reference-flow-definition\n") {
		t.Fatalf("expected reference page row, got: %q", got)
	}
}

func TestDocsFind_DefaultsToRgLikeSnippetRows(t *testing.T) {
	t.Parallel()

	sawPageFetch := false
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/docs/pages":
			if got := r.URL.Query().Get("with-snippets"); got != "true" {
				t.Fatalf("expected default with-snippets=true, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"pages": []map[string]any{
						{
							"slug":          "start-here",
							"title":         "Start Here",
							"source":        "flows-api",
							"snippet":       "Run your first flow end-to-end.",
							"matchedFields": []string{"title", "markdown"},
						},
					},
				},
			})
		case "/api/docs/pages/start-here":
			sawPageFetch = true
			_, _ = w.Write([]byte("# Start Here\n\nRun your first flow end-to-end.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newDocsFindCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"flows run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "docs:start-here\tflows-api\ttitle,markdown\tStart Here\tRun your first flow end-to-end.\tbreyta docs show start-here\n") {
		t.Fatalf("expected rg-like snippet row, got: %q", got)
	}
	if sawPageFetch {
		t.Fatalf("default docs find should not fetch full pages for summaries")
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
	if sawPageFetch {
		t.Fatalf("did not expect page markdown fetch without --with-summary")
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
	cmd.SetArgs([]string{"--timeout-seconds", "1", "--with-summary"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "docs:start-here\t\t\tStart Here\tSummary line.\tbreyta docs show start-here\n") {
		t.Fatalf("expected start summary row, got: %q", got)
	}
	if !strings.Contains(got, "docs:reference-flow-definition\t\t\tReference: Flow Definition\tSummary line.\tbreyta docs show reference-flow-definition\n") {
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
	if !strings.Contains(got, "section\tfield\ttype\trequired\tnotes\n") {
		t.Fatalf("expected tsv header, got: %q", got)
	}
	if !strings.Contains(got, "Canonical Shape\t:connection\tkeyword/string\tYes*\tSlot or connection id\n") ||
		!strings.Contains(got, "Canonical Shape\t:response-as\tkeyword\tNo\t:auto, :json, or :text\n") ||
		!strings.Contains(got, "Canonical Shape\t:persist\tmap\tNo\tPersist large responses\n") {
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
			Slug              string   `json:"slug"`
			AvailableFields   []string `json:"availableFields"`
			AvailableSections []string `json:"availableSections"`
			Fields            []struct {
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
	if strings.Join(payload.Data.AvailableSections, ",") != "Canonical Shape" {
		t.Fatalf("unexpected available sections: %+v", payload.Data.AvailableSections)
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

func TestDocsFields_SectionFilterDisambiguatesRepeatedFields(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-files" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step Files\n\n## Canonical Shape\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:op` | keyword/string | Yes | One of `:read`, `:search` |\n\n## `:read`\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:source` | resource ref or URI | Yes | source-tree-ref or changeset-ref |\n| `:paths` | vector<string> | Conditionally | Read up to `50` paths |\n\n## `:search`\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:source` | resource ref or URI | Yes | source-tree-ref or changeset-ref |\n| `:query` | string | Yes | Search query |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"files", "source", "paths", "--section", "read", "--no-header"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "`:read`\t:source\tresource ref or URI\tYes\tsource-tree-ref or changeset-ref\n") ||
		!strings.Contains(got, "`:read`\t:paths\tvector<string>\tConditionally\tRead up to 50 paths\n") {
		t.Fatalf("expected read section rows, got: %q", got)
	}
	if strings.Contains(got, "`:search`") {
		t.Fatalf("expected section filter to exclude search rows, got: %q", got)
	}
}

func TestDocsFields_ExtractsPerOperationFieldTables(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-table" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step Table\n\n## Canonical Shape\n\nPer-op fields:\n\n| Op | Required fields | Optional fields | Notes |\n| --- | --- | --- | --- |\n| `:query` | `:table`, `:page` | `:select`, `:where`, `:sort` | Paged by default |\n| `:schema` | `:table` | none | Returns columns |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"table", "where", "page", "--section", "query", "--no-header"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Canonical Shape / :query\t:page\t\tYes\tPaged by default\n") ||
		!strings.Contains(got, "Canonical Shape / :query\t:where\t\tNo\tPaged by default\n") {
		t.Fatalf("expected per-op field rows, got: %q", got)
	}
	if strings.Contains(got, ":schema") {
		t.Fatalf("expected section filter to exclude schema rows, got: %q", got)
	}
}

func TestDocsFields_SectionFilterDoesNotUseAmbiguousSubstring(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-job" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step Job\n\n## Canonical Shape\n\n| Op | Required fields | Optional fields | Notes |\n| --- | --- | --- | --- |\n| `:await` | `:job-id` | `:timeout`, `:backoff` | Wait for one job |\n| `:await-batch` | `:batch-id` | `:timeout`, `:backoff` | Wait for one batch |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"job", "timeout", "--section", "await", "--no-header"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Canonical Shape / :await\t:timeout\t\tNo\tWait for one job\n") {
		t.Fatalf("expected await row, got: %q", got)
	}
	if strings.Contains(got, ":await-batch") {
		t.Fatalf("expected exact section match to exclude await-batch, got: %q", got)
	}
}

func TestDocsFields_HandlesEscapedAndCodePipes(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-step-test" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Step Test\n\n## Canonical Shape\n\n| Field | Type | Required | Notes |\n| --- | --- | --- | --- |\n| `:mode` | keyword | No | Use `:a|:b` or escaped A\\|B |\n"))
	}))
	defer srv.Close()

	cmd := newDocsFieldsCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"reference-step-test", "mode", "--no-header"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, "Canonical Shape\t:mode\tkeyword\tNo\tUse :a|:b or escaped A|B\n") {
		t.Fatalf("expected pipe-safe row parsing, got: %q", got)
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

func TestDocsShow_SectionAliasMatchesSingularHeading(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/docs/pages/reference-flow-definition" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# Flow Definition\n\n## Agent Definitions\n\nDefine reusable agents here.\n\n## Interfaces\n\nManual run setup.\n"))
	}))
	defer srv.Close()

	cmd := newDocsShowCmd(&App{APIURL: srv.URL})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"reference-flow-definition", "--section", "Agents"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "## Agent Definitions") || strings.Contains(got, "## Interfaces") {
		t.Fatalf("expected focused agent definitions section, got: %q", got)
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
