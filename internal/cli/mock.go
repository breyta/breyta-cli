package cli

import (
        "breyta-cli/internal/mock"
        "breyta-cli/internal/state"

        "github.com/spf13/cobra"
)

func newMockCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{Use: "mock", Short: "Mock helpers"}
        cmd.AddCommand(newMockSeedCmd(app))
        cmd.AddCommand(newMockAdvanceCmd(app))
        return cmd
}

func newMockSeedCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "seed",
                Short: "Reset mock state to seeded defaults",
                RunE: func(cmd *cobra.Command, args []string) error {
                        store := mock.Store{Path: app.StatePath, WorkspaceID: app.WorkspaceID}
                        st := state.SeedDefault(app.WorkspaceID)
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "status":      "ok",
                                "tick":        st.Tick,
                        })
                },
        }
        return cmd
}

func newMockAdvanceCmd(app *App) *cobra.Command {
        var ticks int
        cmd := &cobra.Command{
                Use:   "advance",
                Short: "Advance mock executions forward",
                RunE: func(cmd *cobra.Command, args []string) error {
                        st, store, err := appStore(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := store.Advance(st, ticks); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := store.Save(st); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "workspaceId": app.WorkspaceID,
                                "tick":        st.Tick,
                                "status":      "ok",
                        })
                },
        }
        cmd.Flags().IntVar(&ticks, "ticks", 1, "How many ticks to advance")
        return cmd
}
