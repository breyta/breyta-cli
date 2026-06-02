package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/breyta/breyta-cli/internal/live"
)

type liveTUIStepIORef struct {
	RowKey     string
	WorkflowID string
	StepID     string
	ToolCallID string
	TargetKind string
	Label      string
	Status     string
	UpdatedAt  time.Time
}

type liveTUIStepIOResult struct {
	Ref        liveTUIStepIORef
	Status     string
	Input      any
	Result     any
	ResultKind string
}

type liveTUIStepIOState struct {
	RowKey  string
	Ref     liveTUIStepIORef
	Loading bool
	Result  liveTUIStepIOResult
	Err     string
}

type liveTUIStepIOLoadedMsg struct {
	rowKey string
	ref    liveTUIStepIORef
	result liveTUIStepIOResult
	err    error
}

func newLiveTUIStepIOLoader(app *App) func(liveTUIStepIORef) (liveTUIStepIOResult, error) {
	return func(ref liveTUIStepIORef) (liveTUIStepIOResult, error) {
		return fetchLiveTUIStepIO(app, ref)
	}
}

func fetchLiveTUIStepIO(app *App, ref liveTUIStepIORef) (liveTUIStepIOResult, error) {
	if app == nil {
		return liveTUIStepIOResult{}, fmt.Errorf("step I/O loader unavailable")
	}
	workflowID := strings.TrimSpace(ref.WorkflowID)
	stepID := strings.TrimSpace(ref.StepID)
	if workflowID == "" || stepID == "" {
		if ref.TargetKind != "run" {
			return liveTUIStepIOResult{}, fmt.Errorf("selected step is missing workflow or step id")
		}
	}
	if ref.TargetKind == "run" {
		return fetchLiveTUIRunIO(app, ref, workflowID)
	}
	out, status, err := runAPICommandWithContextAndTimeout(context.Background(), app, "runs.get", map[string]any{
		"workflowId":         workflowID,
		"includeSteps":       true,
		"includeResult":      false,
		"includeStepResults": true,
		"stepId":             stepID,
	}, 20*time.Second)
	if err != nil {
		return liveTUIStepIOResult{}, err
	}
	if status >= 400 || !isOK(out) {
		return liveTUIStepIOResult{}, fmt.Errorf("runs.get failed with HTTP %d", status)
	}
	run := mapStringAny(mapStringAny(out["data"])["run"])
	step := findRunStep(run, stepID)
	if step == nil {
		return liveTUIStepIOResult{}, fmt.Errorf("step %q not found in run %s", stepID, workflowID)
	}
	stepStatus := firstNonBlankString(step["status"], ref.Status)
	input := firstPresent(step, "input", "params", "inputPreview", "input-preview", "paramsPreview", "params-preview")
	errValue := firstPresent(step, "errorOutput", "error-output", "error", "errorMessage", "error-message", "errorResource", "error-resource")
	resultKind := "output"
	result := firstPresent(step, "output", "result", "resultPreview", "result-preview", "outputPreview", "output-preview", "outputResource", "output-resource")
	if liveTUIProblemStatus(stepStatus) || errValue != nil {
		resultKind = "error"
		result = errValue
	}
	if strings.TrimSpace(ref.ToolCallID) != "" {
		return liveTUIToolCallIOResult(ref, stepStatus, result)
	}
	return liveTUIStepIOResult{
		Ref:        ref,
		Status:     stepStatus,
		Input:      input,
		Result:     result,
		ResultKind: resultKind,
	}, nil
}

func fetchLiveTUIRunIO(app *App, ref liveTUIStepIORef, workflowID string) (liveTUIStepIOResult, error) {
	out, status, err := runAPICommandWithContextAndTimeout(context.Background(), app, "runs.get", map[string]any{
		"workflowId":    workflowID,
		"includeSteps":  false,
		"includeResult": true,
	}, 20*time.Second)
	if err != nil {
		return liveTUIStepIOResult{}, err
	}
	if status >= 400 || !isOK(out) {
		return liveTUIStepIOResult{}, fmt.Errorf("runs.get failed with HTTP %d", status)
	}
	run := mapStringAny(mapStringAny(out["data"])["run"])
	if run == nil {
		return liveTUIStepIOResult{}, fmt.Errorf("run %s not found", workflowID)
	}
	runStatus := firstNonBlankString(run["status"], ref.Status)
	errValue := firstPresent(run, "error", "errorOutput", "error-output", "errorMessage", "error-message", "errorResource", "error-resource")
	resultKind := "output"
	result := firstPresent(run, "result", "resultPreview", "result-preview", "output", "outputPreview", "output-preview", "outputResource", "output-resource", "resultResourceUri", "resultResourceURI", "result-resource-uri")
	if liveTUIProblemStatus(runStatus) || errValue != nil {
		if errValue == nil {
			errValue = firstPresent(run, "resultResourceUri", "resultResourceURI", "result-resource-uri")
		}
		resultKind = "error"
		result = errValue
	}
	return liveTUIStepIOResult{
		Ref:        ref,
		Status:     runStatus,
		Input:      firstPresent(run, "input", "inputPreview", "input-preview"),
		Result:     result,
		ResultKind: resultKind,
	}, nil
}

func (m *liveTUIModel) startCursorStepIOInspect() (tea.Cmd, bool) {
	ref, ok := m.cursorStepIORef(m.visibleNodes())
	if !ok {
		return nil, false
	}
	m.stickEnd = false
	cacheKey := liveTUIStepIOCacheKey(ref)
	if cached, ok := m.stepIOCache[cacheKey]; ok {
		m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Result: cached}
		m.stepIOOffset = 0
		m.stepIOTab = m.stepIOResultKind()
		return nil, true
	}
	if m.loadStepIO == nil {
		m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Err: "step I/O loader unavailable"}
		m.stepIOOffset = 0
		m.stepIOTab = ""
		return nil, true
	}
	loader := m.loadStepIO
	m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Loading: true}
	m.stepIOOffset = 0
	m.stepIOTab = ""
	return func() tea.Msg {
		result, err := loader(ref)
		return liveTUIStepIOLoadedMsg{rowKey: ref.RowKey, ref: ref, result: result, err: err}
	}, true
}

func (m *liveTUIModel) handleStepIOLoaded(msg liveTUIStepIOLoadedMsg) {
	if strings.TrimSpace(msg.rowKey) == "" || msg.rowKey != m.cursorKey || msg.rowKey != m.stepIO.RowKey {
		return
	}
	m.stepIO.Loading = false
	if msg.err != nil {
		m.stepIO.Err = msg.err.Error()
		return
	}
	if m.stepIOCache == nil {
		m.stepIOCache = map[string]liveTUIStepIOResult{}
	}
	m.stepIOCache[liveTUIStepIOCacheKey(msg.ref)] = msg.result
	m.stepIO.Result = msg.result
	m.stepIO.Err = ""
	m.stepIOTab = m.stepIOResultKind()
	m.stepIOOffset = 0
}

func (m *liveTUIModel) clearStepIOIfSelectionChanged() {
	if m.stepIO.RowKey == "" || m.stepIO.RowKey == m.cursorKey {
		return
	}
	m.stepIO = liveTUIStepIOState{}
	m.stepIOTab = ""
	m.stepIOOffset = 0
}

func (m liveTUIModel) cursorStepIORef(visible []liveTreeNode) (liveTUIStepIORef, bool) {
	if m.cursor < 0 || m.cursor >= len(visible) || !isSelectableLiveNode(visible[m.cursor]) {
		return liveTUIStepIORef{}, false
	}
	node := visible[m.cursor]
	line := node.Line
	if !isInspectableStepLine(line) {
		return liveTUIStepIORef{}, false
	}
	if isInspectableRunLine(line) {
		return liveTUIStepIORef{
			RowKey:     node.Key,
			WorkflowID: strings.TrimSpace(line.WorkflowID),
			TargetKind: "run",
			Label:      firstNonBlankString(line.FlowSlug, node.Text),
			Status:     strings.TrimSpace(line.Status),
			UpdatedAt:  line.UpdatedAt,
		}, true
	}
	stepID := strings.TrimSpace(line.StepID)
	toolCallID := ""
	if isToolDisplayLine(line) {
		stepID = firstNonBlankString(line.ParentActivityID, line.ParentStepID, line.StepID)
		toolCallID = toolCallIDFromDisplayLine(line)
	}
	return liveTUIStepIORef{
		RowKey:     node.Key,
		WorkflowID: strings.TrimSpace(line.WorkflowID),
		StepID:     strings.TrimSpace(stepID),
		ToolCallID: strings.TrimSpace(toolCallID),
		Label:      firstNonBlankString(line.ActivityName, line.StepID, node.Text),
		Status:     strings.TrimSpace(line.Status),
		UpdatedAt:  line.UpdatedAt,
	}, true
}

func isInspectableStepLine(line live.DisplayLine) bool {
	if isInspectableRunLine(line) {
		return true
	}
	if line.Planned || line.Active || strings.TrimSpace(line.WorkflowID) == "" {
		return false
	}
	if strings.TrimSpace(line.StepID) == "" && !(isToolDisplayLine(line) && strings.TrimSpace(firstNonBlankString(line.ParentActivityID, line.ParentStepID)) != "") {
		return false
	}
	if strings.TrimSpace(line.RowKind) != "activity" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(line.ActivityKind), "resource") {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(line.Status))
	if status == "" && line.CompletedAt != nil {
		status = "completed"
	}
	return liveTUITerminalStatus(status)
}

func isInspectableRunLine(line live.DisplayLine) bool {
	if line.Planned || line.Active || strings.TrimSpace(line.RowKind) != "run" || strings.TrimSpace(line.WorkflowID) == "" {
		return false
	}
	if strings.TrimSpace(line.ParentWorkflowID) == "" && strings.TrimSpace(line.ParentStepID) == "" {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(line.Status))
	if status == "" && line.CompletedAt != nil {
		status = "completed"
	}
	return liveTUITerminalStatus(status)
}

func isToolDisplayLine(line live.DisplayLine) bool {
	switch strings.ToLower(strings.TrimSpace(line.ActivityKind)) {
	case "tool", "tool_call", "mcp_tool_call":
		return true
	default:
		return false
	}
}

func toolCallIDFromDisplayLine(line live.DisplayLine) string {
	if toolCallID := strings.TrimSpace(line.ToolCallID); toolCallID != "" {
		return toolCallID
	}
	parent := strings.TrimSpace(firstNonBlankString(line.ParentActivityID, line.ParentStepID))
	for _, candidate := range []string{line.ActivityID, line.StepID} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if parent != "" && strings.HasPrefix(candidate, parent+"/") {
			return strings.TrimPrefix(candidate, parent+"/")
		}
		return candidate
	}
	return ""
}

func liveTUIStepIOCacheKey(ref liveTUIStepIORef) string {
	updated := ""
	if !ref.UpdatedAt.IsZero() {
		updated = ref.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return strings.Join([]string{ref.TargetKind, ref.WorkflowID, ref.StepID, ref.ToolCallID, ref.Status, updated}, "\x00")
}

func liveTUITerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "succeeded", "success", "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func liveTUIProblemStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func liveTUIToolCallIOResult(ref liveTUIStepIORef, parentStatus string, parentResult any) (liveTUIStepIOResult, error) {
	toolCall := findLiveTUIToolCallRecord(parentResult, ref.ToolCallID, ref.Label)
	if toolCall == nil {
		return liveTUIStepIOResult{}, fmt.Errorf("tool call %q not found in parent step %s", ref.ToolCallID, ref.StepID)
	}
	errValue := firstPresent(toolCall, "error", "errorMessage", "error-message")
	resultKind := "output"
	result := firstPresent(toolCall, "output", "result", "response", "content")
	status := firstNonBlankString(toolCall["status"], parentStatus)
	if errValue != nil || strings.EqualFold(firstNonBlankString(toolCall["success"]), "false") || liveTUIProblemStatus(status) {
		resultKind = "error"
		result = errValue
		if strings.TrimSpace(status) == "" || strings.EqualFold(status, "completed") {
			status = "failed"
		}
	}
	input := firstPresent(toolCall, "input", "arguments", "args", "params", "parameters")
	return liveTUIStepIOResult{
		Ref:        ref,
		Status:     status,
		Input:      input,
		Result:     result,
		ResultKind: resultKind,
	}, nil
}

func findLiveTUIToolCallRecord(value any, toolCallID string, label string) map[string]any {
	return findLiveTUIToolCallRecordDepth(value, strings.TrimSpace(toolCallID), strings.TrimSpace(label), 0)
}

func findLiveTUIToolCallRecordDepth(value any, toolCallID string, label string, depth int) map[string]any {
	if depth > 8 || value == nil {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		if liveTUIToolCallRecordMatches(v, toolCallID, label) {
			return v
		}
		for _, key := range []string{"toolCalls", "tool_calls", "tool-calls", "tools"} {
			if found := findLiveTUIToolCallRecordDepth(v[key], toolCallID, label, depth+1); found != nil {
				return found
			}
		}
		for _, child := range v {
			if found := findLiveTUIToolCallRecordDepth(child, toolCallID, label, depth+1); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range v {
			if found := findLiveTUIToolCallRecordDepth(child, toolCallID, label, depth+1); found != nil {
				return found
			}
		}
	}
	return nil
}

func liveTUIToolCallRecordMatches(record map[string]any, toolCallID string, label string) bool {
	if toolCallID != "" {
		for _, key := range []string{"id", "toolCallId", "tool_call_id", "tool-call-id", "callId", "call_id"} {
			value := strings.TrimSpace(scalarString(record[key]))
			if value == toolCallID || strings.HasSuffix(value, "/"+toolCallID) || strings.HasSuffix(toolCallID, "/"+value) {
				return true
			}
		}
	}
	name := strings.TrimSpace(firstNonBlankString(record["name"], record["toolName"], record["tool_name"], record["tool-name"]))
	return name != "" && label != "" && strings.Contains(strings.ToLower(label), strings.ToLower(name))
}

func renderStepIOPreview(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return renderCompactPreview(value)
		}
		return string(b)
	}
}

const liveTUIInspectJSONStringMaxBytes = 1 << 20

func normalizeLiveTUIInspectableValue(value any) any {
	return normalizeLiveTUIInspectableValueDepth(value, 0)
}

func normalizeLiveTUIInspectableValueDepth(value any, depth int) any {
	if depth > 8 || value == nil {
		return value
	}
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if len(trimmed) == 0 || len(trimmed) > liveTUIInspectJSONStringMaxBytes {
			return value
		}
		if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
			return value
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return value
		}
		return normalizeLiveTUIInspectableValueDepth(parsed, depth+1)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeLiveTUIInspectableValueDepth(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeLiveTUIInspectableValueDepth(item, depth+1)
		}
		return out
	default:
		return value
	}
}

func redactLiveTUISensitiveValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			if liveTUISensitiveKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactLiveTUISensitiveValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactLiveTUISensitiveValue(item)
		}
		return out
	default:
		return value
	}
}

func liveTUISensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, marker := range []string{"authorization", "password", "secret", "token", "api_key", "api-key", "apikey"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func stepIOStatusStyle(status string) string {
	if liveTUIProblemStatus(status) {
		return styleTUIFg(status, "203")
	}
	return styleTUIFg(status, "121")
}
