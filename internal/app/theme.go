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
		BGCanvas:      lipgloss.Color("#1A1A1A"),
		BGSurface:     lipgloss.Color("#242424"),
		BGPanel:       lipgloss.Color("#2E2E2E"),
		FGPrimary:     lipgloss.Color("#E8E8E8"),
		FGMuted:       lipgloss.Color("#737373"),
		BorderSoft:    lipgloss.Color("#3D3D3D"),
		AccentWater:   lipgloss.Color("#4ADE80"),
		AccentReed:    lipgloss.Color("#4ADE80"),
		AccentClay:    lipgloss.Color("#86EFAC"),
		StatusWarning: lipgloss.Color("#FACC15"),
		StatusDanger:  lipgloss.Color("#F87171"),
		StatusSuccess: lipgloss.Color("#4ADE80"),
		HabitatClaude: lipgloss.Color("#F9A825"),
		HabitatCodex:  lipgloss.Color("#60A5FA"),
	}
}

func LightTheme() Theme {
	return Theme{
		Name:          "light",
		BGCanvas:      lipgloss.Color("#FFFFFF"),
		BGSurface:     lipgloss.Color("#F5F5F5"),
		BGPanel:       lipgloss.Color("#EBEBEB"),
		FGPrimary:     lipgloss.Color("#1A1A1A"),
		FGMuted:       lipgloss.Color("#737373"),
		BorderSoft:    lipgloss.Color("#D4D4D4"),
		AccentWater:   lipgloss.Color("#16A34A"),
		AccentReed:    lipgloss.Color("#16A34A"),
		AccentClay:    lipgloss.Color("#15803D"),
		StatusWarning: lipgloss.Color("#CA8A04"),
		StatusDanger:  lipgloss.Color("#DC2626"),
		StatusSuccess: lipgloss.Color("#16A34A"),
		HabitatClaude: lipgloss.Color("#D97706"),
		HabitatCodex:  lipgloss.Color("#2563EB"),
	}
}

func ThemeByName(name string) Theme {
	if name == "light" {
		return LightTheme()
	}
	return DarkTheme()
}
