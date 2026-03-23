package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

type Adapter interface {
	StartProviderSession(context.Context, domain.Session) (domain.ProviderSessionRef, domain.ProviderProcessState, error)
	ResumeProviderSession(context.Context, domain.Session, domain.ProviderSessionRef) (domain.ProviderSessionRef, domain.ProviderProcessState, error)
	SendProviderTurn(context.Context, domain.Session, domain.ProviderSessionRef, string, func(domain.TurnEvent) error) (domain.ProviderSessionRef, domain.ProviderProcessState, error)
	InterruptProviderTurn(context.Context, domain.Session, domain.ProviderSessionRef, string) error
	CloseProviderSession(context.Context, domain.ProviderSessionRef) error
}

type SessionManager struct {
	store     *store.Store
	directory *SessionDirectory
	adapters  map[domain.Habitat]Adapter
}

func NewSessionManager(store *store.Store, adapters map[domain.Habitat]Adapter) *SessionManager {
	return &SessionManager{store: store, directory: NewSessionDirectory(store), adapters: adapters}
}

func (m *SessionManager) EnsureSession(ctx context.Context, session domain.Session) (domain.ProviderSessionRef, error) {
	ref, err := m.store.GetProviderSessionBySession(ctx, session.ID, domain.SessionRuntimeKindProviderSession)
	if err == nil {
		if strings.TrimSpace(ref.ProviderSessionID) == "" && strings.TrimSpace(ref.ProviderThreadID) == "" {
			adapter, adapterErr := m.adapter(session.CurrentHabitat)
			if adapterErr != nil {
				return domain.ProviderSessionRef{}, adapterErr
			}
			ref, processState, startErr := adapter.StartProviderSession(ctx, session)
			if startErr != nil {
				return ref, startErr
			}
			if persistErr := m.persistProviderState(ctx, session, ref, processState); persistErr != nil {
				return domain.ProviderSessionRef{}, persistErr
			}
			_, _ = m.directory.Upsert(ctx, RuntimeBinding{
				SessionID:   session.ID,
				Provider:    session.CurrentHabitat,
				AdapterKey:  string(session.CurrentHabitat),
				RuntimeKind: domain.SessionRuntimeKindProviderSession,
				Status:      ref.Status,
				ResumeCursor: map[string]any{
					"provider_session_id": ref.ProviderSessionID,
					"provider_thread_id":  ref.ProviderThreadID,
				},
				RuntimePayload: map[string]any{
					"cwd":   session.FolderPath,
					"model": session.CurrentModel,
				},
			})
			return ref, nil
		}
		if ref.Status == domain.ProviderRuntimeStatusReady || ref.Status == domain.ProviderRuntimeStatusRunning {
			return ref, nil
		}
		return m.ResumeSession(ctx, session)
	}
	ref = domain.ProviderSessionRef{
		ID:          uuid.NewString(),
		SessionID:   session.ID,
		Provider:    session.CurrentHabitat,
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      domain.ProviderRuntimeStatusConnecting,
		StartedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := m.store.UpsertProviderSession(ctx, ref); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	if err := m.store.SetActiveProviderSession(ctx, session.ID, ref.ID, session.NativeSessionID); err != nil {
		return domain.ProviderSessionRef{}, err
	}

	adapter, err := m.adapter(session.CurrentHabitat)
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref, processState, err := adapter.StartProviderSession(ctx, session)
	if err != nil {
		ref.LastError = err.Error()
		ref.Status = domain.ProviderRuntimeStatusError
		_ = m.store.UpsertProviderSession(ctx, ref)
		return ref, err
	}
	if err := m.persistProviderState(ctx, session, ref, processState); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	_, _ = m.directory.Upsert(ctx, RuntimeBinding{
		SessionID:   session.ID,
		Provider:    session.CurrentHabitat,
		AdapterKey:  string(session.CurrentHabitat),
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      ref.Status,
		ResumeCursor: map[string]any{
			"provider_session_id": ref.ProviderSessionID,
			"provider_thread_id":  ref.ProviderThreadID,
		},
		RuntimePayload: map[string]any{
			"cwd":   session.FolderPath,
			"model": session.CurrentModel,
		},
	})
	_ = m.store.AppendEvent(ctx, session.ID, "provider.session.started", map[string]any{
		"provider":            session.CurrentHabitat,
		"provider_session_id": ref.ProviderSessionID,
		"provider_thread_id":  ref.ProviderThreadID,
	})
	return ref, nil
}

func (m *SessionManager) ResumeSession(ctx context.Context, session domain.Session) (domain.ProviderSessionRef, error) {
	ref, err := m.store.GetProviderSessionBySession(ctx, session.ID, domain.SessionRuntimeKindProviderSession)
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref, binding, err := m.directory.Apply(ctx, ref)
	if err != nil {
		binding = RuntimeBinding{}
	}
	habitat := session.CurrentHabitat
	if binding.Provider != "" {
		habitat = binding.Provider
	}
	adapter, err := m.adapter(habitat)
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref.Status = domain.ProviderRuntimeStatusConnecting
	if err := m.store.UpsertProviderSession(ctx, ref); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	if habitat != session.CurrentHabitat {
		session.CurrentHabitat = habitat
	}
	ref, processState, err := adapter.ResumeProviderSession(ctx, session, ref)
	if err != nil {
		ref.LastError = err.Error()
		ref.Status = domain.ProviderRuntimeStatusDegraded
		_ = m.store.UpsertProviderSession(ctx, ref)
		_ = m.store.AppendEvent(ctx, session.ID, "provider.session.resume_failed", map[string]any{"error": err.Error()})
		return ref, err
	}
	if err := m.persistProviderState(ctx, session, ref, processState); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	_, _ = m.directory.Upsert(ctx, RuntimeBinding{
		SessionID:   session.ID,
		Provider:    session.CurrentHabitat,
		AdapterKey:  string(session.CurrentHabitat),
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      ref.Status,
		ResumeCursor: map[string]any{
			"provider_session_id": ref.ProviderSessionID,
			"provider_thread_id":  ref.ProviderThreadID,
		},
		RuntimePayload: map[string]any{
			"cwd":   session.FolderPath,
			"model": session.CurrentModel,
		},
	})
	_ = m.store.AppendEvent(ctx, session.ID, "provider.session.restored", map[string]any{"provider_session_id": ref.ProviderSessionID})
	return ref, nil
}

func (m *SessionManager) SendTurn(ctx context.Context, session domain.Session, prompt string, emit func(domain.TurnEvent) error) (domain.ProviderSessionRef, error) {
	ref, err := m.EnsureSession(ctx, session)
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	adapter, err := m.adapter(session.CurrentHabitat)
	if err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref.Status = domain.ProviderRuntimeStatusRunning
	if err := m.store.UpsertProviderSession(ctx, ref); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref, processState, err := adapter.SendProviderTurn(ctx, session, ref, prompt, emit)
	if err != nil {
		ref.LastError = err.Error()
		if resumeRejected(err) {
			ref.Status = domain.ProviderRuntimeStatusDegraded
		} else {
			ref.Status = domain.ProviderRuntimeStatusError
		}
		_ = m.persistProviderState(ctx, session, ref, processState)
		_, _ = m.directory.Upsert(ctx, RuntimeBinding{
			SessionID:   session.ID,
			Provider:    session.CurrentHabitat,
			AdapterKey:  string(session.CurrentHabitat),
			RuntimeKind: domain.SessionRuntimeKindProviderSession,
			Status:      ref.Status,
			ResumeCursor: map[string]any{
				"provider_session_id": ref.ProviderSessionID,
				"provider_thread_id":  ref.ProviderThreadID,
			},
			RuntimePayload: map[string]any{
				"cwd":        session.FolderPath,
				"model":      session.CurrentModel,
				"last_error": ref.LastError,
			},
		})
		return ref, err
	}
	ref.Status = domain.ProviderRuntimeStatusReady
	ref.LastError = ""
	if err := m.persistProviderState(ctx, session, ref, processState); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	_, _ = m.directory.Upsert(ctx, RuntimeBinding{
		SessionID:   session.ID,
		Provider:    session.CurrentHabitat,
		AdapterKey:  string(session.CurrentHabitat),
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      ref.Status,
		ResumeCursor: map[string]any{
			"provider_session_id": ref.ProviderSessionID,
			"provider_thread_id":  ref.ProviderThreadID,
		},
		RuntimePayload: map[string]any{
			"cwd":   session.FolderPath,
			"model": session.CurrentModel,
		},
	})
	return ref, nil
}

func (m *SessionManager) InterruptTurn(ctx context.Context, session domain.Session, turnID string) error {
	ref, err := m.store.GetProviderSessionBySession(ctx, session.ID, domain.SessionRuntimeKindProviderSession)
	if err != nil {
		return err
	}
	adapter, err := m.adapter(session.CurrentHabitat)
	if err != nil {
		return err
	}
	return adapter.InterruptProviderTurn(ctx, session, ref, turnID)
}

func (m *SessionManager) CloseSession(ctx context.Context, sessionID string) error {
	session, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	ref, err := m.store.GetProviderSessionBySession(ctx, sessionID, domain.SessionRuntimeKindProviderSession)
	if err != nil {
		return nil
	}
	adapter, err := m.adapter(session.CurrentHabitat)
	if err != nil {
		return err
	}
	if err := adapter.CloseProviderSession(ctx, ref); err != nil {
		return err
	}
	ref.Status = domain.ProviderRuntimeStatusClosed
	ref.ClosedAt = time.Now().UTC()
	if err := m.store.UpsertProviderSession(ctx, ref); err != nil {
		return err
	}
	return m.directory.Remove(ctx, sessionID, domain.SessionRuntimeKindProviderSession)
}

func (m *SessionManager) WarmRestore(ctx context.Context) error {
	sessions, err := m.store.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		state, err := m.store.LoadSessionRuntimeState(ctx, session.ID)
		if err != nil || !state.Active || session.ActiveProviderSessionID == "" {
			continue
		}
		_, _ = m.ResumeSession(ctx, session)
	}
	return nil
}

func (m *SessionManager) persistProviderState(ctx context.Context, session domain.Session, ref domain.ProviderSessionRef, processState domain.ProviderProcessState) error {
	if ref.ID == "" {
		ref.ID = uuid.NewString()
	}
	if err := m.store.UpsertProviderSession(ctx, ref); err != nil {
		return err
	}
	if err := m.store.SetActiveProviderSession(ctx, session.ID, ref.ID, firstNonEmpty(ref.ProviderSessionID, ref.ProviderThreadID, session.NativeSessionID)); err != nil {
		return err
	}
	processState.ProviderSessionID = ref.ID
	processState.SessionID = session.ID
	processState.Provider = session.CurrentHabitat
	processState.RuntimeKind = domain.SessionRuntimeKindProviderSession
	return m.store.UpsertProviderProcessState(ctx, processState)
}

func (m *SessionManager) adapter(h domain.Habitat) (Adapter, error) {
	adapter := m.adapters[h]
	if adapter == nil {
		return nil, fmt.Errorf("unsupported habitat %q", h)
	}
	return adapter, nil
}

func resumeRejected(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "resume") && (strings.Contains(text, "not found") || strings.Contains(text, "expired") || strings.Contains(text, "invalid"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
