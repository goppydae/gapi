package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/internal/eventbus"
	protopkg "github.com/goppydae/gapi/internal/proto"
	"github.com/goppydae/gapi/internal/transport"
)

type ViewMode int

const (
	ViewList ViewMode = iota
	ViewLogs
	ViewDetail
)

type AgentStatus struct {
	ID     string
	State  string
	Type   string
	CPU    string
	Memory string
	Uptime time.Duration
}

type Model struct {
	view        ViewMode
	agents      []AgentStatus
	selectedIdx int
	logs        []string
	logViewer   LogViewer
	filter      Filter
	width       int
	height      int
	err         error
	quitting    bool
}

func NewModel() Model {
	return Model{
		view:        ViewList,
		agents:      []AgentStatus{},
		selectedIdx: 0,
		logs:        []string{},
		logViewer:   LogViewer{},
		filter:      NewFilter(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchAgentStatus,
		tickEvery(time.Second),
	)
}

// Messages
type agentStatusMsg []AgentStatus
type tickMsg time.Time
type errMsg error

func fetchAgentStatus() tea.Msg {
	cfg, err := config.Load()
	if err != nil {
		return errMsg(fmt.Errorf("failed to load config: %w", err))
	}

	t, err := transport.NewClientFromConfig(cfg.Transport)
	if err != nil {
		return errMsg(fmt.Errorf("failed to create transport: %w", err))
	}
	defer t.Close()

	bus := eventbus.NewEventBus(t)

	done := make(chan []AgentStatus)
	errChan := make(chan error) // Subscribe to detailed status
	bus.SubscribeOnce("system", "", "agents.reply", func(e eventbus.Event[*anypb.Any]) {
		var resp protopkg.AgentStatusResponse
		if err := e.Payload.UnmarshalTo(&resp); err != nil {
			errChan <- err
			return
		}

		agents := make([]AgentStatus, 0, len(resp.Agents))
		for _, a := range resp.Agents {
			agents = append(agents, AgentStatus{
				ID:     a.Id,
				State:  stateToString(a.State),
				Type:   a.Type,
				CPU:    "-", // TODO: Add resource usage
				Memory: "-",
				Uptime: 0, // TODO: Calculate from start time
			})
		}
		done <- agents
	})

	req := &protopkg.AgentStatusRequest{}
	anyReq, _ := anypb.New(req)
	bus.Publish(eventbus.NewEvent("system", "", "agents/", "tui", anyReq, true))

	select {
	case agents := <-done:
		return agentStatusMsg(agents)
	case err := <-errChan:
		return errMsg(err)
	case <-time.After(2 * time.Second):
		return errMsg(fmt.Errorf("timeout fetching agent status"))
	}
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update log viewer size if in log view
		if m.view == ViewLogs {
			var cmd tea.Cmd
			m.logViewer, cmd = m.logViewer.Update(msg, m.agents, m.selectedIdx)
			return m, cmd
		}
		return m, nil

	case agentStatusMsg:
		m.agents = msg
		return m, nil

	case tickMsg:
		// Update log viewer if in log view
		if m.view == ViewLogs {
			var cmd tea.Cmd
			m.logViewer, cmd = m.logViewer.Update(msg, m.agents, m.selectedIdx)
			return m, tea.Batch(
				fetchAgentStatus,
				tickEvery(time.Second),
				cmd,
			)
		}

		return m, tea.Batch(
			fetchAgentStatus,
			tickEvery(time.Second),
		)

	case lifecycleActionMsg:
		if msg.success {
			// Refresh agent status immediately
			return m, fetchAgentStatus
		} else if msg.err != nil {
			m.err = msg.err
		}
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewList:
		return m.handleListKeys(msg)
	case ViewLogs:
		return m.handleLogsKeys(msg)
	case ViewDetail:
		return m.handleDetailKeys(msg)
	}
	return m, nil
}

func (m Model) handleListKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If filter is active, let it handle the input
	if m.filter.active {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "/":
		// Activate name search
		m.filter.Activate(FilterByName)

	case "f":
		// Cycle through state filters
		if m.filter.mode == FilterByState && m.filter.query == "running" {
			m.filter.SetQuery("stopped")
		} else if m.filter.mode == FilterByState && m.filter.query == "stopped" {
			m.filter.mode = FilterNone
			m.filter.SetQuery("")
		} else {
			m.filter.Activate(FilterByState)
			m.filter.SetQuery("running")
			m.filter.Deactivate()
		}

	case "t":
		// Cycle through type filters
		if m.filter.mode == FilterByType && m.filter.query == "service" {
			m.filter.SetQuery("timer")
		} else if m.filter.mode == FilterByType && m.filter.query == "timer" {
			m.filter.SetQuery("socket")
		} else if m.filter.mode == FilterByType && m.filter.query == "socket" {
			m.filter.mode = FilterNone
			m.filter.SetQuery("")
		} else {
			m.filter.Activate(FilterByType)
			m.filter.SetQuery("service")
			m.filter.Deactivate()
		}

	case "esc":
		// Clear filter
		m.filter.mode = FilterNone
		m.filter.SetQuery("")

	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}

	case "down", "j":
		// Count filtered agents
		filteredCount := 0
		for _, agent := range m.agents {
			if m.filter.Matches(agent) {
				filteredCount++
			}
		}
		if m.selectedIdx < filteredCount-1 {
			m.selectedIdx++
		}

	case "enter":
		m.view = ViewDetail

	case "l":
		// Initialize log viewer for selected agent
		if m.selectedIdx < len(m.agents) {
			m.logViewer = NewLogViewer(m.agents[m.selectedIdx].ID, m.width, m.height)
		}
		m.view = ViewLogs

	case "s":
		// Start selected agent
		if m.selectedIdx < len(m.agents) {
			return m, sendLifecycleAction(m.agents[m.selectedIdx].ID, "start")
		}

	case "x":
		// Stop selected agent
		if m.selectedIdx < len(m.agents) {
			return m, sendLifecycleAction(m.agents[m.selectedIdx].ID, "stop")
		}

	case "r":
		// Reload selected agent
		if m.selectedIdx < len(m.agents) {
			return m, sendLifecycleAction(m.agents[m.selectedIdx].ID, "reload")
		}
	}

	return m, nil
}

func (m Model) handleLogsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "esc":
		m.view = ViewList
	}

	return m, nil
}

func (m Model) handleDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "esc":
		m.view = ViewList
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	switch m.view {
	case ViewList:
		return m.renderList()
	case ViewLogs:
		return m.logViewer.View()
	case ViewDetail:
		return m.renderDetail()
	default:
		return "Unknown view"
	}
}

func (m Model) renderList() string {
	var s string

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1)

	running := 0
	stopped := 0
	filtered := 0
	for _, a := range m.agents {
		if a.State == "RUNNING" || a.State == "running" {
			running++
		} else {
			stopped++
		}
		if m.filter.Matches(a) {
			filtered++
		}
	}

	header := fmt.Sprintf("GAPI Monitor - Agents: %d running, %d stopped", running, stopped)
	if m.filter.mode != FilterNone {
		header += fmt.Sprintf(" [%d/%d filtered]", filtered, len(m.agents))
	}
	s += headerStyle.Render(header) + "\n\n"

	// Filter status
	if filterStatus := m.filter.StatusLine(); filterStatus != "" {
		filterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Italic(true)
		s += filterStyle.Render(filterStatus) + "\n\n"
	}

	// Table header
	tableHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99"))

	s += tableHeader.Render(fmt.Sprintf("%-20s %-12s %-8s %-10s %-10s\n",
		"ID", "STATE", "TYPE", "CPU", "MEMORY"))

	// Agent rows (filtered)
	displayIdx := 0
	for _, agent := range m.agents {
		if !m.filter.Matches(agent) {
			continue
		}

		style := lipgloss.NewStyle()

		if displayIdx == m.selectedIdx {
			style = style.Background(lipgloss.Color("63")).Foreground(lipgloss.Color("230"))
		}

		stateColor := lipgloss.Color("10") // green
		if agent.State != "RUNNING" && agent.State != "running" {
			stateColor = lipgloss.Color("9") // red
		}

		row := fmt.Sprintf("%-20s %-12s %-8s %-10s %-10s",
			agent.ID,
			agent.State,
			agent.Type,
			agent.CPU,
			agent.Memory,
		)

		if displayIdx == m.selectedIdx {
			s += style.Render(row) + "\n"
		} else {
			stateStyle := lipgloss.NewStyle().Foreground(stateColor)
			s += fmt.Sprintf("%-20s %s %-8s %-10s %-10s\n",
				agent.ID,
				stateStyle.Render(fmt.Sprintf("%-12s", agent.State)),
				agent.Type,
				agent.CPU,
				agent.Memory,
			)
		}
		displayIdx++
	}

	// Footer
	s += "\n"
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	s += footerStyle.Render("[↑↓/jk] navigate  [/] search  [f] filter state  [t] filter type  [esc] clear  [s/x/r] control  [q] quit")

	return s
}

func (m Model) renderLogs() string {
	var s string

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	if m.selectedIdx < len(m.agents) {
		s += headerStyle.Render(fmt.Sprintf("Logs: %s", m.agents[m.selectedIdx].ID)) + "\n\n"
	}

	// TODO: Show actual logs
	s += "Log streaming not yet implemented\n"
	s += "Press ESC to go back\n"

	return s
}

func (m Model) renderDetail() string {
	if m.selectedIdx >= len(m.agents) {
		return "No agent selected"
	}

	agent := m.agents[m.selectedIdx]

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99"))

	var s string
	s += headerStyle.Render(fmt.Sprintf("Agent: %s", agent.ID)) + "\n\n"

	s += labelStyle.Render("State:       ") + agent.State + "\n"
	s += labelStyle.Render("Type:        ") + agent.Type + "\n"
	s += labelStyle.Render("CPU:         ") + agent.CPU + "\n"
	s += labelStyle.Render("Memory:      ") + agent.Memory + "\n"

	if agent.Uptime > 0 {
		s += labelStyle.Render("Uptime:      ") + agent.Uptime.String() + "\n"
	}

	s += "\n"
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	s += footerStyle.Render("[esc] back  [s] start  [x] stop  [r] reload")

	return s
}
