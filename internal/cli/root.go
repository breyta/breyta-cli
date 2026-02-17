package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/breyta/breyta-cli/internal/format"
	"github.com/breyta/breyta-cli/internal/mock"
	"github.com/breyta/breyta-cli/internal/skillsync"
	"github.com/breyta/breyta-cli/internal/state"
	"github.com/breyta/breyta-cli/internal/updatecheck"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type App struct {
	WorkspaceID          string
	StatePath            string
	PrettyJSON           bool
	APIURL               string
	Token                string
	TokenExplicit        bool
	Profile              string
	DevMode              bool
	DevFlag              string
	DevProfileOverride   string
	visibilityConfigured bool

	updateNotice *updatecheck.Notice
	updateCh     <-chan *updatecheck.Notice
}

func NewRootCmd() *cobra.Command {
	app := &App{}

	cmd := &cobra.Command{
		Use:          "breyta",
		Short:        "Breyta CLI",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&app.WorkspaceID, "workspace", envOr("BREYTA_WORKSPACE", ""), "Workspace id")
	cmd.PersistentFlags().BoolVar(&app.PrettyJSON, "pretty", false, "Pretty-print JSON output")
	cmd.PersistentFlags().StringVar(&app.APIURL, "api", "", "API base URL (e.g. https://flows.breyta.ai)")
	cmd.PersistentFlags().StringVar(&app.Token, "token", "", "API token")
	cmd.PersistentFlags().StringVar(&app.Profile, "profile", envOr("BREYTA_PROFILE", ""), "Config profile name")
	cmd.PersistentFlags().StringVar(&app.DevFlag, "dev", "", "Enable dev-only commands (optional profile name)")
	if f := cmd.PersistentFlags().Lookup("dev"); f != nil {
		f.NoOptDefVal = "true"
	}

	// Ensure dev-only flags and commands remain hidden in help output unless explicitly enabled.
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		configureVisibility(cmd, app)
		configureFlagVisibility(cmd, app)
		defaultHelp(c, args)
		_, _ = fmt.Fprintf(c.OutOrStdout(), "\nDocs: %s\nHelp: %s\n", docsHintForCommand(c), helpHintForCommand(c))
	})
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Parse-time: app.DevMode is set from flags/config. Hide dev-only controls unless explicitly enabled.
		devFlagExplicit := false
		if cmd != nil {
			devFlagExplicit = cmd.Flags().Changed("dev") || cmd.InheritedFlags().Changed("dev")
			if root := cmd.Root(); root != nil {
				devFlagExplicit = devFlagExplicit || root.PersistentFlags().Changed("dev")
			}
		}
		if devFlagExplicit {
			val := strings.TrimSpace(app.DevFlag)
			switch strings.ToLower(val) {
			case "", "true", "1", "yes", "y", "on":
				app.DevMode = true
				app.DevProfileOverride = ""
			case "false", "0", "no", "n", "off":
				app.DevMode = false
				app.DevProfileOverride = ""
			default:
				app.DevMode = true
				app.DevProfileOverride = val
			}
		}
		if !app.DevMode {
			if st, ok := loadDevConfig(app); ok && st.DevMode {
				app.DevMode = true
			}
		}
		configureFlagVisibility(cmd.Root(), app)

		// Default workspace id:
		// - explicit --workspace / BREYTA_WORKSPACE wins
		// - otherwise try ~/.config/breyta/config.json (workspaceId), but only when the
		//   config's apiUrl matches the active API URL (prevents local mock workspace ids
		//   leaking into prod).
		workspaceFlagExplicit := false
		if cmd != nil {
			workspaceFlagExplicit = cmd.Flags().Changed("workspace") || cmd.InheritedFlags().Changed("workspace")
			if root := cmd.Root(); root != nil {
				workspaceFlagExplicit = workspaceFlagExplicit || root.PersistentFlags().Changed("workspace")
			}
		}
		workspaceEnvExplicit := strings.TrimSpace(os.Getenv("BREYTA_WORKSPACE")) != ""

		// Default API URL:
		// - explicit --api wins (dev mode only)
		// - otherwise if dev mode and BREYTA_API_URL set, use it
		// - otherwise: try ~/.config/breyta/config.json (if present)
		// - otherwise fall back to prod (https://flows.breyta.ai)
		//
		// IMPORTANT: We only default when a subcommand is invoked.
		apiFlagExplicit := false
		if cmd != nil {
			// Respect an explicitly passed `--api` even if it's empty (tests and mock mode).
			apiFlagExplicit = cmd.Flags().Changed("api") || cmd.InheritedFlags().Changed("api")
			if root := cmd.Root(); root != nil {
				apiFlagExplicit = apiFlagExplicit || root.PersistentFlags().Changed("api")
			}
		}
		// NOTE: `args` here are the positional args to the *invoked* command, not a signal
		// of whether a subcommand is being executed. For commands like `breyta auth login`,
		// args is usually empty, so we must detect subcommand execution via cmd != cmd.Root().
		isSubcommand := cmd != nil && cmd.Root() != nil && cmd != cmd.Root()
		if isSubcommand {
			if !app.DevMode && apiFlagExplicit {
				return writeErr(cmd, errors.New("--api override is disabled"))
			}
			if app.DevMode && !apiFlagExplicit && strings.TrimSpace(app.APIURL) == "" {
				if st, ok := loadDevConfig(app); ok {
					_, prof, err := resolveDevProfile(app, st)
					if err != nil {
						return writeErr(cmd, err)
					}
					if strings.TrimSpace(prof.APIURL) != "" {
						app.APIURL = strings.TrimSpace(prof.APIURL)
					}
				}
				if strings.TrimSpace(app.APIURL) == "" {
					if envURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); envURL != "" {
						app.APIURL = envURL
					}
				}
			}
			if !apiFlagExplicit && strings.TrimSpace(app.APIURL) == "" {
				if p, err := configstore.DefaultPath(); err == nil && p != "" {
					if st, err := configstore.Load(p); err == nil && st != nil && strings.TrimSpace(st.APIURL) != "" {
						app.APIURL = st.APIURL
					}
				}
				if strings.TrimSpace(app.APIURL) == "" {
					app.APIURL = configstore.DefaultProdAPIURL
				}
			}
		}

		if !workspaceFlagExplicit && !workspaceEnvExplicit && strings.TrimSpace(app.WorkspaceID) == "" {
			if app.DevMode {
				if st, ok := loadDevConfig(app); ok {
					_, prof, err := resolveDevProfile(app, st)
					if err != nil {
						return writeErr(cmd, err)
					}
					if strings.TrimSpace(prof.WorkspaceID) != "" {
						app.WorkspaceID = strings.TrimSpace(prof.WorkspaceID)
					}
				}
			}
			// Only apply default workspace when config apiUrl matches current api url.
			if p, err := configstore.DefaultPath(); err == nil && p != "" {
				if st, err := configstore.Load(p); err == nil && st != nil && strings.TrimSpace(st.WorkspaceID) != "" {
					cfgAPI := strings.TrimRight(strings.TrimSpace(st.APIURL), "/")
					appAPI := strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
					if cfgAPI != "" && appAPI != "" && cfgAPI == appAPI {
						app.WorkspaceID = st.WorkspaceID
					}
				}
			}
		}

		tokenFlagExplicit := false
		if cmd != nil {
			tokenFlagExplicit = cmd.Flags().Changed("token") || cmd.InheritedFlags().Changed("token")
			if root := cmd.Root(); root != nil {
				tokenFlagExplicit = tokenFlagExplicit || root.PersistentFlags().Changed("token")
			}
		}
		tokenEnvExplicit := app.DevMode && strings.TrimSpace(os.Getenv("BREYTA_TOKEN")) != ""
		tokenExplicit := tokenFlagExplicit || tokenEnvExplicit
		if !app.DevMode && tokenFlagExplicit {
			return writeErr(cmd, errors.New("--token override is disabled; use `breyta auth login` instead"))
		}
		if app.DevMode && !tokenFlagExplicit && strings.TrimSpace(app.Token) == "" {
			if st, ok := loadDevConfig(app); ok {
				_, prof, err := resolveDevProfile(app, st)
				if err != nil {
					return writeErr(cmd, err)
				}
				if strings.TrimSpace(prof.Token) != "" {
					app.Token = strings.TrimSpace(prof.Token)
				}
			}
		}
		if tokenEnvExplicit && strings.TrimSpace(app.Token) == "" {
			app.Token = strings.TrimSpace(os.Getenv("BREYTA_TOKEN"))
		}
		app.TokenExplicit = tokenExplicit

		// If token isn't explicitly provided, load it from the local auth store and refresh if expiring.
		// This enables: `breyta auth login` once, then normal `breyta ...` commands with auto-refresh.
		if !tokenExplicit && strings.TrimSpace(app.APIURL) != "" {
			loadTokenFromAuthStore(app)
		}
		configureVisibility(cmd.Root(), app)

		// Best-effort: keep already-installed agent skill bundles in sync with this CLI version.
		// Run this asynchronously so command startup is never blocked by network issues.
		skillsync.MaybeSyncInstalledAsync(buildinfo.DisplayVersion(), app.APIURL, app.Token)

		// Best-effort update check for JSON commands. Never blocks command execution.
		if isSubcommand {
			app.startUpdateCheckNonBlocking(context.Background(), 24*time.Hour)
		}
		return nil
	}

	defaultPath, _ := state.DefaultPath()
	cmd.PersistentFlags().StringVar(&app.StatePath, "state", envOr("BREYTA_MOCK_STATE", defaultPath), "Path to mock state JSON")

	cmd.AddCommand(newFlowsCmd(app))
	cmd.AddCommand(newRunsCmd(app))
	cmd.AddCommand(newStepsCmd(app))
	cmd.AddCommand(newConnectionsCmd(app))
	cmd.AddCommand(newSecretsCmd(app))
	cmd.AddCommand(newProfilesCmd(app))
	cmd.AddCommand(newTriggersCmd(app))
	cmd.AddCommand(newWebhooksCmd(app))
	cmd.AddCommand(newResourcesCmd(app))
	cmd.AddCommand(newDebugCmd(app))
	cmd.AddCommand(newWaitsCmd(app))
	cmd.AddCommand(newWatchCmd(app))
	cmd.AddCommand(newRegistryCmd(app))
	cmd.AddCommand(newPricingCmd(app))
	cmd.AddCommand(newPurchasesCmd(app))
	cmd.AddCommand(newEntitlementsCmd(app))
	cmd.AddCommand(newPayoutsCmd(app))
	cmd.AddCommand(newCreatorCmd(app))
	cmd.AddCommand(newAnalyticsCmd(app))
	cmd.AddCommand(newAuthCmd(app))
	cmd.AddCommand(newAPICmd(app))
	cmd.AddCommand(newWorkspacesCmd(app))
	cmd.AddCommand(newSkillsCmd(app))
	cmd.AddCommand(newInitCmd(app))
	cmd.AddCommand(newDevCmd(app))
	cmd.AddCommand(newRevenueCmd(app))
	cmd.AddCommand(newDemandCmd(app))
	cmd.AddCommand(newDocsCmd(app))
	cmd.AddCommand(newFeedbackCmd(app))
	cmd.AddCommand(newVersionCmd(app))
	cmd.AddCommand(newUpgradeCmd(app))
	cmd.AddCommand(newInternalCmd(app))

	return cmd
}

func appStore(app *App) (*state.State, mock.Store, error) {
	if isAPIMode(app) {
		return nil, mock.Store{}, errors.New("mock state is disabled in API mode (use API commands, or pass --api= to force mock mode)")
	}
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

func writeOut(cmd *cobra.Command, app *App, v any) error {
	return format.WriteJSON(cmd.OutOrStdout(), v, app.PrettyJSON)
}

func writeErr(cmd *cobra.Command, err error) error {
	if cmd == nil {
		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintf(os.Stderr, "Hint: run `%s` for usage or `%s` for docs.\n", helpHintForCommand(nil), docsHintForCommand(nil))
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Hint: run `%s` for usage or `%s` for docs.\n", helpHintForCommand(cmd), docsHintForCommand(cmd))
	return err
}

func helpHintForCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return "breyta help"
	}
	path := strings.TrimSpace(cmd.CommandPath())
	if path == "" {
		if root := cmd.Root(); root != nil && strings.TrimSpace(root.Name()) != "" {
			path = strings.TrimSpace(root.Name())
		} else {
			path = "breyta"
		}
	}
	rootName := "breyta"
	if cmd.Root() != nil && strings.TrimSpace(cmd.Root().Name()) != "" {
		rootName = strings.TrimSpace(cmd.Root().Name())
	}
	tail := strings.TrimSpace(strings.TrimPrefix(path, rootName))
	if tail == "" {
		return rootName + " help"
	}
	return strings.TrimSpace(rootName + " help " + tail)
}

func docsHintForCommand(cmd *cobra.Command) string {
	rootName := "breyta"
	if cmd != nil {
		if root := cmd.Root(); root != nil && strings.TrimSpace(root.Name()) != "" {
			rootName = strings.TrimSpace(root.Name())
		}
	}
	if cmd == nil {
		return rootName + " docs find \"<topic>\""
	}

	path := strings.TrimSpace(cmd.CommandPath())
	if path == "" {
		return rootName + " docs find \"<topic>\""
	}
	tail := strings.TrimSpace(strings.TrimPrefix(path, rootName))
	if tail == "" || strings.HasPrefix(tail, "docs") {
		return rootName + " docs find \"<topic>\""
	}
	return fmt.Sprintf("%s docs find %q", rootName, tail)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
