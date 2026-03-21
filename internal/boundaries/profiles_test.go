package boundaries

import (
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestDefaultProfiles(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) != 4 {
		t.Fatalf("expected 4 profiles, got %d", len(profiles))
	}

	var foundFullAccess bool
	for _, profile := range profiles {
		if profile.ID == ProfileFullAccess {
			foundFullAccess = true
			if !profile.Unsafe {
				t.Fatal("full access must be marked unsafe")
			}
		}
	}
	if !foundFullAccess {
		t.Fatal("full access profile missing")
	}
}

func TestResolveReturnsCompatibility(t *testing.T) {
	var target domain.BoundaryProfile
	for _, profile := range DefaultProfiles() {
		if profile.ID == ProfileWorkspaceWrite {
			target = profile
		}
	}

	claude := Resolve(target, domain.HabitatClaude)
	if claude.Compatibility != domain.BoundaryCompatibilityApproximated {
		t.Fatalf("expected claude workspace write to be approximated, got %s", claude.Compatibility)
	}

	codex := Resolve(target, domain.HabitatCodex)
	if codex.Compatibility != domain.BoundaryCompatibilityExact {
		t.Fatalf("expected codex workspace write to be exact, got %s", codex.Compatibility)
	}
}
