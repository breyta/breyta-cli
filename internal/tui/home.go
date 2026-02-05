package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/breyta/breyta-cli/internal/api"
	"github.com/breyta/breyta-cli/internal/authinfo"
	"github.com/breyta/breyta-cli/internal/authstore"
	"github.com/breyta/breyta-cli/internal/buildinfo"
	"github.com/breyta/breyta-cli/internal/configstore"
	"github.com/breyta/breyta-cli/internal/updatecheck"
	"github.com/breyta/breyta-cli/skills"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HomeConfig struct {
	APIURL      string
	Token       string
	WorkspaceID string
	StatePath   string
}

type homeMode int

const (
	homeModeOptions homeMode = iota
	homeModeWorkspace
)

type homeFocus int

const (
	homeFocusFlows homeFocus = iota
	homeFocusRuns
)

type optionID string

const (
	optAuth        optionID = "auth"
	optDiagnostics optionID = "diagnostics"
)

type optionItem struct {
	id   optionID
	name string
	desc string
}

func (i optionItem) Title() string       { return i.name }
func (i optionItem) Description() string { return i.desc }
func (i optionItem) FilterValue() string { return i.name + " " + i.desc }

type separatorItem struct {
	title string
}

func (i separatorItem) Title() string       { return i.title }
func (i separatorItem) Description() string { return "" }
func (i separatorItem) FilterValue() string { return "" }

type workspaceItem struct {
	id   string
	name string
	desc string
}

func (i workspaceItem) Title() string       { return i.name }
func (i workspaceItem) Description() string { return i.desc }
func (i workspaceItem) FilterValue() string { return i.name + " " + i.desc }

type modalKind int

const (
	modalAuth modalKind = iota
	modalDiagnostics
	modalAgentSkills
	modalUpdate
)

type modalResult int

const (
	modalResultNone modalResult = iota
	modalResultSaved
	modalResultCanceled
)

type modalModel struct {
	kind  modalKind
	title string

	list   list.Model
	status string
	err    error

	result modalResult
}

type modalItem struct {
	id   string
	name string
	desc string
}

func (i modalItem) Title() string       { return i.name }
func (i modalItem) Description() string { return i.desc }
func (i modalItem) FilterValue() string { return i.name + " " + i.desc }

func newModalList(title string, filter bool) list.Model {
	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.Styles = breytaDefaultItemStyles()
	l := list.New([]list.Item{}, d, 0, 0)
	l.Title = title
	// Title is rendered by the modal frame, not the embedded list.
	l.SetShowTitle(false)
	l.Styles = breytaListStyles()
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(filter)
	// ESC should cancel/close modal, not quit.
	l.KeyMap.Quit.SetKeys("q")
	return l
}

func (m modalModel) View(w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}

	boxW := minInt(90, maxInt(40, w-6))
	boxH := minInt(22, maxInt(10, h-6))

	title := lipgloss.NewStyle().Bold(true).Foreground(breytaTextColor).Render(m.title)

	innerW := boxW - 4
	innerH := boxH - 4 // title + status
	if innerH < 3 {
		innerH = 3
	}

	m.list.SetSize(innerW, innerH)

	status := ""
	if strings.TrimSpace(m.status) != "" {
		status = lipgloss.NewStyle().Foreground(breytaMuted).Render(m.status)
	}
	if m.err != nil {
		status = lipgloss.NewStyle().Foreground(breytaDanger).Bold(true).Render(m.err.Error())
	}

	body := strings.Join([]string{title, "", m.list.View(), "", status}, "\n")

	panel := lipgloss.NewStyle().
		Width(boxW).
		Height(boxH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(breytaBorder).
		Padding(1, 1).
		Render(body)

	overlay := lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, panel)
	return overlay
}

func (m *modalModel) handleKey(msg tea.KeyMsg) (modalResult, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace", "ctrl+g":
		m.result = modalResultCanceled
		return m.result, nil
	case "enter":
		m.result = modalResultSaved
		return m.result, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(translateNavKeys(msg))
	return modalResultNone, cmd
}

type homeWorkspaceMeta struct {
	ID        string
	Name      string
	Plan      string
	Owner     string
	UpdatedAt time.Time
}

type homeDiagMsg struct {
	connected  bool
	httpStatus int
	apiError   string
	info       string
	userEmail  string
	wsCount    int
}

type homeWorkspacesMsg struct {
	workspaces []meWorkspace
	httpStatus int
	err        error
	userEmail  string
	wsCount    int
}

type homeUpdateMsg struct {
	notice *updatecheck.Notice
	err    error
}

type homeLoginStartMsg struct {
	apiURL  string
	authURL string
	openErr error
	status  string
	err     error
	session *browserLoginSession
}

type homeLoginMsg struct {
	token  string
	status string
	err    error
}

type homeModel struct {
	cfg HomeConfig

	mode homeMode

	width  int
	height int

	// persisted/config state
	apiURL     string
	token      string
	defaultWS  string
	connected  bool
	httpStatus int
	apiError   string
	lastInfo   string
	userEmail  string

	// workspace listing
	workspaces    []meWorkspace
	workspacesErr error

	// main UI
	options list.Model

	// modal
	modal *modalModel
	// update prompt
	updateNotice *updatecheck.Notice
	doUpgrade    bool

	// workspace view
	selectedWorkspaceID string
	workspaceMeta       *homeWorkspaceMeta
	flowsTable          table.Model
	runsTable           table.Model
	workspaceFocus      homeFocus
}

func RunHome(cfg HomeConfig) error {
	m := newHomeModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	final, err := p.Run()
	if err != nil {
		return err
	}
	hm, ok := final.(homeModel)
	if !ok {
		return nil
	}
	if hm.doUpgrade && hm.updateNotice != nil {
		return runUpgradeAndReexec(hm.updateNotice)
	}
	return nil
}

func newHomeModel(cfg HomeConfig) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles = breytaDefaultItemStyles()

	options := list.New([]list.Item{}, delegate, 0, 0)
	options.Title = "Workspaces"
	options.SetFilteringEnabled(true)
	options.Styles = breytaListStyles()
	options.SetShowHelp(false)
	options.SetShowStatusBar(false)
	options.SetShowPagination(false)
	options.KeyMap.Quit.SetKeys("q")
	// Add emacs-style nav like Clarity.
	up := append([]string{}, options.KeyMap.CursorUp.Keys()...)
	up = append(up, "ctrl+p")
	options.KeyMap.CursorUp.SetKeys(up...)
	down := append([]string{}, options.KeyMap.CursorDown.Keys()...)
	down = append(down, "ctrl+n")
	options.KeyMap.CursorDown.SetKeys(down...)

	flowsTable := table.New(table.WithColumns(flowsColumns(80)), table.WithRows(nil), table.WithFocused(true))
	flowsTable.SetStyles(minimalTableStyles())
	runsTable := table.New(table.WithColumns(runsColumns(80)), table.WithRows(nil), table.WithFocused(false))
	runsTable.SetStyles(minimalTableStyles())

	m := homeModel{
		cfg:            cfg,
		mode:           homeModeOptions,
		options:        options,
		flowsTable:     flowsTable,
		runsTable:      runsTable,
		workspaceFocus: homeFocusFlows,
	}

	m.apiURL, m.defaultWS = m.loadConfig()
	m.token = m.resolveToken()
	m.refreshOptions()
	if strings.TrimSpace(os.Getenv(skipTUIUpdatePromptEnv)) == "" {
		if n := updatecheck.CachedNotice(buildinfo.DisplayVersion()); n != nil && n.Available {
			m.updateNotice = n
			m.modal = m.newUpdateModal(n)
		}
	}
	return m
}

func (m homeModel) Init() tea.Cmd {
	if strings.TrimSpace(os.Getenv(skipTUIUpdatePromptEnv)) != "" {
		return m.refreshTokenCmd()
	}
	return tea.Batch(m.refreshTokenCmd(), m.checkUpdateCmd())
}

func (m homeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case homeDiagMsg:
		m.connected = msg.connected
		m.httpStatus = msg.httpStatus
		m.apiError = msg.apiError
		if strings.TrimSpace(msg.info) != "" {
			m.lastInfo = strings.TrimSpace(msg.info)
		}
		if strings.TrimSpace(msg.userEmail) != "" {
			m.userEmail = strings.TrimSpace(msg.userEmail)
		}
		m.refreshOptions()
		return m, nil

	case homeWorkspacesMsg:
		m.workspacesErr = msg.err
		m.workspaces = msg.workspaces
		if strings.TrimSpace(msg.userEmail) != "" {
			m.userEmail = strings.TrimSpace(msg.userEmail)
		}
		// Never keep showing / using a stale default workspace id (e.g. from mock/local).
		if msg.err == nil && strings.TrimSpace(m.defaultWS) != "" && len(msg.workspaces) > 0 {
			found := false
			for _, ws := range msg.workspaces {
				if strings.TrimSpace(ws.ID) == strings.TrimSpace(m.defaultWS) {
					found = true
					break
				}
			}
			if !found {
				m.defaultWS = ""
				_ = setConfig(m.apiURL, "")
			}
		}
		// If there's only one workspace, always show it as default and auto-enter it.
		if msg.err == nil && len(msg.workspaces) == 1 {
			if id := strings.TrimSpace(msg.workspaces[0].ID); id != "" {
				if strings.TrimSpace(m.defaultWS) == "" {
					m.defaultWS = id
					_ = setConfig(m.apiURL, id)
				}
				if strings.TrimSpace(m.selectedWorkspaceID) == "" {
					m.selectedWorkspaceID = id
					m.mode = homeModeWorkspace
					m.applyWorkspaceFocus()
					m.refreshOptions()
					m.layout()
					return m, m.loadWorkspaceCmd(id)
				}
			}
		}
		// In prod, always auto-pick a real default workspace once authenticated.
		if msg.err == nil && strings.TrimSpace(m.token) != "" && m.connected && isProdAPIURL(m.apiURL) {
			if strings.TrimSpace(m.defaultWS) == "" && len(msg.workspaces) > 0 {
				if id := strings.TrimSpace(msg.workspaces[0].ID); id != "" && id != "ws-acme" {
					m.defaultWS = id
					_ = setConfig(m.apiURL, id)
				}
			}
		}
		m.refreshOptions()
		return m, nil

	case homeUpdateMsg:
		if msg.err == nil && msg.notice != nil && msg.notice.Available {
			m.updateNotice = msg.notice
			if m.modal == nil {
				m.modal = m.newUpdateModal(msg.notice)
			}
		}
		return m, nil

	case homeLoginStartMsg:
		if msg.err != nil {
			m.apiError = msg.err.Error()
			if strings.TrimSpace(msg.status) != "" {
				m.lastInfo = strings.TrimSpace(msg.status)
			}
			m.refreshOptions()
			return m, nil
		}

		m.apiError = ""
		m.lastInfo = "login url: " + strings.TrimSpace(msg.authURL)
		if msg.openErr != nil {
			m.apiError = "could not open browser automatically"
			if strings.TrimSpace(msg.openErr.Error()) != "" {
				m.apiError = m.apiError + ": " + msg.openErr.Error()
			}
		}
		m.refreshOptions()

		sess := msg.session
		apiURL := msg.apiURL
		return m, func() tea.Msg {
			res, err := sess.Wait(context.Background())
			if err != nil {
				return homeLoginMsg{err: err, status: "login failed"}
			}
			tok := strings.TrimSpace(res.Token)
			if tok == "" {
				return homeLoginMsg{err: errors.New("missing token"), status: "login failed"}
			}
			if err := storeAuthRecord(apiURL, tok, res.RefreshToken, res.ExpiresIn); err != nil {
				return homeLoginMsg{err: err, status: "login failed"}
			}
			return homeLoginMsg{token: tok, status: "logged in"}
		}

	case homeLoginMsg:
		if msg.err != nil {
			m.apiError = msg.err.Error()
			if strings.TrimSpace(msg.status) != "" {
				m.lastInfo = strings.TrimSpace(msg.status)
			}
			m.refreshOptions()
			return m, nil
		}
		m.token = msg.token
		m.connected = false
		m.httpStatus = 0
		m.apiError = ""
		if strings.TrimSpace(msg.status) != "" {
			m.lastInfo = strings.TrimSpace(msg.status)
		}
		m.refreshOptions()
		return m, tea.Batch(m.checkConnectionCmd(), m.fetchWorkspacesCmd())

	case homeTokenRefreshedMsg:
		if strings.TrimSpace(msg.token) != "" {
			m.token = strings.TrimSpace(msg.token)
		}
		if msg.err != nil && strings.TrimSpace(m.token) == "" {
			m.apiError = msg.err.Error()
		}
		if msg.refreshed {
			m.lastInfo = "refreshed token"
		}
		m.refreshOptions()
		return m, tea.Batch(m.checkConnectionCmd(), m.fetchWorkspacesCmd())

	case homeWorkspaceLoadedMsg:
		if msg.err != nil {
			m.apiError = msg.err.Error()
			m.selectedWorkspaceID = ""
			m.workspaceMeta = nil
			m.flowsTable.SetRows(nil)
			m.runsTable.SetRows(nil)
			m.mode = homeModeOptions
			m.refreshOptions()
			m.layout()
			return m, nil
		}
		m.apiError = ""
		m.httpStatus = 0
		m.connected = true
		m.refreshOptions()
		return m.applyWorkspaceLoaded(msg), nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

		// Modal takes precedence.
		if m.modal != nil {
			res, cmd := m.modal.handleKey(msg)
			if res == modalResultCanceled {
				m.modal = nil
				return m, nil
			}
			if res == modalResultSaved {
				cmd2 := m.applyModal()
				m.modal = nil
				return m, tea.Batch(cmd, cmd2)
			}
			return m, cmd
		}

		switch m.mode {
		case homeModeOptions:
			return m.updateOptionsMode(msg)
		case homeModeWorkspace:
			return m.updateWorkspaceMode(msg)
		}
	}
	return m, nil
}

func (m homeModel) updateOptionsMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.options, cmd = m.options.Update(translateNavKeys(msg))

	switch msg.String() {
	case "a":
		m.modal = m.newAuthModal()
		return m, cmd
	case "x":
		m.modal = m.newDiagnosticsModal()
		return m, cmd
	case "s":
		m.modal = m.newAgentSkillsModal()
		return m, cmd
	case "r":
		return m, tea.Batch(cmd, m.checkConnectionCmd(), m.fetchWorkspacesCmd())
	case "enter":
		if it, ok := m.options.SelectedItem().(workspaceItem); ok && strings.TrimSpace(it.id) != "" {
			m.selectedWorkspaceID = it.id
			if strings.TrimSpace(m.defaultWS) == "" {
				if id := strings.TrimSpace(it.id); id != "" && id != "ws-acme" {
					m.defaultWS = id
					_ = setConfig(m.apiURL, id)
				}
			}
			m.mode = homeModeWorkspace
			m.applyWorkspaceFocus()
			m.refreshOptions()
			m.layout()
			return m, m.loadWorkspaceCmd(it.id)
		}
		return m, cmd
	default:
		return m, cmd
	}
}

func (m homeModel) updateWorkspaceMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace", "ctrl+g":
		// Hide workspace picker when there's only one workspace.
		if len(m.workspaces) <= 1 && msg.String() != "ctrl+g" {
			return m, nil
		}
		m.mode = homeModeOptions
		m.selectedWorkspaceID = ""
		m.workspaceMeta = nil
		m.refreshOptions()
		m.layout()
		return m, nil
	case "a":
		m.modal = m.newAuthModal()
		return m, nil
	case "x":
		m.modal = m.newDiagnosticsModal()
		return m, nil
	case "s":
		m.modal = m.newAgentSkillsModal()
		return m, nil
	case "tab":
		if m.workspaceFocus == homeFocusFlows {
			m.workspaceFocus = homeFocusRuns
		} else {
			m.workspaceFocus = homeFocusFlows
		}
		m.applyWorkspaceFocus()
		return m, nil
	case "r":
		if strings.TrimSpace(m.selectedWorkspaceID) == "" {
			return m, nil
		}
		return m, tea.Batch(m.checkConnectionCmd(), m.loadWorkspaceCmd(m.selectedWorkspaceID))
	}

	var cmd tea.Cmd
	msg = translateNavKeys(msg)
	if m.workspaceFocus == homeFocusFlows {
		m.flowsTable, cmd = m.flowsTable.Update(msg)
	} else {
		m.runsTable, cmd = m.runsTable.Update(msg)
	}
	return m, cmd
}

func (m homeModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}

	base := lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderBody())
	if m.modal != nil {
		// Dim base with a faint style.
		dim := lipgloss.NewStyle().Foreground(breytaMuted).Render(base)
		return dim + "\n" + m.modal.View(m.width, m.height)
	}
	return base
}

func (m homeModel) headerHeight() int {
	leftLines := 6 // Version, Context, Env, Auth, Default, WS
	if strings.TrimSpace(m.lastInfo) != "" {
		leftLines++
	}
	if strings.TrimSpace(m.apiError) != "" {
		leftLines++
	}

	midLines := len(m.headerKeyHints())

	// Logo is 6 lines when shown.
	rightLines := 0
	w := maxInt(40, m.width)
	rightW := minInt(44, maxInt(0, w-78))
	if rightW >= 40 {
		rightLines = 6
	}

	return maxInt(leftLines, maxInt(midLines, rightLines)) + 1 // + separator line
}

func (m homeModel) renderHeader() string {
	meta := lipgloss.NewStyle().Foreground(breytaMuted).Render
	label := lipgloss.NewStyle().Foreground(breytaTextColor).Render

	apiURL := strings.TrimSpace(m.apiBaseURL())

	envLabel := "local"
	if strings.TrimRight(m.apiURL, "/") == strings.TrimRight(configstore.DefaultProdAPIURL, "/") {
		envLabel = "prod"
	}

	authLabel := "-"
	if strings.TrimSpace(m.token) != "" {
		if email := strings.TrimSpace(m.userEmail); email != "" {
			authLabel = truncateRunes(email, 48)
		} else if email := authinfo.EmailFromToken(m.token); email != "" {
			authLabel = truncateRunes(email, 48)
		} else {
			authLabel = "(unknown user)"
		}
	}

	connLabel := "offline"
	if strings.TrimSpace(m.token) == "" {
		connLabel = "logged out"
	} else if m.connected {
		connLabel = "connected"
	}

	// Show the workspace id until we've loaded a workspace list that can resolve it to a name.
	defWS := cmpOrDash(m.workspaceNameOrID(m.defaultWS))
	activeWS := "-"
	if strings.TrimSpace(m.selectedWorkspaceID) != "" {
		activeWS = strings.TrimSpace(m.selectedWorkspaceID)
	}
	workspaceCount := len(m.workspaces)
	singleWorkspaceActive := workspaceCount == 1 && strings.TrimSpace(m.selectedWorkspaceID) != ""
	workspaceLabel := "WS:      "
	if singleWorkspaceActive {
		workspaceLabel = "Workspace: "
		activeWS = cmpOrDash(m.workspaceNameOrID(m.selectedWorkspaceID))
	}

	keys := m.headerKeyHints()

	logo := []string{
		" ____   ____   _____ __   __ _____   _    ",
		"| __ ) |  _ \\ | ____|\\ \\ / /|_   _| / \\   ",
		"|  _ \\ | |_) ||  _|   \\ V /   | |  / _ \\  ",
		"| |_) ||  _ < | |___   | |    | | / ___ \\ ",
		"|____/ |_| \\_\\|_____|  |_|    |_|/_/   \\_\\",
		"                                          ",
	}

	// Layout as 3 columns similar to k9s:
	// left=status, mid=keys, right=logo.
	// Keep within available width by letting the right column collapse first.
	w := maxInt(40, m.width)
	rightW := minInt(44, maxInt(0, w-78))
	midW := minInt(36, maxInt(24, w-2-rightW-32))
	leftW := maxInt(24, w-2-midW-rightW)

	renderKV := func(k, v string) string {
		avail := maxInt(0, leftW-len([]rune(k)))
		return label(k) + meta(truncateRunes(v, avail))
	}

	left := []string{
		renderKV("Version: ", buildInfoInline()),
		renderKV("Context: ", apiURL),
		renderKV("Env:     ", envLabel+" ("+connLabel+")"),
		renderKV("Auth:    ", authLabel),
	}
	if !singleWorkspaceActive {
		left = append(left, renderKV("Default: ", defWS))
	}
	if m.mode != homeModeWorkspace {
		left = append(left, renderKV(workspaceLabel, activeWS))
	}
	if info := strings.TrimSpace(m.lastInfo); info != "" {
		left = append(left, renderKV("Info:    ", info))
	}
	if errLine := strings.TrimSpace(m.apiError); errLine != "" {
		left = append(left, renderKV("Error:   ", errLine))
	}

	styledKeys := make([]string, 0, len(keys))
	for _, k := range keys {
		styledKeys = append(styledKeys, meta(truncateRunes(k, midW)))
	}

	leftCol := lipgloss.NewStyle().Width(leftW).Render(strings.Join(left, "\n"))
	midCol := lipgloss.NewStyle().Width(midW).Render(strings.Join(styledKeys, "\n"))
	rightCol := ""
	// Only show logo when it fits; avoid truncation that can make it read wrong.
	if rightW >= 40 {
		rightCol = lipgloss.NewStyle().Width(rightW).Foreground(breytaMuted).Render(strings.Join(logo, "\n"))
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", midCol)
	if rightCol != "" {
		header = lipgloss.JoinHorizontal(lipgloss.Top, header, "  ", rightCol)
	}

	sep := lipgloss.NewStyle().Foreground(breytaBorder).Render(strings.Repeat("─", maxInt(0, m.width)))
	return header + "\n" + sep
}

func (m homeModel) renderBody() string {
	switch m.mode {
	case homeModeOptions:
		return m.options.View()
	case homeModeWorkspace:
		return m.renderWorkspace()
	default:
		return ""
	}
}

func (m homeModel) headerKeyHints() []string {
	kv := func(k, v string) string { return fmt.Sprintf("<%s> %s", k, v) }

	common := []string{
		kv("a", "Auth"),
		kv("x", "Diagnostics"),
		kv("s", "Agent skills"),
		kv("r", "Refresh"),
	}
	switch m.mode {
	case homeModeWorkspace:
		keys := []string{
			kv("tab", "Switch focus"),
			kv("q", "Quit"),
		}
		if !(len(m.workspaces) == 1 && strings.TrimSpace(m.selectedWorkspaceID) != "") {
			keys = append(keys, kv("esc", "Back"))
		}
		return append(common, keys...)
	default:
		return append(common,
			kv("enter", "Open WS"),
			kv("q", "Quit"),
		)
	}
}

func (m homeModel) renderWorkspace() string {
	if m.workspaceMeta == nil {
		return "Loading workspace…"
	}

	metaStyle := lipgloss.NewStyle().Foreground(breytaMuted).Render
	h := []string{
		lipgloss.NewStyle().Bold(true).Foreground(breytaTextColor).Render("Workspace"),
		metaStyle(m.workspaceMeta.Name + " (" + m.workspaceMeta.ID + ")"),
	}
	if strings.TrimSpace(m.workspaceMeta.Plan) != "" {
		h = append(h, metaStyle("plan: "+m.workspaceMeta.Plan))
	}
	if strings.TrimSpace(m.workspaceMeta.Owner) != "" {
		h = append(h, metaStyle("owner: "+m.workspaceMeta.Owner))
	}
	if !m.workspaceMeta.UpdatedAt.IsZero() {
		h = append(h, metaStyle("updated: "+m.workspaceMeta.UpdatedAt.Format(time.RFC3339)))
	}

	sep := metaStyle(strings.Repeat("─", maxInt(0, m.width)))
	flowsTitle := lipgloss.NewStyle().Bold(true).Render("Flows")
	runsTitle := lipgloss.NewStyle().Bold(true).Render("Runs")
	return strings.Join([]string{
		strings.Join(h, "\n"),
		sep,
		flowsTitle,
		m.flowsTable.View(),
		sep,
		runsTitle,
		m.runsTable.View(),
	}, "\n")
}

func (m *homeModel) layout() {
	headerH := m.headerHeight()

	bodyH := m.height - headerH
	if bodyH < 5 {
		bodyH = 5
	}

	m.options.SetSize(maxInt(10, m.width), bodyH)

	tableW := maxInt(10, m.width-4)
	inner := bodyH - 8
	if inner < 6 {
		inner = 6
	}
	flowsH := inner / 2
	runsH := inner - flowsH
	if flowsH < 3 {
		flowsH = 3
	}
	if runsH < 3 {
		runsH = 3
	}

	m.flowsTable.SetWidth(tableW)
	m.flowsTable.SetHeight(flowsH)
	safeSetColumns(&m.flowsTable, flowsColumns(tableW))

	m.runsTable.SetWidth(tableW)
	m.runsTable.SetHeight(runsH)
	safeSetColumns(&m.runsTable, runsColumns(tableW))
}

func (m *homeModel) applyWorkspaceFocus() {
	m.flowsTable.Blur()
	m.runsTable.Blur()
	if m.workspaceFocus == homeFocusFlows {
		m.flowsTable.Focus()
	} else {
		m.runsTable.Focus()
	}
}

func (m *homeModel) loadConfig() (apiURL string, defaultWS string) {
	apiURL = strings.TrimSpace(m.cfg.APIURL)
	defaultWS = strings.TrimSpace(m.cfg.WorkspaceID)

	var st *configstore.Store
	if p, err := configstore.DefaultPath(); err == nil && strings.TrimSpace(p) != "" {
		st, _ = configstore.Load(p)
	}
	if st != nil && st.DevMode {
		if strings.TrimSpace(apiURL) == "" {
			if prof, ok := resolveDevProfile(st); ok && strings.TrimSpace(prof.APIURL) != "" {
				apiURL = prof.APIURL
			}
		}
		if strings.TrimSpace(defaultWS) == "" {
			if prof, ok := resolveDevProfile(st); ok && strings.TrimSpace(prof.WorkspaceID) != "" {
				defaultWS = prof.WorkspaceID
			}
		}
		if strings.TrimSpace(apiURL) == "" && strings.TrimSpace(st.DevAPIURL) != "" {
			apiURL = st.DevAPIURL
		}
		if strings.TrimSpace(defaultWS) == "" && strings.TrimSpace(st.DevWorkspaceID) != "" {
			defaultWS = st.DevWorkspaceID
		}
	}
	if st == nil || !st.DevMode {
		apiURL = configstore.DefaultProdAPIURL
	}
	if strings.TrimSpace(apiURL) == "" {
		if st != nil && strings.TrimSpace(st.APIURL) != "" {
			apiURL = st.APIURL
		} else {
			apiURL = configstore.DefaultProdAPIURL
		}
	}

	if strings.TrimSpace(defaultWS) == "" && st != nil && strings.TrimSpace(st.WorkspaceID) != "" {
		// Only apply stored default workspace when it matches the current apiUrl.
		// This prevents local/mock workspace ids (e.g. ws-acme) from showing up in prod.
		cfgAPI := strings.TrimRight(strings.TrimSpace(st.APIURL), "/")
		appAPI := strings.TrimRight(strings.TrimSpace(apiURL), "/")
		if cfgAPI != "" && appAPI != "" && cfgAPI == appAPI {
			defaultWS = st.WorkspaceID
		}
	}
	// Never show or use the mock placeholder workspace as a default.
	if strings.TrimSpace(defaultWS) == "ws-acme" {
		defaultWS = ""
	}
	return strings.TrimSpace(apiURL), strings.TrimSpace(defaultWS)
}

func resolveDevProfile(st *configstore.Store) (configstore.DevProfile, bool) {
	if st == nil {
		return configstore.DevProfile{}, false
	}
	if len(st.DevProfiles) == 0 {
		return configstore.DevProfile{}, false
	}
	name := strings.TrimSpace(st.DevActive)
	if name == "" {
		if _, ok := st.DevProfiles["local"]; ok {
			name = "local"
		}
	}
	if name == "" {
		for candidate := range st.DevProfiles {
			name = candidate
			break
		}
	}
	if name == "" {
		return configstore.DevProfile{}, false
	}
	prof, ok := st.DevProfiles[name]
	if !ok {
		return configstore.DevProfile{}, false
	}
	return prof, true
}

func isProdAPIURL(apiURL string) bool {
	return strings.TrimRight(strings.TrimSpace(apiURL), "/") == strings.TrimRight(configstore.DefaultProdAPIURL, "/")
}

func (m homeModel) workspaceNameOrID(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	for _, ws := range m.workspaces {
		if strings.TrimSpace(ws.ID) == workspaceID {
			if name := strings.TrimSpace(ws.Name); name != "" {
				return name
			}
			return workspaceID
		}
	}
	return workspaceID
}

func (m homeModel) workspaceName(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	for _, ws := range m.workspaces {
		if strings.TrimSpace(ws.ID) == workspaceID {
			return strings.TrimSpace(ws.Name)
		}
	}
	return ""
}

func (m *homeModel) resolveToken() string {
	if tok := strings.TrimSpace(m.cfg.Token); tok != "" {
		return tok
	}
	apiURL := strings.TrimSpace(m.apiURL)
	if apiURL == "" {
		return ""
	}
	p, err := authStorePath()
	if err != nil || strings.TrimSpace(p) == "" {
		return ""
	}
	st, err := authstore.Load(p)
	if err != nil || st == nil {
		return ""
	}
	tok, _ := st.Get(apiURL)
	return strings.TrimSpace(tok)
}

type homeTokenRefreshedMsg struct {
	token     string
	refreshed bool
	err       error
}

func (m *homeModel) refreshTokenCmd() tea.Cmd {
	return func() tea.Msg {
		apiURL := m.apiBaseURL()
		token, refreshed, err := resolveTokenForAPI(apiURL, m.cfg.Token)
		return homeTokenRefreshedMsg{token: token, refreshed: refreshed, err: err}
	}
}

func (m *homeModel) apiBaseURL() string {
	if strings.TrimSpace(m.apiURL) == "" {
		return configstore.DefaultProdAPIURL
	}
	return strings.TrimSpace(m.apiURL)
}

func (m *homeModel) checkConnectionCmd() tea.Cmd {
	return func() tea.Msg {
		return m.checkConnectionMsg(false)
	}
}

func (m *homeModel) checkConnectionInfoCmd(source string) tea.Cmd {
	source = strings.TrimSpace(source)
	return func() tea.Msg {
		msg := m.checkConnectionMsg(true)
		if dm, ok := msg.(homeDiagMsg); ok {
			dm.info = strings.TrimSpace(source) + ": " + dm.info
			return dm
		}
		return msg
	}
}

func (m *homeModel) checkConnectionMsg(includeInfo bool) tea.Msg {
	apiURL := m.apiBaseURL()
	token, _, refreshErr := resolveTokenForAPI(apiURL, m.cfg.Token)
	if token == "" {
		info := ""
		if includeInfo {
			info = "not logged in"
		}
		return homeDiagMsg{connected: false, httpStatus: 0, apiError: "not logged in", info: info}
	}
	client := api.Client{BaseURL: apiURL, Token: token}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/auth/verify", nil, nil)
	if err != nil {
		info := ""
		if includeInfo {
			st := "-"
			if status != 0 {
				st = fmt.Sprintf("%d", status)
			}
			info = "offline (status=" + st + ")"
		}
		apiErr := err.Error()
		if refreshErr != nil {
			apiErr = "auth refresh failed: " + refreshErr.Error()
		}
		return homeDiagMsg{connected: false, httpStatus: status, apiError: apiErr, info: info}
	}
	if status >= 400 {
		info := ""
		if includeInfo {
			info = fmt.Sprintf("offline (status=%d)", status)
		}
		apiErr := fmt.Sprintf("api error (status=%d)", status)
		if refreshErr != nil {
			apiErr = "auth refresh failed: " + refreshErr.Error()
		}
		return homeDiagMsg{connected: false, httpStatus: status, apiError: apiErr, info: info}
	}
	userEmail := parseVerifyEmail(out)
	info := ""
	if includeInfo {
		info = fmt.Sprintf("connected (status=%d)", status)
	}
	return homeDiagMsg{connected: true, httpStatus: status, apiError: "", info: info, userEmail: userEmail}
}

func (m *homeModel) checkMeMsg(includeInfo bool) tea.Msg {
	apiURL := m.apiBaseURL()
	token, _, refreshErr := resolveTokenForAPI(apiURL, m.cfg.Token)
	if token == "" {
		info := ""
		if includeInfo {
			info = "not logged in"
		}
		return homeDiagMsg{connected: false, httpStatus: 0, apiError: "not logged in", info: info}
	}
	client := api.Client{BaseURL: apiURL, Token: token}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		info := ""
		if includeInfo {
			st := "-"
			if status != 0 {
				st = fmt.Sprintf("%d", status)
			}
			info = "offline (status=" + st + ")"
		}
		apiErr := err.Error()
		if refreshErr != nil {
			apiErr = "auth refresh failed: " + refreshErr.Error()
		}
		return homeDiagMsg{connected: false, httpStatus: status, apiError: apiErr, info: info}
	}
	if status >= 400 {
		info := ""
		if includeInfo {
			info = fmt.Sprintf("offline (status=%d)", status)
		}
		apiErr := fmt.Sprintf("api error (status=%d)", status)
		if refreshErr != nil {
			apiErr = "auth refresh failed: " + refreshErr.Error()
		}
		return homeDiagMsg{connected: false, httpStatus: status, apiError: apiErr, info: info}
	}
	userEmail, wsCount := parseMeUserAndCount(out)
	info := ""
	if includeInfo {
		if wsCount >= 0 {
			info = fmt.Sprintf("ok (status=%d, workspaces=%d)", status, wsCount)
		} else {
			info = fmt.Sprintf("ok (status=%d)", status)
		}
	}
	return homeDiagMsg{connected: true, httpStatus: status, apiError: "", info: info, userEmail: userEmail, wsCount: wsCount}
}

func (m *homeModel) fetchWorkspacesCmd() tea.Cmd {
	return func() tea.Msg {
		apiURL := m.apiBaseURL()
		token, _, err := resolveTokenForAPI(apiURL, m.cfg.Token)
		if token == "" {
			return homeWorkspacesMsg{err: errors.New("not logged in")}
		}
		if err != nil {
			return homeWorkspacesMsg{err: err}
		}
		client := api.Client{BaseURL: apiURL, Token: token}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
		if err != nil {
			return homeWorkspacesMsg{httpStatus: status, err: err}
		}
		raw, err := parseMeWorkspaces(out)
		if err != nil {
			return homeWorkspacesMsg{httpStatus: status, err: err}
		}
		userEmail, wsCount := parseMeUserAndCount(out)
		sort.Slice(raw, func(i, j int) bool {
			li := strings.TrimSpace(raw[i].Name)
			if li == "" {
				li = strings.TrimSpace(raw[i].ID)
			}
			lj := strings.TrimSpace(raw[j].Name)
			if lj == "" {
				lj = strings.TrimSpace(raw[j].ID)
			}
			return li < lj
		})
		return homeWorkspacesMsg{workspaces: raw, httpStatus: status, err: nil, userEmail: userEmail, wsCount: wsCount}
	}
}

func (m homeModel) checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		n, err := updatecheck.CheckNow(ctx, buildinfo.DisplayVersion(), 24*time.Hour)
		return homeUpdateMsg{notice: n, err: err}
	}
}

func (m *homeModel) newAuthModal() *modalModel {
	md := &modalModel{
		kind:  modalAuth,
		title: "Auth",
		list:  newModalList("Pick auth action", false),
	}
	_ = md.list.SetItems([]list.Item{
		modalItem{id: "login", name: "Login", desc: "Open browser login and store token"},
		modalItem{id: "logout", name: "Logout", desc: "Clear stored token"},
	})
	return md
}

func (m *homeModel) newDiagnosticsModal() *modalModel {
	md := &modalModel{
		kind:  modalDiagnostics,
		title: "Diagnostics",
		list:  newModalList("Pick diagnostic action", false),
	}
	_ = md.list.SetItems([]list.Item{
		modalItem{id: "me", name: "Check workspaces", desc: "Call /api/me"},
		modalItem{id: "check", name: "Verify token", desc: "Call /api/auth/verify"},
	})
	return md
}

func (m *homeModel) newAgentSkillsModal() *modalModel {
	md := &modalModel{
		kind:  modalAgentSkills,
		title: "Agent skills",
		list:  newModalList("Install: teach your agent to use the Breyta CLI", false),
		status: strings.TrimSpace(`
This installs Breyta workflow authoring instructions into your agent tool.
It writes a local file only (no network). Choose your agent:
- Codex: ~/.codex/skills/breyta/SKILL.md
- Cursor: ~/.cursor/rules/breyta/RULE.md
- Claude Code: ~/.claude/skills/breyta/SKILL.md
`),
	}

	_ = md.list.SetItems([]list.Item{
		modalItem{
			id:   "install:codex",
			name: "Codex",
			desc: "Writes: ~/.codex/skills/breyta/SKILL.md",
		},
		modalItem{
			id:   "install:cursor",
			name: "Cursor",
			desc: "Writes: ~/.cursor/rules/breyta/RULE.md",
		},
		modalItem{
			id:   "install:claude",
			name: "Claude Code",
			desc: "Writes: ~/.claude/skills/breyta/SKILL.md",
		},
	})
	return md
}

func (m *homeModel) newUpdateModal(n *updatecheck.Notice) *modalModel {
	cur := buildinfo.DisplayVersion()
	latest := ""
	if n != nil {
		latest = strings.TrimSpace(n.LatestVersion)
	}
	status := fmt.Sprintf("A new Breyta CLI version is available:\n- current: %s\n- latest:  %s\n", cur, cmpOrDash(latest))

	md := &modalModel{
		kind:   modalUpdate,
		title:  "Update available",
		list:   newModalList("Pick action", false),
		status: strings.TrimSpace(status),
	}

	items := []list.Item{
		modalItem{id: "skip", name: "Skip for now", desc: "Continue into the TUI"},
	}
	if n != nil && n.InstallMethod == updatecheck.InstallMethodBrew && updatecheck.BrewAvailable() {
		items = append([]list.Item{
			modalItem{id: "upgrade", name: "Upgrade now (Homebrew)", desc: "Runs: brew upgrade breyta, then restarts"},
		}, items...)
	}
	_ = md.list.SetItems(items)
	return md
}

func (m *homeModel) applyModal() tea.Cmd {
	if m.modal == nil {
		return nil
	}
	switch m.modal.kind {
	case modalAuth:
		it, _ := m.modal.list.SelectedItem().(modalItem)
		apiURL := m.apiBaseURL()
		switch it.id {
		case "logout":
			if err := logoutStoredToken(apiURL); err != nil {
				m.apiError = err.Error()
				m.refreshOptions()
				return nil
			}
			m.token = ""
			m.cfg.Token = ""
			m.workspaces = nil
			m.workspacesErr = nil
			m.refreshOptions()
			return m.checkConnectionCmd()
		default: // login
			return func() tea.Msg {
				sess, openErr, err := startBrowserLogin(apiURL)
				if err != nil {
					return homeLoginStartMsg{apiURL: apiURL, status: "login failed", err: err}
				}
				return homeLoginStartMsg{
					apiURL:  apiURL,
					authURL: strings.TrimSpace(sess.AuthURL),
					openErr: openErr,
					status:  "login started",
					session: sess,
				}
			}
		}

	case modalDiagnostics:
		it, _ := m.modal.list.SelectedItem().(modalItem)
		apiURL := m.apiBaseURL()
		token, _, err := resolveTokenForAPI(apiURL, m.cfg.Token)
		if token == "" {
			return func() tea.Msg {
				return homeDiagMsg{connected: false, httpStatus: 0, apiError: "not logged in", info: "not logged in"}
			}
		}
		if err != nil {
			return func() tea.Msg {
				return homeDiagMsg{connected: false, httpStatus: 0, apiError: err.Error(), info: "auth refresh failed"}
			}
		}
		client := api.Client{BaseURL: apiURL, Token: token}
		switch it.id {
		case "me":
			return func() tea.Msg {
				return m.checkMeMsg(true)
			}
		case "check":
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/auth/verify", nil, nil)
				if err != nil {
					return homeDiagMsg{connected: false, httpStatus: status, apiError: err.Error(), info: fmt.Sprintf("verify: error (status=%d)", status)}
				}
				if status >= 400 {
					return homeDiagMsg{connected: false, httpStatus: status, apiError: fmt.Sprintf("verify failed (status=%d)", status), info: fmt.Sprintf("verify: failed (status=%d)", status)}
				}
				return homeDiagMsg{connected: true, httpStatus: status, apiError: "", info: fmt.Sprintf("verify: ok (status=%d)", status)}
			}
		default:
			return m.checkConnectionInfoCmd("check")
		}

	case modalAgentSkills:
		it, _ := m.modal.list.SelectedItem().(modalItem)
		id := strings.TrimSpace(it.id)
		if id == "" {
			m.apiError = "no action selected"
			m.refreshOptions()
			return nil
		}

		install := func(p skills.Provider) {
			home, err := os.UserHomeDir()
			if err != nil {
				m.apiError = err.Error()
				return
			}

			target, err := skills.Target(home, p)
			if err != nil {
				m.apiError = err.Error()
				return
			}

			paths, err := skills.InstallBreytaSkill(home, p)
			if err != nil {
				m.apiError = err.Error()
				return
			}
			m.apiError = ""
			_ = paths
			m.lastInfo = "installed skill in " + target.Dir
		}

		switch id {
		case "install:codex":
			install(skills.ProviderCodex)
		case "install:cursor":
			install(skills.ProviderCursor)
		case "install:claude":
			install(skills.ProviderClaude)
		default:
			m.apiError = "unknown action: " + id
		}

		m.refreshOptions()
		return nil

	case modalUpdate:
		it, _ := m.modal.list.SelectedItem().(modalItem)
		switch strings.TrimSpace(it.id) {
		case "upgrade":
			if m.updateNotice == nil {
				m.apiError = "missing update info"
				m.refreshOptions()
				return nil
			}
			m.doUpgrade = true
			return tea.Quit
		default: // skip
			m.lastInfo = "skipped update"
			m.refreshOptions()
			return nil
		}
	}
	return nil
}

func (m *homeModel) loadWorkspaceCmd(workspaceID string) tea.Cmd {
	return func() tea.Msg {
		apiURL := m.apiBaseURL()
		token, _, err := resolveTokenForAPI(apiURL, m.cfg.Token)
		if token == "" {
			return homeWorkspaceLoadedMsg{workspaceID: workspaceID, err: errors.New("not logged in")}
		}
		if err != nil {
			return homeWorkspaceLoadedMsg{workspaceID: workspaceID, err: err}
		}
		meta, flowsValues, runsValues, err := loadWorkspaceDetailAPI(apiURL, token, workspaceID)
		if err != nil {
			return homeWorkspaceLoadedMsg{workspaceID: workspaceID, err: err}
		}
		return homeWorkspaceLoadedMsg{
			workspaceID: workspaceID,
			meta: &homeWorkspaceMeta{
				ID:        meta.ID,
				Name:      meta.Name,
				Plan:      meta.Plan,
				Owner:     meta.Owner,
				UpdatedAt: meta.UpdatedAt,
			},
			flowsValues: flowsValues,
			runsValues:  runsValues,
			status:      "ready",
			err:         nil,
		}
	}
}

type homeWorkspaceLoadedMsg struct {
	workspaceID string
	meta        *homeWorkspaceMeta
	flowsValues []map[string]string
	runsValues  []map[string]string
	status      string
	err         error
}

func (m homeModel) applyWorkspaceLoaded(msg homeWorkspaceLoadedMsg) homeModel {
	m.selectedWorkspaceID = msg.workspaceID
	m.workspaceMeta = msg.meta
	m.mode = homeModeWorkspace
	m.applyWorkspaceFocus()
	m.refreshOptions()
	m.layout()

	flowCols := m.flowsTable.Columns()
	flowsRows := make([]table.Row, 0, len(msg.flowsValues))
	for _, v := range msg.flowsValues {
		flowsRows = append(flowsRows, rowForColumns(flowCols, v))
	}
	m.flowsTable.SetRows(flowsRows)

	runCols := m.runsTable.Columns()
	runsRows := make([]table.Row, 0, len(msg.runsValues))
	for _, v := range msg.runsValues {
		runsRows = append(runsRows, rowForColumns(runCols, v))
	}
	m.runsTable.SetRows(runsRows)
	return m
}

// --- API parsing helpers (kept for tests) ---

type meWorkspace struct {
	ID   string
	Name string
	Raw  map[string]any
}

func parseMeWorkspaces(out any) ([]meWorkspace, error) {
	m, ok := out.(map[string]any)
	if !ok {
		return nil, errors.New("unexpected /api/me response")
	}
	raw, _ := m["workspaces"].([]any)
	items := make([]meWorkspace, 0, len(raw))
	for _, v := range raw {
		wm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := wm["id"].(string)
		name, _ := wm["name"].(string)
		items = append(items, meWorkspace{ID: id, Name: name, Raw: wm})
	}
	return items, nil
}

func parseMeUserAndCount(out any) (email string, wsCount int) {
	wsCount = -1
	m, ok := out.(map[string]any)
	if !ok {
		return "", -1
	}
	if u, ok := m["user"].(map[string]any); ok {
		if e, _ := u["email"].(string); strings.TrimSpace(e) != "" {
			email = strings.TrimSpace(e)
		}
	}
	if raw, ok := m["workspaces"].([]any); ok {
		wsCount = len(raw)
	}
	return email, wsCount
}

func parseVerifyEmail(out any) string {
	m, ok := out.(map[string]any)
	if !ok {
		return ""
	}
	u, ok := m["user"].(map[string]any)
	if !ok {
		return ""
	}
	email, _ := u["email"].(string)
	return strings.TrimSpace(email)
}

func cycleAPIURL(current string) string {
	current = strings.TrimSpace(current)
	switch strings.TrimRight(current, "/") {
	case strings.TrimRight(configstore.DefaultLocalAPIURL, "/"):
		return configstore.DefaultProdAPIURL
	case strings.TrimRight(configstore.DefaultProdAPIURL, "/"):
		return configstore.DefaultLocalAPIURL
	default:
		return configstore.DefaultLocalAPIURL
	}
}

// --- Workspace detail loading (API mode) ---

type apiWorkspaceMeta struct {
	ID        string
	Name      string
	Plan      string
	Owner     string
	UpdatedAt time.Time
}

func loadWorkspaceDetailAPI(apiURL, token, workspaceID string) (*apiWorkspaceMeta, []map[string]string, []map[string]string, error) {
	client := api.Client{BaseURL: apiURL, Token: token}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out, status, err := client.DoRootREST(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	if status >= 400 {
		return nil, nil, nil, fmt.Errorf("api error (status=%d)", status)
	}

	raw, err := parseMeWorkspaces(out)
	if err != nil {
		return nil, nil, nil, err
	}

	meta := &apiWorkspaceMeta{ID: workspaceID}
	for _, ws := range raw {
		if strings.TrimSpace(ws.ID) == strings.TrimSpace(workspaceID) {
			meta.Name = strings.TrimSpace(ws.Name)
			meta.Plan = anyString(ws.Raw, "plan")
			meta.Owner = anyString(ws.Raw, "owner")
			meta.UpdatedAt = anyTime(ws.Raw, "updatedAt")
			break
		}
	}

	wsClient := api.Client{BaseURL: apiURL, WorkspaceID: workspaceID, Token: token}
	flowsResp, _, err := wsClient.DoCommand(ctx, "flows.list", map[string]any{})
	if err != nil {
		return meta, nil, nil, err
	}
	flowsItems := extractDataItems(flowsResp)

	runsResp, _, err := wsClient.DoCommand(ctx, "runs.list", map[string]any{"limit": 50})
	if err != nil {
		return meta, nil, nil, err
	}
	runsItems := extractDataItems(runsResp)

	flowsValues := make([]map[string]string, 0, len(flowsItems))
	for _, v := range flowsItems {
		mm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		slug, _ := mm["flowSlug"].(string)
		name, _ := mm["name"].(string)
		ver := anyToInt(mm["activeVersion"])
		desc, _ := mm["description"].(string)
		tags := anyToStringSlice(mm["tags"])
		tagsStr := "-"
		if len(tags) > 0 {
			tagsStr = strings.Join(tags, ",")
		}
		if strings.TrimSpace(desc) == "" {
			desc = "-"
		}
		flowsValues = append(flowsValues, map[string]string{
			"slug":    strings.TrimSpace(slug),
			"name":    strings.TrimSpace(name),
			"ver":     fmt.Sprintf("v%d", ver),
			"tags":    tagsStr,
			"updated": "-",
			"desc":    desc,
		})
	}

	runsValues := make([]map[string]string, 0, len(runsItems))
	for _, v := range runsItems {
		mm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		runID := firstString(mm, "workflowId", "runId", "id")
		flowSlug, _ := mm["flowSlug"].(string)
		status, _ := mm["status"].(string)
		step := firstString(mm, "currentStep", "stepId")
		if strings.TrimSpace(step) == "" {
			step = "-"
		}
		by := firstString(mm, "triggeredBy", "startedBy")
		if strings.TrimSpace(by) == "" {
			by = "-"
		}
		runsValues = append(runsValues, map[string]string{
			"run":     strings.TrimSpace(runID),
			"flow":    strings.TrimSpace(flowSlug),
			"status":  strings.TrimSpace(status),
			"step":    step,
			"by":      by,
			"started": "-",
			"updated": "-",
		})
	}

	return meta, flowsValues, runsValues, nil
}

func extractDataItems(resp map[string]any) []any {
	if resp == nil {
		return nil
	}
	data, _ := resp["data"].(map[string]any)
	raw, _ := data["items"].([]any)
	if raw == nil {
		return nil
	}
	out := make([]any, 0, len(raw))
	for _, v := range raw {
		if v != nil {
			out = append(out, v)
		}
	}
	return out
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func anyToInt(v any) int64 {
	switch v := v.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func anyToStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func anyString(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func anyTime(m map[string]any, k string) time.Time {
	v := anyString(m, k)
	if v == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	return time.Time{}
}

// --- persistence helpers ---

func setConfig(apiURL, workspaceID string) error {
	p, err := configstore.DefaultPath()
	if err != nil {
		return err
	}
	st, _ := configstore.Load(p)
	if st == nil {
		st = &configstore.Store{}
	}
	st.APIURL = strings.TrimSpace(apiURL)
	st.WorkspaceID = strings.TrimSpace(workspaceID)
	return configstore.SaveAtomic(p, st)
}

func storeToken(apiURL, token string) error {
	return storeAuthRecord(apiURL, token, "", "")
}

func logoutStoredToken(apiURL string) error {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil
	}
	p, err := authStorePath()
	if err != nil {
		return err
	}
	st, err := authstore.Load(p)
	if err != nil {
		return nil
	}
	st.Delete(apiURL)
	return authstore.SaveAtomic(p, st)
}

func cmpOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func translateNavKeys(msg tea.KeyMsg) tea.KeyMsg {
	switch msg.String() {
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+p":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "ctrl+b":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	default:
		return msg
	}
}

func (m *homeModel) refreshOptions() {
	selectedWS := ""
	if it, ok := m.options.SelectedItem().(workspaceItem); ok {
		selectedWS = strings.TrimSpace(it.id)
	}

	items := []list.Item{}

	if strings.TrimSpace(m.token) == "" {
		items = append(items, workspaceItem{id: "", name: "(login to view workspaces)", desc: "Press <a> to login"})
	} else if m.workspacesErr != nil {
		items = append(items, workspaceItem{id: "", name: "(failed to load workspaces)", desc: m.workspacesErr.Error()})
	} else if len(m.workspaces) == 0 {
		items = append(items, workspaceItem{id: "", name: "(no workspaces loaded yet)", desc: "Press 'r' to refresh"})
	} else {
		for _, ws := range m.workspaces {
			id := strings.TrimSpace(ws.ID)
			if id == "" {
				continue
			}
			label := strings.TrimSpace(ws.Name)
			if label == "" {
				label = id
			}
			desc := id
			items = append(items, workspaceItem{id: id, name: label, desc: desc})
		}
	}
	_ = m.options.SetItems(items)

	if selectedWS != "" {
		for idx, it := range items {
			if wi, ok := it.(workspaceItem); ok && strings.TrimSpace(wi.id) == selectedWS {
				m.options.Select(idx)
				break
			}
		}
	}
}
