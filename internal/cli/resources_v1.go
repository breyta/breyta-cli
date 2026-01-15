package cli

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

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
  GET /<workspace>/api/resources/by-uri?uri=...   - Get resource metadata
  GET /<workspace>/api/resources/content?uri=...  - Read resource content
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
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Respect explicit `--api=` forcing mock mode.
			if apiFlagExplicit(cmd) && strings.TrimSpace(app.APIURL) == "" {
				return errors.New("resources requires API mode (set BREYTA_API_URL)")
			}
			return requireAPI(app)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newResourcesListCmd(app))
	cmd.AddCommand(newResourcesGetCmd(app))
	cmd.AddCommand(newResourcesReadCmd(app))
	cmd.AddCommand(newResourcesURLCmd(app))
	cmd.AddCommand(newResourcesWorkflowCmd(app))
	return cmd
}

func newResourcesListCmd(app *App) *cobra.Command {
	var typeFilter string
	var prefix string
	var tags string
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources in workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if typeFilter != "" {
				q.Set("type", typeFilter)
			}
			if prefix != "" {
				q.Set("prefix", prefix)
			}
			if tags != "" {
				q.Set("tags", tags)
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
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by resource type (result, import, file, bundle, external-dir)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Filter by URI prefix")
	cmd.Flags().StringVar(&tags, "tags", "", "Filter by tags (comma-separated)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max results (1-100)")
	return cmd
}

func newResourcesGetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <uri>",
		Short: "Get resource metadata by URI",
		Args:  cobra.ExactArgs(1),
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
	cmd := &cobra.Command{
		Use:   "read <uri>",
		Short: "Read resource content by URI",
		Args:  cobra.ExactArgs(1),
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
				"/api/resources/content",
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

func newResourcesURLCmd(app *App) *cobra.Command {
	var ttl int

	cmd := &cobra.Command{
		Use:   "url <uri>",
		Short: "Get signed URL for resource access",
		Args:  cobra.ExactArgs(1),
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
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "List resources by workflow/step",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

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
		RunE: func(cmd *cobra.Command, args []string) error {
			workflowID := strings.TrimSpace(args[0])
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
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 25, "Max results (1-100)")
	return cmd
}

func newResourcesWorkflowStepCmd(app *App) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "step <workflow-id> <step-id>",
		Short: "List resources for a specific step",
		Args:  cobra.ExactArgs(2),
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
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 25, "Max results (1-100)")
	return cmd
}
