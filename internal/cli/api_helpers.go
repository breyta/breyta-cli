package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/configstore"

	"github.com/spf13/cobra"
)

var pasteURLRe = regexp.MustCompile(`https?://paste\.rs/[^\s"}]+`)

// authRefreshHTTPClient is a test hook to avoid binding local ports in restricted
// sandboxes. When nil, refreshTokenViaAPI uses the default HTTP client behavior.
var authRefreshHTTPClient *http.Client

func isAPIMode(app *App) bool {
	return strings.TrimSpace(app.APIURL) != ""
}

func apiFlagExplicit(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Changed("api") || cmd.InheritedFlags().Changed("api") {
		return true
	}
	if root := cmd.Root(); root != nil && root.PersistentFlags().Changed("api") {
		return true
	}
	return false
}

func ensureAPIURL(app *App) {
	if strings.TrimSpace(app.APIURL) != "" {
		app.APIURL = strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
		return
	}
	if p, err := configstore.DefaultPath(); err == nil && strings.TrimSpace(p) != "" {
		if st, err := configstore.Load(p); err == nil && st != nil && strings.TrimSpace(st.APIURL) != "" {
			app.APIURL = strings.TrimRight(strings.TrimSpace(st.APIURL), "/")
			return
		}
	}
	app.APIURL = configstore.DefaultProdAPIURL
}

func requireAPI(app *App) error {
	ensureAPIURL(app)
	// In mock-auth mode, any non-blank token works, but we still require callers
	// to be explicit about auth being in play.
	if !app.TokenExplicit {
		loadTokenFromAuthStore(app)
	}
	if strings.TrimSpace(app.Token) == "" {
		if app.DevMode {
			return errors.New("missing --token or BREYTA_TOKEN")
		}
		return errors.New("missing token (run `breyta auth login`)")
	}
	return nil
}

func loadTokenFromAuthStore(app *App) {
	if strings.TrimSpace(app.APIURL) == "" {
		return
	}
	storePath := resolveAuthStorePath(app)
	if strings.TrimSpace(storePath) == "" {
		return
	}
	st, err := authstore.Load(storePath)
	if err != nil || st == nil {
		return
	}
	rec, ok := st.GetRecord(app.APIURL)
	if !ok {
		return
	}
	updated := false
	if rec.ExpiresAt.IsZero() {
		if exp, ok := parseJWTExpiry(rec.Token); ok {
			rec.ExpiresAt = exp
			updated = true
		}
	}
	if strings.TrimSpace(rec.RefreshToken) != "" && rec.ExpiresAt.IsZero() {
		if next, err := refreshTokenViaAPI(app.APIURL, rec.RefreshToken); err == nil {
			rec = next
			updated = true
		}
	}
	if strings.TrimSpace(rec.RefreshToken) != "" && !rec.ExpiresAt.IsZero() && time.Until(rec.ExpiresAt) < 2*time.Minute {
		if next, err := refreshTokenViaAPI(app.APIURL, rec.RefreshToken); err == nil {
			rec = next
			updated = true
		}
	}
	if updated {
		st.SetRecord(app.APIURL, rec)
		_ = authstore.SaveAtomic(storePath, st)
	}
	app.Token = rec.Token
}

func parseJWTExpiry(token string) (time.Time, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return time.Time{}, false
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	expAny, ok := claims["exp"]
	if !ok {
		return time.Time{}, false
	}
	var expSeconds int64
	switch v := expAny.(type) {
	case float64:
		expSeconds = int64(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			expSeconds = n
		}
	case int64:
		expSeconds = v
	case int:
		expSeconds = int64(v)
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			expSeconds = n
		}
	}
	if expSeconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(expSeconds, 0).UTC(), true
}

func refreshTokenViaAPI(apiBaseURL string, refreshToken string) (authstore.Record, error) {
	apiBaseURL = strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	refreshToken = strings.TrimSpace(refreshToken)
	if apiBaseURL == "" {
		return authstore.Record{}, errors.New("missing api base url")
	}
	if refreshToken == "" {
		return authstore.Record{}, errors.New("missing refresh token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := api.Client{BaseURL: apiBaseURL, HTTP: authRefreshHTTPClient}
	out, status, err := client.DoRootREST(ctx, http.MethodPost, "/api/auth/refresh", nil, map[string]any{
		// Be tolerant: different backends use different JSON naming conventions.
		"refreshToken":  refreshToken,
		"refresh_token": refreshToken,
	})
	if err != nil {
		return authstore.Record{}, err
	}
	if status < 200 || status > 299 {
		return authstore.Record{}, fmt.Errorf("refresh failed (status=%d)", status)
	}

	m, ok := out.(map[string]any)
	if !ok {
		return authstore.Record{}, fmt.Errorf("refresh returned unexpected response (status=%d)", status)
	}
	if success, _ := m["success"].(bool); !success {
		msg := getErrorMessage(m)
		if strings.TrimSpace(msg) == "" {
			msg = "refresh failed"
		}
		return authstore.Record{}, fmt.Errorf("%s (status=%d)", msg, status)
	}
	token, _ := m["token"].(string)
	if strings.TrimSpace(token) == "" {
		return authstore.Record{}, fmt.Errorf("refresh returned no token (status=%d)", status)
	}
	nextRefresh, _ := m["refreshToken"].(string)
	if strings.TrimSpace(nextRefresh) == "" {
		nextRefresh, _ = m["refresh_token"].(string)
	}
	if strings.TrimSpace(nextRefresh) == "" {
		nextRefresh = refreshToken
	}

	rec := authstore.Record{
		Token:        strings.TrimSpace(token),
		RefreshToken: strings.TrimSpace(nextRefresh),
	}

	// expiresIn is sometimes a string (Firebase APIs), sometimes a number; tolerate both.
	var expiresInSeconds int64
	expiresInAny := m["expiresIn"]
	if expiresInAny == nil {
		expiresInAny = m["expires_in"]
	}
	switch v := expiresInAny.(type) {
	case string:
		if n, err := parseExpiresInSeconds(v); err == nil {
			expiresInSeconds = n
		}
	case float64:
		expiresInSeconds = int64(v)
	}
	if expiresInSeconds > 0 {
		rec.ExpiresAt = time.Now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	}
	return rec, nil
}

func apiClient(app *App) api.Client {
	return api.Client{
		BaseURL:     app.APIURL,
		WorkspaceID: app.WorkspaceID,
		Token:       app.Token,
	}
}

func baseURL(app *App) string {
	ensureAPIURL(app)
	return app.APIURL
}

func activationURL(app *App, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/flows/%s/activate", baseURL(app), app.WorkspaceID, slug)
}

func draftBindingsURL(app *App, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/flows/%s/draft-bindings", baseURL(app), app.WorkspaceID, slug)
}

func getErrorMessage(out map[string]any) string {
	if out == nil {
		return ""
	}
	// API errors sometimes come back as {error: "..."} (string), or {error: {message: "...", ...}}
	if errAny, ok := out["error"]; ok {
		switch v := errAny.(type) {
		case string:
			return strings.TrimSpace(v)
		case map[string]any:
			if msg, _ := v["message"].(string); strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
			if details, _ := v["details"].(string); strings.TrimSpace(details) != "" {
				return strings.TrimSpace(details)
			}
		}
	}
	return ""
}

func formatAPIError(out map[string]any) string {
	msg := getErrorMessage(out)
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	if out == nil {
		return "unknown error"
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return "unknown error"
	}
	rendered := string(payload)
	if len(rendered) > 1000 {
		return rendered[:1000] + "..."
	}
	return rendered
}

func ensureMeta(out map[string]any) map[string]any {
	if out == nil {
		return nil
	}
	if metaAny, ok := out["meta"]; ok {
		if m, ok := metaAny.(map[string]any); ok {
			if m != nil {
				return m
			}
		}
	}
	m := map[string]any{}
	out["meta"] = m
	return m
}

func addActivationHint(app *App, out map[string]any, flowSlug string) {
	url := activationURL(app, flowSlug)
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if strings.TrimSpace(url) != "" {
		meta["activationUrl"] = url
		if _, exists := meta["webUrl"]; !exists {
			meta["webUrl"] = url
		}
	}
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Flow uses :requires slots. Connect-first recommendation: reuse or create+test connections before wiring (breyta connections list; breyta connections create ...; breyta connections test <connection-id>). Then configure workspace target and promote live when needed: breyta flows configure " + flowSlug + " --set <slot>.conn=conn-...; breyta flows promote " + flowSlug + ". Run against live explicitly with --target live or target a specific install with --installation-id <id>."
	}
}

func addDraftBindingsHint(app *App, out map[string]any, flowSlug string) {
	url := draftBindingsURL(app, flowSlug)
	if url == "" {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	meta["draftBindingsUrl"] = url
	if _, exists := meta["webUrl"]; !exists {
		meta["webUrl"] = url
	}
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Draft runs need draft bindings. Set them here: " + url
	}
}

func isOK(out map[string]any) bool {
	if out == nil {
		return false
	}
	okAny, ok := out["ok"]
	if !ok {
		return false
	}
	okb, ok := okAny.(bool)
	return ok && okb
}

func flowLiteralDeclaresRequires(out map[string]any) bool {
	if out == nil {
		return false
	}
	dataAny, ok := out["data"]
	if !ok {
		return false
	}
	data, ok := dataAny.(map[string]any)
	if !ok {
		return false
	}
	flowLiteral, _ := data["flowLiteral"].(string)
	if strings.TrimSpace(flowLiteral) == "" {
		return false
	}
	return strings.Contains(flowLiteral, ":requires") && !strings.Contains(flowLiteral, ":requires nil")
}

func extractRunPasteURL(out map[string]any) string {
	if out == nil {
		return ""
	}

	dataAny, ok := out["data"]
	if !ok {
		return ""
	}
	data, ok := dataAny.(map[string]any)
	if !ok {
		return ""
	}
	runAny, ok := data["run"]
	if !ok {
		return ""
	}
	run, ok := runAny.(map[string]any)
	if !ok {
		return ""
	}
	resultPreviewAny, ok := run["resultPreview"]
	if !ok {
		return ""
	}
	resultPreview, ok := resultPreviewAny.(map[string]any)
	if !ok {
		return ""
	}
	rpDataAny, ok := resultPreview["data"]
	if !ok {
		return ""
	}
	// Fallback: some servers return resultPreview.data as an EDN string.
	// Extract the paste URL without needing a full EDN parser.
	if s, ok := rpDataAny.(string); ok {
		if m := pasteURLRe.FindString(s); strings.TrimSpace(m) != "" {
			return strings.TrimSpace(m)
		}
		return ""
	}

	rpData, ok := rpDataAny.(map[string]any)
	if !ok {
		return ""
	}
	resultAny, ok := rpData["result"]
	if !ok {
		return ""
	}
	result, ok := resultAny.(map[string]any)
	if !ok {
		return ""
	}

	if url, _ := result["pasteUrl"].(string); strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	if url, _ := result["paste-url"].(string); strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	return ""
}

func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	case json.Number:
		return strings.TrimSpace(t.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func errorDocsHintLines(out map[string]any) []string {
	if out == nil {
		return nil
	}
	errAny, ok := out["error"]
	if !ok {
		return nil
	}
	errMap, ok := errAny.(map[string]any)
	if !ok {
		return nil
	}

	hintRefsAny, ok := errMap["hintRefs"]
	if !ok {
		hintRefsAny = errMap["hint-refs"]
	}
	refs, ok := hintRefsAny.([]any)
	if !ok || len(refs) == 0 {
		return nil
	}

	lines := make([]string, 0, len(refs))
	seen := map[string]struct{}{}
	for _, refAny := range refs {
		ref, ok := refAny.(map[string]any)
		if !ok {
			continue
		}
		kind := strings.ToLower(scalarString(ref["kind"]))
		var line string
		switch kind {
		case "page":
			slug := scalarString(ref["slug"])
			if slug == "" {
				continue
			}
			line = fmt.Sprintf("Docs: breyta docs show %s", slug)
		case "find":
			query := scalarString(ref["query"])
			if query == "" {
				continue
			}
			line = fmt.Sprintf("Docs: breyta docs find %q", query)
		default:
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
		if len(lines) >= 3 {
			break
		}
	}
	return lines
}

func writeErrorDocsHints(cmd *cobra.Command, out map[string]any) {
	lines := errorDocsHintLines(out)
	if len(lines) == 0 {
		return
	}
	for _, line := range lines {
		if cmd != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
		} else {
			_, _ = fmt.Fprintln(os.Stderr, line)
		}
	}
}

func writeAPIResult(cmd *cobra.Command, app *App, v map[string]any, status int) error {
	// Add progressive-disclosure hints for common auth/binding issues (best-effort).
	// This keeps agent/human workflows on the “happy path” without requiring out-of-band docs.
	msg := strings.ToLower(getErrorMessage(v))
	if strings.Contains(msg, "requires a flow profile") ||
		strings.Contains(msg, "no profile-id") ||
		strings.Contains(msg, "activate flow") {
		// We don't always know the slug here; command handlers should also add hints where possible.
		meta := ensureMeta(v)
		if meta != nil {
			if _, exists := meta["hint"]; !exists {
				meta["hint"] = "This error usually means the flow needs configuration + installation promotion. Use: breyta flows configure <slug> --set <slot>.conn=conn-...; breyta flows promote <slug>; then run with --target live or --installation-id <id>."
			}
		}
	}

	// Workspace membership flakes are common in local dev (mock auth + restarts).
	// When we recognize the server message, give a direct recovery hint.
	if status == http.StatusForbidden && strings.Contains(msg, "not a workspace member") {
		meta := ensureMeta(v)
		if meta != nil {
			if _, exists := meta["hint"]; !exists {
				ws := strings.TrimSpace(app.WorkspaceID)
				if ws == "" {
					ws = "<workspace-id>"
				}
				meta["hint"] = "If you're running flows-api locally and see membership 403s after restarts, bootstrap the workspace/membership: breyta workspaces bootstrap " + ws
			}
		}
	}

	// Nice-to-have: when a run result contains a paste URL, surface it as structured metadata.
	if pasteURL := extractRunPasteURL(v); pasteURL != "" {
		meta := ensureMeta(v)
		if meta != nil {
			if _, exists := meta["pasteUrl"]; !exists {
				meta["pasteUrl"] = pasteURL
			}
		}
	}
	enrichEnvelopeWebLinks(app, v)

	_ = writeOut(cmd, app, v)

	// Determine exit code: non-2xx OR ok=false => exit non-zero.
	if status >= 400 {
		writeErrorDocsHints(cmd, v)
		return fmt.Errorf("api error (status=%d): %s", status, formatAPIError(v))
	}
	if okAny, ok := v["ok"]; ok {
		if okb, ok := okAny.(bool); ok && !okb {
			writeErrorDocsHints(cmd, v)
			// Surface the server-provided message if present.
			if errAny, ok := v["error"]; ok {
				if em, ok := errAny.(map[string]any); ok {
					if msg, _ := em["message"].(string); strings.TrimSpace(msg) != "" {
						return errors.New(msg)
					}
				}
			}
			return fmt.Errorf("command failed: %s", formatAPIError(v))
		}
	}
	return nil
}

func doAPICommand(cmd *cobra.Command, app *App, command string, args map[string]any) error {
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	client := apiClient(app)
	out, status, err := client.DoCommand(context.Background(), command, args)
	if err != nil {
		return writeErr(cmd, err)
	}
	trackCommandTelemetry(app, command, args, status, status < 400 && isOK(out))

	// Progressive disclosure: only add installation/configuration hints when relevant.
	//
	// Avoid always emitting install/configure hints on successful releases for flows that have no :requires
	// (this was confusing and produced false hints).
	if slug, _ := args["flowSlug"].(string); strings.TrimSpace(slug) != "" {
		switch command {
		case "flows.get":
			if flowLiteralDeclaresRequires(out) {
				addActivationHint(app, out, slug)
			}
		case "runs.start":
			// Only add when runs.start failed (common when missing installation config).
			if status >= 400 || !isOK(out) {
				if source, _ := args["source"].(string); strings.EqualFold(strings.TrimSpace(source), "draft") {
					addDraftBindingsHint(app, out, slug)
				} else {
					addActivationHint(app, out, slug)
				}
			}
		case "flows.deploy":
			// If release/deploy failed, installation promotion may still be relevant.
			if status >= 400 || !isOK(out) {
				addActivationHint(app, out, slug)
			} else {
				// On success, best-effort check whether the released flow declares :requires
				// before adding installation hints (avoids false positives).
				getOut, getStatus, getErr := client.DoCommand(
					context.Background(),
					"flows.get",
					map[string]any{"flowSlug": slug},
				)
				if getErr == nil && getStatus < 400 && flowLiteralDeclaresRequires(getOut) {
					addActivationHint(app, out, slug)
				}
			}
		case "flows.release":
			// If release failed, installation configuration may still be relevant.
			if status >= 400 || !isOK(out) {
				addActivationHint(app, out, slug)
			} else {
				// On success, best-effort check whether the released flow declares :requires
				// before adding installation hints (avoids false positives).
				getOut, getStatus, getErr := client.DoCommand(
					context.Background(),
					"flows.get",
					map[string]any{"flowSlug": slug},
				)
				if getErr == nil && getStatus < 400 && flowLiteralDeclaresRequires(getOut) {
					addActivationHint(app, out, slug)
				}
			}
		}
	}

	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}
