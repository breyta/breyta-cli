package cli

import (
	"github.com/breyta/breyta-cli/internal/mock"
	"github.com/breyta/breyta-cli/internal/state"

	"github.com/spf13/cobra"
)

func newDevCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "dev", Short: "Dev helpers", Hidden: !app.DevMode && !devModeEnabled()}
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
			return writeData(cmd, app, nil, map[string]any{
				"status": "ok",
				"tick":   st.Tick,
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
			return writeData(cmd, app, nil, map[string]any{
				"tick":   st.Tick,
				"status": "ok",
			})
		},
	}
	cmd.Flags().IntVar(&ticks, "ticks", 1, "How many ticks to advance")
	return cmd
}
