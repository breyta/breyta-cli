package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/authinfo"
	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/browseropen"

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
	// Firebase ID tokens are base64url-ish and should not contain single quotes.
	// Still, be defensive to avoid producing unsafe shell output.
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
	cmd.AddCommand(newAuthAPIConnectionCmd(app))
	return cmd
}

func newAuthWhoamiCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show identity for the current token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/auth/verify", nil, nil)
			if err != nil {
				return writeErr(cmd, err)
			}
			meta := map[string]any{"httpStatus": status}
			data := map[string]any{"verify": out}
			if email := authinfo.EmailFromToken(app.Token); email != "" {
				data["email"] = email
			}
			enrichWhoamiWorkspaceSummary(cmd, app, data, meta)
			return writeData(cmd, app, meta, data)
		},
	}
}

func enrichWhoamiWorkspaceSummary(cmd *cobra.Command, app *App, data map[string]any, meta map[string]any) {
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

	ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
	defer cancel()

	out, status, err := authClient(app).DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		if meta != nil {
			meta["workspaceHint"] = "Could not load workspace summary. You can still start with `breyta flows search <query>`."
			meta["hint"] = authWhoamiFallbackHint(workspaceID)
		}
		return
	}
	if status >= http.StatusBadRequest {
		if meta != nil {
			meta["workspaceHint"] = "Could not load workspace summary. You can still start with `breyta flows search <query>`."
			meta["hint"] = authWhoamiFallbackHint(workspaceID)
		}
		return
	}

	body := mapStringAny(out)
	if body == nil {
		if meta != nil {
			meta["workspaceHint"] = "Unexpected workspace summary response. You can still start with `breyta flows search <query>`."
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
		return "Auth is working. Start by browsing approved templates with `breyta flows search <query>`. When you're ready to build or adopt one, create or join a workspace in Breyta."
	case workspaceID != "" && hasCurrent:
		return "Auth is working. Next: browse approved templates with `breyta flows search <query>`."
	case workspaceID != "" && !hasCurrent:
		return "Auth is working. Browse approved templates with `breyta flows search <query>`. If you need a different default workspace, run `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
	case workspaceCount == 1:
		return "Auth is working. You have one workspace. Browse approved templates with `breyta flows search <query>`. Set a default later with `breyta workspaces use <workspace-id>` when you're ready to adopt or build."
	default:
		return "Auth is working. Browse approved templates with `breyta flows search <query>`. When you're ready to adopt or build, pick a default with `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
	}
}

func authWhoamiFallbackHint(workspaceID string) string {
	if workspaceID != "" {
		return "Auth is working. Browse approved templates with `breyta flows search <query>`. If you need a different default workspace later, run `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
	}
	return "Auth is working. Browse approved templates with `breyta flows search <query>`. If you need to choose a default workspace later, run `breyta workspaces list` and `breyta workspaces use <workspace-id>`."
}

func newAuthLoginCmd(app *App) *cobra.Command {
	var email string
	var password string
	var passwordStdin bool
	var printMode string
	var storePath string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login via browser and store a token",
		Long: strings.TrimSpace(`
Default: opens a browser window to complete login, then stores a token locally.

Legacy: you can also pass --email + --password to exchange credentials for a token
via flows-api (/api/auth/token). Prefer browser login.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPIBase(app); err != nil {
				return writeErr(cmd, err)
			}
			email = strings.TrimSpace(email)

			var token string
			var refreshToken string
			var expiresInStr string
			var status int
			var tokenSource string
			var uid any
			var expiresIn any

			switch {
			case email != "":
				// Password exchange (legacy).
				if passwordStdin {
					b, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return writeErr(cmd, err)
					}
					password = strings.TrimSpace(string(b))
				}
				if strings.TrimSpace(password) == "" {
					return writeErr(cmd, errors.New("missing --password (or use --password-stdin)"))
				}

				ctx, cancel := context.WithTimeout(cmd.Context(), 25*time.Second)
				defer cancel()

				client := authClient(app)
				client.Token = ""

				out, st, err := client.DoRootREST(ctx, http.MethodPost, "/api/auth/token", nil, map[string]any{
					"email":    email,
					"password": password,
				})
				if err != nil {
					return writeErr(cmd, err)
				}
				status = st

				// Clear password as soon as we can (best-effort).
				password = ""

				m, ok := out.(map[string]any)
				if !ok {
					return writeFailure(cmd, app, "auth_login_unexpected_response", fmt.Errorf("unexpected response (status=%d)", status), "Expected JSON object from /api/auth/token", out)
				}
				if success, _ := m["success"].(bool); !success {
					msg, _ := m["error"].(string)
					if strings.TrimSpace(msg) == "" {
						msg = "login failed"
					}
					return writeFailure(cmd, app, "auth_login_failed", fmt.Errorf("%s (status=%d)", msg, status), "Check email/password and server config (FIREBASE_WEB_API_KEY, Email/Password provider enabled).", m)
				}
				tok, _ := m["token"].(string)
				token = strings.TrimSpace(tok)
				uid = m["uid"]
				expiresIn = m["expiresIn"]
				tokenSource = "password"

			default:
				// Browser login flow.
				res, err := browserLogin(cmd.Context(), baseURL(app), cmd.ErrOrStderr())
				if err != nil {
					return writeErr(cmd, err)
				}
				token = strings.TrimSpace(res.Token)
				refreshToken = strings.TrimSpace(res.RefreshToken)
				expiresInStr = strings.TrimSpace(res.ExpiresIn)
				if expiresInStr != "" {
					expiresIn = expiresInStr
				}
				status = 200
				tokenSource = "browser"
			}

			if token == "" {
				return writeFailure(cmd, app, "auth_login_missing_token", fmt.Errorf("missing token (status=%d)", status), "Server returned success but no token.", nil)
			}

			if strings.TrimSpace(storePath) == "" {
				storePath = resolveAuthStorePath(app)
			}
			if strings.TrimSpace(storePath) != "" {
				st, _ := authstore.Load(storePath)
				if st == nil {
					st = &authstore.Store{}
				}
				rec := authstore.Record{Token: token, RefreshToken: refreshToken}
				if refreshToken != "" {
					if n, ok := expiresInSeconds(expiresInStr, expiresIn); ok {
						rec.ExpiresAt = time.Now().UTC().Add(time.Duration(n) * time.Second)
					}
				}
				st.SetRecord(app.APIURL, rec)
				if err := authstore.SaveAtomic(storePath, st); err != nil {
					return writeErr(cmd, err)
				}
			}

			trackAuthLoginTelemetry(app, tokenSource, token, uid)

			switch printMode {
			case "token":
				fmt.Fprintln(cmd.OutOrStdout(), token)
				return nil
			case "export":
				if !app.DevMode {
					return writeErr(cmd, errors.New("`--print export` is not available"))
				}
				line := shellExportTokenLine(token)
				if line == "" {
					return writeFailure(cmd, app, "auth_login_shell_export_unsafe", errors.New("cannot render safe shell export"), "Token contained unexpected characters; use --print token.", nil)
				}
				fmt.Fprintln(cmd.OutOrStdout(), line)
				return nil
			default:
				meta := map[string]any{
					"httpStatus": status,
					"stored":     strings.TrimSpace(storePath) != "",
					"storePath":  storePath,
					"source":     tokenSource,
				}
				if strings.TrimSpace(refreshToken) != "" {
					meta["hint"] = "Token is stored locally with a refresh token; future commands will auto-refresh. Next: run `breyta auth whoami`, then `breyta flows search <query>`."
				} else {
					meta["hint"] = "Token is stored locally for future commands. Next: run `breyta auth whoami`, then `breyta flows search <query>`."
				}
				if app.DevMode {
					if line := shellExportTokenLine(token); line != "" {
						meta["export"] = line
					}
				}
				data := map[string]any{"token": token}
				if uid != nil {
					data["uid"] = uid
				}
				if expiresIn != nil {
					data["expiresIn"] = expiresIn
				}
				return writeData(cmd, app, meta, data)
			}
		},
	}

	cmd.Flags().StringVar(&email, "email", envOr("BREYTA_EMAIL", ""), "Email address (legacy password flow)")
	cmd.Flags().StringVar(&password, "password", envOr("BREYTA_PASSWORD", ""), "Password (legacy; use --password-stdin to avoid shell history)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin (legacy)")
	cmd.Flags().StringVar(&printMode, "print", envOr("BREYTA_AUTH_PRINT", "json"), "Output mode: json|token")
	cmd.Flags().StringVar(&storePath, "store", envOr("BREYTA_AUTH_STORE", ""), "Path to auth store (default: user config dir)")

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

			st, err := authstore.Load(storePath)
			if err != nil {
				if os.IsNotExist(err) {
					return writeData(cmd, app, map[string]any{"stored": false, "storePath": storePath}, map[string]any{"tokenPresent": strings.TrimSpace(app.Token) != ""})
				}
				return writeErr(cmd, err)
			}

			if all {
				st.Tokens = map[string]authstore.Record{}
			} else {
				ensureAPIURL(app)
				if strings.TrimSpace(app.APIURL) == "" {
					return writeErr(cmd, errors.New("missing api url (or use --all)"))
				}
				st.Delete(app.APIURL)
			}

			if err := authstore.SaveAtomic(storePath, st); err != nil {
				return writeErr(cmd, err)
			}

			meta := map[string]any{
				"stored":    false,
				"storePath": storePath,
			}
			if app.DevMode {
				meta["hint"] = "If you exported a token into your shell, unset it to use the auth store."
			}
			return writeData(cmd, app, meta, map[string]any{"tokenPresent": strings.TrimSpace(app.Token) != ""})
		},
	}

	cmd.Flags().StringVar(&storePath, "store", envOr("BREYTA_AUTH_STORE", ""), "Path to auth store (default: user config dir)")
	cmd.Flags().BoolVar(&all, "all", false, "Remove all stored tokens")
	return cmd
}

func newAuthAPIConnectionCmd(app *App) *cobra.Command {
	var name string
	var secretID string
	var connectionID string
	var baseURL string

	cmd := &cobra.Command{
		Use:   "api-connection",
		Short: "Create a reusable Breyta API runtime connection from the current login",
		Long: strings.TrimSpace(`
Create or update a secret-backed Breyta API connection that flows can use at runtime.

This takes the refresh token from your current ` + "`breyta auth login`" + ` session and asks
flows-api to provision a normal ` + "`http-api`" + ` connection configured for Breyta API OAuth
refresh. The resulting connection can be bound to flow ` + "`:http-api`" + ` slots and reused
across workspaces without embedding refresh tokens directly in flow activation forms.
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(baseURL) == "" {
				baseURL = strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
			}
			storePath := resolveAuthStorePath(app)
			if strings.TrimSpace(storePath) == "" {
				return writeErr(cmd, errors.New("auth store path is unavailable"))
			}
			st, err := authstore.Load(storePath)
			if err != nil {
				return writeErr(cmd, err)
			}
			rec, ok := st.GetRecord(app.APIURL)
			if !ok {
				return writeErr(cmd, errors.New("no stored auth record for current API URL; run `breyta auth login` first"))
			}
			if strings.TrimSpace(rec.RefreshToken) == "" {
				return writeErr(cmd, errors.New("current login does not have a refresh token; run `breyta auth login` again"))
			}

			body := map[string]any{
				"refreshToken": rec.RefreshToken,
				"token":        rec.Token,
				"baseUrl":      baseURL,
			}
			if !rec.ExpiresAt.IsZero() {
				body["expiresAt"] = rec.ExpiresAt.UTC().Format(time.RFC3339)
			}
			if strings.TrimSpace(name) != "" {
				body["name"] = strings.TrimSpace(name)
			}
			if strings.TrimSpace(secretID) != "" {
				body["secretId"] = strings.TrimSpace(secretID)
			}
			if strings.TrimSpace(connectionID) != "" {
				body["connectionId"] = strings.TrimSpace(connectionID)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
			defer cancel()

			out, status, err := apiClient(app).DoREST(ctx, http.MethodPost, "/api/auth/runtime-connection", nil, body)
			if err != nil {
				return writeErr(cmd, err)
			}
			return writeREST(cmd, app, status, out)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Connection name (default: Breyta API)")
	cmd.Flags().StringVar(&secretID, "secret-id", "", "Secret id to store refreshed auth under")
	cmd.Flags().StringVar(&connectionID, "connection-id", "", "Update an existing connection instead of creating a new one")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Breyta API base URL (default: current --api value)")

	return cmd
}

// Ensure any accidental debug output does not leak sensitive values in JSON mode.
// This is a no-op unless someone sets BREYTA_AUTH_DEBUG=1 while developing.
func maybeAuthDebug(v any) {
	if os.Getenv("BREYTA_AUTH_DEBUG") != "1" {
		return
	}
	_ = json.NewEncoder(io.Discard).Encode(v)
}

func parseExpiresInSeconds(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, errors.New("missing expiresIn")
	}
	// APIs return expires_in as a string number of seconds.
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func expiresInSeconds(expiresInStr string, expiresIn any) (int64, bool) {
	if n, err := parseExpiresInSeconds(expiresInStr); err == nil && n > 0 {
		return n, true
	}
	switch v := expiresIn.(type) {
	case string:
		if n, err := parseExpiresInSeconds(v); err == nil && n > 0 {
			return n, true
		}
	case float64:
		if v > 0 {
			return int64(v), true
		}
	case int64:
		if v > 0 {
			return v, true
		}
	case int:
		if v > 0 {
			return int64(v), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

type browserLoginResult struct {
	Token        string
	RefreshToken string
	ExpiresIn    string
}

func browserLogin(ctx context.Context, apiBaseURL string, out io.Writer) (browserLoginResult, error) {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if apiBaseURL == "" {
		return browserLoginResult{}, errors.New("missing api base url")
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return browserLoginResult{}, err
	}
	defer l.Close()

	st := make([]byte, 32)
	if _, err := rand.Read(st); err != nil {
		return browserLoginResult{}, err
	}
	state := base64.RawURLEncoding.EncodeToString(st)

	addr := l.Addr().String()
	callbackURL := "http://" + addr + "/callback"

	tokenCh := make(chan browserLoginResult, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		tok := strings.TrimSpace(q.Get("token"))
		if tok == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		refresh := strings.TrimSpace(q.Get("refresh_token"))
		if refresh == "" {
			refresh = strings.TrimSpace(q.Get("refreshToken"))
		}
		expiresIn := strings.TrimSpace(q.Get("expires_in"))
		if expiresIn == "" {
			expiresIn = strings.TrimSpace(q.Get("expiresIn"))
		}
		_, _ = io.WriteString(w, "<html><body>Login complete. You can close this tab.</body></html>")
		select {
		case tokenCh <- browserLoginResult{Token: tok, RefreshToken: refresh, ExpiresIn: expiresIn}:
		default:
		}
	})

	go func() {
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	authURL := apiBaseURL + "/cli/auth?redirect_uri=" + url.QueryEscape(callbackURL) + "&state=" + url.QueryEscape(state)
	if out != nil {
		fmt.Fprintln(out, "Opening browser for login:")
		fmt.Fprintln(out, authURL)
	}
	if err := openBrowser(authURL); err != nil && out != nil {
		fmt.Fprintln(out, "Could not open browser automatically; open the URL above manually.")
	}

	timeout := 2 * time.Minute
	select {
	case res := <-tokenCh:
		_ = srv.Shutdown(context.Background())
		return res, nil
	case err := <-errCh:
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, err
	case <-time.After(timeout):
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, errors.New("login timed out (no callback received)")
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return browserLoginResult{}, ctx.Err()
	}
}

func openBrowser(u string) error {
	return browseropen.Open(u)
}
