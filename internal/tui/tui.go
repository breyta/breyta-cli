package tui

import (
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "time"

        "breyta-cli/internal/mock"
        "breyta-cli/internal/state"

        "github.com/charmbracelet/bubbles/help"
        "github.com/charmbracelet/bubbles/key"
        "github.com/charmbracelet/bubbles/list"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
)

type tickMsg struct{}

type viewMode int

type dashFocus int

const (
        modeDashboard viewMode = iota
        modeFlow
        modeRun
        modeStep
        modeSettings
)

const (
        focusFlows dashFocus = iota
        focusRuns
)

type Model struct {
        workspaceID string
        statePath   string
        store       mock.Store
        st          *state.State

        lastMod time.Time
        err     error

        mode  viewMode
        focus dashFocus

        width  int
        height int

        flows    list.Model
        recent   list.Model
        steps    list.Model
        runSteps list.Model

        help help.Model
        keys keyMap

        selectedFlowSlug string
        selectedRunID    string
        selectedStepID   string

        stepContextRunID string // when opened from a run
}

func Run(workspaceID, statePath string, store mock.Store, st *state.State) error {
        m := NewModel(workspaceID, statePath, store, st)
        p := tea.NewProgram(m, tea.WithAltScreen())
        _, err := p.Run()
        return err
}

func NewModel(workspaceID, statePath string, store mock.Store, st *state.State) Model {
        flows := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
        flows.Title = "Flows"
        flows.SetShowHelp(false)
        flows.DisableQuitKeybindings()

        recent := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
        recent.Title = "Recent runs"
        recent.SetShowHelp(false)
        recent.DisableQuitKeybindings()

        steps := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
        steps.Title = "Flow"
        steps.SetShowHelp(false)
        steps.DisableQuitKeybindings()

        runSteps := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
        runSteps.Title = "Run"
        runSteps.SetShowHelp(false)
        runSteps.DisableQuitKeybindings()

        m := Model{
                workspaceID: workspaceID,
                statePath:   statePath,
                store:       store,
                st:          st,
                mode:        modeDashboard,
                focus:       focusFlows,
                flows:       flows,
                recent:      recent,
                steps:       steps,
                runSteps:    runSteps,
                help:        help.New(),
                keys:        defaultKeyMap(),
        }

        m.refreshFromState()
        m.lastMod = fileModTime(statePath)
        return m
}

func (m Model) Init() tea.Cmd {
        return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
        switch msg := msg.(type) {
        case tea.WindowSizeMsg:
                m.width = msg.Width
                m.height = msg.Height
                m.layout()
                return m, nil

        case tickMsg:
                mt := fileModTime(m.statePath)
                if mt.After(m.lastMod) {
                        st, err := m.store.Load()
                        if err == nil {
                                m.st = st
                                m.refreshFromState()
                                m.lastMod = mt
                        } else {
                                m.err = err
                        }
                }
                return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })

        case tea.KeyMsg:
                switch msg.String() {
                case "?":
                        m.help.ShowAll = !m.help.ShowAll
                        return m, nil

                case "q", "ctrl+c":
                        return m, tea.Quit

                case "g":
                        m.mode = modeDashboard
                        m.stepContextRunID = ""
                        m.layout()
                        return m, nil

                case "s":
                        m.mode = modeSettings
                        m.stepContextRunID = ""
                        m.layout()
                        return m, nil

                case "tab":
                        if m.mode == modeDashboard {
                                if m.focus == focusFlows {
                                        m.focus = focusRuns
                                } else {
                                        m.focus = focusFlows
                                }
                                m.layout()
                                return m, nil
                        }

                case "esc", "backspace":
                        switch m.mode {
                        case modeStep:
                                // If step was opened from run context, go back to run view
                                if m.stepContextRunID != "" {
                                        m.mode = modeRun
                                        m.layout()
                                        return m, nil
                                }
                                m.mode = modeFlow
                        case modeRun:
                                m.mode = modeDashboard
                        case modeFlow:
                                m.mode = modeDashboard
                        case modeSettings:
                                m.mode = modeDashboard
                        }
                        m.layout()
                        return m, nil

                case "enter":
                        switch m.mode {
                        case modeDashboard:
                                if m.focus == focusFlows {
                                        if it, ok := m.flows.SelectedItem().(flowItem); ok {
                                                m.selectedFlowSlug = it.slug
                                                m.mode = modeFlow
                                                m.stepContextRunID = ""
                                                m.refreshSteps()
                                                m.layout()
                                                return m, nil
                                        }
                                } else {
                                        if it, ok := m.recent.SelectedItem().(runItem); ok {
                                                m.selectedRunID = it.id
                                                m.mode = modeRun
                                                m.stepContextRunID = ""
                                                m.refreshRunSteps()
                                                m.layout()
                                                return m, nil
                                        }
                                }

                        case modeFlow:
                                if it, ok := m.steps.SelectedItem().(stepItem); ok {
                                        m.selectedStepID = it.id
                                        m.stepContextRunID = "" // latest run preview
                                        m.mode = modeStep
                                        m.layout()
                                        return m, nil
                                }

                        case modeRun:
                                if it, ok := m.runSteps.SelectedItem().(runStepItem); ok {
                                        m.selectedStepID = it.stepID
                                        m.stepContextRunID = m.selectedRunID
                                        m.mode = modeStep
                                        m.layout()
                                        return m, nil
                                }
                        }
                }
        }

        var cmd tea.Cmd
        switch m.mode {
        case modeDashboard:
                if m.focus == focusFlows {
                        m.flows, cmd = m.flows.Update(msg)
                } else {
                        m.recent, cmd = m.recent.Update(msg)
                }
        case modeFlow:
                m.steps, cmd = m.steps.Update(msg)
        case modeRun:
                m.runSteps, cmd = m.runSteps.Update(msg)
        case modeStep:
                // Static text view.
        case modeSettings:
                // Static text view.
        }
        return m, cmd
}

func (m Model) View() string {
        if m.width == 0 || m.height == 0 {
                return "Loading…"
        }

        header := m.renderHeader()
        bodyH := m.height - 3
        if bodyH < 3 {
                bodyH = 3
        }

        var body string
        switch m.mode {
        case modeDashboard:
                // Dashboard renders its own panes + borders (avoid double borders).
                body = m.renderDashboard(bodyH)
        case modeFlow:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.renderFlow(bodyH))
        case modeRun:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.renderRun())
        case modeStep:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.renderStep())
        case modeSettings:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.renderSettings())
        }

        footer := footerStyle().Render(m.help.View(m.keysForMode()))
        return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *Model) layout() {
        if m.height < 8 {
                m.height = 8
        }
        bodyH := m.height - 3
        if bodyH < 3 {
                bodyH = 3
        }

        // Dashboard: two panes in one row (no outer border)
        paneH := bodyH - 4 // title + stats + blank line
        if paneH < 5 {
                paneH = 5
        }
        paneW := (m.width - 1) / 2
        if paneW < 20 {
                paneW = 20
        }
        // Border+padding budget: 4 columns (2 border + 2 padding), 2 rows (border)
        listW := paneW - 4
        if listW < 10 {
                listW = 10
        }
        listH := paneH - 2
        if listH < 3 {
                listH = 3
        }
        m.flows.SetSize(listW, listH)
        m.recent.SetSize(listW, listH)

        // Detail views (inside a bordered panel)
        m.steps.SetSize(m.width-4, bodyH-4)
        m.runSteps.SetSize(m.width-4, bodyH-4)
}

func (m *Model) refreshFromState() {
        m.refreshFlows()
        m.refreshRecentRuns()
        if m.selectedFlowSlug != "" {
                m.refreshSteps()
        }
        if m.selectedRunID != "" {
                m.refreshRunSteps()
        }
}

func (m *Model) refreshFlows() {
        flows, err := m.store.ListFlows(m.st)
        if err != nil {
                m.err = err
                return
        }
        items := make([]list.Item, 0, len(flows))
        for _, f := range flows {
                items = append(items, flowItem{slug: f.Slug, name: f.Name, version: f.ActiveVersion, updatedAt: f.UpdatedAt})
        }
        m.flows.SetItems(items)
}

func (m *Model) refreshRecentRuns() {
        runs, err := m.store.ListRuns(m.st, "")
        if err != nil {
                m.err = err
                return
        }
        // take top N
        if len(runs) > 25 {
                runs = runs[:25]
        }
        items := make([]list.Item, 0, len(runs))
        for _, r := range runs {
                items = append(items, runItem{id: r.WorkflowID, flowSlug: r.FlowSlug, status: r.Status, startedAt: r.StartedAt, version: r.Version})
        }
        m.recent.SetItems(items)
}

func (m *Model) refreshSteps() {
        if m.selectedFlowSlug == "" {
                m.steps.SetItems(nil)
                return
        }
        f, err := m.store.GetFlow(m.st, m.selectedFlowSlug)
        if err != nil {
                m.err = err
                return
        }
        m.steps.Title = "Flow: " + f.Name
        items := make([]list.Item, 0, len(f.Steps))
        for i, s := range f.Steps {
                prefix := "├─"
                if i == len(f.Steps)-1 {
                        prefix = "└─"
                }
                items = append(items, stepItem{id: s.ID, line: fmt.Sprintf("%s %s  (%s)  %s", prefix, s.ID, s.Type, s.Title)})
        }
        m.steps.SetItems(items)
}

func (m *Model) refreshRunSteps() {
        if m.selectedRunID == "" {
                m.runSteps.SetItems(nil)
                return
        }
        r, err := m.store.GetRun(m.st, m.selectedRunID)
        if err != nil {
                m.err = err
                return
        }
        m.runSteps.Title = fmt.Sprintf("Run: %s (%s)", r.WorkflowID, r.Status)
        items := make([]list.Item, 0, len(r.Steps))
        for _, s := range r.Steps {
                items = append(items, runStepItem{stepID: s.StepID, status: s.Status, title: s.Title})
        }
        m.runSteps.SetItems(items)
}

func (m Model) renderHeader() string {
        ws := m.workspace()
        name := m.workspaceID
        plan := ""
        owner := ""
        if ws != nil {
                if ws.Name != "" {
                        name = ws.Name
                }
                plan = ws.Plan
                owner = ws.Owner
        }

        left := lipgloss.NewStyle().Bold(true).Render("Breyta")
        mid := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("workspace=%s", name))
        if plan != "" {
                mid += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  plan=" + plan)
        }
        if owner != "" {
                mid += lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  owner=" + owner)
        }

        right := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("state=" + filepath.Base(m.statePath))
        if m.err != nil {
                right = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("ERROR: " + m.err.Error())
        }
        return lipgloss.JoinHorizontal(lipgloss.Top, left+"  "+mid, padToRight(m.width-lenVisible(left+"  "+mid+"  "+right)), right)
}

func (m Model) renderDashboard(bodyH int) string {
        // Two-column dashboard: flows + recent runs (each pane has its own border).
        paneH := bodyH - 4 // title + stats + blank line
        if paneH < 5 {
                paneH = 5
        }
        paneW := (m.width - 1) / 2
        if paneW < 20 {
                paneW = 20
        }

        flowsPanel := paneStyle(m.focus == focusFlows).Width(paneW).Height(paneH).Render(m.flows.View())
        runsPanel := paneStyle(m.focus == focusRuns).Width(paneW).Height(paneH).Render(m.recent.View())

        ws := m.workspace()
        stats := ""
        if ws != nil {
                stats = fmt.Sprintf("Flows: %d  Runs: %d  Updated: %s", len(ws.Flows), len(ws.Runs), ws.UpdatedAt.Format(time.RFC3339))
        }
        statsLine := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(stats)

        return lipgloss.JoinVertical(lipgloss.Left,
                lipgloss.NewStyle().Bold(true).Render("Workspace dashboard"),
                statsLine,
                "",
                lipgloss.JoinHorizontal(lipgloss.Top, flowsPanel, runsPanel),
        )
}

func (m Model) renderFlow(bodyH int) string {
        f, err := m.store.GetFlow(m.st, m.selectedFlowSlug)
        if err != nil {
                return "Flow not found"
        }

        // NOTE: We intentionally do NOT render the textual spine here for now.
        // The flow panel has a fixed height and the list widget already supports
        // scrolling; mixing a long free-form spine above it caused confusing clipping.
        // We can reintroduce structure later with a dedicated scrollable viewport.

        header := lipgloss.NewStyle().Bold(true).Render("Flow") + "\n" +
                fmt.Sprintf("%s  v%d\n%s\n\n", f.Name, f.ActiveVersion, f.Description)

        return header +
                lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Steps (select → enter)") + "\n" +
                m.steps.View()
}

func (m Model) renderRun() string {
        r, err := m.store.GetRun(m.st, m.selectedRunID)
        if err != nil {
                return "Run not found"
        }

        header := lipgloss.NewStyle().Bold(true).Render("Run") + "\n" +
                fmt.Sprintf("run: %s\nflow: %s  v%d\nstatus: %s  triggeredBy: %s\nstarted: %s\n\n",
                        r.WorkflowID, r.FlowSlug, r.Version, r.Status, r.TriggeredBy, r.StartedAt.Format(time.RFC3339))

        return header + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Steps (select → enter)") + "\n" + m.runSteps.View()
}

func (m Model) renderStep() string {
        flowSlug := m.selectedFlowSlug
        // If opened from run context, align flow slug from the run.
        if m.stepContextRunID != "" {
                r, err := m.store.GetRun(m.st, m.stepContextRunID)
                if err == nil {
                        flowSlug = r.FlowSlug
                }
        }
        f, err := m.store.GetFlow(m.st, flowSlug)
        if err != nil {
                return "Flow not found"
        }
        var step *state.FlowStep
        for i := range f.Steps {
                if f.Steps[i].ID == m.selectedStepID {
                        step = &f.Steps[i]
                        break
                }
        }
        if step == nil {
                return "Step not found"
        }

        var input any
        var output any
        if m.stepContextRunID != "" {
                if r, err := m.store.GetRun(m.st, m.stepContextRunID); err == nil {
                        for i := range r.Steps {
                                if r.Steps[i].StepID == step.ID {
                                        input = r.Steps[i].InputPreview
                                        output = r.Steps[i].ResultPreview
                                        break
                                }
                        }
                }
        } else {
                runs, _ := m.store.ListRuns(m.st, f.Slug)
                if len(runs) > 0 {
                        for _, s := range runs[0].Steps {
                                if s.StepID == step.ID {
                                        input = s.InputPreview
                                        output = s.ResultPreview
                                        break
                                }
                        }
                }
        }

        lines := []string{
                lipgloss.NewStyle().Bold(true).Render("Step"),
                fmt.Sprintf("flow: %s", f.Slug),
                fmt.Sprintf("step: %s (%s)", step.ID, step.Type),
                "",
                lipgloss.NewStyle().Bold(true).Render("Definition"),
                step.Definition,
                "",
                lipgloss.NewStyle().Bold(true).Render("Input (concrete)"),
                prettyJSON(input),
                "",
                lipgloss.NewStyle().Bold(true).Render("Output (concrete)"),
                prettyJSON(output),
                "",
                lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Schemas (reference)"),
                lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Input:  " + step.InputSchema),
                lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Output: " + step.OutputSchema),
        }
        return joinLines(lines)
}

func (m Model) renderSettings() string {
        ws := m.workspace()
        if ws == nil {
                return "Workspace not found"
        }

        lines := []string{
                lipgloss.NewStyle().Bold(true).Render("Settings"),
                fmt.Sprintf("workspace: %s (%s)", ws.ID, ws.Name),
                fmt.Sprintf("plan: %s", ws.Plan),
                fmt.Sprintf("owner: %s", ws.Owner),
                fmt.Sprintf("updated: %s", ws.UpdatedAt.Format(time.RFC3339)),
                "",
                lipgloss.NewStyle().Bold(true).Render("Marketplace (mock)"),
                fmt.Sprintf("revenue events: %d", len(ws.RevenueEvents)),
                fmt.Sprintf("demand items: %d", len(ws.DemandTop)),
                "",
                "Shortcuts:",
                "- g: dashboard",
                "- s: settings",
        }
        return joinLines(lines)
}

func (m Model) keysForMode() keyMap {
        k := m.keys
        k.Dashboard.SetEnabled(true)
        k.Settings.SetEnabled(true)
        k.Focus.SetEnabled(m.mode == modeDashboard)

        switch m.mode {
        case modeDashboard:
                k.Enter.SetHelp("enter", "open")
                k.Back.SetHelp("esc", "")
        case modeFlow:
                k.Enter.SetHelp("enter", "open step")
                k.Back.SetHelp("esc", "back")
        case modeRun:
                k.Enter.SetHelp("enter", "open step")
                k.Back.SetHelp("esc", "back")
        case modeStep:
                k.Enter.SetHelp("enter", "")
                k.Back.SetHelp("esc", "back")
        case modeSettings:
                k.Enter.SetHelp("enter", "")
                k.Back.SetHelp("esc", "back")
        }
        return k
}

func (m Model) workspace() *state.Workspace {
        if m.st == nil {
                return nil
        }
        return m.st.Workspaces[m.workspaceID]
}

// --- Items ------------------------------------------------------------------

type flowItem struct {
        slug      string
        name      string
        version   int
        updatedAt time.Time
}

func (i flowItem) Title() string { return i.name }
func (i flowItem) Description() string {
        return fmt.Sprintf("%s  v%d  updated %s", i.slug, i.version, relTime(i.updatedAt))
}
func (i flowItem) FilterValue() string { return i.slug + " " + i.name }

type runItem struct {
        id        string
        flowSlug  string
        status    string
        startedAt time.Time
        version   int
}

func (i runItem) Title() string { return fmt.Sprintf("%s  %s", i.flowSlug, i.status) }
func (i runItem) Description() string {
        return fmt.Sprintf("run %s  v%d  %s", i.id, i.version, relTime(i.startedAt))
}
func (i runItem) FilterValue() string { return i.id + " " + i.flowSlug }

type stepItem struct {
        id   string
        line string
}

func (i stepItem) Title() string       { return i.line }
func (i stepItem) Description() string { return "" }
func (i stepItem) FilterValue() string { return i.id + " " + i.line }

type runStepItem struct {
        stepID string
        status string
        title  string
}

func (i runStepItem) Title() string       { return fmt.Sprintf("[%s] %s", i.status, i.stepID) }
func (i runStepItem) Description() string { return i.title }
func (i runStepItem) FilterValue() string { return i.stepID + " " + i.title }

// --- Styles / helpers --------------------------------------------------------

func panelStyle() lipgloss.Style {
        return lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("8")).
                Padding(0, 1).
                AlignVertical(lipgloss.Top).
                Align(lipgloss.Left)
}

func paneStyle(active bool) lipgloss.Style {
        if active {
                return lipgloss.NewStyle().
                        Border(lipgloss.RoundedBorder()).
                        BorderForeground(lipgloss.Color("12")).
                        Padding(0, 1).
                        AlignVertical(lipgloss.Top).
                        Align(lipgloss.Left)
        }
        return lipgloss.NewStyle().
                Border(lipgloss.RoundedBorder()).
                BorderForeground(lipgloss.Color("8")).
                Padding(0, 1).
                AlignVertical(lipgloss.Top).
                Align(lipgloss.Left)
}

func footerStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("8")) }

func joinLines(lines []string) string {
        out := ""
        for idx, l := range lines {
                if idx > 0 {
                        out += "\n"
                }
                out += l
        }
        return out
}

func fileModTime(path string) time.Time {
        fi, err := os.Stat(path)
        if err != nil {
                return time.Time{}
        }
        return fi.ModTime()
}

func relTime(t time.Time) string {
        if t.IsZero() {
                return "-"
        }
        d := time.Since(t)
        if d < 0 {
                d = -d
        }
        if d < time.Minute {
                return fmt.Sprintf("%ds ago", int(d.Seconds()))
        }
        if d < time.Hour {
                return fmt.Sprintf("%dm ago", int(d.Minutes()))
        }
        if d < 24*time.Hour {
                return fmt.Sprintf("%dh ago", int(d.Hours()))
        }
        return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func padToRight(n int) string {
        if n <= 0 {
                return ""
        }
        return lipgloss.NewStyle().Render(fmt.Sprintf("%*s", n, ""))
}

func lenVisible(s string) int {
        // naive; good enough for our header.
        return len([]rune(s))
}

func prettyJSON(v any) string {
        if v == nil {
                return "null"
        }
        b, err := json.MarshalIndent(v, "", "  ")
        if err != nil {
                return fmt.Sprintf("%v", v)
        }
        return string(b)
}

// --- Help / keymap -----------------------------------------------------------

type keyMap struct {
        Up        key.Binding
        Down      key.Binding
        Enter     key.Binding
        Back      key.Binding
        Help      key.Binding
        Quit      key.Binding
        Dashboard key.Binding
        Settings  key.Binding
        Focus     key.Binding
}

func defaultKeyMap() keyMap {
        return keyMap{
                Up: key.NewBinding(
                        key.WithKeys("up", "k"),
                        key.WithHelp("↑/k", "up"),
                ),
                Down: key.NewBinding(
                        key.WithKeys("down", "j"),
                        key.WithHelp("↓/j", "down"),
                ),
                Enter: key.NewBinding(
                        key.WithKeys("enter"),
                        key.WithHelp("enter", "open"),
                ),
                Back: key.NewBinding(
                        key.WithKeys("esc", "backspace"),
                        key.WithHelp("esc", "back"),
                ),
                Help: key.NewBinding(
                        key.WithKeys("?"),
                        key.WithHelp("?", "help"),
                ),
                Quit: key.NewBinding(
                        key.WithKeys("q", "ctrl+c"),
                        key.WithHelp("q", "quit"),
                ),
                Dashboard: key.NewBinding(
                        key.WithKeys("g"),
                        key.WithHelp("g", "dashboard"),
                ),
                Settings: key.NewBinding(
                        key.WithKeys("s"),
                        key.WithHelp("s", "settings"),
                ),
                Focus: key.NewBinding(
                        key.WithKeys("tab"),
                        key.WithHelp("tab", "switch pane"),
                ),
        }
}

func (k keyMap) ShortHelp() []key.Binding {
        return []key.Binding{k.Up, k.Down, k.Enter, k.Back, k.Focus, k.Dashboard, k.Settings, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
        return [][]key.Binding{
                {k.Up, k.Down, k.Enter, k.Back},
                {k.Focus, k.Dashboard, k.Settings},
                {k.Help, k.Quit},
        }
}

// Ensure we implement bubbles/help KeyMap interface.
var _ help.KeyMap = keyMap{}

// --- Sorting helpers (not used yet, reserved for future) --------------------

func sortRunsByRecent(runs []*state.Run) {
        sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
}
