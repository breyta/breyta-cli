package cli

import (
	"context"
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
		return liveTUIStepIOResult{}, fmt.Errorf("selected step is missing workflow or step id")
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
	errValue := firstPresent(step, "errorOutput", "error-output", "error", "errorMessage", "error-message")
	resultKind := "output"
	result := firstPresent(step, "output", "result", "resultPreview", "result-preview", "outputPreview", "output-preview")
	if liveTUIProblemStatus(stepStatus) || errValue != nil {
		resultKind = "error"
		result = errValue
	}
	return liveTUIStepIOResult{
		Ref:        ref,
		Status:     stepStatus,
		Input:      input,
		Result:     result,
		ResultKind: resultKind,
	}, nil
}

func (m *liveTUIModel) startCursorStepIOInspect() (tea.Cmd, bool) {
	ref, ok := m.cursorStepIORef(m.visibleNodes())
	if !ok {
		return nil, false
	}
	cacheKey := liveTUIStepIOCacheKey(ref)
	if cached, ok := m.stepIOCache[cacheKey]; ok {
		m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Result: cached}
		return nil, true
	}
	if m.loadStepIO == nil {
		m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Err: "step I/O loader unavailable"}
		return nil, true
	}
	loader := m.loadStepIO
	m.stepIO = liveTUIStepIOState{RowKey: ref.RowKey, Ref: ref, Loading: true}
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
}

func (m *liveTUIModel) clearStepIOIfSelectionChanged() {
	if m.stepIO.RowKey == "" || m.stepIO.RowKey == m.cursorKey {
		return
	}
	m.stepIO = liveTUIStepIOState{}
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
	return liveTUIStepIORef{
		RowKey:     node.Key,
		WorkflowID: strings.TrimSpace(line.WorkflowID),
		StepID:     strings.TrimSpace(line.StepID),
		Label:      firstNonBlankString(line.ActivityName, line.StepID, node.Text),
		Status:     strings.TrimSpace(line.Status),
		UpdatedAt:  line.UpdatedAt,
	}, true
}

func isInspectableStepLine(line live.DisplayLine) bool {
	if line.Planned || line.Active || strings.TrimSpace(line.WorkflowID) == "" || strings.TrimSpace(line.StepID) == "" {
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

func liveTUIStepIOCacheKey(ref liveTUIStepIORef) string {
	updated := ""
	if !ref.UpdatedAt.IsZero() {
		updated = ref.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return strings.Join([]string{ref.WorkflowID, ref.StepID, ref.Status, updated}, "\x00")
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

func (m liveTUIModel) stepIOPaneHeight() int {
	if m.stepIO.RowKey == "" {
		return 0
	}
	available := m.height - m.headerHeight() - m.headerSeparatorHeight() - m.footerHeight()
	if available < 5 {
		return 0
	}
	height := 8
	if available < height+3 {
		height = available / 2
		if height < 4 {
			height = 4
		}
	}
	return height
}

func (m liveTUIModel) stepIOPaneLines() []string {
	height := m.stepIOPaneHeight()
	if height <= 0 {
		return nil
	}
	lines := []string{m.footerSeparator()}
	contentRows := height - 1
	title := "step I/O"
	if label := strings.TrimSpace(m.stepIO.Ref.Label); label != "" {
		title += " " + styleTUIFg(label, "248")
	}
	if stepID := strings.TrimSpace(m.stepIO.Ref.StepID); stepID != "" {
		title += " " + styleTUIFg("["+stepID+"]", "244")
	}
	if status := strings.TrimSpace(firstNonBlankString(m.stepIO.Result.Status, m.stepIO.Ref.Status)); status != "" {
		title += " " + stepIOStatusStyle(status)
	}
	lines = append(lines, title)
	contentRows--
	if contentRows <= 0 {
		return lines[:height]
	}
	if m.stepIO.Loading {
		lines = append(lines, "  "+styleTUIFg("loading input/result...", "220"))
		return padTUILines(lines, height)
	}
	if strings.TrimSpace(m.stepIO.Err) != "" {
		lines = append(lines, "  "+styleTUIFg("error", "203")+" "+m.stepIO.Err)
		return padTUILines(lines, height)
	}
	result := m.stepIO.Result
	resultKind := strings.TrimSpace(result.ResultKind)
	if resultKind == "" {
		resultKind = "output"
	}
	lines = append(lines, stepIOSectionLines("input", result.Input, contentRows/2)...)
	remaining := height - len(lines)
	lines = append(lines, stepIOSectionLines(resultKind, result.Result, remaining)...)
	return padTUILines(lines, height)
}

func stepIOSectionLines(label string, value any, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	lines := []string{styleTUIFg(label, "81")}
	if maxRows == 1 {
		return lines
	}
	preview := "not captured"
	if value != nil {
		preview = renderCompactPreview(redactLiveTUISensitiveValue(value))
	}
	if strings.TrimSpace(preview) == "" {
		preview = "not captured"
	}
	preview, truncated := truncateRunesWithFlag(preview, 700)
	parts := strings.Split(preview, "\n")
	for _, part := range parts {
		if len(lines) >= maxRows {
			break
		}
		lines = append(lines, "  "+part)
	}
	if truncated && len(lines) < maxRows {
		lines = append(lines, "  "+styleTUIFg("...", "244"))
	}
	return lines
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

func padTUILines(lines []string, height int) []string {
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		return lines[:height]
	}
	return lines
}
