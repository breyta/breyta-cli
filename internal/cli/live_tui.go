package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/breyta/breyta-cli/internal/browseropen"
	"github.com/breyta/breyta-cli/internal/live"
)

type liveTUIRunner struct {
	program *tea.Program
	done    chan error

	mu      sync.Mutex
	closing bool
	stopped bool
}

func newLiveTUIRunner(out io.Writer, resolver func(liveTUIWaitAction, string) error, stepIOLoader func(liveTUIStepIORef) (liveTUIStepIOResult, error)) *liveTUIRunner {
	runner := &liveTUIRunner{done: make(chan error, 1)}
	model := newLiveTUIModel()
	model.resolveWaitAction = resolver
	model.loadStepIO = stepIOLoader
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(out))
	runner.program = program
	go func() {
		_, err := program.Run()
		runner.mu.Lock()
		if !runner.closing {
			runner.stopped = true
		}
		runner.mu.Unlock()
		runner.done <- err
	}()
	return runner
}

func (r *liveTUIRunner) SendFrame(frame live.DisplayFrame, waitAction liveTUIWaitAction) {
	if r == nil || r.program == nil {
		return
	}
	r.program.Send(liveTUIFrameMsg{frame: frame, waitAction: waitAction, at: time.Now()})
}

func (r *liveTUIRunner) Close() {
	if r == nil || r.program == nil {
		return
	}
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
	r.program.Quit()
	select {
	case <-r.done:
	case <-time.After(500 * time.Millisecond):
	}
}

func (r *liveTUIRunner) StopRequested() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

type liveTUIFrameMsg struct {
	frame      live.DisplayFrame
	waitAction liveTUIWaitAction
	at         time.Time
}

type liveTUIOpenURLMsg struct {
	url string
	err error
}

type liveTUIWaitResolvedMsg struct {
	waitID string
	action string
	err    error
}

type liveTreeNode struct {
	Key        string
	Text       string
	WebURL     string
	Line       live.DisplayLine
	Depth      int
	Parent     int
	Expandable bool
	Planned    bool
}

type liveTUIModel struct {
	nodes        []liveTreeNode
	header       string
	collapsed    map[string]bool
	cursorKey    string
	cursor       int
	offset       int
	stickEnd     bool
	width        int
	height       int
	updatedAt    time.Time
	openURL      func(string) error
	loadStepIO   func(liveTUIStepIORef) (liveTUIStepIOResult, error)
	stepIOCache  map[string]liveTUIStepIOResult
	stepIO       liveTUIStepIOState
	stepIOTab    string
	stepIOOffset int

	waitAction        liveTUIWaitAction
	waitActionPending string
	waitActionMessage string
	resolveWaitAction func(liveTUIWaitAction, string) error
}

func newLiveTUIModel() liveTUIModel {
	return liveTUIModel{
		collapsed:   map[string]bool{},
		stickEnd:    true,
		width:       80,
		height:      24,
		openURL:     browseropen.Open,
		stepIOCache: map[string]liveTUIStepIOResult{},
	}
}

func (m liveTUIModel) Init() tea.Cmd {
	return nil
}

func (m liveTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.inspectOpen() {
			m.clampStepIOOffset()
		} else {
			m.ensureCursorVisible()
		}
	case tea.KeyMsg:
		if m.inspectOpen() {
			if cmd, handled := m.updateInspectKey(msg); handled {
				return m, cmd
			}
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.stickEnd = false
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
			m.updateStickEnd()
		case "home":
			m.stickEnd = false
			m.setCursor(0)
		case "end":
			visible := m.visibleNodes()
			m.setCursor(len(visible) - 1)
			m.stickEnd = true
			m.scrollToBottom(visible)
		case "pgup":
			m.stickEnd = false
			m.moveCursor(-m.pageSize())
		case "pgdown":
			m.moveCursor(m.pageSize())
			m.updateStickEnd()
		case "right":
			m.expandCursor()
		case "enter":
			if cmd, handled := m.startCursorStepIOInspect(); handled {
				return m, cmd
			}
			if cmd := m.openCursorWebURL(); cmd != nil {
				return m, cmd
			}
			m.expandCursor()
		case "a":
			if cmd := m.startWaitAction("approve"); cmd != nil {
				return m, cmd
			}
		case "r":
			if cmd := m.startWaitAction("reject"); cmd != nil {
				return m, cmd
			}
		case "left":
			m.collapseCursorOrMoveParent()
		case " ":
			m.toggleCursor()
		}
	case liveTUIFrameMsg:
		m.updatedAt = msg.at
		m.setWaitAction(msg.waitAction)
		m.setFrame(msg.frame)
	case liveTUIOpenURLMsg:
		_ = msg
	case liveTUIStepIOLoadedMsg:
		m.handleStepIOLoaded(msg)
	case liveTUIWaitResolvedMsg:
		m.waitActionPending = ""
		if msg.err != nil {
			m.waitActionMessage = msg.err.Error()
			break
		}
		m.waitActionMessage = msg.action + " sent"
		if strings.TrimSpace(msg.waitID) != "" && msg.waitID == m.waitAction.WaitID {
			m.waitAction = liveTUIWaitAction{}
		}
	}
	return m, nil
}

func (m liveTUIModel) View() string {
	if m.height <= 0 {
		return ""
	}
	if m.inspectOpen() {
		return m.inspectView()
	}
	visible := m.visibleNodes()
	m.ensureCursorVisible()
	bodyHeight := m.bodyHeight()
	lines := make([]string, 0, m.height)
	if m.header != "" {
		lines = append(lines, m.header)
	}
	if m.headerSeparatorHeight() > 0 {
		lines = append(lines, m.headerSeparator())
	}
	for row := 0; row < bodyHeight; row++ {
		idx := m.offset + row
		line := ""
		if idx < len(visible) {
			line = m.renderNodeLine(visible[idx], idx == m.cursor, m.width)
		}
		lines = append(lines, line)
	}
	switch m.footerHeight() {
	case 2:
		lines = append(lines, m.footerSeparator(), m.footer(visible))
	case 1:
		lines = append(lines, m.footer(visible))
	}
	for i, line := range lines {
		lines[i] = fitTUILine(line, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *liveTUIModel) setFrame(frame live.DisplayFrame) {
	oldCursorKey := m.cursorKey
	m.header, frame = prepareLiveTUIFrame(frame)
	m.nodes = buildLiveTreeNodes(frame)
	visible := m.visibleNodes()
	if len(visible) == 0 {
		m.cursor = 0
		m.cursorKey = ""
		m.offset = 0
		m.clearStepIOIfSelectionChanged()
		return
	}
	if m.stickEnd {
		m.cursor = followEndIndex(visible)
		m.cursorKey = cursorKeyAt(visible, m.cursor)
		m.ensureCursorVisible()
		m.scrollForStickEnd(visible)
		m.clearStepIOIfSelectionChanged()
		return
	}
	if oldCursorKey != "" {
		for i, node := range visible {
			if node.Key == oldCursorKey && isSelectableLiveNode(node) {
				m.cursor = i
				m.cursorKey = node.Key
				m.ensureCursorVisible()
				m.clearStepIOIfSelectionChanged()
				return
			}
		}
	}
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.cursor = nearestSelectableIndex(visible, m.cursor, 0)
	m.cursorKey = cursorKeyAt(visible, m.cursor)
	m.ensureCursorVisible()
	m.clearStepIOIfSelectionChanged()
}

func (m *liveTUIModel) setWaitAction(wait liveTUIWaitAction) {
	if !wait.Active {
		m.waitAction = liveTUIWaitAction{}
		m.waitActionPending = ""
		m.waitActionMessage = ""
		return
	}
	if wait.WaitID != m.waitAction.WaitID {
		m.waitActionPending = ""
		m.waitActionMessage = ""
	}
	m.waitAction = wait
}

func buildLiveTreeNodes(frame live.DisplayFrame) []liveTreeNode {
	nodes := make([]liveTreeNode, 0, len(frame.Lines))
	stack := []int{}
	for _, line := range frame.Lines {
		depth := displayLineDepth(line.Text)
		parent := -1
		if depth > 0 && len(stack) >= depth {
			parent = stack[depth-1]
		}
		key := line.Key
		if key == "" {
			key = fmt.Sprintf("line:%d", len(nodes))
		}
		webURL := strings.TrimSpace(line.WebURL)
		if line.Planned {
			webURL = ""
		}
		node := liveTreeNode{
			Key:     key,
			Text:    line.Text,
			WebURL:  webURL,
			Line:    line,
			Depth:   depth,
			Parent:  parent,
			Planned: line.Planned,
		}
		nodes = append(nodes, node)
		idx := len(nodes) - 1
		if parent >= 0 {
			nodes[parent].Expandable = true
		}
		if len(stack) > depth {
			stack = stack[:depth]
		}
		if len(stack) == depth {
			stack = append(stack, idx)
		} else {
			for len(stack) < depth {
				stack = append(stack, -1)
			}
			stack = append(stack, idx)
		}
	}
	return nodes
}

func prepareLiveTUIFrame(frame live.DisplayFrame) (string, live.DisplayFrame) {
	header := ""
	lines := make([]live.DisplayLine, 0, len(frame.Lines))
	for _, line := range frame.Lines {
		if strings.HasPrefix(strings.TrimSpace(line.Key), "header") {
			if header == "" {
				header = line.Text
			}
			continue
		}
		lines = append(lines, line)
	}
	lines = removeDuplicateTUIRootFlowLine(lines, header)
	return header, live.DisplayFrame{Lines: lines}
}

func removeDuplicateTUIRootFlowLine(lines []live.DisplayLine, header string) []live.DisplayLine {
	rootIdx := -1
	for i, line := range lines {
		if isTUIRootFlowLine(line, header, i == 0) {
			rootIdx = i
			break
		}
	}
	if rootIdx < 0 {
		return lines
	}
	rootDepth := displayLineDepth(lines[rootIdx].Text)
	out := make([]live.DisplayLine, 0, len(lines)-1)
	for i, line := range lines {
		if i == rootIdx {
			continue
		}
		if displayLineDepth(line.Text) > rootDepth {
			line.Text = trimLeadingTUISpaces(line.Text, 1)
		}
		out = append(out, line)
	}
	return out
}

func isTUIRootFlowLine(line live.DisplayLine, header string, firstBodyLine bool) bool {
	if !strings.HasPrefix(strings.TrimSpace(line.Key), "run:") {
		return false
	}
	if strings.TrimSpace(line.ParentWorkflowID) != "" || strings.TrimSpace(line.ParentStepID) != "" {
		return false
	}
	fields := strings.Fields(stripTUIANSI(line.Text))
	for i, field := range fields {
		if field == "f" || field == "ƒ" {
			if firstBodyLine {
				return true
			}
			return i+1 < len(fields) && strings.Contains(stripTUIANSI(header), fields[i+1])
		}
		if i >= 1 {
			break
		}
	}
	return false
}

func stripTUIANSI(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if end, ok := tuiANSIEscapeEnd(value, i); ok {
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

func trimLeadingTUISpaces(value string, max int) string {
	trimmed := 0
	for trimmed < len(value) && trimmed < max && value[trimmed] == ' ' {
		trimmed++
	}
	return value[trimmed:]
}

func displayLineDepth(text string) int {
	depth := 0
	for _, r := range text {
		if r != ' ' {
			break
		}
		depth++
	}
	return depth / 2
}

func (m liveTUIModel) visibleNodes() []liveTreeNode {
	visible := make([]liveTreeNode, 0, len(m.nodes))
	hiddenDepth := -1
	for _, node := range m.nodes {
		if hiddenDepth >= 0 {
			if node.Depth > hiddenDepth {
				continue
			}
			hiddenDepth = -1
		}
		visible = append(visible, node)
		if node.Expandable && m.collapsed[node.Key] {
			hiddenDepth = node.Depth
		}
	}
	return visible
}

func isSelectableLiveNode(node liveTreeNode) bool {
	key := strings.TrimSpace(node.Key)
	if key == "" || key == "waiting" || key == "summary" || strings.HasPrefix(key, "header") {
		return false
	}
	return strings.TrimSpace(node.Text) != ""
}

func selectableLiveNodeCount(nodes []liveTreeNode) int {
	count := 0
	for _, node := range nodes {
		if isSelectableLiveNode(node) {
			count++
		}
	}
	return count
}

func selectableLiveNodePosition(nodes []liveTreeNode, idx int) int {
	if idx < 0 || idx >= len(nodes) || !isSelectableLiveNode(nodes[idx]) {
		return 0
	}
	position := 0
	for i, node := range nodes {
		if !isSelectableLiveNode(node) {
			continue
		}
		position++
		if i == idx {
			return position
		}
	}
	return 0
}

func cursorKeyAt(nodes []liveTreeNode, idx int) string {
	if idx < 0 || idx >= len(nodes) || !isSelectableLiveNode(nodes[idx]) {
		return ""
	}
	return nodes[idx].Key
}

func lastSelectableIndex(nodes []liveTreeNode) int {
	for i := len(nodes) - 1; i >= 0; i-- {
		if isSelectableLiveNode(nodes[i]) {
			return i
		}
	}
	return 0
}

func firstSelectableIndex(nodes []liveTreeNode) int {
	for i, node := range nodes {
		if isSelectableLiveNode(node) {
			return i
		}
	}
	return 0
}

func followEndIndex(nodes []liveTreeNode) int {
	for i := len(nodes) - 1; i >= 0; i-- {
		if isSelectableLiveNode(nodes[i]) && !nodes[i].Planned {
			return i
		}
	}
	return firstSelectableIndex(nodes)
}

func nearestSelectableIndex(nodes []liveTreeNode, idx int, direction int) int {
	if len(nodes) == 0 {
		return 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(nodes) {
		idx = len(nodes) - 1
	}
	if isSelectableLiveNode(nodes[idx]) {
		return idx
	}
	if direction < 0 {
		for i := idx - 1; i >= 0; i-- {
			if isSelectableLiveNode(nodes[i]) {
				return i
			}
		}
		for i := idx + 1; i < len(nodes); i++ {
			if isSelectableLiveNode(nodes[i]) {
				return i
			}
		}
		return idx
	}
	for i := idx + 1; i < len(nodes); i++ {
		if isSelectableLiveNode(nodes[i]) {
			return i
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if isSelectableLiveNode(nodes[i]) {
			return i
		}
	}
	return idx
}

func (m *liveTUIModel) moveCursor(delta int) {
	visible := m.visibleNodes()
	if len(visible) == 0 || delta == 0 {
		return
	}
	idx := m.cursor + delta
	direction := 1
	if delta < 0 {
		direction = -1
	}
	m.setCursorNearest(idx, direction)
}

func (m *liveTUIModel) setCursor(idx int) {
	m.setCursorNearest(idx, 0)
}

func (m *liveTUIModel) setCursorNearest(idx int, direction int) {
	visible := m.visibleNodes()
	if len(visible) == 0 {
		m.cursor = 0
		m.cursorKey = ""
		m.offset = 0
		m.clearStepIOIfSelectionChanged()
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(visible) {
		idx = len(visible) - 1
	}
	m.cursor = nearestSelectableIndex(visible, idx, direction)
	m.cursorKey = cursorKeyAt(visible, m.cursor)
	m.ensureCursorVisible()
	m.clearStepIOIfSelectionChanged()
}

func (m *liveTUIModel) updateStickEnd() {
	visible := m.visibleNodes()
	last := followEndIndex(visible)
	m.stickEnd = last < 0 || (m.cursor >= last && !hasPlannedAfter(visible, last))
	if m.stickEnd {
		m.scrollForStickEnd(visible)
	}
}

func (m *liveTUIModel) toggleCursor() {
	visible := m.visibleNodes()
	if m.cursor < 0 || m.cursor >= len(visible) || !visible[m.cursor].Expandable || !isSelectableLiveNode(visible[m.cursor]) {
		return
	}
	key := visible[m.cursor].Key
	if m.collapsed[key] {
		delete(m.collapsed, key)
	} else {
		m.collapsed[key] = true
	}
	m.ensureCursorVisible()
}

func (m *liveTUIModel) expandCursor() {
	visible := m.visibleNodes()
	if m.cursor < 0 || m.cursor >= len(visible) || !isSelectableLiveNode(visible[m.cursor]) {
		return
	}
	delete(m.collapsed, visible[m.cursor].Key)
}

func (m liveTUIModel) openCursorWebURL() tea.Cmd {
	url := m.cursorWebURL(m.visibleNodes())
	if url == "" {
		return nil
	}
	openURL := m.openURL
	if openURL == nil {
		openURL = browseropen.Open
	}
	return func() tea.Msg {
		return liveTUIOpenURLMsg{url: url, err: openURL(url)}
	}
}

func (m *liveTUIModel) startWaitAction(action string) tea.Cmd {
	action = strings.ToLower(strings.TrimSpace(action))
	if !m.waitAction.Active || !m.waitAction.Can(action) || m.waitActionPending != "" || m.resolveWaitAction == nil {
		return nil
	}
	wait := m.waitAction
	resolver := m.resolveWaitAction
	m.waitActionPending = action
	m.waitActionMessage = ""
	return func() tea.Msg {
		return liveTUIWaitResolvedMsg{waitID: wait.WaitID, action: action, err: resolver(wait, action)}
	}
}

func (m liveTUIModel) cursorWebURL(visible []liveTreeNode) string {
	if m.cursor < 0 || m.cursor >= len(visible) || !isSelectableLiveNode(visible[m.cursor]) {
		return ""
	}
	return strings.TrimSpace(visible[m.cursor].WebURL)
}

func (m *liveTUIModel) collapseCursorOrMoveParent() {
	visible := m.visibleNodes()
	if m.cursor < 0 || m.cursor >= len(visible) || !isSelectableLiveNode(visible[m.cursor]) {
		return
	}
	node := visible[m.cursor]
	if node.Expandable && !m.collapsed[node.Key] {
		m.collapsed[node.Key] = true
		m.ensureCursorVisible()
		return
	}
	if node.Parent < 0 {
		return
	}
	parentKey := m.nodes[node.Parent].Key
	for i, candidate := range visible {
		if candidate.Key == parentKey {
			m.setCursor(i)
			return
		}
	}
}

func (m liveTUIModel) renderNodeLine(node liveTreeNode, selected bool, width int) string {
	text := node.Text
	marker := " "
	if node.Expandable {
		if m.collapsed[node.Key] {
			marker = styleTreeMarker("›")
		} else {
			marker = styleTreeMarker("⌄")
		}
	}
	if selected && isSelectableLiveNode(node) {
		return marker + highlightTUILabelText(text)
	}
	return marker + text
}

type tuiTextToken struct {
	rawStart int
	rawEnd   int
	text     string
}

func highlightTUILabelText(value string) string {
	if value == "" {
		return ""
	}
	tokens := tuiTextTokens(value)
	start := tuiLabelTokenStart(tokens)
	if start < 0 {
		return value
	}
	end := start + 1
	for end < len(tokens) && !isTUILabelMetadataToken(tokens[end].text) {
		end++
	}
	rawStart := tokens[start].rawStart
	rawEnd := tokens[end-1].rawEnd
	if rawStart < 0 || rawEnd <= rawStart || rawEnd > len(value) {
		return value
	}
	return value[:rawStart] + styleTUISelectedText(value[rawStart:rawEnd]) + value[rawEnd:]
}

func tuiTextTokens(value string) []tuiTextToken {
	tokens := []tuiTextToken{}
	inToken := false
	start := 0
	var b strings.Builder
	for i := 0; i < len(value); {
		if end, ok := tuiANSIEscapeEnd(value, i); ok {
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		if r == ' ' || r == '\t' {
			if inToken {
				tokens = append(tokens, tuiTextToken{rawStart: start, rawEnd: i, text: b.String()})
				b.Reset()
				inToken = false
			}
			i += size
			continue
		}
		if !inToken {
			start = i
			inToken = true
		}
		b.WriteRune(r)
		i += size
	}
	if inToken {
		tokens = append(tokens, tuiTextToken{rawStart: start, rawEnd: len(value), text: b.String()})
	}
	return tokens
}

func tuiLabelTokenStart(tokens []tuiTextToken) int {
	for i, token := range tokens {
		text := strings.TrimSpace(token.text)
		if text == "" || isTUIFoldMarkerToken(text) || isTUIStatusToken(text) || isTUICombinedStatusTypeMarkerToken(text) || isTUITypeMarkerToken(text, i, len(tokens)) {
			continue
		}
		return i
	}
	return -1
}

func isTUIFoldMarkerToken(text string) bool {
	return text == "›" || text == "⌄"
}

func isTUIStatusToken(text string) bool {
	switch text {
	case "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "✗", "■", "○", "•":
		return true
	default:
		return false
	}
}

func isTUICombinedStatusTypeMarkerToken(text string) bool {
	runes := []rune(text)
	if len(runes) != 2 {
		return false
	}
	return isTUIStatusToken(string(runes[0])) && isTUITypeMarkerToken(string(runes[1]), 0, 2)
}

func isTUITypeMarkerToken(text string, idx int, total int) bool {
	switch text {
	case "ƒ", "◉", "⚙", "✣", "↻", "◇", "▣":
		return true
	}
	return total > idx+1 && len([]rune(text)) == 1
}

func isTUILabelMetadataToken(text string) bool {
	if text == "" {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	if strings.HasPrefix(lower, "@") || strings.HasPrefix(lower, "(") || strings.HasPrefix(lower, "[") {
		return true
	}
	switch lower {
	case "failed", "running", "waiting", "cancelled", "canceled", "try", "iter":
		return true
	}
	if strings.Contains(lower, "/") {
		return true
	}
	return isTUINumericUnitToken(lower)
}

func isTUINumericUnitToken(text string) bool {
	for _, suffix := range []string{"ms", "kb", "mb", "gb", "tb", "b", "s", "m", "h"} {
		if !strings.HasSuffix(text, suffix) {
			continue
		}
		number := strings.TrimSuffix(text, suffix)
		if number != "" && tuiStringIsNumber(number) {
			return true
		}
	}
	return false
}

func tuiStringIsNumber(value string) bool {
	digits := 0
	dots := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return digits > 0
}

func styleTUISelectedText(value string) string {
	return "\x1b[48;5;236m" + value + "\x1b[49m"
}

func injectTreeMarker(text string, marker string) string {
	if text == "" {
		return styleTreeMarker(marker)
	}
	runes := []rune(text)
	for i, r := range runes {
		if r != ' ' {
			if i == 0 {
				return styleTreeMarker(marker) + " " + string(runes)
			}
			prefix := string(runes[:i])
			return prefix + styleTreeMarker(marker) + " " + string(runes[i:])
		}
	}
	return string(runes[:len(runes)-1]) + styleTreeMarker(marker)
}

func styleTreeMarker(marker string) string {
	return "\x1b[38;5;244m" + marker + "\x1b[39m"
}

func (m liveTUIModel) footer(visible []liveTreeNode) string {
	cursor := 0
	total := selectableLiveNodeCount(visible)
	if total > 0 {
		cursor = selectableLiveNodePosition(visible, m.cursor)
	}
	parts := []string{
		breytaTUILogoMark(),
		footerCommand("↑↓/jk", "move"),
		footerCommand("←→", "fold"),
		footerCommand("space", "toggle"),
		footerKey("pgup/pgdn"),
	}
	if m.cursorWebURL(visible) != "" {
		parts = append(parts, footerCommand("enter", "open"))
	} else if _, ok := m.cursorStepIORef(visible); ok {
		parts = append(parts, footerCommand("enter", "inspect"))
	}
	if m.waitAction.Active {
		parts = append(parts, footerWaitLabel(m.waitAction.Label()))
		if m.waitAction.Can("approve") {
			parts = append(parts, footerCommand("a", "approve"))
		}
		if m.waitAction.Can("reject") {
			parts = append(parts, footerCommand("r", "reject"))
		}
	}
	if m.waitActionPending != "" {
		parts = append(parts, styleTUIFg(m.waitActionPending+"...", "220"))
	} else if strings.TrimSpace(m.waitActionMessage) != "" {
		parts = append(parts, styleTUIFg(m.waitActionMessage, "248"))
	}
	parts = append(parts,
		footerCommand("q/ctrl+c", "exit"),
		footerPosition(cursor, total),
	)
	return strings.Join(parts, footerDivider())
}

func (m liveTUIModel) footerSeparator() string {
	if m.width <= 0 {
		return ""
	}
	return styleTUIFg(strings.Repeat("─", m.width), "244")
}

func (m liveTUIModel) headerSeparator() string {
	if m.width <= 0 {
		return ""
	}
	return styleTUIFg(strings.Repeat("─", m.width), "244")
}

func breytaTUILogoMark() string {
	return "\x1b[38;5;54;48;5;220m☷\x1b[0m"
}

func footerCommand(keys string, label string) string {
	return footerKey(keys) + " " + styleTUIFg(label, "248")
}

func footerKey(value string) string {
	return styleTUIFg(value, "81")
}

func footerPosition(cursor int, total int) string {
	return styleTUIFg(fmt.Sprintf("%d", cursor), "121") + styleTUIFg("/", "244") + styleTUIFg(fmt.Sprintf("%d", total), "121")
}

func footerWaitLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "wait"
	}
	return styleTUIFg("wait", "220") + " " + styleTUIFg(label, "248")
}

func footerDivider() string {
	return styleTUIFg(" | ", "244")
}

func styleTUIFg(value string, color string) string {
	if value == "" {
		return ""
	}
	return "\x1b[38;5;" + color + "m" + value + "\x1b[39m"
}

func (m liveTUIModel) bodyHeight() int {
	body := m.height - m.headerHeight() - m.headerSeparatorHeight() - m.footerHeight()
	if body < 0 {
		return 0
	}
	return body
}

func (m liveTUIModel) headerHeight() int {
	if m.header == "" || m.height <= 0 {
		return 0
	}
	return 1
}

func (m liveTUIModel) headerSeparatorHeight() int {
	if m.header == "" || m.offset <= 0 {
		return 0
	}
	available := m.height - m.headerHeight() - m.footerHeight()
	if available <= 1 {
		return 0
	}
	return 1
}

func (m liveTUIModel) footerHeight() int {
	available := m.height - m.headerHeight()
	if available > 2 {
		return 2
	}
	if available > 1 {
		return 1
	}
	return 0
}

func (m liveTUIModel) pageSize() int {
	size := m.bodyHeight()
	if size < 1 {
		return 1
	}
	return size
}

func (m *liveTUIModel) ensureCursorVisible() {
	bodyHeight := m.bodyHeight()
	if bodyHeight <= 0 {
		m.offset = 0
		return
	}
	visible := m.visibleNodes()
	if len(visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
	m.cursor = nearestSelectableIndex(visible, m.cursor, 0)
	m.cursorKey = cursorKeyAt(visible, m.cursor)
	if m.offset > m.cursor {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+bodyHeight {
		m.offset = m.cursor - bodyHeight + 1
	}
	maxOffset := len(visible) - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
	if m.stickEnd {
		m.scrollForStickEnd(visible)
	}
}

func (m *liveTUIModel) scrollToBottom(visible []liveTreeNode) {
	bodyHeight := m.bodyHeight()
	if bodyHeight <= 0 {
		m.offset = 0
		return
	}
	maxOffset := len(visible) - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.offset = maxOffset
}

func (m *liveTUIModel) scrollForStickEnd(visible []liveTreeNode) {
	if hasPlannedAfter(visible, m.cursor) {
		m.scrollToCursorBottom(visible)
		return
	}
	m.scrollToBottom(visible)
}

func (m *liveTUIModel) scrollToCursorBottom(visible []liveTreeNode) {
	bodyHeight := m.bodyHeight()
	if bodyHeight <= 0 {
		m.offset = 0
		return
	}
	maxOffset := len(visible) - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.cursor - bodyHeight + 1
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m.offset = offset
}

func hasPlannedAfter(nodes []liveTreeNode, idx int) bool {
	for i := idx + 1; i < len(nodes); i++ {
		if nodes[i].Planned {
			return true
		}
	}
	return false
}

func fitTUILine(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 1 {
		return "\x1b[K"
	}
	return truncateTUIRunes(value, width-1) + "\x1b[K"
}

func truncateTUIRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	visible := 0
	copiedANSI := false
	truncated := false
	for i := 0; i < len(value); {
		if end, ok := tuiANSIEscapeEnd(value, i); ok {
			b.WriteString(value[i:end])
			copiedANSI = true
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		runeWidth := runewidth.RuneWidth(r)
		if runeWidth < 0 {
			runeWidth = 0
		}
		if visible+runeWidth > width-1 {
			truncated = true
			break
		}
		b.WriteRune(r)
		visible += runeWidth
		i += size
	}
	if !truncated {
		return value
	}
	b.WriteString("…")
	if copiedANSI {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func tuiDisplayWidth(value string) int {
	width := 0
	for i := 0; i < len(value); {
		if end, ok := tuiANSIEscapeEnd(value, i); ok {
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(value[i:])
		runeWidth := runewidth.RuneWidth(r)
		if runeWidth > 0 {
			width += runeWidth
		}
		i += size
	}
	return width
}

func tuiANSIEscapeEnd(value string, start int) (int, bool) {
	if start+2 >= len(value) || value[start] != '\x1b' || value[start+1] != '[' {
		return start, false
	}
	for i := start + 2; i < len(value); i++ {
		if value[i] >= '@' && value[i] <= '~' {
			return i + 1, true
		}
	}
	return start, false
}
