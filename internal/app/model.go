package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/brianmeier/estuary/internal/providers"
	"github.com/brianmeier/estuary/internal/sessions"
	"github.com/brianmeier/estuary/internal/store"
	"github.com/brianmeier/estuary/internal/terminal"
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
	modalShortcuts
	modalSlashPicker
	modalFilePicker
	modalTasks
)

const (
	traitName traitField = iota
	traitDescription
	traitDefinition
	traitCount
)

// Spinner frames for thinking animation.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinnerTickMsg struct{}

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

type tasksLoadedMsg struct {
	SessionID string
	Tasks     []domain.SessionTask
	Err       error
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

type shellExecutedMsg struct {
	SessionID string
	Status    string
	Err       error
}

type composeState struct {
	Text string
	Mode domain.ComposeMode
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
	Model        string
	CurrentModel string
	ModelIndex   int
}

type boundaryState struct {
	ProfileIndex int
}

type pickerState struct {
	Query     string
	Selection int
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
	ctx              context.Context
	cwd              string
	store            *store.Store
	prober           *prereq.Prober
	sessions         *sessions.Service
	chat             *chat.Service
	migration        *migration.Service
	traits           *traits.Service
	terminal         *terminal.Manager
	width            int
	height           int
	theme            Theme
	settings         domain.AppSettings
	center           centerView
	modal            modalMode
	sessionIx        int
	sessionList      []domain.Session
	messages         []domain.Message
	health           []domain.HabitatHealth
	profiles         []domain.BoundaryProfile
	traitList        []domain.Trait
	runtimeStates    map[string]domain.SessionRuntimeState
	streams          map[int]<-chan chat.StreamEnvelope
	streamSessions   map[int]string
	activeTurns      map[string]int
	compose          composeState
	palette          paletteState
	migrationUI      migrationState
	boundaryUI       boundaryState
	slashPicker      pickerState
	filePicker       pickerState
	traitEditor      traitEditorState
	status           string
	streamID         int
	spinnerFrame     int
	spinnerActive    bool
	transcriptScroll int
	composeUndoStack []composeState
	escPendingUntil  time.Time
	sessionTasks     map[string][]domain.SessionTask
	filePickerItems  []string
}

func NewModel(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) (Model, error) {
	settings, err := st.LoadAppSettings(ctx)
	if err != nil {
		return Model{}, err
	}
	sessionSvc := sessions.NewService(st)
	manager := providers.NewSessionManager(st, map[domain.Habitat]providers.Adapter{
		domain.HabitatClaude: providers.NewClaudeAdapter(),
		domain.HabitatCodex:  providers.NewCodexAdapter(),
	})
	_ = manager.WarmRestore(ctx)
	chatSvc := chat.NewService(st, manager)
	sessionList, err := sessionSvc.List(ctx)
	if err != nil {
		return Model{}, err
	}

	return Model{
		ctx:            ctx,
		cwd:            cwd,
		store:          st,
		prober:         prober,
		sessions:       sessionSvc,
		chat:           chatSvc,
		migration:      migration.NewService(st),
		traits:         traits.NewService(st),
		terminal:       terminal.NewManager(st),
		theme:          ThemeByName(settings.Theme),
		settings:       settings,
		center:         viewChat,
		sessionList:    sessionList,
		profiles:       boundaries.DefaultProfiles(),
		runtimeStates:  map[string]domain.SessionRuntimeState{},
		streams:        map[int]<-chan chat.StreamEnvelope{},
		streamSessions: map[int]string{},
		activeTurns:    map[string]int{},
		compose:        composeState{Mode: domain.ComposeModeChat},
		sessionTasks:   map[string][]domain.SessionTask{},
		traitEditor: traitEditorState{
			Type:           domain.TraitTypeCommand,
			SupportsClaude: true,
			SupportsCodex:  true,
		},
		status: "Creating a new session in the current directory.",
	}, nil
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		spinnerTickCmd(),
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
	case spinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		hasActive := len(m.activeTurns) > 0
		m.spinnerActive = hasActive
		if hasActive {
			return m, spinnerTickCmd()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.MouseMsg:
		return m.handleMouse(msg)
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
	case tasksLoadedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Load tasks failed: %v", msg.Err)
		} else {
			m.sessionTasks[msg.SessionID] = msg.Tasks
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
		return m, tea.Batch(m.loadMessagesCmd(msg.Session.ID), m.loadRuntimeStateCmd(msg.Session.ID), m.loadTasksCmd(msg.Session.ID))
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
			m.finishStream(msg.StreamID)
			return m, nil
		}
		m.applyStreamEnvelope(msg.Envelope)
		if msg.Envelope.Done {
			m.finishStream(msg.StreamID)
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
			return m, tea.Batch(m.loadMessagesCmd(msg.Session.ID), m.loadRuntimeStateCmd(msg.Session.ID), m.loadTasksCmd(msg.Session.ID))
		}
		m.status = msg.Status
	case shellExecutedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("%s: %v", fallback(msg.Status, "Shell execution failed"), msg.Err)
		} else {
			m.status = msg.Status
		}
		if msg.SessionID != "" {
			return m, tea.Batch(m.loadMessagesCmd(msg.SessionID), m.loadTasksCmd(msg.SessionID))
		}
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
		case modalShortcuts:
			return m.handleShortcutsKey(msg)
		case modalSlashPicker:
			return m.handleSlashPickerKey(msg)
		case modalFilePicker:
			return m.handleFilePickerKey(msg)
		case modalTasks:
			return m.handleTasksKey(msg)
		}
		return m.handleMainKey(msg)
	}
	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.transcriptScroll += 3
	case tea.MouseButtonWheelDown:
		m.transcriptScroll = max(0, m.transcriptScroll-3)
	}
	return m, nil
}

func (m Model) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.isModelSwitchKey(msg) {
		if session, ok := m.selectedSession(); ok {
			m.migrationUI = migrationState{Model: session.CurrentModel, CurrentModel: session.CurrentModel, ModelIndex: m.indexForModel(session.CurrentModel)}
			m.modal = modalMigration
			m.status = "Model picker opened."
			return m, nil
		}
	}
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
	case "?":
		if m.compose.Mode == domain.ComposeModeChat && strings.TrimSpace(m.compose.Text) == "" {
			m.modal = modalShortcuts
			m.status = "Shortcuts opened."
			return m, nil
		}
		m.pushComposeUndo()
		m.compose.Text += "?"
		return m, nil
	case "/", "@":
		if m.compose.Mode == domain.ComposeModeChat && strings.TrimSpace(m.compose.Text) == "" {
			m.pushComposeUndo()
			if msg.String() == "/" {
				m.compose.Text = "/"
				m.compose.Mode = domain.ComposeModeChat
				m.slashPicker = pickerState{}
				m.modal = modalSlashPicker
				m.status = "Command picker opened."
				return m, nil
			}
			m.compose.Text = "@"
			m.compose.Mode = domain.ComposeModeChat
			m.filePicker = pickerState{}
			m.filePickerItems = m.workspaceFiles()
			m.modal = modalFilePicker
			m.status = "File picker opened."
			return m, nil
		}
	case "!":
		if strings.TrimSpace(m.compose.Text) == "" {
			m.pushComposeUndo()
			m.compose.Mode = domain.ComposeModeShell
			m.compose.Text = ""
			m.status = "Shell mode enabled."
			return m, nil
		}
	case "ctrl+o":
		if session, ok := m.selectedSession(); ok {
			state := m.runtimeStates[session.ID]
			state.VerboseOutput = !state.VerboseOutput
			m.runtimeStates[session.ID] = state
			_ = m.store.SaveSessionRuntimeState(m.ctx, session.ID, state)
			if state.VerboseOutput {
				m.status = "Verbose output enabled."
			} else {
				m.status = "Verbose output hidden."
			}
		}
		return m, nil
	case "ctrl+s":
		if session, ok := m.selectedSession(); ok {
			state := m.runtimeStates[session.ID]
			if strings.TrimSpace(m.compose.Text) != "" {
				if m.compose.Mode == domain.ComposeModeShell {
					state.StashedPrompt = "!" + m.compose.Text
				} else {
					state.StashedPrompt = m.compose.Text
				}
				m.compose.Text = ""
				m.compose.Mode = domain.ComposeModeChat
				m.status = "Prompt stashed."
			} else if strings.TrimSpace(state.StashedPrompt) != "" {
				if strings.HasPrefix(state.StashedPrompt, "!") {
					m.compose.Text = strings.TrimPrefix(state.StashedPrompt, "!")
					m.compose.Mode = domain.ComposeModeShell
				} else {
					m.compose.Text = state.StashedPrompt
					m.compose.Mode = domain.ComposeModeChat
				}
				state.StashedPrompt = ""
				m.status = "Prompt restored."
			}
			m.runtimeStates[session.ID] = state
			_ = m.store.SaveSessionRuntimeState(m.ctx, session.ID, state)
		}
		return m, nil
	case "ctrl+t":
		m.modal = modalTasks
		m.status = "Tasks opened."
		return m, nil
	case "shift+tab":
		return m.toggleAutoAcceptEdits()
	case "shift+enter":
		m.pushComposeUndo()
		m.compose.Text += "\n"
		return m, nil
	case "ctrl+_", "ctrl+shift+-":
		if len(m.composeUndoStack) > 0 {
			prev := m.composeUndoStack[len(m.composeUndoStack)-1]
			m.compose.Text = prev.Text
			m.compose.Mode = prev.Mode
			m.composeUndoStack = m.composeUndoStack[:len(m.composeUndoStack)-1]
		}
		return m, nil
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
		if m.compose.Mode == domain.ComposeModeShell {
			commandText := strings.TrimSpace(m.compose.Text)
			if commandText == "" {
				return m, nil
			}
			m.pushComposeUndo()
			m.compose.Text = ""
			m.compose.Mode = domain.ComposeModeChat
			return m, m.execShellCmd(session, commandText)
		}
		trait, resolvedPrompt, err := m.traits.ResolveCommand(m.ctx, session, prompt)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		if trait.ID != "" {
			m.status = fmt.Sprintf("Invoking trait %s.", trait.Name)
		}
		m.pushComposeUndo()
		m.compose.Text = ""
		m.compose.Mode = domain.ComposeModeChat
		m.streamID++
		ch := make(chan chat.StreamEnvelope, 32)
		streamID := m.streamID
		m.streams[streamID] = ch
		m.streamSessions[streamID] = session.ID
		m.activeTurns[session.ID]++
		session.Status = domain.SessionStatusActive
		m.replaceSession(session)
		m.spinnerActive = true
		m.transcriptScroll = 0
		go func() {
			_ = m.chat.SendStream(m.ctx, session, resolvedPrompt, func(item chat.StreamEnvelope) error {
				ch <- item
				return nil
			})
			close(ch)
		}()
		return m, tea.Batch(m.waitStreamCmd(streamID), spinnerTickCmd())
	case "backspace":
		m.pushComposeUndo()
		m.compose.Text = trimLastRune(m.compose.Text)
	case "pgup":
		m.transcriptScroll += 8
	case "pgdown":
		m.transcriptScroll = max(0, m.transcriptScroll-8)
	case "home":
		m.transcriptScroll = 1 << 20
	case "end":
		m.transcriptScroll = 0
	case "esc":
		now := time.Now()
		if now.Before(m.escPendingUntil) {
			m.compose.Text = ""
			m.compose.Mode = domain.ComposeModeChat
			m.escPendingUntil = time.Time{}
			m.status = "Composer cleared."
			return m, nil
		}
		m.escPendingUntil = now.Add(500 * time.Millisecond)
		m.status = "Press Esc again to clear."
		return m, nil
	default:
		if text := keyText(msg); text != "" {
			m.pushComposeUndo()
			m.compose.Text += text
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
		if text := keyText(msg); text != "" {
			m.palette.Query += text
			m.palette.Selection = 0
		}
	}
	return m, nil
}

func (m Model) handleShortcutsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?":
		m.modal = modalNone
		m.status = "Shortcuts closed."
	}
	return m, nil
}

func (m Model) handleSlashPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
	case "backspace":
		if m.compose.Text == "/" {
			m.compose.Text = ""
			m.modal = modalNone
			return m, nil
		}
		m.compose.Text = trimLastRune(m.compose.Text)
	case "up", "k":
		items := m.filteredSlashEntries()
		if len(items) > 0 && m.slashPicker.Selection > 0 {
			m.slashPicker.Selection--
		}
	case "down", "j":
		items := m.filteredSlashEntries()
		if len(items) > 0 && m.slashPicker.Selection < len(items)-1 {
			m.slashPicker.Selection++
		}
	case "enter":
		items := m.filteredSlashEntries()
		if len(items) == 0 {
			return m, nil
		}
		return m.applySlashEntry(items[m.slashPicker.Selection])
	default:
		if text := keyText(msg); text != "" {
			m.compose.Text += text
			m.slashPicker.Selection = 0
		}
	}
	return m, nil
}

func (m Model) handleFilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
	case "backspace":
		if m.compose.Text == "@" {
			m.compose.Text = ""
			m.modal = modalNone
			return m, nil
		}
		m.compose.Text = trimLastRune(m.compose.Text)
	case "up", "k":
		items := m.filteredFilePickerItems()
		if len(items) > 0 && m.filePicker.Selection > 0 {
			m.filePicker.Selection--
		}
	case "down", "j":
		items := m.filteredFilePickerItems()
		if len(items) > 0 && m.filePicker.Selection < len(items)-1 {
			m.filePicker.Selection++
		}
	case "enter":
		items := m.filteredFilePickerItems()
		if len(items) == 0 {
			return m, nil
		}
		m.compose.Text = items[m.filePicker.Selection]
		m.modal = modalNone
		m.status = "File path inserted."
	default:
		if text := keyText(msg); text != "" {
			m.compose.Text += text
			m.filePicker.Selection = 0
		}
	}
	return m, nil
}

func (m Model) handleTasksKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+t":
		m.modal = modalNone
		m.status = "Tasks closed."
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
	case "enter":
		session, ok := m.selectedSession()
		if !ok {
			return m, nil
		}
		model := m.selectedModelOption(m.migrationUI.ModelIndex)
		if model == "" {
			m.status = "Target model is required."
			return m, nil
		}
		m.modal = modalNone
		return m, func() tea.Msg { return m.performMigration(session, model) }
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
		if text := keyText(msg); text != "" {
			m.appendTraitField(text)
		}
	}
	return m, nil
}

func lizardArt() string {
	return "" +
		"       ▄█▄\n" +
		"      ▐● ●▌\n" +
		"       ▀█▀\n" +
		"  ▐▀▄  ███  ▄▀▌\n" +
		"   ▀   ███   ▀\n" +
		"  ▐▄▀  ███  ▀▄▌\n" +
		"       ███\n" +
		"      ▄▀ ▀▄\n" +
		"     ▀     ▀"
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing Estuary..."
	}

	contentW := max(60, m.width-4)
	composer := m.renderComposerDock(contentW)
	composerHeight := lipgloss.Height(composer)
	mainHeight := max(6, m.height-2-composerHeight)
	body := m.renderMainPanel(contentW, mainHeight)
	modal := ""
	if m.modal != modalNone {
		modal = m.renderModal(max(76, m.width-2))
	}

	layoutParts := []string{body}
	if modal != "" && m.isComposerAnchoredModal() {
		layoutParts = append(layoutParts, modal)
	}
	layoutParts = append(layoutParts, composer)
	if modal != "" && !m.isComposerAnchoredModal() {
		layoutParts = append(layoutParts, modal)
	}
	layout := lipgloss.JoinVertical(lipgloss.Left, layoutParts...)

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

	headerExtra := 2
	hero := ""
	if m.shouldShowChatHero(session.ID, height) {
		hero = m.renderChatHero(width)
		headerExtra += lipgloss.Height(hero) + 1
	}
	header := []string{
		fmt.Sprintf("%s  %s", session.Title, session.FolderPath),
		"",
	}
	transcriptHeight := max(4, height-headerExtra)
	transcript, scrollState := m.visibleTranscript(width, transcriptHeight)
	if len(transcript) == 0 {
		green := lipgloss.NewStyle().Foreground(m.theme.AccentWater)
		muted := mutedStyle(m.theme)
		transcript = []string{
			muted.Render("Start typing to talk to Estuary."),
			"",
			green.Render("ctrl+k") + muted.Render(" opens sessions, settings, and model controls."),
		}
	}
	if scrollState != "" {
		transcript = append(transcript, "", mutedStyle(m.theme).Render(scrollState))
	}

	lines := append(header, transcript...)
	if hero != "" {
		lines = append(lines, "", hero)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderComposer(width int) string {
	lines := composerLines(m.compose.Text, width)
	cursor := lipgloss.NewStyle().Foreground(m.theme.AccentWater).Render("|")
	if len(lines) == 0 {
		lines = []string{cursor}
	} else {
		lines[len(lines)-1] += cursor
	}
	scrollHint := ""
	if len(lines) > 8 {
		scrollHint = lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render("  scroll")
		lines = lines[len(lines)-8:]
	}
	promptPrefix := "› "
	if m.compose.Mode == domain.ComposeModeShell {
		promptPrefix = "! "
	}
	body := append([]string{promptPrefix + lines[0] + scrollHint}, prefixLines(lines[1:], "  ")...)
	return strings.Join(body, "\n")
}

func (m Model) renderComposerDock(width int) string {
	contentWidth := max(20, width-4)
	content := m.renderComposer(contentWidth)
	help := ""
	if m.compose.Mode == domain.ComposeModeChat && strings.TrimSpace(m.compose.Text) == "" {
		help = lipgloss.NewStyle().Foreground(m.theme.FGMuted).Render("? for shortcuts")
	}
	lines := strings.Split(content, "\n")
	firstWidth := max(0, contentWidth-lipgloss.Width(help)-1)
	lines[0] = lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(firstWidth).Render(lines[0]),
		help,
	)
	if meta := m.renderComposerMetaBar(contentWidth); meta != "" {
		lines = append(lines, "", meta)
	}
	return panelStyle(m.theme, true).Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderComposerMetaBar(width int) string {
	session, ok := m.selectedSession()
	if !ok {
		return ""
	}

	green := lipgloss.NewStyle().Foreground(m.theme.AccentWater)
	muted := lipgloss.NewStyle().Foreground(m.theme.FGMuted)
	parts := []string{
		green.Render(m.displayModelName(session)),
		m.boundaryLabel(session),
	}
	if state, ok := m.runtimeStates[session.ID]; ok && state.AutoAcceptEdits && session.CurrentHabitat == domain.HabitatClaude && session.BoundaryProfile != boundaries.ProfileFullAccess {
		parts = append(parts, "Auto-accept edits")
	}
	return muted.Width(width).Render(strings.Join(parts, muted.Render("  ·  ")))
}

func (m Model) boundaryLabel(session domain.Session) string {
	label := m.boundaryDisplayName(session.BoundaryProfile)
	if session.ResolvedBoundarySettings == "" {
		return label
	}
	var settings map[string]string
	if err := json.Unmarshal([]byte(session.ResolvedBoundarySettings), &settings); err != nil {
		return label
	}
	if strings.EqualFold(settings["permission_mode"], "plan") {
		return "Planning"
	}
	return label
}

func (m Model) boundaryDisplayName(id domain.ProfileID) string {
	switch id {
	case boundaries.ProfileWorkspaceWrite:
		return "Controlled"
	case boundaries.ProfileFullAccess:
		return "Unrestricted"
	default:
		return string(id)
	}
}

func (m Model) displayModelName(session domain.Session) string {
	label := strings.TrimSpace(session.ModelDescriptor.Label)
	if label != "" && label != session.CurrentModel {
		return label
	}
	return m.friendlyModelName(session.CurrentModel)
}

func (m Model) friendlyModelName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Default model"
	}
	if label := habitats.SupportedModelLabel(raw); label != "" {
		return label
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "gpt":
			parts[i] = strings.ToUpper(part)
		case "claude":
			parts[i] = "Claude"
		case "codex":
			parts[i] = "Codex"
		default:
			if len(part) > 0 && !slices.Contains([]string{"4", "4.5", "4.6", "5", "5.2", "5.3", "5.4"}, part) {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			}
		}
	}
	return strings.Join(parts, " ")
}

func (m Model) isModelSwitchKey(msg tea.KeyMsg) bool {
	if msg.String() == "meta+m" || msg.String() == "alt+m" {
		return true
	}
	return msg.Alt && len(msg.Runes) == 1 && strings.EqualFold(string(msg.Runes[0]), "m")
}

func (m Model) renderChatHero(width int) string {
	green := lipgloss.NewStyle().Foreground(m.theme.AccentWater)
	muted := lipgloss.NewStyle().Foreground(m.theme.FGMuted)
	title := green.Bold(true).Render("E S T U A R Y")
	subtitle := muted.Render(m.headerSubtitle())
	mascot := green.Render(lizardArt())
	hero := lipgloss.JoinVertical(lipgloss.Center, mascot, "", title, subtitle)
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, hero)
}

func (m Model) shouldShowChatHero(sessionID string, height int) bool {
	if len(m.messages) == 0 && len(m.activeTurns) == 0 {
		return true
	}
	if m.activeTurns[sessionID] > 0 {
		return false
	}
	return len(m.messages) <= 2 && height >= 18 && m.transcriptScroll == 0
}

func (m Model) transcriptLines(width int) []string {
	var out []string
	for _, message := range m.visibleMessages() {
		out = append(out, m.renderTranscriptMessage(message, width)...)
		out = append(out, "")
	}
	if session, ok := m.selectedSession(); ok && m.shouldRenderThinking(session.ID) {
		out = append(out, m.renderThinkingMessage(width)...)
		out = append(out, "")
	}
	return out
}

func (m Model) visibleMessages() []domain.Message {
	session, ok := m.selectedSession()
	if !ok {
		return m.messages
	}
	state := m.runtimeStates[session.ID]
	if state.VerboseOutput {
		return m.messages
	}
	var out []domain.Message
	for _, message := range m.messages {
		if message.Role == domain.MessageRoleTool {
			continue
		}
		if message.Role == domain.MessageRoleSystem && strings.Contains(strings.ToLower(message.Source), "provider") {
			continue
		}
		out = append(out, message)
	}
	return out
}

func (m *Model) visibleTranscript(width, height int) ([]string, string) {
	lines := m.transcriptLines(width)
	if len(lines) <= height {
		m.transcriptScroll = 0
		return lines, ""
	}

	maxScroll := max(0, len(lines)-height)
	if m.transcriptScroll > maxScroll {
		m.transcriptScroll = maxScroll
	}
	if m.transcriptScroll < 0 {
		m.transcriptScroll = 0
	}

	end := len(lines) - m.transcriptScroll
	if end < height {
		end = height
	}
	start := max(0, end-height)
	state := fmt.Sprintf("showing lines %d-%d of %d", start+1, end, len(lines))
	return lines[start:end], state
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
	return m.renderShortcutSheet(max(64, m.width-10))
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
	case modalShortcuts:
		return m.renderShortcutSheet(width)
	case modalSlashPicker:
		return m.renderSlashPicker(width)
	case modalFilePicker:
		return m.renderFilePicker(width)
	case modalTasks:
		return m.renderTasks(width)
	default:
		return ""
	}
}

func (m Model) isComposerAnchoredModal() bool {
	switch m.modal {
	case modalSlashPicker, modalFilePicker:
		return true
	default:
		return false
	}
}

func (m Model) renderShortcutSheet(width int) string {
	rows := m.shortcutRows()
	var lines []string
	for _, row := range rows {
		left := lipgloss.NewStyle().Width(28).Foreground(m.theme.AccentWater).Render(row[0])
		right := mutedStyle(m.theme).Render(row[1])
		lines = append(lines, left+" "+right)
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Shortcuts") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderSlashPicker(width int) string {
	items := m.filteredSlashEntries()
	lines := []string{mutedStyle(m.theme).Render("Commands")}
	if len(items) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No commands match."))
	} else {
		start, end := pickerWindow(len(items), m.slashPicker.Selection, 8)
		for i := start; i < end; i++ {
			item := items[i]
			prefix := "  "
			if i == m.slashPicker.Selection {
				prefix = "> "
			}
			line := prefix + item.Label
			if item.Hint != "" {
				line += "  " + mutedStyle(m.theme).Render(item.Hint)
			}
			lines = append(lines, line)
		}
		if len(items) > end-start {
			lines = append(lines, "", mutedStyle(m.theme).Render(fmt.Sprintf("showing %d-%d of %d", start+1, end, len(items))))
		}
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Slash Commands") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderFilePicker(width int) string {
	items := m.filteredFilePickerItems()
	lines := []string{mutedStyle(m.theme).Render("Workspace files")}
	if len(items) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No files match."))
	} else {
		start, end := pickerWindow(len(items), m.filePicker.Selection, 6)
		for i := start; i < end; i++ {
			item := items[i]
			prefix := "  "
			if i == m.filePicker.Selection {
				prefix = "> "
			}
			lines = append(lines, prefix+item)
		}
		if len(items) > end-start {
			lines = append(lines, "", mutedStyle(m.theme).Render(fmt.Sprintf("showing %d-%d of %d", start+1, end, len(items))))
		}
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("File Paths") + "\n" + strings.Join(lines, "\n"))
}

func (m Model) renderTasks(width int) string {
	session, ok := m.selectedSession()
	if !ok {
		return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Tasks") + "\nNo session selected.")
	}
	items := m.sessionTasks[session.ID]
	lines := []string{}
	if len(items) == 0 {
		lines = append(lines, mutedStyle(m.theme).Render("No provider tasks yet."))
	} else {
		for _, item := range items {
			status := mutedStyle(m.theme).Render(item.Status)
			lines = append(lines, fmt.Sprintf("%s  %s", item.Title, status))
			if strings.TrimSpace(item.Detail) != "" {
				lines = append(lines, "  "+mutedStyle(m.theme).Render(item.Detail))
			}
		}
	}
	return panelStyle(m.theme, true).Width(width).Render(m.panelTitle("Tasks") + "\n" + strings.Join(lines, "\n"))
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
	models := m.availableModels()
	lines := []string{mutedStyle(m.theme).Render("Choose the next model for this session.")}
	if len(models) == 0 {
		lines = append(lines, "", mutedStyle(m.theme).Render("No models available."))
	} else {
		if strings.TrimSpace(m.migrationUI.CurrentModel) != "" {
			lines = append(lines, "", mutedStyle(m.theme).Render("Current: "+m.friendlyModelName(m.migrationUI.CurrentModel)))
		}
		lines = append(lines, "")
		start, end := pickerWindow(len(models), m.migrationUI.ModelIndex, 8)
		for i := start; i < end; i++ {
			modelID := models[i]
			prefix := "  "
			if i == m.migrationUI.ModelIndex {
				prefix = "> "
			}
			line := prefix + m.friendlyModelName(modelID)
			if modelID == m.migrationUI.CurrentModel {
				line += "  " + mutedStyle(m.theme).Render("current")
			}
			lines = append(lines, line)
		}
		if len(models) > end-start {
			lines = append(lines, "", mutedStyle(m.theme).Render(fmt.Sprintf("showing %d-%d of %d", start+1, end, len(models))))
		}
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

func (m Model) shortcutRows() [][2]string {
	session, ok := m.selectedSession()
	isClaude := ok && session.CurrentHabitat == domain.HabitatClaude
	stashLabel := "stash prompt"
	if ok {
		if state, exists := m.runtimeStates[session.ID]; exists && strings.TrimSpace(state.StashedPrompt) != "" {
			stashLabel = "pop stash prompt"
		}
	}
	rows := [][2]string{
		{"! for bash mode", "run a local shell command"},
		{"/ for commands", "open the slash command picker"},
		{"@ for file paths", "insert a workspace file path"},
		{"double tap esc", "clear the input"},
		{"ctrl + shift + -", "undo input change"},
		{"ctrl + o", "toggle verbose output"},
		{"ctrl + t", "toggle tasks"},
		{"meta + m", "cycle model"},
		{"shift + enter", "insert newline"},
		{"ctrl + s", stashLabel},
	}
	if isClaude {
		rows = append(rows, [2]string{"shift + tab", "toggle auto-accept edits"})
	}
	return rows
}

func (m Model) filteredSlashEntries() []paletteEntry {
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(m.compose.Text, "/")))
	items := []paletteEntry{
		{Label: "/help", Hint: "open shortcuts", Kind: "slash-help"},
		{Label: "/model", Hint: "switch model", Kind: "slash-model"},
		{Label: "/boundary", Hint: "change boundary", Kind: "slash-boundary"},
		{Label: "/traits", Hint: "open traits", Kind: "slash-traits"},
		{Label: "/theme", Hint: "toggle theme", Kind: "slash-theme"},
		{Label: "/probe", Hint: "re-probe habitats", Kind: "slash-probe"},
	}
	if session, ok := m.selectedSession(); ok {
		for _, item := range m.traitList {
			if item.Type != domain.TraitTypeCommand {
				continue
			}
			if session.CurrentHabitat == domain.HabitatClaude && !item.SupportsClaude {
				continue
			}
			if session.CurrentHabitat == domain.HabitatCodex && !item.SupportsCodex {
				continue
			}
			items = append(items, paletteEntry{Label: "/" + item.Name, Hint: item.Description, Kind: "slash-trait"})
		}
	}
	if query == "" {
		return items
	}
	var out []paletteEntry
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Label+" "+item.Hint), query) {
			out = append(out, item)
		}
	}
	return out
}

func (m Model) filteredFilePickerItems() []string {
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(m.compose.Text, "@")))
	if query == "" {
		return m.filePickerItems
	}
	var out []string
	for _, item := range m.filePickerItems {
		if strings.Contains(strings.ToLower(item), query) {
			out = append(out, item)
		}
	}
	return out
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
			m.migrationUI = migrationState{Model: session.CurrentModel, CurrentModel: session.CurrentModel, ModelIndex: m.indexForModel(session.CurrentModel)}
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
				return m, tea.Batch(m.loadMessagesCmd(session.ID), m.loadRuntimeStateCmd(session.ID), m.loadTasksCmd(session.ID))
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

func (m Model) loadTasksCmd(sessionID string) tea.Cmd {
	return func() tea.Msg {
		items, err := m.store.ListSessionTasks(m.ctx, sessionID)
		return tasksLoadedMsg{SessionID: sessionID, Tasks: items, Err: err}
	}
}

func (m *Model) pushComposeUndo() {
	if len(m.composeUndoStack) > 100 {
		m.composeUndoStack = m.composeUndoStack[1:]
	}
	m.composeUndoStack = append(m.composeUndoStack, m.compose)
}

func (m Model) applySlashEntry(entry paletteEntry) (tea.Model, tea.Cmd) {
	m.modal = modalNone
	switch entry.Kind {
	case "slash-help":
		m.modal = modalShortcuts
		return m, nil
	case "slash-model":
		if session, ok := m.selectedSession(); ok {
			m.migrationUI = migrationState{Model: session.CurrentModel, CurrentModel: session.CurrentModel, ModelIndex: m.indexForModel(session.CurrentModel)}
			m.modal = modalMigration
		}
		return m, nil
	case "slash-boundary":
		if session, ok := m.selectedSession(); ok {
			m.boundaryUI.ProfileIndex = m.indexForProfile(session.BoundaryProfile)
			m.modal = modalBoundary
		}
		return m, nil
	case "slash-traits":
		m.modal = modalTraitEditor
		return m, nil
	case "slash-theme":
		return m.executePaletteEntry(paletteEntry{Kind: "theme"})
	case "slash-probe":
		return m.executePaletteEntry(paletteEntry{Kind: "probe"})
	default:
		m.compose.Text = entry.Label + " "
		return m, nil
	}
}

func (m Model) workspaceFiles() []string {
	session, ok := m.selectedSession()
	if !ok {
		return nil
	}
	root := session.FolderPath
	var out []string
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, ".direnv": true, ".next": true, "dist": true, "build": true, "coverage": true,
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if skipDirs[name] || strings.HasPrefix(name, ".") && name != "." {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(out)
	return out
}

func (m Model) execShellCmd(session domain.Session, commandText string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		feature, err := m.terminal.EnsureFeatureSession(ctx, session, "local_shell_mode", map[string]any{"cwd": session.FolderPath})
		if err != nil {
			return shellExecutedMsg{SessionID: session.ID, Status: "Shell session failed", Err: err}
		}
		_, _ = m.store.CreateMessage(ctx, session.ID, domain.MessageRoleSystem, "$ "+commandText, "shell_summary")
		stdout, stderr, runErr := m.terminal.ExecFeatureCommand(ctx, feature.ID, commandText, session.FolderPath)
		if strings.TrimSpace(stdout) != "" {
			_, _ = m.store.CreateMessage(ctx, session.ID, domain.MessageRoleTool, strings.TrimSpace(stdout), "shell_stdout")
		}
		if strings.TrimSpace(stderr) != "" {
			_, _ = m.store.CreateMessage(ctx, session.ID, domain.MessageRoleTool, strings.TrimSpace(stderr), "shell_stderr")
		}
		status := "Shell command completed."
		if runErr != nil {
			status = "Shell command failed."
		}
		return shellExecutedMsg{SessionID: session.ID, Status: status, Err: runErr}
	}
}

func (m Model) toggleAutoAcceptEdits() (tea.Model, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok || session.CurrentHabitat != domain.HabitatClaude || session.BoundaryProfile == boundaries.ProfileFullAccess {
		return m, nil
	}
	state := m.runtimeStates[session.ID]
	state.AutoAcceptEdits = !state.AutoAcceptEdits
	m.runtimeStates[session.ID] = state
	_ = m.store.SaveSessionRuntimeState(m.ctx, session.ID, state)

	resolution := boundaries.Resolve(m.profileForID(session.BoundaryProfile), session.CurrentHabitat)
	settings := map[string]string{}
	_ = json.Unmarshal([]byte(resolution.NativeSettings), &settings)
	if state.AutoAcceptEdits {
		settings["permission_mode"] = "acceptEdits"
		m.status = "Auto-accept edits enabled."
	} else {
		m.status = "Auto-accept edits disabled."
	}
	raw, _ := json.Marshal(settings)
	session.ResolvedBoundarySettings = string(raw)
	_ = m.store.UpdateSession(m.ctx, session)
	_ = m.chat.ReconnectSession(m.ctx, session.ID)
	m.replaceSession(session)
	return m, nil
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
	case domain.TurnEventToolStarted, domain.TurnEventToolOutput, domain.TurnEventToolFinished:
		text := env.Event.Text
		if env.Event.ToolName != "" {
			text = env.Event.ToolName + ": " + text
		}
		m.appendStreamingMessage(domain.MessageRoleTool, text)
	case domain.TurnEventTaskStarted, domain.TurnEventTaskProgress, domain.TurnEventTaskComplete:
		if env.Event.SessionID != "" {
			items, err := m.store.ListSessionTasks(m.ctx, env.Event.SessionID)
			if err == nil {
				m.sessionTasks[env.Event.SessionID] = items
			}
		}
	case domain.TurnEventNotice:
		m.appendStreamingMessage(domain.MessageRoleSystem, env.Event.Text)
	case domain.TurnEventCompleted:
		if env.Session.ID != "" {
			m.replaceSession(env.Session)
		}
		if len(env.Messages) > 0 {
			m.messages = append([]domain.Message(nil), env.Messages...)
		}
		m.transcriptScroll = 0
		if env.Session.ID != "" {
			items, err := m.store.ListSessionTasks(m.ctx, env.Session.ID)
			if err == nil {
				m.sessionTasks[env.Session.ID] = items
			}
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

func (m *Model) finishStream(streamID int) {
	delete(m.streams, streamID)
	sessionID := m.streamSessions[streamID]
	delete(m.streamSessions, streamID)
	if sessionID == "" {
		return
	}
	if m.activeTurns[sessionID] <= 1 {
		delete(m.activeTurns, sessionID)
		return
	}
	m.activeTurns[sessionID]--
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
	session.RuntimeKind = domain.SessionRuntimeKindProviderSession
	session.ProviderStatus = ""
	session.ActiveProviderSessionID = ""
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
	items := make([]string, 0, len(habitats.SupportedModels()))
	for _, model := range habitats.SupportedModels() {
		items = append(items, model.ID)
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

func (m Model) profileForID(id domain.ProfileID) domain.BoundaryProfile {
	for _, profile := range m.profiles {
		if profile.ID == id {
			return profile
		}
	}
	return domain.BoundaryProfile{ID: id}
}

func (m Model) resumeLabel(session domain.Session, state domain.SessionRuntimeState) string {
	if session.ActiveProviderSessionID == "" && session.NativeSessionID == "" {
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

func (m Model) renderTranscriptMessage(message domain.Message, width int) []string {
	muted := lipgloss.NewStyle().Foreground(m.theme.FGMuted)
	primary := lipgloss.NewStyle().Foreground(m.theme.FGPrimary)

	contentWidth := max(16, width-4)
	switch message.Role {
	case domain.MessageRoleUser:
		userBg := lipgloss.NewStyle().
			Foreground(m.theme.FGPrimary).
			Background(m.theme.BGPanel).
			Padding(0, 1)
		body := userBg.Render(strings.Join(wrapText(message.Content, contentWidth), "\n"))
		return strings.Split(body, "\n")
	case domain.MessageRoleAssistant:
		body := primary.Render(strings.Join(wrapText(message.Content, contentWidth), "\n"))
		return strings.Split("🦎 "+body, "\n")
	case domain.MessageRoleTool:
		body := muted.Render(strings.Join(wrapText(message.Content, contentWidth), "\n"))
		return strings.Split("   "+muted.Render("tool ›")+" "+body, "\n")
	case domain.MessageRoleSummary:
		green := lipgloss.NewStyle().Foreground(m.theme.AccentWater)
		body := muted.Render(strings.Join(wrapText(message.Content, contentWidth), "\n"))
		return strings.Split("   "+green.Render("migration ›")+" "+body, "\n")
	default:
		body := muted.Render(strings.Join(wrapText(message.Content, contentWidth), "\n"))
		return strings.Split("   "+muted.Render("system ›")+" "+body, "\n")
	}
}

func (m Model) renderThinkingMessage(width int) []string {
	green := lipgloss.NewStyle().Foreground(m.theme.AccentWater)
	frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
	return strings.Split("🦎 "+green.Render(frame), "\n")
}

func (m Model) renderMutedLines(text string, width int) []string {
	raw := wrapText(text, width)
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		lines = append(lines, mutedStyle(m.theme).Render(line))
	}
	return lines
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

func (m Model) shouldRenderThinking(sessionID string) bool {
	if m.activeTurns[sessionID] == 0 {
		return false
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		switch m.messages[i].Role {
		case domain.MessageRoleAssistant:
			return false
		case domain.MessageRoleUser:
			return true
		}
	}
	return true
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

func keyText(msg tea.KeyMsg) string {
	if msg.Type == tea.KeyRunes {
		return string(msg.Runes)
	}
	if msg.Type == tea.KeySpace || msg.String() == "space" || msg.String() == " " {
		return " "
	}
	return ""
}

func pickerWindow(total, selection, limit int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if limit <= 0 || total <= limit {
		return 0, total
	}
	if selection < 0 {
		selection = 0
	}
	if selection >= total {
		selection = total - 1
	}
	start := selection - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > total {
		end = total
		start = end - limit
	}
	return start, end
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
