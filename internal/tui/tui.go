package tui

import (
        "encoding/json"
        "fmt"
        "io"
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
        "github.com/charmbracelet/bubbles/table"
        "github.com/charmbracelet/bubbles/viewport"
        tea "github.com/charmbracelet/bubbletea"
        "github.com/charmbracelet/lipgloss"
)

type tickMsg struct{}

type viewMode int

type paneFocus int

type marketTab int

const (
        modeDashboard viewMode = iota
        modeFlows
        modeRuns
        modeFlow
        modeRun
        modeStep
        modeSettings
        modeMarketplace
)

const (
        focusPrimary paneFocus = iota
        focusSecondary
)

const (
        marketRevenue marketTab = iota
        marketDemand
        marketRegistry
        marketPayouts
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

        steps    list.Model
        runSteps list.Model

        flowsTable table.Model
        runsTable  table.Model

        marketTable table.Model
        mTab        marketTab

        detail viewport.Model

        help help.Model
        keys keyMap

        selectedFlowSlug string
        selectedRunID    string
        selectedStepID   string

        stepContextRunID string // when opened from a run

        flowReturnMode viewMode
        runReturnMode  viewMode
}

func Run(workspaceID, statePath string, store mock.Store, st *state.State) error {
        m := NewModel(workspaceID, statePath, store, st)
        p := tea.NewProgram(m, tea.WithAltScreen())
        _, err := p.Run()
        return err
}

func NewModel(workspaceID, statePath string, store mock.Store, st *state.State) Model {
        steps := list.New([]list.Item{}, minimalDelegate{}, 0, 0)
        steps.Title = "Flow"
        steps.SetShowHelp(false)
        steps.SetShowTitle(false)
        steps.SetFilteringEnabled(false)
        steps.SetShowStatusBar(false)
        steps.SetShowPagination(false)
        steps.DisableQuitKeybindings()

        runSteps := list.New([]list.Item{}, minimalDelegate{}, 0, 0)
        runSteps.Title = "Run"
        runSteps.SetShowHelp(false)
        runSteps.SetShowTitle(false)
        runSteps.SetFilteringEnabled(false)
        runSteps.SetShowStatusBar(false)
        runSteps.SetShowPagination(false)
        runSteps.DisableQuitKeybindings()

        flowsTable := table.New(
                table.WithColumns(flowsColumns(60)),
                table.WithRows(nil),
                table.WithFocused(true),
        )
        flowsTable.SetStyles(minimalTableStyles())
        runsTable := table.New(
                table.WithColumns(runsColumns(80)),
                table.WithRows(nil),
                table.WithFocused(true),
        )
        runsTable.SetStyles(minimalTableStyles())
        marketTable := table.New(
                table.WithColumns(revenueColumns(90)),
                table.WithRows(nil),
                table.WithFocused(true),
        )
        marketTable.SetStyles(minimalTableStyles())

        vp := viewport.New(0, 0)
        vp.Style = lipgloss.NewStyle().AlignVertical(lipgloss.Top).Align(lipgloss.Left)

        m := Model{
                workspaceID:    workspaceID,
                statePath:      statePath,
                store:          store,
                st:             st,
                mode:           modeDashboard,
                focus:          focusPrimary,
                steps:          steps,
                runSteps:       runSteps,
                flowsTable:     flowsTable,
                runsTable:      runsTable,
                marketTable:    marketTable,
                mTab:           marketRevenue,
                detail:         vp,
                help:           help.New(),
                keys:           defaultKeyMap(),
                flowReturnMode: modeDashboard,
                runReturnMode:  modeDashboard,
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
                // Columns depend on width; rebuild rows to match current columns.
                m.refreshFromState()
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
                        m.flowReturnMode = modeDashboard
                        m.runReturnMode = modeDashboard
                        m.layout()
                        return m, nil

                case "f":
                        m.mode = modeFlows
                        m.stepContextRunID = ""
                        m.focus = focusPrimary
                        m.layout()
                        m.refreshFromState()
                        return m, nil

                case "r":
                        m.mode = modeRuns
                        m.stepContextRunID = ""
                        m.focus = focusPrimary
                        m.layout()
                        m.refreshFromState()
                        return m, nil

                case "s":
                        m.mode = modeSettings
                        m.stepContextRunID = ""
                        m.focus = focusPrimary
                        m.refreshDetail()
                        m.layout()
                        return m, nil

                case "m":
                        m.mode = modeMarketplace
                        m.stepContextRunID = ""
                        m.focus = focusPrimary
                        m.layout()
                        m.refreshMarket()
                        return m, nil

                case "tab":
                        switch m.mode {
                        case modeFlow, modeRun:
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
                                m.mode = m.runReturnMode
                                m.focus = focusPrimary
                        case modeFlow:
                                m.mode = m.flowReturnMode
                                m.focus = focusPrimary
                        case modeSettings:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        case modeMarketplace:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        case modeFlows:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        case modeRuns:
                                m.mode = modeDashboard
                                m.focus = focusPrimary
                        }
                        m.refreshDetail()
                        m.layout()
                        return m, nil

                case "1":
                        if m.mode == modeMarketplace {
                                m.mTab = marketRevenue
                                m.layout()
                                m.refreshMarket()
                                return m, nil
                        }

                case "2":
                        if m.mode == modeMarketplace {
                                m.mTab = marketDemand
                                m.layout()
                                m.refreshMarket()
                                return m, nil
                        }

                case "3":
                        if m.mode == modeMarketplace {
                                m.mTab = marketRegistry
                                m.layout()
                                m.refreshMarket()
                                return m, nil
                        }

                case "4":
                        if m.mode == modeMarketplace {
                                m.mTab = marketPayouts
                                m.layout()
                                m.refreshMarket()
                                return m, nil
                        }

                case "enter":
                        switch m.mode {
                        case modeFlows:
                                if slug := selectedCell(m.flowsTable.SelectedRow(), 0); slug != "" {
                                        m.openFlow(slug, modeFlows)
                                        return m, nil
                                }

                        case modeRuns:
                                if id := selectedCell(m.runsTable.SelectedRow(), 0); id != "" {
                                        m.openRun(id, modeRuns)
                                        return m, nil
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
                        m.flowsTable, cmd = m.flowsTable.Update(msg)
                } else {
                        m.runsTable, cmd = m.runsTable.Update(msg)
                }
        case modeFlows:
                m.flowsTable, cmd = m.flowsTable.Update(msg)
        case modeRuns:
                m.runsTable, cmd = m.runsTable.Update(msg)
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
        case modeMarketplace:
                m.marketTable, cmd = m.marketTable.Update(msg)
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
        case modeFlows:
                body = m.renderFlowsTableView(bodyH)
        case modeRuns:
                body = m.renderRunsTableView(bodyH)
        case modeFlow:
                body = m.renderFlowView(bodyH)
        case modeRun:
                body = m.renderRunView(bodyH)
        case modeStep:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.detail.View())
        case modeSettings:
                body = panelStyle().Width(m.width).Height(bodyH).Render(m.detail.View())
        case modeMarketplace:
                body = m.renderMarketplaceTableView(bodyH)
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

        // Flow / run: split view (list + scrollable detail viewport)
        leftW := (m.width - 1) / 2
        rightW := (m.width - 1) - leftW // vertical divider already removed
        if leftW < 20 {
                leftW = 20
                rightW = (m.width - 1) - leftW
        }
        if rightW < 20 {
                rightW = 20
                leftW = (m.width - 1) - rightW
        }

        // Split panes are borderless; only padding (1 left + 1 right) => 2 columns.
        leftInnerW := leftW - 2
        if leftInnerW < 10 {
                leftInnerW = 10
        }
        rightInnerW := rightW - 2
        if rightInnerW < 10 {
                rightInnerW = 10
        }

        innerH := bodyH - 2
        if innerH < 3 {
                innerH = 3
        }

        // Reserve some header space in the left pane for stable resource info.
        headerH := 0
        switch m.mode {
        case modeFlow:
                headerH = flowLeftHeaderHeight()
        case modeRun:
                headerH = runLeftHeaderHeight()
        }
        listH := innerH - headerH
        if listH < 3 {
                listH = 3
        }
        m.steps.SetSize(leftInnerW, listH)
        m.runSteps.SetSize(leftInnerW, listH)
        m.detail.Width = rightInnerW
        m.detail.Height = innerH

        // Step / settings: full width scrollable detail in a panel.
        if m.mode == modeStep || m.mode == modeSettings {
                m.detail.Width = m.width - 4
                m.detail.Height = bodyH - 2
        }

        // Full-width table screens (flows / runs / marketplace).
        singleInnerW := m.width - 4
        if singleInnerW < 10 {
                singleInnerW = 10
        }
        singleInnerH := bodyH - 2
        if singleInnerH < 3 {
                singleInnerH = 3
        }
        if m.mode == modeFlows {
                m.flowsTable.SetWidth(singleInnerW)
                m.flowsTable.SetHeight(singleInnerH)
                safeSetColumns(&m.flowsTable, flowsColumns(singleInnerW))
        }
        if m.mode == modeRuns {
                m.runsTable.SetWidth(singleInnerW)
                m.runsTable.SetHeight(singleInnerH)
                safeSetColumns(&m.runsTable, runsColumns(singleInnerW))
        }
        if m.mode == modeMarketplace {
                m.marketTable.SetWidth(singleInnerW)
                m.marketTable.SetHeight(singleInnerH)
                switch m.mTab {
                case marketRevenue:
                        safeSetColumns(&m.marketTable, revenueColumns(singleInnerW))
                case marketDemand:
                        // Demand clusters (preferred) fit the same shape as demandColumns.
                        safeSetColumns(&m.marketTable, demandColumns(singleInnerW))
                case marketRegistry:
                        safeSetColumns(&m.marketTable, registryColumns(singleInnerW))
                case marketPayouts:
                        safeSetColumns(&m.marketTable, payoutsColumns(singleInnerW))
                }
        }

        m.applyFocus()
}

func (m *Model) applyFocus() {
        // Tables are the only components that need explicit focus management.
        m.flowsTable.Blur()
        m.runsTable.Blur()
        m.marketTable.Blur()

        switch m.mode {
        case modeFlows:
                m.flowsTable.Focus()
        case modeRuns:
                m.runsTable.Focus()
        case modeMarketplace:
                m.marketTable.Focus()
        }
}

func flowsColumns(total int) []table.Column {
        // Prefer more columns; drop gracefully on narrow terminals.
        // slug | name | ver | tags | updated | desc
        if total < 10 {
                total = 10
        }

        // Table styles pad cells by 1 on each side => +2 per column.
        // We budget that here to avoid wrapping.
        const padPerCol = 2

        // Decide which optional columns we can afford.
        type col struct {
                title string
                w     int
        }
        base := []col{
                {title: "slug", w: 14},
                {title: "name", w: 0}, // computed
                {title: "ver", w: 4},
                {title: "tags", w: 12},
                {title: "updated", w: 10},
        }
        // Include a description column only when comfortably wide.
        if total >= 120 {
                base = append(base, col{title: "desc", w: 24})
        }

        // Compute name width as remainder and cap it so it doesn't dominate.
        sumFixed := 0
        for _, c := range base {
                if c.title != "name" {
                        sumFixed += c.w
                }
        }
        nameMax := 32
        nameMin := 18

        // Total usable width after accounting for per-column padding.
        usable := total - padPerCol*len(base)
        nameW := usable - sumFixed
        if nameW > nameMax {
                nameW = nameMax
        }
        if nameW < nameMin {
                // Drop optional columns until name fits.
                // Drop desc first, then tags, then updated, then slug (last).
                drop := func(title string) {
                        for i := 0; i < len(base); i++ {
                                if base[i].title == title {
                                        base = append(base[:i], base[i+1:]...)
                                        return
                                }
                        }
                }
                drop("desc")
                drop("tags")
                drop("updated")
                usable = total - padPerCol*len(base)
                sumFixed = 0
                for _, c := range base {
                        if c.title != "name" {
                                sumFixed += c.w
                        }
                }
                nameW = usable - sumFixed
                if nameW < 12 {
                        nameW = maxInt(12, nameW)
                }
        }

        cols := make([]table.Column, 0, len(base))
        for _, c := range base {
                if c.title == "name" {
                        cols = append(cols, table.Column{Title: "name", Width: maxInt(nameMin, nameW)})
                } else {
                        cols = append(cols, table.Column{Title: c.title, Width: maxInt(0, c.w)})
                }
        }
        return cols
}

func runsColumns(total int) []table.Column {
        // Prefer more columns; drop gracefully on narrow terminals.
        // run | flow | status | step | by | started | updated
        if total < 10 {
                total = 10
        }
        runW := 10
        statusW := 10
        stepW := 14
        byW := 12
        startW := 10
        updatedW := 10

        flowW := total - (runW + statusW + stepW + byW + startW + updatedW)

        if flowW < 14 {
                updatedW = 0
                flowW = total - (runW + statusW + stepW + byW + startW)
        }
        if flowW < 14 {
                byW = 0
                flowW = total - (runW + statusW + stepW + startW)
        }
        if flowW < 12 {
                stepW = 0
                flowW = total - (runW + statusW + startW)
        }
        if flowW < 10 {
                startW = 0
                flowW = total - (runW + statusW)
        }

        // Account for cell padding (see minimalTableStyles).
        const padPerCol = 2
        // Compute flow width against usable space.
        colsWanted := 7
        usable := total - padPerCol*colsWanted
        if usable < 0 {
                usable = total
        }
        flowW = usable - (runW + statusW + stepW + byW + startW + updatedW)
        if flowW < 14 {
                updatedW = 0
                colsWanted = 6
                usable = total - padPerCol*colsWanted
                flowW = usable - (runW + statusW + stepW + byW + startW)
        }
        if flowW < 14 {
                byW = 0
                colsWanted = 5
                usable = total - padPerCol*colsWanted
                flowW = usable - (runW + statusW + stepW + startW)
        }
        if flowW < 12 {
                stepW = 0
                colsWanted = 4
                usable = total - padPerCol*colsWanted
                flowW = usable - (runW + statusW + startW)
        }
        if flowW < 10 {
                startW = 0
                colsWanted = 3
                usable = total - padPerCol*colsWanted
                flowW = usable - (runW + statusW)
        }

        cols := []table.Column{
                {Title: "run", Width: runW},
                {Title: "flow", Width: maxInt(10, flowW)},
                {Title: "status", Width: statusW},
        }
        if stepW > 0 {
                cols = append(cols, table.Column{Title: "step", Width: stepW})
        }
        if byW > 0 {
                cols = append(cols, table.Column{Title: "by", Width: byW})
        }
        if startW > 0 {
                cols = append(cols, table.Column{Title: "started", Width: startW})
        }
        if updatedW > 0 {
                cols = append(cols, table.Column{Title: "updated", Width: updatedW})
        }
        return cols
}

func safeSetColumns(t *table.Model, cols []table.Column) {
        // bubbles/table will panic if any existing row has more values than cols
        // during SetColumns (it calls UpdateViewport which calls renderRow).
        //
        // Also, SetRows can panic if rows have more fields than the *current* columns.
        // So: clear rows -> set columns -> set normalized rows.
        n := len(cols)
        rows := t.Rows()

        // 1) Ensure rendering is safe while we change the schema.
        t.SetRows(nil)
        t.SetColumns(cols)

        // 2) Re-add data in the new shape.
        if n <= 0 || len(rows) == 0 {
                return
        }
        fixed := make([]table.Row, 0, len(rows))
        for _, r := range rows {
                if len(r) > n {
                        fixed = append(fixed, r[:n])
                        continue
                }
                if len(r) < n {
                        p := make(table.Row, n)
                        copy(p, r)
                        fixed = append(fixed, p)
                        continue
                }
                fixed = append(fixed, r)
        }
        t.SetRows(fixed)
}

func revenueColumns(total int) []table.Column {
        // when | amount | source | flow | run
        if total < 10 {
                total = 10
        }
        whenW := 10
        amountW := 14
        sourceW := 12
        runW := 10
        flowW := total - whenW - amountW - sourceW - runW - 4
        if flowW < 10 {
                flowW = 10
        }
        return []table.Column{
                {Title: "when", Width: whenW},
                {Title: "amount", Width: amountW},
                {Title: "source", Width: sourceW},
                {Title: "flow", Width: flowW},
                {Title: "run", Width: runW},
        }
}

func registryColumns(total int) []table.Column {
        // listingId | title | price | installs | active | success | rating | revenue
        if total < 10 {
                total = 10
        }
        idW := 20
        priceW := 16
        installsW := 8
        activeW := 6
        successW := 8
        ratingW := 6
        revenueW := 14
        titleW := total - (idW + priceW + installsW + activeW + successW + ratingW + revenueW)
        if titleW < 18 {
                // Drop listingId first by shrinking it.
                idW = 12
                titleW = total - (idW + priceW + installsW + activeW + successW + ratingW + revenueW)
        }
        if titleW < 14 {
                revenueW = 0
                titleW = total - (idW + priceW + installsW + activeW + successW + ratingW)
        }
        if titleW < 12 {
                ratingW = 0
                titleW = total - (idW + priceW + installsW + activeW + successW)
        }
        if titleW < 10 {
                successW = 0
                titleW = total - (idW + priceW + installsW + activeW)
        }
        cols := []table.Column{
                {Title: "listingId", Width: idW},
                {Title: "title", Width: titleW},
                {Title: "price", Width: priceW},
                {Title: "installs", Width: installsW},
                {Title: "active", Width: activeW},
        }
        if successW > 0 {
                cols = append(cols, table.Column{Title: "success", Width: successW})
        }
        if ratingW > 0 {
                cols = append(cols, table.Column{Title: "rating", Width: ratingW})
        }
        if revenueW > 0 {
                cols = append(cols, table.Column{Title: "revenue", Width: revenueW})
        }
        return cols
}

func payoutsColumns(total int) []table.Column {
        // payoutId | period | amount | status | created | paid
        if total < 10 {
                total = 10
        }
        idW := 16
        periodW := 8
        amountW := 14
        statusW := 10
        createdW := 10
        paidW := 10
        if total < 90 {
                paidW = 0
        }
        return []table.Column{
                {Title: "payoutId", Width: idW},
                {Title: "period", Width: periodW},
                {Title: "amount", Width: amountW},
                {Title: "status", Width: statusW},
                {Title: "created", Width: createdW},
                {Title: "paid", Width: paidW},
        }
}

func demandColumns(total int) []table.Column {
        // query | count | window | price | matched
        if total < 10 {
                total = 10
        }
        countW := 6
        windowW := 6
        priceW := 10
        queryW := 24
        matchedW := total - queryW - countW - windowW - priceW - 4
        if matchedW < 10 {
                matchedW = 10
                queryW = maxInt(12, total-matchedW-countW-windowW-priceW-4)
        }
        return []table.Column{
                {Title: "query", Width: queryW},
                {Title: "count", Width: countW},
                {Title: "win", Width: windowW},
                {Title: "price", Width: priceW},
                {Title: "matched", Width: matchedW},
        }
}

func selectedCell(row table.Row, idx int) string {
        if idx < 0 || idx >= len(row) {
                return ""
        }
        return row[idx]
}

func (m *Model) openFlow(slug string, returnMode viewMode) {
        m.selectedFlowSlug = slug
        m.selectedStepID = ""
        m.stepContextRunID = ""
        m.flowReturnMode = returnMode
        m.mode = modeFlow
        m.focus = focusPrimary
        m.refreshSteps()
        m.refreshDetail()
        m.layout()
}

func (m *Model) openRun(id string, returnMode viewMode) {
        m.selectedRunID = id
        m.selectedStepID = ""
        m.stepContextRunID = ""
        m.runReturnMode = returnMode
        m.mode = modeRun
        m.focus = focusPrimary
        m.refreshRunSteps()
        m.refreshDetail()
        m.layout()
}

func maxInt(a, b int) int {
        if a > b {
                return a
        }
        return b
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
        m.refreshMarket()
        m.refreshDetail()
}

func (m *Model) refreshFlows() {
        flows, err := m.store.ListFlows(m.st)
        if err != nil {
                m.err = err
                return
        }
        rows := make([]table.Row, 0, len(flows))
        cols := m.flowsTable.Columns()
        for _, f := range flows {
                tags := "-"
                if len(f.Tags) > 0 {
                        tags = strings.Join(f.Tags, ",")
                }
                desc := f.Description
                if desc == "" {
                        desc = "-"
                }

                rows = append(rows, rowForColumns(cols, map[string]string{
                        "slug":    f.Slug,
                        "name":    f.Name,
                        "ver":     fmt.Sprintf("v%d", f.ActiveVersion),
                        "tags":    tags,
                        "updated": relTime(f.UpdatedAt),
                        "desc":    desc,
                }))
        }
        m.flowsTable.SetRows(rows)
}

func (m *Model) refreshRecentRuns() {
        runs, err := m.store.ListRuns(m.st, "")
        if err != nil {
                m.err = err
                return
        }
        rows := make([]table.Row, 0, len(runs))
        cols := m.runsTable.Columns()
        for _, r := range runs {
                step := r.CurrentStep
                if step == "" {
                        step = "-"
                }
                by := r.TriggeredBy
                if by == "" {
                        by = "-"
                }
                started := relTime(r.StartedAt)
                updated := relTime(r.UpdatedAt)

                rows = append(rows, rowForColumns(cols, map[string]string{
                        "run":     r.WorkflowID,
                        "flow":    r.FlowSlug,
                        "status":  r.Status,
                        "step":    step,
                        "by":      by,
                        "started": started,
                        "updated": updated,
                }))
        }
        m.runsTable.SetRows(rows)
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
        meta := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)

        mid := meta.Render(name)
        if plan != "" {
                mid += meta.Render(" · " + plan)
        }
        if owner != "" {
                mid += meta.Render(" · " + owner)
        }

        right := meta.Render(filepath.Base(m.statePath))
        if m.err != nil {
                right = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("ERROR: " + m.err.Error())
        }
        return lipgloss.JoinHorizontal(lipgloss.Top, left+"  "+mid, padToRight(m.width-lenVisible(left+"  "+mid+"  "+right)), right)
}

func (m Model) renderDashboard(bodyH int) string {
        ws := m.workspace()
        stats := ""
        if ws != nil {
                stats = fmt.Sprintf("Flows: %d  Runs: %d  Updated: %s", len(ws.Flows), len(ws.Runs), ws.UpdatedAt.Format(time.RFC3339))
        }
        meta := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
        statsLine := meta.Render(stats)
        shortcutsLine := meta.Render("f flows · r runs · m marketplace · s settings · q quit")

        return lipgloss.JoinVertical(lipgloss.Left,
                lipgloss.NewStyle().Bold(true).Render("Dashboard"),
                rule(m.width),
                statsLine,
                shortcutsLine,
                "",
                lipgloss.NewStyle().Bold(true).Render("Navigate"),
                "",
                "  f  flows",
                "  r  runs",
                "  m  marketplace",
                "  s  settings",
        )
}

func (m Model) renderFlowsTableView(bodyH int) string {
        title := lipgloss.NewStyle().Bold(true).Render("Flows")
        hint := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("enter open · esc back · g dashboard")
        return lipgloss.JoinVertical(lipgloss.Left, title, rule(m.width), hint, m.flowsTable.View())
}

func (m Model) renderRunsTableView(bodyH int) string {
        title := lipgloss.NewStyle().Bold(true).Render("Runs")
        hint := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("enter open · esc back · g dashboard")
        return lipgloss.JoinVertical(lipgloss.Left, title, rule(m.width), hint, m.runsTable.View())
}

func (m Model) renderFlowView(bodyH int) string {
        leftW := (m.width - 1) / 2
        rightW := (m.width - 1) - leftW

        left := splitPaneStyle().Width(leftW).Height(bodyH).Render(m.renderFlowLeftPane())
        divider := vRule(bodyH)
        right := splitPaneStyle().Width(rightW).Height(bodyH).Render(m.detail.View())
        return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func (m Model) renderRunView(bodyH int) string {
        leftW := (m.width - 1) / 2
        rightW := (m.width - 1) - leftW

        left := splitPaneStyle().Width(leftW).Height(bodyH).Render(m.renderRunLeftPane())
        divider := vRule(bodyH)
        right := splitPaneStyle().Width(rightW).Height(bodyH).Render(m.detail.View())
        return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
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
                "- f: flows",
                "- r: runs",
                "- s: settings",
                "- m: marketplace",
        }
        return joinLines(lines)
}

func (m Model) renderFlowLeftPane() string {
        f, err := m.store.GetFlow(m.st, m.selectedFlowSlug)
        if err != nil || f == nil {
                return "Flow not found"
        }
        w := m.steps.Width()
        if w <= 0 {
                w = 40
        }
        meta := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)

        tags := "-"
        if len(f.Tags) > 0 {
                tags = strings.Join(f.Tags, ",")
        }
        spine := fmt.Sprintf("%d items", len(f.Spine))
        if len(f.Spine) > 0 {
                // Show a short preview, not the whole thing.
                n := 2
                if len(f.Spine) < n {
                        n = len(f.Spine)
                }
                spine = strings.Join(f.Spine[:n], " · ")
                if len(f.Spine) > n {
                        spine += fmt.Sprintf(" · +%d", len(f.Spine)-n)
                }
        }

        headerLines := []string{
                lipgloss.NewStyle().Bold(true).Render(f.Name),
                meta.Render(f.Slug + " · v" + fmt.Sprintf("%d", f.ActiveVersion)),
                meta.Render("tags " + tags),
                meta.Render("updated " + relTime(f.UpdatedAt)),
                meta.Render("spine " + spine),
                "",
                lipgloss.NewStyle().Bold(true).Render("Steps"),
        }
        for i := range headerLines {
                // Keep the header stable: avoid wrapping by truncating to pane width.
                headerLines[i] = truncateRunes(headerLines[i], w)
        }
        header := joinLines(headerLines)
        return lipgloss.JoinVertical(lipgloss.Left, header, m.steps.View())
}

func (m Model) renderRunLeftPane() string {
        r, err := m.store.GetRun(m.st, m.selectedRunID)
        if err != nil || r == nil {
                return "Run not found"
        }
        w := m.runSteps.Width()
        if w <= 0 {
                w = 40
        }
        meta := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)

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

        by := r.TriggeredBy
        if by == "" {
                by = "-"
        }

        barW := 34
        // Keep the progress line from overflowing the pane.
        if w-10 < barW {
                barW = maxInt(10, w-10)
        }

        headerLines := []string{
                lipgloss.NewStyle().Bold(true).Render("Run " + r.WorkflowID),
                meta.Render(r.FlowSlug + " · v" + fmt.Sprintf("%d", r.Version)),
                meta.Render("status " + r.Status),
                meta.Render("by " + by),
                meta.Render("started " + relTime(r.StartedAt) + " · updated " + relTime(r.UpdatedAt)),
                meta.Render(renderProgressBar(barW, pct) + " " + fmt.Sprintf("%d/%d", done, total)),
                "",
                lipgloss.NewStyle().Bold(true).Render("Steps"),
        }
        for i := range headerLines {
                headerLines[i] = truncateRunes(headerLines[i], w)
        }
        header := joinLines(headerLines)
        return lipgloss.JoinVertical(lipgloss.Left, header, m.runSteps.View())
}

func (m *Model) refreshDetail() {
        switch m.mode {
        case modeFlow:
                m.detail.SetContent(m.renderFlowFocusedStepDetail())
        case modeRun:
                m.detail.SetContent(m.renderRunFocusedStepDetail())
        case modeStep:
                m.detail.SetContent(m.renderStep())
        case modeSettings:
                m.detail.SetContent(m.renderSettings())
        }
}

func (m Model) renderFlowFocusedStepDetail() string {
        if m.selectedFlowSlug == "" {
                return "No flow selected"
        }
        f, err := m.store.GetFlow(m.st, m.selectedFlowSlug)
        if err != nil {
                return "Flow not found"
        }

        it, ok := m.steps.SelectedItem().(stepItem)
        if !ok || it.id == "" {
                return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("Select a step on the left.")
        }

        var step *state.FlowStep
        for i := range f.Steps {
                if f.Steps[i].ID == it.id {
                        step = &f.Steps[i]
                        break
                }
        }
        if step == nil {
                return "Step not found"
        }

        // Try to surface concrete IO from the latest run for this flow.
        var input any
        var output any
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

        var b strings.Builder
        b.WriteString(lipgloss.NewStyle().Bold(true).Render(step.ID))
        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(step.Type))
        if step.Title != "" {
                b.WriteString(" · ")
                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(step.Title))
        }
        b.WriteString("\n\n")

        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Definition"))
        b.WriteString("\n")
        b.WriteString(step.Definition)
        b.WriteString("\n\n")

        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Input (concrete)"))
        b.WriteString("\n")
        b.WriteString(prettyJSON(input))
        b.WriteString("\n\n")

        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Output (concrete)"))
        b.WriteString("\n")
        b.WriteString(prettyJSON(output))
        b.WriteString("\n\n")

        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("Schemas"))
        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("in  " + step.InputSchema))
        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("out " + step.OutputSchema))
        return b.String()
}

func (m Model) renderRunFocusedStepDetail() string {
        if m.selectedRunID == "" {
                return "No run selected"
        }
        r, err := m.store.GetRun(m.st, m.selectedRunID)
        if err != nil {
                return "Run not found"
        }

        it, ok := m.runSteps.SelectedItem().(runStepItem)
        if !ok || it.stepID == "" {
                return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("Select a step on the left.")
        }

        var exec *state.StepExecution
        for i := range r.Steps {
                if r.Steps[i].StepID == it.stepID {
                        exec = &r.Steps[i]
                        break
                }
        }
        if exec == nil {
                return "Step not found"
        }

        var b strings.Builder
        b.WriteString(lipgloss.NewStyle().Bold(true).Render(exec.StepID))
        b.WriteString("\n")
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(exec.Status))
        if exec.Title != "" {
                b.WriteString(" · ")
                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(exec.Title))
        }
        b.WriteString("\n\n")

        if exec.Error != "" {
                b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render(exec.Error))
                b.WriteString("\n\n")
        }

        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Input"))
        b.WriteString("\n")
        b.WriteString(prettyJSON(exec.InputPreview))
        b.WriteString("\n\n")

        b.WriteString(lipgloss.NewStyle().Bold(true).Render("Output"))
        b.WriteString("\n")
        b.WriteString(prettyJSON(exec.ResultPreview))
        return b.String()
}

func flowLeftHeaderHeight() int { return 7 }
func runLeftHeaderHeight() int  { return 8 }

func (m *Model) refreshMarket() {
        ws := m.workspace()
        if ws == nil {
                m.marketTable.SetRows(nil)
                return
        }

        switch m.mTab {
        case marketRevenue:
                events := make([]state.RevenueEvent, len(ws.RevenueEvents))
                copy(events, ws.RevenueEvents)
                sort.Slice(events, func(i, j int) bool { return events[i].At.After(events[j].At) })
                rows := make([]table.Row, 0, len(events))
                cols := m.marketTable.Columns()
                for _, e := range events {
                        src := e.Source
                        if src == "" {
                                src = "-"
                        }
                        flow := e.FlowSlug
                        if flow == "" {
                                flow = "-"
                        }
                        runID := e.RunID
                        if runID == "" {
                                runID = "-"
                        }
                        rows = append(rows, rowForColumns(cols, map[string]string{
                                "when":   relTime(e.At),
                                "amount": fmt.Sprintf("%s %.2f", e.Currency, float64(e.AmountCents)/100.0),
                                "source": src,
                                "flow":   flow,
                                "run":    runID,
                        }))
                }
                m.marketTable.SetRows(rows)

        case marketDemand:
                // Prefer clustered demand in v1.
                clusters := make([]state.DemandCluster, 0, len(ws.DemandClusters))
                clusters = append(clusters, ws.DemandClusters...)
                sort.Slice(clusters, func(i, j int) bool { return clusters[i].Count > clusters[j].Count })
                rows := make([]table.Row, 0, len(clusters))
                cols := m.marketTable.Columns()
                for _, c := range clusters {
                        price := c.SuggestedPrice
                        if price == "" {
                                price = "-"
                        }
                        matched := "-"
                        if len(c.MatchedListings) > 0 {
                                matched = strings.Join(c.MatchedListings, ", ")
                        }
                        rows = append(rows, rowForColumns(cols, map[string]string{
                                "query":   c.Title,
                                "count":   fmt.Sprintf("%d", c.Count),
                                "win":     c.Window,
                                "window":  c.Window,
                                "price":   price,
                                "matched": matched,
                        }))
                }
                m.marketTable.SetRows(rows)

        case marketRegistry:
                entries := make([]*state.RegistryEntry, 0, len(ws.Registry))
                for _, e := range ws.Registry {
                        entries = append(entries, e)
                }
                sort.Slice(entries, func(i, j int) bool { return entries[i].Stats.RevenueCents > entries[j].Stats.RevenueCents })
                rows := make([]table.Row, 0, len(entries))
                cols := m.marketTable.Columns()
                for _, e := range entries {
                        price := fmt.Sprintf("%s %0.2f", e.Pricing.Currency, float64(e.Pricing.AmountCents)/100.0)
                        if e.Pricing.Model == "subscription" && e.Pricing.Interval != "" {
                                price = fmt.Sprintf("%s %0.2f/%s", e.Pricing.Currency, float64(e.Pricing.AmountCents)/100.0, e.Pricing.Interval)
                        } else if e.Pricing.Model == "per_success" {
                                price = fmt.Sprintf("%s %0.2f/success", e.Pricing.Currency, float64(e.Pricing.AmountCents)/100.0)
                        } else if e.Pricing.Model == "per_run" {
                                price = fmt.Sprintf("%s %0.2f/run", e.Pricing.Currency, float64(e.Pricing.AmountCents)/100.0)
                        }
                        rows = append(rows, rowForColumns(cols, map[string]string{
                                "listingId": e.ID,
                                "slug":      e.Slug,
                                "title":     e.Title,
                                "price":     price,
                                "installs":  fmt.Sprintf("%d", e.Stats.Installs),
                                "active":    fmt.Sprintf("%d", e.Stats.Active),
                                "success":   fmt.Sprintf("%0.0f%%", e.Stats.SuccessRate*100),
                                "rating":    fmt.Sprintf("%0.1f", e.Stats.Rating),
                                "revenue":   fmt.Sprintf("%s %0.2f", e.Pricing.Currency, float64(e.Stats.RevenueCents)/100.0),
                        }))
                }
                m.marketTable.SetRows(rows)

        case marketPayouts:
                items := make([]*state.Payout, 0, len(ws.Payouts))
                for _, p := range ws.Payouts {
                        items = append(items, p)
                }
                sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
                rows := make([]table.Row, 0, len(items))
                cols := m.marketTable.Columns()
                for _, p := range items {
                        paid := "-"
                        if p.PaidAt != nil {
                                paid = relTime(*p.PaidAt)
                        }
                        rows = append(rows, rowForColumns(cols, map[string]string{
                                "payoutId": p.ID,
                                "period":   p.Period,
                                "amount":   fmt.Sprintf("%s %0.2f", p.Currency, float64(p.AmountCents)/100.0),
                                "status":   p.Status,
                                "created":  relTime(p.CreatedAt),
                                "paid":     paid,
                        }))
                }
                m.marketTable.SetRows(rows)
        }
}

func rowForColumns(cols []table.Column, values map[string]string) table.Row {
        r := make(table.Row, len(cols))
        for i, c := range cols {
                if v, ok := values[c.Title]; ok {
                        r[i] = v
                } else {
                        r[i] = ""
                }
        }
        return r
}

func (m Model) renderMarketplaceTableView(bodyH int) string {
        title := "Marketplace"
        switch m.mTab {
        case marketRevenue:
                title += " · Revenue"
        case marketDemand:
                title += " · Demand"
        case marketRegistry:
                title += " · Registry"
        case marketPayouts:
                title += " · Payouts"
        }
        header := lipgloss.NewStyle().Bold(true).Render(title)
        hint := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render("1 revenue · 2 demand · 3 registry · 4 payouts · esc back · g dashboard")
        return lipgloss.JoinVertical(lipgloss.Left, header, rule(m.width), hint, m.marketTable.View())
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
        bar := "[" + strings.Repeat("=", filled) + strings.Repeat(".", inner-filled) + "]"
        return lipgloss.NewStyle().Render(bar) + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(fmt.Sprintf("%3.0f%%", pct*100))
}

func (m Model) keysForMode() keyMap {
        k := m.keys
        k.Dashboard.SetEnabled(true)
        k.Flows.SetEnabled(true)
        k.Runs.SetEnabled(true)
        k.Settings.SetEnabled(true)
        k.Marketplace.SetEnabled(true)
        k.Focus.SetEnabled(m.mode == modeDashboard || m.mode == modeFlow || m.mode == modeRun)
        k.RevenueTab.SetEnabled(m.mode == modeMarketplace)
        k.DemandTab.SetEnabled(m.mode == modeMarketplace)
        k.RegistryTab.SetEnabled(m.mode == modeMarketplace)
        k.PayoutsTab.SetEnabled(m.mode == modeMarketplace)

        switch m.mode {
        case modeDashboard:
                k.Enter.SetHelp("enter", "open")
                k.Back.SetHelp("esc", "")
        case modeFlows:
                k.Enter.SetHelp("enter", "open flow")
                k.Back.SetHelp("esc", "back")
        case modeRuns:
                k.Enter.SetHelp("enter", "open run")
                k.Back.SetHelp("esc", "back")
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
        case modeMarketplace:
                k.Enter.SetHelp("enter", "")
                k.Back.SetHelp("esc", "back")
                k.Focus.SetEnabled(false)
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
                Border(lipgloss.HiddenBorder()).
                Padding(0, 1).
                AlignVertical(lipgloss.Top).
                Align(lipgloss.Left)
}

func paneStyle(active bool) lipgloss.Style {
        if active {
                return lipgloss.NewStyle().
                        Border(lipgloss.HiddenBorder()).
                        Padding(0, 1).
                        AlignVertical(lipgloss.Top).
                        Align(lipgloss.Left)
        }
        return lipgloss.NewStyle().
                Border(lipgloss.HiddenBorder()).
                Padding(0, 1).
                AlignVertical(lipgloss.Top).
                Align(lipgloss.Left)
}

func footerStyle() lipgloss.Style {
        return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
}

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

func sparkBar(width, value, max int) string {
        if max <= 0 {
                max = 1
        }
        if value < 0 {
                value = 0
        }
        if width < 3 {
                width = 3
        }
        filled := int(float64(width) * float64(value) / float64(max))
        if filled < 0 {
                filled = 0
        }
        if filled > width {
                filled = width
        }
        return strings.Repeat("=", filled) + strings.Repeat(".", width-filled)
}

func rule(width int) string {
        if width <= 0 {
                width = 10
        }
        return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(strings.Repeat("-", width))
}

func splitPaneStyle() lipgloss.Style {
        // Borderless, whitespace-driven. Keep a small horizontal padding.
        return lipgloss.NewStyle().Padding(0, 1).AlignVertical(lipgloss.Top).Align(lipgloss.Left)
}

func vRule(height int) string {
        if height <= 0 {
                height = 1
        }
        var b strings.Builder
        for i := 0; i < height; i++ {
                if i > 0 {
                        b.WriteByte('\n')
                }
                b.WriteByte('|')
        }
        return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Render(b.String())
}

// --- Help / keymap -----------------------------------------------------------

type keyMap struct {
        Up          key.Binding
        Down        key.Binding
        Enter       key.Binding
        Back        key.Binding
        Help        key.Binding
        Quit        key.Binding
        Dashboard   key.Binding
        Flows       key.Binding
        Runs        key.Binding
        Settings    key.Binding
        Marketplace key.Binding
        RevenueTab  key.Binding
        DemandTab   key.Binding
        RegistryTab key.Binding
        PayoutsTab  key.Binding
        Focus       key.Binding
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
                Flows: key.NewBinding(
                        key.WithKeys("f"),
                        key.WithHelp("f", "flows"),
                ),
                Runs: key.NewBinding(
                        key.WithKeys("r"),
                        key.WithHelp("r", "runs"),
                ),
                Settings: key.NewBinding(
                        key.WithKeys("s"),
                        key.WithHelp("s", "settings"),
                ),
                Marketplace: key.NewBinding(
                        key.WithKeys("m"),
                        key.WithHelp("m", "marketplace"),
                ),
                RevenueTab: key.NewBinding(
                        key.WithKeys("1"),
                        key.WithHelp("1", "revenue"),
                ),
                DemandTab: key.NewBinding(
                        key.WithKeys("2"),
                        key.WithHelp("2", "demand"),
                ),
                RegistryTab: key.NewBinding(
                        key.WithKeys("3"),
                        key.WithHelp("3", "registry"),
                ),
                PayoutsTab: key.NewBinding(
                        key.WithKeys("4"),
                        key.WithHelp("4", "payouts"),
                ),
                Focus: key.NewBinding(
                        key.WithKeys("tab"),
                        key.WithHelp("tab", "switch pane"),
                ),
        }
}

func (k keyMap) ShortHelp() []key.Binding {
        return []key.Binding{k.Up, k.Down, k.Enter, k.Back, k.Focus, k.Dashboard, k.Flows, k.Runs, k.Settings, k.Marketplace, k.RevenueTab, k.DemandTab, k.RegistryTab, k.PayoutsTab, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
        return [][]key.Binding{
                {k.Up, k.Down, k.Enter, k.Back},
                {k.Focus, k.Dashboard, k.Flows, k.Runs},
                {k.Settings, k.Marketplace, k.RevenueTab, k.DemandTab, k.RegistryTab, k.PayoutsTab},
                {k.Help, k.Quit},
        }
}

// Ensure we implement bubbles/help KeyMap interface.
var _ help.KeyMap = keyMap{}

// --- Sorting helpers (not used yet, reserved for future) --------------------

func sortRunsByRecent(runs []*state.Run) {
        sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
}

// --- Minimal styling helpers (text-first) -----------------------------------

func minimalTableStyles() table.Styles {
        s := table.DefaultStyles()
        // Plain header/cells, but keep a tiny bit of horizontal breathing room.
        // (table columns are "tight" otherwise and feel cramped.)
        s.Header = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true).Padding(0, 1)
        s.Cell = lipgloss.NewStyle().Padding(0, 1)
        // Selected row: typographic emphasis rather than color blocks.
        s.Selected = lipgloss.NewStyle().Bold(true).Underline(true)
        return s
}

type minimalDelegate struct{}

func (d minimalDelegate) Height() int                               { return 1 }
func (d minimalDelegate) Spacing() int                              { return 0 }
func (d minimalDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d minimalDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
        selected := index == m.Index()
        prefix := "  "
        if selected {
                prefix = "> "
        }

        title := item.FilterValue()
        desc := ""
        if di, ok := item.(list.DefaultItem); ok {
                title = di.Title()
                desc = di.Description()
        }
        titleRaw := prefix + title
        descRaw := desc

        maxW := m.Width()
        if maxW <= 0 {
                maxW = 80
        }

        // Truncate title first, then fit description if there’s room.
        titleRunes := []rune(titleRaw)
        if len(titleRunes) > maxW {
                titleRaw = truncateRunes(titleRaw, maxW)
                descRaw = ""
        }

        sep := " — "
        remain := maxW - len([]rune(titleRaw))
        if descRaw != "" && remain > len([]rune(sep))+1 {
                descRaw = truncateRunes(descRaw, remain-len([]rune(sep)))
        } else {
                descRaw = ""
        }

        titleStyle := lipgloss.NewStyle()
        if selected {
                titleStyle = titleStyle.Bold(true)
        }
        descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)

        out := titleStyle.Render(titleRaw)
        if descRaw != "" {
                out += descStyle.Render(sep + descRaw)
        }
        _, _ = io.WriteString(w, out)
}

func truncateRunes(s string, max int) string {
        if max <= 0 {
                return ""
        }
        r := []rune(s)
        if len(r) <= max {
                return s
        }
        if max <= 1 {
                return "…"
        }
        return string(r[:max-1]) + "…"
}
