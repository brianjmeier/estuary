package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brianmeier/estuary/internal/boundaries"
	"github.com/brianmeier/estuary/internal/chat"
	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/migration"
	"github.com/brianmeier/estuary/internal/providers"
	"github.com/brianmeier/estuary/internal/store"
	"github.com/brianmeier/estuary/internal/traits"
)

func TestKeyTextIncludesSpace(t *testing.T) {
	if got := keyText(tea.KeyMsg{Type: tea.KeySpace}); got != " " {
		t.Fatalf("expected space key to append a literal space, got %q", got)
	}
}

func TestShouldRenderThinkingTracksPendingAssistantReply(t *testing.T) {
	model := Model{
		activeTurns: map[string]int{"session-1": 1},
		messages: []domain.Message{
			{Role: domain.MessageRoleAssistant, Content: "previous reply"},
			{Role: domain.MessageRoleUser, Content: "new prompt"},
		},
	}

	if !model.shouldRenderThinking("session-1") {
		t.Fatal("expected thinking placeholder while waiting on a new assistant reply")
	}

	model.messages = append(model.messages, domain.Message{Role: domain.MessageRoleAssistant, Content: "streaming"})
	if model.shouldRenderThinking("session-1") {
		t.Fatal("expected thinking placeholder to disappear once assistant output begins")
	}
}

func TestQuestionMarkOpensShortcutsWhenComposerEmpty(t *testing.T) {
	model := Model{
		compose: composeState{Mode: domain.ComposeModeChat},
	}

	next, _ := model.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	got := next.(Model)

	if got.modal != modalShortcuts {
		t.Fatalf("modal = %v, want %v", got.modal, modalShortcuts)
	}
	if got.compose.Text != "" {
		t.Fatalf("compose text = %q, want empty", got.compose.Text)
	}
}

func TestQuestionMarkTypesWhenComposerHasText(t *testing.T) {
	model := Model{
		compose: composeState{Text: "hello", Mode: domain.ComposeModeChat},
	}

	next, _ := model.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	got := next.(Model)

	if got.modal != modalNone {
		t.Fatalf("modal = %v, want %v", got.modal, modalNone)
	}
	if got.compose.Text != "hello?" {
		t.Fatalf("compose text = %q, want %q", got.compose.Text, "hello?")
	}
}

func TestRenderComposerDockHidesShortcutHintWithInput(t *testing.T) {
	model := Model{
		theme:   DarkTheme(),
		compose: composeState{Mode: domain.ComposeModeChat},
	}

	empty := model.renderComposerDock(80)
	if !strings.Contains(empty, "? for shortcuts") {
		t.Fatal("expected empty composer to show shortcuts hint")
	}

	model.compose.Text = "hello"
	filled := model.renderComposerDock(80)
	if strings.Contains(filled, "? for shortcuts") {
		t.Fatal("expected typed composer to hide shortcuts hint")
	}

	model.compose = composeState{Mode: domain.ComposeModeShell}
	shell := model.renderComposerDock(80)
	if strings.Contains(shell, "? for shortcuts") {
		t.Fatal("expected shell mode to hide shortcuts hint")
	}
}

func TestBangEnablesShellModeWithoutTypingBang(t *testing.T) {
	model := Model{
		compose: composeState{Mode: domain.ComposeModeChat},
	}

	next, _ := model.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	got := next.(Model)

	if got.compose.Mode != domain.ComposeModeShell {
		t.Fatalf("compose mode = %q, want shell", got.compose.Mode)
	}
	if got.compose.Text != "" {
		t.Fatalf("compose text = %q, want empty", got.compose.Text)
	}
}

func TestCtrlSStashesAndRestoresPrompt(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	model.compose.Text = "draft prompt"

	next, _ := model.handleMainKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = next.(Model)

	if model.compose.Text != "" {
		t.Fatalf("compose text = %q, want empty after stash", model.compose.Text)
	}
	state, err := st.LoadSessionRuntimeState(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if state.StashedPrompt != "draft prompt" {
		t.Fatalf("stashed prompt = %q", state.StashedPrompt)
	}

	next, _ = model.handleMainKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = next.(Model)

	if model.compose.Text != "draft prompt" {
		t.Fatalf("compose text = %q, want restored draft", model.compose.Text)
	}
	state, err = st.LoadSessionRuntimeState(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if state.StashedPrompt != "" {
		t.Fatalf("stashed prompt = %q, want empty after restore", state.StashedPrompt)
	}
}

func TestVisibleMessagesHidesVerboseNoiseByDefault(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	model.messages = []domain.Message{
		{Role: domain.MessageRoleUser, Content: "user"},
		{Role: domain.MessageRoleTool, Content: "tool"},
		{Role: domain.MessageRoleSystem, Content: "provider restored", Source: "provider"},
		{Role: domain.MessageRoleSystem, Content: "boundary changed", Source: "estuary"},
		{Role: domain.MessageRoleAssistant, Content: "assistant"},
	}

	got := model.visibleMessages()
	if len(got) != 3 {
		t.Fatalf("visible messages = %d, want 3", len(got))
	}
	if got[0].Role != domain.MessageRoleUser || got[1].Source != "estuary" || got[2].Role != domain.MessageRoleAssistant {
		t.Fatalf("unexpected visible messages: %#v", got)
	}

	state := model.runtimeStates[session.ID]
	state.VerboseOutput = true
	model.runtimeStates[session.ID] = state
	if got = model.visibleMessages(); len(got) != 5 {
		t.Fatalf("visible messages with verbose = %d, want 5", len(got))
	}
}

func TestShiftTabTogglesClaudeAutoAcceptEdits(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	if got := permissionMode(t, session.ResolvedBoundarySettings); got != "default" {
		t.Fatalf("initial permission mode = %q, want default", got)
	}

	next, _ := model.handleMainKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = next.(Model)

	state, err := st.LoadSessionRuntimeState(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if !state.AutoAcceptEdits {
		t.Fatal("expected auto-accept edits to be enabled")
	}
	updated, err := st.GetSession(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got := permissionMode(t, updated.ResolvedBoundarySettings); got != "acceptEdits" {
		t.Fatalf("permission mode after enable = %q, want acceptEdits", got)
	}

	next, _ = model.handleMainKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = next.(Model)

	state, err = st.LoadSessionRuntimeState(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if state.AutoAcceptEdits {
		t.Fatal("expected auto-accept edits to be disabled")
	}
	updated, err = st.GetSession(model.ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got := permissionMode(t, updated.ResolvedBoundarySettings); got != "default" {
		t.Fatalf("permission mode after disable = %q, want default", got)
	}
}

func TestMetaMOpensModelPicker(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	model.health = []domain.HabitatHealth{
		{Habitat: domain.HabitatClaude, AvailableModels: []string{"claude-opus-4-6"}},
	}

	next, cmd := model.handleMainKey(tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{'m'}})
	model = next.(Model)
	if cmd != nil {
		t.Fatal("expected modal open, not immediate migration")
	}
	if model.modal != modalMigration {
		t.Fatalf("modal = %v, want migration", model.modal)
	}
	if model.migrationUI.Model != session.CurrentModel {
		t.Fatalf("selected model = %q, want %q", model.migrationUI.Model, session.CurrentModel)
	}
}

func TestRenderMigrationUsesFriendlyNames(t *testing.T) {
	model, st, _ := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	model.migrationUI = migrationState{Model: "claude-sonnet-4-6", CurrentModel: "claude-sonnet-4-6", ModelIndex: 0}

	view := model.renderMigration(80)
	if !strings.Contains(view, "Sonnet 4.6") {
		t.Fatalf("expected friendly current model name, got %q", view)
	}
	if !strings.Contains(view, "Opus 4.6") {
		t.Fatalf("expected friendly alternative model name, got %q", view)
	}
	if strings.Contains(view, "claude-opus-4-6") {
		t.Fatalf("expected no raw model ids in picker, got %q", view)
	}
}

func TestAvailableModelsUsesCuratedSupportedList(t *testing.T) {
	model := Model{}
	models := model.availableModels()

	want := []string{}
	for _, item := range habitats.SupportedModels() {
		want = append(want, item.ID)
	}
	if !slices.Equal(models, want) {
		t.Fatalf("models = %#v, want %#v", models, want)
	}
}

func TestRenderMigrationMarksOnlySessionCurrentModel(t *testing.T) {
	model, st, _ := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	model.migrationUI = migrationState{Model: "claude-opus-4-6", CurrentModel: "claude-sonnet-4-6", ModelIndex: 1}

	view := model.renderMigration(80)
	if strings.Count(view, "current") != 1 {
		t.Fatalf("expected exactly one current marker, got %q", view)
	}
}

func TestShortcutRowsShowPopStashPromptWhenAvailable(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	rows := model.shortcutRows()
	if !hasShortcut(rows, "ctrl + s", "stash prompt") {
		t.Fatalf("expected stash prompt shortcut, got %#v", rows)
	}

	state := model.runtimeStates[session.ID]
	state.StashedPrompt = "draft"
	model.runtimeStates[session.ID] = state
	rows = model.shortcutRows()
	if !hasShortcut(rows, "ctrl + s", "pop stash prompt") {
		t.Fatalf("expected pop stash prompt shortcut, got %#v", rows)
	}
}

func TestRenderComposerMetaBarShowsAutoAcceptState(t *testing.T) {
	model, st, session := newTestModel(t, domain.HabitatClaude)
	defer func() { _ = st.Close() }()

	state := model.runtimeStates[session.ID]
	state.AutoAcceptEdits = true
	model.runtimeStates[session.ID] = state

	bar := model.renderComposerMetaBar(100)
	if !strings.Contains(bar, "Auto-accept edits") {
		t.Fatalf("expected auto-accept state in meta bar, got %q", bar)
	}
}

func TestPickerWindowCentersSelectionWithinLimit(t *testing.T) {
	start, end := pickerWindow(20, 10, 6)
	if start != 7 || end != 13 {
		t.Fatalf("window = %d-%d, want 7-13", start, end)
	}

	start, end = pickerWindow(20, 1, 6)
	if start != 0 || end != 6 {
		t.Fatalf("window near start = %d-%d, want 0-6", start, end)
	}

	start, end = pickerWindow(20, 19, 6)
	if start != 14 || end != 20 {
		t.Fatalf("window near end = %d-%d, want 14-20", start, end)
	}
}

func TestComposerAnchoredModalsRenderAboveComposer(t *testing.T) {
	model := Model{
		theme:       DarkTheme(),
		width:       100,
		height:      40,
		center:      viewChat,
		modal:       modalSlashPicker,
		compose:     composeState{Text: "/", Mode: domain.ComposeModeChat},
		sessionList: []domain.Session{{ID: "session-1", Title: "agenator", FolderPath: "/tmp/project"}},
	}

	view := model.View()
	slashIndex := strings.Index(view, "Slash Commands")
	composerIndex := strings.Index(view, "› /")
	if slashIndex == -1 || composerIndex == -1 {
		t.Fatalf("expected slash picker and composer in view, got %q", view)
	}
	if slashIndex > composerIndex {
		t.Fatal("expected slash picker to render above composer")
	}
}

func newTestModel(t *testing.T, habitat domain.Habitat) (Model, *store.Store, domain.Session) {
	t.Helper()

	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	profiles := boundaries.DefaultProfiles()
	profile := profileByID(t, profiles, boundaries.ProfileWorkspaceWrite)
	session, err := st.CreateSession(ctx, domain.SessionDraft{
		FolderPath:      t.TempDir(),
		Model:           "claude-sonnet-4-6",
		BoundaryProfile: string(profile.ID),
	}, habitat, boundaries.Resolve(profile, habitat))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	manager := providers.NewSessionManager(st, map[domain.Habitat]providers.Adapter{
		domain.HabitatClaude: providers.NewClaudeAdapter(),
		domain.HabitatCodex:  providers.NewCodexAdapter(),
	})
	model := Model{
		ctx:           ctx,
		store:         st,
		chat:          chat.NewService(st, manager),
		migration:     migration.NewService(st),
		traits:        traits.NewService(st),
		theme:         DarkTheme(),
		profiles:      profiles,
		sessionList:   []domain.Session{session},
		runtimeStates: map[string]domain.SessionRuntimeState{session.ID: {}},
		sessionTasks:  map[string][]domain.SessionTask{},
		compose:       composeState{Mode: domain.ComposeModeChat},
	}
	return model, st, session
}

func profileByID(t *testing.T, profiles []domain.BoundaryProfile, id domain.ProfileID) domain.BoundaryProfile {
	t.Helper()
	for _, profile := range profiles {
		if profile.ID == id {
			return profile
		}
	}
	t.Fatalf("profile %q not found", id)
	return domain.BoundaryProfile{}
}

func permissionMode(t *testing.T, raw string) string {
	t.Helper()
	var settings map[string]string
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		t.Fatalf("unmarshal permission settings: %v", err)
	}
	return settings["permission_mode"]
}

func hasShortcut(rows [][2]string, key, label string) bool {
	for _, row := range rows {
		if row[0] == key && row[1] == label {
			return true
		}
	}
	return false
}
