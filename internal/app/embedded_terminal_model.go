package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/handoff"
	"github.com/brianmeier/estuary/internal/prereq"
	"github.com/brianmeier/estuary/internal/providers"
	"github.com/brianmeier/estuary/internal/sessions"
	"github.com/brianmeier/estuary/internal/store"
	termemu "github.com/brianmeier/estuary/internal/terminalemulator"
)

type embeddedOverlay int

const (
	embeddedOverlayNone embeddedOverlay = iota
	embeddedOverlayHelp
	embeddedOverlaySessions
	embeddedOverlayModels
)

type embeddedFrameTickMsg struct{}

type embeddedRuntimeStartedMsg struct {
	session        domain.Session
	sessionList    []domain.Session
	runtime        *embeddedRuntime
	injectionText  string
	injectionDelay time.Duration
	status         string
	err            error
}

type embeddedRuntime struct {
	emulator *termemu.Emulator
	cmd      *exec.Cmd
	recordID string
	closed   bool
}

type EmbeddedTerminalModel struct {
	ctx        context.Context
	cwd        string
	store      *store.Store
	prober     *prereq.Prober
	sessions   *sessions.Service
	handoffSvc *handoff.Service
	adapters   map[domain.Habitat]providers.TerminalAdapter
	trace      *terminalTrace

	theme   Theme
	width   int
	height  int
	status  string
	health  []domain.HabitatHealth
	session domain.Session

	sessionList []domain.Session
	runtime     *embeddedRuntime

	runtimeStarting bool
	overlay         embeddedOverlay
	overlayIndex    int
	leaderActive    bool

	pendingTerminalInput string
	recentTerminalInputs []string
}

func NewEmbeddedTerminalModel(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) (*EmbeddedTerminalModel, error) {
	settings, err := st.LoadAppSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}

	sessionSvc := sessions.NewService(st)
	sessionList, err := sessionSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	trace, err := newTerminalTraceFromEnv()
	if err != nil {
		return nil, err
	}

	return &EmbeddedTerminalModel{
		ctx:        ctx,
		cwd:        cwd,
		store:      st,
		prober:     prober,
		sessions:   sessionSvc,
		handoffSvc: handoff.NewService(st),
		adapters: map[domain.Habitat]providers.TerminalAdapter{
			domain.HabitatClaude: &providers.ClaudeTerminalAdapter{},
			domain.HabitatCodex:  &providers.CodexTerminalAdapter{},
		},
		trace:       trace,
		theme:       ThemeByName(settings.Theme),
		status:      "Starting session...",
		sessionList: sessionList,
	}, nil
}

func RunEmbeddedTerminal(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) error {
	model, err := NewEmbeddedTerminalModel(ctx, cwd, st, prober)
	if err != nil {
		return err
	}
	defer model.shutdownRuntime(true)
	defer func() {
		if model.trace != nil {
			_ = model.trace.Close()
		}
	}()

	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m *EmbeddedTerminalModel) Init() tea.Cmd {
	return tea.Batch(
		m.probeCmd(),
		embeddedFrameTickCmd(),
	)
}

func (m *EmbeddedTerminalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeRuntime()
		if m.runtime == nil && !m.runtimeStarting {
			m.runtimeStarting = true
			return m, m.startInitialRuntimeCmd()
		}
		return m, nil
	case probeResultMsg:
		m.health = msg.Health
		return m, nil
	case embeddedRuntimeStartedMsg:
		m.runtimeStarting = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Start failed: %v", msg.err)
			return m, nil
		}
		m.session = msg.session
		m.sessionList = prependSession(msg.session, msg.sessionList)
		m.runtime = msg.runtime
		m.overlay = embeddedOverlayNone
		m.overlayIndex = 0
		m.status = fallback(msg.status, "Session ready.")
		if msg.injectionText != "" {
			m.injectAfter(msg.runtime, msg.injectionText, msg.injectionDelay)
		}
		return m, nil
	case embeddedFrameTickMsg:
		m.handleRuntimeExit()
		return m, embeddedFrameTickCmd()
	case tea.KeyMsg:
		if m.overlay != embeddedOverlayNone {
			return m.handleOverlayKey(msg)
		}
		if m.leaderActive {
			return m.handleLeaderKey(msg)
		}
		if msg.String() == "ctrl+k" {
			m.leaderActive = true
			m.status = "leader active: ? help · s sessions · m models · r reconnect · q quit · Ctrl+K cancel"
			return m, nil
		}
		if data, ok := encodeTerminalKey(msg); ok {
			m.writeToRuntime(data)
		}
		return m, nil
	}
	return m, nil
}

func (m *EmbeddedTerminalModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing Estuary..."
	}

	termOuter, sideOuter, _, _ := embeddedLayout(m.width, m.height)
	terminal := m.renderTerminalPane(termOuter)
	sidebar := m.renderSidebarPane(sideOuter)

	return lipgloss.NewStyle().
		Background(m.theme.BGCanvas).
		Foreground(m.theme.FGPrimary).
		Width(m.width).
		Height(m.height).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, terminal, sidebar))
}

func (m *EmbeddedTerminalModel) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case embeddedOverlayHelp:
		switch msg.String() {
		case "esc", "enter", "ctrl+k":
			m.overlay = embeddedOverlayNone
			m.status = "Session ready."
		}
		return m, nil
	case embeddedOverlaySessions:
		items := m.sessionItems()
		switch msg.String() {
		case "esc", "ctrl+k":
			m.overlay = embeddedOverlayNone
			m.status = "Session ready."
			return m, nil
		case "up", "k":
			if m.overlayIndex > 0 {
				m.overlayIndex--
			}
			return m, nil
		case "down", "j":
			if m.overlayIndex < len(items)-1 {
				m.overlayIndex++
			}
			return m, nil
		case "enter":
			if len(items) == 0 {
				m.overlay = embeddedOverlayNone
				return m, nil
			}
			target := items[m.overlayIndex]
			m.overlay = embeddedOverlayNone
			return m, m.switchSessionCmd(target)
		}
	case embeddedOverlayModels:
		items := habitats.SupportedModels()
		switch msg.String() {
		case "esc", "ctrl+k":
			m.overlay = embeddedOverlayNone
			m.status = "Session ready."
			return m, nil
		case "up", "k":
			if m.overlayIndex > 0 {
				m.overlayIndex--
			}
			return m, nil
		case "down", "j":
			if m.overlayIndex < len(items)-1 {
				m.overlayIndex++
			}
			return m, nil
		case "enter":
			if len(items) == 0 {
				m.overlay = embeddedOverlayNone
				return m, nil
			}
			m.overlay = embeddedOverlayNone
			return m, m.switchModelCmd(items[m.overlayIndex])
		}
	}

	if data, ok := encodeTerminalKey(msg); ok {
		m.writeToRuntime(data)
	}
	return m, nil
}

func (m *EmbeddedTerminalModel) handleLeaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "ctrl+k", "esc":
		m.leaderActive = false
		m.status = "Session ready."
		return m, nil
	case "?":
		m.leaderActive = false
		m.overlay = embeddedOverlayHelp
		return m, nil
	case "s":
		m.leaderActive = false
		m.overlay = embeddedOverlaySessions
		m.overlayIndex = m.currentSessionIndex()
		m.status = "Select a session."
		return m, nil
	case "m":
		m.leaderActive = false
		m.overlay = embeddedOverlayModels
		m.overlayIndex = m.currentModelIndex()
		m.status = "Select a model."
		return m, nil
	case "r":
		m.leaderActive = false
		return m, m.reconnectCmd()
	case "q":
		m.leaderActive = false
		m.shutdownRuntime(true)
		return m, tea.Quit
	default:
		m.status = "leader active: ? help · s sessions · m models · r reconnect · q quit · Ctrl+K cancel"
		return m, nil
	}
}

func (m *EmbeddedTerminalModel) renderTerminalPane(width int) string {
	style := terminalPaneStyle(m.theme)
	innerWidth := max(1, width-style.GetHorizontalFrameSize())
	innerHeight := max(1, m.height-style.GetVerticalFrameSize())

	var lines []string
	if m.runtime == nil || m.runtime.emulator == nil {
		lines = wrapText(m.status, innerWidth)
	} else {
		lines = append(lines, m.runtime.emulator.GetPlainScreen()...)
	}

	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) > innerWidth {
			lines[i] = string(runes[:innerWidth])
			continue
		}
		lines[i] = line + strings.Repeat(" ", innerWidth-len(runes))
	}
	for len(lines) < innerHeight {
		lines = append(lines, strings.Repeat(" ", innerWidth))
	}

	content := strings.Join(lines, "\n")
	return style.
		Width(innerWidth).
		Height(innerHeight).
		Render(content)
}

func (m *EmbeddedTerminalModel) renderSidebarPane(width int) string {
	style := panelStyle(m.theme, true)
	contentWidth := max(16, width-style.GetHorizontalFrameSize())
	contentHeight := max(1, m.height-style.GetVerticalFrameSize())

	var body string
	switch m.overlay {
	case embeddedOverlayHelp:
		body = m.renderHelpSidebar(contentWidth)
	case embeddedOverlaySessions:
		body = m.renderSessionsSidebar(contentWidth)
	case embeddedOverlayModels:
		body = m.renderModelsSidebar(contentWidth)
	default:
		body = m.renderInfoSidebar(contentWidth)
	}

	return style.
		Width(contentWidth).
		Height(contentHeight).
		Render(body)
}

func (m *EmbeddedTerminalModel) renderInfoSidebar(width int) string {
	accent := lipgloss.NewStyle().Foreground(m.theme.AccentWater).Bold(true)
	muted := mutedStyle(m.theme)

	lines := []string{
		accent.Render("◆ Estuary"),
		"",
		m.panelTitle("Session"),
		fmt.Sprintf("%s", fallback(shortDir(m.session.FolderPath), shortDir(m.cwd))),
		m.displayModelName(),
	}

	if m.leaderActive {
		lines = append(lines, "", m.panelTitle("Leader"))
		lines = append(lines, m.renderLeaderShortcuts()...)
	} else {
		lines = append(lines, "", muted.Render("Ctrl+K  commands"))
	}

	if runtime := m.runtimeNotice(); runtime != "" {
		lines = append(lines, "", m.panelTitle("Runtime"))
		for _, line := range wrapText(runtime, width) {
			lines = append(lines, muted.Render(line))
		}
	}

	if issues := m.providerIssues(); len(issues) > 0 {
		lines = append(lines, "", m.panelTitle("Providers"))
		for _, issue := range issues {
			for _, line := range wrapText(issue, width) {
				lines = append(lines, muted.Render(line))
			}
		}
	}

	if notice := m.sidebarNotice(); notice != "" {
		lines = append(lines, "", m.panelTitle("Notice"))
		for _, line := range wrapText(notice, width) {
			lines = append(lines, muted.Render(line))
		}
	}

	return strings.Join(lines, "\n")
}

func (m *EmbeddedTerminalModel) renderLeaderShortcuts() []string {
	return []string{
		formatShortcut("?", "help"),
		formatShortcut("s", "switch session"),
		formatShortcut("m", "switch model"),
		formatShortcut("r", "reconnect"),
		formatShortcut("q", "quit"),
		formatShortcut("Ctrl+K", "cancel"),
	}
}

func (m *EmbeddedTerminalModel) renderHelpSidebar(width int) string {
	muted := mutedStyle(m.theme)
	lines := []string{
		m.panelTitle("Leader Help"),
		"",
		formatShortcut("Ctrl+K ?", "show help"),
		formatShortcut("Ctrl+K s", "choose another session"),
		formatShortcut("Ctrl+K m", "choose another model"),
		formatShortcut("Ctrl+K r", "restart provider terminal"),
		formatShortcut("Ctrl+K q", "quit Estuary"),
		formatShortcut("Ctrl+K", "cancel leader mode"),
		"",
		muted.Render("Arrow keys, Enter, Tab, Ctrl+C, and normal shell keys go straight to the embedded terminal."),
		"",
		muted.Render("Press Esc to return."),
	}
	return strings.Join(lines, "\n")
}

func (m *EmbeddedTerminalModel) renderSessionsSidebar(width int) string {
	lines := []string{m.panelTitle("Switch Session"), ""}
	items := m.sessionItems()
	if len(items) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No sessions yet."))
	} else {
		for i, session := range items {
			marker := "  "
			if i == m.overlayIndex {
				marker = "▸ "
			}
			label := fmt.Sprintf("%s%s  [%s / %s]", marker, shortDir(session.FolderPath), session.CurrentHabitat, session.CurrentModel)
			if session.ID == m.session.ID {
				label += "  current"
			}
			if i == m.overlayIndex {
				lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.AccentWater).Bold(true).Render(label))
			} else {
				lines = append(lines, label)
			}
		}
	}
	lines = append(lines, "", mutedStyle(m.theme).Render("↑↓ move · enter select · esc cancel"))
	return strings.Join(lines, "\n")
}

func (m *EmbeddedTerminalModel) renderModelsSidebar(width int) string {
	lines := []string{m.panelTitle("Switch Model"), ""}
	items := habitats.SupportedModels()
	if len(items) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No models available."))
	} else {
		for i, model := range items {
			marker := "  "
			if i == m.overlayIndex {
				marker = "▸ "
			}
			label := marker + formatModelDescriptor(model)
			if model.ID == m.session.CurrentModel && model.Habitat == m.session.CurrentHabitat {
				label += "  current"
			} else if model.Habitat == m.session.CurrentHabitat {
				label += "  native"
			} else {
				label += "  handoff"
			}
			if i == m.overlayIndex {
				lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.AccentWater).Bold(true).Render(label))
			} else {
				lines = append(lines, label)
			}
		}
	}
	lines = append(lines, "", mutedStyle(m.theme).Render("same provider: use native picker · other provider: startup handoff"))
	lines = append(lines, mutedStyle(m.theme).Render("↑↓ move · enter select · esc cancel"))
	return strings.Join(lines, "\n")
}

func (m *EmbeddedTerminalModel) runtimeNotice() string {
	if m.runtime == nil && m.runtimeStarting {
		return "Starting provider terminal."
	}
	if m.runtime == nil {
		return ""
	}
	if m.runtime.closed {
		if m.runtime.cmd != nil && m.runtime.cmd.ProcessState != nil {
			return fmt.Sprintf("Provider terminal exited with code %d.", m.runtime.cmd.ProcessState.ExitCode())
		}
		return "Provider terminal is not running."
	}
	return ""
}

func (m *EmbeddedTerminalModel) providerIssues() []string {
	var issues []string
	for _, item := range m.health {
		switch {
		case !item.Installed:
			issues = append(issues, fmt.Sprintf("%s is not installed.", m.formatHabitat(item.Habitat)))
		case !item.Authenticated:
			issues = append(issues, fmt.Sprintf("%s needs authentication.", m.formatHabitat(item.Habitat)))
		}
	}
	return issues
}

func (m *EmbeddedTerminalModel) startInitialRuntimeCmd() tea.Cmd {
	cols, rows := m.currentTerminalSize()
	return func() tea.Msg {
		session, err := m.findOrCreateSession()
		if err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		if err := m.store.TouchSession(m.ctx, session.ID); err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		runtime, err := launchEmbeddedRuntime(m.ctx, m.store, m.adapters, session, cols, rows, domain.AttachStrategyFresh, "", "")
		if err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		sessionList, listErr := m.sessions.List(m.ctx)
		if listErr != nil {
			return embeddedRuntimeStartedMsg{err: listErr}
		}
		return embeddedRuntimeStartedMsg{
			session:     session,
			sessionList: sessionList,
			runtime:     runtime,
			status:      "Session ready.",
		}
	}
}

func (m *EmbeddedTerminalModel) reconnectCmd() tea.Cmd {
	if m.session.ID == "" {
		return nil
	}
	session := m.session
	m.status = "Reconnecting..."
	m.shutdownRuntime(true)
	cols, rows := m.currentTerminalSize()
	return func() tea.Msg {
		if err := m.store.TouchSession(m.ctx, session.ID); err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		runtime, err := launchEmbeddedRuntime(m.ctx, m.store, m.adapters, session, cols, rows, domain.AttachStrategyFresh, "", "")
		if err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		sessionList, listErr := m.sessions.List(m.ctx)
		if listErr != nil {
			return embeddedRuntimeStartedMsg{err: listErr}
		}
		return embeddedRuntimeStartedMsg{
			session:     session,
			sessionList: sessionList,
			runtime:     runtime,
			status:      "Reconnected.",
		}
	}
}

func (m *EmbeddedTerminalModel) switchSessionCmd(target domain.Session) tea.Cmd {
	if target.ID == "" || target.ID == m.session.ID {
		m.status = "Session ready."
		return nil
	}

	m.status = fmt.Sprintf("Switching to %s...", shortDir(target.FolderPath))
	m.shutdownRuntime(true)
	cols, rows := m.currentTerminalSize()

	return func() tea.Msg {
		if err := m.store.TouchSession(m.ctx, target.ID); err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		runtime, err := launchEmbeddedRuntime(m.ctx, m.store, m.adapters, target, cols, rows, domain.AttachStrategyFresh, "", "")
		if err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		sessionList, listErr := m.sessions.List(m.ctx)
		if listErr != nil {
			return embeddedRuntimeStartedMsg{err: listErr}
		}
		return embeddedRuntimeStartedMsg{
			session:     target,
			sessionList: sessionList,
			runtime:     runtime,
			status:      fmt.Sprintf("Switched to %s.", shortDir(target.FolderPath)),
		}
	}
}

func (m *EmbeddedTerminalModel) switchModelCmd(target domain.ModelDescriptor) tea.Cmd {
	if m.session.ID == "" {
		return nil
	}
	if target.ID == m.session.CurrentModel && target.Habitat == m.session.CurrentHabitat {
		m.status = "That model is already active."
		return nil
	}

	if target.Habitat == m.session.CurrentHabitat {
		m.status = fmt.Sprintf("Use %s's native model picker for %s. Handoff only runs across providers.", m.formatHabitat(target.Habitat), formatModelDescriptor(target))
		if m.store != nil {
			_ = m.store.AppendEvent(m.ctx, m.session.ID, "model.native_switch_requested", map[string]any{
				"provider":     target.Habitat,
				"target_model": target.ID,
			})
		}
		return nil
	}

	packet, err := m.handoffSvc.GenerateWithTerminalSnapshot(
		m.ctx,
		m.session,
		target.ID,
		target.Habitat,
		domain.SwitchTypeCrossProvider,
		m.handoffContextLines(),
	)
	if err != nil {
		_ = m.store.AppendEvent(m.ctx, m.session.ID, "handoff.error", map[string]any{"error": err.Error()})
	}

	session := m.session
	session.CurrentModel = target.ID
	session.CurrentHabitat = target.Habitat
	session.NativeSessionID = ""

	if err := m.store.UpdateSession(m.ctx, session); err != nil {
		m.status = fmt.Sprintf("Update model failed: %v", err)
		return nil
	}

	m.session = session
	m.status = fmt.Sprintf("Switching to %s...", target.ID)
	m.shutdownRuntime(true)
	cols, rows := m.currentTerminalSize()

	handoffText := ""
	if packet.ID != "" {
		handoffText = handoff.InjectionText(packet)
	}

	return func() tea.Msg {
		runtime, err := launchEmbeddedRuntime(m.ctx, m.store, m.adapters, session, cols, rows, domain.AttachStrategyHandoff, packet.ID, handoffText)
		if err != nil {
			return embeddedRuntimeStartedMsg{err: err}
		}
		sessionList, listErr := m.sessions.List(m.ctx)
		if listErr != nil {
			return embeddedRuntimeStartedMsg{err: listErr}
		}
		if packet.ID != "" {
			_ = m.store.AppendEvent(m.ctx, session.ID, "handoff.injected", map[string]any{
				"packet_id":   packet.ID,
				"switch_type": string(domain.SwitchTypeCrossProvider),
				"mode":        "startup_context",
			})
		}
		return embeddedRuntimeStartedMsg{
			session:     session,
			sessionList: sessionList,
			runtime:     runtime,
			status:      fmt.Sprintf("Switched to %s with startup handoff. You can type immediately.", target.ID),
		}
	}
}

func (m *EmbeddedTerminalModel) injectAfter(runtime *embeddedRuntime, text string, delay time.Duration) {
	if runtime == nil || runtime.emulator == nil || strings.TrimSpace(text) == "" {
		return
	}
	go func(current *embeddedRuntime) {
		time.Sleep(delay)
		if m.runtime != current || current.closed {
			return
		}
		_, _ = current.emulator.Write([]byte(text + "\n"))
	}(runtime)
}

func (m *EmbeddedTerminalModel) terminalSnapshot() []string {
	if m.runtime == nil || m.runtime.closed || m.runtime.emulator == nil {
		return nil
	}
	return m.runtime.emulator.GetPlainScreen()
}

func (m *EmbeddedTerminalModel) handoffContextLines() []string {
	var lines []string
	if len(m.recentTerminalInputs) > 0 || strings.TrimSpace(m.pendingTerminalInput) != "" {
		lines = append(lines, "Recent user terminal input:")
		for _, input := range m.recentTerminalInputs {
			lines = append(lines, "- "+input)
		}
		if pending := strings.TrimSpace(m.pendingTerminalInput); pending != "" {
			lines = append(lines, "- "+pending+" [unsubmitted]")
		}
	}
	if snapshot := m.terminalSnapshot(); len(snapshot) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "Visible terminal:")
		lines = append(lines, snapshot...)
	}
	return lines
}

func (m *EmbeddedTerminalModel) handleRuntimeExit() {
	if m.runtime == nil || m.runtime.closed || m.runtime.emulator == nil || !m.runtime.emulator.IsProcessExited() {
		return
	}

	exitCode := 0
	if m.runtime.cmd != nil && m.runtime.cmd.ProcessState != nil {
		exitCode = m.runtime.cmd.ProcessState.ExitCode()
	}

	m.closeRuntimeRecord(exitCode)
	m.status = fmt.Sprintf("Session ended (exit %d) · Ctrl+K r reconnect · Ctrl+K q quit", exitCode)
}

func (m *EmbeddedTerminalModel) shutdownRuntime(kill bool) {
	if m.runtime == nil {
		return
	}
	m.closeRuntimeRecord(0)
	if kill && m.runtime.cmd != nil && m.runtime.cmd.Process != nil && m.runtime.cmd.ProcessState == nil {
		_ = m.runtime.cmd.Process.Kill()
	}
	if m.runtime.emulator != nil {
		_ = m.runtime.emulator.Close()
	}
}

func (m *EmbeddedTerminalModel) closeRuntimeRecord(exitCode int) {
	if m.runtime == nil || m.runtime.closed {
		return
	}
	if m.runtime.recordID != "" {
		_ = m.store.ClosePTYSession(m.ctx, m.runtime.recordID, exitCode)
	}
	if m.session.ID != "" {
		_ = m.store.UpdateSessionStatus(m.ctx, m.session.ID, domain.SessionStatusIdle, m.session.NativeSessionID)
	}
	m.runtime.closed = true
}

func (m *EmbeddedTerminalModel) resizeRuntime() {
	if m.runtime == nil || m.runtime.emulator == nil {
		return
	}
	cols, rows := m.currentTerminalSize()
	_ = m.runtime.emulator.Resize(cols, rows)
}

func (m *EmbeddedTerminalModel) writeToRuntime(data []byte) {
	if len(data) == 0 || m.runtime == nil || m.runtime.closed || m.runtime.emulator == nil {
		return
	}
	m.recordTerminalInput(data)
	if m.trace != nil {
		m.trace.Logf("stdin forwarded=%s", traceBytes(data))
	}
	_, _ = m.runtime.emulator.Write(data)
}

func (m *EmbeddedTerminalModel) recordTerminalInput(data []byte) {
	for _, b := range data {
		switch {
		case b == '\r' || b == '\n':
			m.commitTerminalInput()
		case b == 0x7f || b == '\b':
			m.pendingTerminalInput = trimLastRune(m.pendingTerminalInput)
		case b == '\t':
			m.pendingTerminalInput += "\t"
		case b >= 0x20 && b != 0x7f:
			m.pendingTerminalInput += string(rune(b))
		default:
			// Ignore control and escape sequences. They are terminal navigation,
			// not durable user context for a provider handoff.
		}
	}
	if len([]rune(m.pendingTerminalInput)) > 1000 {
		runes := []rune(m.pendingTerminalInput)
		m.pendingTerminalInput = string(runes[len(runes)-1000:])
	}
}

func (m *EmbeddedTerminalModel) commitTerminalInput() {
	line := strings.TrimSpace(m.pendingTerminalInput)
	m.pendingTerminalInput = ""
	if line == "" {
		return
	}
	m.recentTerminalInputs = append(m.recentTerminalInputs, line)
	if len(m.recentTerminalInputs) > 12 {
		m.recentTerminalInputs = m.recentTerminalInputs[len(m.recentTerminalInputs)-12:]
	}
}

func (m *EmbeddedTerminalModel) findOrCreateSession() (domain.Session, error) {
	existing, found, err := m.sessions.FindForFolder(m.ctx, m.cwd)
	if err != nil {
		return domain.Session{}, err
	}
	if found {
		return existing, nil
	}
	session, _, err := m.sessions.CreateCurrent(m.ctx, m.cwd)
	return session, err
}

func (m *EmbeddedTerminalModel) probeCmd() tea.Cmd {
	return func() tea.Msg {
		return probeResultMsg{Health: m.prober.ProbeAll(m.ctx)}
	}
}

func (m *EmbeddedTerminalModel) sessionItems() []domain.Session {
	return m.sessionList
}

func (m *EmbeddedTerminalModel) currentSessionIndex() int {
	for i, session := range m.sessionList {
		if session.ID == m.session.ID {
			return i
		}
	}
	return 0
}

func (m *EmbeddedTerminalModel) currentModelIndex() int {
	models := habitats.SupportedModels()
	for i, model := range models {
		if model.ID == m.session.CurrentModel && model.Habitat == m.session.CurrentHabitat {
			return i
		}
	}
	return 0
}

func (m *EmbeddedTerminalModel) currentTerminalSize() (cols, rows int) {
	if m.width == 0 || m.height == 0 {
		return 80, 24
	}
	_, _, cols, rows = embeddedLayout(m.width, m.height)
	return cols, rows
}

func embeddedFrameTickCmd() tea.Cmd {
	return tea.Tick(time.Second/30, func(time.Time) tea.Msg {
		return embeddedFrameTickMsg{}
	})
}

func embeddedLayout(width, height int) (termOuter, sideOuter, termCols, termRows int) {
	sideOuter = 32
	if width >= 140 {
		sideOuter = 36
	}
	if width <= 100 {
		sideOuter = 28
	}
	if maxSide := max(24, width/3); sideOuter > maxSide {
		sideOuter = maxSide
	}
	termOuter = width - sideOuter
	if termOuter < 24 {
		termOuter = max(16, width-24)
		sideOuter = max(8, width-termOuter)
	}
	termCols = max(8, termOuter-2)
	termRows = max(4, height-2)
	return termOuter, sideOuter, termCols, termRows
}

func terminalPaneStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderSoft).
		Background(theme.BGSurface)
}

func formatShortcut(keys, action string) string {
	return fmt.Sprintf("%-8s %s", keys, action)
}

func (m *EmbeddedTerminalModel) displayModelName() string {
	return friendlyModelLabel(m.session.CurrentModel)
}

func (m *EmbeddedTerminalModel) displayHabitatName() string {
	return m.formatHabitat(m.session.CurrentHabitat)
}

func (m *EmbeddedTerminalModel) formatHabitat(h domain.Habitat) string {
	raw := strings.TrimSpace(string(h))
	if raw == "" {
		return "Unknown"
	}
	return strings.ToUpper(raw[:1]) + raw[1:]
}

func (m *EmbeddedTerminalModel) sidebarNotice() string {
	notice := strings.TrimSpace(m.status)
	switch notice {
	case "", "Session ready.", "Starting session...", "Select a session.", "Select a model.":
		return ""
	}
	if strings.HasPrefix(notice, "leader active:") {
		return ""
	}
	return notice
}

func humanizeModelName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Default model"
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		switch lower {
		case "gpt":
			parts[i] = "GPT"
		case "claude":
			parts[i] = "Claude"
		case "codex":
			parts[i] = "Codex"
		default:
			if unicode.IsDigit(rune(part[0])) {
				parts[i] = strings.ReplaceAll(part, "-", ".")
			} else {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			}
		}
	}
	return strings.Join(parts, " ")
}

func formatModelDescriptor(model domain.ModelDescriptor) string {
	label := strings.TrimSpace(model.Label)
	if label != "" && label != model.ID {
		return label
	}
	return friendlyModelLabel(model.ID)
}

func friendlyModelLabel(modelID string) string {
	if label := habitats.SupportedModelLabel(strings.TrimSpace(modelID)); label != "" {
		return label
	}
	return humanizeModelName(modelID)
}

func (m *EmbeddedTerminalModel) panelTitle(s string) string {
	return lipgloss.NewStyle().Foreground(m.theme.AccentClay).Bold(true).Render(s)
}

func launchEmbeddedRuntime(
	ctx context.Context,
	st *store.Store,
	adapters map[domain.Habitat]providers.TerminalAdapter,
	session domain.Session,
	cols int,
	rows int,
	strategy domain.AttachStrategy,
	handoffPacketID string,
	handoffText string,
) (*embeddedRuntime, error) {
	adapter, ok := adapters[session.CurrentHabitat]
	if !ok {
		return nil, fmt.Errorf("no terminal adapter for provider %q", session.CurrentHabitat)
	}

	strategyUsed := strategy
	var command string
	var args []string
	var env []string

	switch strategy {
	case domain.AttachStrategyHandoff:
		command, args, env = adapter.HandoffArgs(session, handoffText)
	case domain.AttachStrategyResume:
		if session.NativeSessionID != "" {
			command, args, env = adapter.ResumeArgs(session, session.NativeSessionID)
		} else {
			command, args, env = adapter.StartArgs(session)
			strategyUsed = domain.AttachStrategyFresh
		}
	default:
		if session.NativeSessionID != "" {
			command, args, env = adapter.ResumeArgs(session, session.NativeSessionID)
			strategyUsed = domain.AttachStrategyResume
		} else {
			command, args, env = adapter.StartArgs(session)
		}
	}

	emu, err := termemu.New(cols, rows)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), env...)
	if session.FolderPath != "" {
		cmd.Dir = session.FolderPath
	}

	if err := emu.StartCommand(cmd); err != nil {
		_ = emu.Close()
		return nil, err
	}

	_ = st.TouchSession(ctx, session.ID)

	recordID := uuid.NewString()
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	_ = st.SavePTYSession(ctx, domain.TerminalSession{
		ID:              recordID,
		SessionID:       session.ID,
		Provider:        session.CurrentHabitat,
		PID:             pid,
		AttachStrategy:  strategyUsed,
		NativeSessionID: session.NativeSessionID,
		HandoffPacketID: handoffPacketID,
		Status:          domain.ProviderRuntimeStatusRunning,
		StartedAt:       time.Now().UTC(),
	})

	return &embeddedRuntime{
		emulator: emu,
		cmd:      cmd,
		recordID: recordID,
	}, nil
}
