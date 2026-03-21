package store

var schema = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		folder_path TEXT NOT NULL,
		current_model TEXT NOT NULL,
		current_habitat TEXT NOT NULL,
		native_session_id TEXT NOT NULL DEFAULT '',
		boundary_profile_id TEXT NOT NULL,
		resolved_boundary_settings TEXT NOT NULL DEFAULT '{}',
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
		boundary_behavior TEXT NOT NULL,
		last_probe_at TIMESTAMP NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS boundary_profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		policy_level TEXT NOT NULL,
		file_access_policy TEXT NOT NULL,
		command_execution_policy TEXT NOT NULL,
		network_tool_policy TEXT NOT NULL,
		default_approval_behavior TEXT NOT NULL,
		habitat_override_json TEXT NOT NULL,
		compatibility_notes TEXT NOT NULL DEFAULT '',
		unsafe INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS session_boundary_resolutions (
		session_id TEXT NOT NULL,
		profile_id TEXT NOT NULL,
		habitat TEXT NOT NULL,
		compatibility TEXT NOT NULL,
		summary TEXT NOT NULL,
		native_settings TEXT NOT NULL,
		PRIMARY KEY (session_id, profile_id, habitat)
	);`,
	`CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);`,
}
