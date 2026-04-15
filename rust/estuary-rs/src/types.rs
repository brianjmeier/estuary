use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Habitat {
    Claude,
    Codex,
}

impl Habitat {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Claude => "claude",
            Self::Codex => "codex",
        }
    }

    pub fn from_str(value: &str) -> Option<Self> {
        match value {
            "claude" => Some(Self::Claude),
            "codex" => Some(Self::Codex),
            _ => None,
        }
    }
}

impl std::fmt::Display for Habitat {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

#[derive(Clone, Debug)]
pub struct ModelDescriptor {
    pub id: &'static str,
    pub label: &'static str,
    pub habitat: Habitat,
}

pub const SUPPORTED_MODELS: &[ModelDescriptor] = &[
    ModelDescriptor {
        id: "claude-sonnet-4-6",
        label: "Sonnet 4.6",
        habitat: Habitat::Claude,
    },
    ModelDescriptor {
        id: "claude-sonnet-4-5",
        label: "Sonnet 4.5",
        habitat: Habitat::Claude,
    },
    ModelDescriptor {
        id: "claude-opus-4-6",
        label: "Opus 4.6",
        habitat: Habitat::Claude,
    },
    ModelDescriptor {
        id: "claude-opus-4-5",
        label: "Opus 4.5",
        habitat: Habitat::Claude,
    },
    ModelDescriptor {
        id: "gpt-5.4",
        label: "GPT-5.4",
        habitat: Habitat::Codex,
    },
    ModelDescriptor {
        id: "gpt-5.3",
        label: "GPT-5.3",
        habitat: Habitat::Codex,
    },
    ModelDescriptor {
        id: "gpt-5.3-codex",
        label: "GPT-5.3 Codex",
        habitat: Habitat::Codex,
    },
];

pub fn habitat_for_model(model: &str) -> Option<Habitat> {
    let normalized = model.trim().to_lowercase();
    if normalized.contains("claude") {
        Some(Habitat::Claude)
    } else if normalized.starts_with("gpt-")
        || normalized.starts_with("codex-")
        || normalized == "codex"
        || normalized.starts_with('o')
    {
        Some(Habitat::Codex)
    } else {
        None
    }
}

#[derive(Clone, Debug)]
pub struct Session {
    pub id: String,
    pub title: String,
    pub folder_path: String,
    pub current_model: String,
    pub current_habitat: Habitat,
    pub native_session_id: String,
    pub status: String,
}

#[derive(Clone, Debug)]
pub struct Message {
    pub role: String,
    pub content: String,
}

#[derive(Clone, Debug)]
pub struct RuntimeEvent {
    pub event_type: String,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SwitchType {
    SameProvider,
    CrossProvider,
    Restore,
    Fresh,
}

impl SwitchType {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::SameProvider => "same_provider",
            Self::CrossProvider => "cross_provider",
            Self::Restore => "restore",
            Self::Fresh => "fresh",
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum AttachStrategy {
    Fresh,
    Resume,
    Handoff,
}

impl AttachStrategy {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Fresh => "fresh",
            Self::Resume => "resume",
            Self::Handoff => "handoff",
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct MigrationCheckpoint {
    pub id: String,
    pub session_id: String,
    pub active_objective: String,
    pub important_decisions: Vec<String>,
    pub folder_path: String,
    pub current_model: String,
    pub current_habitat: Habitat,
    pub conversation_summary: String,
    pub open_tasks: Vec<String>,
    pub recent_tool_outputs: Vec<String>,
    pub habitat_notes: std::collections::BTreeMap<String, String>,
    pub created_at: DateTime<Utc>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct HandoffPacket {
    #[serde(flatten)]
    pub checkpoint: MigrationCheckpoint,
    pub recent_work_summary: String,
    pub file_references: Vec<String>,
    pub source_model: String,
    pub source_provider: Habitat,
    pub target_model: String,
    pub target_provider: Habitat,
    pub switch_type: SwitchType,
    pub user_note: String,
}

#[derive(Clone, Debug)]
pub struct ChromeState {
    pub model: String,
    pub habitat: Habitat,
    pub folder: String,
    pub notice: Option<String>,
}
