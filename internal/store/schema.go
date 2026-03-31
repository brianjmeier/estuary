package store

var schema = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		folder_path TEXT NOT NULL,
		current_model TEXT NOT NULL,
		current_habitat TEXT NOT NULL,
		runtime_kind TEXT NOT NULL DEFAULT 'provider_session',
		active_provider_session_id TEXT NOT NULL DEFAULT '',
		native_session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		migration_generation INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_opened_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS session_runtime (
		session_id TEXT PRIMARY KEY,
		state_json TEXT NOT NULL DEFAULT '{}',
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		turn_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		source TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS migration_checkpoints (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		summary TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS traits (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		scope TEXT NOT NULL,
		canonical_definition TEXT NOT NULL,
		supports_claude INTEGER NOT NULL,
		supports_codex INTEGER NOT NULL,
		sync_mode TEXT NOT NULL,
		dispatch_mode TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS habitat_settings (
		habitat TEXT PRIMARY KEY,
		settings_json TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS ecosystem_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		habitat TEXT NOT NULL,
		installed INTEGER NOT NULL,
		authenticated INTEGER NOT NULL,
		version TEXT NOT NULL,
		available_models_json TEXT NOT NULL,
		warnings_json TEXT NOT NULL,
		config_path_hint TEXT NOT NULL,
		last_probe_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS provider_sessions (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		runtime_kind TEXT NOT NULL,
		provider_session_id TEXT NOT NULL DEFAULT '',
		provider_thread_id TEXT NOT NULL DEFAULT '',
		resume_cursor_json TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		closed_at TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS provider_runtime_processes (
		id TEXT PRIMARY KEY,
		provider_session_id TEXT NOT NULL,
		transport TEXT NOT NULL,
		warm INTEGER NOT NULL DEFAULT 0,
		pid INTEGER NOT NULL DEFAULT 0,
		connected INTEGER NOT NULL DEFAULT 0,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS terminal_feature_sessions (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		feature_key TEXT NOT NULL,
		terminal_session_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		closed_at TIMESTAMP
	);`,
	`CREATE TABLE IF NOT EXISTS session_tasks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider_task_id TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL,
		provider TEXT NOT NULL,
		title TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		closed_at TIMESTAMP
	);`,
	// handoff_packets stores context snapshots used when switching model or provider.
	// payload_json is the full HandoffPacket serialized as JSON.
	`CREATE TABLE IF NOT EXISTS handoff_packets (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		source_model TEXT NOT NULL,
		source_provider TEXT NOT NULL,
		target_model TEXT NOT NULL,
		target_provider TEXT NOT NULL,
		switch_type TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL
	);`,
	// pty_sessions tracks live PTY-backed native provider sessions.
	// attach_strategy: "fresh" | "resume" | "handoff"
	// status: "running" | "exited" | "killed"
	`CREATE TABLE IF NOT EXISTS pty_sessions (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		pid INTEGER NOT NULL DEFAULT 0,
		attach_strategy TEXT NOT NULL DEFAULT 'fresh',
		native_session_id TEXT NOT NULL DEFAULT '',
		handoff_packet_id TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'running',
		exit_code INTEGER NOT NULL DEFAULT 0,
		started_at TIMESTAMP NOT NULL,
		exited_at TIMESTAMP
	);`,
}
