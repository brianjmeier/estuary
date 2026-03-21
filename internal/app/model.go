package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brianmeier/estuary/internal/boundaries"
	"github.com/brianmeier/estuary/internal/chat"
	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/migration"
	"github.com/brianmeier/estuary/internal/prereq"
	"github.com/brianmeier/estuary/internal/sessions"
	"github.com/brianmeier/estuary/internal/store"
	"github.com/brianmeier/estuary/internal/traits"
)

type centerView int
type modalMode int
type traitField int

const (
	viewChat centerView = iota
	viewSettings
	viewHelp
)

const (
	modalNone modalMode = iota
	modalPalette
	modalMigration
	modalBoundary
	modalTraitEditor
)

const (
	traitName traitField = iota
	traitDescription
	traitDefinition
	traitCount
)

type probeResultMsg struct {
	Health []domain.HabitatHealth
}

type sessionCreatedMsg struct {
	Session       domain.Session
	Resolution    domain.BoundaryResolution
	ExistingCount int
	Err           error
}

type messagesLoadedMsg struct {
	SessionID string
	Messages  []domain.Message
	Err       error
}

type runtimeStateLoadedMsg struct {
	SessionID string
	State     domain.SessionRuntimeState
	Err       error
}

type traitsLoadedMsg struct {
	Traits []domain.Trait
	Err    error
}

type streamMsg struct {
	Envelope chat.StreamEnvelope
	StreamID int
	Closed   bool
}

type operationMsg struct {
	Session domain.Session
	Err     error
	Status  string
}

type composeState struct {
	Text string
}

type paletteEntry struct {
	Label     string
	Hint      string
	Kind      string
	SessionID string
}

type paletteState struct {
	Query     string
	Selection int
}

type migrationState struct {
	Model      string
	ModelIndex int
}

type boundaryState struct {
	ProfileIndex int
}

type traitEditorState struct {
	ID             string
	Field          traitField
	Name           string
	Description    string
	Definition     string
	Type           domain.TraitType
	SupportsClaude bool
	SupportsCodex  bool
}

type Model struct {
	ctx           context.Context
	cwd           string
	store         *store.Store
	prober        *prereq.Prober
	sessions      *sessions.Service
	chat          *chat.Service
	migration     *migration.Service
	traits        *traits.Service
	width         int
	height        int
	theme         Theme
	settings      domain.AppSettings
	center        centerView
	modal         modalMode
	sessionIx     int
	sessionList   []domain.Session
	messages      []domain.Message
	health        []domain.HabitatHealth
	profiles      []domain.BoundaryProfile
	traitList     []domain.Trait
	runtimeStates map[string]domain.SessionRuntimeState
	streams       map[int]<-chan chat.StreamEnvelope
	compose       composeState
	palette       paletteState
	migrationUI   migrationState
	boundaryUI    boundaryState
	traitEditor   traitEditorState
	status        string
	streamID      int
}

func NewModel(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) (Model, error) {
	settings, err := st.LoadAppSettings(ctx)
	if err != nil {
		return Model{}, err
	}
	sessionSvc := sessions.NewService(st)
	chatSvc := chat.NewService(st, habitats.NewRuntime())
	sessionList, err := sessionSvc.List(ctx)
	if err != nil {
		return Model{}, err
	}

	return Model{
		ctx:           ctx,
		cwd:           cwd,
		store:         st,
		prober:        prober,
		sessions:      sessionSvc,
		chat:          chatSvc,
		migration:     migration.NewService(st),
		traits:        traits.NewService(st),
		theme:         ThemeByName(settings.Theme),
		settings:      settings,
		center:        viewChat,
		sessionList:   sessionList,
		profiles:      boundaries.DefaultProfiles(),
		runtimeStates: map[string]domain.SessionRuntimeState{},
		streams:       map[int]<-chan chat.StreamEnvelope{},
		traitEditor: traitEditorState{
			Type:           domain.TraitTypeCommand,
			SupportsClaude: true,
			SupportsCodex:  true,
		},
		status: "Creating a new session in the current directory.",
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return probeResultMsg{Health: m.prober.ProbeAll(m.ctx)} },
		func() tea.Msg {
			items, err := m.traits.List(m.ctx)
			return traitsLoadedMsg{Traits: items, Err: err}
		},
		func() tea.Msg {
			_ = m.store.MarkAllSessionsInactive(m.ctx)
			session, resolution, count, err := m.sessions.CreateCurrent(m.ctx, m.cwd, m.profiles)
			return sessionCreatedMsg{Session: session, Resolution: resolution, ExistingCount: count, Err: err}
		},
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case probeResultMsg:
		m.health = msg.Health
		for _, item := range msg.Health {
			_ = m.store.UpsertEcosystemSnapshot(m.ctx, item)
		}
	case traitsLoadedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Traits load failed: %v", msg.Err)
		} else {
			m.traitList = msg.Traits
		}
	case sessionCreatedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Create session failed: %v", msg.Err)
			return m, nil
		}
		m.sessionList = append([]domain.Session{msg.Session}, m.sessionList...)
		m.sessionIx = 0
		m.runtimeStates[msg.Session.ID] = domain.SessionRuntimeState{Active: true, FirstRunCompleted: true}
		if msg.ExistingCount >= 2 {
			m.status = fmt.Sprintf("Warning: %d active sessions already target %s.", msg.ExistingCount, msg.Session.FolderPath)
		} else {
			m.status = fmt.Sprintf("New session ready in %s on %s.", msg.Session.FolderPath, msg.Session.CurrentModel)
		}
		return m, tea.Batch(m.loadMessagesCmd(msg.Session.ID), m.loadRuntimeStateCmd(msg.Session.ID))
	case messagesLoadedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Load messages failed: %v", msg.Err)
			return m, nil
		}
		if selected, ok := m.selectedSession(); ok && selected.ID == msg.SessionID {
			m.messages = msg.Messages
		}
	case runtimeStateLoadedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Load runtime state failed: %v", msg.Err)
			return m, nil
		}
		m.runtimeStates[msg.SessionID] = msg.State
	case streamMsg:
		if msg.Closed {
			delete(m.streams, msg.StreamID)
			return m, nil
		}
		m.applyStreamEnvelope(msg.Envelope)
		if msg.Envelope.Done {
			delete(m.streams, msg.StreamID)
			return m, nil
		}
		return m, m.waitStreamCmd(msg.StreamID)
	case operationMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("%s: %v", fallback(msg.Status, "Operation failed"), msg.Err)
			return m, nil
		}
		if msg.Session.ID != "" {
			m.replaceSession(msg.Session)
			return m, tea.Batch(m.loadMessagesCmd(msg.Session.ID), m.loadRuntimeStateCmd(msg.Session.ID))
		}
		m.status = msg.Status
	case tea.KeyMsg:
		switch m.modal {
		case modalPalette:
			return m.handlePaletteKey(msg)
		case modalMigration:
			return m.handleMigrationKey(msg)
		case modalBoundary:
			return m.handleBoundaryKey(msg)
		case modalTraitEditor:
			return m.handleTraitEditorKey(msg)
		}
		return m.handleMainKey(msg)
	}
	return m, nil
}

func (m Model) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if session, ok := m.selectedSession(); ok {
			state := m.runtimeStates[session.ID]
			state.Active = false
			_ = m.store.SaveSessionRuntimeState(m.ctx, session.ID, state)
			_ = m.store.AppendEvent(m.ctx, session.ID, "session.closed", map[string]any{"closed_at": time.Now().UTC()})
		}
		return m, tea.Quit
	case "ctrl+k":
		m.modal = modalPalette
		m.palette = paletteState{}
		m.status = "Command palette opened."
	case "enter":
		session, ok := m.selectedSession()
		if !ok {
			m.status = "No session selected."
			return m, nil
		}
		prompt := strings.TrimSpace(m.compose.Text)
		if prompt == "" {
			return m, nil
		}
		trait, resolvedPrompt, err := m.traits.ResolveCommand(m.ctx, session, prompt)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		if trait.ID != "" {
			m.status = fmt.Sprintf("Invoking trait %s.", trait.Name)
		}
		m.compose.Text = ""
		m.streamID++
		ch := make(chan chat.StreamEnvelope, 32)
		streamID := m.streamID
		m.streams[streamID] = ch
		go func() {
			_ = m.chat.SendStream(m.ctx, session, resolvedPrompt, func(item chat.StreamEnvelope) error {
				ch <- item
				return nil
			})
			close(ch)
		}()
		return m, m.waitStreamCmd(streamID)
	case "backspace":
		m.compose.Text = trimLastRune(m.compose.Text)
	case "esc":
		m.compose.Text = ""
		m.status = "Composer cleared."
	default:
		if msg.Type == tea.KeyRunes {
			m.compose.Text += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+k":
		m.modal = modalNone
		m.status = "Palette closed."
	case "backspace":
		m.palette.Query = trimLastRune(m.palette.Query)
		m.palette.Selection = 0
	case "up", "k":
		entries := m.filteredPaletteEntries()
		if len(entries) > 0 && m.palette.Selection > 0 {
			m.palette.Selection--
		}
	case "down", "j":
		entries := m.filteredPaletteEntries()
		if len(entries) > 0 && m.palette.Selection < len(entries)-1 {
			m.palette.Selection++
		}
	case "enter":
		entries := m.filteredPaletteEntries()
		if len(entries) == 0 {
			return m, nil
		}
		return m.executePaletteEntry(entries[m.palette.Selection])
	default:
		if msg.Type == tea.KeyRunes {
			m.palette.Query += string(msg.Runes)
			m.palette.Selection = 0
		}
	}
	return m, nil
}

func (m Model) handleMigrationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
	case "up", "k":
		m.migrationUI.ModelIndex = m.cycleIndex(m.migrationUI.ModelIndex-1, len(m.availableModels()))
		m.migrationUI.Model = m.selectedModelOption(m.migrationUI.ModelIndex)
	case "down", "j":
		m.migrationUI.ModelIndex = m.cycleIndex(m.migrationUI.ModelIndex+1, len(m.availableModels()))
		m.migrationUI.Model = m.selectedModelOption(m.migrationUI.ModelIndex)
	case "backspace":
		m.migrationUI.Model = trimLastRune(m.migrationUI.Model)
	case "enter":
		session, ok := m.selectedSession()
		if !ok {
			return m, nil
		}
		model := strings.TrimSpace(m.migrationUI.Model)
		if model == "" {
			m.status = "Target model is required."
			return m, nil
		}
		m.modal = modalNone
		return m, func() tea.Msg { return m.performMigration(session, model) }
	default:
		if msg.Type == tea.KeyRunes {
			m.migrationUI.Model += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) handleBoundaryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
	case "up", "k":
		m.boundaryUI.ProfileIndex = m.cycleIndex(m.boundaryUI.ProfileIndex-1, len(m.profiles))
	case "down", "j":
		m.boundaryUI.ProfileIndex = m.cycleIndex(m.boundaryUI.ProfileIndex+1, len(m.profiles))
	case "enter":
		session, ok := m.selectedSession()
		if !ok {
			return m, nil
		}
		profile := m.profiles[m.boundaryUI.ProfileIndex]
		m.modal = modalNone
		return m, func() tea.Msg { return m.performBoundaryChange(session, profile) }
	}
	return m, nil
}

func (m Model) handleTraitEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
	case "tab":
		m.traitEditor.Field = (m.traitEditor.Field + 1) % traitCount
	case "shift+tab":
		m.traitEditor.Field = (m.traitEditor.Field + traitCount - 1) % traitCount
	case "backspace":
		m.backspaceTraitField()
	case "ctrl+t":
		switch m.traitEditor.Type {
		case domain.TraitTypeCommand:
			m.traitEditor.Type = domain.TraitTypeSkill
		case domain.TraitTypeSkill:
			m.traitEditor.Type = domain.TraitTypeTool
		default:
			m.traitEditor.Type = domain.TraitTypeCommand
		}
	case "y":
		m.traitEditor.SupportsClaude = !m.traitEditor.SupportsClaude
	case "x":
		m.traitEditor.SupportsCodex = !m.traitEditor.SupportsCodex
	case "enter":
		editor := m.traitEditor
		m.modal = modalNone
		return m, func() tea.Msg {
			if _, err := m.traits.Save(m.ctx, domain.Trait{
				ID:             editor.ID,
				Type:           editor.Type,
				Name:           editor.Name,
				Description:    editor.Description,
				CanonicalDef:   editor.Definition,
				Scope:          domain.TraitScopeShared,
				SupportsClaude: editor.SupportsClaude,
				SupportsCodex:  editor.SupportsCodex,
				SyncMode:       domain.TraitSyncBootstrap,
				DispatchMode:   dispatchModeFor(editor.Type, editor.SupportsClaude, editor.SupportsCodex),
			}); err != nil {
				return operationMsg{Err: err, Status: "Save trait failed"}
			}
			items, listErr := m.traits.List(m.ctx)
			if listErr == nil {
				return traitsLoadedMsg{Traits: items}
			}
			return operationMsg{Err: listErr, Status: "Reload traits failed"}
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.appendTraitField(string(msg.Runes))
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing Estuary..."
	}

	header := lipgloss.NewStyle().Foreground(m.theme.AccentWater).Bold(true).Render("Estuary")
	header += "  " + lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render(m.headerSubtitle())
	hints := lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render("Enter Send  Ctrl+K Palette  Esc Clear  Ctrl+C Quit")

	contentW := max(60, m.width-4)
	composer := m.renderComposerDock(contentW)
	composerHeight := lipgloss.Height(composer)
	mainHeight := max(10, m.height-5-composerHeight)
	body := m.renderMainPanel(contentW, mainHeight)

	layout := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		hints,
		body,
		composer,
		lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render(m.status),
	)
	if m.modal != modalNone {
		layout = lipgloss.JoinVertical(lipgloss.Left, layout, m.renderModal(max(76, m.width-2)))
	}

	return lipgloss.NewStyle().
		Foreground(m.theme.FGPrimary).
		Width(m.width).
		Height(m.height).
		Render(layout)
}

func (m Model) renderMainPanel(width, height int) string {
	title, content := "Chat", m.renderChat(width-4, height-3)
	switch m.center {
	case viewSettings:
		title, content = "Settings", m.renderSettings()
	case viewHelp:
		title, content = "Help", m.renderHelp()
	}
	return panelStyle(m.theme, true).Width(width).Height(height).Render(m.panelTitle(title) + "\n" + content)
}

func (m Model) renderChat(width, height int) string {
	session, ok := m.selectedSession()
	if !ok {
		return mutedStyle(m.theme).Render("Starting a new session in the current directory...")
	}

	header := []string{
		fmt.Sprintf("%s  [%s]  %s", session.Title, session.CurrentModel, session.FolderPath),
		fmt.Sprintf("Resume: %s  Boundary: %s  Habitat: %s", m.resumeLabel(session, m.runtimeStates[session.ID]), session.BoundaryProfile, session.CurrentHabitat),
		"",
	}
	transcriptHeight := max(4, height-3)
	transcript := m.transcriptLines(width)
	if len(transcript) > transcriptHeight {
		transcript = transcript[len(transcript)-transcriptHeight:]
	}
	if len(transcript) == 0 {
		transcript = []string{
			mutedStyle(m.theme).Render("Start typing to talk to Estuary."),
			"",
			mutedStyle(m.theme).Render("Ctrl+K opens sessions, settings, and model controls."),
		}
	}

	lines := append(header, transcript...)
	return strings.Join(lines, "\n")
}

func (m Model) renderComposer(width int) string {
	lines := composerLines(m.compose.Text, width)
	if len(lines) == 0 {
		lines = []string{mutedStyle(m.theme).Render("Message the current session...")}
	}
	scrollHint := ""
	if len(lines) > 8 {
		scrollHint = lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render("  scroll")
		lines = lines[len(lines)-8:]
	}
	body := append([]string{"› " + lines[0] + scrollHint}, prefixLines(lines[1:], "  ")...)
	return strings.Join(body, "\n")
}

func (m Model) renderComposerDock(width int) string {
	content := m.renderComposer(width - 4)
	help := lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render("? for shortcuts")
	return panelStyle(m.theme, true).
		Width(width).
		Render(m.panelTitle("Input") + "\n" + content + "\n" + help)
}

func (m Model) transcriptLines(width int) []string {
	var out []string
	for _, message := range m.messages {
		label := strings.ToUpper(string(message.Role))
		switch message.Role {
		case domain.MessageRoleSummary:
			label = "MIGRATION"
		case domain.MessageRoleTool:
			label = "TOOL"
		}
		out = append(out, lipgloss.NewStyle().Foreground(m.theme.AccentClay).Bold(true).Render(label))
		out = append(out, wrapText(message.Content, width)...)
		out = append(out, "")
	}
	return out
}

func (m Model) renderSettings() string {
	session, ok := m.selectedSession()
	if !ok {
		return "No session selected."
	}
	lines := []string{
		"Current session",
		fmt.Sprintf("Directory: %s", session.FolderPath),
		fmt.Sprintf("Model: %s", session.CurrentModel),
		fmt.Sprintf("Boundary: %s", session.BoundaryProfile),
		fmt.Sprintf("Habitat: %s", session.CurrentHabitat),
		"",
		"Recent sessions",
	}
	for i, item := range m.sessionList {
		if i >= 6 {
			break
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s", item.Title, item.CurrentModel, item.FolderPath))
	}
	lines = append(lines, "", "Use Ctrl+K to switch sessions, change model, change boundaries, or open traits.")
	return strings.Join(lines, "\n")
}

func (m Model) renderHelp() string {
	return strings.Join([]string{
		"Enter send message",
		"Ctrl+K open command palette",
		"Esc clear composer or close modal",
		"Ctrl+C quit",
		"",
		"Palette entries handle sessions, settings, migration, boundaries, traits, and re-probing.",
	}, "\n")
}

func (m Model) renderModal(width int) string {
	switch m.modal {
	case modalPalette:
		return m.renderPalette(width)
	case modalMigration:
		return m.renderMigration(width)
	case modalBoundary:
		return m.renderBoundary(width)
	case modalTraitEditor:
		return m.renderTraitEditor(width)
	default:
		return ""
	}
}

func (m Model) renderPalette(width int) string {
	entries := m.filteredPaletteEntries()
	lines := []string{"Query: " + fallback(m.palette.Query, "(empty)"), ""}
	if len(entries) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No matches."))
	} else {
		for i, entry := range entries {
			prefix := "  "
			if i == m.palette.Selection {
				prefix = "> "
			}
			line := prefix + entry.Label
			if entry.Hint != "" {
				line += "  " + lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render(entry.Hint)
			}
			if i == m.palette.Selection {
				line = lipgloss.NewStyle().Foreground(m.theme.AccentWater).Bold(true).Render(line)
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "", "Type to filter. Enter selects. Esc closes.")
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Command Palette") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderMigration(width int) string {
	lines := []string{
		"Choose the next model for this session.",
		"",
		"Model: " + fallback(m.migrationUI.Model, m.selectedModelOption(m.migrationUI.ModelIndex)),
		"",
		"Available models: " + fallback(strings.Join(m.availableModels(), ", "), "manual entry enabled"),
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Change Model") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderBoundary(width int) string {
	profile := m.profiles[m.boundaryUI.ProfileIndex]
	session, _ := m.selectedSession()
	resolution := boundaries.Resolve(profile, session.CurrentHabitat)
	lines := []string{
		fmt.Sprintf("Profile: %s", profile.Name),
		profile.Description,
		fmt.Sprintf("Compatibility: %s", strings.ToUpper(string(resolution.Compatibility))),
		"Summary: " + resolution.Summary,
	}
	if profile.Unsafe {
		lines = append(lines, "Unsafe: yes")
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Boundary") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderTraitEditor(width int) string {
	lines := []string{
		m.renderTraitField("Name", m.traitEditor.Name, m.traitEditor.Field == traitName),
		m.renderTraitField("Description", m.traitEditor.Description, m.traitEditor.Field == traitDescription),
		m.renderTraitField("Definition", m.traitEditor.Definition, m.traitEditor.Field == traitDefinition),
		"",
		fmt.Sprintf("Type: %s", m.traitEditor.Type),
		fmt.Sprintf("Supports Claude: %t", m.traitEditor.SupportsClaude),
		fmt.Sprintf("Supports Codex: %t", m.traitEditor.SupportsCodex),
		"",
		"Tab cycles fields. y toggles Claude. x toggles Codex. Ctrl+T cycles type.",
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Trait Editor") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) filteredPaletteEntries() []paletteEntry {
	query := strings.ToLower(strings.TrimSpace(m.palette.Query))
	all := m.paletteEntries()
	if query == "" {
		return all
	}
	var out []paletteEntry
	for _, entry := range all {
		haystack := strings.ToLower(entry.Label + " " + entry.Hint)
		if strings.Contains(haystack, query) {
			out = append(out, entry)
		}
	}
	return out
}

func (m Model) paletteEntries() []paletteEntry {
	entries := []paletteEntry{
		{Label: "New Session Here", Hint: m.cwd, Kind: "new"},
		{Label: "Settings", Hint: "view configuration and recent sessions", Kind: "settings"},
		{Label: "Change Model", Hint: "migration", Kind: "model"},
		{Label: "Change Boundary", Hint: "workspace permissions", Kind: "boundary"},
		{Label: "Traits", Hint: "shared commands and skills", Kind: "traits"},
		{Label: "Re-probe Habitats", Hint: "refresh local Claude/Codex state", Kind: "probe"},
		{Label: "Toggle Theme", Hint: m.theme.Name, Kind: "theme"},
		{Label: "Help", Hint: "minimal keybind reference", Kind: "help"},
	}
	for _, session := range m.sessionList {
		entries = append(entries, paletteEntry{
			Label:     session.Title,
			Hint:      fmt.Sprintf("[%s] %s", session.CurrentModel, session.FolderPath),
			Kind:      "session",
			SessionID: session.ID,
		})
	}
	return entries
}

func (m Model) executePaletteEntry(entry paletteEntry) (tea.Model, tea.Cmd) {
	m.modal = modalNone
	switch entry.Kind {
	case "new":
		return m, func() tea.Msg {
			session, resolution, count, err := m.sessions.CreateCurrent(m.ctx, m.cwd, m.profiles)
			return sessionCreatedMsg{Session: session, Resolution: resolution, ExistingCount: count, Err: err}
		}
	case "settings":
		m.center = viewSettings
		m.status = "Settings opened."
		return m, nil
	case "help":
		m.center = viewHelp
		m.status = "Help opened."
		return m, nil
	case "model":
		if session, ok := m.selectedSession(); ok {
			m.migrationUI = migrationState{Model: session.CurrentModel, ModelIndex: m.indexForModel(session.CurrentModel)}
			m.modal = modalMigration
		}
		return m, nil
	case "boundary":
		if session, ok := m.selectedSession(); ok {
			m.boundaryUI.ProfileIndex = m.indexForProfile(session.BoundaryProfile)
			m.modal = modalBoundary
		}
		return m, nil
	case "traits":
		m.modal = modalTraitEditor
		m.traitEditor = traitEditorState{Type: domain.TraitTypeCommand, SupportsClaude: true, SupportsCodex: true}
		return m, nil
	case "probe":
		m.status = "Re-probing habitats."
		return m, func() tea.Msg { return probeResultMsg{Health: m.prober.ProbeAll(m.ctx)} }
	case "theme":
		if m.theme.Name == "dark" {
			m.theme = LightTheme()
		} else {
			m.theme = DarkTheme()
		}
		m.settings.Theme = m.theme.Name
		if err := m.store.SaveAppSettings(m.ctx, m.settings); err != nil {
			m.status = fmt.Sprintf("Failed to persist theme: %v", err)
		} else {
			m.status = fmt.Sprintf("Theme switched to %s.", m.theme.Name)
		}
		return m, nil
	case "session":
		for i, session := range m.sessionList {
			if session.ID == entry.SessionID {
				m.sessionIx = i
				state, _ := m.store.LoadSessionRuntimeState(m.ctx, session.ID)
				state.Active = true
				_ = m.store.SaveSessionRuntimeState(m.ctx, session.ID, state)
				m.center = viewChat
				m.status = fmt.Sprintf("Switched to %s.", session.Title)
				return m, tea.Batch(m.loadMessagesCmd(session.ID), m.loadRuntimeStateCmd(session.ID))
			}
		}
	}
	return m, nil
}

func (m Model) loadMessagesCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := m.chat.List(m.ctx, sessionID)
		return messagesLoadedMsg{SessionID: sessionID, Messages: messages, Err: err}
	}
}

func (m Model) loadRuntimeStateCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		state, err := m.store.LoadSessionRuntimeState(m.ctx, sessionID)
		return runtimeStateLoadedMsg{SessionID: sessionID, State: state, Err: err}
	}
}

func (m *Model) applyStreamEnvelope(env chat.StreamEnvelope) {
	if env.Event.NativeSessionID != "" {
		if session, ok := m.selectedSession(); ok && session.ID == env.Event.SessionID {
			session.NativeSessionID = env.Event.NativeSessionID
			m.replaceSession(session)
		}
	}
	if len(env.Messages) > 0 {
		m.messages = append([]domain.Message(nil), env.Messages...)
	}
	switch env.Event.Kind {
	case domain.TurnEventDelta:
		m.appendStreamingMessage(domain.MessageRoleAssistant, env.Event.Text)
		m.status = "Streaming assistant output..."
	case domain.TurnEventToolStarted, domain.TurnEventToolOutput, domain.TurnEventToolFinished:
		text := env.Event.Text
		if env.Event.ToolName != "" {
			text = env.Event.ToolName + ": " + text
		}
		m.appendStreamingMessage(domain.MessageRoleTool, text)
	case domain.TurnEventNotice:
		m.appendStreamingMessage(domain.MessageRoleSystem, env.Event.Text)
	case domain.TurnEventCompleted:
		m.status = "Turn completed."
		if env.Session.ID != "" {
			m.replaceSession(env.Session)
		}
		if len(env.Messages) > 0 {
			m.messages = append([]domain.Message(nil), env.Messages...)
		}
	case domain.TurnEventHabitatError:
		m.status = fallback(env.Event.Text, "Habitat error")
	}
	if env.Done && env.Err != nil {
		m.status = fmt.Sprintf("Turn finished with error: %v", env.Err)
	}
}

func (m *Model) appendStreamingMessage(role domain.MessageRole, delta string) {
	if strings.TrimSpace(delta) == "" {
		return
	}
	if len(m.messages) > 0 {
		last := &m.messages[len(m.messages)-1]
		if last.Role == role && last.Source == "stream" {
			last.Content += delta
			return
		}
	}
	m.messages = append(m.messages, domain.Message{
		ID:        fmt.Sprintf("stream-%d", time.Now().UnixNano()),
		Role:      role,
		Content:   delta,
		Source:    "stream",
		CreatedAt: time.Now().UTC(),
	})
}

func (m Model) waitStreamCmd(id int) tea.Cmd {
	stream := m.streams[id]
	return func() tea.Msg {
		if stream == nil {
			return streamMsg{Closed: true, StreamID: id}
		}
		item, ok := <-stream
		if !ok {
			return streamMsg{Closed: true, StreamID: id}
		}
		return streamMsg{Envelope: item, StreamID: id}
	}
}

func (m Model) performMigration(session domain.Session, model string) tea.Msg {
	nextHabitat, ok := habitats.HabitatForModel(model)
	if !ok {
		return operationMsg{Err: fmt.Errorf("could not map model %q to a habitat", model), Status: "Migration failed"}
	}
	activeTraits, err := m.traits.ActiveForSession(m.ctx, session)
	if err != nil {
		return operationMsg{Err: err, Status: "Migration failed"}
	}
	checkpoint, err := m.migration.CreateCheckpoint(m.ctx, session, activeTraits)
	if err != nil {
		return operationMsg{Err: err, Status: "Migration failed"}
	}
	state, err := m.store.LoadSessionRuntimeState(m.ctx, session.ID)
	if err != nil {
		return operationMsg{Err: err, Status: "Migration failed"}
	}
	state.PendingCheckpointID = checkpoint.ID
	state.PendingContinuation = m.migration.ContinuationText(checkpoint, model, nextHabitat)
	if err := m.store.SaveSessionRuntimeState(m.ctx, session.ID, state); err != nil {
		return operationMsg{Err: err, Status: "Migration failed"}
	}
	_, _ = m.store.CreateMessage(m.ctx, session.ID, domain.MessageRoleSummary, fmt.Sprintf("Migrated from %s/%s to %s/%s.", session.CurrentHabitat, session.CurrentModel, nextHabitat, model), "migration")
	_, _ = m.store.CreateMessage(m.ctx, session.ID, domain.MessageRoleSystem, fmt.Sprintf("── migrated to %s ──", model), "migration")

	session.CurrentModel = model
	session.ModelDescriptor = domain.ModelDescriptor{ID: model, Label: model, Habitat: nextHabitat}
	session.CurrentHabitat = nextHabitat
	session.NativeSessionID = ""
	session.MigrationGeneration++
	if err := m.store.UpdateSession(m.ctx, session); err != nil {
		return operationMsg{Err: err, Status: "Migration failed"}
	}
	_ = m.store.AppendEvent(m.ctx, session.ID, "model.changed", map[string]any{"model": model})
	_ = m.store.AppendEvent(m.ctx, session.ID, "habitat.changed", map[string]any{"habitat": nextHabitat})
	return operationMsg{Session: session, Status: fmt.Sprintf("Migrated to %s.", model)}
}

func (m Model) performBoundaryChange(session domain.Session, profile domain.BoundaryProfile) tea.Msg {
	resolution := boundaries.Resolve(profile, session.CurrentHabitat)
	session.BoundaryProfile = profile.ID
	session.ResolvedBoundarySettings = resolution.NativeSettings
	if err := m.store.UpdateSession(m.ctx, session); err != nil {
		return operationMsg{Err: err, Status: "Boundary update failed"}
	}
	_ = m.store.AppendEvent(m.ctx, session.ID, "boundary.changed", map[string]any{"profile": profile.ID, "compatibility": resolution.Compatibility})
	_, _ = m.store.CreateMessage(m.ctx, session.ID, domain.MessageRoleSystem, fmt.Sprintf("Boundary profile changed to %s (%s).", profile.Name, resolution.Compatibility), "estuary")
	return operationMsg{Session: session, Status: fmt.Sprintf("Boundary updated to %s.", profile.Name)}
}

func (m Model) selectedSession() (domain.Session, bool) {
	if len(m.sessionList) == 0 || m.sessionIx >= len(m.sessionList) {
		return domain.Session{}, false
	}
	return m.sessionList[m.sessionIx], true
}

func (m *Model) replaceSession(session domain.Session) {
	for i := range m.sessionList {
		if m.sessionList[i].ID == session.ID {
			m.sessionList[i] = session
			return
		}
	}
}

func (m Model) availableModels() []string {
	var items []string
	seen := map[string]bool{"claude-sonnet-4-6": true}
	items = append(items, "claude-sonnet-4-6")
	for _, health := range m.health {
		for _, model := range health.AvailableModels {
			if !seen[model] {
				seen[model] = true
				items = append(items, model)
			}
		}
	}
	return items
}

func (m Model) selectedModelOption(index int) string {
	models := m.availableModels()
	if len(models) == 0 {
		return "claude-sonnet-4-6"
	}
	return models[m.cycleIndex(index, len(models))]
}

func (m Model) cycleIndex(index, size int) int {
	if size == 0 {
		return 0
	}
	for index < 0 {
		index += size
	}
	return index % size
}

func (m Model) indexForModel(model string) int {
	for i, item := range m.availableModels() {
		if item == model {
			return i
		}
	}
	return 0
}

func (m Model) indexForProfile(id domain.ProfileID) int {
	for i, profile := range m.profiles {
		if profile.ID == id {
			return i
		}
	}
	return 0
}

func (m Model) resumeLabel(session domain.Session, state domain.SessionRuntimeState) string {
	if session.NativeSessionID == "" {
		return "Transcript only"
	}
	if state.ResumeExplicit {
		return "Resume requested"
	}
	if strings.TrimSpace(state.LastResumeStatus) != "" {
		return state.LastResumeStatus
	}
	return "Available"
}

func (m *Model) traitFieldPtr() *string {
	fields := []*string{&m.traitEditor.Name, &m.traitEditor.Description, &m.traitEditor.Definition}
	if int(m.traitEditor.Field) < len(fields) {
		return fields[m.traitEditor.Field]
	}
	return nil
}

func (m *Model) appendTraitField(text string) {
	if f := m.traitFieldPtr(); f != nil {
		*f += text
	}
}

func (m *Model) backspaceTraitField() {
	if f := m.traitFieldPtr(); f != nil {
		*f = trimLastRune(*f)
	}
}

func (m Model) renderTraitField(label, value string, active bool) string {
	indicator := " "
	if active {
		indicator = ">"
	}
	if strings.TrimSpace(value) == "" {
		value = "(empty)"
	}
	return fmt.Sprintf("%s %s: %s", indicator, label, value)
}

func composerLines(text string, width int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return wrapText(text, max(12, width-2))
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var out []string
	for _, rawLine := range strings.Split(text, "\n") {
		runes := []rune(rawLine)
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}

func prefixLines(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, prefix+line)
	}
	return out
}

func trimLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	return string(runes[:len(runes)-1])
}

func dispatchModeFor(kind domain.TraitType, supportsClaude, supportsCodex bool) string {
	if !supportsClaude && !supportsCodex || kind == domain.TraitTypeTool {
		return domain.TraitDispatchUnsupported
	}
	return domain.TraitDispatchInjected
}

func (m Model) headerSubtitle() string {
	session, ok := m.selectedSession()
	if !ok {
		return fmt.Sprintf("%s  •  default model claude-sonnet-4-6", m.cwd)
	}
	return fmt.Sprintf("%s  •  %s  •  %s", session.FolderPath, session.CurrentModel, session.CurrentHabitat)
}

func (m Model) panelTitle(s string) string {
	return lipgloss.NewStyle().Foreground(m.theme.AccentClay).Bold(true).Render(s)
}

func panelStyle(theme Theme, focused bool) lipgloss.Style {
	borderColor := theme.BorderSoft
	if focused {
		borderColor = theme.AccentWater
	}
	return lipgloss.NewStyle().
		Foreground(theme.FGPrimary).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)
}

func mutedStyle(theme Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.FGMuted)
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
