package app

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Name          string
	BGCanvas      lipgloss.Color
	BGSurface     lipgloss.Color
	BGPanel       lipgloss.Color
	FGPrimary     lipgloss.Color
	FGMuted       lipgloss.Color
	BorderSoft    lipgloss.Color
	AccentWater   lipgloss.Color
	AccentReed    lipgloss.Color
	AccentClay    lipgloss.Color
	StatusWarning lipgloss.Color
	StatusDanger  lipgloss.Color
	StatusSuccess lipgloss.Color
	HabitatClaude lipgloss.Color
	HabitatCodex  lipgloss.Color
}

func DarkTheme() Theme {
	return Theme{
		Name:          "dark",
		BGCanvas:      lipgloss.Color("#182229"),
		BGSurface:     lipgloss.Color("#21313A"),
		BGPanel:       lipgloss.Color("#263842"),
		FGPrimary:     lipgloss.Color("#E8E2D6"),
		FGMuted:       lipgloss.Color("#9BA8AB"),
		BorderSoft:    lipgloss.Color("#36505C"),
		AccentWater:   lipgloss.Color("#78A8B8"),
		AccentReed:    lipgloss.Color("#8DAA6C"),
		AccentClay:    lipgloss.Color("#BE8C65"),
		StatusWarning: lipgloss.Color("#C8A562"),
		StatusDanger:  lipgloss.Color("#A56A5F"),
		StatusSuccess: lipgloss.Color("#7BA770"),
		HabitatClaude: lipgloss.Color("#C28E62"),
		HabitatCodex:  lipgloss.Color("#6E9BB2"),
	}
}

func LightTheme() Theme {
	return Theme{
		Name:          "light",
		BGCanvas:      lipgloss.Color("#E7E0D3"),
		BGSurface:     lipgloss.Color("#F2ECE1"),
		BGPanel:       lipgloss.Color("#F7F3EA"),
		FGPrimary:     lipgloss.Color("#25353E"),
		FGMuted:       lipgloss.Color("#65757C"),
		BorderSoft:    lipgloss.Color("#B8C2BE"),
		AccentWater:   lipgloss.Color("#4D7A90"),
		AccentReed:    lipgloss.Color("#698351"),
		AccentClay:    lipgloss.Color("#9C6A4F"),
		StatusWarning: lipgloss.Color("#A9823C"),
		StatusDanger:  lipgloss.Color("#8E564D"),
		StatusSuccess: lipgloss.Color("#5A7C4A"),
		HabitatClaude: lipgloss.Color("#AD7553"),
		HabitatCodex:  lipgloss.Color("#537992"),
	}
}

func ThemeByName(name string) Theme {
	if name == "light" {
		return LightTheme()
	}
	return DarkTheme()
}
