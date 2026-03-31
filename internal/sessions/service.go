package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

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

// FindForFolder returns the most recently opened session for folderPath.
// The second return value is false if no session exists for that folder.
func (s *Service) FindForFolder(ctx context.Context, folderPath string) (domain.Session, bool, error) {
	return s.store.FindRecentSessionForFolder(ctx, filepath.Clean(folderPath))
}

func (s *Service) CreateCurrent(ctx context.Context, folderPath string) (domain.Session, int, error) {
	return s.Create(ctx, domain.SessionDraft{
		FolderPath: folderPath,
		Model:      "claude-sonnet-4-6",
	})
}

func (s *Service) Create(ctx context.Context, draft domain.SessionDraft) (domain.Session, int, error) {
	folderPath := filepath.Clean(strings.TrimSpace(draft.FolderPath))
	model := strings.TrimSpace(draft.Model)

	if folderPath == "" {
		return domain.Session{}, 0, fmt.Errorf("folder path is required")
	}
	info, err := os.Stat(folderPath)
	if err != nil {
		return domain.Session{}, 0, fmt.Errorf("folder path is not accessible: %w", err)
	}
	if !info.IsDir() {
		return domain.Session{}, 0, fmt.Errorf("folder path must be a directory")
	}
	if model == "" {
		return domain.Session{}, 0, fmt.Errorf("model is required")
	}

	habitat, ok := habitats.HabitatForModel(model)
	if !ok {
		return domain.Session{}, 0, fmt.Errorf("could not map model %q to a habitat", model)
	}

	existingCount, err := s.store.CountActiveSessionsByFolder(ctx, folderPath)
	if err != nil {
		return domain.Session{}, 0, err
	}

	session, err := s.store.CreateSession(ctx, domain.SessionDraft{
		FolderPath: folderPath,
		Model:      model,
	}, habitat)
	if err != nil {
		return domain.Session{}, existingCount, err
	}
	if err := s.store.SaveSessionRuntimeState(ctx, session.ID, domain.SessionRuntimeState{Active: true, FirstRunCompleted: true}); err != nil {
		return domain.Session{}, existingCount, err
	}
	providerRef := domain.ProviderSessionRef{
		ID:          uuid.NewString(),
		SessionID:   session.ID,
		Provider:    habitat,
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      domain.ProviderRuntimeStatusConnecting,
		StartedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := s.store.UpsertProviderSession(ctx, providerRef); err != nil {
		return domain.Session{}, existingCount, err
	}
	if err := s.store.SetActiveProviderSession(ctx, session.ID, providerRef.ID, ""); err != nil {
		return domain.Session{}, existingCount, err
	}
	session.ActiveProviderSessionID = providerRef.ID
	session.ProviderStatus = providerRef.Status

	return session, existingCount, nil
}
