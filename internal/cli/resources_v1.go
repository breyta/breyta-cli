package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func requireResourcesAPI(cmd *cobra.Command, app *App) error {
	// Respect explicit `--api=` forcing mock mode.
	if apiFlagExplicit(cmd) && strings.TrimSpace(app.APIURL) == "" {
		return errors.New("resources requires API mode (set BREYTA_API_URL)")
	}
	return requireAPI(app)
}

func parseJSONFlag(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func parseCommaFields(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields = append(fields, part)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func applyTablePartitions(body map[string]any, partitionKey string, partitionKeys string) error {
	key := strings.TrimSpace(partitionKey)
	keys := parseCommaFields(partitionKeys)
	if key != "" && len(keys) > 0 {
		return errors.New("use either --partition-key or --partition-keys, not both")
	}
	if key != "" {
		body["partition-key"] = key
	}
	if len(keys) > 0 {
		body["partition-keys"] = keys
	}
	return nil
}

func applyTablePreviewPartitions(q url.Values, partitionKey string, partitionKeys string) error {
	key := strings.TrimSpace(partitionKey)
	keys := parseCommaFields(partitionKeys)
	if key != "" && len(keys) > 0 {
		return errors.New("use either --partition-key or --partition-keys, not both")
	}
	if key != "" {
		q.Set("tablePartition", key)
	}
	if len(keys) > 0 {
		q.Set("tablePartitions", strings.Join(keys, ","))
	}
	return nil
}

func parseKeyAssignments(values []string) (map[string]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --key value %q (expected field=value)", raw)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid --key value %q (missing field)", raw)
		}
		out[key] = strings.TrimSpace(parts[1])
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func readCSVFile(path string) (string, error) {
	bytes, err := readExplicitFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func newResourcesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "Unified resource access (results, imports, files)",
		Long: strings.TrimSpace(`
Resources provide a unified model for all data produced and consumed by flows:
- Results: Step captures and explicit result persistence (KV/storage)
- Imports: External data fetched into flows
- Files/Bundles: File-backed resources

API routes:
  GET /<workspace>/api/resources                  - List resources
  GET /<workspace>/api/resources/search?q=...     - Search resources
  POST /<workspace>/api/resources/search-index    - Inspect/update per-resource search-index metadata
  GET /<workspace>/api/resources/by-uri?uri=...   - Get resource metadata
  GET /<workspace>/api/resources/content?uri=...  - Read resource content
  DELETE /<workspace>/api/files/by-uri?uri=...    - Delete a file/storage resource
  POST /<workspace>/api/resources/table/*         - Query/update/import/export table resources
  GET /<workspace>/api/resources/url?uri=...      - Get signed URL
  GET /<workspace>/api/resources/workflow/<id>    - List workflow resources
  GET /<workspace>/api/resources/workflow/<id>/step/<step-id> - List step resources

Resource URI format:
  res://v1/ws/<workspace-id>/<type>/<resource-id>

Types:
  - result: Step captures or explicit result storage
  - import: Imported external data
  - file: File-backed resource
  - bundle: Bundle-backed resource
  - external-dir: External directory mount
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newResourcesListCmd(app))
	cmd.AddCommand(newResourcesSearchCmd(app))
	cmd.AddCommand(newResourcesSearchIndexCmd(app))
	cmd.AddCommand(newResourcesUploadCmd(app))
	cmd.AddCommand(newResourcesDeleteCmd(app))
	cmd.AddCommand(newResourcesGetCmd(app))
	cmd.AddCommand(newResourcesReadCmd(app))
	cmd.AddCommand(newResourcesTableCmd(app))
	cmd.AddCommand(newResourcesURLCmd(app))
	cmd.AddCommand(newResourcesWorkflowCmd(app))
	return cmd
}

func newResourcesUploadCmd(app *App) *cobra.Command {
	var name string
	var contentType string
	var folder string
	var replace bool
	var overwrite bool
	var printURI bool

	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a local file as a workspace file resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireResourcesAPI(cmd, app); err != nil {
				return writeErr(cmd, err)
			}
			path := strings.TrimSpace(args[0])
			if path == "" {
				return writeErr(cmd, errors.New("upload requires a local file path"))
			}
			filename := strings.TrimSpace(name)
			if filename == "" {
				filename = filepath.Base(path)
			}
			replaceExisting := replace || overwrite || cmd.Flags().Changed("folder") || cmd.Flags().Changed("name")
			result, err := jobsWorkerUploadFileResource(cmd.Context(), app, path, filename, contentType, folder, replaceExisting)
			if err != nil {
				return writeErr(cmd, err)
			}
			uri := firstNonBlankString(result["resourceUri"], result["uri"])
			if printURI {
				if uri == "" {
					return writeErr(cmd, errors.New("upload response missing resource URI"))
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), uri)
				return err
			}
			return writeData(cmd, app, nil, map[string]any{"resource": result})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Resource filename to store; defaults to the local file basename and replaces the existing stable target when set")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content type to store; defaults to extension or file sniffing")
	cmd.Flags().StringVar(&folder, "folder", "", "Store the file inside this Storage folder, e.g. \"Company information\"; replaces the existing stable target when set")
	cmd.Flags().BoolVar(&replace, "replace", false, "Replace the existing resource with the same stable Storage folder and filename when present")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Alias for --replace")
	cmd.Flags().BoolVar(&printURI, "print-uri", false, "Print only the uploaded res:// URI")
	return cmd
}

func newResourcesDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <uri>",
		Aliases: []string{"archive", "rm"},
		Short:   "Delete a workspace file/storage resource by URI",
		Args:    cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			q := url.Values{}
			q.Set("uri", uri)

			out, status, err := apiClient(app).DoREST(
				cmd.Context(),
				http.MethodDelete,
				"/api/files/by-uri",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newResourcesListCmd(app *App) *cobra.Command {
	var typeFilter string
	var typesFilter string
	var prefix string
	var query string
	var accept string
	var excludeTier string
	var tags string
	var storageBackend string
	var storageRoot string
	var pathPrefix string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources in workspace",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if typeFilter != "" {
				q.Set("type", typeFilter)
			}
			if strings.TrimSpace(typesFilter) != "" {
				q.Set("types", strings.TrimSpace(typesFilter))
			}
			if strings.TrimSpace(query) != "" {
				q.Set("query", strings.TrimSpace(query))
			}
			if strings.TrimSpace(accept) != "" {
				q.Set("accept", strings.TrimSpace(accept))
			}
			if strings.TrimSpace(excludeTier) != "" {
				q.Set("exclude-tier", strings.TrimSpace(excludeTier))
			}
			if prefix != "" {
				q.Set("prefix", prefix)
			}
			if tags != "" {
				q.Set("tags", tags)
			}
			if strings.TrimSpace(storageBackend) != "" {
				q.Set("storage-backend", strings.TrimSpace(storageBackend))
			}
			if strings.TrimSpace(storageRoot) != "" {
				q.Set("storage-root", strings.TrimSpace(storageRoot))
			}
			if strings.TrimSpace(pathPrefix) != "" {
				q.Set("path-prefix", strings.TrimSpace(pathPrefix))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			out = compactResourceListPayload(enrichResourceListPayload(out))
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by resource type (result, import, file, bundle, external-dir)")
	cmd.Flags().StringVar(&typesFilter, "types", "", "Filter by resource types (comma-separated; supports file,result for picker-style queries)")
	cmd.Flags().StringVar(&query, "query", "", "Search query to combine with list filters")
	cmd.Flags().StringVar(&accept, "accept", "", "Filter by MIME types or wildcards (comma-separated, e.g. text/*,application/json)")
	cmd.Flags().StringVar(&excludeTier, "exclude-tier", "", "Exclude storage tiers (comma-separated; e.g. ephemeral)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Filter by URI prefix")
	cmd.Flags().StringVar(&tags, "tags", "", "Filter by tags (comma-separated)")
	cmd.Flags().StringVar(&storageBackend, "storage-backend", "", "Filter by storage backend id (e.g. platform)")
	cmd.Flags().StringVar(&storageRoot, "storage-root", "", "Filter by configured storage root (e.g. reports/acme)")
	cmd.Flags().StringVar(&pathPrefix, "path-prefix", "", "Filter by relative path prefix under the storage root (e.g. exports/2026)")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (0 to use server default, 1-1000)")
	return cmd
}

func newResourcesSearchCmd(app *App) *cobra.Command {
	var typeFilter string
	var contentSources string
	var storageBackend string
	var storageRoot string
	var pathPrefix string
	var mode string
	var keywordMode string
	var limit int
	var offset int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search resources in workspace",
		Long: strings.TrimSpace(`
Search workspace resources.

Use --keyword-mode balanced for natural-language questions over small resource
sets where requiring every query term may be too strict. Supported
keyword-mode values are all, balanced, and any.
`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(args[0])
			if query == "" {
				return writeErr(cmd, errors.New("missing search query"))
			}

			q := url.Values{}
			q.Set("q", query)
			if typeFilter != "" {
				q.Set("type", strings.TrimSpace(typeFilter))
			}
			if strings.TrimSpace(contentSources) != "" {
				q.Set("content-sources", strings.TrimSpace(contentSources))
			}
			if strings.TrimSpace(storageBackend) != "" {
				q.Set("storage-backend", strings.TrimSpace(storageBackend))
			}
			if strings.TrimSpace(storageRoot) != "" {
				q.Set("storage-root", strings.TrimSpace(storageRoot))
			}
			if strings.TrimSpace(pathPrefix) != "" {
				q.Set("path-prefix", strings.TrimSpace(pathPrefix))
			}
			if strings.TrimSpace(mode) != "" {
				q.Set("mode", strings.TrimSpace(mode))
			}
			if strings.TrimSpace(keywordMode) != "" {
				q.Set("keyword-mode", strings.TrimSpace(keywordMode))
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/search",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			out = compactResourceListPayload(enrichResourceListPayload(out))
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by resource type (result, import, file, bundle, external-dir)")
	cmd.Flags().StringVar(&contentSources, "content-sources", "file,result", "Comma-separated resource source types to search")
	cmd.Flags().StringVar(&storageBackend, "storage-backend", "", "Filter by storage backend id (e.g. platform)")
	cmd.Flags().StringVar(&storageRoot, "storage-root", "", "Filter by configured storage root (e.g. reports/acme)")
	cmd.Flags().StringVar(&pathPrefix, "path-prefix", "", "Filter by relative path prefix under the storage root (e.g. exports/2026)")
	cmd.Flags().StringVar(&mode, "mode", "", "Search mode: lexical, hybrid, or semantic")
	cmd.Flags().StringVar(&keywordMode, "keyword-mode", "", "Keyword matching mode: all, balanced, or any")
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (0 to use server default, 1-100)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset (>=0)")
	return cmd
}

func resourceSearchIndexRequest(cmd *cobra.Command, app *App, body map[string]any) error {
	out, status, err := apiClient(app).DoREST(
		cmd.Context(),
		http.MethodPost,
		"/api/resources/search-index",
		nil,
		body,
	)
	if err != nil {
		return writeErr(cmd, err)
	}
	return writeREST(cmd, app, status, out)
}

func newResourcesSearchIndexCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search-index",
		Short: "Inspect or update one resource's search-index metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newResourcesSearchIndexShowCmd(app))
	cmd.AddCommand(newResourcesSearchIndexUpdateCmd(app))
	cmd.AddCommand(newResourcesSearchIndexClearCmd(app))
	cmd.AddCommand(newResourcesSearchIndexReindexCmd(app))
	return cmd
}

func newResourcesSearchIndexShowCmd(app *App) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "show <uri>",
		Short: "Show search-index metadata and index preview for one resource",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}
			body := map[string]any{
				"op":   "show",
				"uri":  uri,
				"full": full,
			}
			return resourceSearchIndexRequest(cmd, app, body)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Include indexed content fields in the preview")
	return cmd
}

func newResourcesSearchIndexUpdateCmd(app *App) *cobra.Command {
	var searchIndexFile string
	var searchIndexRaw string
	var text string
	var sourceLabel string
	var tagsCSV string
	var tags []string
	var includeRawContent bool
	var full bool

	cmd := &cobra.Command{
		Use:   "update <uri>",
		Short: "Update search-index metadata for one resource and reindex it",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			fileSet := strings.TrimSpace(searchIndexFile) != ""
			rawSet := strings.TrimSpace(searchIndexRaw) != ""
			fieldSet := strings.TrimSpace(text) != "" ||
				strings.TrimSpace(sourceLabel) != "" ||
				strings.TrimSpace(tagsCSV) != "" ||
				len(tags) > 0 ||
				cmd.Flags().Changed("include-raw-content")
			if (fileSet || rawSet) && fieldSet {
				return writeErr(cmd, errors.New("use either --search-index-file/--search-index or field flags, not both"))
			}
			if fileSet && rawSet {
				return writeErr(cmd, errors.New("use either --search-index-file or --search-index, not both"))
			}
			body := map[string]any{
				"op":   "update",
				"uri":  uri,
				"full": full,
			}
			switch {
			case fileSet:
				b, err := readExplicitFile(strings.TrimSpace(searchIndexFile))
				if err != nil {
					return writeErr(cmd, err)
				}
				body["searchIndex"] = string(b)
			case rawSet:
				body["searchIndex"] = strings.TrimSpace(searchIndexRaw)
			case fieldSet:
				searchIndex := map[string]any{}
				if strings.TrimSpace(text) != "" {
					searchIndex["text"] = strings.TrimSpace(text)
				}
				if strings.TrimSpace(sourceLabel) != "" {
					searchIndex["sourceLabel"] = strings.TrimSpace(sourceLabel)
				}
				tagValues := append([]string{}, parseCommaFields(tagsCSV)...)
				for _, tag := range tags {
					tagValues = append(tagValues, parseCommaFields(tag)...)
				}
				if len(tagValues) > 0 {
					searchIndex["tags"] = tagValues
				}
				if cmd.Flags().Changed("include-raw-content") {
					searchIndex["includeRawContent"] = includeRawContent
				}
				body["searchIndex"] = searchIndex
			default:
				return writeErr(cmd, errors.New("provide --search-index-file, --search-index, or at least one search-index field flag"))
			}
			return resourceSearchIndexRequest(cmd, app, body)
		},
	}
	cmd.Flags().StringVar(&searchIndexFile, "search-index-file", "", "EDN or JSON map containing search-index metadata")
	cmd.Flags().StringVar(&searchIndexRaw, "search-index", "", "Inline EDN or JSON map containing search-index metadata")
	cmd.Flags().StringVar(&text, "text", "", "Override indexed search text")
	cmd.Flags().StringVar(&sourceLabel, "source-label", "", "Override the search result source label")
	cmd.Flags().StringVar(&tagsCSV, "tags", "", "Comma-separated search tags")
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Add one or more search tags; may be repeated")
	cmd.Flags().BoolVar(&includeRawContent, "include-raw-content", false, "Append raw resource content after --text when indexing")
	cmd.Flags().BoolVar(&full, "full", false, "Include indexed content fields in the response preview")
	return cmd
}

func newResourcesSearchIndexClearCmd(app *App) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "clear <uri>",
		Short: "Clear search-index metadata for one resource and reindex it",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}
			body := map[string]any{
				"op":   "clear",
				"uri":  uri,
				"full": full,
			}
			return resourceSearchIndexRequest(cmd, app, body)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Include indexed content fields in the response preview")
	return cmd
}

func newResourcesSearchIndexReindexCmd(app *App) *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "reindex <uri>",
		Short: "Rebuild one resource search-index document",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}
			body := map[string]any{
				"op":   "reindex",
				"uri":  uri,
				"full": full,
			}
			return resourceSearchIndexRequest(cmd, app, body)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Include indexed content fields in the response preview")
	return cmd
}

func newResourcesGetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <uri>",
		Short: "Get resource metadata by URI",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			q := url.Values{}
			q.Set("uri", uri)

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/by-uri",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	return cmd
}

func newResourcesReadCmd(app *App) *cobra.Command {
	var limit int
	var offset int
	var partitionKey string
	var partitionKeys string
	var full bool

	cmd := &cobra.Command{
		Use:   "read <uri>",
		Short: "Read resource content by URI",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			q := url.Values{}
			q.Set("uri", uri)
			if !full {
				q.Set("view", "summary")
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			} else if !full {
				q.Set("limit", "25")
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			if err := applyTablePreviewPartitions(q, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/content",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status == http.StatusForbidden {
				if fallback, ok := readRunStepResourceViaRunsGet(app, uri, full); ok {
					return writeREST(cmd, app, http.StatusOK, fallback)
				}
			}
			if !full {
				out = compactResourceReadPayload(out, uri)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Table preview page size when reading table resources (default 25, 1-1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Table preview offset when reading table resources")
	cmd.Flags().BoolVar(&full, "full", false, "Read the full resource payload instead of the default compact blob/table preview")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Preview a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Preview a comma-separated subset of table partitions")
	return cmd
}

var runStepResourceURIRe = regexp.MustCompile(`^res://v1/ws/([^/]+)/result/run/([^/]+)/step/([^/]+)/(input|output|error)$`)

type runStepResourceRef struct {
	WorkspaceID string
	WorkflowID  string
	StepID      string
	Kind        string
}

func parseRunStepResourceURI(raw string) (runStepResourceRef, bool) {
	m := runStepResourceURIRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 5 {
		return runStepResourceRef{}, false
	}
	stepID, err := url.QueryUnescape(m[3])
	if err != nil {
		stepID = m[3]
	}
	return runStepResourceRef{
		WorkspaceID: m[1],
		WorkflowID:  m[2],
		StepID:      strings.TrimPrefix(stepID, ":"),
		Kind:        m[4],
	}, true
}

func readRunStepResourceViaRunsGet(app *App, rawURI string, full bool) (any, bool) {
	ref, ok := parseRunStepResourceURI(rawURI)
	if !ok || strings.TrimSpace(ref.WorkflowID) == "" || strings.TrimSpace(ref.StepID) == "" {
		return nil, false
	}
	out, status, err := runAPICommand(app, "runs.get", map[string]any{
		"workflowId":         ref.WorkflowID,
		"includeSteps":       true,
		"includeResult":      false,
		"includeStepResults": true,
		"stepId":             ref.StepID,
	})
	if err != nil || status >= 400 || !isOK(out) {
		return nil, false
	}
	run := mapStringAny(mapStringAny(out["data"])["run"])
	step := findRunStep(run, ref.StepID)
	if step == nil {
		return nil, false
	}
	var payload any
	switch ref.Kind {
	case "output":
		payload = firstPresent(step, "output", "result")
	case "error":
		payload = firstPresent(step, "errorOutput", "error-output", "error")
	case "input":
		payload = firstPresent(step, "input", "params")
	}
	if payload == nil {
		return nil, false
	}
	if full {
		return payload, true
	}
	return compactResourceReadPayload(map[string]any{"data": payload}, rawURI), true
}

func newResourcesURLCmd(app *App) *cobra.Command {
	var ttl int

	cmd := &cobra.Command{
		Use:   "url <uri>",
		Short: "Get signed URL for resource access",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			q := url.Values{}
			q.Set("uri", uri)
			if ttl > 0 {
				q.Set("ttl", strconv.Itoa(ttl))
			}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/url",
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().IntVar(&ttl, "ttl", 3600, "URL TTL in seconds (60-86400)")
	return cmd
}

func newResourcesWorkflowCmd(app *App) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "workflow [list|step] <workflow-id>",
		Short: "List resources by workflow/step",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				return cmd.Help()
			case 1:
				if err := requireResourcesAPI(cmd, app); err != nil {
					return err
				}
				return runResourcesWorkflowList(cmd, app, args[0], limit)
			default:
				return writeErr(cmd, errors.New("use `breyta resources workflow list <workflow-id>` or `breyta resources workflow step <workflow-id> <step-id>`"))
			}
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results for direct workflow lookup (0 to use server default, 1-100)")
	cmd.AddCommand(newResourcesWorkflowListCmd(app))
	cmd.AddCommand(newResourcesWorkflowStepCmd(app))
	return cmd
}

func newResourcesWorkflowListCmd(app *App) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list <workflow-id>",
		Short: "List all resources for a workflow execution",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResourcesWorkflowList(cmd, app, args[0], limit)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (0 to use server default, 1-100)")
	return cmd
}

func runResourcesWorkflowList(cmd *cobra.Command, app *App, workflowID string, limit int) error {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return writeErr(cmd, errors.New("missing workflow-id"))
	}

	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	out, status, err := apiClient(app).DoREST(
		context.Background(),
		http.MethodGet,
		"/api/resources/workflow/"+url.PathEscape(workflowID),
		q,
		nil,
	)
	if err != nil {
		return writeErr(cmd, err)
	}
	out = compactResourceListPayload(enrichResourceListPayload(out))
	return writeREST(cmd, app, status, out)
}

func newResourcesWorkflowStepCmd(app *App) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "step <workflow-id> <step-id>",
		Short: "List resources for a specific step",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := strings.TrimSpace(args[0])
			stepID := strings.TrimSpace(args[1])
			if workflowID == "" || stepID == "" {
				return writeErr(cmd, errors.New("missing workflow-id or step-id"))
			}

			q := url.Values{}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}

			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/workflow/"+url.PathEscape(workflowID)+"/step/"+url.PathEscape(stepID),
				q,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			out = compactResourceListPayload(enrichResourceListPayload(out))
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results (0 to use server default, 1-100)")
	return cmd
}

func newResourcesTableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "table",
		Short: "Query and mutate table resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newResourcesTableQueryCmd(app))
	cmd.AddCommand(newResourcesTableGetRowCmd(app))
	cmd.AddCommand(newResourcesTableAggregateCmd(app))
	cmd.AddCommand(newResourcesTableSchemaCmd(app))
	cmd.AddCommand(newResourcesTableVerifyCmd(app))
	cmd.AddCommand(newResourcesTableExportCmd(app))
	cmd.AddCommand(newResourcesTableImportCmd(app))
	cmd.AddCommand(newResourcesTableUpdateCellCmd(app))
	cmd.AddCommand(newResourcesTableUpdateCellFormatCmd(app))
	cmd.AddCommand(newResourcesTableSetColumnCmd(app))
	cmd.AddCommand(newResourcesTableRecomputeCmd(app))
	cmd.AddCommand(newResourcesTableMaterializeJoinCmd(app))
	return cmd
}

func newResourcesTableQueryCmd(app *App) *cobra.Command {
	var selectFields string
	var whereJSON string
	var sortJSON string
	var limit int
	var pageMode string
	var offset int
	var cursor string
	var includeTotalCount bool
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "query <uri>",
		Short: "Run a bounded query against a table resource",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			where, err := parseJSONFlag(whereJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --where-json: %w", err))
			}
			sortValue, err := parseJSONFlag(sortJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --sort-json: %w", err))
			}
			body := map[string]any{
				"uri": uri,
			}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			page, err := buildTableQueryPage(cmd, pageMode, limit, offset, cursor, includeTotalCount, sortValue)
			if err != nil {
				return writeErr(cmd, err)
			}
			body["page"] = page
			if selectValue := parseCommaFields(selectFields); len(selectValue) > 0 {
				body["select"] = selectValue
			}
			if where != nil {
				body["where"] = where
			}
			if sortValue != nil {
				body["sort"] = sortValue
			}

			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/query", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status < 400 {
				out = compactTableQueryPayload(out, uri)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&selectFields, "select", "", "Comma-separated projected fields")
	cmd.Flags().StringVar(&whereJSON, "where-json", "", "Raw JSON predicate vector, e.g. [[\"status\",\"=\",\"open\"]]")
	cmd.Flags().StringVar(&sortJSON, "sort-json", "", "Raw JSON sort vector, e.g. [[\"updated-at\",\"desc\"]]")
	cmd.Flags().IntVar(&limit, "limit", 25, "Page size (1-1000)")
	cmd.Flags().StringVar(&pageMode, "page-mode", "", "Pagination mode: offset or cursor (defaults to offset for bounded reads; --cursor implies cursor)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Page offset (>=0)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Opaque pagination cursor for forward scans")
	cmd.Flags().BoolVar(&includeTotalCount, "include-total-count", false, "Include total-count in cursor-paged responses")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Target a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableVerifyCmd(app *App) *cobra.Command {
	var limit int
	var partitionKey string
	var full bool

	cmd := &cobra.Command{
		Use:   "verify <uri>",
		Short: "Verify table metadata and bounded readback",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			if uri == "" {
				return writeErr(cmd, errors.New("missing resource URI"))
			}

			metaQuery := url.Values{}
			metaQuery.Set("uri", uri)
			metadataOut, metadataStatus, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/by-uri",
				metaQuery,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			metadataOutMap := mapStringAny(metadataOut)
			if !restPayloadOK(metadataStatus, metadataOutMap) {
				return writeRESTPayloadFailure(cmd, app, metadataStatus, metadataOut)
			}

			contentQuery := url.Values{}
			contentQuery.Set("uri", uri)
			contentQuery.Set("limit", strconv.Itoa(limit))
			if !full {
				contentQuery.Set("view", "summary")
			}
			if strings.TrimSpace(partitionKey) != "" {
				contentQuery.Set("tablePartition", strings.TrimSpace(partitionKey))
			}
			previewOut, previewStatus, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodGet,
				"/api/resources/content",
				contentQuery,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			previewOutMap := mapStringAny(previewOut)
			if !restPayloadOK(previewStatus, previewOutMap) {
				return writeRESTPayloadFailure(cmd, app, previewStatus, previewOut)
			}

			metadata := restDataPayload(metadataOutMap)
			preview := restDataPayload(previewOutMap)
			verification := map[string]any{
				"uri":              uri,
				"contentType":      firstNonBlankString(metadata["contentType"], metadata["content-type"]),
				"tableName":        firstNonBlankString(preview["table-name"], preview["tableName"], metadata["tableName"], metadata["table-name"]),
				"readback":         true,
				"previewRows":      tableVerifyPreviewRows(preview),
				"rowsWritten":      tableVerifyRowsWritten(preview),
				"selectedTableKey": firstNonBlankString(preview["selected-table-key"], preview["selectedTableKey"]),
			}
			data := map[string]any{
				"verification": verification,
			}
			if full {
				data["metadata"] = metadata
				data["preview"] = preview
			} else {
				data["metadata"] = compactTableVerifyMetadata(uri, metadata, preview)
				data["preview"] = compactTableVerifyPreview(preview, limit, partitionKey)
			}
			out := map[string]any{
				"ok":          true,
				"workspaceId": app.WorkspaceID,
				"meta": map[string]any{
					"nextCommands": []string{
						"breyta resources read " + uri,
						"breyta resources table query " + uri + " --limit " + strconv.Itoa(limit),
						"breyta resources url " + uri,
					},
				},
				"data": data,
			}
			enrichEnvelopeWebLinks(app, out)
			return writeOut(cmd, app, out)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "Readback preview row limit")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Preview a single table partition")
	cmd.Flags().BoolVar(&full, "full", false, "Include raw metadata and preview payloads")
	return cmd
}

func compactTableVerifyMetadata(uri string, metadata map[string]any, preview map[string]any) map[string]any {
	schema := mapStringAny(firstPresentAny(metadata["schema"], preview["schema"]))
	return compactNonEmptyFields(map[string]any{
		"uri":         firstNonBlankString(metadata["uri"], metadata["resourceUri"], metadata["resource-uri"], uri),
		"contentType": firstNonBlankString(metadata["contentType"], metadata["content-type"]),
		"tableName":   firstNonBlankString(preview["table-name"], preview["tableName"], metadata["tableName"], metadata["table-name"]),
		"tableId":     firstNonBlankString(preview["table-id"], preview["tableId"], metadata["tableId"], metadata["table-id"]),
		"rowCount": firstPresentAny(
			metadata["rowCount"],
			metadata["row-count"],
			metadata["rowsWritten"],
			metadata["rows-written"],
			tableVerifyRowsWritten(preview),
		),
		"sizeBytes": firstPresentAny(
			metadata["sizeBytes"],
			metadata["size-bytes"],
			metadata["bytes"],
			metadata["length"],
		),
		"schemaMode": firstNonBlankString(metadata["schemaMode"], metadata["schema-mode"], schema["mode"]),
		"updatedAt":  firstNonBlankString(metadata["updatedAt"], metadata["updated-at"], metadata["createdAt"], metadata["created-at"]),
	})
}

func compactTableVerifyPreview(preview map[string]any, limit int, partitionKey string) map[string]any {
	query := mapStringAny(preview["query"])
	page := mapStringAny(query["page"])
	return compactNonEmptyFields(map[string]any{
		"rowsPreviewed":    tableVerifyPreviewRows(preview),
		"limit":            firstPresentAny(query["limit"], page["limit"], limit),
		"offset":           firstPresentAny(query["offset"], page["offset"]),
		"hasMore":          firstPresentAny(query["hasMore"], query["has-more"], page["hasMore"], page["has-more"]),
		"nextOffset":       firstPresentAny(query["nextOffset"], query["next-offset"], page["nextOffset"], page["next-offset"]),
		"totalCount":       firstPresentAny(query["totalCount"], query["total-count"], page["totalCount"], page["total-count"]),
		"selectedTableKey": firstNonBlankString(preview["selected-table-key"], preview["selectedTableKey"], partitionKey),
	})
}

func compactNonEmptyFields(fields map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range fields {
		if value == nil {
			continue
		}
		if s, ok := value.(string); ok {
			if strings.TrimSpace(s) == "" {
				continue
			}
		}
		out[key] = value
	}
	return out
}

func tableVerifyPreviewRows(preview map[string]any) int {
	query := mapStringAny(preview["query"])
	rows := sliceAny(query["rows"])
	if len(rows) == 0 {
		rows = sliceAny(query["items"])
	}
	if len(rows) > 0 {
		return len(rows)
	}
	if count := asInt(query["count"]); count > 0 {
		return count
	}
	return 0
}

func tableVerifyRowsWritten(preview map[string]any) any {
	query := mapStringAny(preview["query"])
	if page := mapStringAny(query["page"]); page != nil {
		if total := firstPresentAny(page["total-count"], page["totalCount"]); total != nil {
			return total
		}
	}
	if count := firstPresentAny(query["total-count"], query["totalCount"], query["count"]); count != nil {
		return count
	}
	return nil
}

func firstPresentAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func restPayloadOK(status int, out map[string]any) bool {
	if status >= 400 {
		return false
	}
	if _, hasOK := out["ok"]; hasOK {
		return isOK(out)
	}
	return true
}

func writeRESTPayloadFailure(cmd *cobra.Command, app *App, status int, out any) error {
	outMap := mapStringAny(out)
	if status < 400 && outMap != nil {
		if okAny, hasOK := outMap["ok"]; hasOK {
			if okb, ok := okAny.(bool); ok && !okb {
				return writeAPIResult(cmd, app, outMap, status)
			}
		}
	}
	return writeREST(cmd, app, status, out)
}

func restDataPayload(out map[string]any) map[string]any {
	if data := mapStringAny(out["data"]); data != nil {
		return data
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func buildTableQueryPage(cmd *cobra.Command, mode string, limit int, offset int, cursor string, includeTotalCount bool, sortValue any) (map[string]any, error) {
	if !cmd.Flags().Changed("page-mode") {
		if cmd.Flags().Changed("cursor") || cmd.Flags().Changed("include-total-count") {
			mode = "cursor"
		} else {
			mode = "offset"
		}
	}
	page := map[string]any{
		"mode":  strings.TrimSpace(mode),
		"limit": limit,
	}
	switch page["mode"] {
	case "offset":
		if cmd.Flags().Changed("cursor") {
			return nil, errors.New("offset-paged queries cannot use --cursor")
		}
		if cmd.Flags().Changed("include-total-count") {
			return nil, errors.New("offset-paged queries do not accept --include-total-count")
		}
		if cmd.Flags().Changed("offset") {
			page["offset"] = offset
		}
	case "cursor":
		if sortValue == nil {
			return nil, errors.New("cursor-paged queries require --sort-json")
		}
		if cmd.Flags().Changed("offset") {
			return nil, errors.New("cursor-paged queries cannot use --offset")
		}
		if cmd.Flags().Changed("cursor") {
			cur := strings.TrimSpace(cursor)
			if cur == "" {
				return nil, errors.New("invalid --cursor: must be non-empty")
			}
			page["cursor"] = cur
		}
		if cmd.Flags().Changed("include-total-count") {
			page["include-total-count?"] = includeTotalCount
		}
	default:
		return nil, fmt.Errorf("invalid --page-mode %q: use offset or cursor", page["mode"])
	}
	return page, nil
}

func newResourcesTableGetRowCmd(app *App) *cobra.Command {
	var rowID string
	var keyPairs []string
	var partitionKey string

	cmd := &cobra.Command{
		Use:   "get-row <uri>",
		Short: "Fetch a single row from a table resource",
		Example: strings.TrimSpace(`
breyta resources table get-row <res://table-uri> --row-id <row-id>
breyta resources table get-row <res://table-uri> --key order-id=ord-1
breyta resources table get-row <res://table-uri> --key meeting-key=m1 --key agenda-item-number=1`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			keyMap, err := parseKeyAssignments(keyPairs)
			if err != nil {
				return writeErr(cmd, err)
			}
			body := map[string]any{"uri": uri}
			if err := applyTablePartitions(body, partitionKey, ""); err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(rowID) != "" {
				body["row-id"] = strings.TrimSpace(rowID)
			}
			if len(keyMap) > 0 {
				body["key"] = keyMap
			}
			if _, ok := body["row-id"]; !ok && len(keyMap) == 0 {
				return writeErr(cmd, errors.New("get-row requires --row-id or at least one --key field=value"))
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/get-row", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&rowID, "row-id", "", "Stable row id")
	cmd.Flags().StringArrayVar(&keyPairs, "key", nil, "Key field assignment (repeat for composite keys), e.g. --key order-id=ord-1")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	return cmd
}

func newResourcesTableAggregateCmd(app *App) *cobra.Command {
	var whereJSON string
	var groupBy string
	var groupByJSON string
	var metricsJSON string
	var havingJSON string
	var orderByJSON string
	var limit int
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "aggregate <uri>",
		Short: "Run a bounded aggregate against a table resource",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			where, err := parseJSONFlag(whereJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --where-json: %w", err))
			}
			metrics, err := parseJSONFlag(metricsJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --metrics-json: %w", err))
			}
			having, err := parseJSONFlag(havingJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --having-json: %w", err))
			}
			groupBySpec, err := parseJSONFlag(groupByJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --group-by-json: %w", err))
			}
			orderBy, err := parseJSONFlag(orderByJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --order-by-json: %w", err))
			}
			if strings.TrimSpace(groupBy) != "" && groupBySpec != nil {
				return writeErr(cmd, errors.New("use either --group-by or --group-by-json, not both"))
			}
			body := map[string]any{
				"uri":   uri,
				"limit": limit,
			}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			if where != nil {
				body["where"] = where
			}
			if groupBySpec != nil {
				body["group-by"] = groupBySpec
			} else if groupByFields := parseCommaFields(groupBy); len(groupByFields) > 0 {
				body["group-by"] = groupByFields
			}
			if metrics != nil {
				body["metrics"] = metrics
			}
			if having != nil {
				body["having"] = having
			}
			if orderBy != nil {
				body["order-by"] = orderBy
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/aggregate", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&whereJSON, "where-json", "", "Raw JSON predicate vector")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Comma-separated group-by fields")
	cmd.Flags().StringVar(&groupByJSON, "group-by-json", "", "Raw JSON group-by vector, including bucket specs")
	cmd.Flags().StringVar(&metricsJSON, "metrics-json", "", "Raw JSON metric vector")
	cmd.Flags().StringVar(&havingJSON, "having-json", "", "Raw JSON aggregate having predicate vector")
	cmd.Flags().StringVar(&orderByJSON, "order-by-json", "", "Raw JSON aggregate order-by vector")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max aggregate groups")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Target a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableSchemaCmd(app *App) *cobra.Command {
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "schema <uri>",
		Short: "Fetch schema and stats for a table resource",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{"uri": strings.TrimSpace(args[0])}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/schema", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Target a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableExportCmd(app *App) *cobra.Command {
	var selectFields string
	var whereJSON string
	var sortJSON string
	var outPath string
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "export <uri>",
		Short: "Export a table resource as CSV",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			where, err := parseJSONFlag(whereJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --where-json: %w", err))
			}
			sortValue, err := parseJSONFlag(sortJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --sort-json: %w", err))
			}
			body := map[string]any{"uri": uri}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			if selectValue := parseCommaFields(selectFields); len(selectValue) > 0 {
				body["select"] = selectValue
			}
			if where != nil {
				body["where"] = where
			}
			if sortValue != nil {
				body["sort"] = sortValue
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/export", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 {
				return writeREST(cmd, app, status, out)
			}
			csvText, ok := out.(string)
			if !ok {
				return writeErr(cmd, fmt.Errorf("unexpected export response type %T", out))
			}
			if strings.TrimSpace(outPath) == "" || strings.TrimSpace(outPath) == "-" {
				_, err = io.WriteString(cmd.OutOrStdout(), csvText)
				return err
			}
			if err := writePublicFile(outPath, []byte(csvText)); err != nil {
				return writeErr(cmd, err)
			}
			return writeOut(cmd, app, map[string]any{
				"ok":          true,
				"workspaceId": app.WorkspaceID,
				"data": map[string]any{
					"uri":   uri,
					"path":  outPath,
					"bytes": len(csvText),
				},
				"meta": map[string]any{"status": status},
			})
		},
	}

	cmd.Flags().StringVar(&selectFields, "select", "", "Comma-separated projected fields")
	cmd.Flags().StringVar(&whereJSON, "where-json", "", "Raw JSON predicate vector")
	cmd.Flags().StringVar(&sortJSON, "sort-json", "", "Raw JSON sort vector")
	cmd.Flags().StringVar(&outPath, "out", "-", "Write CSV to this file instead of stdout")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Export a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Export a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableImportCmd(app *App) *cobra.Command {
	var filePath string
	var writeMode string
	var partitionKey string
	var partitionKeys string
	var keyFields string
	var indexFields string

	cmd := &cobra.Command{
		Use:   "import <uri-or-table-name>",
		Short: "Import CSV rows into a table resource or named table",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := strings.TrimSpace(args[0])
			if strings.TrimSpace(filePath) == "" {
				return writeErr(cmd, errors.New("import requires --file <path>"))
			}
			if target == "" {
				return writeErr(cmd, errors.New("import requires a table resource URI or table name"))
			}
			var csvText string
			var err error
			if strings.TrimSpace(filePath) == "-" {
				data, readErr := io.ReadAll(cmd.InOrStdin())
				if readErr != nil {
					return writeErr(cmd, readErr)
				}
				csvText = string(data)
			} else {
				csvText, err = readCSVFile(filePath)
				if err != nil {
					return writeErr(cmd, err)
				}
			}
			body := map[string]any{
				"csv":        csvText,
				"write-mode": strings.TrimSpace(writeMode),
			}
			if strings.HasPrefix(target, "res://") {
				body["uri"] = target
			} else {
				body["table"] = target
			}
			if fields := parseCommaFields(keyFields); len(fields) > 0 {
				body["key-fields"] = fields
			}
			if fields := parseCommaFields(indexFields); len(fields) > 0 {
				body["index-fields"] = fields
			}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/import-csv", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&filePath, "file", "", "CSV file to import, or - for stdin")
	cmd.Flags().StringVar(&writeMode, "write-mode", "append", "Import mode: append or upsert (new upsert tables require --key-fields)")
	cmd.Flags().StringVar(&keyFields, "key-fields", "", "Comma-separated key fields to use when creating or upserting into a named table")
	cmd.Flags().StringVar(&indexFields, "index-fields", "", "Comma-separated index fields to apply when creating a named table")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Write into a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Write into a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableUpdateCellCmd(app *App) *cobra.Command {
	var rowID string
	var keyPairs []string
	var column string
	var value string
	var valueJSON string
	var partitionKey string

	cmd := &cobra.Command{
		Use:   "update-cell <uri>",
		Short: "Update a single table cell value",
		Example: strings.TrimSpace(`
breyta resources table update-cell <res://table-uri> --row-id <row-id> --column status --value closed
breyta resources table update-cell <res://table-uri> --key order-id=ord-1 --column status --value closed
breyta resources table update-cell <res://table-uri> --key meeting-key=m1 --key agenda-item-number=1 --column reviewed --value-json true`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			keyMap, err := parseKeyAssignments(keyPairs)
			if err != nil {
				return writeErr(cmd, err)
			}
			body := map[string]any{
				"uri":    uri,
				"column": strings.TrimSpace(column),
			}
			if err := applyTablePartitions(body, partitionKey, ""); err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(column) == "" {
				return writeErr(cmd, errors.New("update-cell requires --column"))
			}
			if strings.TrimSpace(rowID) != "" {
				body["row-id"] = strings.TrimSpace(rowID)
			}
			if len(keyMap) > 0 {
				body["key"] = keyMap
			}
			if _, ok := body["row-id"]; !ok && len(keyMap) == 0 {
				return writeErr(cmd, errors.New("update-cell requires --row-id or at least one --key field=value"))
			}
			switch {
			case cmd.Flags().Changed("value-json"):
				parsed, err := parseJSONFlag(valueJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --value-json: %w", err))
				}
				body["value"] = parsed
			case cmd.Flags().Changed("value"):
				body["value"] = value
			default:
				return writeErr(cmd, errors.New("update-cell requires --value or --value-json"))
			}

			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/update-cell", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&rowID, "row-id", "", "Stable row id")
	cmd.Flags().StringArrayVar(&keyPairs, "key", nil, "Key field assignment (repeat for composite keys), e.g. --key order-id=ord-1")
	cmd.Flags().StringVar(&column, "column", "", "Target column")
	cmd.Flags().StringVar(&value, "value", "", "String cell value")
	cmd.Flags().StringVar(&valueJSON, "value-json", "", "Raw JSON cell value")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	return cmd
}

func newResourcesTableUpdateCellFormatCmd(app *App) *cobra.Command {
	var rowID string
	var keyPairs []string
	var column string
	var formatJSON string
	var clear bool
	var partitionKey string

	cmd := &cobra.Command{
		Use:   "update-cell-format <uri>",
		Short: "Update or clear a sparse table cell formatting override",
		Example: strings.TrimSpace(`
breyta resources table update-cell-format <res://table-uri> --row-id <row-id> --column amount --format-json '{"display":"currency","currency":"USD"}'
breyta resources table update-cell-format <res://table-uri> --key order-id=ord-1 --column amount --format-json '{"display":"currency","currency":"USD"}'
breyta resources table update-cell-format <res://table-uri> --key meeting-key=m1 --key agenda-item-number=1 --column amount --clear`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			keyMap, err := parseKeyAssignments(keyPairs)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(column) == "" {
				return writeErr(cmd, errors.New("update-cell-format requires --column"))
			}
			body := map[string]any{
				"uri":    uri,
				"column": strings.TrimSpace(column),
			}
			if err := applyTablePartitions(body, partitionKey, ""); err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(rowID) != "" {
				body["row-id"] = strings.TrimSpace(rowID)
			}
			if len(keyMap) > 0 {
				body["key"] = keyMap
			}
			if _, ok := body["row-id"]; !ok && len(keyMap) == 0 {
				return writeErr(cmd, errors.New("update-cell-format requires --row-id or at least one --key field=value"))
			}
			if clear {
				body["format"] = nil
			} else {
				parsed, err := parseJSONFlag(formatJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --format-json: %w", err))
				}
				if parsed == nil {
					return writeErr(cmd, errors.New("update-cell-format requires --format-json or --clear"))
				}
				body["format"] = parsed
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/update-cell-format", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&rowID, "row-id", "", "Stable row id")
	cmd.Flags().StringArrayVar(&keyPairs, "key", nil, "Key field assignment (repeat for composite keys), e.g. --key order-id=ord-1")
	cmd.Flags().StringVar(&column, "column", "", "Target column")
	cmd.Flags().StringVar(&formatJSON, "format-json", "", "Raw JSON formatting payload")
	cmd.Flags().BoolVar(&clear, "clear", false, "Clear the sparse formatting override")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Target a single table partition")
	return cmd
}

func newResourcesTableSetColumnCmd(app *App) *cobra.Command {
	var column string
	var displayName string
	var typeHint string
	var semanticType string
	var enumJSON string
	var computedJSON string
	var referenceJSON string
	var formatJSON string
	var validationJSON string
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "set-column <uri>",
		Short: "Create or update one logical table column definition",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			columnName := strings.TrimSpace(column)
			if columnName == "" {
				return writeErr(cmd, errors.New("set-column requires --column"))
			}
			definition := map[string]any{}
			if cmd.Flags().Changed("display-name") {
				definition["display-name"] = strings.TrimSpace(displayName)
			}
			if cmd.Flags().Changed("type-hint") {
				definition["type-hint"] = strings.TrimSpace(typeHint)
			}
			if cmd.Flags().Changed("semantic-type") {
				definition["semantic-type"] = strings.TrimSpace(semanticType)
			}
			if cmd.Flags().Changed("enum-json") {
				parsed, err := parseJSONFlag(enumJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --enum-json: %w", err))
				}
				definition["enum"] = parsed
			}
			if cmd.Flags().Changed("computed-json") {
				parsed, err := parseJSONFlag(computedJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --computed-json: %w", err))
				}
				definition["computed"] = parsed
			}
			if cmd.Flags().Changed("reference-json") {
				parsed, err := parseJSONFlag(referenceJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --reference-json: %w", err))
				}
				definition["reference"] = parsed
			}
			if cmd.Flags().Changed("format-json") {
				parsed, err := parseJSONFlag(formatJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --format-json: %w", err))
				}
				definition["format"] = parsed
			}
			if cmd.Flags().Changed("validation-json") {
				parsed, err := parseJSONFlag(validationJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --validation-json: %w", err))
				}
				definition["validation"] = parsed
			}
			body := map[string]any{
				"uri":        uri,
				"column":     columnName,
				"definition": definition,
			}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/set-column", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&column, "column", "", "Column name to create/update")
	cmd.Flags().StringVar(&displayName, "display-name", "", "Optional display label")
	cmd.Flags().StringVar(&typeHint, "type-hint", "", "Optional storage/query type hint")
	cmd.Flags().StringVar(&semanticType, "semantic-type", "", "Optional semantic type, e.g. currency, url, reference")
	cmd.Flags().StringVar(&enumJSON, "enum-json", "", "Raw JSON dynamic enum definition, e.g. {\"options\":[{\"id\":\"open\",\"name\":\"Open\"}]}")
	cmd.Flags().StringVar(&computedJSON, "computed-json", "", "Raw JSON computed-column definition")
	cmd.Flags().StringVar(&referenceJSON, "reference-json", "", "Raw JSON same-workspace reference definition")
	cmd.Flags().StringVar(&formatJSON, "format-json", "", "Raw JSON default formatting metadata")
	cmd.Flags().StringVar(&validationJSON, "validation-json", "", "Raw JSON validation metadata")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Apply to a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Apply to a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableRecomputeCmd(app *App) *cobra.Command {
	var whereJSON string
	var limit int
	var offset int
	var partitionKey string
	var partitionKeys string

	cmd := &cobra.Command{
		Use:   "recompute <uri>",
		Short: "Recompute materialized computed/reference columns for existing rows",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			uri := strings.TrimSpace(args[0])
			where, err := parseJSONFlag(whereJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --where-json: %w", err))
			}
			body := map[string]any{
				"uri":    uri,
				"limit":  limit,
				"offset": offset,
			}
			if err := applyTablePartitions(body, partitionKey, partitionKeys); err != nil {
				return writeErr(cmd, err)
			}
			if where != nil {
				body["where"] = where
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/recompute", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&whereJSON, "where-json", "", "Optional raw JSON predicate vector to limit recompute scope")
	cmd.Flags().IntVar(&limit, "limit", 1000, "Max rows to recompute in this request (1-1000)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Row offset for recompute windows")
	cmd.Flags().StringVar(&partitionKey, "partition-key", "", "Recompute a single table partition")
	cmd.Flags().StringVar(&partitionKeys, "partition-keys", "", "Recompute a comma-separated subset of table partitions")
	return cmd
}

func newResourcesTableMaterializeJoinCmd(app *App) *cobra.Command {
	var leftJSON string
	var rightJSON string
	var onJSON string
	var projectJSON string
	var intoJSON string
	var joinType string
	var opID string

	cmd := &cobra.Command{
		Use:   "materialize-join",
		Short: "Materialize a bounded join into a destination table resource",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireResourcesAPI(cmd, app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			left, err := parseJSONFlag(leftJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --left-json: %w", err))
			}
			right, err := parseJSONFlag(rightJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --right-json: %w", err))
			}
			on, err := parseJSONFlag(onJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --on-json: %w", err))
			}
			project, err := parseJSONFlag(projectJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --project-json: %w", err))
			}
			into, err := parseJSONFlag(intoJSON)
			if err != nil {
				return writeErr(cmd, fmt.Errorf("invalid --into-json: %w", err))
			}
			if left == nil {
				return writeErr(cmd, errors.New("materialize-join requires --left-json"))
			}
			if right == nil {
				return writeErr(cmd, errors.New("materialize-join requires --right-json"))
			}
			if on == nil {
				return writeErr(cmd, errors.New("materialize-join requires --on-json"))
			}
			if into == nil {
				return writeErr(cmd, errors.New("materialize-join requires --into-json"))
			}
			body := map[string]any{
				"left":  left,
				"right": right,
				"on":    on,
				"into":  into,
			}
			if project != nil {
				body["project"] = project
			}
			if strings.TrimSpace(joinType) != "" {
				body["join-type"] = strings.TrimSpace(joinType)
			}
			if strings.TrimSpace(opID) != "" {
				body["op-id"] = strings.TrimSpace(opID)
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/resources/table/materialize-join", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&leftJSON, "left-json", "", "Raw JSON left-source definition, e.g. {\"table\":{\"ref\":\"res://...\"}} or {\"rows\":[...]}")
	cmd.Flags().StringVar(&rightJSON, "right-json", "", "Raw JSON right-source definition, e.g. {\"table\":{\"ref\":\"res://...\"},\"select\":[...]}")
	cmd.Flags().StringVar(&onJSON, "on-json", "", "Raw JSON join key vector, e.g. [{\"left-field\":\"customer-id\",\"right-field\":\"customer-id\"}]")
	cmd.Flags().StringVar(&projectJSON, "project-json", "", "Raw JSON projection config, e.g. {\"keep-left\":\"all\",\"right-fields\":[...]}")
	cmd.Flags().StringVar(&intoJSON, "into-json", "", "Raw JSON destination config, e.g. {\"table\":\"joined-orders\",\"write-mode\":\"upsert\",\"key-fields\":[\"order-id\"]}")
	cmd.Flags().StringVar(&joinType, "join-type", "left", "Join type: left or inner")
	cmd.Flags().StringVar(&opID, "op-id", "", "Optional idempotency key for the materialized join write")
	return cmd
}
