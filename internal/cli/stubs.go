package cli

import (
        "errors"
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

// --- Instances --------------------------------------------------------------

func newInstancesCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "instances", Short: "Manage flow instances (activation/bindings)"}
        cmd.AddCommand(newInstancesListCmd(app))
        cmd.AddCommand(newInstancesShowCmd(app))
        cmd.AddCommand(newInstancesCreateCmd(app))
        cmd.AddCommand(newInstancesDeleteCmd(app))

        bindings := &cobra.Command{Use: "bindings", Short: "Manage instance bindings"}
        bindings.AddCommand(newInstancesBindingsSetCmd(app))
        cmd.AddCommand(bindings)

        return cmd
}

func newInstancesListCmd(app *App) *cobra.Command {
        var flow string
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List instances",
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, _, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        ws, err := getWorkspace(st, app.WorkspaceID)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        items := make([]*state.Instance, 0)
                        for _, it := range ws.Instances {
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

func newInstancesShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "show <id>",
                Short: "Show instance",
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
                        it := ws.Instances[args[0]]
                        if it == nil {
                                return writeErr(cmd, errors.New("instance not found"))
                        }
                        return writeData(cmd, app, nil, map[string]any{"instance": it})
                },
        }
        return cmd
}

func newInstancesCreateCmd(app *App) *cobra.Command {
        var flow, name string
        var version int
        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create instance",
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
                        if ws.Instances == nil {
                                ws.Instances = map[string]*state.Instance{}
                        }
                        id := "inst-" + time.Now().UTC().Format("20060102-150405")
                        if name == "" {
                                name = flow
                        }
                        now := time.Now().UTC()
                        it := &state.Instance{ID: id, FlowSlug: flow, Version: version, Name: name, Enabled: true, AutoUpgrade: true, UpdatedAt: now}
                        ws.Instances[id] = it
                        ws.UpdatedAt = now
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeData(cmd, app, nil, map[string]any{"instance": it})
                },
        }
        cmd.Flags().StringVar(&flow, "flow", "", "Flow slug")
        cmd.Flags().IntVar(&version, "version", 0, "Version")
        cmd.Flags().StringVar(&name, "name", "", "Instance name")
        return cmd
}

func newInstancesBindingsSetCmd(app *App) *cobra.Command {
        var bindings string
        cmd := &cobra.Command{
                Use:   "set <id>",
                Short: "Set instance bindings",
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
                        it := ws.Instances[args[0]]
                        if it == nil {
                                return writeErr(cmd, errors.New("instance not found"))
                        }
                        it.Bindings = map[string]any{"raw": bindings}
                        it.UpdatedAt = time.Now().UTC()
                        ws.UpdatedAt = it.UpdatedAt
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeData(cmd, app, nil, map[string]any{"instance": it})
                },
        }
        cmd.Flags().StringVar(&bindings, "bindings", "", "Bindings (mock string)")
        return cmd
}

func newInstancesDeleteCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "delete <id>",
                Short: "Delete instance",
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
                        if ws.Instances[args[0]] == nil {
                                return writeErr(cmd, errors.New("instance not found"))
                        }
                        delete(ws.Instances, args[0])
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
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List triggers",
                RunE: func(cmd *cobra.Command, args []string) error {
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
        return cmd
}

func newTriggersFireCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "fire <trigger-id>",
                Short: "Fire a trigger (mock)",
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
        return cmd
}

// --- Waits -------------------------------------------------------------------

func newWaitsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "waits", Short: "Manage waits"}
        cmd.AddCommand(newWaitsListCmd(app))
        cmd.AddCommand(newWaitsShowCmd(app))
        cmd.AddCommand(newWaitsCompleteCmd(app))
        cmd.AddCommand(newWaitsCancelCmd(app))
        return cmd
}

func newWaitsListCmd(app *App) *cobra.Command {
        var runID string
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List waits",
                RunE: func(cmd *cobra.Command, args []string) error {
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
        return cmd
}

func newWaitsShowCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "show <wait-id>", Short: "Show wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
        cmd.Flags().StringVar(&payload, "payload", "", "Payload (mock string)")
        return cmd
}

func newWaitsCancelCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "cancel <wait-id>", Short: "Cancel wait", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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

// --- Watch/Registry/Auth/Workspaces (mock placeholders) ----------------------

func newWatchCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "watch", Short: "Stream live updates"}
        cmd.RunE = func(cmd *cobra.Command, args []string) error {
                return writeNotImplemented(cmd, app, "Mock: watch will tail SSE/event streams")
        }
        return cmd
}

func newRegistryCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "registry", Short: "Marketplace registry"}
        cmd.AddCommand(&cobra.Command{Use: "search <query>", Short: "Search registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
                return writeData(cmd, app, map[string]any{"hint": "Registry is mocked minimally in v1; no items yet"}, map[string]any{"items": []any{}})
        }})
        cmd.AddCommand(&cobra.Command{Use: "show <ref>", Short: "Show registry entry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
                return writeNotImplemented(cmd, app, "Registry show not mocked yet")
        }})
        cmd.AddCommand(&cobra.Command{Use: "install <ref>", Short: "Install from registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
                return writeNotImplemented(cmd, app, "Registry install not mocked yet")
        }})
        cmd.AddCommand(&cobra.Command{Use: "publish <flow-slug>", Short: "Publish to registry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
                return writeNotImplemented(cmd, app, "Registry publish not mocked yet")
        }})
        return cmd
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
