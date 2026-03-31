package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/brianmeier/estuary/internal/domain"
)

// ModelPickerModel is a minimal Bubble Tea program for choosing a model and
// provider. It runs as a short-lived alt-screen program just like PaletteModel.
type ModelPickerModel struct {
	models  []domain.ModelDescriptor
	current string // currently active model ID (highlighted differently)
	theme   Theme
	query   string
	sel     int
	width   int
	height  int
	chosen  domain.ModelDescriptor
	done    bool
}

func NewModelPickerModel(models []domain.ModelDescriptor, currentModelID string, theme Theme) ModelPickerModel {
	sel := 0
	for i, m := range models {
		if m.ID == currentModelID {
			sel = i
			break
		}
	}
	return ModelPickerModel{
		models:  models,
		current: currentModelID,
		theme:   theme,
		sel:     sel,
	}
}

// Chosen returns the selected descriptor and whether the user confirmed.
func (m ModelPickerModel) Chosen() (domain.ModelDescriptor, bool) {
	return m.chosen, m.done && m.chosen.ID != ""
}

func (m ModelPickerModel) Init() tea.Cmd { return nil }

func (m ModelPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		items := m.filtered()
		switch msg.String() {
		case "esc", "ctrl+k", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.sel > 0 {
				m.sel--
			}
		case "down", "j":
			if m.sel < len(items)-1 {
				m.sel++
			}
		case "enter":
			if len(items) > 0 {
				m.chosen = items[m.sel]
			}
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

func (m ModelPickerModel) View() string {
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
	active := s(t.AccentClay)

	panelW := min(56, m.width-4)

	title := accent.Render("◆ estuary") + muted.Render("  switch model")
	searchLine := muted.Render("  › ") + normal.Render(m.query) + accent.Render("│")
	div := muted.Render("  " + strings.Repeat("─", panelW-4))

	items := m.filtered()
	var lines []string
	for i, item := range items {
		label := fmt.Sprintf("%-18s", item.Label)
		suffix := ""
		if item.ID == m.current {
			suffix = " ●"
		}
		if i == m.sel {
			lines = append(lines, selStyle.Render("  ▸ "+label)+muted.Render("  "+suffix))
		} else if item.ID == m.current {
			lines = append(lines, active.Render("    "+label)+muted.Render("  "+suffix))
		} else {
			lines = append(lines, normal.Render("    "+label))
		}
	}
	if len(lines) == 0 {
		lines = []string{muted.Render("    no matches")}
	}

	footerHint := muted.Render("  ↑↓ navigate · enter select · esc cancel")
	body := strings.Join(append(
		[]string{"", title, "", searchLine, div},
		append(lines, "", footerHint, "")...,
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

func (m ModelPickerModel) filtered() []domain.ModelDescriptor {
	q := strings.ToLower(strings.TrimSpace(m.query))
	if q == "" {
		return m.models
	}
	var out []domain.ModelDescriptor
	for _, md := range m.models {
		haystack := strings.ToLower(md.Label + " " + string(md.Habitat) + " " + md.ID)
		if strings.Contains(haystack, q) {
			out = append(out, md)
		}
	}
	return out
}
