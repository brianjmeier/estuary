package handoff

import (
	"strings"
	"testing"
	"time"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestExtractFileReferences(t *testing.T) {
	messages := []domain.Message{
		{Content: "review /tmp/foo.rs and internal/app/terminal_session.go:42 before touching README.md"},
	}

	got := extractFileReferences(messages)

	if !containsString(got, "/tmp/foo.rs") {
		t.Fatalf("expected absolute file path in %v", got)
	}
	if !containsString(got, "internal/app/terminal_session.go:42") {
		t.Fatalf("expected repo file reference in %v", got)
	}
	if !containsString(got, "README.md") {
		t.Fatalf("expected markdown file reference in %v", got)
	}
}

func TestInjectionTextIncludesStructuredContext(t *testing.T) {
	packet := domain.HandoffPacket{
		MigrationCheckpoint: domain.MigrationCheckpoint{
			FolderPath:          "/Users/brianmeier/dev/agenator",
			ActiveObjective:     "Fix scrolling and model switching",
			ImportantDecisions:  []string{"remove in-band chrome", "keep Ctrl+K minimal"},
			OpenTasks:           []string{"port leader mode", "improve handoff"},
			RecentToolOutputs:   []string{"go test ./internal/app"},
			ConversationSummary: "user wants simpler PTY ownership",
			CreatedAt:           time.Now(),
		},
		RecentWorkSummary: "user wants simpler PTY ownership",
		FileReferences:    []string{"internal/app/terminal_session.go", "internal/handoff/service.go"},
		SourceModel:       "claude-sonnet-4-6",
		SourceProvider:    domain.HabitatClaude,
		TargetModel:       "gpt-5.4",
		TargetProvider:    domain.HabitatCodex,
	}

	got := InjectionText(packet)

	checks := []string{
		"Estuary session handoff.",
		"Working directory: /Users/brianmeier/dev/agenator",
		"Active objective: Fix scrolling and model switching",
		"Important decisions: remove in-band chrome; keep Ctrl+K minimal",
		"Open tasks: port leader mode; improve handoff",
		"Relevant files: internal/app/terminal_session.go, internal/handoff/service.go",
		"Continue this exact session from the supplied context.",
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("InjectionText() missing %q in %q", check, got)
		}
	}
}
