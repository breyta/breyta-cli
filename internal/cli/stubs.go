package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/breyta/breyta-cli/internal/state"

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
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/connections", nil, nil)
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
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/connections/"+url.PathEscape(id), nil, nil)
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
	var name, typ, baseURL, description, slot, apiKey, backend, configJSON string
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
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				body := map[string]any{
					"type": typ,
					"name": name,
				}
				if strings.TrimSpace(baseURL) != "" {
					body["base-url"] = strings.TrimSpace(baseURL)
				}
				if strings.TrimSpace(description) != "" {
					body["description"] = strings.TrimSpace(description)
				}
				if strings.TrimSpace(slot) != "" {
					body["slot"] = strings.TrimSpace(slot)
				}
				if strings.TrimSpace(apiKey) != "" {
					body["api-key"] = strings.TrimSpace(apiKey)
				}
				if strings.TrimSpace(backend) != "" {
					body["backend"] = strings.TrimSpace(backend)
				}
				if strings.TrimSpace(configJSON) != "" {
					var v any
					if err := json.Unmarshal([]byte(configJSON), &v); err != nil {
						return writeErr(cmd, errors.New("invalid --config JSON"))
					}
					body["config"] = v
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/connections", nil, body)
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
	cmd.Flags().StringVar(&backend, "backend", "", "Connection backend (API mode)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL (HTTP connections)")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&slot, "slot", "", "Suggested slot name")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key (connection-dependent)")
	cmd.Flags().StringVar(&configJSON, "config", "", "Config JSON object (API mode)")
	return cmd
}

func newConnectionsUpdateCmd(app *App) *cobra.Command {
	var name, status, configJSON string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				body := map[string]any{"id": id}
				if name != "" {
					body["name"] = name
				}
				if status != "" {
					body["status"] = status
				}
				if strings.TrimSpace(configJSON) != "" {
					var v any
					if err := json.Unmarshal([]byte(configJSON), &v); err != nil {
						return writeErr(cmd, errors.New("invalid --config JSON"))
					}
					body["config"] = v
				}
				out, httpStatus, err := apiClient(app).DoREST(context.Background(), http.MethodPut, "/api/connections/"+url.PathEscape(id), nil, body)
				if err != nil {
					return writeErr(cmd, err)
				}
				return writeREST(cmd, app, httpStatus, out)
			}
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
			if strings.TrimSpace(configJSON) != "" {
				var v any
				if err := json.Unmarshal([]byte(configJSON), &v); err != nil {
					return writeErr(cmd, errors.New("invalid --config JSON"))
				}
				c.Config = v
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
	cmd.Flags().StringVar(&configJSON, "config", "", "Config JSON object (API mode)")
	return cmd
}

func newConnectionsDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete connection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodDelete, "/api/connections/"+url.PathEscape(id), nil, nil)
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
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/connections/"+url.PathEscape(id)+"/test", nil, nil)
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
	cmd.AddCommand(newProfilesActivateCmd(app))
	cmd.AddCommand(newProfilesListCmd(app))
	cmd.AddCommand(newProfilesShowCmd(app))
	cmd.AddCommand(newProfilesCreateCmd(app))
	cmd.AddCommand(newProfilesDeleteCmd(app))

	bindings := &cobra.Command{Use: "bindings", Short: "Manage profile bindings"}
	bindings.AddCommand(newProfilesBindingsSetCmd(app))
	cmd.AddCommand(bindings)

	return cmd
}

func newProfilesActivateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <flow-slug>",
		Short: "Activate flow profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow := strings.TrimSpace(args[0])
			if flow == "" {
				return writeErr(cmd, errors.New("missing flow slug"))
			}
			if isAPIMode(app) {
				return doAPICommand(cmd, app, "profiles.activate", map[string]any{"flowSlug": flow})
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
			if ws.Profiles == nil {
				ws.Profiles = map[string]*state.Profile{}
			}
			var active *state.Profile
			for _, it := range ws.Profiles {
				if it.FlowSlug == flow && it.ProfileType == "prod" {
					active = it
					break
				}
			}
			now := time.Now().UTC()
			if active == nil {
				id := "prof-" + now.Format("20060102-150405")
				active = &state.Profile{
					ID:          id,
					FlowSlug:    flow,
					Version:     f.ActiveVersion,
					Name:        flow,
					Enabled:     true,
					ProfileType: "prod",
					UpdatedAt:   now,
				}
				ws.Profiles[id] = active
			} else {
				active.Enabled = true
				if f.ActiveVersion > 0 {
					active.Version = f.ActiveVersion
				}
				active.UpdatedAt = now
			}
			ws.UpdatedAt = now
			if err := store.Save(st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, nil, map[string]any{"profile": active, "activated": true})
		},
	}
	return cmd
}

func newProfilesListCmd(app *App) *cobra.Command {
	var flow string
	var profileType string
	var userID string
	var enabled string
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				q := url.Values{}
				if strings.TrimSpace(flow) != "" {
					q.Set("flow-slug", strings.TrimSpace(flow))
				}
				if strings.TrimSpace(profileType) != "" {
					q.Set("profile-type", strings.TrimSpace(profileType))
				}
				if strings.TrimSpace(userID) != "" {
					q.Set("user-id", strings.TrimSpace(userID))
				}
				if strings.TrimSpace(enabled) != "" {
					e := strings.ToLower(strings.TrimSpace(enabled))
					if e != "true" && e != "false" {
						return writeErr(cmd, errors.New("--enabled must be true or false"))
					}
					q.Set("enabled", e)
				}
				if limit > 0 {
					q.Set("limit", strconv.Itoa(limit))
				}
				if strings.TrimSpace(cursor) != "" {
					q.Set("cursor", strings.TrimSpace(cursor))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/flow-profiles", q, nil)
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
	cmd.Flags().StringVar(&profileType, "profile-type", "", "Filter by profile type (prod|draft) (API mode only)")
	cmd.Flags().StringVar(&userID, "user-id", "", "Filter by user id (API mode only)")
	cmd.Flags().StringVar(&enabled, "enabled", "", "Filter by enabled (true|false) (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per page (API mode only)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	return cmd
}

func newProfilesShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/flow-profiles/"+url.PathEscape(id), nil, nil)
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
	var profileType string
	var bindingsJSON string
	var configJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flow == "" {
				return writeErr(cmd, errors.New("missing --flow"))
			}
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				body := map[string]any{
					"flow-slug":    strings.TrimSpace(flow),
					"profile-type": strings.ToLower(strings.TrimSpace(profileType)),
					"bindings":     map[string]any{},
					"config":       map[string]any{},
				}
				if version > 0 {
					body["version"] = version
				}
				if strings.TrimSpace(bindingsJSON) != "" {
					var v any
					if err := json.Unmarshal([]byte(bindingsJSON), &v); err != nil {
						return writeErr(cmd, errors.New("invalid --bindings JSON"))
					}
					m, ok := v.(map[string]any)
					if !ok {
						return writeErr(cmd, errors.New("--bindings must be a JSON object"))
					}
					body["bindings"] = m
				}
				if strings.TrimSpace(configJSON) != "" {
					var v any
					if err := json.Unmarshal([]byte(configJSON), &v); err != nil {
						return writeErr(cmd, errors.New("invalid --config JSON"))
					}
					m, ok := v.(map[string]any)
					if !ok {
						return writeErr(cmd, errors.New("--config must be a JSON object"))
					}
					body["config"] = m
				}

				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPost, "/api/flow-profiles", nil, body)
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
	cmd.Flags().StringVar(&profileType, "profile-type", "prod", "Profile type (prod|draft) (API mode only)")
	cmd.Flags().StringVar(&bindingsJSON, "bindings", "", "Bindings JSON object (API mode only)")
	cmd.Flags().StringVar(&configJSON, "config", "", "Config JSON object (API mode only)")
	return cmd
}

func newProfilesBindingsSetCmd(app *App) *cobra.Command {
	var bindings string
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set profile bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				if strings.TrimSpace(bindings) == "" {
					return writeErr(cmd, errors.New("missing --bindings"))
				}
				var v any
				if err := json.Unmarshal([]byte(bindings), &v); err != nil {
					return writeErr(cmd, errors.New("invalid --bindings JSON"))
				}
				m, ok := v.(map[string]any)
				if !ok {
					return writeErr(cmd, errors.New("--bindings must be a JSON object"))
				}
				payload := map[string]any{"bindings": m}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodPut, "/api/flow-profiles/"+url.PathEscape(id)+"/bindings", nil, payload)
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
	cmd.Flags().StringVar(&bindings, "bindings", "", "Bindings (mock: string; API mode: JSON object)")
	return cmd
}

func newProfilesDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				id := strings.TrimSpace(args[0])
				if id == "" {
					return writeErr(cmd, errors.New("missing id"))
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodDelete, "/api/flow-profiles/"+url.PathEscape(id), nil, nil)
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
	var limit int
	var cursor string
	var triggerType string
	cmd := &cobra.Command{
		Use:   "triggers <flow-slug>",
		Short: "Manage triggers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow := strings.TrimSpace(args[0])
			if flow == "" {
				return writeErr(cmd, errors.New("flow-slug is required"))
			}

			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				q := url.Values{}
				q.Set("flow", flow)
				if strings.TrimSpace(triggerType) != "" {
					q.Set("type", strings.TrimSpace(triggerType))
				}
				if strings.TrimSpace(cursor) != "" {
					q.Set("cursor", strings.TrimSpace(cursor))
				}
				if limit > 0 {
					q.Set("limit", strconv.Itoa(limit))
				}
				outAny, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/triggers", q, nil)
				if err != nil {
					return writeErr(cmd, err)
				}
				if status >= 400 {
					return writeREST(cmd, app, status, outAny)
				}

				out, _ := outAny.(map[string]any)
				rawTriggers, _ := out["triggers"].([]any)
				items := make([]map[string]any, 0, len(rawTriggers))
				for _, tAny := range rawTriggers {
					t, _ := tAny.(map[string]any)
					if t == nil {
						continue
					}
					config, _ := t["config"].(map[string]any)
					path, _ := config["path"].(string)
					source, _ := config["source"].(string)
					triggerType, _ := t["type"].(string)
					isWebhook := strings.EqualFold(strings.TrimSpace(triggerType), "webhook")
					if !isWebhook {
						isWebhook = strings.EqualFold(strings.TrimSpace(triggerType), "event") && strings.EqualFold(strings.TrimSpace(source), "webhook")
					}
					if isWebhook && path != "" {
						webhookURL := webhookEventURL(app, path)
						if param, ok := webhookAuthQueryParam(config); ok {
							webhookURL = appendWebhookQueryPlaceholder(webhookURL, param)
						}
						t["webhookUrl"] = webhookURL
					}
					items = append(items, t)
				}
				meta := map[string]any{"total": len(items)}
				if next, _ := out["next-cursor"].(string); strings.TrimSpace(next) != "" {
					meta["nextCursor"] = next
				}
				if hasMore, ok := out["has-more"].(bool); ok {
					meta["hasMore"] = hasMore
				}
				return writeData(cmd, app, meta, map[string]any{"items": items})
			}

			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}
			items := make([]map[string]any, 0)
			for _, t := range ws.Triggers {
				if t == nil || t.FlowSlug != flow {
					continue
				}
				cfg, _ := t.Config.(map[string]any)
				path, _ := cfg["path"].(string)
				entry := map[string]any{
					"id":        t.ID,
					"flowSlug":  t.FlowSlug,
					"type":      t.Type,
					"label":     t.Name,
					"enabled":   t.Enabled,
					"updatedAt": t.UpdatedAt,
					"config":    t.Config,
				}
				source, _ := cfg["source"].(string)
				isWebhook := strings.EqualFold(strings.TrimSpace(t.Type), "webhook")
				if !isWebhook {
					isWebhook = strings.EqualFold(strings.TrimSpace(t.Type), "event") && strings.EqualFold(strings.TrimSpace(source), "webhook")
				}
				if isWebhook && path != "" {
					webhookURL := webhookEventURL(app, path)
					if param, ok := webhookAuthQueryParam(cfg); ok {
						webhookURL = appendWebhookQueryPlaceholder(webhookURL, param)
					}
					entry["webhookUrl"] = webhookURL
				}
				items = append(items, entry)
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per page (API mode only)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	cmd.Flags().StringVar(&triggerType, "type", "", "Filter by trigger type (API mode only)")
	cmd.AddCommand(newTriggersListCmd(app))
	cmd.AddCommand(newTriggersShowCmd(app))
	cmd.AddCommand(newTriggersWebhookURLCmd(app))
	cmd.AddCommand(newTriggersWebhookSecretCmd(app))
	cmd.AddCommand(newTriggersFireCmd(app))
	return cmd
}

func newTriggersShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <trigger-id>",
		Short: "Show trigger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			triggerID := strings.TrimSpace(args[0])
			if triggerID == "" {
				return writeErr(cmd, errors.New("missing trigger id"))
			}

			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				out, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/triggers/"+url.PathEscape(triggerID), nil, nil)
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
			t := ws.Triggers[triggerID]
			if t == nil {
				return writeErr(cmd, errors.New("trigger not found"))
			}
			return writeData(cmd, app, nil, map[string]any{"trigger": t})
		},
	}
	return cmd
}

func webhookEventURL(app *App, webhookPath string) string {
	p := strings.TrimSpace(webhookPath)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/events/%s", baseURL(app), app.WorkspaceID, p)
}

func webhookAuthQueryParam(config map[string]any) (string, bool) {
	if len(config) == 0 {
		return "", false
	}
	authAny := config["auth"]
	auth, ok := authAny.(map[string]any)
	if !ok || len(auth) == 0 {
		return "", false
	}
	authType := strings.TrimSpace(fmt.Sprint(auth["type"]))
	if !strings.EqualFold(authType, "api-key") {
		return "", false
	}
	location := strings.TrimSpace(fmt.Sprint(auth["location"]))
	if location == "" {
		location = strings.TrimSpace(fmt.Sprint(auth["in"]))
	}
	if location == "" {
		return "", false
	}
	isQuery := strings.EqualFold(location, "query") || strings.EqualFold(location, "query-param") || strings.EqualFold(location, "param")
	if !isQuery {
		return "", false
	}
	param := strings.TrimSpace(fmt.Sprint(auth["param"]))
	if param == "" {
		param = "token"
	}
	return param, true
}

func appendWebhookQueryPlaceholder(rawURL, param string) string {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(param) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	if q.Get(param) == "" {
		q.Set(param, "<api-key>")
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func newTriggersListCmd(app *App) *cobra.Command {
	var flow string
	var triggerType string
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List triggers (legacy; use `triggers <flow-slug>`)",
		Long:  "List triggers. Prefer `breyta triggers <flow-slug>` for a flow-scoped view with webhook URLs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				q := url.Values{}
				if strings.TrimSpace(flow) != "" {
					q.Set("flow", strings.TrimSpace(flow))
				}
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
	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
	cmd.Flags().StringVar(&triggerType, "type", "", "Filter by trigger type (event|schedule|manual|webhook) (API mode only)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per page (API mode only)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor (API mode only)")
	return cmd
}

func newTriggersWebhookURLCmd(app *App) *cobra.Command {
	var flow string
	var limit int

	cmd := &cobra.Command{
		Use:   "webhook-url <flow-slug>",
		Short: "Show webhook URL(s) for webhook triggers",
		Long: strings.TrimSpace(`
Webhook triggers are fired via the public events endpoint:
  POST /<workspace>/events/<path>

This command lists webhook trigger URLs for a flow so you can copy/paste them into external systems.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return writeErr(cmd, errors.New("flow-slug is required"))
			}
			if flow == "" {
				flow = strings.TrimSpace(args[0])
			}
			if isAPIMode(app) {
				if err := requireAPI(app); err != nil {
					return writeErr(cmd, err)
				}
				if limit <= 0 {
					limit = 100
				}

				q := url.Values{}
				q.Set("type", "webhook")
				q.Set("flow", strings.TrimSpace(flow))
				q.Set("limit", strconv.Itoa(limit))

				outAny, status, err := apiClient(app).DoREST(context.Background(), http.MethodGet, "/api/triggers", q, nil)
				if err != nil {
					return writeErr(cmd, err)
				}
				if status >= 400 {
					return writeREST(cmd, app, status, outAny)
				}

				out, _ := outAny.(map[string]any)
				rawTriggers, _ := out["triggers"].([]any)

				items := make([]map[string]any, 0, len(rawTriggers))
				for _, tAny := range rawTriggers {
					t, _ := tAny.(map[string]any)
					if t == nil {
						continue
					}

					id, _ := t["id"].(string)
					label, _ := t["label"].(string)
					flowSlug, _ := t["flow-slug"].(string)
					if flowSlug == "" {
						flowSlug, _ = t["flowSlug"].(string)
					}
					config, _ := t["config"].(map[string]any)
					path, _ := config["path"].(string)

					items = append(items, map[string]any{
						"triggerId": id,
						"flowSlug":  flowSlug,
						"label":     label,
						"path":      path,
						"url":       webhookEventURL(app, path),
					})
				}

				meta := map[string]any{
					"total": len(items),
				}
				if next, _ := out["next-cursor"].(string); strings.TrimSpace(next) != "" {
					meta["nextCursor"] = next
				}
				if hasMore, ok := out["has-more"].(bool); ok {
					meta["hasMore"] = hasMore
				}
				return writeData(cmd, app, meta, map[string]any{"items": items})
			}

			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			ws, err := getWorkspace(st, app.WorkspaceID)
			if err != nil {
				return writeErr(cmd, err)
			}

			items := make([]map[string]any, 0)
			for _, t := range ws.Triggers {
				if t == nil {
					continue
				}
				if flow != "" && t.FlowSlug != flow {
					continue
				}
				if strings.ToLower(strings.TrimSpace(t.Type)) != "webhook" {
					continue
				}
				cfg, _ := t.Config.(map[string]any)
				path, _ := cfg["path"].(string)
				items = append(items, map[string]any{
					"triggerId": t.ID,
					"flowSlug":  t.FlowSlug,
					"label":     t.Name,
					"path":      path,
					"url":       webhookEventURL(app, path),
				})
			}
			meta := map[string]any{"total": len(items)}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		},
	}

	cmd.Flags().StringVar(&flow, "flow", "", "Filter by flow slug")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max items per page (API mode only)")
	return cmd
}

func newTriggersWebhookSecretCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook-secret <trigger-id>",
		Short: "Generate webhook signing secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Use API mode to generate webhook secrets.")
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			out, status, err := apiClient(app).DoREST(
				context.Background(),
				http.MethodPost,
				"/api/triggers/"+url.PathEscape(args[0])+"/webhook-secret",
				nil,
				nil,
			)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status >= 400 {
				return writeREST(cmd, app, status, out)
			}
			meta := map[string]any{
				"warning": "Secret is shown once. Regenerating invalidates the previous secret.",
			}
			return writeData(cmd, app, meta, out)
		},
	}
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

func newWorkspacesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "workspaces", Short: "Manage workspaces"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List workspaces", RunE: func(cmd *cobra.Command, args []string) error {
		// In API mode, never fall back to mock data.
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}

			m, ok := out.(map[string]any)
			if !ok {
				return writeFailure(cmd, app, "workspaces_list_unexpected_response", fmt.Errorf("unexpected response (status=%d)", status), "Expected JSON object from /api/me", out)
			}

			raw, _ := m["workspaces"].([]any)
			items := make([]any, 0, len(raw))
			for _, v := range raw {
				wm, ok := v.(map[string]any)
				if !ok || wm == nil {
					continue
				}
				// Mark the configured default workspace for quick scanning in terminals.
				id, _ := wm["id"].(string)
				m2 := make(map[string]any, len(wm)+1)
				for k, vv := range wm {
					m2[k] = vv
				}
				m2["current"] = strings.TrimSpace(id) != "" && strings.TrimSpace(id) == strings.TrimSpace(app.WorkspaceID)
				items = append(items, m2)
			}
			meta := map[string]any{"total": len(items), "httpStatus": status}
			return writeData(cmd, app, meta, map[string]any{"items": items})
		}

		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		items := make([]map[string]any, 0, len(st.Workspaces))
		for id, ws := range st.Workspaces {
			items = append(items, map[string]any{"id": id, "name": ws.Name, "plan": ws.Plan, "owner": ws.Owner, "updatedAt": ws.UpdatedAt, "current": strings.TrimSpace(id) != "" && strings.TrimSpace(id) == strings.TrimSpace(app.WorkspaceID)})
		}
		return writeData(cmd, app, map[string]any{"total": len(items)}, map[string]any{"items": items})
	}})

	cmd.AddCommand(&cobra.Command{Use: "current", Short: "Show current workspace", RunE: func(cmd *cobra.Command, args []string) error {
		wsID := strings.TrimSpace(app.WorkspaceID)
		source := "config"
		workspaceFlagExplicit := false
		if cmd != nil {
			workspaceFlagExplicit = cmd.Flags().Changed("workspace") || cmd.InheritedFlags().Changed("workspace")
			if root := cmd.Root(); root != nil {
				workspaceFlagExplicit = workspaceFlagExplicit || root.PersistentFlags().Changed("workspace")
			}
		}
		workspaceEnvExplicit := strings.TrimSpace(os.Getenv("BREYTA_WORKSPACE")) != ""
		if workspaceFlagExplicit {
			source = "flag"
		} else if workspaceEnvExplicit {
			source = "env"
		} else if wsID == "" {
			source = "none"
		}

		if wsID == "" {
			meta := map[string]any{
				"workspaceIdSource": source,
				"hint":              "Set a workspace via --workspace or BREYTA_WORKSPACE.",
			}
			return writeData(cmd, app, meta, map[string]any{"workspace": map[string]any{"id": "", "name": ""}})
		}

		// Prefer a name/plan/owner if we can resolve it, but don't require auth just to show the configured id.
		if isAPIMode(app) {
			// Try to load a stored token (if any) so we can resolve workspace details without requiring explicit flags/env.
			if !app.TokenExplicit && strings.TrimSpace(app.Token) == "" {
				loadTokenFromAuthStore(app)
			}
		}
		if isAPIMode(app) && strings.TrimSpace(app.Token) != "" {
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
			if err == nil {
				if m, ok := out.(map[string]any); ok {
					if raw, ok := m["workspaces"].([]any); ok {
						for _, v := range raw {
							wm, ok := v.(map[string]any)
							if !ok || wm == nil {
								continue
							}
							if id, _ := wm["id"].(string); strings.TrimSpace(id) == wsID {
								m2 := make(map[string]any, len(wm)+1)
								for k, vv := range wm {
									m2[k] = vv
								}
								m2["current"] = true
								meta := map[string]any{"httpStatus": status, "workspaceIdSource": source, "hint": "Override per-run via --workspace or BREYTA_WORKSPACE."}
								return writeData(cmd, app, meta, map[string]any{"workspace": m2})
							}
						}
					}
				}
			}
			// Fall through to a minimal response if the API call fails; the command is still useful offline.
		}

		if !isAPIMode(app) {
			st, _, err := appStore(app)
			if err != nil {
				return writeErr(cmd, err)
			}
			if ws := st.Workspaces[wsID]; ws != nil {
				meta := map[string]any{"workspaceIdSource": source, "hint": "Override per-run via --workspace or BREYTA_WORKSPACE."}
				data := map[string]any{"id": ws.ID, "name": ws.Name, "plan": ws.Plan, "owner": ws.Owner, "updatedAt": ws.UpdatedAt, "current": true}
				return writeData(cmd, app, meta, map[string]any{"workspace": data})
			}
		}

		meta := map[string]any{
			"workspaceIdSource": source,
			"warning":           "Unable to resolve workspace details (missing token, API error, or workspace not found); showing workspaceId only.",
			"hint":              "Run `breyta auth login` (API mode) or `breyta workspaces list` to see available workspaces.",
		}
		return writeData(cmd, app, meta, map[string]any{"workspace": map[string]any{"id": wsID, "name": "", "current": true}})
	}})

	cmd.AddCommand(&cobra.Command{Use: "show <workspace-id>", Short: "Show workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		workspaceID := strings.TrimSpace(args[0])
		if workspaceID == "" {
			return writeErr(cmd, errors.New("workspace-id required"))
		}

		// In API mode, never fall back to mock data.
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}

			m, ok := out.(map[string]any)
			if !ok {
				return writeFailure(cmd, app, "workspaces_show_unexpected_response", fmt.Errorf("unexpected response (status=%d)", status), "Expected JSON object from /api/me", out)
			}

			raw, _ := m["workspaces"].([]any)
			for _, v := range raw {
				wm, ok := v.(map[string]any)
				if !ok {
					continue
				}
				if id, _ := wm["id"].(string); strings.TrimSpace(id) == workspaceID {
					meta := map[string]any{"httpStatus": status, "hint": "Use --workspace to select"}
					return writeData(cmd, app, meta, map[string]any{"workspace": wm})
				}
			}
			return writeErr(cmd, errors.New("workspace not found"))
		}

		st, _, err := appStore(app)
		if err != nil {
			return writeErr(cmd, err)
		}
		ws := st.Workspaces[workspaceID]
		if ws == nil {
			return writeErr(cmd, errors.New("workspace not found"))
		}
		data := map[string]any{"id": ws.ID, "name": ws.Name, "plan": ws.Plan, "owner": ws.Owner, "updatedAt": ws.UpdatedAt, "flows": len(ws.Flows), "runs": len(ws.Runs)}
		meta := map[string]any{
			"hint": "Use --workspace to select",
		}
		return writeData(cmd, app, meta, map[string]any{"workspace": data})
	}})
	cmd.AddCommand(&cobra.Command{Use: "use <workspace-id>", Short: "Set default workspace", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		workspaceID := strings.TrimSpace(args[0])
		if workspaceID == "" {
			return writeErr(cmd, errors.New("workspace-id required"))
		}

		// In API mode, validate membership before saving.
		if isAPIMode(app) {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			m, ok := out.(map[string]any)
			if !ok {
				return writeFailure(cmd, app, "workspaces_use_unexpected_response", fmt.Errorf("unexpected response (status=%d)", status), "Expected JSON object from /api/me", out)
			}
			raw, _ := m["workspaces"].([]any)
			found := false
			for _, v := range raw {
				wm, ok := v.(map[string]any)
				if !ok || wm == nil {
					continue
				}
				if id, _ := wm["id"].(string); strings.TrimSpace(id) == workspaceID {
					found = true
					break
				}
			}
			if !found {
				return writeErr(cmd, errors.New("workspace not found"))
			}
		}

		p, err := configstore.DefaultPath()
		if err != nil {
			return writeErr(cmd, err)
		}
		st, _ := configstore.Load(p)
		if st == nil {
			st = &configstore.Store{}
		}
		st.APIURL = strings.TrimSpace(app.APIURL)
		st.WorkspaceID = workspaceID
		if err := configstore.SaveAtomic(p, st); err != nil {
			return writeErr(cmd, err)
		}
		app.WorkspaceID = workspaceID
		meta := map[string]any{"workspaceIdSource": "config", "hint": "Override per-run via --workspace or BREYTA_WORKSPACE."}
		return writeData(cmd, app, meta, map[string]any{"workspace": map[string]any{"id": workspaceID, "current": true}})
	}})

	var bootstrapName string
	bootstrapCmd := &cobra.Command{Use: "bootstrap <workspace-id>", Short: "Bootstrap workspace + membership (API mode)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAPI(app); err != nil {
			return writeErr(cmd, err)
		}
		workspaceID := strings.TrimSpace(args[0])
		if workspaceID == "" {
			return writeErr(cmd, errors.New("workspace-id required"))
		}

		endpoint := strings.TrimRight(app.APIURL, "/") + "/api/debug/workspace/bootstrap"
		payload := map[string]any{"workspaceId": workspaceID}
		if strings.TrimSpace(bootstrapName) != "" {
			payload["name"] = strings.TrimSpace(bootstrapName)
		}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			return writeErr(cmd, err)
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, &buf)
		if err != nil {
			return writeErr(cmd, err)
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(app.Token) != "" {
			req.Header.Set("Authorization", "Bearer "+app.Token)
			req.Header.Set("x-debug-user-id", app.Token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return writeErr(cmd, err)
		}
		defer resp.Body.Close()

		var out any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			out = map[string]any{"error": err.Error()}
		}

		meta := map[string]any{"status": resp.StatusCode}
		if resp.StatusCode < 400 {
			meta["hint"] = "Now run commands with --workspace " + workspaceID + " (or export BREYTA_WORKSPACE=" + workspaceID + ")"
		}
		_ = writeOut(cmd, app, map[string]any{
			"ok":          resp.StatusCode < 400,
			"workspaceId": workspaceID,
			"data":        out,
			"meta":        meta,
		})
		if resp.StatusCode >= 400 {
			return errors.New("api error")
		}
		return nil
	}}
	bootstrapCmd.Flags().StringVar(&bootstrapName, "name", "", "Workspace name (optional)")
	cmd.AddCommand(bootstrapCmd)
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
