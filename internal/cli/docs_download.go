package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/spf13/cobra"
)

type docsPageMeta struct {
	Slug          string   `json:"slug"`
	Title         string   `json:"title,omitempty"`
	Source        string   `json:"source,omitempty"`
	Category      string   `json:"category,omitempty"`
	Order         int      `json:"order,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Score         float64  `json:"score,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
	MatchedFields []string `json:"matchedFields,omitempty"`
	Explain       string   `json:"explain,omitempty"`
}

type docsPagesQueryOptions struct {
	Source       string
	Query        string
	WithSnippets bool
	Explain      bool
	Limit        int
	Offset       int
}

type docsPagesQueryResult struct {
	Pages []docsPageMeta
	Total int
	Limit int
}

func newDocsSyncCmd(app *App) *cobra.Command {
	var outDir string
	var clean bool
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download API docs to a local directory for offline grep/search",
		RunE: func(cmd *cobra.Command, args []string) error {
			ensureAPIURL(app)
			if strings.TrimSpace(app.APIURL) == "" {
				return writeErr(cmd, errors.New("missing api base url"))
			}

			rootOut := strings.TrimSpace(outDir)
			if rootOut == "" {
				return writeErr(cmd, errors.New("missing output directory"))
			}
			rootOut = filepath.Clean(rootOut)

			if clean {
				if err := validateDocsSyncCleanTarget(rootOut); err != nil {
					return writeErr(cmd, err)
				}
				if err := os.RemoveAll(rootOut); err != nil {
					return writeErr(cmd, fmt.Errorf("clean output dir: %w", err))
				}
			}
			if err := os.MkdirAll(rootOut, 0o755); err != nil {
				return writeErr(cmd, fmt.Errorf("create output dir: %w", err))
			}

			timeout := time.Duration(timeoutSeconds) * time.Second
			if timeout <= 0 {
				timeout = 90 * time.Second
			}
			client := api.Client{
				BaseURL: app.APIURL,
				Token:   app.Token,
			}

			pages, err := fetchAllDocsPages(client, docsPagesQueryOptions{Limit: 100}, timeout)
			if err != nil {
				return writeErr(cmd, err)
			}
			if len(pages) == 0 {
				return writeErr(cmd, errors.New("no docs pages returned by API"))
			}

			if err := writeDocsPages(client, rootOut, pages, timeout); err != nil {
				return writeErr(cmd, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %d docs pages to %s\n", len(pages), filepath.Join(rootOut, "pages"))
			fmt.Fprintf(cmd.OutOrStdout(), "Ready for grep: rg -n \"<query>\" %s\n", rootOut)
			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", ".breyta-docs", "Output directory for downloaded docs")
	cmd.Flags().BoolVar(&clean, "clean", false, "Delete output directory before download")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 90, "Request timeout in seconds")
	return cmd
}

func fetchDocsPages(ctx context.Context, client api.Client, opts docsPagesQueryOptions) (docsPagesQueryResult, error) {
	query := url.Values{}
	if strings.TrimSpace(opts.Source) != "" {
		query.Set("source", strings.TrimSpace(opts.Source))
	}
	if strings.TrimSpace(opts.Query) != "" {
		q := strings.TrimSpace(opts.Query)
		// Send both params for compatibility across server versions.
		query.Set("query", q)
		query.Set("q", q)
	}
	if opts.WithSnippets {
		query.Set("with-snippets", "true")
	}
	if opts.Explain {
		query.Set("explain", "true")
	}
	if opts.Limit >= 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", opts.Offset))
	}

	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/docs/pages", query, nil)
	if err != nil {
		return docsPagesQueryResult{}, fmt.Errorf("fetch docs pages: %w", err)
	}
	if status < 200 || status > 299 {
		return docsPagesQueryResult{}, fmt.Errorf("fetch docs pages failed (status=%d)", status)
	}

	var payload struct {
		Data struct {
			Pages []docsPageMeta `json:"pages"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
			Limit int `json:"limit"`
		} `json:"meta"`
	}
	if err := decodeLooseJSON(out, &payload); err != nil {
		return docsPagesQueryResult{}, fmt.Errorf("decode docs pages response: %w", err)
	}
	return docsPagesQueryResult{
		Pages: payload.Data.Pages,
		Total: payload.Meta.Total,
		Limit: payload.Meta.Limit,
	}, nil
}

func fetchAllDocsPages(client api.Client, opts docsPagesQueryOptions, timeout time.Duration) ([]docsPageMeta, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	all := make([]docsPageMeta, 0, opts.Limit)
	seen := map[string]struct{}{}
	offset := opts.Offset

	// Guard against server-side pagination bugs that never return an empty page.
	for attempt := 0; attempt < 1000; attempt++ {
		reqCtx, cancel := withRequestTimeout(timeout)
		opts.Offset = offset
		result, err := fetchDocsPages(reqCtx, client, opts)
		cancel()
		if err != nil {
			return nil, err
		}
		if len(result.Pages) == 0 {
			break
		}

		added := 0
		for _, page := range result.Pages {
			slug := strings.TrimSpace(page.Slug)
			if slug == "" {
				continue
			}
			if _, exists := seen[slug]; exists {
				continue
			}
			seen[slug] = struct{}{}
			all = append(all, page)
			added++
		}

		offset += len(result.Pages)
		if result.Total > 0 && offset >= result.Total {
			break
		}
		if added == 0 {
			// No new pages discovered; avoid infinite loops if server ignores offset.
			break
		}
	}
	return all, nil
}

func fetchDocsPageMarkdown(ctx context.Context, client api.Client, slug string) (string, error) {
	return fetchDocsPageContent(ctx, client, slug, "markdown")
}

func fetchDocsPageContent(ctx context.Context, client api.Client, slug string, outFormat string) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", errors.New("missing docs page slug")
	}

	format := strings.ToLower(strings.TrimSpace(outFormat))
	switch format {
	case "", "md":
		format = "markdown"
	case "markdown", "html", "json":
		// ok
	default:
		return "", fmt.Errorf("unsupported docs page format %q (expected markdown|html|json)", outFormat)
	}

	q := url.Values{}
	q.Set("format", format)
	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/docs/pages/"+url.PathEscape(slug), q, nil)
	if err != nil {
		return "", fmt.Errorf("fetch docs page %q (%s): %w", slug, format, err)
	}
	if status < 200 || status > 299 {
		if raw, ok := out.(string); ok && strings.TrimSpace(raw) != "" {
			return "", fmt.Errorf("fetch docs page %q failed (status=%d): %s", slug, status, strings.TrimSpace(raw))
		}
		return "", fmt.Errorf("fetch docs page %q failed (status=%d)", slug, status)
	}

	if format == "json" {
		pretty, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode json docs page %q response: %w", slug, err)
		}
		return string(pretty) + "\n", nil
	}

	if s, ok := out.(string); ok {
		return s, nil
	}

	var payload struct {
		Data struct {
			Page struct {
				Markdown string `json:"markdown"`
				HTML     string `json:"html"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := decodeLooseJSON(out, &payload); err != nil {
		return "", fmt.Errorf("decode %s docs page %q response: %w", format, slug, err)
	}

	if format == "html" {
		return payload.Data.Page.HTML, nil
	}
	return payload.Data.Page.Markdown, nil
}

func writeDocsPages(client api.Client, rootOut string, pages []docsPageMeta, timeout time.Duration) error {
	pagesOutDir := filepath.Join(rootOut, "pages")
	if err := os.MkdirAll(pagesOutDir, 0o755); err != nil {
		return fmt.Errorf("create pages output dir: %w", err)
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Slug < pages[j].Slug
	})

	for _, page := range pages {
		slug, err := sanitizeDocSlug(page.Slug)
		if err != nil {
			return err
		}
		ctx, cancel := withRequestTimeout(timeout)
		markdown, err := fetchDocsPageMarkdown(ctx, client, page.Slug)
		cancel()
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(pagesOutDir, slug+".md"), []byte(markdown)); err != nil {
			return err
		}
	}

	indexPayload := map[string]any{
		"sourceApi":    strings.TrimRight(strings.TrimSpace(client.BaseURL), "/"),
		"downloadedAt": time.Now().UTC().Format(time.RFC3339),
		"pages":        pages,
	}
	indexJSON, err := json.MarshalIndent(indexPayload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pages index: %w", err)
	}
	if err := writeFile(filepath.Join(rootOut, "pages-index.json"), indexJSON); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(rootOut, ".breyta-docs-marker"), []byte("breyta-docs-v1\n")); err != nil {
		return err
	}
	return nil
}

func withRequestTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}

func validateDocsSyncCleanTarget(rootOut string) error {
	rootOut = filepath.Clean(strings.TrimSpace(rootOut))
	if rootOut == "" {
		return errors.New("missing output directory")
	}
	if rootOut == "." || rootOut == ".." {
		return fmt.Errorf("refusing to clean dangerous output path %q", rootOut)
	}

	if abs, err := filepath.Abs(rootOut); err == nil {
		if isFilesystemRoot(abs) {
			return fmt.Errorf("refusing to clean filesystem root %q", abs)
		}
		if depth := absolutePathDepth(abs); depth < 2 {
			return fmt.Errorf("refusing to clean high-level output path %q", abs)
		}
		if cwd, err := os.Getwd(); err == nil && sameCleanPath(abs, cwd) {
			return fmt.Errorf("refusing to clean current working directory %q", abs)
		}
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" && sameCleanPath(abs, home) {
			return fmt.Errorf("refusing to clean home directory %q", abs)
		}
	}
	return nil
}

func sameCleanPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func isFilesystemRoot(path string) bool {
	clean := filepath.Clean(path)
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	sep := string(filepath.Separator)
	return rest == sep || rest == ""
}

func absolutePathDepth(path string) int {
	clean := filepath.Clean(path)
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	sep := string(filepath.Separator)
	rest = strings.TrimPrefix(rest, sep)
	if rest == "" {
		return 0
	}
	parts := strings.Split(rest, sep)
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}

func decodeLooseJSON(raw any, into any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func sanitizeDocSlug(slug string) (string, error) {
	slug = strings.TrimSpace(strings.ReplaceAll(slug, "\\", "/"))
	slug = strings.Trim(slug, "/")
	if slug == "" {
		return "", errors.New("docs page slug is empty")
	}
	if strings.Contains(slug, "..") || strings.Contains(slug, "/") {
		return "", fmt.Errorf("unsafe docs page slug %q", slug)
	}
	return slug, nil
}
