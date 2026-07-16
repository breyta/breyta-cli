package cli

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// runFlowStepAfterWrite keeps the common draft authoring loop in one command:
// persist the step first, then prove the saved draft step with its authored
// defaults. The response is run-centric, but always retains the write result
// so a failed proof cannot be mistaken for a failed save.
func runFlowStepAfterWrite(cmd *cobra.Command, app *App, flowSlug string, stepID string, source string, idempotencyKey string, writeOut map[string]any, writeStatus int) error {
	if writeStatus >= 400 || !isOK(writeOut) {
		return writeAPIResult(cmd, app, writeOut, writeStatus)
	}

	payload := map[string]any{
		"flowSlug": strings.TrimSpace(flowSlug),
		"stepId":   strings.TrimSpace(stepID),
		"source":   strings.TrimSpace(source),
		"params":   map[string]any{},
	}
	if key := strings.TrimSpace(idempotencyKey); key != "" {
		payload["idempotencyKey"] = key
	}

	runOut, runStatus, err := apiClient(app).DoCommand(cmd.Context(), "flows.steps.run", payload)
	if err != nil {
		out := map[string]any{
			"ok": false,
			"data": map[string]any{
				"write": flowStepEnvelopeData(writeOut),
				"run":   nil,
			},
			"error": map[string]any{
				"code":    "step_run_after_write_failed",
				"message": "step was saved, but the proof run could not be completed",
				"details": map[string]any{
					"writeSucceeded": true,
					"cause":          err.Error(),
				},
			},
		}
		if workspaceID, _ := writeOut["workspaceId"].(string); strings.TrimSpace(workspaceID) != "" {
			out["workspaceId"] = workspaceID
		}
		addFlowStepAfterWriteHint(out, flowSlug, stepID)
		return writeAPIResult(cmd, app, out, http.StatusBadGateway)
	}

	// Match the normal flows steps run output contract before nesting it under
	// the combined response. This keeps --run compact by default.
	if err := compactStepsRunResult(runOut, stepID, stepResultPreviewOptions{}); err != nil {
		return writeErr(cmd, err)
	}
	if runOut == nil {
		runOut = map[string]any{
			"ok":    false,
			"error": map[string]any{"message": "proof run returned no response"},
		}
		runStatus = http.StatusBadGateway
	}
	writeData := flowStepEnvelopeData(writeOut)
	runData := flowStepEnvelopeData(runOut)
	runOut["data"] = map[string]any{
		"write": writeData,
		"run":   runData,
	}
	if _, exists := runOut["workspaceId"]; !exists {
		if workspaceID, _ := writeOut["workspaceId"].(string); strings.TrimSpace(workspaceID) != "" {
			runOut["workspaceId"] = workspaceID
		}
	}
	addFlowStepAfterWriteHint(runOut, flowSlug, stepID)
	if runStatus >= 400 || !isOK(runOut) {
		if errMap := mapStringAny(runOut["error"]); errMap != nil {
			details := mapStringAny(errMap["details"])
			if details == nil {
				details = map[string]any{}
				errMap["details"] = details
			}
			details["writeSucceeded"] = true
		}
	}
	return writeAPIResult(cmd, app, runOut, runStatus)
}

func flowStepEnvelopeData(out map[string]any) any {
	if out == nil {
		return nil
	}
	return out["data"]
}

func addFlowStepAfterWriteHint(out map[string]any, flowSlug string, stepID string) {
	if out == nil {
		return
	}
	meta := ensureMeta(out)
	if meta == nil {
		return
	}
	if isOK(out) {
		if _, exists := meta["hint"]; !exists {
			meta["hint"] = "Draft step saved and proved in one command."
		}
		return
	}
	meta["hint"] = "Draft step was saved, but its proof run failed; fix the step and rerun the proof."
	appendMetaNextCommands(meta, fmt.Sprintf("breyta flows steps run %s %s --source draft", strings.TrimSpace(flowSlug), strings.TrimSpace(stepID)))
}
