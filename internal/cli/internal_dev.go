package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/spf13/cobra"
)

func newInternalCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal CLI helpers",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newInternalDevCmd(app))
	return cmd
}

func newInternalDevCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dev",
		Short:  "Manage internal dev-mode settings",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newInternalDevEnableCmd(app))
	cmd.AddCommand(newInternalDevDisableCmd(app))
	cmd.AddCommand(newInternalDevShowCmd(app))
	cmd.AddCommand(newInternalDevSetCmd(app))
	cmd.AddCommand(newInternalDevUseCmd(app))
	cmd.AddCommand(newInternalDevListCmd(app))
	return cmd
}

func newInternalDevEnableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "enable",
		Short:  "Enable internal dev mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			st.DevMode = true
			if err := configstore.SaveAtomic(path, st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{"devMode": true})
		},
	}
	return cmd
}

func newInternalDevDisableCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "disable",
		Short:  "Disable internal dev mode",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			st.DevMode = false
			if err := configstore.SaveAtomic(path, st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{"devMode": false})
		},
	}
	return cmd
}

func newInternalDevShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "show",
		Short:  "Show internal dev-mode settings",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			activeName := strings.TrimSpace(st.DevActive)
			activeProfile := st.DevProfiles[activeName]
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{
				"devMode":          st.DevMode,
				"devActive":        activeName,
				"devApiUrl":        activeProfile.APIURL,
				"devWorkspaceId":   activeProfile.WorkspaceID,
				"devTokenSet":      strings.TrimSpace(activeProfile.Token) != "",
				"devRunConfigId":   activeProfile.RunConfigID,
				"devAuthStorePath": activeProfile.AuthStorePath,
			})
		},
	}
	return cmd
}

func newInternalDevSetCmd(app *App) *cobra.Command {
	var apiURL string
	var workspaceID string
	var token string
	var runConfigID string
	var authStorePath string

	cmd := &cobra.Command{
		Use:    "set <name>",
		Short:  "Set internal dev-mode settings",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return writeErr(cmd, errors.New("missing name"))
			}
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			prof := st.DevProfiles[name]
			if strings.TrimSpace(apiURL) != "" {
				prof.APIURL = strings.TrimSpace(apiURL)
			}
			if strings.TrimSpace(workspaceID) != "" {
				prof.WorkspaceID = strings.TrimSpace(workspaceID)
			}
			if strings.TrimSpace(token) != "" {
				prof.Token = strings.TrimSpace(token)
			}
			if strings.TrimSpace(runConfigID) != "" {
				prof.RunConfigID = strings.TrimSpace(runConfigID)
			}
			if strings.TrimSpace(authStorePath) != "" {
				prof.AuthStorePath = strings.TrimSpace(authStorePath)
			}
			st.DevProfiles[name] = prof
			if strings.TrimSpace(st.DevActive) == "" {
				st.DevActive = name
			}
			if err := configstore.SaveAtomic(path, st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{
				"devMode":          st.DevMode,
				"devActive":        st.DevActive,
				"devName":          name,
				"devApiUrl":        prof.APIURL,
				"devWorkspaceId":   prof.WorkspaceID,
				"devTokenSet":      strings.TrimSpace(prof.Token) != "",
				"devRunConfigId":   prof.RunConfigID,
				"devAuthStorePath": prof.AuthStorePath,
			})
		},
	}
	cmd.Flags().StringVar(&apiURL, "api", "", "Dev API base URL")
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "Dev workspace id")
	cmd.Flags().StringVar(&token, "token", "", "Dev API token")
	cmd.Flags().StringVar(&runConfigID, "runcfg", "", "Dev run config id")
	cmd.Flags().StringVar(&authStorePath, "auth-store", "", "Dev auth store path")
	return cmd
}

func newInternalDevUseCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "use <name>",
		Short:  "Set the active dev profile",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if name == "" {
				return writeErr(cmd, errors.New("missing name"))
			}
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			if _, ok := st.DevProfiles[name]; !ok {
				return writeErr(cmd, errors.New("unknown dev profile"))
			}
			st.DevActive = name
			if err := configstore.SaveAtomic(path, st); err != nil {
				return writeErr(cmd, err)
			}
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{"devActive": name})
		},
	}
	return cmd
}

func newInternalDevListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "list",
		Short:  "List configured dev profiles",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, path, err := loadConfigStore()
			if err != nil {
				return writeErr(cmd, err)
			}
			items := []map[string]any{}
			for name, prof := range st.DevProfiles {
				items = append(items, map[string]any{
					"name":          name,
					"apiUrl":        prof.APIURL,
					"workspaceId":   prof.WorkspaceID,
					"tokenSet":      strings.TrimSpace(prof.Token) != "",
					"runConfigId":   prof.RunConfigID,
					"authStorePath": prof.AuthStorePath,
					"active":        name == st.DevActive,
				})
			}
			return writeData(cmd, app, map[string]any{"path": path}, map[string]any{"items": items})
		},
	}
	return cmd
}

func loadConfigStore() (*configstore.Store, string, error) {
	path, err := configstore.DefaultPath()
	if err != nil {
		return nil, "", err
	}
	st, err := configstore.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &configstore.Store{}, path, nil
		}
		return nil, path, err
	}
	return st, path, nil
}

func loadDevConfig(app *App) (*configstore.Store, bool) {
	st, _, err := loadConfigStore()
	if err != nil || st == nil {
		return nil, false
	}
	if !st.DevMode && (app == nil || !app.DevMode) {
		return nil, false
	}
	if len(st.DevProfiles) == 0 {
		st.DevProfiles = map[string]configstore.DevProfile{}
	}
	if strings.TrimSpace(st.DevActive) == "" && len(st.DevProfiles) > 0 {
		for name := range st.DevProfiles {
			st.DevActive = name
			break
		}
	}
	if strings.TrimSpace(st.DevActive) == "" {
		if _, ok := st.DevProfiles["local"]; ok {
			st.DevActive = "local"
		}
	}
	return st, true
}

func resolveDevProfile(app *App, st *configstore.Store) (string, configstore.DevProfile, error) {
	if st == nil {
		return "", configstore.DevProfile{}, errors.New("missing dev config")
	}
	name := strings.TrimSpace(st.DevActive)
	if app != nil {
		if override := strings.TrimSpace(app.DevProfileOverride); override != "" {
			name = override
		}
	}
	if name == "" {
		if _, ok := st.DevProfiles["local"]; ok {
			name = "local"
		}
	}
	if name == "" && len(st.DevProfiles) > 0 {
		for candidate := range st.DevProfiles {
			name = candidate
			break
		}
	}
	if name == "" {
		return "", configstore.DevProfile{}, nil
	}
	prof, ok := st.DevProfiles[name]
	if !ok {
		if app != nil && strings.TrimSpace(app.DevProfileOverride) != "" && len(st.DevProfiles) == 0 {
			return "", configstore.DevProfile{}, nil
		}
		return "", configstore.DevProfile{}, fmt.Errorf("unknown dev profile: %s", name)
	}
	return name, prof, nil
}

func devModeEnabled() bool {
	st, _, err := loadConfigStore()
	if err != nil || st == nil {
		return false
	}
	return st.DevMode
}

func loadRunConfigID(app *App) string {
	st, ok := loadDevConfig(app)
	if !ok {
		return ""
	}
	_, prof, err := resolveDevProfile(app, st)
	if err != nil {
		return ""
	}
	cfgAPI := strings.TrimRight(strings.TrimSpace(prof.APIURL), "/")
	appAPI := strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
	if cfgAPI != "" && appAPI != "" && cfgAPI != appAPI {
		return ""
	}
	return strings.TrimSpace(prof.RunConfigID)
}

func resolveAuthStorePath(app *App) string {
	if app != nil && app.DevMode {
		if st, ok := loadDevConfig(app); ok {
			_, prof, err := resolveDevProfile(app, st)
			if err == nil {
				if p := strings.TrimSpace(prof.AuthStorePath); p != "" {
					return p
				}
			}
		}
	}
	if p := strings.TrimSpace(os.Getenv("BREYTA_AUTH_STORE")); p != "" {
		return p
	}
	p, _ := authstore.DefaultPath()
	return strings.TrimSpace(p)
}
