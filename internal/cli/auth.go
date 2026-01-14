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
			data := map[string]any{"verify": out}
			if email := authinfo.EmailFromToken(app.Token); email != "" {
				data["email"] = email
			}
			return writeData(cmd, app, map[string]any{"httpStatus": status}, data)
		},
	}
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
				if p, err := authstore.DefaultPath(); err == nil {
					storePath = p
				}
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
					return writeFailure(cmd, app, "auth_login_shell_export_unsafe", errors.New("cannot render safe shell export"), "Token contained unexpected characters; use --print token or --format json.", nil)
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
					meta["hint"] = "Token is stored locally with a refresh token; future commands will auto-refresh."
				} else {
					meta["hint"] = "Token is stored locally for future commands."
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
				if p, err := authstore.DefaultPath(); err == nil {
					storePath = p
				}
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
