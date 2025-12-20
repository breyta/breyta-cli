package cli

import (
        "context"
        "encoding/json"
        "errors"
        "io"
        "net/http"
        "net/url"
        "strconv"
        "strings"
        "time"

        "github.com/spf13/cobra"
)

func newArtifactsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "artifacts",
                Short: "Inspect step artifacts for a run",
                Long: strings.TrimSpace(`
Artifacts are full-fidelity per-step inputs/outputs stored by flows-api.

API routes:
  GET /<workspace>/api/executions/<workflow-id>/steps/<step-id>/artifact
  GET /<workspace>/api/executions/<workflow-id>/steps/<step-id>/artifact/url

Notes:
- Useful for agents to debug and to fetch results that were returned as
  {:type :artifact-ref ...} when a step uses :expect :ref.
`),
                PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
                        if !isAPIMode(app) {
                                return errors.New("artifacts requires API mode (set BREYTA_API_URL)")
                        }
                        return requireAPI(app)
                },
                RunE: func(cmd *cobra.Command, args []string) error {
                        return cmd.Help()
                },
        }

        cmd.AddCommand(newArtifactsShowCmd(app))
        cmd.AddCommand(newArtifactsURLCmd(app))
        cmd.AddCommand(newArtifactsFetchCmd(app))
        return cmd
}

func newArtifactsShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <workflow-id> <step-id>",
                Short: "Show artifact metadata (and inline meta)",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        workflowID := strings.TrimSpace(args[0])
                        stepID := strings.TrimSpace(args[1])
                        if workflowID == "" || stepID == "" {
                                return writeErr(cmd, errors.New("missing workflow-id or step-id"))
                        }
                        out, status, err := apiClient(app).DoREST(
                                context.Background(),
                                http.MethodGet,
                                "/api/executions/"+url.PathEscape(workflowID)+"/steps/"+url.PathEscape(stepID)+"/artifact",
                                nil,
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

func newArtifactsURLCmd(app *App) *cobra.Command {
        var ttl int
        cmd := &cobra.Command{
                Use:   "url <workflow-id> <step-id>",
                Short: "Get a signed/proxy URL for the artifact",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        workflowID := strings.TrimSpace(args[0])
                        stepID := strings.TrimSpace(args[1])
                        if workflowID == "" || stepID == "" {
                                return writeErr(cmd, errors.New("missing workflow-id or step-id"))
                        }
                        q := url.Values{}
                        if ttl > 0 {
                                q.Set("ttl", strconv.Itoa(ttl))
                        }
                        out, status, err := apiClient(app).DoREST(
                                context.Background(),
                                http.MethodGet,
                                "/api/executions/"+url.PathEscape(workflowID)+"/steps/"+url.PathEscape(stepID)+"/artifact/url",
                                q,
                                nil,
                        )
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        cmd.Flags().IntVar(&ttl, "ttl", 3600, "URL TTL in seconds (server clamps 60..86400)")
        return cmd
}

func newArtifactsFetchCmd(app *App) *cobra.Command {
        var ttl int
        cmd := &cobra.Command{
                Use:   "fetch <workflow-id> <step-id>",
                Short: "Fetch the artifact content (dev proxy)",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        workflowID := strings.TrimSpace(args[0])
                        stepID := strings.TrimSpace(args[1])
                        if workflowID == "" || stepID == "" {
                                return writeErr(cmd, errors.New("missing workflow-id or step-id"))
                        }

                        // Step 1: get a URL from the server (may be a signed URL in prod, or /api/artifacts/<path> in dev).
                        q := url.Values{}
                        if ttl > 0 {
                                q.Set("ttl", strconv.Itoa(ttl))
                        }
                        urlOut, urlStatus, err := apiClient(app).DoREST(
                                context.Background(),
                                http.MethodGet,
                                "/api/executions/"+url.PathEscape(workflowID)+"/steps/"+url.PathEscape(stepID)+"/artifact/url",
                                q,
                                nil,
                        )
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if urlStatus >= 400 {
                                return writeREST(cmd, app, urlStatus, urlOut)
                        }

                        m, ok := urlOut.(map[string]any)
                        if !ok {
                                return writeErr(cmd, errors.New("unexpected artifact/url response shape"))
                        }
                        u, _ := m["url"].(string)
                        if strings.TrimSpace(u) == "" {
                                return writeErr(cmd, errors.New("artifact url missing in response"))
                        }

                        // Step 2: only support the dev proxy URL (relative /api/artifacts/*) for now.
                        // If we got a full https://... signed URL, we avoid implementing a generic fetcher here.
                        u = strings.TrimSpace(u)
                        if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
                                return writeREST(cmd, app, 200, map[string]any{
                                        "url":  u,
                                        "hint": "Received a signed URL. Fetch it with your HTTP client (CLI fetch not implemented for arbitrary URLs).",
                                })
                        }
                        if !strings.HasPrefix(u, "/") {
                                return writeErr(cmd, errors.New("unexpected artifact url format (expected /api/artifacts/...)"))
                        }

                        // /api/artifacts/* is NOT workspace-scoped in flows-api.
                        // Our generic DoREST helper always prefixes "/<workspace>/" so it would break here.
                        // Fetch it manually against the base URL.
                        var (
                                contentOut    any
                                contentStatus int
                        )
                        if strings.HasPrefix(u, "/api/artifacts/") {
                                httpClient := &http.Client{Timeout: 30 * time.Second}
                                req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL(app)+u, nil)
                                if reqErr != nil {
                                        return writeErr(cmd, reqErr)
                                }
                                if strings.TrimSpace(app.Token) != "" {
                                        req.Header.Set("Authorization", "Bearer "+app.Token)
                                }
                                resp, doErr := httpClient.Do(req)
                                if doErr != nil {
                                        return writeErr(cmd, doErr)
                                }
                                defer resp.Body.Close()
                                b, readErr := io.ReadAll(resp.Body)
                                if readErr != nil {
                                        return writeErr(cmd, readErr)
                                }
                                contentStatus = resp.StatusCode
                                if jsonErr := json.Unmarshal(b, &contentOut); jsonErr != nil {
                                        contentOut = string(b)
                                }
                        } else {
                                contentOut, contentStatus, err = apiClient(app).DoREST(
                                        context.Background(),
                                        http.MethodGet,
                                        u,
                                        nil,
                                        nil,
                                )
                        }
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, contentStatus, contentOut)
                },
        }
        cmd.Flags().IntVar(&ttl, "ttl", 3600, "URL TTL in seconds (server clamps 60..86400)")
        return cmd
}
