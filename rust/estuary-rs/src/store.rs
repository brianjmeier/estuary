use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use chrono::Utc;
use rusqlite::{params, Connection, OptionalExtension};
use uuid::Uuid;

use crate::types::{AttachStrategy, Habitat, HandoffPacket, Message, RuntimeEvent, Session};

pub struct Store {
    conn: Connection,
}

impl Store {
    pub fn open_default() -> Result<Self> {
        let home = dirs::home_dir().context("resolve home dir")?;
        let data_dir = home.join(".estuary").join("data");
        fs::create_dir_all(&data_dir).context("create ~/.estuary/data")?;
        let db_path = data_dir.join("estuary.db");
        let conn = Connection::open(db_path).context("open sqlite database")?;
        let store = Self { conn };
        store.migrate()?;
        Ok(store)
    }

    fn migrate(&self) -> Result<()> {
        self.conn.execute_batch(
            r#"
            CREATE TABLE IF NOT EXISTS sessions (
                id TEXT PRIMARY KEY,
                title TEXT NOT NULL,
                folder_path TEXT NOT NULL,
                current_model TEXT NOT NULL,
                current_habitat TEXT NOT NULL,
                runtime_kind TEXT NOT NULL DEFAULT 'provider_terminal',
                active_provider_session_id TEXT NOT NULL DEFAULT '',
                native_session_id TEXT NOT NULL DEFAULT '',
                status TEXT NOT NULL,
                migration_generation INTEGER NOT NULL DEFAULT 0,
                created_at TIMESTAMP NOT NULL,
                updated_at TIMESTAMP NOT NULL,
                last_opened_at TIMESTAMP NOT NULL
            );
            CREATE TABLE IF NOT EXISTS messages (
                id TEXT PRIMARY KEY,
                session_id TEXT NOT NULL,
                turn_id TEXT NOT NULL DEFAULT '',
                role TEXT NOT NULL,
                content TEXT NOT NULL,
                source TEXT NOT NULL DEFAULT '',
                created_at TIMESTAMP NOT NULL
            );
            CREATE TABLE IF NOT EXISTS events (
                id TEXT PRIMARY KEY,
                session_id TEXT NOT NULL,
                event_type TEXT NOT NULL,
                payload TEXT NOT NULL DEFAULT '{}',
                created_at TIMESTAMP NOT NULL
            );
            CREATE TABLE IF NOT EXISTS handoff_packets (
                id TEXT PRIMARY KEY,
                session_id TEXT NOT NULL,
                source_model TEXT NOT NULL,
                source_provider TEXT NOT NULL,
                target_model TEXT NOT NULL,
                target_provider TEXT NOT NULL,
                switch_type TEXT NOT NULL,
                payload_json TEXT NOT NULL,
                created_at TIMESTAMP NOT NULL
            );
            CREATE TABLE IF NOT EXISTS pty_sessions (
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
            );
        "#,
        )?;
        Ok(())
    }

    pub fn list_sessions(&self) -> Result<Vec<Session>> {
        let mut stmt = self.conn.prepare(
            r#"
            SELECT id, title, folder_path, current_model, current_habitat, native_session_id, status
            FROM sessions
            ORDER BY last_opened_at DESC, updated_at DESC
        "#,
        )?;

        let rows = stmt.query_map([], |row| self.scan_session(row))?;
        let mut sessions = Vec::new();
        for row in rows {
            sessions.push(row?);
        }
        Ok(sessions)
    }

    pub fn find_recent_session_for_folder(&self, folder_path: &Path) -> Result<Option<Session>> {
        let mut stmt = self.conn.prepare(
            r#"
            SELECT id, title, folder_path, current_model, current_habitat, native_session_id, status
            FROM sessions
            WHERE folder_path = ?
            ORDER BY last_opened_at DESC
            LIMIT 1
        "#,
        )?;

        let folder = folder_path.to_string_lossy().to_string();
        stmt.query_row([folder], |row| self.scan_session(row))
            .optional()
            .map_err(Into::into)
    }

    pub fn create_session(&self, folder_path: &Path, model: &str, habitat: Habitat) -> Result<Session> {
        let now = Utc::now().to_rfc3339();
        let folder = folder_path.to_string_lossy().to_string();
        let title = folder_path
            .file_name()
            .and_then(|value| value.to_str())
            .filter(|value| !value.is_empty())
            .unwrap_or(&folder)
            .to_string();

        let session = Session {
            id: Uuid::new_v4().to_string(),
            title,
            folder_path: folder,
            current_model: model.to_string(),
            current_habitat: habitat,
            native_session_id: String::new(),
            status: "active".to_string(),
        };

        self.conn.execute(
            r#"
            INSERT INTO sessions (
                id, title, folder_path, current_model, current_habitat, runtime_kind,
                active_provider_session_id, native_session_id, status, migration_generation,
                created_at, updated_at, last_opened_at
            )
            VALUES (?, ?, ?, ?, ?, 'provider_terminal', '', '', ?, 0, ?, ?, ?)
        "#,
            params![
                session.id,
                session.title,
                session.folder_path,
                session.current_model,
                session.current_habitat.as_str(),
                session.status,
                now,
                now,
                now
            ],
        )?;

        Ok(session)
    }

    pub fn touch_session(&self, session_id: &str) -> Result<()> {
        let now = Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE sessions SET last_opened_at = ?, updated_at = ?, status = 'active' WHERE id = ?",
            params![now, now, session_id],
        )?;
        Ok(())
    }

    pub fn update_session(&self, session: &Session) -> Result<()> {
        let now = Utc::now().to_rfc3339();
        self.conn.execute(
            r#"
            UPDATE sessions
            SET title = ?, folder_path = ?, current_model = ?, current_habitat = ?, native_session_id = ?, status = ?, updated_at = ?
            WHERE id = ?
        "#,
            params![
                session.title,
                session.folder_path,
                session.current_model,
                session.current_habitat.as_str(),
                session.native_session_id,
                session.status,
                now,
                session.id
            ],
        )?;
        Ok(())
    }

    pub fn set_session_status(&self, session_id: &str, status: &str) -> Result<()> {
        let now = Utc::now().to_rfc3339();
        self.conn.execute(
            "UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?",
            params![status, now, session_id],
        )?;
        Ok(())
    }

    pub fn list_messages(&self, session_id: &str) -> Result<Vec<Message>> {
        let mut stmt = self.conn.prepare(
            r#"
            SELECT role, content
            FROM messages
            WHERE session_id = ?
            ORDER BY created_at ASC
        "#,
        )?;

        let rows = stmt.query_map([session_id], |row| {
            Ok(Message {
                role: row.get(0)?,
                content: row.get(1)?,
            })
        })?;

        let mut messages = Vec::new();
        for row in rows {
            messages.push(row?);
        }
        Ok(messages)
    }

    pub fn list_events(&self, session_id: &str, limit: usize) -> Result<Vec<RuntimeEvent>> {
        let mut stmt = self.conn.prepare(
            r#"
            SELECT event_type
            FROM events
            WHERE session_id = ?
            ORDER BY created_at DESC
            LIMIT ?
        "#,
        )?;

        let rows = stmt.query_map(params![session_id, limit as i64], |row| {
            Ok(RuntimeEvent {
                event_type: row.get(0)?,
            })
        })?;

        let mut events = Vec::new();
        for row in rows {
            events.push(row?);
        }
        Ok(events)
    }

    pub fn save_handoff_packet(&self, packet: &HandoffPacket) -> Result<()> {
        let payload = serde_json::to_string(packet)?;
        self.conn.execute(
            r#"
            INSERT INTO handoff_packets (
                id, session_id, source_model, source_provider, target_model, target_provider, switch_type, payload_json, created_at
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        "#,
            params![
                packet.checkpoint.id,
                packet.checkpoint.session_id,
                packet.source_model,
                packet.source_provider.as_str(),
                packet.target_model,
                packet.target_provider.as_str(),
                packet.switch_type.as_str(),
                payload,
                packet.checkpoint.created_at.to_rfc3339()
            ],
        )?;
        Ok(())
    }

    pub fn save_pty_session(
        &self,
        id: &str,
        session: &Session,
        attach_strategy: AttachStrategy,
        pid: u32,
        handoff_packet_id: Option<&str>,
    ) -> Result<()> {
        self.conn.execute(
            r#"
            INSERT OR REPLACE INTO pty_sessions (
                id, session_id, provider, pid, attach_strategy, native_session_id, handoff_packet_id, status, exit_code, started_at
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, 'running', 0, ?)
        "#,
            params![
                id,
                session.id,
                session.current_habitat.as_str(),
                pid as i64,
                attach_strategy.as_str(),
                session.native_session_id,
                handoff_packet_id.unwrap_or(""),
                Utc::now().to_rfc3339()
            ],
        )?;
        Ok(())
    }

    pub fn close_pty_session(&self, id: &str, exit_code: i32) -> Result<()> {
        self.conn.execute(
            "UPDATE pty_sessions SET status = 'exited', exit_code = ?, exited_at = ? WHERE id = ?",
            params![exit_code, Utc::now().to_rfc3339(), id],
        )?;
        Ok(())
    }

    pub fn canonical_folder_path(path: &Path) -> Result<PathBuf> {
        path.canonicalize()
            .or_else(|_| Ok(path.to_path_buf()))
            .map_err(Into::into)
    }

    fn scan_session(&self, row: &rusqlite::Row<'_>) -> rusqlite::Result<Session> {
        let habitat: String = row.get(4)?;
        Ok(Session {
            id: row.get(0)?,
            title: row.get(1)?,
            folder_path: row.get(2)?,
            current_model: row.get(3)?,
            current_habitat: Habitat::from_str(&habitat).unwrap_or(Habitat::Claude),
            native_session_id: row.get(5)?,
            status: row.get(6)?,
        })
    }
}
