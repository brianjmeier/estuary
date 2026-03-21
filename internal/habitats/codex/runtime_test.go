package codex

import (
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestParserCapturesThreadAndAssistantText(t *testing.T) {
	p := &parser{sessionID: "s1"}
	if err := p.handleLine(`{"type":"thread.started","thread_id":"abc-123"}`, nil); err != nil {
		t.Fatalf("thread started: %v", err)
	}
	if err := p.handleLine(`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"hello from codex"}}`, nil); err != nil {
		t.Fatalf("item completed: %v", err)
	}
	if p.nativeSessionID != "abc-123" {
		t.Fatalf("thread id = %q", p.nativeSessionID)
	}
	if p.assistantText != "hello from codex" {
		t.Fatalf("assistant text = %q", p.assistantText)
	}
}

func TestParserEmitsDelta(t *testing.T) {
	p := &parser{sessionID: "s1"}
	var seen domain.TurnEvent
	err := p.handleLine(`{"type":"agent_message_delta","delta":"hello"}`, func(event domain.TurnEvent) error {
		seen = event
		return nil
	})
	if err != nil {
		t.Fatalf("delta: %v", err)
	}
	if seen.Kind != domain.TurnEventDelta || seen.Text != "hello" {
		t.Fatalf("unexpected event: %+v", seen)
	}
}
