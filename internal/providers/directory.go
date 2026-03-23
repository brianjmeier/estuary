package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

type RuntimeBinding struct {
	SessionID      string
	Provider       domain.Habitat
	AdapterKey     string
	RuntimeKind    domain.SessionRuntimeKind
	Status         domain.ProviderRuntimeStatus
	ResumeCursor   map[string]any
	RuntimePayload map[string]any
}

type SessionDirectory struct {
	store *store.Store
}

func NewSessionDirectory(store *store.Store) *SessionDirectory {
	return &SessionDirectory{store: store}
}

func (d *SessionDirectory) Upsert(ctx context.Context, binding RuntimeBinding) (domain.ProviderSessionRef, error) {
	if strings.TrimSpace(binding.SessionID) == "" {
		return domain.ProviderSessionRef{}, fmt.Errorf("session id is required")
	}
	existing, err := d.store.GetProviderSessionBySession(ctx, binding.SessionID, binding.RuntimeKind)
	if err != nil {
		existing = domain.ProviderSessionRef{
			ID:          uuid.NewString(),
			SessionID:   binding.SessionID,
			Provider:    binding.Provider,
			RuntimeKind: binding.RuntimeKind,
			StartedAt:   time.Now().UTC(),
		}
	}
	if binding.AdapterKey != "" {
		if binding.RuntimePayload == nil {
			binding.RuntimePayload = map[string]any{}
		}
		binding.RuntimePayload["adapter_key"] = binding.AdapterKey
	}
	if payload := mergeJSONPayload(existing.ProviderResumeCursorJSON, binding.ResumeCursor, binding.RuntimePayload); payload != "" {
		existing.ProviderResumeCursorJSON = payload
	}
	existing.Provider = binding.Provider
	existing.RuntimeKind = binding.RuntimeKind
	if binding.Status != "" {
		existing.Status = binding.Status
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := d.store.UpsertProviderSession(ctx, existing); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	return existing, d.store.SetActiveProviderSession(ctx, binding.SessionID, existing.ID, firstNonEmpty(existing.ProviderSessionID, existing.ProviderThreadID))
}

func (d *SessionDirectory) Get(ctx context.Context, sessionID string, kind domain.SessionRuntimeKind) (RuntimeBinding, error) {
	ref, err := d.store.GetProviderSessionBySession(ctx, sessionID, kind)
	if err != nil {
		return RuntimeBinding{}, err
	}
	payload := map[string]any{}
	if ref.ProviderResumeCursorJSON != "" {
		_ = json.Unmarshal([]byte(ref.ProviderResumeCursorJSON), &payload)
	}
	binding := RuntimeBinding{
		SessionID:   ref.SessionID,
		Provider:    ref.Provider,
		RuntimeKind: ref.RuntimeKind,
		Status:      ref.Status,
	}
	if value, ok := payload["adapter_key"].(string); ok {
		binding.AdapterKey = value
		delete(payload, "adapter_key")
	}
	if resume, ok := payload["resume_cursor"].(map[string]any); ok {
		binding.ResumeCursor = resume
		delete(payload, "resume_cursor")
	}
	if len(payload) > 0 {
		binding.RuntimePayload = payload
	}
	return binding, nil
}

func (d *SessionDirectory) Apply(ctx context.Context, ref domain.ProviderSessionRef) (domain.ProviderSessionRef, RuntimeBinding, error) {
	binding, err := d.Get(ctx, ref.SessionID, ref.RuntimeKind)
	if err != nil {
		return ref, RuntimeBinding{}, err
	}
	if len(binding.ResumeCursor) > 0 {
		ref.ProviderResumeCursorJSON = mustJSON(binding.ResumeCursor)
		if ref.ProviderSessionID == "" {
			ref.ProviderSessionID = firstNonEmpty(
				stringValue(binding.ResumeCursor["provider_session_id"]),
				stringValue(binding.ResumeCursor["sessionId"]),
				stringValue(binding.ResumeCursor["resume"]),
			)
		}
		if ref.ProviderThreadID == "" {
			ref.ProviderThreadID = firstNonEmpty(
				stringValue(binding.ResumeCursor["provider_thread_id"]),
				stringValue(binding.ResumeCursor["threadId"]),
				ref.ProviderSessionID,
			)
		}
	}
	if binding.Provider != "" {
		ref.Provider = binding.Provider
	}
	return ref, binding, nil
}

func (d *SessionDirectory) Remove(ctx context.Context, sessionID string, kind domain.SessionRuntimeKind) error {
	ref, err := d.store.GetProviderSessionBySession(ctx, sessionID, kind)
	if err != nil {
		return err
	}
	ref.Status = domain.ProviderRuntimeStatusClosed
	ref.ClosedAt = time.Now().UTC()
	return d.store.UpsertProviderSession(ctx, ref)
}

func mergeJSONPayload(existingJSON string, resumeCursor map[string]any, runtimePayload map[string]any) string {
	payload := map[string]any{}
	if existingJSON != "" {
		_ = json.Unmarshal([]byte(existingJSON), &payload)
	}
	if resumeCursor != nil {
		payload["resume_cursor"] = resumeCursor
	}
	for key, value := range runtimePayload {
		payload[key] = value
	}
	if len(payload) == 0 {
		return ""
	}
	b, _ := json.Marshal(payload)
	return string(b)
}
