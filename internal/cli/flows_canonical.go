package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const defaultFlowRunWaitTimeout = 5 * time.Minute

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
	default:
		return 0
	}
	return 0
}

func releaseLiveRuntimeSummary(flowSlug string, releaseData, promoteData map[string]any) map[string]any {
	releaseRuntime := mapStringAny(releaseData["liveRuntime"])
	promoteRuntime := mapStringAny(promoteData["liveRuntime"])
	activeVersion := asInt(firstPresentAny(releaseData["activeVersion"], promoteData["activeVersion"], promoteRuntime["activeVersion"]))
	latestVersion := asInt(firstPresentAny(
		promoteData["latestVersion"],
		promoteRuntime["latestVersion"],
		releaseData["latestVersion"],
		releaseRuntime["latestVersion"],
		promoteData["latestAvailable"],
		releaseData["latestAvailable"],
	))
	runtimeVersion := asInt(firstPresentAny(promoteData["liveRuntimeVersion"], promoteData["version"], promoteRuntime["version"]))

	summary := map[string]any{
		"flowSlug": flowSlug,
		"target":   "live",
	}
	if profileID := strings.TrimSpace(scalarString(firstPresentAny(promoteData["profileId"], promoteRuntime["profileId"]))); profileID != "" {
		summary["profileId"] = profileID
	}
	if runtimeVersion > 0 {
		summary["version"] = runtimeVersion
	}
	if activeVersion > 0 {
		summary["activeVersion"] = activeVersion
	}
	if latestVersion > 0 {
		summary["latestVersion"] = latestVersion
	}
	if runtimeVersion > 0 && activeVersion > 0 {
		summary["runtimeMatchesActiveVersion"] = runtimeVersion == activeVersion
	}
	if latestVersion > 0 && activeVersion > 0 {
		summary["latestMatchesActiveVersion"] = latestVersion == activeVersion
	}

	status := "live_runtime_version_verified"
	if activeVersion == 0 {
		status = "active_version_missing"
	} else if runtimeVersion == 0 {
		status = "live_runtime_version_missing"
	} else if runtimeVersion != activeVersion {
		status = "live_runtime_version_differs_from_active_version"
	} else if latestVersion > 0 && latestVersion != activeVersion {
		status = "latest_version_differs_from_active_version"
	}
	summary["status"] = status
	return summary
}

func releaseLiveRuntimeWarnings(summary map[string]any) []string {
	runtimeVersion := asInt(summary["version"])
	activeVersion := asInt(summary["activeVersion"])
	latestVersion := asInt(summary["latestVersion"])
	warnings := []string{}
	if runtimeVersion > 0 && activeVersion > 0 && runtimeVersion != activeVersion {
		warnings = append(warnings, fmt.Sprintf("Live target now uses version %d while activeVersion is %d; webhook/live traffic follows the live target version.", runtimeVersion, activeVersion))
	}
	if latestVersion > 0 && activeVersion > 0 && latestVersion != activeVersion {
		warnings = append(warnings, fmt.Sprintf("latestVersion is %d while activeVersion is %d; release/promote the intended version explicitly if this was not intentional.", latestVersion, activeVersion))
	}
	if runtimeVersion == 0 {
		slug := strings.TrimSpace(scalarString(summary["flowSlug"]))
		if slug == "" {
			slug = "<slug>"
		}
		warnings = append(warnings, fmt.Sprintf("Live target version was not returned by promote; verify with `breyta flows show %s --target live` before assuming webhook traffic changed.", slug))
	}
	return warnings
}

func commandWorkspaceID(out map[string]any) string {
	if out == nil {
		return ""
	}
	if v, ok := out["workspaceId"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func waitRetryCommand(command string, flowSlug string, payload map[string]any) string {
	flowSlug = strings.TrimSpace(flowSlug)
	if flowSlug == "" {
		return ""
	}
	var parts []string
	switch command {
	case "flows.run_step":
		stepID := argString(payload, "stepId", "step-id")
		if stepID == "" {
			return ""
		}
		parts = []string{"breyta", "flows", "run-step", flowSlug, stepID}
		if retryFlags, ok := runStepRetryInputFlags(payload); ok {
			parts = append(parts, retryFlags...)
		} else {
			return ""
		}
	default:
		parts = []string{"breyta", "flows", "run", flowSlug}
	}
	if installationID := argString(payload, "installationId", "installation-id"); installationID != "" {
		parts = append(parts, "--installation-id", installationID)
	} else if profileID := argString(payload, "profileId", "profile-id"); profileID != "" {
		parts = append(parts, "--profile-id", profileID)
	} else if target := argString(payload, "target"); target != "" {
		parts = append(parts, "--target", target)
	}
	if version := asInt(payload["version"]); version > 0 {
		parts = append(parts, "--version", strconv.Itoa(version))
	}
	parts = append(parts, "--wait", "--timeout", "5m")
	return strings.Join(parts, " ")
}

func runStepRetryInputFlags(payload map[string]any) ([]string, bool) {
	var parts []string
	if invocation := argString(payload, "invocation", "invocationId", "invocation-id"); invocation != "" {
		parts = append(parts, "--invocation", invocation)
	}
	input, hasInput := payload["input"]
	if !hasInput || input == nil {
		return parts, true
	}
	b, err := json.Marshal(input)
	if err != nil {
		return nil, false
	}
	parts = append(parts, "--input", shellSingleQuote(string(b)))
	return parts, true
}

func doRunCommandWithOptionalWait(cmd *cobra.Command, app *App, command string, payload map[string]any, wait bool, timeout time.Duration, poll time.Duration) error {
	client := apiClient(app)
	startResp, status, err := client.DoCommand(context.Background(), command, payload)
	if err != nil {
		return writeErr(cmd, err)
	}
	startOK := status < 400 && isOK(startResp)
	trackCommandTelemetry(app, command, payload, status, startOK)
	enrichCommandHints(app, command, payload, status, startResp)
	flowSlug, _ := payload["flowSlug"].(string)
	if startOK {
		trackCLIEvent(app, "cli_flow_run_started", nil, app.Token, map[string]any{
			"product":   "flows",
			"channel":   "cli",
			"api_host":  apiHostname(app.APIURL),
			"flow_slug": strings.TrimSpace(flowSlug),
			"command":   strings.TrimSpace(command),
			"wait":      wait,
		})
	}
	if !wait || !startOK {
		if err := writeAPIResult(cmd, app, startResp, status); err != nil {
			return writeErr(cmd, err)
		}
		return nil
	}

	return waitForRunCompletion(cmd, app, startResp, strings.TrimSpace(flowSlug), command, payload, timeout, poll)
}

func waitForRunCompletion(cmd *cobra.Command, app *App, startResp map[string]any, flowSlug string, command string, payload map[string]any, timeout time.Duration, poll time.Duration) error {
	client := apiClient(app)
	data, _ := startResp["data"].(map[string]any)
	workflowID := workflowIDFromRunData(data)
	if strings.TrimSpace(workflowID) == "" {
		return writeErr(cmd, errors.New("missing data.workflowId in start response"))
	}
	installationID := installationIDFromRunData(data)
	deadline := time.Now().Add(timeout)
	polls := 0
	var nextTerminalFallback time.Time
	finishReconciledTerminal := func(finalResp map[string]any, finalStatus int, finalRunStatus string) error {
		trackCLIEvent(app, "cli_flow_run_completed", nil, app.Token, map[string]any{
			"product":     "flows",
			"channel":     "cli",
			"api_host":    apiHostname(app.APIURL),
			"flow_slug":   flowSlug,
			"command":     strings.TrimSpace(command),
			"workflow_id": workflowID,
			"run_status":  finalRunStatus,
			"wait":        true,
			"reconciled":  true,
		})
		if err := writeAPIResult(cmd, app, finalResp, finalStatus); err != nil {
			return writeErr(cmd, err)
		}
		if runStatusFailedForExit(finalRunStatus) {
			return guidedCLIErrorForCommand(cmd, "flow run finished with status "+finalRunStatus, []string{
				"Inspect failed steps: breyta runs inspect " + workflowID,
				"List resources: breyta resources workflow list " + workflowID,
			})
		}
		return nil
	}
	for {
		runsGetPayload := compactRunsGetPayload(workflowID)
		if installationID != "" {
			runsGetPayload["installationId"] = installationID
		}
		execResp, execStatus, err := client.DoCommand(context.Background(), "runs.get", runsGetPayload)
		if err != nil {
			return writeErr(cmd, err)
		}
		if execStatus == 404 {
			polls++
			if shouldCheckTerminalWaitFallback(polls, nextTerminalFallback) {
				nextTerminalFallback = time.Now().Add(terminalWaitFallbackInterval(poll))
				if finalResp, finalStatus, finalRunStatus, ok, err := terminalRunFallback(client, workflowID, installationID); err == nil && ok {
					return finishReconciledTerminal(finalResp, finalStatus, finalRunStatus)
				}
			}
			if time.Now().After(deadline) {
				if err := writeAPIResult(cmd, app, execResp, execStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}
			time.Sleep(poll)
			continue
		}
		if execStatus >= 400 {
			if err := writeAPIResult(cmd, app, execResp, execStatus); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		}

		execData, _ := execResp["data"].(map[string]any)
		run, _ := execData["run"].(map[string]any)
		s := canonicalRunStatus(run["status"])
		if isTerminalRunStatus(s) {
			trackCLIEvent(app, "cli_flow_run_completed", nil, app.Token, map[string]any{
				"product":     "flows",
				"channel":     "cli",
				"api_host":    apiHostname(app.APIURL),
				"flow_slug":   flowSlug,
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"run_status":  s,
				"wait":        true,
			})
			finalResp, finalStatus, err := hydrateTerminalWaitRun(client, workflowID, installationID)
			if err != nil {
				return writeErr(cmd, err)
			}
			if finalStatus >= 400 {
				finalResp = execResp
				finalStatus = execStatus
			}
			if err := writeAPIResult(cmd, app, finalResp, finalStatus); err != nil {
				return writeErr(cmd, err)
			}
			if runStatusFailedForExit(s) {
				return guidedCLIErrorForCommand(cmd, "flow run finished with status "+s, []string{
					"Inspect failed steps: breyta runs inspect " + workflowID,
					"List resources: breyta resources workflow list " + workflowID,
				})
			}
			return nil
		}
		polls++
		if shouldCheckTerminalWaitFallback(polls, nextTerminalFallback) {
			nextTerminalFallback = time.Now().Add(terminalWaitFallbackInterval(poll))
			if finalResp, finalStatus, finalRunStatus, ok, err := terminalRunFallback(client, workflowID, installationID); err == nil && ok {
				return finishReconciledTerminal(finalResp, finalStatus, finalRunStatus)
			}
		}

		if time.Now().After(deadline) {
			trackCLIEvent(app, "cli_flow_run_wait_timed_out", nil, app.Token, map[string]any{
				"product":     "flows",
				"channel":     "cli",
				"api_host":    apiHostname(app.APIURL),
				"flow_slug":   flowSlug,
				"command":     strings.TrimSpace(command),
				"workflow_id": workflowID,
				"wait":        true,
			})
			lastPoll := execResp
			if snapshot, snapshotStatus, err := hydrateWaitRunSnapshot(client, workflowID, installationID); err == nil && snapshotStatus < 400 {
				lastPoll = snapshot
			}
			nextCommands := []string{
				"breyta runs inspect " + workflowID,
				"breyta runs show " + workflowID + " --include-steps",
				"breyta resources workflow list " + workflowID,
			}
			if retryCommand := waitRetryCommand(command, flowSlug, payload); retryCommand != "" {
				nextCommands = append(nextCommands, retryCommand)
			}
			timeoutOut := map[string]any{
				"ok": false,
				"error": map[string]any{
					"message": fmt.Sprintf("timed out waiting for run completion (workflowId=%s)", workflowID),
					"details": map[string]any{
						"workflowId": workflowID,
						"timeoutMs":  timeout.Milliseconds(),
						"pollMs":     poll.Milliseconds(),
					},
				},
				"meta": map[string]any{
					"timedOut":     true,
					"hint":         "The run may still be in progress. Inspect the workflow id, or use a longer --timeout on the next waited run.",
					"nextCommands": nextCommands,
				},
				"data": map[string]any{
					"workflowId": workflowID,
					"start":      startResp,
					"lastPoll":   lastPoll,
				},
			}
			if err := writeAPIResult(cmd, app, timeoutOut, 200); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		}
		time.Sleep(poll)
	}
}

func runStatusFailedForExit(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "cancelled", "canceled", "terminated", "timed-out", "timed_out":
		return true
	default:
		return false
	}
}

func parseFlowRunUpload(raw string) (string, string, error) {
	field, path, ok := strings.Cut(raw, "=")
	field = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(field), ":"))
	path = strings.TrimSpace(path)
	if !ok || field == "" || path == "" {
		return "", "", fmt.Errorf("invalid --upload %q (expected field=path)", raw)
	}
	return field, path, nil
}

type flowRunUploadSpec struct {
	field string
	path  string
}

func parseFlowRunUploads(uploads []string, existingFields map[string]bool) ([]flowRunUploadSpec, error) {
	specs := make([]flowRunUploadSpec, 0, len(uploads))
	for _, raw := range trimStringSlice(uploads) {
		field, path, err := parseFlowRunUpload(raw)
		if err != nil {
			return nil, err
		}
		if existingFields[field] {
			return nil, fmt.Errorf("--upload field %q conflicts with --input; remove the field from --input or omit --upload", field)
		}
		specs = append(specs, flowRunUploadSpec{field: field, path: path})
	}
	return specs, nil
}

func flowRunUploadResourceRef(result map[string]any) (map[string]any, error) {
	uri := firstNonBlankString(result["resourceUri"], result["uri"])
	if uri == "" {
		return nil, errors.New("upload response missing resource URI")
	}
	ref := map[string]any{
		"type": "resource-ref",
		"uri":  uri,
	}
	if contentType := firstNonBlankString(result["contentType"], result["content-type"]); contentType != "" {
		ref["contentType"] = contentType
	}
	if filename := firstNonBlankString(result["filename"], result["name"]); filename != "" {
		ref["filename"] = filename
	}
	if sizeBytes, ok := firstPresentAny(result["sizeBytes"], result["size-bytes"]).(int64); ok {
		ref["sizeBytes"] = sizeBytes
	} else if sizeBytes := firstPresentAny(result["sizeBytes"], result["size-bytes"]); sizeBytes != nil {
		ref["sizeBytes"] = sizeBytes
	}
	return ref, nil
}

func addFlowRunUploadInput(input map[string]any, field string, ref map[string]any, existingFields map[string]bool) error {
	if existingFields[field] {
		return fmt.Errorf("--upload field %q conflicts with --input; remove the field from --input or omit --upload", field)
	}
	if existing, ok := input[field]; ok {
		switch items := existing.(type) {
		case []any:
			input[field] = append(items, ref)
		default:
			input[field] = []any{items, ref}
		}
		return nil
	}
	input[field] = ref
	return nil
}

func applyFlowRunUploads(ctx context.Context, app *App, input map[string]any, uploads []string, existingFields map[string]bool) error {
	specs, err := parseFlowRunUploads(uploads, existingFields)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		result, err := jobsWorkerUploadFileResource(ctx, app, spec.path, filepath.Base(spec.path), "")
		if err != nil {
			return fmt.Errorf("upload %s: %w", spec.field, err)
		}
		ref, err := flowRunUploadResourceRef(result)
		if err != nil {
			return fmt.Errorf("upload %s: %w", spec.field, err)
		}
		if err := addFlowRunUploadInput(input, spec.field, ref, existingFields); err != nil {
			return err
		}
	}
	return nil
}

func newFlowsRunCmd(app *App) *cobra.Command {
	var installationID string
	var target string
	var version int
	var invocation string
	var interfaceID string
	var triggerID string
	var inputJSON string
	var uploads []string
	var buyerTest bool
	var wait bool
	var timeout time.Duration
	var poll time.Duration

	cmd := &cobra.Command{
		Use:   "run <flow-slug>",
		Short: "Start a flow run",
		Long: strings.TrimSpace(`
Default:
- breyta flows run <flow-slug> [--input '{...}'] [--wait]
- file/blob-ref manual inputs: use --upload field=path to emulate a browser upload
- brand-new unreleased flows: add --target draft while authoring, or release first

	Advanced targeting:
	- --installation-id <id> : run a specific installation target
	- --buyer-test : make the installation run intent explicit for Buyer Test Mode
	- --invocation <id> : select a named invocation input contract
	- --interface-id <id> : select the declared manual interface explicitly
	- --target draft|live : select workspace draft/live when not using --installation-id
	- --version <n> : force a specific release version for this invocation
	- --trigger-id <id> : compatibility alias for legacy trigger-backed flows
	`),
		Example: strings.TrimSpace(`
breyta flows run order-ingest --wait
breyta flows run order-ingest --input '{"region":"EU"}' --wait
breyta flows run thesis-pdf-review-docx --target draft --interface-id run --upload thesis=./thesis.pdf --wait

	# Advanced
	breyta flows run order-ingest --target live --wait
	breyta flows run order-ingest --target draft --wait
	breyta flows run order-ingest --invocation import-orders --input '{"region":"EU"}' --wait
	breyta flows run order-ingest --target draft --interface-id manual-import --input '{"limit":5}' --wait
	breyta flows run order-ingest --installation-id inst_123 --wait
	breyta flows run paid-public-flow --buyer-test --installation-id inst_buyer_test --wait
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "flows run requires --api/BREYTA_API_URL")
			}
			installationID = strings.TrimSpace(installationID)
			if buyerTest {
				if installationID == "" {
					return writeErr(cmd, errors.New("--buyer-test requires --installation-id; create or list the Buyer Test installation from the Buyer Test workspace with `breyta flows installations create <flow-slug> --buyer-test-source-install --source-workspace-id <source-workspace-id> --source-flow-slug <flow-slug>`"))
				}
				if cmd.Flags().Changed("target") {
					return writeErr(cmd, errors.New("--buyer-test cannot be combined with --target; Buyer Test runs are installation-scoped"))
				}
			}
			resolvedTarget := ""
			if cmd.Flags().Changed("target") {
				var err error
				resolvedTarget, err = normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
			} else if installationID != "" {
				resolvedTarget = "live"
			} else {
				resolvedTarget = "draft"
			}
			payload := map[string]any{"flowSlug": args[0]}
			if resolvedTarget != "" {
				payload["target"] = resolvedTarget
			}
			if installationID != "" {
				payload["installationId"] = installationID
			}
			if version > 0 {
				payload["version"] = version
			}
			invocation = strings.TrimSpace(invocation)
			if invocation != "" {
				payload["invocation"] = invocation
			}
			interfaceID = strings.TrimSpace(interfaceID)
			triggerID = strings.TrimSpace(triggerID)
			if interfaceID != "" && triggerID != "" && interfaceID != triggerID {
				return writeErr(cmd, fmt.Errorf("--interface-id and --trigger-id refer to the same manual selector; provide only one"))
			}
			manualSelector := interfaceID
			if manualSelector == "" {
				manualSelector = triggerID
			}
			if manualSelector != "" {
				payload["triggerId"] = manualSelector
			}
			if strings.TrimSpace(inputJSON) != "" {
				m, err := parseJSONObjectFlag(inputJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
				payload["input"] = m
			}
			if len(trimStringSlice(uploads)) > 0 {
				input, _ := payload["input"].(map[string]any)
				if input == nil {
					input = map[string]any{}
				}
				existingFields := map[string]bool{}
				for key := range input {
					existingFields[key] = true
				}
				if err := applyFlowRunUploads(cmd.Context(), app, input, uploads, existingFields); err != nil {
					return writeErr(cmd, err)
				}
				payload["input"] = input
			}
			return doRunCommandWithOptionalWait(cmd, app, "flows.run", payload, wait, timeout, poll)
		},
	}

	cmd.Flags().StringVar(&installationID, "installation-id", "", "Advanced: run under a specific installation id")
	cmd.Flags().StringVar(&target, "target", "", "Advanced: run target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Advanced: release version override")
	cmd.Flags().StringVar(&invocation, "invocation", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&invocation, "invocation-id", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&interfaceID, "interface-id", "", "Manual interface id to select explicitly")
	cmd.Flags().StringVar(&interfaceID, "interface", "", "Alias for --interface-id")
	cmd.Flags().StringVar(&triggerID, "trigger-id", "", "Compatibility: legacy manual trigger id")
	cmd.Flags().StringVar(&triggerID, "trigger", "", "Compatibility alias for --trigger-id")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input")
	cmd.Flags().StringArrayVar(&uploads, "upload", nil, "Upload local file into a manual file/blob-ref input (field=path, repeatable)")
	cmd.Flags().BoolVar(&buyerTest, "buyer-test", false, "Buyer Test Mode: run the specified Buyer Test installation id")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Wait timeout")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting")
	return cmd
}

func newFlowsRunStepCmd(app *App) *cobra.Command {
	var installationID string
	var profileID string
	var target string
	var version int
	var invocation string
	var inputJSON string
	var wait bool
	var timeout time.Duration
	var poll time.Duration

	cmd := &cobra.Command{
		Use:   "run-step <flow-slug> <step-id>",
		Short: "Run one named flow step without other flow steps",
		Long: strings.TrimSpace(`
Run one step from a selected flow target/profile using the normal flow runtime
bindings, templates, and run output surfaces. Non-selected flow steps are not
executed, and the run stops immediately after the selected step completes.
This author/debug command requires workspace creator or admin permissions.

Default:
- breyta flows run-step <flow-slug> <step-id> [--input '{...}'] [--wait]

Advanced targeting:
- --target draft|live : select workspace draft/live when not using --installation-id
- --installation-id <id> : run under a specific live installation/profile
- --profile-id <id> : alias for selecting a specific profile
- --version <n> : force a specific release version
- --invocation <id> : validate input against a named invocation contract
		`),
		Example: strings.TrimSpace(`
breyta flows run-step ai-social-publisher draft-platform-posts --target live --input '{"topic":"launch"}' --wait
breyta flows run-step order-ingest normalize-order --target draft --input '{"orderId":"ord_123"}' --wait
breyta flows run-step report-builder summarize --installation-id prof_123 --input '{"range":"last_week"}' --wait
		`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "flows run-step requires --api/BREYTA_API_URL")
			}
			installationID = strings.TrimSpace(installationID)
			profileID = strings.TrimSpace(profileID)
			if installationID != "" && profileID != "" {
				return writeErr(cmd, fmt.Errorf("--installation-id and --profile-id refer to the same run profile; provide only one"))
			}
			resolvedTarget := ""
			if cmd.Flags().Changed("target") {
				var err error
				resolvedTarget, err = normalizeInstallTarget(target)
				if err != nil {
					return writeErr(cmd, err)
				}
			} else if installationID != "" || profileID != "" {
				resolvedTarget = "live"
			} else {
				resolvedTarget = "draft"
			}
			payload := map[string]any{
				"flowSlug": args[0],
				"stepId":   args[1],
			}
			if resolvedTarget != "" {
				payload["target"] = resolvedTarget
			}
			if installationID != "" {
				payload["installationId"] = installationID
			}
			if profileID != "" {
				payload["profileId"] = profileID
			}
			if version > 0 {
				payload["version"] = version
			}
			invocation = strings.TrimSpace(invocation)
			if invocation != "" {
				payload["invocation"] = invocation
			}
			if strings.TrimSpace(inputJSON) != "" {
				m, err := parseJSONObjectFlag(inputJSON)
				if err != nil {
					return writeErr(cmd, fmt.Errorf("invalid --input JSON: %w", err))
				}
				payload["input"] = m
			}
			return doRunCommandWithOptionalWait(cmd, app, "flows.run_step", payload, wait, timeout, poll)
		},
	}

	cmd.Flags().StringVar(&installationID, "installation-id", "", "Advanced: run under a specific installation id")
	cmd.Flags().StringVar(&profileID, "profile-id", "", "Advanced: run under a specific profile id")
	cmd.Flags().StringVar(&target, "target", "", "Advanced: run target override (draft|live)")
	cmd.Flags().IntVar(&version, "version", 0, "Advanced: release version override")
	cmd.Flags().StringVar(&invocation, "invocation", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&invocation, "invocation-id", "", "Advanced: named invocation input contract")
	cmd.Flags().StringVar(&inputJSON, "input", "", "JSON object input")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for run completion")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultFlowRunWaitTimeout, "Wait timeout")
	cmd.Flags().DurationVar(&poll, "poll", 250*time.Millisecond, "Poll interval while waiting")
	return cmd
}

func newFlowsReleaseCmd(app *App) *cobra.Command {
	var skipPromoteInstallations bool
	var version string
	var deployKey string
	var releaseNote string
	var releaseNoteFile string
	var legacyNote string

	cmd := &cobra.Command{
		Use:   "release <flow-slug>",
		Short: "Activate the latest pushed version, promote live, and promote track-latest installations by default",
		Long: strings.TrimSpace(`
Activate a released flow version for the current workspace.

By default, release reuses the latest version from workspace current, promotes
live, and promotes track-latest installations in the current workspace. Use
--version to activate a specific released version instead. Use
--skip-promote-installations when you want to update live without promoting
end-user installations.

When you know what changed, attach a markdown release note so the activated
version carries operator-facing context:
- breyta flows release my-flow --release-note 'Updated retry policy and fixed idempotency'
- breyta flows release my-flow --release-note-file ./release-note.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "release requires --api/BREYTA_API_URL")
			}

			payload := map[string]any{"flowSlug": args[0]}
			version = strings.TrimSpace(version)
			if version != "" && version != "latest" {
				v, err := parsePositiveIntFlag(version)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["version"] = v
			}

			resolvedDeployKey := strings.TrimSpace(deployKey)
			if resolvedDeployKey == "" {
				resolvedDeployKey = strings.TrimSpace(os.Getenv("BREYTA_FLOW_DEPLOY_KEY"))
			}
			if resolvedDeployKey != "" {
				payload["deployKey"] = resolvedDeployKey
			}
			resolvedReleaseNote, err := resolveReleaseNoteInput(releaseNote, legacyNote, releaseNoteFile)
			if err != nil {
				return writeErr(cmd, err)
			}
			if strings.TrimSpace(resolvedReleaseNote) != "" {
				payload["releaseNote"] = resolvedReleaseNote
			}

			if err := requireAPI(app); err != nil {
				return writeErr(cmd, err)
			}

			client := apiClient(app)
			releaseOut, releaseStatus, err := client.DoCommand(context.Background(), "flows.release", payload)
			if err != nil {
				return writeErr(cmd, err)
			}
			releaseOK := releaseStatus < 400 && isOK(releaseOut)
			trackCommandTelemetry(app, "flows.release", payload, releaseStatus, releaseOK)
			if !releaseOK {
				if err := writeAPIResult(cmd, app, releaseOut, releaseStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}

			promotePayload := map[string]any{"flowSlug": args[0], "target": "live"}
			if skipPromoteInstallations {
				promotePayload["scope"] = "live"
			}
			releaseData, _ := releaseOut["data"].(map[string]any)
			if activeVersion := asInt(releaseData["activeVersion"]); activeVersion > 0 {
				promotePayload["version"] = activeVersion
			}
			if resolvedDeployKey != "" {
				promotePayload["deployKey"] = resolvedDeployKey
			}
			promoteOut, promoteStatus, err := client.DoCommand(context.Background(), "flows.promote", promotePayload)
			if err != nil {
				return writeErr(cmd, err)
			}
			promoteOK := promoteStatus < 400 && isOK(promoteOut)
			trackCommandTelemetry(app, "flows.promote", promotePayload, promoteStatus, promoteOK)
			if !promoteOK {
				if err := writeAPIResult(cmd, app, promoteOut, promoteStatus); err != nil {
					return writeErr(cmd, err)
				}
				return nil
			}

			activeVersion := asInt(releaseData["activeVersion"])
			promoteData := mapStringAny(promoteOut["data"])
			liveRuntime := releaseLiveRuntimeSummary(args[0], releaseData, promoteData)
			warnings := releaseLiveRuntimeWarnings(liveRuntime)
			combined := map[string]any{
				"ok": true,
				"workspaceId": func() string {
					if ws := commandWorkspaceID(releaseOut); ws != "" {
						return ws
					}
					return commandWorkspaceID(promoteOut)
				}(),
				"meta": func() map[string]any {
					meta := map[string]any{
						"released":    true,
						"installed":   !skipPromoteInstallations,
						"target":      "live",
						"liveRuntime": liveRuntime,
						"verifyHint":  "Live runtime can differ from flow activeVersion. Verify with `breyta flows show <slug> --target live` and `breyta flows run <slug> --target live --wait`.",
						"scope": func() string {
							if skipPromoteInstallations {
								return "live"
							}
							return "all"
						}(),
						"verifyCommands": []string{
							"breyta flows show " + args[0] + " --target live",
							"breyta flows run " + args[0] + " --target live --wait",
						},
					}
					if len(warnings) > 0 {
						meta["warnings"] = warnings
					}
					return meta
				}(),
				"data": map[string]any{
					"release":     releaseOut["data"],
					"install":     promoteOut["data"],
					"liveRuntime": liveRuntime,
				},
			}
			addPublicAppURLHint(app, combined, args[0])
			appendEnvelopeHints(combined, releaseNoteHintCommands(args[0], activeVersion)...)
			if err := writeAPIResult(cmd, app, combined, 200); err != nil {
				return writeErr(cmd, err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&skipPromoteInstallations, "skip-promote-installations", false, "Activate the version and promote live, but skip promoting end-user installations")
	cmd.Flags().StringVar(&version, "version", "", "Released version to activate (default latest from workspace current)")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "", "Deploy key for guarded flows (default: BREYTA_FLOW_DEPLOY_KEY)")
	cmd.Flags().StringVar(&releaseNote, "release-note", "", "Markdown release note to attach to the activated version")
	cmd.Flags().StringVar(&releaseNoteFile, "release-note-file", "", "Read markdown release note from file")
	cmd.Flags().StringVar(&legacyNote, "note", "", "Deprecated alias for --release-note")
	_ = cmd.Flags().MarkHidden("note")
	return cmd
}

func newFlowsDiffCmd(app *App) *cobra.Command {
	var from string
	var to string
	var fromVersion int
	var toVersion int
	var full bool
	var file string

	cmd := &cobra.Command{
		Use:   "diff <flow-slug>",
		Short: "Show a source diff between draft, live, or released versions",
		Long: strings.TrimSpace(`
Show a unified diff for flow source.

Defaults to live versus draft so you can inspect unpublished changes as
additions to the draft. If a
draft-only flow has pushed draft history but no live version, the server can
compare the current draft to the previous pushed draft version.

- breyta flows diff my-flow
- breyta flows diff my-flow --file ./flows/my-flow.clj
- breyta flows diff my-flow --from draft --to version --to-version 7
- breyta flows diff my-flow --from version --from-version 6 --to version --to-version 7
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isAPIMode(app) {
				return writeNotImplemented(cmd, app, "diff requires --api/BREYTA_API_URL")
			}

			fromChanged := cmd.Flags().Changed("from")
			toChanged := cmd.Flags().Changed("to")
			file = strings.TrimSpace(file)
			if file != "" && !fromChanged && !toChanged {
				from = "draft"
				to = "file"
			}
			if file != "" && !diffSourceStringIsFile(from) && !diffSourceStringIsFile(to) {
				return writeErr(cmd, errors.New("--file compares one side of the diff; use --from file or --to file when also passing explicit --from/--to"))
			}

			payload := map[string]any{"flowSlug": args[0]}
			if strings.TrimSpace(from) != "" {
				payload["from"] = strings.TrimSpace(from)
			}
			if strings.TrimSpace(to) != "" {
				payload["to"] = strings.TrimSpace(to)
			}
			if file != "" {
				b, err := readExplicitFile(file)
				if err != nil {
					return writeErr(cmd, err)
				}
				payload["fileLiteral"] = string(b)
				payload["fileLabel"] = filepath.Base(file)
			}
			if fromVersion > 0 {
				payload["fromVersion"] = fromVersion
			}
			if toVersion > 0 {
				payload["toVersion"] = toVersion
			}
			if full {
				payload["view"] = "full"
			} else {
				payload["view"] = "summary"
			}
			return doAPICommand(cmd, app, "flows.diff", payload)
		},
	}

	cmd.Flags().StringVar(&from, "from", "live", "Diff source (draft|live|version|file)")
	cmd.Flags().StringVar(&to, "to", "draft", "Diff target (draft|live|version|file)")
	cmd.Flags().IntVar(&fromVersion, "from-version", 0, "Version number when --from=version")
	cmd.Flags().IntVar(&toVersion, "to-version", 0, "Version number when --to=version")
	cmd.Flags().BoolVar(&full, "full", false, "Include the full unified diff")
	cmd.Flags().StringVar(&file, "file", "", "Compare a local .clj file against one side of the diff (default: draft to file)")
	return cmd
}

func diffSourceStringIsFile(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "file")
}
