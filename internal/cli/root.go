package cli

import (
        "errors"
        "fmt"
        "os"

        "breyta-cli/internal/format"
        "breyta-cli/internal/mock"
        "breyta-cli/internal/state"
        "breyta-cli/internal/tui"

        "github.com/spf13/cobra"
)

type App struct {
        WorkspaceID string
        StatePath   string
        PrettyJSON  bool
        Format      string
        APIURL      string
        Token       string
        Profile     string
        DevMode     bool
}

func NewRootCmd() *cobra.Command {
        app := &App{}

        cmd := &cobra.Command{
                Use:          "breyta",
                Short:        "Breyta CLI (mock)",
                SilenceUsage: true,
                RunE: func(cmd *cobra.Command, args []string) error {
                        // No subcommand => interactive TUI.
                        if cmd.HasSubCommands() && len(args) == 0 {
                                return runTUI(app)
                        }
                        return cmd.Help()
                },
        }

        cmd.PersistentFlags().StringVar(&app.WorkspaceID, "workspace", envOr("BREYTA_WORKSPACE", "demo-workspace"), "Workspace id")
        cmd.PersistentFlags().BoolVar(&app.PrettyJSON, "pretty", false, "Pretty-print JSON output")
        cmd.PersistentFlags().StringVar(&app.Format, "format", envOr("BREYTA_FORMAT", "json"), "Output format (json|edn)")
        cmd.PersistentFlags().StringVar(&app.APIURL, "api", envOr("BREYTA_API_URL", ""), "API base URL (e.g. https://api.breyta.com)")
        cmd.PersistentFlags().StringVar(&app.Token, "token", envOr("BREYTA_TOKEN", ""), "API token (or set BREYTA_TOKEN)")
        cmd.PersistentFlags().StringVar(&app.Profile, "profile", envOr("BREYTA_PROFILE", ""), "Config profile name")
        cmd.PersistentFlags().BoolVar(&app.DevMode, "dev", envOr("BREYTA_DEV", "") == "1", "Enable dev-only commands")

        defaultPath, _ := state.DefaultPath()
        cmd.PersistentFlags().StringVar(&app.StatePath, "state", envOr("BREYTA_MOCK_STATE", defaultPath), "Path to mock state JSON")

        cmd.AddCommand(newFlowsCmd(app))
        cmd.AddCommand(newRunsCmd(app))
        cmd.AddCommand(newConnectionsCmd(app))
        cmd.AddCommand(newInstancesCmd(app))
        cmd.AddCommand(newTriggersCmd(app))
        cmd.AddCommand(newWaitsCmd(app))
        cmd.AddCommand(newWatchCmd(app))
        cmd.AddCommand(newRegistryCmd(app))
        cmd.AddCommand(newAuthCmd(app))
        cmd.AddCommand(newWorkspacesCmd(app))
        cmd.AddCommand(newDevCmd(app))
        cmd.AddCommand(newRevenueCmd(app))
        cmd.AddCommand(newDemandCmd(app))
        cmd.AddCommand(newDocsCmd(cmd, app))

        return cmd
}

func runTUI(app *App) error {
        st, store, err := appStore(app)
        if err != nil {
                return err
        }
        return tui.Run(app.WorkspaceID, app.StatePath, store, st)
}

func appStore(app *App) (*state.State, mock.Store, error) {
        if app.StatePath == "" {
                return nil, mock.Store{}, errors.New("missing --state")
        }
        store := mock.Store{Path: app.StatePath, WorkspaceID: app.WorkspaceID}
        st, err := store.Ensure()
        if err != nil {
                return nil, store, err
        }
        return st, store, nil
}

func envOr(k, d string) string {
        if v := os.Getenv(k); v != "" {
                return v
        }
        return d
}

func must(err error) {
        if err != nil {
                panic(err)
        }
}

func writeOut(cmd *cobra.Command, app *App, v any) error {
        return format.Write(cmd.OutOrStdout(), v, app.Format, app.PrettyJSON)
}

func writeErr(cmd *cobra.Command, err error) error {
        fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
        return err
}
