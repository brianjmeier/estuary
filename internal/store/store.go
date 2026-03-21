package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/brianmeier/estuary/internal/boundaries"
	"github.com/brianmeier/estuary/internal/domain"
)

type Store struct {
	db *sql.DB
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".estuary", "data"), nil
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "estuary.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	st := &Store{db: db}
	if err := st.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.seedBoundaryProfiles(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.seedAppSettings(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return st, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
	}
	return nil
}

func (s *Store) seedBoundaryProfiles(ctx context.Context) error {
	for _, profile := range boundaries.DefaultProfiles() {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO boundary_profiles
				(id, name, description, policy_level, file_access_policy, command_execution_policy, network_tool_policy, default_approval_behavior, habitat_override_json, compatibility_notes, unsafe)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				policy_level = excluded.policy_level,
				file_access_policy = excluded.file_access_policy,
				command_execution_policy = excluded.command_execution_policy,
				network_tool_policy = excluded.network_tool_policy,
				default_approval_behavior = excluded.default_approval_behavior,
				habitat_override_json = excluded.habitat_override_json,
				compatibility_notes = excluded.compatibility_notes,
				unsafe = excluded.unsafe
		`, string(profile.ID), profile.Name, profile.Description, string(profile.PolicyLevel), string(profile.FileAccessPolicy), string(profile.CommandExecution), string(profile.NetworkToolPolicy), string(profile.DefaultApproval), profile.HabitatOverrideJSON, profile.CompatibilityNotes, profile.Unsafe); err != nil {
			return fmt.Errorf("seed boundary profile %s: %w", profile.ID, err)
		}
	}
	return nil
}

func (s *Store) seedAppSettings(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ('theme', 'dark', ?)
		ON CONFLICT(key) DO NOTHING
	`, time.Now().UTC())
	return err
}

func (s *Store) LoadAppSettings(ctx context.Context) (domain.AppSettings, error) {
	settings := domain.AppSettings{Theme: "dark"}
	row := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'theme'`)
	if err := row.Scan(&settings.Theme); err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return domain.AppSettings{}, err
	}
	return settings, nil
}

func (s *Store) SaveAppSettings(ctx context.Context, settings domain.AppSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ('theme', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, settings.Theme, time.Now().UTC())
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(s scanner) (domain.Session, error) {
	var session domain.Session
	var currentHabitat, boundaryProfile, status string
	if err := s.Scan(&session.ID, &session.Title, &session.FolderPath, &session.CurrentModel, &currentHabitat, &session.NativeSessionID, &boundaryProfile, &session.ResolvedBoundarySettings, &status, &session.MigrationGeneration, &session.CreatedAt, &session.UpdatedAt, &session.LastOpenedAt); err != nil {
		return domain.Session{}, err
	}
	session.CurrentHabitat = domain.Habitat(currentHabitat)
	session.ModelDescriptor = domain.ModelDescriptor{
		ID:      session.CurrentModel,
		Label:   session.CurrentModel,
		Habitat: session.CurrentHabitat,
	}
	session.BoundaryProfile = domain.ProfileID(boundaryProfile)
	session.Status = domain.SessionStatus(status)
	return session, nil
}

func (s *Store) ListSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, folder_path, current_model, current_habitat, native_session_id, boundary_profile_id, resolved_boundary_settings, status, migration_generation, created_at, updated_at, last_opened_at
		FROM sessions
		ORDER BY last_opened_at DESC, updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var sessions []domain.Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) GetSession(ctx context.Context, sessionID string) (domain.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, folder_path, current_model, current_habitat, native_session_id, boundary_profile_id, resolved_boundary_settings, status, migration_generation, created_at, updated_at, last_opened_at
		FROM sessions
		WHERE id = ?
	`, sessionID)
	return scanSession(row)
}

func (s *Store) CreateSession(ctx context.Context, draft domain.SessionDraft, habitat domain.Habitat, resolution domain.BoundaryResolution) (domain.Session, error) {
	now := time.Now().UTC()

	session := domain.Session{
		ID:                       uuid.NewString(),
		Title:                    filepath.Base(draft.FolderPath),
		FolderPath:               draft.FolderPath,
		CurrentModel:             draft.Model,
		ModelDescriptor:          domain.ModelDescriptor{ID: draft.Model, Label: draft.Model, Habitat: habitat},
		CurrentHabitat:           habitat,
		BoundaryProfile:          domain.ProfileID(draft.BoundaryProfile),
		ResolvedBoundarySettings: resolution.NativeSettings,
		Status:                   domain.SessionStatusIdle,
		CreatedAt:                now,
		UpdatedAt:                now,
		LastOpenedAt:             now,
	}
	if session.Title == "." || session.Title == string(filepath.Separator) || session.Title == "" {
		session.Title = draft.FolderPath
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, title, folder_path, current_model, current_habitat, native_session_id, boundary_profile_id, resolved_boundary_settings, status, migration_generation, created_at, updated_at, last_opened_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?, ?, 0, ?, ?, ?)
	`, session.ID, session.Title, session.FolderPath, session.CurrentModel, string(session.CurrentHabitat), string(session.BoundaryProfile), session.ResolvedBoundarySettings, string(session.Status), session.CreatedAt, session.UpdatedAt, session.LastOpenedAt)
	if err != nil {
		return domain.Session{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_boundary_resolutions (session_id, profile_id, habitat, compatibility, summary, native_settings)
		VALUES (?, ?, ?, ?, ?, ?)
	`, session.ID, string(resolution.ProfileID), string(resolution.Habitat), string(resolution.Compatibility), resolution.Summary, resolution.NativeSettings)
	if err != nil {
		return domain.Session{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO events (id, session_id, event_type, payload, created_at)
		VALUES (?, ?, 'session.created', ?, ?)
	`, uuid.NewString(), session.ID, fmt.Sprintf(`{"folder_path":%q,"model":%q,"habitat":%q}`, session.FolderPath, session.CurrentModel, session.CurrentHabitat), now)
	if err != nil {
		return domain.Session{}, err
	}

	return session, nil
}

func (s *Store) UpdateSessionStatus(ctx context.Context, sessionID string, status domain.SessionStatus, nativeSessionID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET status = ?, native_session_id = ?, updated_at = ?, last_opened_at = ?
		WHERE id = ?
	`, string(status), nativeSessionID, now, now, sessionID)
	return err
}

func (s *Store) UpdateSession(ctx context.Context, session domain.Session) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET title = ?, folder_path = ?, current_model = ?, current_habitat = ?, native_session_id = ?, boundary_profile_id = ?, resolved_boundary_settings = ?, status = ?, migration_generation = ?, updated_at = ?, last_opened_at = ?
		WHERE id = ?
	`, session.Title, session.FolderPath, session.CurrentModel, string(session.CurrentHabitat), session.NativeSessionID, string(session.BoundaryProfile), session.ResolvedBoundarySettings, string(session.Status), session.MigrationGeneration, now, now, session.ID)
	return err
}

func (s *Store) CountSessionsByFolder(ctx context.Context, folderPath string) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE folder_path = ?`, folderPath)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountActiveSessionsByFolder(ctx context.Context, folderPath string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(sr.state_json, '{}')
		FROM sessions s
		LEFT JOIN session_runtime sr ON sr.session_id = s.id
		WHERE s.folder_path = ?
	`, folderPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var sessionID string
		var raw string
		if err := rows.Scan(&sessionID, &raw); err != nil {
			return 0, err
		}
		var state domain.SessionRuntimeState
		if err := json.Unmarshal([]byte(raw), &state); err == nil && state.Active {
			count++
		}
	}
	return count, rows.Err()
}

func (s *Store) MarkAllSessionsInactive(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, state_json FROM session_runtime`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type item struct {
		id    string
		state domain.SessionRuntimeState
	}
	var items []item
	for rows.Next() {
		var raw string
		var sessionID string
		if err := rows.Scan(&sessionID, &raw); err != nil {
			return err
		}
		var state domain.SessionRuntimeState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			continue
		}
		if state.Active {
			state.Active = false
			items = append(items, item{id: sessionID, state: state})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		if err := s.SaveSessionRuntimeState(ctx, item.id, item.state); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateMessage(ctx context.Context, sessionID string, role domain.MessageRole, content string, source string) (domain.Message, error) {
	now := time.Now().UTC()
	message := domain.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Source:    source,
		CreatedAt: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, turn_id, role, content, source, created_at)
		VALUES (?, ?, '', ?, ?, ?, ?)
	`, message.ID, message.SessionID, string(message.Role), message.Content, message.Source, message.CreatedAt)
	if err != nil {
		return domain.Message{}, err
	}
	return message, nil
}

func (s *Store) AppendEvent(ctx context.Context, sessionID, eventType string, payload any) error {
	now := time.Now().UTC()
	raw := "{}"
	if payload != nil {
		switch v := payload.(type) {
		case string:
			raw = v
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			raw = string(b)
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (id, session_id, event_type, payload, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, uuid.NewString(), sessionID, eventType, raw, now)
	return err
}

func (s *Store) ListEvents(ctx context.Context, sessionID string, limit int) ([]domain.RuntimeEvent, error) {
	query := `
		SELECT id, session_id, event_type, payload, created_at
		FROM events
		WHERE session_id = ?
		ORDER BY created_at DESC, id DESC
	`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		query += " LIMIT ?"
		rows, err = s.db.QueryContext(ctx, query, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query, sessionID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.RuntimeEvent
	for rows.Next() {
		var event domain.RuntimeEvent
		if err := rows.Scan(&event.ID, &event.SessionID, &event.EventType, &event.Payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) LoadSessionRuntimeState(ctx context.Context, sessionID string) (domain.SessionRuntimeState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT state_json FROM session_runtime WHERE session_id = ?`, sessionID)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return domain.SessionRuntimeState{}, nil
		}
		return domain.SessionRuntimeState{}, err
	}
	var state domain.SessionRuntimeState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return domain.SessionRuntimeState{}, err
	}
	return state, nil
}

func (s *Store) SaveSessionRuntimeState(ctx context.Context, sessionID string, state domain.SessionRuntimeState) error {
	now := time.Now().UTC()
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_runtime (session_id, state_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET state_json = excluded.state_json, updated_at = excluded.updated_at
	`, sessionID, string(b), now)
	return err
}

func (s *Store) CreateMigrationCheckpoint(ctx context.Context, checkpoint domain.MigrationCheckpoint) error {
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO migration_checkpoints (id, session_id, summary, payload, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, checkpoint.ID, checkpoint.SessionID, checkpoint.ConversationSummary, string(payload), checkpoint.CreatedAt.UTC())
	return err
}

func (s *Store) ListMigrationCheckpoints(ctx context.Context, sessionID string, limit int) ([]domain.MigrationCheckpoint, error) {
	query := `
		SELECT payload
		FROM migration_checkpoints
		WHERE session_id = ?
		ORDER BY created_at DESC, id DESC
	`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		query += " LIMIT ?"
		rows, err = s.db.QueryContext(ctx, query, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query, sessionID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.MigrationCheckpoint
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item domain.MigrationCheckpoint
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListTraits(ctx context.Context) ([]domain.Trait, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, name, description, scope, canonical_definition, supports_claude, supports_codex, sync_mode, dispatch_mode, created_at, updated_at
		FROM traits
		ORDER BY updated_at DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Trait
	for rows.Next() {
		var item domain.Trait
		var supportsClaude, supportsCodex int
		var typ string
		if err := rows.Scan(&item.ID, &typ, &item.Name, &item.Description, &item.Scope, &item.CanonicalDef, &supportsClaude, &supportsCodex, &item.SyncMode, &item.DispatchMode, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Type = domain.TraitType(typ)
		item.SupportsClaude = supportsClaude == 1
		item.SupportsCodex = supportsCodex == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertTrait(ctx context.Context, trait domain.Trait) (domain.Trait, error) {
	now := time.Now().UTC()
	if trait.ID == "" {
		trait.ID = uuid.NewString()
		trait.CreatedAt = now
	}
	trait.UpdatedAt = now
	if trait.CreatedAt.IsZero() {
		trait.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO traits (id, type, name, description, scope, canonical_definition, supports_claude, supports_codex, sync_mode, dispatch_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
			description = excluded.description,
			scope = excluded.scope,
			canonical_definition = excluded.canonical_definition,
			supports_claude = excluded.supports_claude,
			supports_codex = excluded.supports_codex,
			sync_mode = excluded.sync_mode,
			dispatch_mode = excluded.dispatch_mode,
			updated_at = excluded.updated_at
	`, trait.ID, string(trait.Type), trait.Name, trait.Description, trait.Scope, trait.CanonicalDef, boolToInt(trait.SupportsClaude), boolToInt(trait.SupportsCodex), trait.SyncMode, trait.DispatchMode, trait.CreatedAt, trait.UpdatedAt)
	if err != nil {
		return domain.Trait{}, err
	}
	return trait, nil
}

func (s *Store) DeleteTrait(ctx context.Context, traitID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM traits WHERE id = ?`, traitID)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]domain.Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, turn_id, role, content, source, created_at
		FROM messages
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []domain.Message
	for rows.Next() {
		var msg domain.Message
		var role string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.TurnID, &role, &msg.Content, &msg.Source, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.Role = domain.MessageRole(role)
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *Store) UpsertEcosystemSnapshot(ctx context.Context, health domain.HabitatHealth) error {
	modelsJSON, _ := json.Marshal(health.AvailableModels)
	warningsJSON, _ := json.Marshal(health.Warnings)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ecosystem_snapshots (habitat, installed, authenticated, version, available_models_json, warnings_json, config_path_hint, boundary_behavior, last_probe_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(health.Habitat), health.Installed, health.Authenticated, health.Version, string(modelsJSON), string(warningsJSON), health.ConfigPathHint, health.BoundaryBehavior, health.LastProbeAt.UTC())
	return err
}
