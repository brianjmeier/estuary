package migration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

type Service struct {
	store *store.Store
}

func NewService(st *store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) CreateCheckpoint(ctx context.Context, session domain.Session, activeTraits []domain.Trait) (domain.MigrationCheckpoint, error) {
	messages, err := s.store.ListMessages(ctx, session.ID)
	if err != nil {
		return domain.MigrationCheckpoint{}, err
	}
	events, err := s.store.ListEvents(ctx, session.ID, 20)
	if err != nil {
		return domain.MigrationCheckpoint{}, err
	}

	checkpoint := domain.MigrationCheckpoint{
		ID:                  uuid.NewString(),
		SessionID:           session.ID,
		FolderPath:          session.FolderPath,
		CurrentModel:        session.CurrentModel,
		CurrentHabitat:      session.CurrentHabitat,
		ActiveObjective:     lastUserObjective(messages),
		ImportantDecisions:  collectImportantDecisions(messages),
		ConversationSummary: summarizeMessages(messages),
		OpenTasks:           collectOpenTasks(messages),
		ActiveTraits:        traitNames(activeTraits),
		RecentToolOutputs:   collectToolOutputs(messages),
		HabitatNotes: map[string]string{
			"native_session_id": session.NativeSessionID,
			"recent_events":     summarizeEvents(events),
		},
		CreatedAt: time.Now().UTC(),
	}
	return checkpoint, s.store.CreateMigrationCheckpoint(ctx, checkpoint)
}

func (s *Service) ContinuationText(checkpoint domain.MigrationCheckpoint, nextModel string, nextHabitat domain.Habitat) string {
	lines := []string{
		fmt.Sprintf("Migration checkpoint from %s.", checkpoint.CurrentHabitat),
		fmt.Sprintf("Continue this Estuary session in model %s on habitat %s.", nextModel, nextHabitat),
	}
	if checkpoint.ActiveObjective != "" {
		lines = append(lines, "Active objective: "+checkpoint.ActiveObjective)
	}
	if checkpoint.ConversationSummary != "" {
		lines = append(lines, "Conversation summary: "+checkpoint.ConversationSummary)
	}
	if len(checkpoint.ImportantDecisions) > 0 {
		lines = append(lines, "Important decisions: "+strings.Join(checkpoint.ImportantDecisions, "; "))
	}
	if len(checkpoint.OpenTasks) > 0 {
		lines = append(lines, "Open tasks/questions: "+strings.Join(checkpoint.OpenTasks, "; "))
	}
	if len(checkpoint.ActiveTraits) > 0 {
		lines = append(lines, "Active traits: "+strings.Join(checkpoint.ActiveTraits, ", "))
	}
	if len(checkpoint.RecentToolOutputs) > 0 {
		lines = append(lines, "Recent tool outputs: "+strings.Join(checkpoint.RecentToolOutputs, "; "))
	}
	return strings.Join(lines, "\n")
}

// summaryWindow is the number of recent messages included in migration checkpoints.
// Enough to establish context without bloating the continuation prompt.
const summaryWindow = 8

func summarizeMessages(messages []domain.Message) string {
	if len(messages) == 0 {
		return ""
	}
	start := 0
	if len(messages) > summaryWindow {
		start = len(messages) - summaryWindow
	}
	parts := make([]string, 0, len(messages[start:]))
	for _, msg := range messages[start:] {
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		if len(text) > 180 {
			text = text[:180] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s: %s", msg.Role, text))
	}
	return strings.Join(parts, " | ")
}

func lastUserObjective(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == domain.MessageRoleUser {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func collectImportantDecisions(messages []domain.Message) []string {
	return collectMessageSnippets(messages, domain.MessageRoleAssistant, 3)
}

func collectOpenTasks(messages []domain.Message) []string {
	return collectMessageSnippets(messages, domain.MessageRoleUser, 3)
}

func collectToolOutputs(messages []domain.Message) []string {
	return collectMessageSnippets(messages, domain.MessageRoleTool, 3)
}

func collectMessageSnippets(messages []domain.Message, role domain.MessageRole, limit int) []string {
	var out []string
	for i := len(messages) - 1; i >= 0 && len(out) < limit; i-- {
		if messages[i].Role != role {
			continue
		}
		text := strings.TrimSpace(messages[i].Content)
		if text == "" {
			continue
		}
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		out = append([]string{text}, out...)
	}
	return out
}

func summarizeEvents(events []domain.RuntimeEvent) string {
	if len(events) == 0 {
		return ""
	}
	parts := make([]string, 0, min(4, len(events)))
	for i := len(events) - 1; i >= 0 && len(parts) < 4; i-- {
		parts = append(parts, events[i].EventType)
	}
	return strings.Join(parts, ", ")
}

func traitNames(traits []domain.Trait) []string {
	out := make([]string, 0, len(traits))
	for _, trait := range traits {
		out = append(out, trait.Name)
	}
	return out
}
