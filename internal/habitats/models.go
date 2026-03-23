package habitats

import "github.com/brianmeier/estuary/internal/domain"

var supportedModels = []domain.ModelDescriptor{
	{ID: "claude-sonnet-4-6", Label: "Sonnet 4.6", Habitat: domain.HabitatClaude},
	{ID: "claude-sonnet-4-5", Label: "Sonnet 4.5", Habitat: domain.HabitatClaude},
	{ID: "claude-opus-4-6", Label: "Opus 4.6", Habitat: domain.HabitatClaude},
	{ID: "claude-opus-4-5", Label: "Opus 4.5", Habitat: domain.HabitatClaude},
	{ID: "gpt-5.4", Label: "GPT-5.4", Habitat: domain.HabitatCodex},
	{ID: "gpt-5.3", Label: "GPT-5.3", Habitat: domain.HabitatCodex},
	{ID: "gpt-5.3-codex", Label: "GPT-5.3 Codex", Habitat: domain.HabitatCodex},
}

func SupportedModels() []domain.ModelDescriptor {
	out := make([]domain.ModelDescriptor, len(supportedModels))
	copy(out, supportedModels)
	return out
}

func SupportedModelLabel(id string) string {
	for _, model := range supportedModels {
		if model.ID == id {
			return model.Label
		}
	}
	return ""
}
