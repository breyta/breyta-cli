package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

func newConnectionsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "connections", Short: "Manage connections"}
	cmd.AddCommand(newConnectionsListCmd(app))
	cmd.AddCommand(newConnectionsShowCmd(app))
	cmd.AddCommand(newConnectionsCreateCmd(app))
	cmd.AddCommand(newConnectionsUpdateCmd(app))
	cmd.AddCommand(newConnectionsDeleteCmd(app))
	cmd.AddCommand(newConnectionsTestCmd(app))
	return cmd
}

func newConnectionsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connections",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]*state.Connection, 0)
			for _, c := range ws.Connections {
				items = append(items, c)
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	return cmd
}

func newConnectionsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			c := ws.Connections[args[0]]
			if c == nil {
				return writeErr(cmd, errors.New("connection not found"))
			}
			return writeData(cmd, app, nil, map[string]any{"connection": c})
		},
	}
	return cmd
}

func newConnectionsCreateCmd(app *App) *cobra.Command {
	var name, typ string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			if typ == "" {
				return writeErr(cmd, errors.New("missing --type"))
			}
			if name == "" {
				name = typ
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws.Connections == nil {
				ws.Connections = map[string]*state.Connection{}
			}
			id := "conn-" + time.Now().UTC().Format("20060102-150405")
			now := time.Now().UTC()
			c := &state.Connection{ID: id, Name: name, Type: typ, Status: "ready", UpdatedAt: now}
			ws.Connections[id] = c
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"connection": c})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Connection name")
	cmd.Flags().StringVar(&typ, "type", "", "Connection type")
	return cmd
}

func newConnectionsUpdateCmd(app *App) *cobra.Command {
	var name, status string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			c := ws.Connections[args[0]]
			if c == nil {
				return writeErr(cmd, errors.New("connection not found"))
			}
			if name != "" {
				c.Name = name
			}
			if status != "" {
				c.Status = status
			}
			c.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = c.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"connection": c})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.Flags().StringVar(&status, "status", "", "Status")
	return cmd
}

func newConnectionsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws.Connections[args[0]] == nil {
				return writeErr(cmd, errors.New("connection not found"))
			}
			delete(ws.Connections, args[0])
			ws.UpdatedAt = time.Now().UTC()
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"deleted": true, "id": args[0]})
		},
	}
	return cmd
}

func newConnectionsTestCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Test connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			c := ws.Connections[args[0]]
			if c == nil {
				return writeErr(cmd, errors.New("connection not found"))
			}
			now := time.Now().UTC()
			c.Status = "ready"
			c.UpdatedAt = now
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"id": c.ID, "ok": true, "status": c.Status})
		},
	}
	return cmd
}

// --- Profiles --------------------------------------------------------------

func newProfilesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "profiles", Short: "Manage flow profiles (activation/bindings)"}
	cmd.AddCommand(newProfilesListCmd(app))
	cmd.AddCommand(newProfilesShowCmd(app))
	cmd.AddCommand(newProfilesCreateCmd(app))
	cmd.AddCommand(newProfilesDeleteCmd(app))

	bindings := &cobra.Command{Use: "bindings", Short: "Manage profile bindings"}
	bindings.AddCommand(newProfilesBindingsSetCmd(app))
	cmd.AddCommand(bindings)

	return cmd
}

func newProfilesListCmd(app *App) *cobra.Command {
	var flow string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]*state.Profile, 0)
			for _, it := range ws.Profiles {
				if flow == "" || it.FlowSlug == flow {
					items = append(items, it)
				}
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
	return cmd
}

func newProfilesShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			it := ws.Profiles[args[0]]
			if it == nil {
				return writeErr(cmd, errors.New("profile not found"))
			}
			return writeData(cmd, app, nil, map[string]any{"profile": it})
		},
	}
	return cmd
}

func newProfilesCreateCmd(app *App) *cobra.Command {
	var flow, name string
	var version int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flow == "" {
				return writeErr(cmd, errors.New("missing --flow"))
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			f := ws.Flows[flow]
			if f == nil {
				return writeErr(cmd, errors.New("flow not found"))
			}
			if version == 0 {
				version = f.ActiveVersion
			}
			if ws.Profiles == nil {
				ws.Profiles = map[string]*state.Profile{}
			}
			id := "prof-" + time.Now().UTC().Format("20060102-150405")
			if name == "" {
				name = flow
			}
			now := time.Now().UTC()
			it := &state.Profile{ID: id, FlowSlug: flow, Version: version, Name: name, Enabled: true, ProfileType: "prod", UpdatedAt: now}
			ws.Profiles[id] = it
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"profile": it})
		},
	}
	cmd.Flags().StringVar(&flow, "flow", "", "Flow slug")
	cmd.Flags().IntVar(&version, "version", 0, "Version")
	cmd.Flags().StringVar(&name, "name", "", "Profile name")
	return cmd
}

func newProfilesBindingsSetCmd(app *App) *cobra.Command {
	var bindings string
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set profile bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			it := ws.Profiles[args[0]]
			if it == nil {
				return writeErr(cmd, errors.New("profile not found"))
			}
			it.Bindings = map[string]any{"raw": bindings}
			it.UpdatedAt = time.Now().UTC()
			ws.UpdatedAt = it.UpdatedAt
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"profile": it})
		},
	}
	cmd.Flags().StringVar(&bindings, "bindings", "", "Bindings (mock string)")
	return cmd
}

func newProfilesDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws.Profiles[args[0]] == nil {
				return writeErr(cmd, errors.New("profile not found"))
			}
			delete(ws.Profiles, args[0])
			ws.UpdatedAt = time.Now().UTC()
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"deleted": true, "id": args[0]})
		},
	}
	return cmd
}

// --- Triggers ----------------------------------------------------------------

func newTriggersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "triggers", Short: "Manage triggers"}
	cmd.AddCommand(newTriggersListCmd(app))
	cmd.AddCommand(newTriggersFireCmd(app))
	return cmd
}

func newTriggersListCmd(app *App) *cobra.Command {
	var flow string
	var triggerType string
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List triggers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				q := url.Values{}
				if strings.TrimSpace(triggerType) != "" {
					q.Set("type", strings.TrimSpace(triggerType))
				}
				if strings.TrimSpace(cursor) != "" {
					q.Set("cursor", strings.TrimSpace(cursor))
				}
				if limit > 0 {
					q.Set("limit", strconv.Itoa(limit))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/triggers", q, nil)
				if err != nil {
					return writeErr(cmd, err)
				}
				return writeREST(cmd, app, status, out)
			}
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]*state.Trigger, 0)
			for _, t := range ws.Triggers {
				if flow == "" || t.FlowSlug == flow {
					items = append(items, t)
				}
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug (mock mode only)")
	cmd.Flags().StringVar(&triggerType, "type", "", "Filter by trigger type (event|schedule|manual|webhook) (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per page (API mode only)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	return cmd
}

func newTriggersFireCmd(app *App) *cobra.Command {
	var payload string
	cmd := &cobra.Command{
		Use:   "fire <trigger-id>",
		Short: "Fire a trigger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				body := map[string]any{}
				if strings.TrimSpace(payload) != "" {
					var v any
					if err := json.Unmarshal([]byte(payload), &v); err != nil {
						return writeErr(cmd, errors.New("invalid --payload JSON"))
					}
					m, ok := v.(map[string]any)
					if !ok {
						return writeErr(cmd, errors.New("--payload must be a JSON object"))
					}
					body = m
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/triggers/"+url.PathEscape(args[0])+"/fire", nil, body)
				if err != nil {
					return writeErr(cmd, err)
				}
				return writeREST(cmd, app, status, out)
			}
			st, store, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			t := ws.Triggers[args[0]]
			if t == nil {
				return writeErr(cmd, errors.New("trigger not found"))
			}
			run, err := store.StartRun(st, t.FlowSlug, 0)
			if err != nil {
				return writeErr(cmd, err)
			}
			run.TriggeredBy = "trigger:" + t.ID
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"trigger": t, "run": run})
		},
	}
	cmd.Flags().StringVar(&payload, "payload", "", "JSON object payload to send as trigger input (API mode only)")
	return cmd
}

// --- Waits -------------------------------------------------------------------

func newWaitsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "waits", Short: "Manage waits"}
	cmd.AddCommand(newWaitsListCmd(app))
	cmd.AddCommand(newWaitsShowCmd(app))
	cmd.AddCommand(newWaitsApproveCmd(app))
	cmd.AddCommand(newWaitsRejectCmd(app))
	cmd.AddCommand(newWaitsCompleteCmd(app))
	cmd.AddCommand(newWaitsCancelCmd(app))
	cmd.AddCommand(newWaitsActionCmd(app))
	return cmd
}

func newWaitsListCmd(app *App) *cobra.Command {
	var runID string
	var workflowID string
	var flowSlug string
	var showsInUIOnly bool
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List waits",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
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
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/waits", q, nil)
				if err != nil {
					return writeErr(cmd, err)
				}
				return writeREST(cmd, app, status, out)
			}
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]*state.Wait, 0)
			for _, w := range ws.Waits {
				if runID == "" || w.RunID == runID {
					items = append(items, w)
				}
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().StringVar(&runID, "run", "", "Filter by run id")
	cmd.Flags().StringVar(&workflowID, "workflow-id", "", "Filter by workflow id (API mode)")
	cmd.Flags().StringVar(&flowSlug, "flow", "", "Filter by flow slug (API mode)")
	cmd.Flags().BoolVar(&showsInUIOnly, "shows-in-ui-only", false, "Only waits with UI notify config (API mode)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results (API mode)")
	return cmd
}

func newWaitsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <wait-id>", Short: "Show wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			// Prefer dedicated endpoint when present.
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/waits/"+url.PathEscape(waitID), nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			// Backwards-compat fallback: if server doesn't yet have GET /api/waits/:wait-id, try list and filter.
			if status == 404 {
				q := url.Values{}
				q.Set("limit", "200")
				listOut, listStatus, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/waits", q, nil)
				if err != nil {
					return writeErr(cmd, err)
				}
				if listStatus >= 400 {
					return writeREST(cmd, app, listStatus, listOut)
				}
				// Expect listOut to be a map with "items".
				if m, ok := listOut.(map[string]any); ok {
					if itemsAny, ok := m["items"].([]any); ok {
						for _, it := range itemsAny {
							if w, ok := it.(map[string]any); ok {
								if id, _ := w["waitId"].(string); id == waitID {
									return writeREST(cmd, app, 200, map[string]any{"wait": w})
								}
							}
						}
					}
				}
			}
			return writeREST(cmd, app, status, out)
		}
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		w := ws.Waits[args[0]]
		if w == nil {
			return writeErr(cmd, errors.New("wait not found"))
		}
		return writeData(cmd, app, nil, map[string]any{"wait": w})
	}}
	return cmd
}

func newWaitsCompleteCmd(app *App) *cobra.Command {
	var payload string
	cmd := &cobra.Command{Use: "complete <wait-id>", Short: "Complete wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			// Payload is JSON in API mode (default {}).
			body := map[string]any{}
			if strings.TrimSpace(payload) != "" {
				var v any
				if err := json.Unmarshal([]byte(payload), &v); err != nil {
					return writeErr(cmd, errors.New("invalid --payload JSON"))
				}
				if m, ok := v.(map[string]any); ok {
					body = m
				} else {
					return writeErr(cmd, errors.New("--payload must be a JSON object"))
				}
			}
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/complete", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		}
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		w := ws.Waits[args[0]]
		if w == nil {
			return writeErr(cmd, errors.New("wait not found"))
		}
		w.Status = "completed"
		w.Payload = map[string]any{"raw": payload}
		ws.UpdatedAt = time.Now().UTC()
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		return writeData(cmd, app, nil, map[string]any{"wait": w})
	}}
	cmd.Flags().StringVar(&payload, "payload", "", "Payload (mock: string; API mode: JSON object)")
	return cmd
}

func newWaitsCancelCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "cancel <wait-id>", Short: "Cancel wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodDelete, "/api/waits/"+url.PathEscape(waitID), nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		}
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		w := ws.Waits[args[0]]
		if w == nil {
			return writeErr(cmd, errors.New("wait not found"))
		}
		w.Status = "cancelled"
		ws.UpdatedAt = time.Now().UTC()
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		return writeData(cmd, app, nil, map[string]any{"wait": w})
	}}
	return cmd
}

func newWaitsApproveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "approve <wait-id>", Short: "Approve wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/approve", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		}
		return writeNotImplemented(cmd, app, "Mock-only waits; approvals are only meaningful in API mode")
	}}
	return cmd
}

func newWaitsRejectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "reject <wait-id>", Short: "Reject wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/reject", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		}
		return writeNotImplemented(cmd, app, "Mock-only waits; rejections are only meaningful in API mode")
	}}
	return cmd
}

func newWaitsActionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "action <wait-id> <action-name>", Short: "Perform a wait action", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			waitID := strings.TrimSpace(args[0])
			action := strings.TrimSpace(args[1])
			out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/waits/"+url.PathEscape(waitID)+"/action/"+url.PathEscape(action), nil, map[string]any{})
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		}
		return writeNotImplemented(cmd, app, "Mock-only waits; actions are only meaningful in API mode")
	}}
	return cmd
}

// --- Watch/Registry/Auth/Workspaces (mock placeholders) ----------------------

func newWatchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "watch", Short: "Stream live updates"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return writeNotImplemented(cmd, app, "Mock: watch will tail SSE/event streams")
	}
	return cmd
}

func newRegistryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Marketplace registry (mock)"}
	cmd.AddCommand(newRegistrySearchCmd(app))
	cmd.AddCommand(newRegistryShowCmd(app))
	cmd.AddCommand(newRegistryPublishCmd(app))
	cmd.AddCommand(newRegistryVersionsCmd(app))
	cmd.AddCommand(newRegistryMatchCmd(app))
	cmd.AddCommand(newRegistryInstallCmd(app))
	return cmd
}

func newRegistrySearchCmd(app *App) *cobra.Command {
	var limit int
	cmd := &cobra.Command{Use: "search <query>", Short: "Search registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		q := strings.ToLower(strings.TrimSpace(args[0]))
		if q == "" {
			return writeErr(cmd, errors.New("empty query"))
		}
		type scored struct {
			e      *state.RegistryEntry
			score  float64
			reason string
		}
		scoredItems := make([]scored, 0, len(ws.Registry))
		for _, e := range ws.Registry {
			hay := strings.ToLower(e.Slug + " " + e.Title + " " + e.Summary + " " + strings.Join(e.Tags, " "))
			score := 0.0
			reason := ""
			if strings.Contains(hay, q) {
				score = 0.95
				reason = "exact substring match"
			} else {
				// cheap token overlap score
				toks := strings.Fields(q)
				hit := 0
				for _, t := range toks {
					if t != "" && strings.Contains(hay, t) {
						hit++
					}
				}
				if len(toks) > 0 {
					score = float64(hit) / float64(len(toks))
				}
				if score > 0 {
					reason = "token overlap"
				}
			}
			if score > 0 {
				scoredItems = append(scoredItems, scored{e: e, score: score, reason: reason})
			}
		}
		sort.Slice(scoredItems, func(i, j int) bool { return scoredItems[i].score > scoredItems[j].score })
		if limit <= 0 {
			limit = 20
		}
		if len(scoredItems) > limit {
			scoredItems = scoredItems[:limit]
		}
		items := make([]any, 0, len(scoredItems))
		for _, s := range scoredItems {
			items = append(items, map[string]any{
				"listingId": s.e.ID,
				"slug":      s.e.Slug,
				"title":     s.e.Title,
				"summary":   s.e.Summary,
				"creator":   s.e.Creator,
				"tags":      s.e.Tags,
				"pricing":   s.e.Pricing,
				"stats":     s.e.Stats,
				"score":     s.score,
				"reason":    s.reason,
			})
		}
		meta := map[string]any{"query": args[0], "total": len(items), "hint": "Mock ranking; use `registry show <id|slug>` to inspect a listing"}
		if err := store.Save(st); err != nil { // no-op but keeps TUI in sync if future changes happen
			return writeErr(cmd, err)
		}
		return writeData(cmd, app, meta, map[string]any{"items": items})
	}}
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results")
	return cmd
}

func newRegistryShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <ref>", Short: "Show registry entry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		meta := map[string]any{"hint": "Use `registry versions <id|slug>` for publish history; `pricing show <id|slug>` for price."}
		return writeData(cmd, app, meta, map[string]any{"entry": e})
	}}
	return cmd
}

func newRegistryVersionsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "versions <ref>", Short: "List published versions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		items := make([]any, 0, len(e.Versions))
		for _, v := range e.Versions {
			items = append(items, v)
		}
		sort.Slice(items, func(i, j int) bool {
			vi, _ := items[i].(state.RegistryVersion)
			vj, _ := items[j].(state.RegistryVersion)
			return vi.Version > vj.Version
		})
		return writeData(cmd, app, map[string]any{"listingId": e.ID, "slug": e.Slug}, map[string]any{"items": items})
	}}
	return cmd
}

func newRegistryPublishCmd(app *App) *cobra.Command {
	var (
		title    string
		summary  string
		model    string
		amount   int64
		currency string
		interval string
		note     string
	)
	cmd := &cobra.Command{Use: "publish <flow-slug>", Short: "Publish a flow to the registry (mock)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		flowSlug := args[0]
		f := ws.Flows[flowSlug]
		if f == nil {
			return writeErr(cmd, errors.New("flow not found"))
		}
		// Create or update listing.
		id := "wrk-" + flowSlug
		e := ws.Registry[id]
		now := time.Now().UTC()
		if e == nil {
			e = &state.RegistryEntry{
				ID:          id,
				Slug:        flowSlug,
				Title:       f.Name,
				Summary:     f.Description,
				Description: f.Description,
				Creator:     ws.Owner,
				Category:    firstTagOr("general", f.Tags),
				Tags:        f.Tags,
				Pricing:     state.Pricing{Model: "per_run", Currency: "USD", AmountCents: 250},
				UpdatedAt:   now,
				PublishedAt: now,
				Versions:    []state.RegistryVersion{},
				Stats:       state.RegistryStats{Views: 0, Installs: 0, Active: 0, SuccessRate: 1.0, Rating: 0, Reviews: 0, RevenueCents: 0},
			}
			ws.Registry[id] = e
		}
		if title != "" {
			e.Title = title
		}
		if summary != "" {
			e.Summary = summary
		}
		if model != "" {
			e.Pricing.Model = model
		}
		if currency != "" {
			e.Pricing.Currency = currency
		}
		if amount > 0 {
			e.Pricing.AmountCents = amount
		}
		if interval != "" {
			e.Pricing.Interval = interval
		}
		e.Tags = f.Tags
		e.UpdatedAt = now
		if e.PublishedAt.IsZero() {
			e.PublishedAt = now
		}
		nextV := len(e.Versions) + 1
		e.Versions = append(e.Versions, state.RegistryVersion{
			Version:     nextV,
			PublishedAt: now,
			Note:        note,
			FlowSlug:    flowSlug,
			FlowVersion: f.ActiveVersion,
		})
		ws.UpdatedAt = now
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		meta := map[string]any{"hint": "Mock publish. Use `registry show` and `pricing show`."}
		return writeData(cmd, app, meta, map[string]any{"entry": e})
	}}
	cmd.Flags().StringVar(&title, "title", "", "Listing title")
	cmd.Flags().StringVar(&summary, "summary", "", "Listing summary")
	cmd.Flags().StringVar(&model, "model", "", "Pricing model (per_run|per_success|subscription)")
	cmd.Flags().Int64Var(&amount, "amount-cents", 0, "Price amount in cents")
	cmd.Flags().StringVar(&currency, "currency", "", "Currency (e.g. USD)")
	cmd.Flags().StringVar(&interval, "interval", "", "Subscription interval (month|year)")
	cmd.Flags().StringVar(&note, "note", "", "Publish note/changelog")
	return cmd
}

func newRegistryMatchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "match <need>", Short: "Suggest a workflow for a need (mock)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		need := strings.TrimSpace(args[0])
		if need == "" {
			return writeErr(cmd, errors.New("empty need"))
		}
		// Simple strategy: if a demand cluster matches, return its listings; otherwise fall back to search.
		bestCluster := (*state.DemandCluster)(nil)
		nl := strings.ToLower(need)
		for i := range ws.DemandClusters {
			c := &ws.DemandClusters[i]
			if strings.Contains(nl, strings.ToLower(c.Title)) {
				bestCluster = c
				break
			}
		}
		candidates := make([]*state.RegistryEntry, 0)
		if bestCluster != nil {
			for _, id := range bestCluster.MatchedListings {
				if e := ws.Registry[id]; e != nil {
					candidates = append(candidates, e)
				}
			}
		}
		if len(candidates) == 0 {
			// Fallback: naive search across registry
			toks := strings.Fields(strings.ToLower(need))
			for _, e := range ws.Registry {
				hay := strings.ToLower(e.Slug + " " + e.Title + " " + e.Summary + " " + strings.Join(e.Tags, " "))
				hit := 0
				for _, t := range toks {
					if t != "" && strings.Contains(hay, t) {
						hit++
					}
				}
				if hit > 0 {
					candidates = append(candidates, e)
				}
			}
		}
		if len(candidates) == 0 {
			return writeData(cmd, app, map[string]any{"hint": "No match in mock registry"}, map[string]any{"match": nil, "items": []any{}})
		}
		// Pick the highest success rate as a tiebreaker.
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Stats.SuccessRate > candidates[j].Stats.SuccessRate })
		items := make([]any, 0, len(candidates))
		for _, e := range candidates {
			items = append(items, map[string]any{
				"listingId": e.ID,
				"slug":      e.Slug,
				"title":     e.Title,
				"pricing":   e.Pricing,
				"stats":     e.Stats,
				"reason":    "mock-match",
			})
		}
		meta := map[string]any{"need": need, "hint": "Mock matching. Use `registry search` for manual exploration."}
		return writeData(cmd, app, meta, map[string]any{"match": candidates[0], "items": items})
	}}
	return cmd
}

func newRegistryInstallCmd(app *App) *cobra.Command {
	var buyer string
	cmd := &cobra.Command{Use: "install <ref>", Short: "Install from registry (mock purchase + entitlement)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		if buyer == "" {
			buyer = "buyer@demo.test"
		}
		now := time.Now().UTC()
		purID := fmt.Sprintf("pur-%d", now.Unix())
		paid := now
		ws.Purchases[purID] = &state.Purchase{ID: purID, ListingID: e.ID, Buyer: buyer, Status: "paid", CreatedAt: now, PaidAt: &paid, AmountCents: e.Pricing.AmountCents, Currency: e.Pricing.Currency}
		entID := fmt.Sprintf("ent-%d", now.Unix())
		ws.Entitlements[entID] = &state.Entitlement{ID: entID, ListingID: e.ID, Buyer: buyer, Status: "active", CreatedAt: now, Limits: map[string]any{"runsPerMonth": 200}}
		e.Stats.Installs++
		e.Stats.Active++
		ws.UpdatedAt = now
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		meta := map[string]any{"hint": "Mock install creates a paid purchase + active entitlement in state."}
		return writeData(cmd, app, meta, map[string]any{"purchaseId": purID, "entitlementId": entID, "listingId": e.ID})
	}}
	cmd.Flags().StringVar(&buyer, "buyer", "buyer@demo.test", "Buyer identity (mock)")
	return cmd
}

func findRegistry(ws *state.Workspace, ref string) *state.RegistryEntry {
	if ws == nil {
		return nil
	}
	if ws.Registry == nil {
		return nil
	}
	if e := ws.Registry[ref]; e != nil {
		return e
	}
	// try id prefix
	if e := ws.Registry["wrk-"+ref]; e != nil {
		return e
	}
	// try slug match
	for _, e := range ws.Registry {
		if e.Slug == ref {
			return e
		}
	}
	return nil
}

func firstTagOr(d string, tags []string) string {
	if len(tags) == 0 {
		return d
	}
	if tags[0] == "" {
		return d
	}
	return tags[0]
}

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate"}
	cmd.AddCommand(&cobra.Command{Use: "whoami", Short: "Show identity", RunE: func(cmd *cobra.Command, args []string) error {
		meta := map[string]any{"hint": "Mock auth; use --token/BREYTA_TOKEN when wired to real API"}
		return writeData(cmd, app, meta, map[string]any{"tokenPresent": app.Token != ""})
	}})
	cmd.AddCommand(&cobra.Command{Use: "login", Short: "Login", RunE: func(cmd *cobra.Command, args []string) error {
		return writeNotImplemented(cmd, app, "Planned: device/browser login")
	}})
	cmd.AddCommand(&cobra.Command{Use: "logout", Short: "Logout", RunE: func(cmd *cobra.Command, args []string) error {
		return writeNotImplemented(cmd, app, "Planned: clear local auth")
	}})
	return cmd
}

func newWorkspacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "workspaces", Short: "Manage workspaces"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List workspaces (from state)", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		items := make([]map[string]any, 0, len(st.Workspaces))
		for id, ws := range st.Workspaces {
			items = append(items, map[string]any{"id": id, "name": ws.Name, "plan": ws.Plan, "owner": ws.Owner, "updatedAt": ws.UpdatedAt})
		}
		return writeData(cmd, app, map[string]any{"total": len(items)}, map[string]any{"items": items})
	}})
	cmd.AddCommand(&cobra.Command{Use: "show <workspace-id>", Short: "Show workspace (from state)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws := st.Workspaces[args[0]]
		if ws == nil {
			return writeErr(cmd, errors.New("workspace not found"))
		}
		data := map[string]any{"id": ws.ID, "name": ws.Name, "plan": ws.Plan, "owner": ws.Owner, "updatedAt": ws.UpdatedAt, "flows": len(ws.Flows), "runs": len(ws.Runs)}
		return writeData(cmd, app, map[string]any{"hint": "Use --workspace to select"}, map[string]any{"workspace": data})
	}})
	cmd.AddCommand(&cobra.Command{Use: "use <workspace-id>", Short: "Set default workspace (mock)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return writeNotImplemented(cmd, app, "Planned: write local config/profile")
	}})
	return cmd
}

// --- Marketplace v1 (mock) ---------------------------------------------------

func newPricingCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "pricing", Short: "Pricing for registry listings (mock)"}
	cmd.AddCommand(newPricingShowCmd(app))
	cmd.AddCommand(newPricingSetCmd(app))
	return cmd
}

func newPricingShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <ref>", Short: "Show pricing", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		return writeData(cmd, app, map[string]any{"listingId": e.ID, "slug": e.Slug}, map[string]any{"pricing": e.Pricing})
	}}
	return cmd
}

func newPricingSetCmd(app *App) *cobra.Command {
	var (
		model    string
		amount   int64
		currency string
		interval string
	)
	cmd := &cobra.Command{Use: "set <ref>", Short: "Set pricing", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		if model != "" {
			e.Pricing.Model = model
		}
		if currency != "" {
			e.Pricing.Currency = currency
		}
		if amount > 0 {
			e.Pricing.AmountCents = amount
		}
		if interval != "" {
			e.Pricing.Interval = interval
		}
		e.UpdatedAt = time.Now().UTC()
		ws.UpdatedAt = e.UpdatedAt
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		return writeData(cmd, app, map[string]any{"hint": "Mock pricing updated on registry entry"}, map[string]any{"entry": e})
	}}
	cmd.Flags().StringVar(&model, "model", "", "per_run|per_success|subscription")
	cmd.Flags().Int64Var(&amount, "amount-cents", 0, "Amount in cents")
	cmd.Flags().StringVar(&currency, "currency", "USD", "Currency")
	cmd.Flags().StringVar(&interval, "interval", "", "Subscription interval (month|year)")
	return cmd
}

func newPurchasesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "purchases", Short: "Purchases (mock)"}
	cmd.AddCommand(newPurchasesListCmd(app))
	cmd.AddCommand(newPurchasesShowCmd(app))
	cmd.AddCommand(newPurchasesCreateCmd(app))
	return cmd
}

func newPurchasesListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "List purchases", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		items := make([]any, 0, len(ws.Purchases))
		for _, p := range ws.Purchases {
			items = append(items, p)
		}
		sort.Slice(items, func(i, j int) bool {
			pi, _ := items[i].(*state.Purchase)
			pj, _ := items[j].(*state.Purchase)
			if pi == nil || pj == nil {
				return false
			}
			return pi.CreatedAt.After(pj.CreatedAt)
		})
		return writeData(cmd, app, map[string]any{"total": len(items)}, map[string]any{"items": items})
	}}
	return cmd
}

func newPurchasesShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <purchase-id>", Short: "Show purchase", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		p := ws.Purchases[args[0]]
		if p == nil {
			return writeErr(cmd, errors.New("purchase not found"))
		}
		return writeData(cmd, app, nil, map[string]any{"purchase": p})
	}}
	return cmd
}

func newPurchasesCreateCmd(app *App) *cobra.Command {
	var buyer string
	cmd := &cobra.Command{Use: "create <listing-ref>", Short: "Create purchase (paid) + entitlement", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, store, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := findRegistry(ws, args[0])
		if e == nil {
			return writeErr(cmd, errors.New("registry entry not found"))
		}
		if buyer == "" {
			buyer = "buyer@demo.test"
		}
		now := time.Now().UTC()
		purID := fmt.Sprintf("pur-%d", now.UnixNano())
		paid := now
		ws.Purchases[purID] = &state.Purchase{ID: purID, ListingID: e.ID, Buyer: buyer, Status: "paid", CreatedAt: now, PaidAt: &paid, AmountCents: e.Pricing.AmountCents, Currency: e.Pricing.Currency}
		entID := fmt.Sprintf("ent-%d", now.UnixNano())
		ws.Entitlements[entID] = &state.Entitlement{ID: entID, ListingID: e.ID, Buyer: buyer, Status: "active", CreatedAt: now, Limits: map[string]any{"runsPerMonth": 200}}
		e.Stats.Installs++
		e.Stats.Active++
		ws.UpdatedAt = now
		if err := store.Save(st); err != nil {
			return writeErr(cmd, err)
		}
		return writeData(cmd, app, map[string]any{"hint": "Mock purchase creates entitlement"}, map[string]any{"purchaseId": purID, "entitlementId": entID, "listingId": e.ID})
	}}
	cmd.Flags().StringVar(&buyer, "buyer", "buyer@demo.test", "Buyer identity (mock)")
	return cmd
}

func newEntitlementsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "entitlements", Short: "Entitlements (mock)"}
	cmd.AddCommand(newEntitlementsListCmd(app))
	cmd.AddCommand(newEntitlementsShowCmd(app))
	return cmd
}

func newEntitlementsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "List entitlements", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		items := make([]any, 0, len(ws.Entitlements))
		for _, e := range ws.Entitlements {
			items = append(items, e)
		}
		return writeData(cmd, app, map[string]any{"total": len(items)}, map[string]any{"items": items})
	}}
	return cmd
}

func newEntitlementsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <entitlement-id>", Short: "Show entitlement", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		e := ws.Entitlements[args[0]]
		if e == nil {
			return writeErr(cmd, errors.New("entitlement not found"))
		}
		return writeData(cmd, app, nil, map[string]any{"entitlement": e})
	}}
	return cmd
}

func newPayoutsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "payouts", Short: "Creator payouts (mock)"}
	cmd.AddCommand(newPayoutsListCmd(app))
	cmd.AddCommand(newPayoutsShowCmd(app))
	return cmd
}

func newPayoutsListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "list", Short: "List payouts", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		items := make([]any, 0, len(ws.Payouts))
		for _, p := range ws.Payouts {
			items = append(items, p)
		}
		return writeData(cmd, app, map[string]any{"total": len(items)}, map[string]any{"items": items})
	}}
	return cmd
}

func newPayoutsShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "show <payout-id>", Short: "Show payout", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		p := ws.Payouts[args[0]]
		if p == nil {
			return writeErr(cmd, errors.New("payout not found"))
		}
		return writeData(cmd, app, nil, map[string]any{"payout": p})
	}}
	return cmd
}

func newCreatorCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "creator", Short: "Creator dashboard (mock)"}
	cmd.AddCommand(newCreatorDashboardCmd(app))
	return cmd
}

func newCreatorDashboardCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Creator dashboard summary", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		// Aggregate from registry stats + payouts.
		totalRevenue := int64(0)
		activeInstalls := 0
		top := (*state.RegistryEntry)(nil)
		for _, e := range ws.Registry {
			totalRevenue += e.Stats.RevenueCents
			activeInstalls += e.Stats.Active
			if top == nil || e.Stats.RevenueCents > top.Stats.RevenueCents {
				top = e
			}
		}
		topDemand := ""
		if len(ws.DemandClusters) > 0 {
			topDemand = ws.DemandClusters[0].ID
		}
		return writeData(cmd, app, map[string]any{"hint": "Mock creator dashboard"}, map[string]any{
			"creator": map[string]any{
				"id": ws.Owner,
			},
			"summary": map[string]any{
				"totalRevenueCents": totalRevenue,
				"activeInstalls":    activeInstalls,
				"topListingId":      idOr("", top),
				"topDemandCluster":  topDemand,
			},
		})
	}}
	return cmd
}

func idOr(d string, e *state.RegistryEntry) string {
	if e == nil {
		return d
	}
	return e.ID
}

func newAnalyticsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "analytics", Short: "Marketplace analytics (mock)"}
	cmd.AddCommand(&cobra.Command{Use: "overview", Short: "High-level analytics", RunE: func(cmd *cobra.Command, args []string) error {
		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws, err := getWorkspace(st, app.WorkspaceID)
		if err != nil {
			return writeErr(cmd, err)
		}
		// Compute a simple funnel based on stats.
		views := 0
		installs := 0
		active := 0
		for _, e := range ws.Registry {
			views += e.Stats.Views
			installs += e.Stats.Installs
			active += e.Stats.Active
		}
		return writeData(cmd, app, map[string]any{"hint": "Mock analytics overview"}, map[string]any{
			"funnel": map[string]any{"views": views, "installs": installs, "active": active},
		})
	}})
	return cmd
}
