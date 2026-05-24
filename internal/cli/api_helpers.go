package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

func flagExplicit(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Changed(name) || cmd.InheritedFlags().Changed(name) {
		return true
	}
	if root := cmd.Root(); root != nil && root.PersistentFlags().Changed(name) {
		return true
	}
	return false
}

func rootPersistentFlagExplicit(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	root := cmd.Root()
	if root == nil {
		return false
	}
	return root.PersistentFlags().Changed(name)
}

func apiFlagExplicit(cmd *cobra.Command) bool {
	return flagExplicit(cmd, "api")
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
	resolveAPIToken(app)
	if strings.TrimSpace(app.Token) == "" {
		if app.DevMode {
			return errors.New("missing token (--token, BREYTA_TOKEN, --api-key, or BREYTA_API_KEY)")
		}
		return errors.New("missing token (run `breyta auth login` or provide --api-key / BREYTA_API_KEY)")
	}
	return nil
}

func resolveAPIToken(app *App) {
	ensureAPIURL(app)
	if !app.TokenExplicit {
		loadTokenFromAuthStore(app)
	}
}

func isLoopbackAPIURL(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	switch strings.ToLower(host) {
	case "localhost":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	base := workspaceWebBaseURL(app)
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/flows/%s/activate", base, slug)
}

func draftBindingsURL(app *App, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	base := workspaceWebBaseURL(app)
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/flows/%s/draft-bindings", base, slug)
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

func applyCompactFlowsGetPayload(payload map[string]any) {
	payload["view"] = "summary"
	payload["includeFlowLiteral"] = false
	payload["includeTemplates"] = false
	payload["includeFunctions"] = false
}

func applyFullFlowsGetPayload(payload map[string]any) {
	payload["view"] = "full"
	payload["includeFlowLiteral"] = true
	payload["includeTemplates"] = true
	payload["includeFunctions"] = true
}

func applyFlowsGetVerbosityPayload(payload map[string]any, full bool, include string) {
	if full {
		applyFullFlowsGetPayload(payload)
		return
	}
	include = strings.TrimSpace(include)
	if include == "" {
		applyCompactFlowsGetPayload(payload)
		return
	}

	payload["view"] = "full"
	payload["includeFlowLiteral"] = false
	payload["includeTemplates"] = false
	payload["includeFunctions"] = false
	for _, raw := range splitNonEmpty(include) {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "all", "full", "definition":
			applyFullFlowsGetPayload(payload)
			return
		case "flow", "flow-literal", "flowliteral", "source":
			payload["includeFlowLiteral"] = true
		case "templates", "template":
			payload["includeTemplates"] = true
		case "functions", "function":
			payload["includeFunctions"] = true
		}
	}
}

func compactRunsGetPayload(workflowID string) map[string]any {
	return map[string]any{
		"workflowId":    workflowID,
		"includeSteps":  false,
		"includeResult": false,
	}
}

func finalWaitRunsGetPayload(workflowID string) map[string]any {
	return map[string]any{
		"workflowId":    workflowID,
		"includeSteps":  true,
		"includeResult": false,
	}
}

func hydrateTerminalWaitRun(client apiCommandRunner, workflowID string, installationID string) (map[string]any, int, error) {
	return hydrateWaitRunSnapshot(client, workflowID, installationID)
}

func hydrateWaitRunSnapshot(client apiCommandRunner, workflowID string, installationID string) (map[string]any, int, error) {
	payload := finalWaitRunsGetPayload(workflowID)
	if strings.TrimSpace(installationID) != "" {
		payload["installationId"] = strings.TrimSpace(installationID)
	}
	out, status, err := client.DoCommand(context.Background(), "runs.get", payload)
	if err != nil {
		return nil, status, err
	}
	compactWaitRunOutput(out)
	addWaitRunNextCommands(out, workflowID)
	return out, status, nil
}

func compactWaitRunOutput(out map[string]any) {
	data := mapStringAny(out["data"])
	run := mapStringAny(data["run"])
	if run == nil {
		return
	}
	if _, ok := run["steps"]; ok {
		run["steps"] = compactRunStepSummaries(run["steps"])
	}
}

func compactRunStepSummaries(value any) []any {
	steps := sliceAny(value)
	if len(steps) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(steps))
	for _, raw := range steps {
		step := mapStringAny(raw)
		if step == nil {
			continue
		}
		summary := map[string]any{}
		for _, key := range []string{"stepId", "stepType", "status", "durationMs", "attempt"} {
			if v, ok := step[key]; ok && v != nil {
				summary[key] = v
			}
		}
		if errMap := mapStringAny(step["error"]); errMap != nil {
			if message := firstNonBlankString(errMap["message"], errMap["error"], errMap["code"]); message != "" {
				summary["error"] = map[string]any{"message": message}
			}
		}
		if len(summary) > 0 {
			out = append(out, summary)
		}
	}
	return out
}

func addWaitRunNextCommands(out map[string]any, workflowID string) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return
	}
	meta := ensureMeta(out)
	appendMetaNextCommands(meta,
		"breyta runs inspect "+workflowID,
		"breyta resources workflow list "+workflowID)
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
		meta["hint"] = "Flow needs required setup before live/install runs."
	}
	appendMetaNextCommands(meta,
		"breyta connections list",
		"breyta connections test <connection-id>",
		"breyta flows configure "+flowSlug+" --set <slot>.conn=conn-...",
		"breyta flows promote "+flowSlug)
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
		meta["hint"] = "Draft runs need draft setup. Set it here: " + url
	}
}

func draftBindingsHintRelevant(out map[string]any) bool {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return false
	}
	switch strings.ToLower(firstNonBlankString(errMap["code"])) {
	case "profile_missing", "profile_activation_inputs_incomplete", "profile_bindings_incomplete":
		return true
	default:
		return false
	}
}

func runFailureShouldUseDraftBindings(command string, args map[string]any, out map[string]any) bool {
	if !draftBindingsHintRelevant(out) {
		return false
	}
	switch command {
	case "runs.start":
		source, _ := args["source"].(string)
		return strings.EqualFold(strings.TrimSpace(source), "draft")
	case "flows.run":
		if _, ok := args["profileId"]; ok {
			return false
		}
		if version, ok := args["version"]; ok {
			switch v := version.(type) {
			case int:
				if v > 0 {
					return false
				}
			case int32:
				if v > 0 {
					return false
				}
			case int64:
				if v > 0 {
					return false
				}
			case float64:
				if v > 0 {
					return false
				}
			case float32:
				if v > 0 {
					return false
				}
			}
		}
		target, _ := args["target"].(string)
		target = strings.TrimSpace(target)
		return target == "" || strings.EqualFold(target, "draft")
	default:
		return false
	}
}

func enrichCommandHints(app *App, command string, args map[string]any, status int, out map[string]any) {
	slug, _ := args["flowSlug"].(string)
	if strings.TrimSpace(slug) == "" {
		return
	}

	switch command {
	case "flows.get":
		if flowLiteralDeclaresRequires(out) {
			addActivationHint(app, out, slug)
		}
	case "runs.start", "flows.run":
		if status >= 400 || !isOK(out) {
			if runFailureIsMissingFlow(out) {
				addMissingFlowAuthoringHint(out, slug)
			} else if runFailureIsMissingRunInputs(out) {
				addMissingRunInputsHint(out, slug)
			} else if runFailureShouldUseDraftBindings(command, args, out) {
				addDraftBindingsHint(app, out, slug)
			} else {
				addActivationHint(app, out, slug)
			}
		}
	case "flows.deploy":
		if status >= 400 || !isOK(out) {
			addActivationHint(app, out, slug)
		} else {
			client := apiClient(app)
			getPayload := map[string]any{"flowSlug": slug}
			applyCompactFlowsGetPayload(getPayload)
			getOut, getStatus, getErr := client.DoCommand(
				context.Background(),
				"flows.get",
				getPayload,
			)
			if getErr == nil && getStatus < 400 && flowLiteralDeclaresRequires(getOut) {
				addActivationHint(app, out, slug)
			}
		}
	case "flows.release":
		if status >= 400 || !isOK(out) {
			addActivationHint(app, out, slug)
		} else {
			client := apiClient(app)
			getPayload := map[string]any{"flowSlug": slug}
			applyCompactFlowsGetPayload(getPayload)
			getOut, getStatus, getErr := client.DoCommand(
				context.Background(),
				"flows.get",
				getPayload,
			)
			if getErr == nil && getStatus < 400 && flowLiteralDeclaresRequires(getOut) {
				addActivationHint(app, out, slug)
			}
		}
	}
}

func runFailureIsMissingFlow(out map[string]any) bool {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return false
	}
	code := strings.ToLower(firstNonBlankString(errMap["code"]))
	message := strings.ToLower(firstNonBlankString(errMap["message"]))
	return strings.Contains(code, "not_found") && strings.Contains(message, "flow not found")
}

func runFailureIsMissingRunInputs(out map[string]any) bool {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return false
	}
	message := strings.ToLower(firstNonBlankString(errMap["message"]))
	return strings.Contains(message, "missing required run inputs")
}

func addMissingRunInputsHint(out map[string]any, flowSlug string) {
	flowSlug = strings.TrimSpace(flowSlug)
	if flowSlug == "" {
		flowSlug = "<slug>"
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	errMap := mapStringAny(out["error"])
	details := mapStringAny(errMap["details"])
	missing := stringSlice(firstPresentAny(details["missingKeys"], details["missing-keys"]))
	example := map[string]any{}
	for _, key := range missing {
		key = strings.TrimSpace(key)
		if key != "" {
			example[key] = "<value>"
		}
	}
	inputJSON := "'{\"<field>\":\"<value>\"}'"
	if len(example) > 0 {
		if b, err := json.Marshal(example); err == nil {
			inputJSON = shellSingleQuote(string(b))
		}
	}
	meta["hint"] = "Provide the missing per-run inputs with --input."
	meta["missingRunInputs"] = missing
	appendMetaNextCommands(meta,
		"breyta flows run "+flowSlug+" --target draft --input "+inputJSON+" --wait",
		"breyta flows show "+flowSlug)
}

func addMissingFlowAuthoringHint(out map[string]any, flowSlug string) {
	flowSlug = strings.TrimSpace(flowSlug)
	if flowSlug == "" {
		flowSlug = "<slug>"
	}
	meta := ensureMeta(out)
	if meta != nil {
		if _, exists := meta["hint"]; !exists {
			meta["hint"] = "No flow with that slug exists in this workspace."
		}
		appendMetaNextCommands(meta,
			"breyta flows push --file <file>",
			"breyta flows create --slug "+flowSlug,
			"breyta flows list --limit 10")
	}
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return
	}
	details := mapStringAny(errMap["details"])
	if details == nil {
		details = map[string]any{}
		errMap["details"] = details
	}
	if _, exists := details["nextCommands"]; !exists {
		details["nextCommands"] = []any{
			"breyta flows push --file <file>",
			"breyta flows create --slug " + flowSlug,
			"breyta flows list --limit 10",
		}
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
	if flowAny, ok := data["flow"]; ok {
		if flow, ok := flowAny.(map[string]any); ok {
			if requiresAny, ok := flow["requires"]; ok {
				switch requires := requiresAny.(type) {
				case []any:
					if len(requires) > 0 {
						return true
					}
				case []map[string]any:
					if len(requires) > 0 {
						return true
					}
				case map[string]any:
					if len(requires) > 0 {
						return true
					}
				}
			}
		}
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
	case json.Number:
		return strings.TrimSpace(t.String())
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func mapStringAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func sliceAny(v any) []any {
	items, _ := v.([]any)
	return items
}

func firstNonBlankString(values ...any) string {
	for _, value := range values {
		if s := scalarString(value); s != "" {
			return s
		}
	}
	return ""
}

func appendMetaNextCommands(meta map[string]any, commands ...string) {
	if meta == nil {
		return
	}
	seen := map[string]struct{}{}
	var out []any
	for _, existing := range sliceAny(meta["nextCommands"]) {
		cmd := firstNonBlankString(existing)
		if cmd == "" {
			continue
		}
		if _, ok := seen[cmd]; ok {
			continue
		}
		seen[cmd] = struct{}{}
		out = append(out, cmd)
	}
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if _, ok := seen[cmd]; ok {
			continue
		}
		seen[cmd] = struct{}{}
		out = append(out, cmd)
	}
	if len(out) > 0 {
		meta["nextCommands"] = out
	}
}

func validateJSONOnlyFormat(raw, command string) error {
	format := strings.TrimSpace(strings.ToLower(raw))
	if format == "" || format == "json" {
		return nil
	}
	command = strings.TrimSpace(command)
	if command == "" {
		command = "this command"
	}
	return fmt.Errorf("invalid --format %q (%s supports json output only)", raw, command)
}

func workflowIDFromEnvelope(out map[string]any) string {
	data := mapStringAny(out["data"])
	if data == nil {
		return ""
	}
	if workflowID := firstNonBlankString(data["workflowId"], data["workflow-id"]); workflowID != "" {
		return workflowID
	}
	if run := mapStringAny(data["run"]); run != nil {
		if workflowID := firstNonBlankString(run["workflowId"], run["workflow-id"], run["id"]); workflowID != "" {
			return workflowID
		}
	}
	if errMap := mapStringAny(out["error"]); errMap != nil {
		if details := mapStringAny(errMap["details"]); details != nil {
			return firstNonBlankString(details["workflowId"], details["workflow-id"])
		}
	}
	return ""
}

func envelopeTextContains(out map[string]any, needles ...string) bool {
	if out == nil {
		return false
	}
	b, err := json.Marshal(out)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(b))
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func enrichExternalRequestFailureHints(out map[string]any) {
	if out == nil || isOK(out) {
		return
	}
	if !envelopeTextContains(out, "read timed out", "deadline exceeded", "request timed out", "timed out waiting for response") {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if _, exists := meta["diagnosticHint"]; !exists {
		meta["diagnosticHint"] = "External request timed out. Inspect failed steps and HTTP timeout/retry/persist settings."
	}
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = "Inspect failed steps; check HTTP timeout/retry/persist fields."
	}
	workflowID := workflowIDFromEnvelope(out)
	if workflowID == "" {
		workflowID = "<workflow-id>"
	}
	appendMetaNextCommands(meta,
		"breyta runs show "+workflowID+" --errors",
		"breyta runs inspect "+workflowID,
		"breyta resources workflow list "+workflowID,
		"breyta docs fields http timeout retry persist --format json")
}

func defaultRecoveryActionLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "billing":
		return "Billing"
	case "draft-bindings":
		return "Draft setup"
	case "flow-activation":
		return "Flow activation"
	case "installation":
		return "Installation"
	case "connection-edit":
		return "Edit connection"
	default:
		return "Open page"
	}
}

func normalizeRecoveryAction(app *App, action map[string]any) map[string]any {
	if action == nil {
		return nil
	}
	kind := firstNonBlankString(action["kind"])
	url := firstNonBlankString(action["url"])
	if strings.TrimSpace(url) == "" {
		return nil
	}
	baseRoot := ""
	if app != nil {
		baseRoot = strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
	}
	url = absolutizeWebURL(baseRoot, url)
	label := firstNonBlankString(action["label"])
	if label == "" {
		label = defaultRecoveryActionLabel(kind)
	}
	out := make(map[string]any, len(action)+3)
	for key, value := range action {
		out[key] = value
	}
	out["kind"] = kind
	out["label"] = label
	out["url"] = url
	if slot := firstNonBlankString(action["slot"]); slot != "" {
		out["slot"] = slot
	}
	if connectionID := firstNonBlankString(action["connectionId"], action["connection-id"]); connectionID != "" {
		out["connectionId"] = connectionID
	}
	if profileID := firstNonBlankString(action["profileId"], action["profile-id"]); profileID != "" {
		out["profileId"] = profileID
	}
	return out
}

func appendRecoveryAction(actions []map[string]any, seen map[string]struct{}, action map[string]any) []map[string]any {
	if action == nil {
		return actions
	}
	url := strings.TrimSpace(firstNonBlankString(action["url"]))
	if url == "" {
		return actions
	}
	key := url
	if _, exists := seen[key]; exists {
		return actions
	}
	seen[key] = struct{}{}
	return append(actions, action)
}

func newRecoveryAction(app *App, kind, label, url string, extra map[string]any) map[string]any {
	action := map[string]any{
		"kind":  strings.TrimSpace(kind),
		"label": strings.TrimSpace(label),
		"url":   strings.TrimSpace(url),
	}
	for key, value := range extra {
		action[key] = value
	}
	return normalizeRecoveryAction(app, action)
}

func connectionRecoveryActions(app *App, bindings []any) []map[string]any {
	base := workspaceWebBaseURL(app)
	if base == "" || len(bindings) == 0 {
		return nil
	}
	actions := make([]map[string]any, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, item := range bindings {
		binding := mapStringAny(item)
		if binding == nil {
			continue
		}
		connectionID := firstNonBlankString(binding["connectionId"], binding["connection-id"])
		if connectionID == "" {
			continue
		}
		extra := map[string]any{"connectionId": connectionID}
		if slot := firstNonBlankString(binding["slot"]); slot != "" {
			extra["slot"] = slot
		}
		actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "connection-edit", "Edit connection", connectionEditWebURL(base, connectionID), extra))
	}
	return actions
}

func serverRecoveryActions(app *App, out map[string]any) []map[string]any {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return nil
	}
	var actions []map[string]any
	seen := map[string]struct{}{}
	for _, item := range sliceAny(errMap["actions"]) {
		action := mapStringAny(item)
		if action == nil {
			continue
		}
		normalized := normalizeRecoveryAction(app, action)
		if normalized == nil {
			continue
		}
		actions = appendRecoveryAction(actions, seen, normalized)
	}
	return actions
}

func legacyRecoveryActions(app *App, out map[string]any) []map[string]any {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return nil
	}
	details := mapStringAny(errMap["details"])
	meta := mapStringAny(out["meta"])
	base := workspaceWebBaseURL(app)
	code := strings.ToLower(firstNonBlankString(errMap["code"]))
	message := strings.ToLower(firstNonBlankString(errMap["message"], out["hint"]))
	flowSlug := firstNonBlankString(details["flowSlug"], details["flow-slug"])
	profileID := firstNonBlankString(details["profileId"], details["profile-id"])
	draftBindingsHintURL := firstNonBlankString(meta["draftBindingsUrl"])
	activationHintURL := firstNonBlankString(meta["activationUrl"])

	actions := []map[string]any{}
	seen := map[string]struct{}{}
	if billingURL := firstNonBlankString(details["billingUrl"], mapStringAny(details["billing"])["billingUrl"]); billingURL != "" {
		actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "billing", "Billing", billingURL, nil))
	}

	for _, key := range []string{"invalidConnectionBindings", "unhealthyConnectionBindings", "errors"} {
		for _, action := range connectionRecoveryActions(app, sliceAny(details[key])) {
			actions = appendRecoveryAction(actions, seen, action)
		}
	}

	if base != "" && flowSlug != "" {
		switch code {
		case "installation_disabled", "profile_disabled", "profile_activation_inputs_incomplete", "profile_bindings_incomplete", "profile_version_not_found":
			if profileID != "" {
				actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "installation", "Installation", installationWebURL(base, flowSlug, profileID), map[string]any{"profileId": profileID}))
			}
		case "profile_missing":
			if draftBindingsHintURL != "" {
				actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "draft-bindings", "Draft setup", draftBindingsHintURL, nil))
			} else if strings.Contains(message, "draft profile") {
				actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "draft-bindings", "Draft setup", draftBindingsURL(app, flowSlug), nil))
			} else {
				actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "flow-activation", "Flow activation", activationURL(app, flowSlug), nil))
			}
		case "installation_not_found", "live_config_incomplete":
			actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "flow-activation", "Flow activation", activationURL(app, flowSlug), nil))
		}
	}

	if draftBindingsHintURL != "" {
		actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "draft-bindings", "Draft setup", draftBindingsHintURL, nil))
	}
	if activationHintURL != "" {
		actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "flow-activation", "Flow activation", activationHintURL, nil))
	}
	if webURL := firstNonBlankString(meta["webUrl"]); webURL != "" {
		actions = appendRecoveryAction(actions, seen, newRecoveryAction(app, "open", "Open page", webURL, nil))
	}
	return actions
}

func ensureErrorRecoveryActions(app *App, out map[string]any) []map[string]any {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return nil
	}

	actions := serverRecoveryActions(app, out)
	if len(actions) == 0 {
		actions = legacyRecoveryActions(app, out)
	}
	if len(actions) == 0 {
		return nil
	}

	serialized := make([]any, 0, len(actions))
	for _, action := range actions {
		serialized = append(serialized, action)
	}
	errMap["actions"] = serialized

	meta := ensureMeta(out)
	if meta != nil {
		if _, exists := meta["webUrl"]; !exists {
			meta["webUrl"] = firstNonBlankString(actions[0]["url"])
		} else if first := firstNonBlankString(actions[0]["url"]); first != "" {
			meta["webUrl"] = first
		}
	}
	return actions
}

func errorRecoveryActions(out map[string]any) []map[string]any {
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return nil
	}
	items := sliceAny(errMap["actions"])
	if len(items) == 0 {
		return nil
	}
	actions := make([]map[string]any, 0, len(items))
	for _, item := range items {
		action := mapStringAny(item)
		if action == nil {
			continue
		}
		actions = append(actions, action)
	}
	return actions
}

func recoveryActionDisplayLabel(action map[string]any) string {
	label := firstNonBlankString(action["label"])
	if label == "" {
		label = defaultRecoveryActionLabel(firstNonBlankString(action["kind"]))
	}
	if slot := firstNonBlankString(action["slot"]); slot != "" && firstNonBlankString(action["kind"]) == "connection-edit" {
		return label + " (" + slot + ")"
	}
	return label
}

func errorRecoveryActionLines(out map[string]any) []string {
	actions := errorRecoveryActions(out)
	if len(actions) == 0 {
		return nil
	}
	lines := make([]string, 0, len(actions))
	for _, action := range actions {
		url := firstNonBlankString(action["url"])
		if url == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("Open %s: %s", recoveryActionDisplayLabel(action), url))
	}
	return lines
}

func guidedCLIErrorForCommand(cmd *cobra.Command, message string, lines []string) error {
	parts := []string{strings.TrimSpace(message)}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts = append(parts, line)
	}
	if more := moreHintForCommand(cmd); more != "" {
		parts = append(parts, fmt.Sprintf("More: %s", more))
	}
	parts = append(parts, fmt.Sprintf("Hint: run `%s` for usage or `%s` for docs.", helpHintForCommand(cmd), docsHintForCommand(cmd)))
	return &guidedCLIError{message: strings.Join(parts, "\n")}
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

func apiWarningMaps(out map[string]any) []map[string]any {
	meta := mapStringAny(out["meta"])
	if meta == nil {
		return nil
	}
	raw, ok := meta["warnings"]
	if !ok {
		return nil
	}
	appendWarning := func(dst []map[string]any, value any) []map[string]any {
		if m := mapStringAny(value); m != nil {
			return append(dst, m)
		}
		return dst
	}
	switch warnings := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(warnings))
		for _, warning := range warnings {
			out = appendWarning(out, warning)
		}
		return out
	case []map[string]any:
		return warnings
	case map[string]any:
		return []map[string]any{warnings}
	default:
		return nil
	}
}

func apiDeprecationWarningLines(app *App, out map[string]any) []string {
	warnings := apiWarningMaps(out)
	if len(warnings) == 0 {
		return nil
	}
	baseRoot := ""
	if app != nil {
		baseRoot = strings.TrimRight(strings.TrimSpace(app.APIURL), "/")
	}
	var lines []string
	seen := map[string]struct{}{}
	for _, warning := range warnings {
		kind := strings.ToLower(firstNonBlankString(warning["kind"], warning["category"], warning["type"]))
		code := strings.ToLower(firstNonBlankString(warning["code"]))
		if kind != "deprecation" && !strings.HasPrefix(code, "deprecated_") {
			continue
		}
		message := firstNonBlankString(warning["message"], warning["warning"], warning["title"])
		if message == "" {
			if code == "" {
				continue
			}
			message = code
		}
		docsURL := firstNonBlankString(warning["docsUrl"], warning["docsURL"], warning["docs-url"], warning["documentationUrl"], warning["docUrl"])
		if docsURL != "" {
			docsURL = absolutizeWebURL(baseRoot, docsURL)
		}
		var b strings.Builder
		b.WriteString("warning: deprecated flow source pattern: ")
		b.WriteString(message)
		if path := warningPathString(warning["path"]); path != "" {
			b.WriteString("\n  path: ")
			b.WriteString(path)
		}
		if replacement := firstNonBlankString(warning["replacement"], warning["use"], warning["correctPattern"]); replacement != "" {
			b.WriteString("\n  use: ")
			b.WriteString(replacement)
		}
		if docsURL != "" {
			b.WriteString("\n  docs: ")
			b.WriteString(docsURL)
		}
		line := b.String()
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	return lines
}

func warningPathString(value any) string {
	var parts []string
	appendPart := func(v any) {
		s := scalarString(v)
		if s != "" {
			parts = append(parts, s)
		}
	}
	switch path := value.(type) {
	case []any:
		for _, part := range path {
			appendPart(part)
		}
	case []string:
		for _, part := range path {
			appendPart(part)
		}
	case string:
		appendPart(path)
	default:
		appendPart(path)
	}
	return strings.Join(parts, " ")
}

func writeAPIDeprecationWarnings(cmd *cobra.Command, app *App, out map[string]any) {
	lines := apiDeprecationWarningLines(app, out)
	if len(lines) == 0 {
		return
	}
	writer := os.Stderr
	if cmd != nil {
		for _, line := range lines {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), line)
		}
		return
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(writer, line)
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
				meta["hint"] = "Flow setup is incomplete for this target."
			}
			appendMetaNextCommands(meta,
				"breyta flows configure <slug> --set <slot>.conn=conn-...",
				"breyta flows promote <slug>",
				"breyta flows run <slug> --target live --wait")
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
				meta["hint"] = "Local workspace membership missing."
				appendMetaNextCommands(meta, "breyta workspaces bootstrap "+ws)
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
	enrichExternalRequestFailureHints(v)
	enrichEnvelopeWebLinks(app, v)
	ensureErrorRecoveryActions(app, v)

	_ = writeOut(cmd, app, v)
	if status < 400 && isOK(v) {
		writeAPIDeprecationWarnings(cmd, app, v)
	}

	// Determine exit code: non-2xx OR ok=false => exit non-zero.
	if status >= 400 {
		lines := append(errorRecoveryActionLines(v), errorDocsHintLines(v)...)
		return guidedCLIErrorForCommand(cmd, fmt.Sprintf("api error (status=%d): %s", status, formatAPIError(v)), lines)
	}
	if okAny, ok := v["ok"]; ok {
		if okb, ok := okAny.(bool); ok && !okb {
			// Surface the server-provided message if present.
			lines := append(errorRecoveryActionLines(v), errorDocsHintLines(v)...)
			if errAny, ok := v["error"]; ok {
				if em, ok := errAny.(map[string]any); ok {
					if msg, _ := em["message"].(string); strings.TrimSpace(msg) != "" {
						return guidedCLIErrorForCommand(cmd, msg, lines)
					}
				}
			}
			return guidedCLIErrorForCommand(cmd, fmt.Sprintf("command failed: %s", formatAPIError(v)), lines)
		}
	}
	return nil
}

func enrichAPICommandResult(app *App, client api.Client, command string, args map[string]any, out map[string]any, status int) {
	if out == nil {
		return
	}
	_ = client
	if command == "runs.list" {
		annotateRunsListResult(app, out, args)
	}
	enrichCommandHints(app, command, args, status, out)
}

func runAPICommandWithContextAndTimeout(ctx context.Context, app *App, command string, args map[string]any, timeout time.Duration) (map[string]any, int, error) {
	if err := requireAPI(app); err != nil {
		return nil, 0, err
	}
	if timeout < 0 {
		return nil, 0, fmt.Errorf("timeout must be non-negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	client := apiClient(app)
	if timeout > 0 {
		client.HTTP = &http.Client{Timeout: timeout}
	}
	out, status, err := client.DoCommand(ctx, command, args)
	if err != nil {
		return nil, 0, err
	}
	trackCommandTelemetry(app, command, args, status, status < 400 && isOK(out))
	enrichAPICommandResult(app, client, command, args, out, status)
	return out, status, nil
}

func runAPICommandWithContext(ctx context.Context, app *App, command string, args map[string]any) (map[string]any, int, error) {
	return runAPICommandWithContextAndTimeout(ctx, app, command, args, 0)
}

func runAPICommand(app *App, command string, args map[string]any) (map[string]any, int, error) {
	return runAPICommandWithContext(context.Background(), app, command, args)
}

func doAPICommand(cmd *cobra.Command, app *App, command string, args map[string]any) error {
	out, status, err := runAPICommand(app, command, args)
	if err != nil {
		return writeErr(cmd, err)
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}

func doAPICommandWithTimeout(cmd *cobra.Command, app *App, command string, args map[string]any, timeout time.Duration) error {
	out, status, err := runAPICommandWithContextAndTimeout(cmd.Context(), app, command, args, timeout)
	if err != nil {
		return writeErr(cmd, err)
	}
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}

func doGlobalAPICommand(cmd *cobra.Command, app *App, command string, args map[string]any) error {
	if err := requireAPI(app); err != nil {
		return writeErr(cmd, err)
	}
	client := apiClient(app)
	out, status, err := client.DoGlobalCommand(context.Background(), command, args)
	if err != nil {
		return writeErr(cmd, err)
	}
	trackCommandTelemetry(app, command, args, status, status < 400 && isOK(out))
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, err)
	}
	return nil
}
