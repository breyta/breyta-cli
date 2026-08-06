package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/clojure/parenrepair"
	"github.com/breyta/breyta-cli/internal/clojure/parinfer"
	"github.com/breyta/breyta-cli/internal/tools"
	"github.com/spf13/cobra"
)

func newFlowsPushCmd(app *App) *cobra.Command {
	var file string
	var target string
	var repairDelimiters bool
	var noRepairWriteback bool
	var validate bool
	var timeout time.Duration
	var deployKey string
	var includeProvenance bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push a local .clj flow file as a draft, creating the flow if needed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "Push requires --api/BREYTA_API_URL")
			}
			if cmd.Flags().Changed("target") {
				resolvedTarget, err := normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
				if resolvedTarget == "live" {
					return writeErr(cmd, errors.New("--target live is not supported for flows push; push always updates workspace current. Use `breyta flows release <slug>` to publish/install live, or `breyta flows promote <slug>` to retarget live"))
				}
			}
			if strings.TrimSpace(file) == "" {
				return writeErr(cmd, errors.New("missing --file"))
			}
			if timeout <= 0 {
				return writeErr(cmd, errors.New("--timeout must be > 0"))
			}
			b, err := readExplicitFile(file)
			if err != nil {
				return writeErr(cmd, err)
			}

			orig := string(b)
			flowLiteral := orig
			if repairDelimiters {
				checkErr := parenrepair.Check(flowLiteral)
				if checkErr != nil && !errors.Is(checkErr, parenrepair.ErrUnbalancedDelimiters) {
					return writeErr(cmd, checkErr)
				}
				if checkErr != nil {
					parinferPath := tools.FindParinferRust()
					if parinferPath != "" {
						if repaired, _, err := (parinfer.Runner{BinaryPath: parinferPath}).RepairIndent(flowLiteral); err == nil {
							flowLiteral = repaired
						}
					}
					// Fallback best-effort repair (always runs if parinfer isn't available or fails).
					if repaired, _, err := parenrepair.Repair(flowLiteral, false); err == nil {
						flowLiteral = repaired
					}
				}
			}
			if !repairDelimiters {
				if checkErr := parenrepair.Check(flowLiteral); checkErr != nil {
					hint := "Run: breyta flows lint --file " + file
					if errors.Is(checkErr, parenrepair.ErrUnbalancedDelimiters) {
						hint = "Run: breyta flows paren-repair --file " + file + " or retry push with --repair-delimiters=true"
					}
					return writeErr(cmd, fmt.Errorf("flow source is not readable: %w. %s", checkErr, hint))
				}
			}

			repairWriteback := !noRepairWriteback
			if repairWriteback && flowLiteral != orig {
				if err := atomicWriteFile(file, []byte(flowLiteral), 0o644); err != nil {
					return writeErr(cmd, err)
				}
			}
			flowLiteral, err = expandFlowSourceIncludes(file, flowLiteral)
			if err != nil {
				return writeErr(cmd, err)
			}
			flowSlug := ""
			if entries, parseErr := parseSingleTopLevelMapEntries(flowLiteral); parseErr == nil {
				flowSlug, _ = localFlowSlugFromEntries(flowLiteral, entries)
			}

			if useDoAPICommandFn {
				payload := map[string]any{"flowLiteral": flowLiteral}
				resolvedDeployKey := strings.TrimSpace(deployKey)
				if resolvedDeployKey == "" {
					resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
				}
				if resolvedDeployKey != "" {
					payload["deploy-key"] = resolvedDeployKey
				}
				return doAPICommandFn(cmd, app, "flows.put_draft", payload)
			}
			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}
			payload := map[string]any{"flowLiteral": flowLiteral}
			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deploy-key"] = resolvedDeployKey
			}
			out, status, err := runAPICommandWithContextAndTimeout(cmd.Context(), app, "flows.put_draft", payload, timeout)
			if err != nil {
				if flowPushResponseTimedOut(status) {
					trackFlowPushTimeout(app, flowSlug, "saving")
					return writeFlowPushTimeoutAPIResponse(cmd, app, flowPushTimeoutErrorResponse(out, err), status, timeout, flowSlug, "saving")
				}
				if flowPushRequestTimedOut(err) {
					trackFlowPushTimeout(app, flowSlug, "saving")
					return writeFlowPushRequestTimeoutAPIResponse(cmd, app, flowPushTimeoutErrorResponse(out, err), status, timeout, flowSlug, "saving")
				}
				return writeFlowPushFailure(cmd, app, out, status, flowSlug, "saving", err)
			}
			if status >= 400 || !isOK(out) {
				if flowPushResponseTimedOut(status) {
					trackFlowPushTimeout(app, flowSlug, "saving")
					return writeFlowPushTimeoutAPIResponse(cmd, app, out, status, timeout, flowSlug, "saving")
				}
				return writeFlowPushFailure(cmd, app, out, status, flowSlug, "saving", nil)
			}
			if dataAny, ok := out["data"]; ok {
				if data, ok := dataAny.(map[string]any); ok {
					if slug, _ := data["flowSlug"].(string); strings.TrimSpace(slug) != "" {
						flowSlug = strings.TrimSpace(slug)
					}
				}
			}
			if !validate {
				if flowSlug != "" {
					_ = appendProvenanceHintsWithOptions(out, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug, includeProvenance)
				}
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":   "flows",
					"channel":   "cli",
					"api_host":  apiHostname(app.APIURL),
					"flow_slug": flowSlug,
					"validated": false,
				})
				return writeAPIResult(cmd, app, out, status)
			}
			if flowSlug == "" {
				meta := ensureMeta(out)
				if meta != nil {
					meta["hint"] = "Draft pushed, but flowSlug missing for validation. Run: breyta flows validate <slug>"
				}
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":   "flows",
					"channel":   "cli",
					"api_host":  apiHostname(app.APIURL),
					"validated": false,
				})
				return writeAPIResult(cmd, app, out, status)
			}

			validateOut, validateStatus, err := validateDraftFlowWithTimeout(cmd, app, flowSlug, timeout)
			if err != nil {
				if flowPushResponseTimedOut(validateStatus) {
					trackFlowPushTimeout(app, flowSlug, "validating")
					return writeFlowPushTimeoutAPIResponse(cmd, app, flowPushTimeoutErrorResponse(validateOut, err), validateStatus, timeout, flowSlug, "validating")
				}
				if flowPushRequestTimedOut(err) {
					trackFlowPushTimeout(app, flowSlug, "validating")
					return writeFlowPushRequestTimeoutAPIResponse(cmd, app, flowPushTimeoutErrorResponse(validateOut, err), validateStatus, timeout, flowSlug, "validating")
				}
				return writeFlowPushFailure(cmd, app, validateOut, validateStatus, flowSlug, "validating", err)
			}
			if validateStatus >= 400 || !isOK(validateOut) {
				if flowPushResponseTimedOut(validateStatus) {
					trackFlowPushTimeout(app, flowSlug, "validating")
					return writeFlowPushTimeoutAPIResponse(cmd, app, validateOut, validateStatus, timeout, flowSlug, "validating")
				}
				if postPushValidationFlowNotFound(validateOut, validateStatus, flowSlug) {
					meta := ensureMeta(out)
					if meta != nil {
						meta["validated"] = false
						meta["validateSource"] = "draft"
						meta["validationWarning"] = "Draft was saved, but immediate validation could not read the new flow yet. Retry validation after the draft is visible."
						appendMetaNextCommands(meta, "breyta flows validate "+flowSlug, "breyta flows show "+flowSlug)
					}
					trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
						"product":         "flows",
						"channel":         "cli",
						"api_host":        apiHostname(app.APIURL),
						"flow_slug":       flowSlug,
						"validated":       false,
						"validate_source": "draft",
					})
					return writeAPIResult(cmd, app, out, status)
				}
				_ = appendProvenanceHintsWithOptions(validateOut, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug, includeProvenance)
				trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
					"product":         "flows",
					"channel":         "cli",
					"api_host":        apiHostname(app.APIURL),
					"flow_slug":       flowSlug,
					"validated":       false,
					"validate_source": "draft",
				})
				return writeFlowPushFailure(cmd, app, validateOut, validateStatus, flowSlug, "validating", nil)
			}
			meta := ensureMeta(out)
			if meta != nil {
				meta["validated"] = true
				meta["validateSource"] = "draft"
			}
			_ = appendProvenanceHintsWithOptions(out, workspaceIDFromEnvelope(out, app.WorkspaceID), flowSlug, includeProvenance)
			trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, map[string]any{
				"product":         "flows",
				"channel":         "cli",
				"api_host":        apiHostname(app.APIURL),
				"flow_slug":       flowSlug,
				"validated":       true,
				"validate_source": "draft",
			})
			return writeAPIResult(cmd, app, out, status)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Path to a flow .clj file")
	cmd.Flags().StringVar(&target, "target", "", "Target override (draft|live). live is not valid for push")
	cmd.Flags().BoolVar(&repairDelimiters, "repair-delimiters", false, "Attempt best-effort delimiter repair before uploading")
	cmd.Flags().BoolVar(&noRepairWriteback, "no-repair-writeback", false, "Do not write repaired content back to --file (default: write back when changed)")
	cmd.Flags().BoolVar(&validate, "validate", true, "Validate the working copy after pushing")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowPushTimeout, "API request timeout for draft push and validation")
	cmd.Flags().BoolVar(&includeProvenance, "provenance", false, "Include full consulted provenance candidate list in output")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key for guarded flows (default: BREYTA_FLOW_DEPLOY_KEY)")
	must(cmd.MarkFlagRequired("file"))
	return cmd
}

func flowPushTimeoutMessage(timeout time.Duration, flowSlug, phase string) string {
	return flowPushTimeoutMessageWithDuration(timeout, flowSlug, phase, true)
}

func flowPushTimeoutResponseMessage(flowSlug, phase string) string {
	return flowPushTimeoutMessageWithDuration(0, flowSlug, phase, false)
}

func flowPushTimeoutMessageWithDuration(timeout time.Duration, flowSlug, phase string, includeDuration bool) string {
	duration := ""
	if includeDuration {
		duration = fmt.Sprintf(" after %s", timeout)
	}
	if strings.EqualFold(strings.TrimSpace(phase), "validating") {
		if slug := strings.TrimSpace(flowSlug); slug != "" {
			return fmt.Sprintf("flows push timed out%s while validating saved draft %s; the draft was already saved. Verify with `breyta flows show %s` or `breyta flows validate %s` before retrying", duration, slug, slug, slug)
		}
		return fmt.Sprintf("flows push timed out%s while validating the saved draft; the draft was already saved. Inspect the target flow with `breyta flows show <slug>` or validate it before retrying", duration)
	}
	if slug := strings.TrimSpace(flowSlug); slug != "" {
		return fmt.Sprintf("flows push timed out%s while saving %s; the draft may already have been saved. Verify with `breyta flows show %s` or `breyta flows validate %s` before retrying", duration, slug, slug, slug)
	}
	return fmt.Sprintf("flows push timed out%s; the draft may already have been saved. Inspect the target flow with `breyta flows show <slug>` or validate it before retrying", duration)
}

func flowPushResponseTimedOut(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout
}

func flowPushTimeoutErrorResponse(out map[string]any, err error) map[string]any {
	if out != nil {
		return out
	}
	message := "API returned an empty response"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return map[string]any{
		"ok":    false,
		"error": map[string]any{"message": message},
	}
}

func writeFlowPushFailure(cmd *cobra.Command, app *App, out map[string]any, status int, flowSlug, phase string, cause error) error {
	out = flowPushFailureEnvelope(out, flowSlug, phase)
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		if cause != nil {
			return writeErr(cmd, fmt.Errorf("%s: %w", flowPushFailureMessage(flowSlug, phase), cause))
		}
		return writeErr(cmd, err)
	}
	return nil
}

func flowPushFailureEnvelope(out map[string]any, flowSlug, phase string) map[string]any {
	if out == nil {
		out = map[string]any{"ok": false}
	}
	out["ok"] = false
	phase = flowPushFailurePhase(phase)
	errorMessage := getErrorMessage(out)
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		errMap = map[string]any{}
		if errorMessage != "" {
			errMap["message"] = errorMessage
		}
		out["error"] = errMap
	}
	if firstNonBlankString(errMap["code"]) == "" {
		errMap["code"] = flowPushFailureCode(phase)
	}
	if firstNonBlankString(errMap["message"]) == "" {
		errMap["message"] = flowPushFailureMessage(flowSlug, phase)
	}

	meta := ensureMeta(out)
	if meta == nil {
		return out
	}
	meta["failurePhase"] = phase
	if phase == "validating" {
		meta["draftOutcome"] = "saved"
		meta["validated"] = false
		meta["validateSource"] = "draft"
	}
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = flowPushFailureMessage(flowSlug, phase)
	}
	slug := strings.TrimSpace(flowSlug)
	if slug == "" {
		slug = "<slug>"
	}
	if phase == "validating" {
		appendMetaNextCommands(meta, "breyta flows validate "+slug, "breyta flows show "+slug)
	} else {
		appendMetaNextCommands(meta, "breyta flows show "+slug, "breyta flows validate "+slug)
	}
	return out
}

func flowPushFailurePhase(phase string) string {
	if strings.EqualFold(strings.TrimSpace(phase), "validating") {
		return "validating"
	}
	return "saving"
}

func flowPushFailureCode(phase string) string {
	if phase == "validating" {
		return "flows_push_validation_failed"
	}
	return "flows_push_save_failed"
}

func flowPushFailureMessage(flowSlug, phase string) string {
	slug := strings.TrimSpace(flowSlug)
	if phase == "validating" {
		if slug != "" {
			return fmt.Sprintf("Draft %s was saved, but validation did not complete. Run `breyta flows validate %s` before retrying.", slug, slug)
		}
		return "The draft was saved, but validation did not complete. Run `breyta flows validate <slug>` before retrying."
	}
	if slug != "" {
		return fmt.Sprintf("Draft save for %s was not confirmed. Run `breyta flows show %s` before retrying.", slug, slug)
	}
	return "Draft save was not confirmed. Run `breyta flows show <slug>` before retrying."
}

func appendFlowPushTimeoutRecovery(out map[string]any, timeout time.Duration, flowSlug, phase string) {
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	message := flowPushTimeoutResponseMessage(flowSlug, phase)
	meta["timeoutRecovery"] = message
	meta["timeoutPhase"] = strings.TrimSpace(phase)
	if strings.EqualFold(strings.TrimSpace(phase), "validating") {
		meta["draftOutcome"] = "saved"
		meta["validated"] = false
		meta["validateSource"] = "draft"
	} else {
		meta["draftOutcome"] = "unknown"
	}
	if _, exists := meta["hint"]; !exists {
		meta["hint"] = message
	}
	slug := strings.TrimSpace(flowSlug)
	if slug == "" {
		slug = "<slug>"
	}
	appendMetaNextCommands(meta, "breyta flows show "+slug, "breyta flows validate "+slug)
}

func writeFlowPushTimeoutAPIResponse(cmd *cobra.Command, app *App, out map[string]any, status int, timeout time.Duration, flowSlug, phase string) error {
	appendFlowPushTimeoutRecovery(out, timeout, flowSlug, phase)
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, errors.New(flowPushTimeoutResponseMessage(flowSlug, phase)+": "+err.Error()))
	}
	return nil
}

func writeFlowPushRequestTimeoutAPIResponse(cmd *cobra.Command, app *App, out map[string]any, status int, timeout time.Duration, flowSlug, phase string) error {
	appendFlowPushTimeoutRecovery(out, timeout, flowSlug, phase)
	if err := writeAPIResult(cmd, app, out, status); err != nil {
		return writeErr(cmd, errors.New(flowPushTimeoutMessage(timeout, flowSlug, phase)+": "+err.Error()))
	}
	return nil
}

func trackFlowPushTimeout(app *App, flowSlug, phase string) {
	properties := map[string]any{
		"product":       "flows",
		"channel":       "cli",
		"api_host":      apiHostname(app.APIURL),
		"validated":     false,
		"timed_out":     true,
		"timeout_phase": strings.TrimSpace(phase),
		"draft_outcome": "unknown",
	}
	if slug := strings.TrimSpace(flowSlug); slug != "" {
		properties["flow_slug"] = slug
	}
	if strings.EqualFold(strings.TrimSpace(phase), "validating") {
		properties["draft_outcome"] = "saved"
		properties["validate_source"] = "draft"
	}
	trackCLIEvent(app, "cli_flow_pushed", nil, app.Token, properties)
}

func flowPushRequestTimedOut(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout") ||
		strings.Contains(message, "timeout awaiting response headers")
}

func postPushValidationFlowNotFound(out map[string]any, status int, flowSlug string) bool {
	if status != 404 {
		return false
	}
	errMap := mapStringAny(out["error"])
	if errMap == nil {
		return false
	}
	code := strings.ToLower(firstNonBlankString(errMap["code"]))
	message := strings.ToLower(firstNonBlankString(errMap["message"]))
	if code != "" && code != "not_found" {
		return false
	}
	if !strings.Contains(message, "flow not found") {
		return false
	}
	details := mapStringAny(errMap["details"])
	if details == nil || strings.TrimSpace(flowSlug) == "" {
		return true
	}
	detailSlug := firstNonBlankString(details["flowSlug"], details["flow-slug"], details["slug"])
	return detailSlug == "" || strings.EqualFold(strings.TrimSpace(detailSlug), strings.TrimSpace(flowSlug))
}

func validateDraftFlow(cmd *cobra.Command, app *App, flowSlug string) (map[string]any, int, error) {
	return apiClient(app).DoCommand(cmd.Context(), "flows.validate", map[string]any{
		"flowSlug": strings.TrimSpace(flowSlug),
		"source":   "draft",
	})
}

func validateDraftFlowWithTimeout(cmd *cobra.Command, app *App, flowSlug string, timeout time.Duration) (map[string]any, int, error) {
	if timeout <= 0 {
		return validateDraftFlow(cmd, app, flowSlug)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	client := apiClientWithTimeout(app, timeout)
	return client.DoCommand(ctx, "flows.validate", map[string]any{
		"flowSlug": strings.TrimSpace(flowSlug),
		"source":   "draft",
	})
}
