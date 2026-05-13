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
	Ref           string   `json:"ref,omitempty"`
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
	NextCommand   string   `json:"nextCommand,omitempty"`
}

type docsConfigFieldRow struct {
	Slug     string   `json:"slug"`
	Section  string   `json:"section,omitempty"`
	Field    string   `json:"field"`
	Type     string   `json:"type,omitempty"`
	Required string   `json:"required,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
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
			"for example: `source:cli release`, `\"flow release\"`, `bindings -oauth`.\n\n" +
			"Default output is rg-like: compact hit rows include the source, matched fields,\n" +
			"a bounded snippet, and the next `docs show` command for focused inspection.",
		Example: strings.TrimSpace(`
breyta docs find "flows push"
breyta docs find "source:cli release"
breyta docs find "\"live\" AND source:flows-api" --format json
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

			result, err := fetchDocsPages(ctx, client, docsPagesQueryOptions{
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
			pages := result.Pages

			rows := make([]docsIndexRow, 0, len(pages))
			for _, p := range pages {
				snippet := strings.TrimSpace(p.Snippet)
				rows = append(rows, docsIndexRow{
					Slug:          p.Slug,
					Ref:           "docs:" + p.Slug,
					Title:         p.Title,
					Source:        p.Source,
					Category:      p.Category,
					Order:         p.Order,
					Tags:          p.Tags,
					Score:         p.Score,
					Snippet:       snippet,
					MatchedFields: append([]string{}, p.MatchedFields...),
					Explain:       p.Explain,
					Description:   snippet,
					NextCommand:   "breyta docs show " + p.Slug,
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
					_, _ = io.WriteString(cmd.OutOrStdout(), "ref\tsource\tmatched\ttitle\tsnippet\tnext\n")
				}
				for _, r := range rows {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\n",
						escapeTSV(r.Ref),
						escapeTSV(r.Source),
						escapeTSV(strings.Join(r.MatchedFields, ",")),
						escapeTSV(r.Title),
						escapeTSV(r.Description),
						escapeTSV(r.NextCommand))
				}
				return nil
			case "json":
				return cliFormat.Write(cmd.OutOrStdout(), map[string]any{
					"ok": true,
					"data": map[string]any{
						"pages": rows,
					},
					"meta": map[string]any{
						"outputView": "compact-rg",
						"total":      result.Total,
						"limit":      result.Limit,
						"offset":     offset,
						"query":      strings.TrimSpace(query),
						"nextCommands": []string{
							"breyta docs show <slug> --section <heading>",
							"breyta docs fields <step> [field...]",
							"breyta docs find <query> --with-summary",
						},
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
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results to return (-1 = API default)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset for pagination")
	cmd.Flags().BoolVar(&withSummary, "with-summary", false, "Fetch each page and include first summary line instead of only search snippets")
	cmd.Flags().BoolVar(&withSnippets, "with-snippets", true, "Ask API to include search snippets in results")
	cmd.Flags().BoolVar(&explain, "explain", false, "Ask API to include query explanation per result")
	cmd.Flags().BoolVar(&noHeader, "no-header", false, "Do not print tsv header row")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 30, "Request timeout in seconds")
	return cmd
}

func newDocsFieldsCmd(app *App) *cobra.Command {
	var outFormat string
	var section string
	var timeoutSeconds int
	var noHeader bool

	cmd := &cobra.Command{
		Use:   "fields <step-type-or-doc-slug> [field ...]",
		Short: "Show compact step config field docs",
		Long: "Show compact field docs extracted from reference tables.\n\n" +
			"Pass only the step type for an overview of the config object, or pass one\n" +
			"or more field names for a targeted read.",
		Example: strings.TrimSpace(`
breyta docs fields http
breyta docs fields http response-as persist retry
breyta docs fields files source paths --section read
breyta docs fields :llm model output tools --format json
breyta docs fields reference-step-files op source paths --format markdown
`),
		Args: cobra.MinimumNArgs(1),
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

			slug := docsFieldsSlug(args[0])
			client := api.Client{
				BaseURL: app.APIURL,
				Token:   app.Token,
			}
			content, err := fetchDocsPageContent(ctx, client, slug, "markdown")
			if err != nil {
				return writeErr(cmd, err)
			}
			rows := extractDocsConfigFieldRows(content, slug)
			if len(rows) == 0 {
				return writeErr(cmd, fmt.Errorf("no config field table found in %s; try `breyta docs show %s --section \"Canonical Shape\"`", slug, slug))
			}
			availableSections := docsAvailableConfigSections(rows)
			section = strings.TrimSpace(section)
			if section != "" {
				filtered := filterDocsConfigFieldRowsBySection(rows, section)
				if len(filtered) == 0 {
					return writeErr(cmd, fmt.Errorf("section %q not found in %s; available sections: %s",
						section,
						slug,
						strings.Join(availableSections, ", ")))
				}
				rows = filtered
			}
			availableFields := docsAvailableConfigFields(rows)

			requested := args[1:]
			if len(requested) > 0 {
				selected, missing := selectDocsConfigFieldRows(rows, requested)
				if len(missing) > 0 {
					return writeErr(cmd, fmt.Errorf("field(s) not found in %s: %s; available fields: %s",
						slug,
						strings.Join(missing, ", "),
						strings.Join(availableFields, ", ")))
				}
				rows = selected
			}

			switch strings.ToLower(strings.TrimSpace(outFormat)) {
			case "", "tsv", "text":
				if !noHeader {
					_, _ = io.WriteString(cmd.OutOrStdout(), "section\tfield\ttype\trequired\tnotes\n")
				}
				for _, row := range rows {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n",
						escapeTSV(row.Section),
						escapeTSV(row.Field),
						escapeTSV(row.Type),
						escapeTSV(row.Required),
						escapeTSV(row.Notes))
				}
				return nil
			case "json":
				return cliFormat.Write(cmd.OutOrStdout(), map[string]any{
					"ok": true,
					"data": map[string]any{
						"slug":              slug,
						"fields":            rows,
						"availableFields":   availableFields,
						"availableSections": availableSections,
					},
				}, "json", true)
			case "markdown", "md":
				return writeDocsConfigFieldsMarkdown(cmd.OutOrStdout(), slug, rows)
			default:
				return writeErr(cmd, fmt.Errorf("unknown format %q (expected tsv|json|markdown)", outFormat))
			}
		},
	}

	cmd.Flags().StringVar(&outFormat, "format", "tsv", "Output format (tsv|json|markdown)")
	cmd.Flags().StringVar(&section, "section", "", "Filter rows by markdown section heading, for example read or Canonical Shape")
	cmd.Flags().BoolVar(&noHeader, "no-header", false, "Do not print tsv header row")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout-seconds", 30, "Request timeout in seconds")
	return cmd
}

func newDocsShowCmd(app *App) *cobra.Command {
	var outFormat string
	var timeoutSeconds int
	var full bool
	var section string
	var maxChars int

	cmd := &cobra.Command{
		Use:   "show <slug>",
		Short: "Print a compact docs page preview to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format := strings.ToLower(strings.TrimSpace(outFormat))
			if format == "" || format == "md" {
				format = "markdown"
			}
			if strings.TrimSpace(section) != "" && format != "markdown" {
				return writeErr(cmd, fmt.Errorf("--section only applies to markdown docs output"))
			}
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
			content, err := fetchDocsPageContent(ctx, client, args[0], format)
			if err != nil {
				return writeErr(cmd, err)
			}
			if format == "markdown" {
				section = strings.TrimSpace(section)
				if section != "" {
					selected, ok := extractMarkdownSection(content, section)
					if !ok {
						return writeErr(cmd, docsSectionNotFoundError(args[0], section, content))
					}
					if full {
						content = strings.TrimRight(selected, "\n")
					} else {
						content = compactDocsMarkdown(content, args[0], section, maxChars)
					}
				} else if !full {
					content = compactDocsMarkdown(content, args[0], "", maxChars)
				}
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
	cmd.Flags().BoolVar(&full, "full", false, "Print the full markdown page instead of the compact default preview")
	cmd.Flags().StringVar(&section, "section", "", "Print a focused markdown section by heading text")
	cmd.Flags().IntVar(&maxChars, "max-chars", compactDocsDefaultRunes, "Approximate markdown preview character budget before --full is required")
	return cmd
}

func docsFieldsSlug(input string) string {
	raw := strings.TrimSpace(input)
	normalized := normalizeDocsFieldName(raw)
	switch normalized {
	case "http":
		return "reference-step-http"
	case "llm":
		return "reference-step-llm"
	case "agent":
		return "reference-step-agent"
	case "breyta":
		return "reference-step-breyta"
	case "db":
		return "reference-step-db"
	case "sql", "postgres", "mysql", "clickhouse", "db-sql":
		return "reference-step-db-sql"
	case "bigquery", "db-bigquery":
		return "reference-step-db-bigquery"
	case "firestore", "db-firestore":
		return "reference-step-db-firestore"
	case "wait":
		return "reference-step-wait"
	case "function", "fn":
		return "reference-step-function"
	case "job":
		return "reference-step-job"
	case "notify":
		return "reference-step-notify"
	case "kv":
		return "reference-step-kv"
	case "table":
		return "reference-step-table"
	case "files":
		return "reference-step-files"
	case "sleep":
		return "reference-step-sleep"
	case "ssh":
		return "reference-step-ssh"
	case "fanout":
		return "reference-step-fanout"
	case "search":
		return "reference-step-search"
	}
	if strings.HasPrefix(normalized, "reference-") ||
		strings.HasPrefix(normalized, "guide-") ||
		strings.HasPrefix(normalized, "playbook-") {
		return normalized
	}
	return "reference-step-" + normalized
}

func extractDocsConfigFieldRows(markdown string, slug string) []docsConfigFieldRow {
	lines := strings.Split(markdown, "\n")
	rows := make([]docsConfigFieldRow, 0)
	currentSection := ""
	inCode := false

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "```") {
			inCode = !inCode
			continue
		}
		if inCode || line == "" {
			continue
		}
		if heading := markdownHeadingText(line); heading != "" {
			currentSection = heading
			continue
		}
		if i+1 >= len(lines) || !looksLikeMarkdownTableRow(line) || !looksLikeMarkdownTableSeparator(lines[i+1]) {
			continue
		}
		headers := splitMarkdownTableRow(line)
		indexes, ok := docsConfigFieldHeaderIndexes(headers)
		if ok {
			for j := i + 2; j < len(lines); j++ {
				rowLine := strings.TrimSpace(lines[j])
				if rowLine == "" || !looksLikeMarkdownTableRow(rowLine) {
					break
				}
				cells := splitMarkdownTableRow(rowLine)
				if indexes.field >= len(cells) {
					continue
				}
				field := cleanDocsTableCell(cells[indexes.field])
				if field == "" {
					continue
				}
				row := docsConfigFieldRow{
					Slug:     slug,
					Section:  currentSection,
					Field:    field,
					Type:     cellAt(cells, indexes.typ),
					Required: cellAt(cells, indexes.required),
					Notes:    cellAt(cells, indexes.notes),
					Aliases:  docsConfigFieldAliases(cells[indexes.field]),
				}
				rows = append(rows, row)
			}
			continue
		}
		opIndexes, ok := docsConfigOpHeaderIndexes(headers)
		if ok {
			for j := i + 2; j < len(lines); j++ {
				rowLine := strings.TrimSpace(lines[j])
				if rowLine == "" || !looksLikeMarkdownTableRow(rowLine) {
					break
				}
				cells := splitMarkdownTableRow(rowLine)
				if opIndexes.op >= len(cells) {
					continue
				}
				op := cleanDocsTableCell(cells[opIndexes.op])
				if op == "" {
					continue
				}
				rows = append(rows, docsConfigRowsFromOpCell(slug, currentSection, op, cellRaw(cells, opIndexes.required), "Yes", cellAt(cells, opIndexes.notes))...)
				rows = append(rows, docsConfigRowsFromOpCell(slug, currentSection, op, cellRaw(cells, opIndexes.optional), "No", cellAt(cells, opIndexes.notes))...)
			}
			continue
		}
	}
	return rows
}

func docsConfigRowsFromOpCell(slug string, section string, op string, cell string, required string, notes string) []docsConfigFieldRow {
	clean := strings.ToLower(cleanDocsTableCell(cell))
	if clean == "" || clean == "none" || clean == "n/a" || clean == "-" {
		return nil
	}
	aliases := docsConfigFieldAliases(cell)
	if len(aliases) == 0 {
		return nil
	}
	opSection := strings.TrimSpace(section)
	if opSection == "" {
		opSection = op
	} else {
		opSection = opSection + " / " + op
	}
	out := make([]docsConfigFieldRow, 0, len(aliases))
	for _, alias := range aliases {
		field := displayDocsFieldName(alias)
		if field == "" {
			continue
		}
		out = append(out, docsConfigFieldRow{
			Slug:     slug,
			Section:  opSection,
			Field:    field,
			Required: required,
			Notes:    notes,
			Aliases:  []string{alias},
		})
	}
	return out
}

type docsConfigFieldHeaderIndexSet struct {
	field    int
	typ      int
	required int
	notes    int
}

func docsConfigFieldHeaderIndexes(headers []string) (docsConfigFieldHeaderIndexSet, bool) {
	indexes := docsConfigFieldHeaderIndexSet{field: -1, typ: -1, required: -1, notes: -1}
	for i, header := range headers {
		normalized := normalizeDocsTableHeader(header)
		switch normalized {
		case "field", "option", "key":
			indexes.field = i
		case "type":
			indexes.typ = i
		case "required", "required?":
			indexes.required = i
		case "notes", "note", "description", "meaning", "typical use":
			if indexes.notes < 0 {
				indexes.notes = i
			}
		}
	}
	return indexes, indexes.field >= 0 && (indexes.typ >= 0 || indexes.required >= 0 || indexes.notes >= 0)
}

type docsConfigOpHeaderIndexSet struct {
	op       int
	required int
	optional int
	notes    int
}

func docsConfigOpHeaderIndexes(headers []string) (docsConfigOpHeaderIndexSet, bool) {
	indexes := docsConfigOpHeaderIndexSet{op: -1, required: -1, optional: -1, notes: -1}
	for i, header := range headers {
		normalized := normalizeDocsTableHeader(header)
		switch normalized {
		case "op", "operation":
			indexes.op = i
		case "required fields", "required":
			indexes.required = i
		case "optional fields", "optional":
			indexes.optional = i
		case "notes", "note", "description":
			indexes.notes = i
		}
	}
	return indexes, indexes.op >= 0 && (indexes.required >= 0 || indexes.optional >= 0)
}

func selectDocsConfigFieldRows(rows []docsConfigFieldRow, requested []string) ([]docsConfigFieldRow, []string) {
	selected := make([]docsConfigFieldRow, 0, len(requested))
	missing := make([]string, 0)
	seen := map[int]struct{}{}
	for _, raw := range requested {
		needle := normalizeDocsFieldName(raw)
		if needle == "" {
			continue
		}
		found := false
		for i, row := range rows {
			if !docsConfigFieldRowMatches(row, needle) {
				continue
			}
			found = true
			if _, ok := seen[i]; !ok {
				selected = append(selected, row)
				seen[i] = struct{}{}
			}
		}
		if !found {
			missing = append(missing, raw)
		}
	}
	return selected, missing
}

func filterDocsConfigFieldRowsBySection(rows []docsConfigFieldRow, section string) []docsConfigFieldRow {
	needle := normalizeHeadingText(section)
	if needle == "" {
		return rows
	}
	out := make([]docsConfigFieldRow, 0, len(rows))
	for _, row := range rows {
		if docsConfigSectionMatches(row.Section, needle) {
			out = append(out, row)
		}
	}
	return out
}

func docsConfigSectionMatches(section string, normalizedNeedle string) bool {
	full := normalizeHeadingText(section)
	if full == normalizedNeedle {
		return true
	}
	for _, part := range strings.Split(section, "/") {
		if normalizeHeadingText(part) == normalizedNeedle {
			return true
		}
	}
	return false
}

func docsConfigFieldRowMatches(row docsConfigFieldRow, needle string) bool {
	for _, alias := range row.Aliases {
		if normalizeDocsFieldName(alias) == needle {
			return true
		}
	}
	return normalizeDocsFieldName(row.Field) == needle
}

func docsAvailableConfigSections(rows []docsConfigFieldRow) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, row := range rows {
		section := strings.TrimSpace(row.Section)
		if section == "" {
			continue
		}
		key := normalizeHeadingText(section)
		if key == "" {
			key = section
		}
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, section)
		seen[key] = struct{}{}
	}
	return out
}

func docsAvailableConfigFields(rows []docsConfigFieldRow) []string {
	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		names := make([]string, 0, len(row.Aliases))
		for _, alias := range row.Aliases {
			alias = normalizeDocsFieldName(alias)
			if alias != "" {
				names = append(names, alias)
			}
		}
		if len(names) == 0 {
			if name := normalizeDocsFieldName(row.Field); name != "" {
				names = append(names, name)
			}
		}
		for _, name := range names {
			if _, ok := seen[name]; ok {
				continue
			}
			out = append(out, name)
			seen[name] = struct{}{}
		}
	}
	return out
}

func writeDocsConfigFieldsMarkdown(w io.Writer, slug string, rows []docsConfigFieldRow) error {
	_, _ = fmt.Fprintf(w, "# %s fields\n\n", slug)
	_, _ = io.WriteString(w, "| Section | Field | Type | Required | Notes |\n")
	_, _ = io.WriteString(w, "| --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			escapeMarkdownTableCell(row.Section),
			escapeMarkdownTableCell(row.Field),
			escapeMarkdownTableCell(row.Type),
			escapeMarkdownTableCell(row.Required),
			escapeMarkdownTableCell(row.Notes))
	}
	return nil
}

func looksLikeMarkdownTableRow(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && strings.Count(line, "|") >= 2
}

func looksLikeMarkdownTableSeparator(line string) bool {
	cells := splitMarkdownTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func splitMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	out := make([]string, 0)
	var b strings.Builder
	inCode := false
	escaped := false
	for _, r := range line {
		if escaped {
			if r == '|' {
				b.WriteRune(r)
			} else {
				b.WriteRune('\\')
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '`' {
			inCode = !inCode
			b.WriteRune(r)
			continue
		}
		if r == '|' && !inCode {
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	out = append(out, strings.TrimSpace(b.String()))
	return out
}

func markdownHeadingText(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#") {
		return ""
	}
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return ""
	}
	return strings.TrimSpace(line[level+1:])
}

func cellAt(cells []string, index int) string {
	return cleanDocsTableCell(cellRaw(cells, index))
}

func cellRaw(cells []string, index int) string {
	if index < 0 || index >= len(cells) {
		return ""
	}
	return cells[index]
}

func cleanDocsTableCell(s string) string {
	replacer := strings.NewReplacer(
		"`", "",
		"**", "",
		"__", "",
		"<br>", " ",
		"<br/>", " ",
		"<br />", " ",
		"\\|", "|",
		"\t", " ",
	)
	s = replacer.Replace(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), " ")
}

func docsConfigFieldAliases(cell string) []string {
	aliases := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(v string) {
		v = normalizeDocsFieldName(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		aliases = append(aliases, v)
		seen[v] = struct{}{}
	}

	rest := cell
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			break
		}
		rest = rest[start+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			break
		}
		add(rest[:end])
		rest = rest[end+1:]
	}
	if len(aliases) == 0 {
		for _, part := range strings.FieldsFunc(cell, func(r rune) bool {
			return r == '/' || r == ',' || r == ' ' || r == '\t'
		}) {
			add(part)
		}
	}
	return aliases
}

func normalizeDocsFieldName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, "`")
	s = strings.TrimPrefix(s, ":")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.Trim(s, "[](){}.,;:")
	s = strings.TrimSpace(s)
	return s
}

func displayDocsFieldName(s string) string {
	s = normalizeDocsFieldName(s)
	if s == "" {
		return ""
	}
	return ":" + s
}

func normalizeDocsTableHeader(s string) string {
	s = cleanDocsTableCell(s)
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Trim(s, "?")
}

func escapeTSV(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func escapeMarkdownTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func docsSectionNotFoundError(slug string, section string, markdown string) error {
	headings := markdownContents(markdown, 8)
	if len(headings) == 0 {
		return fmt.Errorf("section %q not found in %s; page has no markdown headings", section, slug)
	}
	return fmt.Errorf("section %q not found in %s; available headings: %s", section, slug, strings.Join(headings, ", "))
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
