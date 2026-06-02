package cli

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *liveTUIModel) updateInspectKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit, true
	case "esc", "backspace", "q":
		m.closeStepIOInspect()
		return nil, true
	case "up", "k":
		m.scrollStepIO(-1)
		return nil, true
	case "down", "j":
		m.scrollStepIO(1)
		return nil, true
	case "pgup":
		m.scrollStepIO(-m.inspectContentHeight())
		return nil, true
	case "pgdown":
		m.scrollStepIO(m.inspectContentHeight())
		return nil, true
	case "home":
		m.stepIOOffset = 0
		return nil, true
	case "end":
		m.stepIOOffset = m.maxStepIOOffset()
		return nil, true
	case "i":
		m.setStepIOTab("input")
		return nil, true
	case "o":
		m.setStepIOTab("output")
		return nil, true
	case "e":
		if m.stepIOResultKind() == "error" {
			m.setStepIOTab("error")
			return nil, true
		}
	case "tab":
		m.toggleStepIOTab()
		return nil, true
	}
	return nil, false
}

func (m liveTUIModel) inspectOpen() bool {
	return strings.TrimSpace(m.stepIO.RowKey) != ""
}

func (m *liveTUIModel) closeStepIOInspect() {
	m.stepIO = liveTUIStepIOState{}
	m.stepIOOffset = 0
	m.stepIOTab = ""
	m.ensureCursorVisible()
}

func (m *liveTUIModel) scrollStepIO(delta int) {
	if delta == 0 {
		return
	}
	m.stepIOOffset += delta
	m.clampStepIOOffset()
}

func (m *liveTUIModel) setStepIOTab(tab string) {
	tab = strings.ToLower(strings.TrimSpace(tab))
	if tab == "error" {
		tab = m.stepIOResultKind()
	}
	if tab != "input" && tab != "output" {
		tab = m.stepIOResultKind()
	}
	m.stepIOTab = tab
	m.stepIOOffset = 0
}

func (m *liveTUIModel) toggleStepIOTab() {
	if m.currentStepIOTab() == "input" {
		m.setStepIOTab(m.stepIOResultKind())
		return
	}
	m.setStepIOTab("input")
}

func (m liveTUIModel) currentStepIOTab() string {
	tab := strings.ToLower(strings.TrimSpace(m.stepIOTab))
	if tab == "input" || tab == "output" || tab == "error" {
		return tab
	}
	return m.stepIOResultKind()
}

func (m liveTUIModel) stepIOResultKind() string {
	kind := strings.ToLower(strings.TrimSpace(m.stepIO.Result.ResultKind))
	if kind == "" {
		kind = strings.ToLower(strings.TrimSpace(m.stepIO.Ref.Status))
	}
	if kind == "error" || liveTUIProblemStatus(kind) {
		return "error"
	}
	return "output"
}

func (m liveTUIModel) inspectView() string {
	lines := make([]string, 0, m.height)
	if m.header != "" {
		lines = append(lines, m.header)
	}
	if m.inspectHeaderSeparatorHeight() > 0 {
		lines = append(lines, m.headerSeparator())
	}
	content := m.inspectContentLines()
	contentHeight := m.inspectContentHeight()
	for row := 0; row < contentHeight; row++ {
		idx := m.stepIOOffset + row
		line := ""
		if idx < len(content) {
			line = content[idx]
		}
		lines = append(lines, line)
	}
	switch m.footerHeight() {
	case 2:
		lines = append(lines, m.footerSeparator(), m.inspectFooter(len(content)))
	case 1:
		lines = append(lines, m.inspectFooter(len(content)))
	}
	for i, line := range lines {
		lines[i] = fitTUILine(line, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m liveTUIModel) inspectContentLines() []string {
	lines := []string{m.inspectTitleLine()}
	if m.stepIO.Loading {
		lines = append(lines, "", "  "+styleTUIFg("loading input/result...", "220"))
		return lines
	}
	if strings.TrimSpace(m.stepIO.Err) != "" {
		lines = append(lines, "")
		for i, line := range inspectErrorLines(m.stepIO.Err, m.inspectContentWidth()) {
			prefix := "    "
			if i == 0 {
				prefix = "  " + styleTUIFg("error", "203") + " "
			}
			lines = append(lines, prefix+line)
		}
		return lines
	}
	lines = append(lines, m.inspectTabsLine(), "")
	tab := m.currentStepIOTab()
	label := tab
	value := m.stepIO.Result.Result
	if tab == "input" {
		value = m.stepIO.Result.Input
	} else if label == "" {
		label = "output"
	}
	lines = append(lines, styleTUIFg(label, "81"))
	lines = append(lines, inspectValueLines(value, m.inspectContentWidth())...)
	return lines
}

func (m liveTUIModel) inspectTitleLine() string {
	title := "step I/O"
	if strings.TrimSpace(m.stepIO.Ref.ToolCallID) != "" {
		title = "tool call I/O"
	} else if strings.TrimSpace(m.stepIO.Ref.TargetKind) == "run" {
		title = "run I/O"
	}
	if label := strings.TrimSpace(m.stepIO.Ref.Label); label != "" {
		title += " " + styleTUIFg(label, "248")
	}
	if stepID := strings.TrimSpace(m.stepIO.Ref.StepID); stepID != "" {
		title += " " + styleTUIFg("["+stepID+"]", "244")
	}
	if toolCallID := strings.TrimSpace(m.stepIO.Ref.ToolCallID); toolCallID != "" {
		title += " " + styleTUIFg("["+toolCallID+"]", "244")
	}
	if status := strings.TrimSpace(firstNonBlankString(m.stepIO.Result.Status, m.stepIO.Ref.Status)); status != "" {
		title += " " + stepIOStatusStyle(status)
	}
	return title
}

func (m liveTUIModel) inspectTabsLine() string {
	resultKind := m.stepIOResultKind()
	current := m.currentStepIOTab()
	input := inspectTabLabel("i", "input", current == "input")
	output := inspectTabLabel("o", resultKind, current == resultKind)
	return input + footerDivider() + output
}

func inspectTabLabel(key string, label string, selected bool) string {
	text := footerKey(key) + " " + styleTUIFg(label, "248")
	if selected {
		return styleTUISelectedText(stripTUIANSI(text))
	}
	return text
}

func (m liveTUIModel) inspectFooter(contentLen int) string {
	position := footerPosition(minInt(m.stepIOOffset+1, maxInt(contentLen, 1)), maxInt(contentLen, 1))
	parts := []string{
		breytaTUILogoMark(),
		footerCommand("↑↓/jk", "scroll"),
		footerKey("pgup/pgdn"),
		footerCommand("i/o", "tabs"),
		footerCommand("q/esc", "back"),
		footerCommand("ctrl+c", "exit"),
		position,
	}
	return strings.Join(parts, footerDivider())
}

func (m liveTUIModel) inspectContentHeight() int {
	body := m.height - m.headerHeight() - m.inspectHeaderSeparatorHeight() - m.footerHeight()
	if body < 0 {
		return 0
	}
	return body
}

func (m liveTUIModel) inspectHeaderSeparatorHeight() int {
	if m.header == "" {
		return 0
	}
	available := m.height - m.headerHeight() - m.footerHeight()
	if available <= 1 {
		return 0
	}
	return 1
}

func (m liveTUIModel) inspectContentWidth() int {
	width := m.width - 2
	if width < 20 {
		width = m.width
	}
	if width < 1 {
		return 1
	}
	return width
}

func (m *liveTUIModel) clampStepIOOffset() {
	maxOffset := m.maxStepIOOffset()
	if m.stepIOOffset > maxOffset {
		m.stepIOOffset = maxOffset
	}
	if m.stepIOOffset < 0 {
		m.stepIOOffset = 0
	}
}

func (m liveTUIModel) maxStepIOOffset() int {
	contentHeight := m.inspectContentHeight()
	if contentHeight <= 0 {
		return 0
	}
	maxOffset := len(m.inspectContentLines()) - contentHeight
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func inspectValueLines(value any, width int) []string {
	preview := "not captured"
	if value != nil {
		preview = renderStepIOPreview(redactLiveTUISensitiveValue(normalizeLiveTUIInspectableValue(value)))
	}
	preview = sanitizeLiveTUIInspectText(preview)
	if strings.TrimSpace(preview) == "" {
		preview = "not captured"
	}
	raw := strings.Split(preview, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		wrapped := wrapInspectLine(line, width)
		if len(wrapped) == 0 {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapped...)
	}
	return lines
}

func wrapInspectLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}
	indent := 0
	for indent < len(runes) && runes[indent] == ' ' {
		indent++
	}
	continuationIndent := indent + 2
	if continuationIndent > width/2 {
		continuationIndent = indent
	}
	prefix := strings.Repeat(" ", continuationIndent)
	out := []string{}
	for len(runes) > width {
		cut := width
		for cut > 0 && runes[cut-1] != ' ' && runes[cut-1] != ',' {
			cut--
		}
		if cut <= indent || cut < width/2 {
			cut = width
		}
		part := strings.TrimRight(string(runes[:cut]), " ")
		out = append(out, part)
		rest := strings.TrimLeft(string(runes[cut:]), " ")
		runes = []rune(prefix + rest)
	}
	out = append(out, string(runes))
	return out
}

func sanitizeLiveTUIInspectText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripTUIANSI(text)
	var b strings.Builder
	for _, r := range text {
		switch r {
		case '\n':
			b.WriteRune('\n')
		case '\t':
			b.WriteString("  ")
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeTUIInlineText(text string) string {
	text = sanitizeLiveTUIInspectText(text)
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func inspectErrorLines(errText string, width int) []string {
	text := sanitizeLiveTUIInspectText(errText)
	if strings.TrimSpace(text) == "" {
		return []string{"unknown error"}
	}
	innerWidth := width - 8
	if innerWidth < 20 {
		innerWidth = width
	}
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapInspectLine(line, innerWidth)...)
	}
	if len(lines) == 0 {
		return []string{"unknown error"}
	}
	return lines
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
