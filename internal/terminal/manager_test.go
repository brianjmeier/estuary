package terminal

import (
	"context"
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

func TestStartFeatureSessionPersistsSeparateTerminalSession(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	manager := NewManager(st)
	item, err := manager.StartFeatureSession(ctx, domain.Session{
		ID:             "session-1",
		CurrentHabitat: domain.HabitatClaude,
	}, "claude_agent_feature", map[string]any{"mode": "team"})
	if err != nil {
		t.Fatalf("start feature session: %v", err)
	}
	if item.FeatureKey != "claude_agent_feature" {
		t.Fatalf("feature key = %q", item.FeatureKey)
	}
	if item.Status != domain.ProviderRuntimeStatusReady {
		t.Fatalf("status = %q", item.Status)
	}
	attached, err := manager.AttachFeatureSession(ctx, domain.Session{ID: "session-1"}, "claude_agent_feature")
	if err != nil {
		t.Fatalf("attach feature session: %v", err)
	}
	if attached.ID != item.ID {
		t.Fatalf("attached session = %q, want %q", attached.ID, item.ID)
	}
	if err := manager.WriteFeatureInput(ctx, item.ID, "continue"); err != nil {
		t.Fatalf("write feature input: %v", err)
	}
	updated, err := manager.AttachFeatureSession(ctx, domain.Session{ID: "session-1"}, "claude_agent_feature")
	if err != nil {
		t.Fatalf("reattach feature session: %v", err)
	}
	if updated.Status != domain.ProviderRuntimeStatusRunning {
		t.Fatalf("updated status = %q", updated.Status)
	}
	if err := manager.CloseFeatureSession(ctx, item.ID); err != nil {
		t.Fatalf("close feature session: %v", err)
	}
}
