use std::collections::BTreeMap;

use anyhow::Result;
use chrono::Utc;
use regex::Regex;
use uuid::Uuid;

use crate::store::Store;
use crate::types::{HandoffPacket, Message, MigrationCheckpoint, Session, SwitchType};

const SUMMARY_WINDOW: usize = 10;

pub fn build_packet(
    store: &Store,
    session: &Session,
    target_model: &str,
    target_provider: crate::types::Habitat,
    switch_type: SwitchType,
    user_note: Option<&str>,
) -> Result<HandoffPacket> {
    let messages = store.list_messages(&session.id)?;
    let events = store.list_events(&session.id, 20)?;

    let checkpoint = MigrationCheckpoint {
        id: Uuid::new_v4().to_string(),
        session_id: session.id.clone(),
        active_objective: last_user_objective(&messages),
        important_decisions: collect_message_snippets(&messages, "assistant", 4),
        folder_path: session.folder_path.clone(),
        current_model: session.current_model.clone(),
        current_habitat: session.current_habitat,
        conversation_summary: summarize_messages(&messages),
        open_tasks: collect_message_snippets(&messages, "user", 4),
        recent_tool_outputs: collect_message_snippets(&messages, "tool", 4),
        habitat_notes: BTreeMap::from([
            ("native_session_id".to_string(), session.native_session_id.clone()),
            ("recent_events".to_string(), summarize_events(&events)),
        ]),
        created_at: Utc::now(),
    };

    let packet = HandoffPacket {
        recent_work_summary: checkpoint.conversation_summary.clone(),
        file_references: collect_file_references(&messages),
        source_model: session.current_model.clone(),
        source_provider: session.current_habitat,
        target_model: target_model.to_string(),
        target_provider,
        switch_type,
        user_note: user_note.unwrap_or("").trim().to_string(),
        checkpoint,
    };

    store.save_handoff_packet(&packet)?;
    Ok(packet)
}

pub fn injection_text(packet: &HandoffPacket) -> String {
    let mut lines = vec![
        "Estuary session handoff.".to_string(),
        format!(
            "Source: {}/{}",
            packet.source_provider, packet.source_model
        ),
        format!(
            "Target: {}/{}",
            packet.target_provider, packet.target_model
        ),
        format!("Working directory: {}", packet.checkpoint.folder_path),
    ];

    if !packet.checkpoint.active_objective.is_empty() {
        lines.push(format!(
            "Active objective: {}",
            packet.checkpoint.active_objective
        ));
    }
    if !packet.recent_work_summary.is_empty() {
        lines.push(format!("Recent context: {}", packet.recent_work_summary));
    }
    if !packet.checkpoint.important_decisions.is_empty() {
        lines.push(format!(
            "Important decisions: {}",
            packet.checkpoint.important_decisions.join("; ")
        ));
    }
    if !packet.checkpoint.open_tasks.is_empty() {
        lines.push(format!(
            "Open tasks: {}",
            packet.checkpoint.open_tasks.join("; ")
        ));
    }
    if !packet.file_references.is_empty() {
        lines.push(format!(
            "Relevant files: {}",
            packet.file_references.join(", ")
        ));
    }
    if !packet.checkpoint.recent_tool_outputs.is_empty() {
        lines.push(format!(
            "Recent tool outputs: {}",
            packet.checkpoint.recent_tool_outputs.join("; ")
        ));
    }
    if !packet.user_note.is_empty() {
        lines.push(format!("Operator note: {}", packet.user_note));
    }

    lines.push(
        "Do not restart from scratch. Continue the same task from this context and preserve prior decisions."
            .to_string(),
    );

    lines.join("\n")
}

fn summarize_messages(messages: &[Message]) -> String {
    let start = messages.len().saturating_sub(SUMMARY_WINDOW);
    messages[start..]
        .iter()
        .filter_map(|message| {
            let text = message.content.trim();
            if text.is_empty() {
                return None;
            }
            let text = truncate(text, 220);
            Some(format!("{}: {}", message.role, text))
        })
        .collect::<Vec<_>>()
        .join(" | ")
}

fn last_user_objective(messages: &[Message]) -> String {
    messages
        .iter()
        .rev()
        .find(|message| message.role == "user" && !message.content.trim().is_empty())
        .map(|message| message.content.trim().to_string())
        .unwrap_or_default()
}

fn collect_message_snippets(messages: &[Message], role: &str, limit: usize) -> Vec<String> {
    let mut snippets = messages
        .iter()
        .rev()
        .filter(|message| message.role == role && !message.content.trim().is_empty())
        .take(limit)
        .map(|message| truncate(message.content.trim(), 160))
        .collect::<Vec<_>>();
    snippets.reverse();
    snippets
}

fn collect_file_references(messages: &[Message]) -> Vec<String> {
    let pattern = Regex::new(r"(?x)
        (?:
            /[A-Za-z0-9._/\-]+
            |
            [A-Za-z0-9._\-]+(?:/[A-Za-z0-9._\-]+)+
        )
        (?:\:\d+)?
    ")
    .expect("valid regex");

    let mut refs = Vec::new();
    for message in messages.iter().rev().take(SUMMARY_WINDOW) {
        for matched in pattern.find_iter(&message.content) {
            let candidate = matched.as_str().trim_matches(|ch: char| ",.;:()[]{}".contains(ch));
            if looks_like_file(candidate) && !refs.iter().any(|existing| existing == candidate) {
                refs.push(candidate.to_string());
            }
            if refs.len() >= 8 {
                return refs;
            }
        }
    }
    refs
}

fn looks_like_file(candidate: &str) -> bool {
    candidate.contains('/')
        || candidate.ends_with(".go")
        || candidate.ends_with(".rs")
        || candidate.ends_with(".md")
        || candidate.ends_with(".json")
        || candidate.ends_with(".toml")
        || candidate.ends_with(".yaml")
        || candidate.ends_with(".yml")
}

fn summarize_events(events: &[crate::types::RuntimeEvent]) -> String {
    events
        .iter()
        .take(4)
        .map(|event| event.event_type.as_str())
        .collect::<Vec<_>>()
        .join(", ")
}

fn truncate(value: &str, max: usize) -> String {
    if value.len() <= max {
        value.to_string()
    } else {
        format!("{}...", &value[..max])
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extracts_file_references() {
        let messages = vec![Message {
            role: "user".to_string(),
            content: "check /tmp/foo.rs and internal/app/model.go:42 plus README.md".to_string(),
        }];
        let refs = collect_file_references(&messages);
        assert!(refs.iter().any(|entry| entry == "/tmp/foo.rs"));
        assert!(refs.iter().any(|entry| entry == "internal/app/model.go:42"));
        assert!(refs.iter().any(|entry| entry == "README.md"));
    }
}
