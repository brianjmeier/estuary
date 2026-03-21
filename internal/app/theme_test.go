package app

import "testing"

func TestThemeTokensPresent(t *testing.T) {
	themes := []Theme{DarkTheme(), LightTheme()}
	for _, theme := range themes {
		if theme.BGCanvas == "" || theme.BGSurface == "" || theme.BGPanel == "" {
			t.Fatalf("%s missing background tokens", theme.Name)
		}
		if theme.FGPrimary == "" || theme.FGMuted == "" {
			t.Fatalf("%s missing foreground tokens", theme.Name)
		}
		if theme.BorderSoft == "" || theme.AccentWater == "" || theme.AccentReed == "" || theme.AccentClay == "" {
			t.Fatalf("%s missing accent tokens", theme.Name)
		}
		if theme.StatusWarning == "" || theme.StatusDanger == "" || theme.StatusSuccess == "" {
			t.Fatalf("%s missing status tokens", theme.Name)
		}
		if theme.HabitatClaude == "" || theme.HabitatCodex == "" {
			t.Fatalf("%s missing habitat tokens", theme.Name)
		}
	}
}
