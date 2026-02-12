package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/skilldocs"
	"github.com/breyta/breyta-cli/skills"
	"github.com/spf13/cobra"
)

type docsPageMeta struct {
	Slug     string   `json:"slug"`
	Title    string   `json:"title,omitempty"`
	Source   string   `json:"source,omitempty"`
	Category string   `json:"category,omitempty"`
	Order    int      `json:"order,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func newDocsDownloadCmd(app *App) *cobra.Command {
	var outDir string
	var clean bool
	var includeSkill bool
	var skillSlug string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "download",
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
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			client := api.Client{
				BaseURL: app.APIURL,
				Token:   app.Token,
			}

			pages, err := fetchDocsPages(ctx, client, "", "")
			if err != nil {
				return writeErr(cmd, err)
			}
			if len(pages) == 0 {
				return writeErr(cmd, errors.New("no docs pages returned by API"))
			}

			if err := writeDocsPages(ctx, client, rootOut, pages); err != nil {
				return writeErr(cmd, err)
			}

			skillCount := 0
			if includeSkill {
				slug := strings.TrimSpace(skillSlug)
				if slug == "" {
					return writeErr(cmd, errors.New("skill slug cannot be empty when --include-skill=true"))
				}
				written, err := writeSkillBundle(ctx, rootOut, app.APIURL, app.Token, slug)
				if err != nil {
					return writeErr(cmd, err)
				}
				skillCount = written
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Downloaded %d docs pages to %s\n", len(pages), filepath.Join(rootOut, "pages"))
			if includeSkill {
				fmt.Fprintf(cmd.OutOrStdout(), "Downloaded skill bundle (%s) with %d files to %s\n",
					strings.TrimSpace(skillSlug), skillCount, filepath.Join(rootOut, "skills", strings.TrimSpace(skillSlug)))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Ready for grep: rg -n \"<query>\" %s\n", rootOut)
			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", ".breyta-docs", "Output directory for downloaded docs")
	cmd.Flags().BoolVar(&clean, "clean", false, "Delete output directory before download")
	cmd.Flags().BoolVar(&includeSkill, "include-skill", true, "Also download docs skill bundle files")
	cmd.Flags().StringVar(&skillSlug, "skill-slug", skills.BreytaSkillSlug, "Skill slug to download when --include-skill=true")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 90, "Request timeout in seconds")
	return cmd
}

func fetchDocsPages(ctx context.Context, client api.Client, source, q string) ([]docsPageMeta, error) {
	query := url.Values{}
	if strings.TrimSpace(source) != "" {
		query.Set("source", strings.TrimSpace(source))
	}
	if strings.TrimSpace(q) != "" {
		query.Set("q", strings.TrimSpace(q))
	}

	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/docs/pages", query, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch docs pages: %w", err)
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("fetch docs pages failed (status=%d)", status)
	}

	var payload struct {
		Data struct {
			Pages []docsPageMeta `json:"pages"`
		} `json:"data"`
	}
	if err := decodeLooseJSON(out, &payload); err != nil {
		return nil, fmt.Errorf("decode docs pages response: %w", err)
	}
	return payload.Data.Pages, nil
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

func writeDocsPages(ctx context.Context, client api.Client, rootOut string, pages []docsPageMeta) error {
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
		markdown, err := fetchDocsPageMarkdown(ctx, client, page.Slug)
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
	return nil
}

func writeSkillBundle(ctx context.Context, rootOut, apiURL, token, skillSlug string) (int, error) {
	manifest, files, err := skilldocs.FetchBundle(ctx, nil, apiURL, token, skillSlug)
	if err != nil {
		return 0, fmt.Errorf("fetch skill bundle %q: %w", skillSlug, err)
	}

	skillRoot := filepath.Join(rootOut, "skills", skillSlug)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("encode skill manifest: %w", err)
	}
	if err := writeFile(filepath.Join(skillRoot, "manifest.json"), manifestJSON); err != nil {
		return 0, err
	}

	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)

	written := 0
	for _, rel := range paths {
		safeRel, ok := sanitizeRelativePath(rel)
		if !ok {
			return written, fmt.Errorf("invalid skill file path %q", rel)
		}
		dest := filepath.Join(skillRoot, "files", filepath.FromSlash(safeRel))
		if err := writeFile(dest, files[rel]); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
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

func sanitizeRelativePath(rel string) (string, bool) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", false
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", false
	}
	return cleaned, true
}
