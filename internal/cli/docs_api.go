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
	Slug        string   `json:"slug"`
	Title       string   `json:"title,omitempty"`
	Source      string   `json:"source,omitempty"`
	Category    string   `json:"category,omitempty"`
	Order       int      `json:"order,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
}

func newDocsIndexCmd(app *App) *cobra.Command {
	var outFormat string
	var source string
	var query string
	var withSummary bool
	var timeoutSeconds int
	var noHeader bool

	cmd := &cobra.Command{
		Use:     "index",
		Aliases: []string{"pages", "list"},
		Short:   "List available API docs pages",
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

			pages, err := fetchDocsPages(ctx, client, source, query)
			if err != nil {
				return writeErr(cmd, err)
			}

			rows := make([]docsIndexRow, 0, len(pages))
			for _, p := range pages {
				rows = append(rows, docsIndexRow{
					Slug:     p.Slug,
					Title:    p.Title,
					Source:   p.Source,
					Category: p.Category,
					Order:    p.Order,
					Tags:     p.Tags,
				})
			}

			if withSummary {
				for i := range rows {
					md, err := fetchDocsPageContent(ctx, client, rows[i].Slug, "markdown")
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
	cmd.Flags().StringVar(&query, "q", "", "Query filter for slug/title/source")
	cmd.Flags().BoolVar(&withSummary, "with-summary", true, "Fetch each page and include first summary line")
	cmd.Flags().BoolVar(&noHeader, "no-header", false, "Do not print tsv header row")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 30, "Request timeout in seconds")
	return cmd
}

func newDocsPageCmd(app *App) *cobra.Command {
	var outFormat string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "page <slug>",
		Short: "Print a docs page to stdout for piping",
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
