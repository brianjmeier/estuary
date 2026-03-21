package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/store"
)

type Service struct {
	store   *store.Store
	runtime *habitats.Runtime
}

type StreamEnvelope struct {
	Event    domain.TurnEvent
	Session  domain.Session
	Messages []domain.Message
	Err      error
	Done     bool
}

func NewService(store *store.Store, runtime *habitats.Runtime) *Service {
	return &Service{store: store, runtime: runtime}
}

func (s *Service) List(ctx context.Context, sessionID string) ([]domain.Message, error) {
	return s.store.ListMessages(ctx, sessionID)
}

func (s *Service) MarkResumeRequested(ctx context.Context, session domain.Session) error {
	state, err := s.store.LoadSessionRuntimeState(ctx, session.ID)
	if err != nil {
		return err
	}
	state.ResumeExplicit = true
	state.LastResumeStatus = "requested"
	state.LastResumeAttemptAt = time.Now().UTC()
	if err := s.store.SaveSessionRuntimeState(ctx, session.ID, state); err != nil {
		return err
	}
	return s.store.AppendEvent(ctx, session.ID, "session.resumed", map[string]any{
		"mode":              "explicit",
		"native_session_id": session.NativeSessionID,
	})
}

func (s *Service) Send(ctx context.Context, session domain.Session, prompt string) (domain.Session, []domain.Message, error) {
	var (
		updated  domain.Session
		messages []domain.Message
	)
	err := s.SendStream(ctx, session, prompt, func(item StreamEnvelope) error {
		if len(item.Messages) > 0 {
			messages = append([]domain.Message(nil), item.Messages...)
		}
		if item.Session.ID != "" {
			updated = item.Session
		}
		return nil
	})
	return updated, messages, err
}

func (s *Service) SendStream(ctx context.Context, session domain.Session, prompt string, emit func(StreamEnvelope) error) error {
	state, err := s.store.LoadSessionRuntimeState(ctx, session.ID)
	if err != nil {
		return err
	}
	finalPrompt := s.applyPendingContext(prompt, state)
	userMessage, err := s.store.CreateMessage(ctx, session.ID, domain.MessageRoleUser, prompt, "user")
	if err != nil {
		return err
	}
	persisted := []domain.Message{userMessage}
	if state.PendingContinuation != "" {
		systemMessage, err := s.store.CreateMessage(ctx, session.ID, domain.MessageRoleSystem, state.PendingContinuation, "migration")
		if err != nil {
			return err
		}
		persisted = append([]domain.Message{systemMessage}, persisted...)
	}

	if err := s.store.UpdateSessionStatus(ctx, session.ID, domain.SessionStatusActive, session.NativeSessionID); err != nil {
		return err
	}
	if err := s.store.AppendEvent(ctx, session.ID, "turn.started", map[string]any{"model": session.CurrentModel, "habitat": session.CurrentHabitat}); err != nil {
		return err
	}
	if state.ResumeExplicit || session.NativeSessionID != "" {
		state.LastResumeAttemptAt = time.Now().UTC()
		state.LastResumeStatus = "attempting"
		if err := s.store.SaveSessionRuntimeState(ctx, session.ID, state); err != nil {
			return err
		}
	}
	if emit != nil {
		if err := emit(StreamEnvelope{
			Event:    domain.TurnEvent{Kind: domain.TurnEventStarted, SessionID: session.ID},
			Session:  session,
			Messages: persisted,
		}); err != nil {
			return err
		}
	}

	var assistantText strings.Builder
	nativeID := session.NativeSessionID
	err = s.runtime.ExecuteTurnStream(ctx, session, finalPrompt, func(event domain.TurnEvent) error {
		event.SessionID = session.ID
		if strings.TrimSpace(event.NativeSessionID) != "" {
			nativeID = event.NativeSessionID
		}
		switch event.Kind {
		case domain.TurnEventDelta:
			assistantText.WriteString(event.Text)
			_ = s.store.AppendEvent(ctx, session.ID, "assistant.delta", map[string]any{"text": event.Text, "native_session_id": event.NativeSessionID})
		case domain.TurnEventToolStarted:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.started", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventToolOutput:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.output", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventToolFinished:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.finished", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventNotice:
			_ = s.store.AppendEvent(ctx, session.ID, "notice", map[string]any{"text": event.Text})
		case domain.TurnEventHabitatError:
			_ = s.store.AppendEvent(ctx, session.ID, "habitat.error", map[string]any{"text": event.Text, "native_session_id": event.NativeSessionID, "metadata": event.Metadata})
		case domain.TurnEventCompleted:
			_ = s.store.AppendEvent(ctx, session.ID, "turn.completed", map[string]any{"native_session_id": event.NativeSessionID})
		}
		if emit != nil {
			return emit(StreamEnvelope{Event: event, Session: session})
		}
		return nil
	})

	if err != nil && session.NativeSessionID != "" && resumeRejected(err) {
		return s.retryWithoutNativeResume(ctx, session, state, prompt, persisted, emit, err)
	}

	return s.finishTurn(ctx, session, state, persisted, assistantText.String(), err, nativeID, emit)
}

func (s *Service) retryWithoutNativeResume(ctx context.Context, session domain.Session, state domain.SessionRuntimeState, prompt string, persisted []domain.Message, emit func(StreamEnvelope) error, originalErr error) error {
	session.NativeSessionID = ""
	state.ResumeExplicit = false
	state.LastResumeStatus = "restore-failed"
	if err := s.store.SaveSessionRuntimeState(ctx, session.ID, state); err != nil {
		return err
	}
	noticeText := "Native session could not be resumed. Estuary restored the transcript and continued without provider-native resume."
	notice, err := s.store.CreateMessage(ctx, session.ID, domain.MessageRoleSystem, noticeText, "estuary")
	if err != nil {
		return err
	}
	persisted = append(persisted, notice)
	_ = s.store.AppendEvent(ctx, session.ID, "session.restore-failed", map[string]any{"error": originalErr.Error()})
	if emit != nil {
		_ = emit(StreamEnvelope{
			Event:    domain.TurnEvent{Kind: domain.TurnEventNotice, SessionID: session.ID, Text: noticeText},
			Messages: persisted,
			Session:  session,
		})
	}

	var assistantText strings.Builder
	nativeID := ""
	err = s.runtime.ExecuteTurnStream(ctx, session, prompt, func(event domain.TurnEvent) error {
		if strings.TrimSpace(event.NativeSessionID) != "" {
			nativeID = event.NativeSessionID
		}
		switch event.Kind {
		case domain.TurnEventDelta:
			assistantText.WriteString(event.Text)
			_ = s.store.AppendEvent(ctx, session.ID, "assistant.delta", map[string]any{"text": event.Text, "native_session_id": event.NativeSessionID})
		case domain.TurnEventToolStarted:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.started", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventToolOutput:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.output", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventToolFinished:
			_ = s.store.AppendEvent(ctx, session.ID, "tool.finished", map[string]any{"tool": event.ToolName, "text": event.Text})
		case domain.TurnEventNotice:
			_ = s.store.AppendEvent(ctx, session.ID, "notice", map[string]any{"text": event.Text})
		case domain.TurnEventHabitatError:
			_ = s.store.AppendEvent(ctx, session.ID, "habitat.error", map[string]any{"text": event.Text, "native_session_id": event.NativeSessionID, "metadata": event.Metadata})
		}
		if emit != nil {
			return emit(StreamEnvelope{Event: event, Session: session})
		}
		return nil
	})
	return s.finishTurn(ctx, session, state, persisted, assistantText.String(), err, nativeID, emit)
}

func (s *Service) finishTurn(ctx context.Context, session domain.Session, state domain.SessionRuntimeState, persisted []domain.Message, assistantText string, runErr error, nativeID string, emit func(StreamEnvelope) error) error {
	status := domain.SessionStatusIdle
	if runErr != nil {
		status = domain.SessionStatusError
	}

	if strings.TrimSpace(nativeID) == "" {
		nativeID = session.NativeSessionID
	}
	if len(persisted) > 0 && persisted[0].Role == domain.MessageRoleSystem && persisted[0].Source == "migration" {
		state.PendingContinuation = ""
		state.PendingCheckpointID = ""
	}
	if runErr == nil {
		state.ResumeExplicit = false
		if nativeID != "" {
			state.LastResumeStatus = "resumed"
		}
	}

	messages := append([]domain.Message(nil), persisted...)
	if runErr != nil {
		state.LastResumeStatus = fallbackResumeStatus(state.LastResumeStatus, "error")
		errMsg, err := s.store.CreateMessage(ctx, session.ID, domain.MessageRoleSystem, fmt.Sprintf("Habitat error: %v", runErr), "provider")
		if err != nil {
			return err
		}
		messages = append(messages, errMsg)
	}

	if strings.TrimSpace(assistantText) != "" {
		msg, err := s.store.CreateMessage(ctx, session.ID, domain.MessageRoleAssistant, strings.TrimSpace(assistantText), "provider")
		if err != nil {
			return err
		}
		messages = append(messages, msg)
	}

	if err := s.store.SaveSessionRuntimeState(ctx, session.ID, state); err != nil {
		return err
	}
	if err := s.store.UpdateSessionStatus(ctx, session.ID, status, nativeID); err != nil {
		return err
	}
	updatedSession, err := s.store.GetSession(ctx, session.ID)
	if err != nil {
		return err
	}

	if emit != nil {
		if err := emit(StreamEnvelope{
			Event:    domain.TurnEvent{Kind: domain.TurnEventCompleted, SessionID: session.ID, NativeSessionID: nativeID, Text: strings.TrimSpace(assistantText)},
			Session:  updatedSession,
			Messages: messages,
			Err:      runErr,
			Done:     true,
		}); err != nil {
			return err
		}
	}
	return runErr
}

func (s *Service) applyPendingContext(prompt string, state domain.SessionRuntimeState) string {
	if strings.TrimSpace(state.PendingContinuation) == "" {
		return prompt
	}
	return state.PendingContinuation + "\n\nUser request:\n" + prompt
}

func resumeRejected(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "resume") && (strings.Contains(text, "not found") || strings.Contains(text, "invalid") || strings.Contains(text, "expired"))
}

func fallbackResumeStatus(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
