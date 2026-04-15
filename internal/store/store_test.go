package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/brianmeier/estuary/internal/domain"
)

func TestOpenMigratesLegacyHandoffPacketsTable(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "estuary.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE handoff_packets (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			active_objective TEXT NOT NULL,
			recent_summary TEXT NOT NULL,
			open_tasks_json TEXT NOT NULL DEFAULT '[]',
			important_decisions_json TEXT NOT NULL DEFAULT '[]',
			references_json TEXT NOT NULL DEFAULT '[]',
			source_provider TEXT NOT NULL,
			source_model TEXT NOT NULL,
			target_provider TEXT NOT NULL,
			target_model TEXT NOT NULL,
			boundary_profile_id TEXT NOT NULL,
			user_note TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		);
		INSERT INTO handoff_packets
			(id, session_id, active_objective, recent_summary, source_provider, source_model, target_provider, target_model, boundary_profile_id, created_at)
		VALUES
			('legacy-packet', 'session-1', 'old objective', 'old summary', 'claude', 'claude-sonnet-4-6', 'codex', 'gpt-5.4', 'default', '2000-01-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	st, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	packet := domain.HandoffPacket{
		MigrationCheckpoint: domain.MigrationCheckpoint{
			ID:        "packet-1",
			SessionID: "session-1",
			CreatedAt: time.Now().UTC(),
		},
		SourceModel:    "claude-sonnet-4-6",
		SourceProvider: domain.HabitatClaude,
		TargetModel:    "gpt-5.4",
		TargetProvider: domain.HabitatCodex,
		SwitchType:     domain.SwitchTypeCrossProvider,
		UserNote:       "this conversation is about bananas",
	}
	if err := st.SaveHandoffPacket(ctx, packet); err != nil {
		t.Fatalf("SaveHandoffPacket after migration: %v", err)
	}

	got, ok, err := st.LatestHandoffForSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("LatestHandoffForSession: %v", err)
	}
	if !ok {
		t.Fatal("LatestHandoffForSession ok=false, want true")
	}
	if got.ID != "packet-1" || got.UserNote != "this conversation is about bananas" {
		t.Fatalf("latest handoff = %#v, want saved packet", got)
	}
}
