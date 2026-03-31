package handoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/migration"
	"github.com/brianmeier/estuary/internal/store"
)

// Service generates and persists HandoffPackets used when switching model or
// provider within a session. It delegates conversation extraction to the
// migration.Service, which already holds the extraction helpers.
type Service struct {
	store  *store.Store
	migSvc *migration.Service
}

func NewService(st *store.Store) *Service {
	return &Service{
		store:  st,
		migSvc: migration.NewService(st),
	}
}

// Generate creates a HandoffPacket for the current session targeting a new
// model and provider. It uses BuildCheckpoint (no extra DB write) and persists
// only the packet itself.
func (s *Service) Generate(ctx context.Context, session domain.Session, targetModel string, targetProvider domain.Habitat, switchType domain.SwitchType) (domain.HandoffPacket, error) {
	checkpoint, err := s.migSvc.BuildCheckpoint(ctx, session, nil)
	if err != nil {
		return domain.HandoffPacket{}, fmt.Errorf("checkpoint: %w", err)
	}

	packet := domain.HandoffPacket{
		MigrationCheckpoint: checkpoint,
		RecentWorkSummary:   checkpoint.ConversationSummary,
		SourceModel:         session.CurrentModel,
		SourceProvider:      session.CurrentHabitat,
		TargetModel:         targetModel,
		TargetProvider:      targetProvider,
		SwitchType:          switchType,
	}

	if err := s.store.SaveHandoffPacket(ctx, packet); err != nil {
		return domain.HandoffPacket{}, fmt.Errorf("save: %w", err)
	}
	return packet, nil
}

// InjectionText formats a HandoffPacket as a brief natural-language prompt
// suitable for injection into a new PTY session via stdin. The text is sent
// after the provider CLI initialises so it arrives as the first user message.
func InjectionText(packet domain.HandoffPacket) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Estuary context handoff: %s/%s → %s/%s]\n",
		packet.SourceProvider, packet.SourceModel,
		packet.TargetProvider, packet.TargetModel)
	if packet.ActiveObjective != "" {
		fmt.Fprintf(&b, "Active objective: %s\n", packet.ActiveObjective)
	}
	if packet.ConversationSummary != "" {
		fmt.Fprintf(&b, "Context: %s\n", packet.ConversationSummary)
	}
	if len(packet.ImportantDecisions) > 0 {
		fmt.Fprintf(&b, "Key decisions: %s\n", strings.Join(packet.ImportantDecisions, "; "))
	}
	if len(packet.OpenTasks) > 0 {
		fmt.Fprintf(&b, "Open tasks: %s\n", strings.Join(packet.OpenTasks, "; "))
	}
	b.WriteString("Please continue where the previous session left off.")
	return b.String()
}
