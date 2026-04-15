package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestEncodeTerminalKeyCtrlC(t *testing.T) {
	got, ok := encodeTerminalKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !ok {
		t.Fatal("encodeTerminalKey() returned ok=false")
	}
	if string(got) != "\x03" {
		t.Fatalf("encodeTerminalKey(ctrl+c) = %q, want %q", string(got), "\x03")
	}
}

func TestEncodeTerminalKeyArrowUp(t *testing.T) {
	got, ok := encodeTerminalKey(tea.KeyMsg{Type: tea.KeyUp})
	if !ok {
		t.Fatal("encodeTerminalKey() returned ok=false")
	}
	if string(got) != "\x1b[A" {
		t.Fatalf("encodeTerminalKey(up) = %q, want %q", string(got), "\x1b[A")
	}
}

func TestEncodeTerminalKeyAltRune(t *testing.T) {
	got, ok := encodeTerminalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true})
	if !ok {
		t.Fatal("encodeTerminalKey() returned ok=false")
	}
	if string(got) != "\x1bx" {
		t.Fatalf("encodeTerminalKey(alt+x) = %q, want %q", string(got), "\x1bx")
	}
}

func TestEmbeddedLayoutReservesSidebar(t *testing.T) {
	termOuter, sideOuter, cols, rows := embeddedLayout(120, 40)
	if termOuter <= sideOuter {
		t.Fatalf("embeddedLayout() termOuter=%d sideOuter=%d, want terminal wider than sidebar", termOuter, sideOuter)
	}
	if cols <= 0 || rows <= 0 {
		t.Fatalf("embeddedLayout() cols=%d rows=%d, want positive sizes", cols, rows)
	}
}

func TestEmbeddedViewFitsRequestedSize(t *testing.T) {
	model := &EmbeddedTerminalModel{
		theme:  DarkTheme(),
		width:  120,
		height: 30,
		status: "starting",
	}

	view := model.View()
	if got := lipgloss.Width(view); got != 120 {
		t.Fatalf("View() width = %d, want %d", got, 120)
	}
	if got := lipgloss.Height(view); got != 30 {
		t.Fatalf("View() height = %d, want %d", got, 30)
	}
}

func TestLeaderModeTogglesWithCtrlK(t *testing.T) {
	model := &EmbeddedTerminalModel{theme: DarkTheme()}

	gotModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	got := gotModel.(*EmbeddedTerminalModel)
	if !got.leaderActive {
		t.Fatal("leaderActive = false, want true after first ctrl+k")
	}

	gotModel, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	got = gotModel.(*EmbeddedTerminalModel)
	if !got.leaderActive {
		t.Fatal("leaderActive = false, want true after unrelated key")
	}

	gotModel, _ = got.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	got = gotModel.(*EmbeddedTerminalModel)
	if got.leaderActive {
		t.Fatal("leaderActive = true, want false after second ctrl+k")
	}
}

func TestSidebarUsesFriendlyModelAndHidesReadySections(t *testing.T) {
	model := &EmbeddedTerminalModel{
		theme:  DarkTheme(),
		width:  120,
		height: 30,
		status: "Session ready.",
		session: domain.Session{
			FolderPath:     "/tmp/agenator",
			CurrentModel:   "claude-sonnet-4-6",
			CurrentHabitat: domain.HabitatClaude,
		},
		health: []domain.HabitatHealth{
			{Habitat: domain.HabitatClaude, Installed: true, Authenticated: true},
			{Habitat: domain.HabitatCodex, Installed: true, Authenticated: true},
		},
	}

	got := model.renderInfoSidebar(28)
	if !strings.Contains(got, "◆ Estuary") {
		t.Fatalf("renderInfoSidebar() missing capitalized sidebar title: %q", got)
	}
	if strings.Contains(got, "◆ estuary") {
		t.Fatalf("renderInfoSidebar() includes lowercase sidebar title: %q", got)
	}
	if !strings.Contains(got, "Sonnet 4.6") {
		t.Fatalf("renderInfoSidebar() missing friendly model label: %q", got)
	}
	if strings.Contains(got, "claude-sonnet-4-6") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes raw model id: %q", got)
	}
	if strings.Contains(got, "[Claude]") || strings.Contains(got, "[claude]") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes provider tag: %q", got)
	}
	if strings.Contains(got, "Runtime") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes Runtime section: %q", got)
	}
	if strings.Contains(got, "Providers") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes Providers section: %q", got)
	}
	if strings.Contains(got, "Status") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes Status section: %q", got)
	}
	if !strings.Contains(got, "Ctrl+K  commands") {
		t.Fatalf("renderInfoSidebar() missing compact command hint: %q", got)
	}
	if strings.Contains(got, "switch session") {
		t.Fatalf("renderInfoSidebar() unexpectedly includes expanded shortcuts: %q", got)
	}
}

func TestSidebarShowsExpandedLeaderShortcutsOnlyWhenActive(t *testing.T) {
	model := &EmbeddedTerminalModel{
		theme:        DarkTheme(),
		status:       "Session ready.",
		leaderActive: true,
		session: domain.Session{
			FolderPath:     "/tmp/agenator",
			CurrentModel:   "claude-sonnet-4-6",
			CurrentHabitat: domain.HabitatClaude,
		},
	}

	got := model.renderInfoSidebar(28)
	if !strings.Contains(got, "Leader") {
		t.Fatalf("renderInfoSidebar() missing Leader section: %q", got)
	}
	if !strings.Contains(got, "s        switch session") {
		t.Fatalf("renderInfoSidebar() missing expanded shortcut rows: %q", got)
	}
	if strings.Contains(got, "Active. ? help") {
		t.Fatalf("renderInfoSidebar() still includes wrapped leader prose: %q", got)
	}
}

func TestModelsSidebarUsesFriendlyLabelsWithoutProviders(t *testing.T) {
	model := &EmbeddedTerminalModel{
		theme: DarkTheme(),
		session: domain.Session{
			CurrentModel:   "claude-sonnet-4-6",
			CurrentHabitat: domain.HabitatClaude,
		},
	}

	got := model.renderModelsSidebar(40)
	if !strings.Contains(got, "Sonnet 4.6") {
		t.Fatalf("renderModelsSidebar() missing friendly model label: %q", got)
	}
	if strings.Contains(got, "claude-sonnet-4-6") {
		t.Fatalf("renderModelsSidebar() unexpectedly includes raw model id: %q", got)
	}
	if strings.Contains(got, "[claude]") || strings.Contains(got, "[codex]") {
		t.Fatalf("renderModelsSidebar() unexpectedly includes provider tag: %q", got)
	}
}

func TestSameProviderNoticeUsesFriendlyModelAndWordWrap(t *testing.T) {
	model := &EmbeddedTerminalModel{
		theme: DarkTheme(),
		session: domain.Session{
			ID:             "session-1",
			CurrentModel:   "claude-sonnet-4-6",
			CurrentHabitat: domain.HabitatClaude,
		},
	}

	cmd := model.switchModelCmd(domain.ModelDescriptor{
		ID:      "claude-opus-4-6",
		Label:   "Opus 4.6",
		Habitat: domain.HabitatClaude,
	})

	if cmd != nil {
		t.Fatal("same-provider model switch returned command, want provider-native notice only")
	}
	if !strings.Contains(model.status, "Opus 4.6") {
		t.Fatalf("status missing friendly model label: %q", model.status)
	}
	if strings.Contains(model.status, "claude-opus-4-6") {
		t.Fatalf("status includes raw model id: %q", model.status)
	}
	for _, line := range wrapText(model.status, 24) {
		if len([]rune(line)) > 24 {
			t.Fatalf("wrapped line %q exceeds width", line)
		}
	}
}

func TestWrapTextKeepsWordsTogether(t *testing.T) {
	got := wrapText("Use Claude's native model picker for Opus 4.6.", 18)
	want := []string{"Use Claude's", "native model", "picker for Opus", "4.6."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrapText() = %#v, want %#v", got, want)
	}
}

func TestHandoffContextIncludesRecentTerminalInput(t *testing.T) {
	model := &EmbeddedTerminalModel{}
	model.recordTerminalInput([]byte("this conversation is about bananas"))
	model.recordTerminalInput([]byte{'\r'})

	got := strings.Join(model.handoffContextLines(), "\n")
	if !strings.Contains(got, "Recent user terminal input:") {
		t.Fatalf("handoff context missing input heading: %q", got)
	}
	if !strings.Contains(got, "- this conversation is about bananas") {
		t.Fatalf("handoff context missing terminal input: %q", got)
	}
}

func TestRecordTerminalInputHandlesBackspace(t *testing.T) {
	model := &EmbeddedTerminalModel{}
	model.recordTerminalInput([]byte("bananaz"))
	model.recordTerminalInput([]byte{0x7f})
	model.recordTerminalInput([]byte("s\r"))

	if len(model.recentTerminalInputs) != 1 || model.recentTerminalInputs[0] != "bananas" {
		t.Fatalf("recentTerminalInputs = %#v, want bananas", model.recentTerminalInputs)
	}
}
