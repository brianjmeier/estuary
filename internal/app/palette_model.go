package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brianmeier/estuary/internal/domain"
)

// PaletteAction is the result returned when the palette exits.
type PaletteAction struct {
	Kind      string // "" = dismissed, "quit", "new", "session", "reconnect", "probe", "theme", "model"
	SessionID string // set when Kind == "session"
}

type paletteItem struct {
	Label     string
	Hint      string
	Kind      string
	SessionID string
}

// PaletteModel is the Bubble Tea model for the Ctrl+K command palette.
// It runs as a short-lived full-screen program (tea.WithAltScreen) on top
// of the live PTY session and returns a PaletteAction when dismissed.
type PaletteModel struct {
	sessions []domain.Session
	theme    Theme
	query    string
	sel      int
	width    int
	height   int
	result   PaletteAction
	done     bool
}

func NewPaletteModel(sessions []domain.Session, theme Theme) PaletteModel {
	return PaletteModel{sessions: sessions, theme: theme}
}

func (m PaletteModel) Result() PaletteAction { return m.result }

func (m PaletteModel) Init() tea.Cmd { return nil }

func (m PaletteModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+k", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			items := m.filtered()
			if m.sel > 0 && len(items) > 0 {
				m.sel--
			}
		case "down", "j":
			items := m.filtered()
			if m.sel < len(items)-1 {
				m.sel++
			}
		case "enter":
			items := m.filtered()
			if len(items) == 0 {
				m.done = true
				return m, tea.Quit
			}
			item := items[m.sel]
			m.result = PaletteAction{Kind: item.Kind, SessionID: item.SessionID}
			m.done = true
			return m, tea.Quit
		case "backspace":
			m.query = trimLastRune(m.query)
			m.sel = 0
		default:
			if t := keyText(msg); t != "" {
				m.query += t
				m.sel = 0
			}
		}
	}
	return m, nil
}

func (m PaletteModel) View() string {
	if m.width == 0 {
		return ""
	}

	t := m.theme
	bg := t.BGSurface

	s := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Background(bg)
	}
	accent := s(t.AccentWater).Bold(true)
	muted := s(t.FGMuted)
	selStyle := s(t.AccentWater).Bold(true)
	normal := s(t.FGPrimary)

	panelW := min(64, m.width-4)

	title := accent.Render("◆ estuary") + muted.Render("  command palette")
	searchLine := muted.Render("  › ") + normal.Render(m.query) + accent.Render("│")
	div := muted.Render("  " + strings.Repeat("─", panelW-4))

	items := m.filtered()
	var entryLines []string
	for i, item := range items {
		if i == m.sel {
			line := selStyle.Render("  ▸ "+item.Label) + muted.Render("  "+item.Hint)
			entryLines = append(entryLines, line)
		} else {
			line := normal.Render("    "+item.Label) + muted.Render("  "+item.Hint)
			entryLines = append(entryLines, line)
		}
	}
	if len(entryLines) == 0 {
		entryLines = []string{muted.Render("    no matches")}
	}

	footerHint := muted.Render("  ↑↓ navigate · enter select · esc dismiss")

	body := strings.Join(append(
		[]string{"", title, "", searchLine, div},
		append(entryLines, "", footerHint, "")...,
	), "\n")

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderSoft).
		Background(bg).
		Width(panelW).
		Padding(0, 1).
		Render(body)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel,
		lipgloss.WithWhitespaceBackground(t.BGCanvas))
}

func (m PaletteModel) filtered() []paletteItem {
	all := m.items()
	q := strings.ToLower(strings.TrimSpace(m.query))
	if q == "" {
		return all
	}
	var out []paletteItem
	for _, item := range all {
		if strings.Contains(strings.ToLower(item.Label+" "+item.Hint), q) {
			out = append(out, item)
		}
	}
	return out
}

func (m PaletteModel) items() []paletteItem {
	return []paletteItem{
		{Label: "New Session", Hint: "start a fresh session in the current directory", Kind: "new"},
		{Label: "Switch Session", Hint: "open a previous session", Kind: "sessions"},
		{Label: "Switch Model", Hint: "change model and provider", Kind: "model"},
		{Label: "Reconnect", Hint: "restart the current native session", Kind: "reconnect"},
		{Label: "Re-probe Ecosystem", Hint: "refresh Claude / Codex state", Kind: "probe"},
		{Label: "Toggle Theme", Hint: m.theme.Name, Kind: "theme"},
		{Label: "Quit", Hint: "exit Estuary", Kind: "quit"},
	}
}
