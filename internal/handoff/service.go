package handoff

import (
	"context"
	"fmt"
	"regexp"
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
	messages, err := s.store.ListMessages(ctx, session.ID)
	if err != nil {
		return domain.HandoffPacket{}, fmt.Errorf("messages: %w", err)
	}

	packet := domain.HandoffPacket{
		MigrationCheckpoint: checkpoint,
		RecentWorkSummary:   checkpoint.ConversationSummary,
		FileReferences:      extractFileReferences(messages),
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
	fmt.Fprintf(&b, "Estuary session handoff.\n")
	fmt.Fprintf(&b, "Source: %s/%s\n",
		packet.SourceProvider, packet.SourceModel,
	)
	fmt.Fprintf(&b, "Target: %s/%s\n",
		packet.TargetProvider, packet.TargetModel)
	if packet.FolderPath != "" {
		fmt.Fprintf(&b, "Working directory: %s\n", packet.FolderPath)
	}
	if packet.ActiveObjective != "" {
		fmt.Fprintf(&b, "Active objective: %s\n", packet.ActiveObjective)
	}
	if packet.RecentWorkSummary != "" {
		fmt.Fprintf(&b, "Recent context: %s\n", packet.RecentWorkSummary)
	}
	if len(packet.ImportantDecisions) > 0 {
		fmt.Fprintf(&b, "Important decisions: %s\n", strings.Join(packet.ImportantDecisions, "; "))
	}
	if len(packet.OpenTasks) > 0 {
		fmt.Fprintf(&b, "Open tasks: %s\n", strings.Join(packet.OpenTasks, "; "))
	}
	if len(packet.FileReferences) > 0 {
		fmt.Fprintf(&b, "Relevant files: %s\n", strings.Join(packet.FileReferences, ", "))
	}
	if len(packet.RecentToolOutputs) > 0 {
		fmt.Fprintf(&b, "Recent tool outputs: %s\n", strings.Join(packet.RecentToolOutputs, "; "))
	}
	if packet.UserNote != "" {
		fmt.Fprintf(&b, "Operator note: %s\n", packet.UserNote)
	}
	b.WriteString("Continue this exact session from the supplied context. Preserve prior decisions and start from the open tasks instead of restarting the analysis.")
	return b.String()
}

var fileReferencePattern = regexp.MustCompile(`(?:/[A-Za-z0-9._/\-]+|[A-Za-z0-9._\-]+(?:/[A-Za-z0-9._\-]+)+|[A-Za-z0-9._\-]+\.[A-Za-z0-9._\-]+)(?::\d+)?`)

func extractFileReferences(messages []domain.Message) []string {
	if len(messages) == 0 {
		return nil
	}

	var refs []string
	for i := len(messages) - 1; i >= 0 && len(refs) < 8; i-- {
		for _, candidate := range fileReferencePattern.FindAllString(messages[i].Content, -1) {
			candidate = strings.Trim(candidate, " \t\r\n,.;:()[]{}<>")
			if candidate == "" || !looksLikeFileReference(candidate) || containsString(refs, candidate) {
				continue
			}
			refs = append(refs, candidate)
			if len(refs) >= 8 {
				break
			}
		}
	}
	return refs
}

func looksLikeFileReference(candidate string) bool {
	return strings.Contains(candidate, "/") ||
		strings.HasSuffix(candidate, ".go") ||
		strings.HasSuffix(candidate, ".rs") ||
		strings.HasSuffix(candidate, ".md") ||
		strings.HasSuffix(candidate, ".json") ||
		strings.HasSuffix(candidate, ".yaml") ||
		strings.HasSuffix(candidate, ".yml") ||
		strings.HasSuffix(candidate, ".toml")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
