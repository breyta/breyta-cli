package cli

import (
        "context"
        "encoding/json"
        "errors"
        "net/http"
        "net/url"
        "strconv"
        "strings"

        "breyta-cli/internal/api"

        "github.com/spf13/cobra"
)

func newWaitsAPICmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "waits", Short: "Manage waits"}
        cmd.AddCommand(newWaitsAPIListCmd(app))
        cmd.AddCommand(newWaitsAPIShowCmd(app))
        cmd.AddCommand(newWaitsAPIApproveCmd(app))
        cmd.AddCommand(newWaitsAPIRejectCmd(app))
        cmd.AddCommand(newWaitsAPICompleteCmd(app))
        cmd.AddCommand(newWaitsAPICancelCmd(app))
        cmd.AddCommand(newWaitsAPIActionCmd(app))
        return cmd
}

func apiREST(app *App) api.Client {
        return apiClient(app)
}

func writeREST(cmd *cobra.Command, app *App, status int, data any) error {
        // Normalize REST endpoints into the stable CLI envelope shape.
        ok := status < 400
        if !ok {
                // If server returned a structured error map, preserve it in "error".
                if m, _ := data.(map[string]any); m != nil {
                        if errAny, exists := m["error"]; exists {
                                _ = writeOut(cmd, app, map[string]any{
                                        "ok":          false,
                                        "workspaceId": app.WorkspaceID,
                                        "error":       errAny,
                                        "data":        m,
                                })
                                return errors.New("api error")
                        }
                }
                _ = writeOut(cmd, app, map[string]any{
                        "ok":          false,
                        "workspaceId": app.WorkspaceID,
                        "data":        data,
                        "meta":        map[string]any{"status": status},
                })
                return errors.New("api error")
        }
        return writeOut(cmd, app, map[string]any{
                "ok":          true,
                "workspaceId": app.WorkspaceID,
                "data":        data,
                "meta":        map[string]any{"status": status},
        })
}

func newWaitsAPIListCmd(app *App) *cobra.Command {
        var workflowID string
        var flowSlug string
        var showsInUIOnly bool
        var limit int

        cmd := &cobra.Command{
                Use:   "list",
                Short: "List active waits",
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        q := url.Values{}
                        if strings.TrimSpace(workflowID) != "" {
                                q.Set("workflowId", strings.TrimSpace(workflowID))
                        }
                        if strings.TrimSpace(flowSlug) != "" {
                                q.Set("flowSlug", strings.TrimSpace(flowSlug))
                        }
                        if showsInUIOnly {
                                q.Set("showsInUiOnly", "true")
                        }
                        if limit > 0 {
                                q.Set("limit", strconv.Itoa(limit))
                        }
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodGet, "/api/waits", q, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        cmd.Flags().StringVar(&workflowID, "workflow-id", "", "Filter by workflow id")
        cmd.Flags().StringVar(&flowSlug, "flow", "", "Filter by flow slug")
        cmd.Flags().BoolVar(&showsInUIOnly, "shows-in-ui-only", false, "Only waits with UI notify config")
        cmd.Flags().IntVar(&limit, "limit", 50, "Max results (1-200)")
        return cmd
}

func newWaitsAPIShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <wait-id>",
                Short: "Show a wait by wait-id",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        if waitID == "" {
                                return writeErr(cmd, errors.New("missing wait-id"))
                        }
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodGet, "/api/waits/"+url.PathEscape(waitID), nil, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        return cmd
}

func newWaitsAPIApproveCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "approve <wait-id>",
                Short: "Approve a wait",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/approve", nil, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        return cmd
}

func newWaitsAPIRejectCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "reject <wait-id>",
                Short: "Reject a wait",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/reject", nil, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        return cmd
}

func newWaitsAPICompleteCmd(app *App) *cobra.Command {
        var payloadJSON string
        cmd := &cobra.Command{
                Use:   "complete <wait-id>",
                Short: "Complete a wait with a custom JSON payload",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        var payload any = map[string]any{}
                        if strings.TrimSpace(payloadJSON) != "" {
                                if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
                                        return writeErr(cmd, errors.New("invalid --payload JSON"))
                                }
                        }
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/complete", nil, payload)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        cmd.Flags().StringVar(&payloadJSON, "payload", "", "JSON payload to submit (default: {})")
        return cmd
}

func newWaitsAPICancelCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "cancel <wait-id>",
                Short: "Cancel/dismiss a wait",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodDelete, "/api/waits/"+url.PathEscape(waitID), nil, nil)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        return cmd
}

func newWaitsAPIActionCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "action <wait-id> <action-name>",
                Short: "Invoke a wait action (dynamic actions from UI config)",
                Args:  cobra.ExactArgs(2),
                RunE: func(cmd *cobra.Command, args []string) error {
                        if err := requireAPI(app); err != nil {
                                return writeErr(cmd, err)
                        }
                        waitID := strings.TrimSpace(args[0])
                        action := strings.TrimSpace(args[1])
                        if waitID == "" || action == "" {
                                return writeErr(cmd, errors.New("missing wait-id or action-name"))
                        }
                        out, status, err := apiREST(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/action/"+url.PathEscape(action), nil, map[string]any{})
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeREST(cmd, app, status, out)
                },
        }
        return cmd
}
