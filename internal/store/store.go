package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	for _, stmt := range []string{
		`ALTER TABLE sessions ADD COLUMN runtime_kind TEXT NOT NULL DEFAULT 'provider_session'`,
		`ALTER TABLE sessions ADD COLUMN active_provider_session_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE provider_runtime_processes ADD COLUMN warm INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("apply schema upgrade: %w", err)
		}
	}
	return s.backfillProviderSessions(ctx)
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
	var currentHabitat, boundaryProfile, status, runtimeKind, providerStatus string
	if err := s.Scan(&session.ID, &session.Title, &session.FolderPath, &session.CurrentModel, &currentHabitat, &runtimeKind, &session.ActiveProviderSessionID, &session.NativeSessionID, &boundaryProfile, &session.ResolvedBoundarySettings, &status, &providerStatus, &session.MigrationGeneration, &session.CreatedAt, &session.UpdatedAt, &session.LastOpenedAt); err != nil {
		return domain.Session{}, err
	}
	session.CurrentHabitat = domain.Habitat(currentHabitat)
	session.RuntimeKind = domain.SessionRuntimeKind(runtimeKind)
	session.ModelDescriptor = domain.ModelDescriptor{
		ID:      session.CurrentModel,
		Label:   session.CurrentModel,
		Habitat: session.CurrentHabitat,
	}
	session.BoundaryProfile = domain.ProfileID(boundaryProfile)
	session.Status = domain.SessionStatus(status)
	session.ProviderStatus = domain.ProviderRuntimeStatus(providerStatus)
	return session, nil
}

func (s *Store) ListSessions(ctx context.Context) ([]domain.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.folder_path, s.current_model, s.current_habitat, s.runtime_kind, s.active_provider_session_id, s.native_session_id, s.boundary_profile_id, s.resolved_boundary_settings, s.status, COALESCE(ps.status, ''), s.migration_generation, s.created_at, s.updated_at, s.last_opened_at
		FROM sessions s
		LEFT JOIN provider_sessions ps ON ps.id = s.active_provider_session_id
		ORDER BY s.last_opened_at DESC, s.updated_at DESC
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
		SELECT s.id, s.title, s.folder_path, s.current_model, s.current_habitat, s.runtime_kind, s.active_provider_session_id, s.native_session_id, s.boundary_profile_id, s.resolved_boundary_settings, s.status, COALESCE(ps.status, ''), s.migration_generation, s.created_at, s.updated_at, s.last_opened_at
		FROM sessions s
		LEFT JOIN provider_sessions ps ON ps.id = s.active_provider_session_id
		WHERE s.id = ?
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
		RuntimeKind:              domain.SessionRuntimeKindProviderSession,
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
		INSERT INTO sessions (id, title, folder_path, current_model, current_habitat, runtime_kind, active_provider_session_id, native_session_id, boundary_profile_id, resolved_boundary_settings, status, migration_generation, created_at, updated_at, last_opened_at)
		VALUES (?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, 0, ?, ?, ?)
	`, session.ID, session.Title, session.FolderPath, session.CurrentModel, string(session.CurrentHabitat), string(session.RuntimeKind), string(session.BoundaryProfile), session.ResolvedBoundarySettings, string(session.Status), session.CreatedAt, session.UpdatedAt, session.LastOpenedAt)
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
		SET title = ?, folder_path = ?, current_model = ?, current_habitat = ?, runtime_kind = ?, active_provider_session_id = ?, native_session_id = ?, boundary_profile_id = ?, resolved_boundary_settings = ?, status = ?, migration_generation = ?, updated_at = ?, last_opened_at = ?
		WHERE id = ?
	`, session.Title, session.FolderPath, session.CurrentModel, string(session.CurrentHabitat), string(session.RuntimeKind), session.ActiveProviderSessionID, session.NativeSessionID, string(session.BoundaryProfile), session.ResolvedBoundarySettings, string(session.Status), session.MigrationGeneration, now, now, session.ID)
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

func (s *Store) backfillProviderSessions(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, current_habitat, native_session_id, runtime_kind, active_provider_session_id, created_at, updated_at
		FROM sessions
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		sessionID   string
		provider    string
		nativeID    string
		runtimeKind string
		activeRefID string
		createdAt   time.Time
		updatedAt   time.Time
	}
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.sessionID, &item.provider, &item.nativeID, &item.runtimeKind, &item.activeRefID, &item.createdAt, &item.updatedAt); err != nil {
			return err
		}
		if strings.TrimSpace(item.activeRefID) == "" && strings.TrimSpace(item.nativeID) != "" {
			items = append(items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		ref := domain.ProviderSessionRef{
			ID:                uuid.NewString(),
			SessionID:         item.sessionID,
			Provider:          domain.Habitat(item.provider),
			RuntimeKind:       domain.SessionRuntimeKind(item.runtimeKind),
			ProviderSessionID: item.nativeID,
			ProviderThreadID:  item.nativeID,
			Status:            domain.ProviderRuntimeStatusReady,
			StartedAt:         item.createdAt,
			UpdatedAt:         item.updatedAt,
		}
		if err := s.UpsertProviderSession(ctx, ref); err != nil {
			return err
		}
		if err := s.SetActiveProviderSession(ctx, item.sessionID, ref.ID, item.nativeID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetActiveProviderSession(ctx context.Context, sessionID, providerSessionRefID, nativeSessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET active_provider_session_id = ?, native_session_id = ?, updated_at = ?, last_opened_at = ?
		WHERE id = ?
	`, providerSessionRefID, nativeSessionID, time.Now().UTC(), time.Now().UTC(), sessionID)
	return err
}

func (s *Store) GetProviderSessionByID(ctx context.Context, id string) (domain.ProviderSessionRef, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, provider, runtime_kind, provider_session_id, provider_thread_id, resume_cursor_json, status, last_error, created_at, updated_at, closed_at
		FROM provider_sessions
		WHERE id = ?
	`, id)
	return scanProviderSession(row)
}

func (s *Store) GetProviderSessionBySession(ctx context.Context, sessionID string, kind domain.SessionRuntimeKind) (domain.ProviderSessionRef, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, provider, runtime_kind, provider_session_id, provider_thread_id, resume_cursor_json, status, last_error, created_at, updated_at, closed_at
		FROM provider_sessions
		WHERE session_id = ? AND runtime_kind = ?
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`, sessionID, string(kind))
	return scanProviderSession(row)
}

func (s *Store) ListProviderSessions(ctx context.Context) ([]domain.ProviderSessionRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, provider, runtime_kind, provider_session_id, provider_thread_id, resume_cursor_json, status, last_error, created_at, updated_at, closed_at
		FROM provider_sessions
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.ProviderSessionRef
	for rows.Next() {
		item, err := scanProviderSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertProviderSession(ctx context.Context, ref domain.ProviderSessionRef) error {
	now := time.Now().UTC()
	if ref.ID == "" {
		ref.ID = uuid.NewString()
	}
	if ref.StartedAt.IsZero() {
		ref.StartedAt = now
	}
	ref.UpdatedAt = now
	var closedAt any
	if !ref.ClosedAt.IsZero() {
		closedAt = ref.ClosedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_sessions (id, session_id, provider, runtime_kind, provider_session_id, provider_thread_id, resume_cursor_json, status, last_error, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			provider = excluded.provider,
			runtime_kind = excluded.runtime_kind,
			provider_session_id = excluded.provider_session_id,
			provider_thread_id = excluded.provider_thread_id,
			resume_cursor_json = excluded.resume_cursor_json,
			status = excluded.status,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at,
			closed_at = excluded.closed_at
	`, ref.ID, ref.SessionID, string(ref.Provider), string(ref.RuntimeKind), ref.ProviderSessionID, ref.ProviderThreadID, ref.ProviderResumeCursorJSON, string(ref.Status), ref.LastError, ref.StartedAt, ref.UpdatedAt, closedAt)
	return err
}

func (s *Store) UpsertProviderProcessState(ctx context.Context, state domain.ProviderProcessState) error {
	now := time.Now().UTC()
	if state.ID == "" {
		state.ID = uuid.NewString()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_runtime_processes (id, provider_session_id, transport, warm, pid, connected, metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider_session_id = excluded.provider_session_id,
			transport = excluded.transport,
			warm = excluded.warm,
			pid = excluded.pid,
			connected = excluded.connected,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`, state.ID, state.ProviderSessionID, state.Transport, boolToInt(state.Warm), state.PID, boolToInt(state.Connected), state.MetadataJSON, state.CreatedAt, state.UpdatedAt)
	return err
}

func (s *Store) UpsertTerminalFeatureSession(ctx context.Context, feature domain.TerminalFeatureSession) error {
	now := time.Now().UTC()
	if feature.ID == "" {
		feature.ID = uuid.NewString()
	}
	if feature.CreatedAt.IsZero() {
		feature.CreatedAt = now
	}
	feature.UpdatedAt = now
	var closedAt any
	if !feature.ClosedAt.IsZero() {
		closedAt = feature.ClosedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO terminal_feature_sessions (id, session_id, provider, feature_key, terminal_session_id, status, metadata_json, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			provider = excluded.provider,
			feature_key = excluded.feature_key,
			terminal_session_id = excluded.terminal_session_id,
			status = excluded.status,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			closed_at = excluded.closed_at
	`, feature.ID, feature.SessionID, string(feature.Provider), feature.FeatureKey, feature.TerminalSessionID, string(feature.Status), feature.MetadataJSON, feature.CreatedAt, feature.UpdatedAt, closedAt)
	return err
}

func (s *Store) GetTerminalFeatureSessionByID(ctx context.Context, id string) (domain.TerminalFeatureSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, provider, feature_key, terminal_session_id, status, metadata_json, created_at, updated_at, closed_at
		FROM terminal_feature_sessions
		WHERE id = ?
	`, id)
	return scanTerminalFeatureSession(row)
}

func (s *Store) GetTerminalFeatureSessionByFeature(ctx context.Context, sessionID, featureKey string) (domain.TerminalFeatureSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, provider, feature_key, terminal_session_id, status, metadata_json, created_at, updated_at, closed_at
		FROM terminal_feature_sessions
		WHERE session_id = ? AND feature_key = ?
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`, sessionID, featureKey)
	return scanTerminalFeatureSession(row)
}

func (s *Store) UpsertSessionTask(ctx context.Context, task domain.SessionTask) error {
	now := time.Now().UTC()
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
	var closedAt any
	if !task.ClosedAt.IsZero() {
		closedAt = task.ClosedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO session_tasks (id, session_id, provider_task_id, source, provider, title, detail, status, created_at, updated_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			session_id = excluded.session_id,
			provider_task_id = excluded.provider_task_id,
			source = excluded.source,
			provider = excluded.provider,
			title = excluded.title,
			detail = excluded.detail,
			status = excluded.status,
			updated_at = excluded.updated_at,
			closed_at = excluded.closed_at
	`, task.ID, task.SessionID, task.ProviderTaskID, string(task.Source), string(task.Provider), task.Title, task.Detail, task.Status, task.CreatedAt, task.UpdatedAt, closedAt)
	return err
}

func (s *Store) GetSessionTaskByProviderTaskID(ctx context.Context, sessionID, providerTaskID string) (domain.SessionTask, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, provider_task_id, source, provider, title, detail, status, created_at, updated_at, closed_at
		FROM session_tasks
		WHERE session_id = ? AND provider_task_id = ?
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`, sessionID, providerTaskID)
	return scanSessionTask(row)
}

func (s *Store) ListSessionTasks(ctx context.Context, sessionID string) ([]domain.SessionTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, provider_task_id, source, provider, title, detail, status, created_at, updated_at, closed_at
		FROM session_tasks
		WHERE session_id = ?
		ORDER BY updated_at DESC, created_at DESC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.SessionTask
	for rows.Next() {
		item, err := scanSessionTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanProviderSession(s scanner) (domain.ProviderSessionRef, error) {
	var ref domain.ProviderSessionRef
	var provider, runtimeKind, status string
	var closedAt sql.NullTime
	if err := s.Scan(&ref.ID, &ref.SessionID, &provider, &runtimeKind, &ref.ProviderSessionID, &ref.ProviderThreadID, &ref.ProviderResumeCursorJSON, &status, &ref.LastError, &ref.StartedAt, &ref.UpdatedAt, &closedAt); err != nil {
		return domain.ProviderSessionRef{}, err
	}
	ref.Provider = domain.Habitat(provider)
	ref.RuntimeKind = domain.SessionRuntimeKind(runtimeKind)
	ref.Status = domain.ProviderRuntimeStatus(status)
	if closedAt.Valid {
		ref.ClosedAt = closedAt.Time
	}
	return ref, nil
}

func scanTerminalFeatureSession(s scanner) (domain.TerminalFeatureSession, error) {
	var item domain.TerminalFeatureSession
	var provider, status string
	var closedAt sql.NullTime
	if err := s.Scan(&item.ID, &item.SessionID, &provider, &item.FeatureKey, &item.TerminalSessionID, &status, &item.MetadataJSON, &item.CreatedAt, &item.UpdatedAt, &closedAt); err != nil {
		return domain.TerminalFeatureSession{}, err
	}
	item.Provider = domain.Habitat(provider)
	item.Status = domain.ProviderRuntimeStatus(status)
	if closedAt.Valid {
		item.ClosedAt = closedAt.Time
	}
	return item, nil
}

func scanSessionTask(s scanner) (domain.SessionTask, error) {
	var item domain.SessionTask
	var source, provider string
	var closedAt sql.NullTime
	if err := s.Scan(&item.ID, &item.SessionID, &item.ProviderTaskID, &source, &provider, &item.Title, &item.Detail, &item.Status, &item.CreatedAt, &item.UpdatedAt, &closedAt); err != nil {
		return domain.SessionTask{}, err
	}
	item.Source = domain.TaskSource(source)
	item.Provider = domain.Habitat(provider)
	if closedAt.Valid {
		item.ClosedAt = closedAt.Time
	}
	return item, nil
}
