package cli

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "regexp"
        "strings"

        "breyta-cli/internal/api"

        "github.com/spf13/cobra"
)

var pasteURLRe = regexp.MustCompile(`https?://paste\.rs/[^\s"}]+`)

func isAPIMode(app *App) bool {
        return strings.TrimSpace(app.APIURL) != ""
}

func requireAPI(app *App) error {
        if strings.TrimSpace(app.APIURL) == "" {
                return errors.New("missing --api or BREYTA_API_URL")
        }
        // In mock-auth mode, any non-blank token works, but we still require callers
        // to be explicit about auth being in play.
        if strings.TrimSpace(app.Token) == "" {
                return errors.New("missing --token or BREYTA_TOKEN")
        }
        return nil
}

func apiClient(app *App) api.Client {
        return api.Client{
                BaseURL:     app.APIURL,
                WorkspaceID: app.WorkspaceID,
                Token:       app.Token,
        }
}

func baseURL(app *App) string {
        b := strings.TrimSpace(app.APIURL)
        if b == "" {
                return "https://flows.breyta.io"
        }
        return strings.TrimRight(b, "/")
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
                        return m
                }
        }
        m := map[string]any{}
        out["meta"] = m
        return m
}

func addActivationHint(app *App, out map[string]any, flowSlug string) {
        url := activationURL(app, flowSlug)
        if url == "" {
                return
        }
        meta := ensureMeta(out)
        if meta == nil {
                return
        }
        // Progressive disclosure: add both a structured field and a human hint.
        meta["activationUrl"] = url
        if _, exists := meta["hint"]; !exists {
                meta["hint"] = "Flow uses :requires slots. Activate it to create a profile and bind credentials: " + url
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
                                meta["hint"] = "This error usually means the flow must be activated to create a profile and bind :requires slots. Use: breyta flows activate-url <slug>"
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

        _ = writeOut(cmd, app, v)

        // Determine exit code: non-2xx OR ok=false => exit non-zero.
        if status >= 400 {
                return fmt.Errorf("api error (status=%d): %s", status, formatAPIError(v))
        }
        if okAny, ok := v["ok"]; ok {
                if okb, ok := okAny.(bool); ok && !okb {
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

        // Progressive disclosure (activation): only add activation hints when they are likely relevant.
        //
        // Avoid always emitting activationUrl/hints on successful deploys for flows that have no :requires
        // (this was confusing and produced false hints).
        if slug, _ := args["flowSlug"].(string); strings.TrimSpace(slug) != "" {
                switch command {
                case "flows.get":
                        if flowLiteralDeclaresRequires(out) {
                                addActivationHint(app, out, slug)
                        }
                case "runs.start":
                        // Only add when runs.start failed (common when missing activation/bindings).
                        if status >= 400 || !isOK(out) {
                                if source, _ := args["source"].(string); strings.EqualFold(strings.TrimSpace(source), "draft") {
                                        addDraftBindingsHint(app, out, slug)
                                } else {
                                        addActivationHint(app, out, slug)
                                }
                        }
                case "flows.deploy":
                        // If deploy failed, activation may still be relevant.
                        if status >= 400 || !isOK(out) {
                                addActivationHint(app, out, slug)
                        } else {
                                // On success, best-effort check whether the deployed flow declares :requires
                                // before adding activation hints (avoids false positives).
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

        return writeAPIResult(cmd, app, out, status)
}
