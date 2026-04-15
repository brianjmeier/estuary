package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
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

func TestFitTerminalLinePreservesANSIWidth(t *testing.T) {
	line := "\x1b[38;5;208mClaude\x1b[0m"
	got := fitTerminalLine(line, 12)

	if width := xansi.StringWidth(got); width != 12 {
		t.Fatalf("fitTerminalLine() width = %d, want %d", width, 12)
	}
	if stripped := xansi.Strip(got); stripped != "Claude      " {
		t.Fatalf("fitTerminalLine() stripped = %q, want %q", stripped, "Claude      ")
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

func TestRenderTerminalFrameFitsANSIContent(t *testing.T) {
	theme := DarkTheme()
	lines := []string{
		"\x1b[38;5;208mClaude\x1b[0m " + "\x1b[48;5;109mbar\x1b[0m",
	}

	view := renderTerminalFrame(theme, 40, lines)
	if got := lipgloss.Width(view); got != 40 {
		t.Fatalf("renderTerminalFrame() width = %d, want %d", got, 40)
	}
	if got := lipgloss.Height(view); got != 3 {
		t.Fatalf("renderTerminalFrame() height = %d, want %d", got, 3)
	}
}
