package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/breyta/breyta-cli/internal/configstore"

	"github.com/spf13/cobra"
)

func newAPICmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Configure API base URL",
	}
	cmd.AddCommand(newAPIShowCmd(app))
	cmd.AddCommand(newAPIUseCmd(app))
	cmd.AddCommand(newAPICheckCmd(app))
	return cmd
}

func newAPIShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show effective API base URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configstore.DefaultPath()
			if err != nil {
				return writeErr(cmd, err)
			}
			st, loadErr := configstore.Load(path)
			meta := map[string]any{
				"storePath": path,
				"stored":    loadErr == nil,
			}
			storedURL := ""
			if st != nil {
				storedURL = strings.TrimRight(strings.TrimSpace(st.APIURL), "/")
			}
			switch {
			case flagExplicit(cmd, "api"):
				meta["source"] = "flag"
			case strings.TrimSpace(os.Getenv("BREYTA_API_URL")) != "":
				meta["source"] = "env"
			case storedURL != "":
				meta["source"] = "store"
			default:
				meta["source"] = "default"
			}
			if storedURL != "" && storedURL != app.APIURL {
				meta["storedApiUrl"] = storedURL
			}
			return writeData(cmd, app, meta, map[string]any{
				"apiUrl": app.APIURL,
			})
		},
	}
}

func newAPIUseCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <local|url>",
		Short: "Switch API base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := strings.TrimSpace(args[0])
			if target == "" {
				return writeErr(cmd, errors.New("missing target"))
			}

			var apiURL string
			switch strings.ToLower(target) {
			case "local":
				apiURL = configstore.DefaultLocalAPIURL
			default:
				apiURL = target
			}
			apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
			if apiURL == "" {
				return writeErr(cmd, errors.New("invalid api url"))
			}
			if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
				return writeErr(cmd, fmt.Errorf("invalid api url (expected http/https): %s", apiURL))
			}

			path, err := configstore.DefaultPath()
			if err != nil {
				return writeErr(cmd, err)
			}
			st := &configstore.Store{APIURL: apiURL}
			if err := configstore.SaveAtomic(path, st); err != nil {
				return writeErr(cmd, err)
			}

			meta := map[string]any{
				"storePath": path,
				"stored":    true,
				"hint":      "You can still override per-run via --api or BREYTA_API_URL.",
			}
			if envURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); envURL != "" {
				envURL = strings.TrimRight(envURL, "/")
				if envURL != apiURL {
					meta["warning"] = "BREYTA_API_URL is set and will override this config in your current shell."
					meta["unsetEnv"] = "unset BREYTA_API_URL"
				}
			}
			return writeData(cmd, app, meta, map[string]any{"apiUrl": apiURL})
		},
	}
	return cmd
}
