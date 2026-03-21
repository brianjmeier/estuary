package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brianmeier/estuary/internal/boundaries"
	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/store"
)

type Service struct {
	store *store.Store
}

func NewService(store *store.Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context) ([]domain.Session, error) {
	return s.store.ListSessions(ctx)
}

func (s *Service) CreateCurrent(ctx context.Context, folderPath string, profiles []domain.BoundaryProfile) (domain.Session, domain.BoundaryResolution, int, error) {
	return s.Create(ctx, domain.SessionDraft{
		FolderPath:      folderPath,
		Model:           "claude-sonnet-4-6",
		BoundaryProfile: string(boundaries.ProfileWorkspaceWrite),
	}, profiles)
}

func (s *Service) Create(ctx context.Context, draft domain.SessionDraft, profiles []domain.BoundaryProfile) (domain.Session, domain.BoundaryResolution, int, error) {
	folderPath := filepath.Clean(strings.TrimSpace(draft.FolderPath))
	model := strings.TrimSpace(draft.Model)
	profileID := strings.TrimSpace(draft.BoundaryProfile)

	if folderPath == "" {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("folder path is required")
	}
	info, err := os.Stat(folderPath)
	if err != nil {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("folder path is not accessible: %w", err)
	}
	if !info.IsDir() {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("folder path must be a directory")
	}
	if model == "" {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("model is required")
	}

	habitat, ok := habitats.HabitatForModel(model)
	if !ok {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("could not map model %q to a habitat", model)
	}

	var profile domain.BoundaryProfile
	found := false
	for _, candidate := range profiles {
		if candidate.ID == domain.ProfileID(profileID) {
			profile = candidate
			found = true
			break
		}
	}
	if !found {
		return domain.Session{}, domain.BoundaryResolution{}, 0, fmt.Errorf("unknown boundary profile %q", profileID)
	}

	resolution := boundaries.Resolve(profile, habitat)
	existingCount, err := s.store.CountActiveSessionsByFolder(ctx, folderPath)
	if err != nil {
		return domain.Session{}, domain.BoundaryResolution{}, 0, err
	}

	session, err := s.store.CreateSession(ctx, domain.SessionDraft{
		FolderPath:      folderPath,
		Model:           model,
		BoundaryProfile: profileID,
	}, habitat, resolution)
	if err != nil {
		return domain.Session{}, domain.BoundaryResolution{}, existingCount, err
	}
	if err := s.store.SaveSessionRuntimeState(ctx, session.ID, domain.SessionRuntimeState{Active: true, FirstRunCompleted: true}); err != nil {
		return domain.Session{}, domain.BoundaryResolution{}, existingCount, err
	}

	return session, resolution, existingCount, nil
}
