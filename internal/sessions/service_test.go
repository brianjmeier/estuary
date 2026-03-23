package sessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/brianmeier/estuary/internal/boundaries"
	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

func TestCreateSessionPersistsAndWarnsOnDuplicateFolder(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()

	svc := NewService(st)
	profiles := boundaries.DefaultProfiles()

	first, resolution, duplicates, err := svc.Create(ctx, domain.SessionDraft{
		FolderPath:      projectDir,
		Model:           "gpt-5",
		BoundaryProfile: string(boundaries.ProfileWorkspaceWrite),
	}, profiles)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if duplicates != 0 {
		t.Fatalf("expected no duplicates, got %d", duplicates)
	}
	if first.CurrentHabitat != domain.HabitatCodex {
		t.Fatalf("expected codex habitat, got %s", first.CurrentHabitat)
	}
	if resolution.Compatibility != domain.BoundaryCompatibilityExact {
		t.Fatalf("expected exact codex mapping, got %s", resolution.Compatibility)
	}
	ref, err := st.GetProviderSessionBySession(ctx, first.ID, domain.SessionRuntimeKindProviderSession)
	if err != nil {
		t.Fatalf("provider session: %v", err)
	}
	if ref.Provider != domain.HabitatCodex {
		t.Fatalf("expected codex provider session, got %s", ref.Provider)
	}
	if ref.Status != domain.ProviderRuntimeStatusConnecting {
		t.Fatalf("expected connecting provider status, got %s", ref.Status)
	}

	_, _, duplicates, err = svc.Create(ctx, domain.SessionDraft{
		FolderPath:      projectDir,
		Model:           "claude-sonnet-4",
		BoundaryProfile: string(boundaries.ProfileWorkspaceWrite),
	}, profiles)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if duplicates != 1 {
		t.Fatalf("expected one duplicate warning, got %d", duplicates)
	}
}

func TestCreateSessionRejectsUnknownModel(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	projectDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	st, err := store.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	}()

	svc := NewService(st)

	_, _, _, err = svc.Create(ctx, domain.SessionDraft{
		FolderPath:      projectDir,
		Model:           "mystery-model",
		BoundaryProfile: string(boundaries.ProfileWorkspaceWrite),
	}, boundaries.DefaultProfiles())
	if err == nil {
		t.Fatal("expected unknown model error")
	}
}
