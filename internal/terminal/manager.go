package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

type Manager struct {
	store *store.Store
}

func NewManager(store *store.Store) *Manager {
	return &Manager{store: store}
}

func (m *Manager) StartFeatureSession(ctx context.Context, session domain.Session, featureKey string, options map[string]any) (domain.TerminalFeatureSession, error) {
	raw, _ := json.Marshal(options)
	item := domain.TerminalFeatureSession{
		ID:                uuid.NewString(),
		SessionID:         session.ID,
		Provider:          session.CurrentHabitat,
		FeatureKey:        featureKey,
		TerminalSessionID: uuid.NewString(),
		Status:            domain.ProviderRuntimeStatusReady,
		MetadataJSON:      string(raw),
	}
	return item, m.store.UpsertTerminalFeatureSession(ctx, item)
}

func (m *Manager) AttachFeatureSession(ctx context.Context, session domain.Session, featureKey string) (domain.TerminalFeatureSession, error) {
	return m.store.GetTerminalFeatureSessionByFeature(ctx, session.ID, featureKey)
}

func (m *Manager) WriteFeatureInput(ctx context.Context, featureSessionID, input string) error {
	item, err := m.store.GetTerminalFeatureSessionByID(ctx, featureSessionID)
	if err != nil {
		return err
	}
	if item.Status == domain.ProviderRuntimeStatusClosed {
		return errors.New("terminal feature session is closed")
	}
	var metadata map[string]any
	if item.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(item.MetadataJSON), &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["last_input"] = input
	metadata["last_input_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	raw, _ := json.Marshal(metadata)
	item.MetadataJSON = string(raw)
	item.Status = domain.ProviderRuntimeStatusRunning
	return m.store.UpsertTerminalFeatureSession(ctx, item)
}

func (m *Manager) CloseFeatureSession(ctx context.Context, featureSessionID string) error {
	item, err := m.store.GetTerminalFeatureSessionByID(ctx, featureSessionID)
	if err != nil {
		return err
	}
	item.Status = domain.ProviderRuntimeStatusClosed
	item.ClosedAt = time.Now().UTC()
	return m.store.UpsertTerminalFeatureSession(ctx, item)
}

func (m *Manager) EnsureFeatureSession(ctx context.Context, session domain.Session, featureKey string, options map[string]any) (domain.TerminalFeatureSession, error) {
	item, err := m.store.GetTerminalFeatureSessionByFeature(ctx, session.ID, featureKey)
	if err == nil && item.Status != domain.ProviderRuntimeStatusClosed {
		return item, nil
	}
	return m.StartFeatureSession(ctx, session, featureKey, options)
}

func (m *Manager) ExecFeatureCommand(ctx context.Context, featureSessionID, commandText, cwd string) (string, string, error) {
	item, err := m.store.GetTerminalFeatureSessionByID(ctx, featureSessionID)
	if err != nil {
		return "", "", err
	}
	if item.Status == domain.ProviderRuntimeStatusClosed {
		return "", "", errors.New("terminal feature session is closed")
	}
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shellPath, "-lc", commandText)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var metadata map[string]any
	if item.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(item.MetadataJSON), &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["last_command"] = commandText
	metadata["last_command_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	metadata["last_exit_error"] = ""
	if runErr != nil {
		metadata["last_exit_error"] = runErr.Error()
	}
	raw, _ := json.Marshal(metadata)
	item.MetadataJSON = string(raw)
	item.Status = domain.ProviderRuntimeStatusReady
	if persistErr := m.store.UpsertTerminalFeatureSession(ctx, item); persistErr != nil {
		return stdout.String(), stderr.String(), persistErr
	}
	return stdout.String(), stderr.String(), runErr
}
