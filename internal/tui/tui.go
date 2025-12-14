package tui

import (
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"

        "breyta-cli/internal/mock"
        "breyta-cli/internal/state"

        "github.com/charmbracelet/bubbles/help"
        "github.com/charmbracelet/bubbles/key"
        "github.com/charmbracelet/bubbles/list"
        "github.com/charmbracelet/bubbles/viewport"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
)

type tickMsg struct{}

type viewMode int

type paneFocus int

const (
        modeDashboard viewMode = iota
        modeFlow
        modeRun
        modeStep
        modeSettings
)

const (
        focusPrimary paneFocus = iota
        focusSecondary
)

type Model struct {
        workspaceID string
        statePath   string
        store       mock.Store
        st          *state.State

        lastMod time.Time
        err     error

        mode  viewMode
        focus paneFocus

        width  int
        height int

        flows    list.Model
        recent   list.Model
        steps    list.Model
        runSteps list.Model

        detail viewport.Model

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

        vp := viewport.New(0, 0)
        vp.Style = lipgloss.NewStyle().AlignVertical(lipgloss.Top).Align(lipgloss.Left)

        m := Model{
                workspaceID: workspaceID,
                statePath:   statePath,
                store:       store,
                st:          st,
                mode:        modeDashboard,
                focus:       focusPrimary,
                flows:       flows,
                recent:      recent,
                steps:       steps,
                runSteps:    runSteps,
                detail:      vp,
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
                        m.focus = focusPrimary
                        m.layout()
                        return m, nil

                case "s":
                        m.mode = modeSettings
                        m.stepContextRunID = ""
                        m.focus = focusPrimary
                        m.refreshDetail()
                        m.layout()
                        return m, nil

                case "tab":
                        switch m.mode {
                        case modeDashboard, modeFlow, modeRun:
                                if m.focus == focusPrimary {
                                        m.focus = focusSecondary
                                } else {
                                        m.focus = focusPrimary
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
                                        m.focus = focusPrimary
                                        m.layout()
                                        return m, nil
                                }
                                m.mode = modeFlow
                                m.focus = focusPrimary
                        case modeRun:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        case modeFlow:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        case modeSettings:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        }
                        m.refreshDetail()
                        m.layout()
                        return m, nil

                case "enter":
                        switch m.mode {
                        case modeDashboard:
                                if m.focus == focusPrimary {
                                        if it, ok := m.flows.SelectedItem().(flowItem); ok {
                                                m.selectedFlowSlug = it.slug
                                                m.mode = modeFlow
                                                m.stepContextRunID = ""
                                                m.refreshSteps()
                                                m.refreshDetail()
                                                m.layout()
                                                return m, nil
                                        }
                                } else {
                                        if it, ok := m.recent.SelectedItem().(runItem); ok {
                                                m.selectedRunID = it.id
                                                m.mode = modeRun
                                                m.stepContextRunID = ""
                                                m.refreshRunSteps()
                                                m.refreshDetail()
                                                m.layout()
                                                return m, nil
                                        }
                                }

                        case modeFlow:
                                if m.focus == focusPrimary {
                                        if it, ok := m.steps.SelectedItem().(stepItem); ok {
                                                m.selectedStepID = it.id
                                                m.stepContextRunID = "" // latest run preview
                                                m.mode = modeStep
                                                m.focus = focusPrimary
                                                m.refreshDetail()
                                                m.layout()
                                                return m, nil
                                        }
                                }

                        case modeRun:
                                if m.focus == focusPrimary {
                                        if it, ok := m.runSteps.SelectedItem().(runStepItem); ok {
                                                m.selectedStepID = it.stepID
                                                m.stepContextRunID = m.selectedRunID
                                                m.mode = modeStep
                                                m.focus = focusPrimary
                                                m.refreshDetail()
                                                m.layout()
                                                return m, nil
                                        }
                                }
                        }
                }
        }

        var cmd tea.Cmd
        switch m.mode {
        case modeDashboard:
                if m.focus == focusPrimary {
                        m.flows, cmd = m.flows.Update(msg)
                } else {
                        m.recent, cmd = m.recent.Update(msg)
                }
        case modeFlow:
                if m.focus == focusPrimary {
                        m.steps, cmd = m.steps.Update(msg)
                } else {
                        m.detail, cmd = m.detail.Update(msg)
                }
                m.refreshDetail()
        case modeRun:
                if m.focus == focusPrimary {
                        m.runSteps, cmd = m.runSteps.Update(msg)
                } else {
                        m.detail, cmd = m.detail.Update(msg)
                }
                m.refreshDetail()
        case modeStep:
                m.detail, cmd = m.detail.Update(msg)
        case modeSettings:
                m.detail, cmd = m.detail.Update(msg)
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
                body = m.renderFlowView(bodyH)
        case modeRun:
                body = m.renderRunView(bodyH)
        case modeStep:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.detail.View())
        case modeSettings:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.detail.View())
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

        // Flow / run: split view (list + scrollable detail viewport)
        leftW := (m.width - 1) / 2
        if leftW > 56 {
                leftW = 56
        }
        if leftW < 26 {
                leftW = 26
        }
        rightW := m.width - leftW
        if rightW < 26 {
                rightW = 26
        }

        leftInnerW := leftW - 4
        if leftInnerW < 10 {
                leftInnerW = 10
        }
        rightInnerW := rightW - 4
        if rightInnerW < 10 {
                rightInnerW = 10
        }

        innerH := bodyH - 2
        if innerH < 3 {
                innerH = 3
        }

        m.steps.SetSize(leftInnerW, innerH)
        m.runSteps.SetSize(leftInnerW, innerH)
        m.detail.Width = rightInnerW
        m.detail.Height = innerH

        // Step / settings: full width scrollable detail in a panel.
        if m.mode == modeStep || m.mode == modeSettings {
                m.detail.Width = m.width - 4
                m.detail.Height = bodyH - 2
        }
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
        m.refreshDetail()
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

        flowsPanel := paneStyle(m.focus == focusPrimary).Width(paneW).Height(paneH).Render(m.flows.View())
        runsPanel := paneStyle(m.focus == focusSecondary).Width(paneW).Height(paneH).Render(m.recent.View())

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

func (m Model) renderFlowView(bodyH int) string {
        leftW := (m.width - 1) / 2
        if leftW > 56 {
                leftW = 56
        }
        if leftW < 26 {
                leftW = 26
        }
        rightW := m.width - leftW
        if rightW < 26 {
                rightW = 26
        }

        left := paneStyle(m.focus == focusPrimary).Width(leftW).Height(bodyH).Render(m.steps.View())
        right := paneStyle(m.focus == focusSecondary).Width(rightW).Height(bodyH).Render(m.detail.View())
        return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderRunView(bodyH int) string {
        leftW := (m.width - 1) / 2
        if leftW > 56 {
                leftW = 56
        }
        if leftW < 26 {
                leftW = 26
        }
        rightW := m.width - leftW
        if rightW < 26 {
                rightW = 26
        }

        left := paneStyle(m.focus == focusPrimary).Width(leftW).Height(bodyH).Render(m.runSteps.View())
        right := paneStyle(m.focus == focusSecondary).Width(rightW).Height(bodyH).Render(m.detail.View())
        return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
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

func (m *Model) refreshDetail() {
        switch m.mode {
        case modeFlow:
                m.detail.SetContent(m.renderFlowDetail())
        case modeRun:
                m.detail.SetContent(m.renderRunDetail())
        case modeStep:
                m.detail.SetContent(m.renderStep())
        case modeSettings:
                m.detail.SetContent(m.renderSettings())
        }
}

func (m Model) renderFlowDetail() string {
        if m.selectedFlowSlug == "" {
                return "No flow selected"
        }
        f, err := m.store.GetFlow(m.st, m.selectedFlowSlug)
        if err != nil {
                return "Flow not found"
        }

        var b strings.Builder
        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Flow details"))
        b.WriteString("\n")
        b.WriteString(fmt.Sprintf("%s  (%s)\n", f.Name, f.Slug))
        b.WriteString(fmt.Sprintf("version: v%d\n", f.ActiveVersion))
        if !f.UpdatedAt.IsZero() {
                b.WriteString(fmt.Sprintf("updated: %s\n", f.UpdatedAt.Format(time.RFC3339)))
        }
        if f.Description != "" {
                b.WriteString("\n")
                b.WriteString(lipgloss.NewStyle().Bold(true).Render("Description"))
                b.WriteString("\n")
                b.WriteString(f.Description)
                b.WriteString("\n")
        }

        if len(f.Spine) > 0 {
                b.WriteString("\n")
                b.WriteString(lipgloss.NewStyle().Bold(true).Render("Spine"))
                b.WriteString("\n")
                for _, line := range f.Spine {
                        b.WriteString("- ")
                        b.WriteString(line)
                        b.WriteString("\n")
                }
        }

        // Step preview (selected in the steps list)
        if it, ok := m.steps.SelectedItem().(stepItem); ok && it.id != "" {
                var step *state.FlowStep
                for i := range f.Steps {
                        if f.Steps[i].ID == it.id {
                                step = &f.Steps[i]
                                break
                        }
                }
                if step != nil {
                        b.WriteString("\n")
                        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Selected step"))
                        b.WriteString("\n")
                        b.WriteString(fmt.Sprintf("%s  (%s)\n", step.ID, step.Type))
                        if step.Title != "" {
                                b.WriteString(step.Title)
                                b.WriteString("\n")
                        }
                        if step.Definition != "" {
                                b.WriteString("\n")
                                b.WriteString(lipgloss.NewStyle().Bold(true).Render("Definition"))
                                b.WriteString("\n")
                                b.WriteString(step.Definition)
                                b.WriteString("\n")
                        }
                        if step.InputSchema != "" || step.OutputSchema != "" {
                                b.WriteString("\n")
                                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Schemas (reference)"))
                                b.WriteString("\n")
                                if step.InputSchema != "" {
                                        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Input:  " + step.InputSchema))
                                        b.WriteString("\n")
                                }
                                if step.OutputSchema != "" {
                                        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Output: " + step.OutputSchema))
                                        b.WriteString("\n")
                                }
                        }
                }
        }

        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Tip: tab switches focus; scroll the detail pane with ↑/↓, pgup/pgdn."))
        return b.String()
}

func (m Model) renderRunDetail() string {
        if m.selectedRunID == "" {
                return "No run selected"
        }
        r, err := m.store.GetRun(m.st, m.selectedRunID)
        if err != nil {
                return "Run not found"
        }

        total := len(r.Steps)
        done := 0
        for _, s := range r.Steps {
                if isTerminalStatus(s.Status) {
                        done++
                }
        }
        pct := 0.0
        if total > 0 {
                pct = float64(done) / float64(total)
        }

        var b strings.Builder
        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Run details"))
        b.WriteString("\n")
        b.WriteString(fmt.Sprintf("run: %s\n", r.WorkflowID))
        b.WriteString(fmt.Sprintf("flow: %s  v%d\n", r.FlowSlug, r.Version))
        b.WriteString(fmt.Sprintf("status: %s\n", r.Status))
        if r.TriggeredBy != "" {
                b.WriteString(fmt.Sprintf("triggeredBy: %s\n", r.TriggeredBy))
        }
        if !r.StartedAt.IsZero() {
                b.WriteString(fmt.Sprintf("started: %s\n", r.StartedAt.Format(time.RFC3339)))
        }
        if !r.UpdatedAt.IsZero() {
                b.WriteString(fmt.Sprintf("updated: %s\n", r.UpdatedAt.Format(time.RFC3339)))
        }
        if r.CompletedAt != nil {
                b.WriteString(fmt.Sprintf("completed: %s\n", r.CompletedAt.Format(time.RFC3339)))
        }
        if r.CurrentStep != "" {
                b.WriteString(fmt.Sprintf("currentStep: %s\n", r.CurrentStep))
        }
        if r.Error != "" {
                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("error: " + r.Error))
                b.WriteString("\n")
        }

        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Progress"))
        b.WriteString("\n")
        b.WriteString(renderProgressBar(m.detail.Width, pct))
        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("%d/%d steps completed", done, total)))
        b.WriteString("\n")

        // Step preview (selected in the run steps list)
        if it, ok := m.runSteps.SelectedItem().(runStepItem); ok && it.stepID != "" {
                var exec *state.StepExecution
                for i := range r.Steps {
                        if r.Steps[i].StepID == it.stepID {
                                exec = &r.Steps[i]
                                break
                        }
                }
                if exec != nil {
                        b.WriteString("\n")
                        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Selected step"))
                        b.WriteString("\n")
                        b.WriteString(fmt.Sprintf("%s  [%s]\n", exec.StepID, exec.Status))
                        if exec.Title != "" {
                                b.WriteString(exec.Title)
                                b.WriteString("\n")
                        }
                        if exec.Error != "" {
                                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(exec.Error))
                                b.WriteString("\n")
                        }
                        b.WriteString("\n")
                        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Input (preview)"))
                        b.WriteString("\n")
                        b.WriteString(prettyJSON(exec.InputPreview))
                        b.WriteString("\n\n")
                        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Output (preview)"))
                        b.WriteString("\n")
                        b.WriteString(prettyJSON(exec.ResultPreview))
                        b.WriteString("\n")
                }
        }

        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Tip: enter opens a full step view; tab switches focus; scroll the detail pane."))
        return b.String()
}

func isTerminalStatus(status string) bool {
        switch status {
        case "succeeded", "failed", "cancelled", "canceled":
                return true
        default:
                return false
        }
}

func renderProgressBar(width int, pct float64) string {
        if pct < 0 {
                pct = 0
        }
        if pct > 1 {
                pct = 1
        }
        w := width
        if w > 48 {
                w = 48
        }
        if w < 10 {
                w = 10
        }
        inner := w - 2
        filled := int(float64(inner) * pct)
        if filled < 0 {
                filled = 0
        }
        if filled > inner {
                filled = inner
        }
        bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", inner-filled) + "]"
        return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(bar) +
                " " +
                lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("%3.0f%%", pct*100))
}

func (m Model) keysForMode() keyMap {
        k := m.keys
        k.Dashboard.SetEnabled(true)
        k.Settings.SetEnabled(true)
        k.Focus.SetEnabled(m.mode == modeDashboard || m.mode == modeFlow || m.mode == modeRun)

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
