package providers

import (
	"context"
	"testing"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/store"
)

func TestSessionDirectoryUpsertsResumeCursorAndPayload(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	dir := NewSessionDirectory(st)
	_, err = dir.Upsert(ctx, RuntimeBinding{
		SessionID:   "session-1",
		Provider:    domain.HabitatClaude,
		AdapterKey:  "claude-sdk",
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
		Status:      domain.ProviderRuntimeStatusReady,
		ResumeCursor: map[string]any{
			"resume": "claude-session-1",
		},
		RuntimePayload: map[string]any{
			"cwd": "/tmp/workspace",
		},
	})
	if err != nil {
		t.Fatalf("upsert binding: %v", err)
	}

	binding, err := dir.Get(ctx, "session-1", domain.SessionRuntimeKindProviderSession)
	if err != nil {
		t.Fatalf("get binding: %v", err)
	}
	if binding.AdapterKey != "claude-sdk" {
		t.Fatalf("adapter key = %q", binding.AdapterKey)
	}
	if binding.ResumeCursor["resume"] != "claude-session-1" {
		t.Fatalf("resume cursor = %#v", binding.ResumeCursor)
	}
	if binding.RuntimePayload["cwd"] != "/tmp/workspace" {
		t.Fatalf("runtime payload = %#v", binding.RuntimePayload)
	}

	ref, _, err := dir.Apply(ctx, domain.ProviderSessionRef{
		SessionID:   "session-1",
		RuntimeKind: domain.SessionRuntimeKindProviderSession,
	})
	if err != nil {
		t.Fatalf("apply binding: %v", err)
	}
	if ref.ProviderResumeCursorJSON == "" {
		t.Fatal("expected provider resume cursor json to be populated")
	}
	if ref.ProviderSessionID != "claude-session-1" {
		t.Fatalf("provider session id = %q", ref.ProviderSessionID)
	}
}
