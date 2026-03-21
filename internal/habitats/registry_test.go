package habitats

import (
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestHabitatForModel(t *testing.T) {
	cases := []struct {
		model  string
		want   domain.Habitat
		wantOK bool
	}{
		{model: "claude-sonnet-4", want: domain.HabitatClaude, wantOK: true},
		{model: "gpt-5", want: domain.HabitatCodex, wantOK: true},
		{model: "codex-mini", want: domain.HabitatCodex, wantOK: true},
		{model: "mystery-model", wantOK: false},
	}

	for _, tc := range cases {
		got, ok := HabitatForModel(tc.model)
		if ok != tc.wantOK {
			t.Fatalf("model %q ok=%t want %t", tc.model, ok, tc.wantOK)
		}
		if ok && got != tc.want {
			t.Fatalf("model %q habitat=%s want %s", tc.model, got, tc.want)
		}
	}
}
