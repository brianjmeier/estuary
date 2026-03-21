package habitats

import (
	"context"
	"fmt"
	"strings"

	"github.com/brianmeier/estuary/internal/domain"
	clauderuntime "github.com/brianmeier/estuary/internal/habitats/claude"
	codexruntime "github.com/brianmeier/estuary/internal/habitats/codex"
)

type Runtime struct {
	claude *clauderuntime.Runtime
	codex  *codexruntime.Runtime
}

type TurnResult struct {
	NativeSessionID string
	AssistantText   string
	Notices         []string
	ResumeRejected  bool
}

func NewRuntime() *Runtime {
	return &Runtime{
		claude: clauderuntime.New(),
		codex:  codexruntime.New(),
	}
}

func (r *Runtime) ExecuteTurn(ctx context.Context, session domain.Session, prompt string) (TurnResult, error) {
	var result TurnResult
	err := r.ExecuteTurnStream(ctx, session, prompt, func(event domain.TurnEvent) error {
		result.NativeSessionID = firstNonEmpty(event.NativeSessionID, result.NativeSessionID)
		switch event.Kind {
		case domain.TurnEventDelta:
			result.AssistantText += event.Text
		case domain.TurnEventNotice:
			if strings.TrimSpace(event.Text) != "" {
				result.Notices = append(result.Notices, event.Text)
			}
		case domain.TurnEventCompleted:
			if text := strings.TrimSpace(event.Text); text != "" {
				result.AssistantText = text
			}
		case domain.TurnEventHabitatError:
			if event.Metadata["resume_rejected"] == "true" {
				result.ResumeRejected = true
			}
		}
		return nil
	})
	return result, err
}

func (r *Runtime) ExecuteTurnStream(ctx context.Context, session domain.Session, prompt string, emit func(domain.TurnEvent) error) error {
	switch session.CurrentHabitat {
	case domain.HabitatClaude:
		return r.claude.ExecuteTurnStream(ctx, session, prompt, emit)
	case domain.HabitatCodex:
		return r.codex.ExecuteTurnStream(ctx, session, prompt, emit)
	default:
		return fmt.Errorf("unsupported habitat %q", session.CurrentHabitat)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

