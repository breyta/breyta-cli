package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/authinfo"
	"github.com/breyta/breyta-cli/internal/authstore"

	"github.com/spf13/cobra"
)

func requireAPIBase(app *App) error {
	ensureAPIURL(app)
	return nil
}

func authClient(app *App) api.Client {
	return api.Client{
		BaseURL:     app.APIURL,
		WorkspaceID: app.WorkspaceID,
		Token:       app.Token,
	}
}

func shellExportTokenLine(token string) string {
	// Be defensive to avoid producing unsafe shell output.
	if strings.Contains(token, "'") {
		return ""
	}
	return "export BREYTA_TOKEN='" + token + "'"
}

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate"}
	cmd.AddCommand(newAuthWhoamiCmd(app))
	cmd.AddCommand(newAuthLoginCmd(app))
	cmd.AddCommand(newAuthLogoutCmd(app))
	return cmd
}

func newAuthWhoamiCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show identity for the current token",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolveAPIToken(app)
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, authMethod, err := whoamiVerify(ctx, app)
			if err != nil {
				return writeErr(cmd, err)
			}
			meta := map[string]any{"httpStatus": status}
			if authMethod != "" {
				meta["authMethod"] = authMethod
			}
			data := map[string]any{"verify": out}
			if email := authinfo.EmailFromToken(app.Token); email != "" {
				data["email"] = email
			}
			enrichWhoamiWorkspaceSummary(cmd, app, data, meta, authVerifySucceeded(status, out))
			return writeData(cmd, app, meta, data)
		},
	}
}

func whoamiVerify(ctx context.Context, app *App) (any, int, string, error) {
	out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/auth/me", nil, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return out, status, "personal-token", fmt.Errorf("authentication failed (status=%d)", status)
	}
	return out, status, "personal-token", nil
}

func enrichWhoamiWorkspaceSummary(cmd *cobra.Command, app *App, data map[string]any, meta map[string]any, verifyOK bool) {
	if data == nil || app == nil {
		return
	}

	workspaceID, source := whoamiWorkspaceSelection(cmd, app)
	if meta != nil {
		meta["workspaceIdSource"] = source
	}
	data["workspaceSelection"] = map[string]any{
		"id":       workspaceID,
		"source":   source,
		"selected": workspaceID != "",
	}
	if !verifyOK {
		if meta != nil {
			meta["hint"] = authWhoamiVerifyFailedHint()
		}
		return
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
	defer cancel()

	out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/auth/me", nil, nil)
	if err != nil {
		if meta != nil {
			meta["workspaceHint"] = "Could not load workspace summary. You can still inspect the selected workspace with `breyta flows list`."
			meta["hint"] = authWhoamiFallbackHint(workspaceID)
		}
		return
	}
	if status >= http.StatusBadRequest {
		if meta != nil {
			meta["workspaceHint"] = "Could not load workspace summary. You can still inspect the selected workspace with `breyta flows list`."
			meta["hint"] = authWhoamiFallbackHint(workspaceID)
		}
		return
	}

	body := mapStringAny(out)
	if body == nil {
		if meta != nil {
			meta["workspaceHint"] = "Unexpected workspace summary response. You can still inspect the selected workspace with `breyta flows list`."
			meta["hint"] = authWhoamiFallbackHint(workspaceID)
		}
		return
	}

	rawItems := sliceAny(body["workspaces"])
	items := make([]any, 0, len(rawItems))
	var currentWorkspace map[string]any
	for _, raw := range rawItems {
		item := mapStringAny(raw)
		if item == nil {
			continue
		}
		cloned := make(map[string]any, len(item)+1)
		for k, v := range item {
			cloned[k] = v
		}
		id, _ := cloned["id"].(string)
		cloned["current"] = workspaceID != "" && strings.TrimSpace(id) == workspaceID
		if current, _ := cloned["current"].(bool); current {
			currentWorkspace = cloned
		}
		items = append(items, cloned)
	}

	data["workspaces"] = items
	if len(items) == 1 && workspaceID == "" {
		if suggested, ok := items[0].(map[string]any); ok {
			data["suggestedWorkspace"] = suggested
		}
	}
	if currentWorkspace != nil {
		data["currentWorkspace"] = currentWorkspace
	}

	if meta != nil {
		meta["workspaceHTTPStatus"] = status
		meta["workspaceCount"] = len(items)
		if workspaceID != "" && currentWorkspace == nil {
			meta["warning"] = "Configured workspace is not present in the accessible workspace list."
		}
		meta["hint"] = authWhoamiHint(workspaceID, len(items), currentWorkspace != nil)
	}
}

func authVerifySucceeded(status int, out any) bool {
	if status >= http.StatusBadRequest {
		return false
	}
	body := mapStringAny(out)
	if body == nil {
		return true
	}
	if success, ok := body["success"].(bool); ok {
		return success
	}
	return true
}

func whoamiWorkspaceSelection(cmd *cobra.Command, app *App) (string, string) {
	workspaceID := strings.TrimSpace(app.WorkspaceID)
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
	} else if workspaceID == "" {
		source = "none"
	}
	return workspaceID, source
}

func authWhoamiHint(workspaceID string, workspaceCount int, hasCurrent bool) string {
	switch {
	case workspaceCount == 0:
		return "Auth is working, but this identity has no workspace membership. Ask an administrator for an invite."
	case workspaceID != "" && hasCurrent:
		return "Auth is working. Next: inspect workspace flows with `breyta flows list`."
	case workspaceID != "" && !hasCurrent:
		return "Auth is working, but the selected workspace is unavailable. Run `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
	case workspaceCount == 1:
		return "Auth is working. Select the workspace with `breyta workspaces use <workspace-id>`, then run `breyta flows list`."
	default:
		return "Auth is working. Choose a default with `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
	}
}

func authWhoamiFallbackHint(workspaceID string) string {
	if workspaceID != "" {
		return "Auth is working. Inspect workspace flows with `breyta flows list`, or select another workspace with `breyta workspaces use <workspace-id>`."
	}
	return "Auth is working. Choose a default with `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
}

func authWhoamiVerifyFailedHint() string {
	return "Auth check failed. Re-run `breyta auth login`, then `breyta auth whoami`."
}

func newAuthLoginCmd(app *App) *cobra.Command {
	var storePath string
	var printMode string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Verify and store a personal access token",
		Long:  "Provide the token with --token or BREYTA_TOKEN. Tokens are created in the engine operator UI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()
			identity, status, _, err := whoamiVerify(ctx, app)
			if err != nil {
				return writeErr(cmd, err)
			}
			if status < 200 || status >= 300 {
				return writeFailure(cmd, app, "auth_login_failed", fmt.Errorf("token verification failed (status=%d)", status), "Create a personal access token in the engine operator UI.", identity)
			}
			if strings.TrimSpace(storePath) == "" {
				storePath = resolveAuthStorePath(app)
			}
			if strings.TrimSpace(storePath) == "" {
				return writeErr(cmd, errors.New("cannot determine auth store path"))
			}
			if err := authstore.UpdateAtomicOrReset(storePath, func(store *authstore.Store) error {
				store.SetRecord(app.APIURL, authstore.Record{Token: strings.TrimSpace(app.Token)})
				return nil
			}); err != nil {
				return writeErr(cmd, err)
			}
			switch printMode {
			case "token":
				fmt.Fprintln(cmd.OutOrStdout(), app.Token)
				return nil
			case "export":
				line := shellExportTokenLine(app.Token)
				if line == "" {
					return writeErr(cmd, errors.New("cannot render safe shell export"))
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
				return nil
			case "json":
				return writeData(cmd, app, map[string]any{"httpStatus": status, "stored": true, "storePath": storePath}, map[string]any{"identity": identity})
			default:
				return writeErr(cmd, errors.New("--print must be json, token, or export"))
			}
		},
	}
	cmd.Flags().StringVar(&storePath, "store", envOr("BREYTA_AUTH_STORE", ""), "Path to auth store")
	cmd.Flags().StringVar(&printMode, "print", "json", "Output mode: json|token|export")
	return cmd
}

func newAuthLogoutCmd(app *App) *cobra.Command {
	var storePath string
	var all bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Logout (remove stored token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(storePath) == "" {
				storePath = resolveAuthStorePath(app)
			}

			if strings.TrimSpace(storePath) == "" {
				return writeErr(cmd, errors.New("cannot determine auth store path"))
			}

			if !all {
				ensureAPIURL(app)
				if strings.TrimSpace(app.APIURL) == "" {
					return writeErr(cmd, errors.New("missing api url (or use --all)"))
				}
			}
			if err := authstore.UpdateAtomic(storePath, func(st *authstore.Store) error {
				if all {
					st.Tokens = map[string]authstore.Record{}
				} else {
					st.Delete(app.APIURL)
				}
				return nil
			}); err != nil {
				return writeErr(cmd, err)
			}

			meta := map[string]any{
				"stored":    false,
				"storePath": storePath,
			}
			meta["hint"] = "If BREYTA_TOKEN is set, unset it to use the auth store."
			return writeData(cmd, app, meta, map[string]any{"tokenPresent": strings.TrimSpace(app.Token) != ""})
		},
	}

	cmd.Flags().StringVar(&storePath, "store", envOr("BREYTA_AUTH_STORE", ""), "Path to auth store (default: user config dir)")
	cmd.Flags().BoolVar(&all, "all", false, "Remove all stored tokens")
	return cmd
}

func parseExpiresInSeconds(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("missing expiresIn")
	}
	return strconv.ParseInt(value, 10, 64)
}
