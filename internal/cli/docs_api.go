package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	cliFormat "github.com/breyta/breyta-cli/internal/format"
	"github.com/spf13/cobra"
)

type docsIndexRow struct {
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
	Description   string   `json:"description,omitempty"`
}

func newDocsFindCmd(app *App) *cobra.Command {
	var outFormat string
	var source string
	var query string
	var withSummary bool
	var withSnippets bool
	var explain bool
	var limit int
	var offset int
	var timeoutSeconds int
	var noHeader bool

	cmd := &cobra.Command{
		Use:   "find [query]",
		Short: "Find docs pages",
		Long: "Find docs pages from the API.\n\n" +
			"Query supports plain terms and Lucene-style expressions when available on the server,\n" +
			"for example: `source:cli deploy`, `\"flow deploy\"`, `bindings -oauth`.",
		Example: strings.TrimSpace(`
breyta docs find "flows push"
breyta docs find "source:cli deploy"
breyta docs find "\"end-user\" AND source:flows-api" --format json
`),
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				if strings.TrimSpace(query) != "" {
					return writeErr(cmd, fmt.Errorf("query provided twice; use either positional [query] or --q"))
				}
				query = strings.Join(args, " ")
			}
			if limit < -1 {
				return writeErr(cmd, fmt.Errorf("invalid --limit: must be -1 (default) or >= 0"))
			}
			if offset < 0 {
				return writeErr(cmd, fmt.Errorf("invalid --offset: must be >= 0"))
			}

			ensureAPIURL(app)
			if strings.TrimSpace(app.APIURL) == "" {
				return writeErr(cmd, fmt.Errorf("missing api base url"))
			}

			timeout := time.Duration(timeoutSeconds) * time.Second
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			ctx, cancel := withRequestTimeout(timeout)
			defer cancel()

			client := api.Client{
				BaseURL: app.APIURL,
				Token:   app.Token,
			}

			pages, err := fetchDocsPages(ctx, client, docsPagesQueryOptions{
				Source:       source,
				Query:        query,
				WithSnippets: withSnippets,
				Explain:      explain,
				Limit:        limit,
				Offset:       offset,
			})
			if err != nil {
				return writeErr(cmd, err)
			}

			rows := make([]docsIndexRow, 0, len(pages))
			for _, p := range pages {
				rows = append(rows, docsIndexRow{
					Slug:          p.Slug,
					Title:         p.Title,
					Source:        p.Source,
					Category:      p.Category,
					Order:         p.Order,
					Tags:          p.Tags,
					Score:         p.Score,
					Snippet:       p.Snippet,
					MatchedFields: append([]string{}, p.MatchedFields...),
					Explain:       p.Explain,
					Description:   p.Snippet,
				})
			}

			if withSummary {
				for i := range rows {
					pageCtx, pageCancel := withRequestTimeout(timeout)
					md, err := fetchDocsPageContent(pageCtx, client, rows[i].Slug, "markdown")
					pageCancel()
					if err != nil {
						return writeErr(cmd, err)
					}
					rows[i].Description = summarizeMarkdown(md)
				}
			}

			switch strings.ToLower(strings.TrimSpace(outFormat)) {
			case "", "tsv", "text":
				if !noHeader {
					_, _ = io.WriteString(cmd.OutOrStdout(), "slug\ttitle\tdescription\n")
				}
				for _, r := range rows {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n",
						r.Slug,
						strings.ReplaceAll(r.Title, "\t", " "),
						strings.ReplaceAll(r.Description, "\t", " "))
				}
				return nil
			case "json":
				return cliFormat.Write(cmd.OutOrStdout(), map[string]any{
					"ok": true,
					"data": map[string]any{
						"pages": rows,
					},
				}, "json", true)
			default:
				return writeErr(cmd, fmt.Errorf("unknown format %q (expected tsv|json)", outFormat))
			}
		},
	}

	cmd.Flags().StringVar(&outFormat, "format", "tsv", "Output format (tsv|json)")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source (flows-api|cli|all)")
	cmd.Flags().StringVar(&query, "q", "", "Query expression (plain terms or Lucene syntax)")
	cmd.Flags().IntVar(&limit, "limit", -1, "Max results to return (-1 = API default)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset for pagination")
	cmd.Flags().BoolVar(&withSummary, "with-summary", true, "Fetch each page and include first summary line")
	cmd.Flags().BoolVar(&withSnippets, "with-snippets", false, "Ask API to include search snippets in results")
	cmd.Flags().BoolVar(&explain, "explain", false, "Ask API to include query explanation per result")
	cmd.Flags().BoolVar(&noHeader, "no-header", false, "Do not print tsv header row")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 30, "Request timeout in seconds")
	return cmd
}

func newDocsShowCmd(app *App) *cobra.Command {
	var outFormat string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Print a docs page to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ensureAPIURL(app)
			if strings.TrimSpace(app.APIURL) == "" {
				return writeErr(cmd, fmt.Errorf("missing api base url"))
			}

			timeout := time.Duration(timeoutSeconds) * time.Second
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			client := api.Client{
				BaseURL: app.APIURL,
				Token:   app.Token,
			}
			content, err := fetchDocsPageContent(ctx, client, args[0], outFormat)
			if err != nil {
				return writeErr(cmd, err)
			}
			_, _ = io.WriteString(cmd.OutOrStdout(), content)
			if !strings.HasSuffix(content, "\n") {
				_, _ = io.WriteString(cmd.OutOrStdout(), "\n")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&outFormat, "format", "markdown", "Page format (markdown|html|json)")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 30, "Request timeout in seconds")
	return cmd
}

func summarizeMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inCode := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode || line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "- ") ||
			strings.HasPrefix(line, "* ") {
			continue
		}

		s := line
		s = strings.ReplaceAll(s, "`", "")
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "__", "")
		s = strings.Join(strings.Fields(s), " ")
		return truncateRunes(s, 160)
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
