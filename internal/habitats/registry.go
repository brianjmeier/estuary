package habitats

import (
	"strings"

	"github.com/brianmeier/estuary/internal/domain"
)

type Descriptor struct {
	ID          domain.Habitat
	Label       string
	Binary      string
	ConfigHint  string
	AccentToken string
}

func Registry() []Descriptor {
	return []Descriptor{
		{
			ID:          domain.HabitatClaude,
			Label:       "Claude",
			Binary:      "claude",
			ConfigHint:  "~/.claude",
			AccentToken: "habitat.claude",
		},
		{
			ID:          domain.HabitatCodex,
			Label:       "Codex",
			Binary:      "codex",
			ConfigHint:  "~/.codex",
			AccentToken: "habitat.codex",
		},
	}
}

func HabitatForModel(model string) (domain.Habitat, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case m == "":
		return "", false
	case strings.Contains(m, "claude"):
		return domain.HabitatClaude, true
	case strings.Contains(m, "gpt"), strings.Contains(m, "o3"), strings.Contains(m, "o4"), strings.Contains(m, "codex"):
		return domain.HabitatCodex, true
	default:
		return "", false
	}
}
