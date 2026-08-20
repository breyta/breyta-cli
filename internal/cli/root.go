package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/breyta/breyta-cli/internal/format"
	"github.com/breyta/breyta-cli/internal/mock"
	"github.com/breyta/breyta-cli/internal/state"
	"github.com/breyta/breyta-cli/internal/updatecheck"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type App struct {
	WorkspaceID          string
	StatePath            string
	PrettyJSON           bool
	APIURL               string
	HTTP                 *http.Client
	Token                string
	APIKey               string
	TokenExplicit        bool
	APIKeyExplicit       bool
	Profile              string
	visibilityConfigured bool

	updateNotice        *updatecheck.Notice
	updateCh            <-chan *updatecheck.Notice
	updateReminderShown bool
}

type guidedCLIError struct {
	message string
}

const allowAPIEnvOverrideAnnotation = "allow_api_env_override"

func withPublicFlagHelpValues(cmd *cobra.Command, help func(*cobra.Command, []string), args []string) {
	if cmd == nil || help == nil {
		help(cmd, args)
		return
	}
	originals := map[string]string{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.NoOptDefVal == cliBareTrueValue {
			originals[flag.Name] = flag.NoOptDefVal
			flag.NoOptDefVal = "true"
		}
	})
	defer func() {
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if original, ok := originals[flag.Name]; ok {
				flag.NoOptDefVal = original
			}
		})
	}()
	help(cmd, args)
}

func (e *guidedCLIError) Error() string {
	return strings.TrimSpace(e.message)
}

func commandAllowsAPIEnvOverride(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations != nil && strings.EqualFold(strings.TrimSpace(current.Annotations[allowAPIEnvOverrideAnnotation]), "true") {
			return true
		}
	}
	return false
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
	cmd.SetHelpCommand(newHelpCmd(app, cmd))

	cmd.PersistentFlags().StringVar(&app.WorkspaceID, "workspace", envOr("BREYTA_WORKSPACE", ""), "Workspace id")
	cmd.PersistentFlags().BoolVar(&app.PrettyJSON, "pretty", false, "Pretty-print JSON output")
	cmd.PersistentFlags().StringVar(&app.APIURL, "api", "", "API base URL (e.g. http://localhost:8090)")
	cmd.PersistentFlags().StringVar(&app.Token, "token", "", "API token")
	cmd.PersistentFlags().StringVar(&app.APIKey, "api-key", "", "Service account API key")
	cmd.PersistentFlags().StringVar(&app.Profile, "profile", envOr("BREYTA_PROFILE", ""), "Config profile name")
	// Keep help output aligned with the canonical command tree.
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		target := c
		if target == nil {
			target = cmd
		}
		if root := target.Root(); root != nil {
			target = root
		}
		configureVisibility(target, app)
		configureFlagVisibility(target, app)
		withPublicFlagHelpValues(c, defaultHelp, args)
		more := moreHintForCommand(c)
		if more != "" {
			_, _ = fmt.Fprintf(c.OutOrStdout(), "\nDocs: %s\nMore: %s\nHelp: %s\n", docsHintForCommand(c), more, helpHintForCommand(c))
		} else {
			_, _ = fmt.Fprintf(c.OutOrStdout(), "\nDocs: %s\nHelp: %s\n", docsHintForCommand(c), helpHintForCommand(c))
		}
	})
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		apiFlagExplicit := flagExplicit(cmd, "api")
		if !apiFlagExplicit {
			if apiURL := strings.TrimSpace(os.Getenv("BREYTA_API_URL")); apiURL != "" {
				app.APIURL = apiURL
			}
		}
		if strings.TrimSpace(app.APIURL) == "" {
			if path, err := configstore.DefaultPath(); err == nil {
				if stored, err := configstore.Load(path); err == nil && stored != nil {
					app.APIURL = strings.TrimSpace(stored.APIURL)
				}
			}
		}
		if strings.TrimSpace(app.APIURL) == "" {
			app.APIURL = configstore.DefaultAPIURL
		}
		app.APIURL = strings.TrimRight(strings.TrimSpace(app.APIURL), "/")

		workspaceExplicit := flagExplicit(cmd, "workspace") || strings.TrimSpace(os.Getenv("BREYTA_WORKSPACE")) != ""
		if !workspaceExplicit && strings.TrimSpace(app.WorkspaceID) == "" {
			if path, err := configstore.DefaultPath(); err == nil {
				if stored, err := configstore.Load(path); err == nil && stored != nil &&
					strings.TrimRight(strings.TrimSpace(stored.APIURL), "/") == app.APIURL {
					app.WorkspaceID = strings.TrimSpace(stored.WorkspaceID)
				}
			}
		}

		tokenFlagExplicit := rootPersistentFlagExplicit(cmd, "token")
		apiKeyFlagExplicit := rootPersistentFlagExplicit(cmd, "api-key")
		if tokenFlagExplicit && apiKeyFlagExplicit {
			return writeErr(cmd, errors.New("cannot use --token and --api-key together"))
		}
		if !tokenFlagExplicit && strings.TrimSpace(app.Token) == "" {
			app.Token = strings.TrimSpace(os.Getenv("BREYTA_TOKEN"))
		}
		if !apiKeyFlagExplicit && strings.TrimSpace(app.APIKey) == "" {
			app.APIKey = strings.TrimSpace(os.Getenv("BREYTA_API_KEY"))
		}
		if strings.TrimSpace(app.APIKey) != "" && !tokenFlagExplicit {
			app.Token = strings.TrimSpace(app.APIKey)
		}
		app.APIKeyExplicit = apiKeyFlagExplicit || strings.TrimSpace(os.Getenv("BREYTA_API_KEY")) != ""
		app.TokenExplicit = tokenFlagExplicit || strings.TrimSpace(os.Getenv("BREYTA_TOKEN")) != "" || app.APIKeyExplicit

		skipBackgroundNetwork := commandShouldSkipBackgroundNetwork(cmd)
		if !skipBackgroundNetwork && !app.TokenExplicit {
			loadTokenFromAuthStore(app)
		}
		configureVisibility(cmd.Root(), app)
		configureFlagVisibility(cmd.Root(), app)

		if cmd != nil && cmd != cmd.Root() && !skipBackgroundNetwork {
			app.startUpdateCheckNonBlocking(context.Background(), 24*time.Hour)
			app.emitUpdateReminder(cmd)
		}
		return nil
	}
	cmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		app.emitUpdateReminder(cmd)
	}

	cmd.AddCommand(newFlowsCmd(app))
	cmd.AddCommand(newRunsCmd(app))
	cmd.AddCommand(newConnectionsCmd(app))
	cmd.AddCommand(newSecretsCmd(app))
	cmd.AddCommand(newTriggersCmd(app))
	cmd.AddCommand(newResourcesCmd(app))
	cmd.AddCommand(newServiceAccountsCmd(app))
	cmd.AddCommand(newWaitsCmd(app))
	cmd.AddCommand(newAuthCmd(app))
	cmd.AddCommand(newAPICmd(app))
	cmd.AddCommand(newWorkspaceCmd(app))
	cmd.AddCommand(newWorkspacesCmd(app))
	cmd.AddCommand(newVersionCmd(app))
	cmd.AddCommand(newUpgradeCmd(app))
	cmd.AddCommand(newSkillsCmd(app))

	return cmd
}

func commandShouldSkipBackgroundNetwork(cmd *cobra.Command) bool {
	if topLevelCommandName(cmd) == "mcp" {
		return true
	}
	return commandIsFlowsLintLocalOnly(cmd)
}

func commandConsumesMCPTokenEnvCredential(cmd *cobra.Command) bool {
	return topLevelCommandName(cmd) == "mcp" && (cmd.Name() == "stdio" || cmd.Name() == "doctor")
}

func topLevelCommandName(cmd *cobra.Command) string {
	if cmd == nil || cmd.Root() == nil || cmd == cmd.Root() {
		return ""
	}
	root := cmd.Root()
	top := cmd
	for top.Parent() != nil && top.Parent() != root {
		top = top.Parent()
	}
	return strings.TrimSpace(top.Name())
}

func commandIsFlowsLintLocalOnly(cmd *cobra.Command) bool {
	if cmd == nil || strings.TrimSpace(cmd.Name()) != "lint" {
		return false
	}
	parent := cmd.Parent()
	if parent == nil || strings.TrimSpace(parent.Name()) != "flows" {
		return false
	}
	flag := cmd.Flags().Lookup("local-only")
	return flag != nil && flag.Changed && strings.EqualFold(strings.TrimSpace(flag.Value.String()), "true")
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
	if cmd != nil {
		// writeErr already renders the message to stderr for this execution path.
		// Suppress Cobra's fallback error echo so guided and generic errors only print once.
		cmd.SilenceErrors = true
	}
	var guided *guidedCLIError
	if errors.As(err, &guided) {
		if cmd == nil {
			fmt.Fprintln(os.Stderr, guided.Error())
			return err
		}
		fmt.Fprintln(cmd.ErrOrStderr(), guided.Error())
		return err
	}
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

func docsHintForCommand(_ *cobra.Command) string {
	return "https://github.com/breyta/breyta-cli#readme"
}

func moreHintForCommand(_ *cobra.Command) string {
	return ""
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func newHelpCmd(app *App, root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long:  "Help provides help for any command in the application.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := root
			if len(args) > 0 {
				found, _, err := root.Find(args)
				if err != nil {
					return err
				}
				target = found
			}
			configureVisibility(root, app)
			configureFlagVisibility(root, app)
			return target.Help()
		},
	}
}
