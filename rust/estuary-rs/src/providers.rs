use crate::types::{Habitat, Session};

#[derive(Clone, Debug)]
pub struct LaunchSpec {
    pub cmd: String,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
}

pub trait TerminalAdapter {
    fn start(&self, session: &Session) -> LaunchSpec;
    fn resume(&self, session: &Session, native_id: &str) -> LaunchSpec;
    fn model_switch_input(&self, model_id: &str) -> Option<String>;
}

pub fn adapter_for(habitat: Habitat) -> Box<dyn TerminalAdapter> {
    match habitat {
        Habitat::Claude => Box::new(ClaudeTerminalAdapter),
        Habitat::Codex => Box::new(CodexTerminalAdapter),
    }
}

struct ClaudeTerminalAdapter;

impl TerminalAdapter for ClaudeTerminalAdapter {
    fn start(&self, _session: &Session) -> LaunchSpec {
        LaunchSpec {
            cmd: "claude".to_string(),
            args: Vec::new(),
            env: Vec::new(),
        }
    }

    fn resume(&self, session: &Session, native_id: &str) -> LaunchSpec {
        if native_id.trim().is_empty() {
            return self.start(session);
        }
        LaunchSpec {
            cmd: "claude".to_string(),
            args: vec!["--resume".to_string(), native_id.to_string()],
            env: Vec::new(),
        }
    }

    fn model_switch_input(&self, model_id: &str) -> Option<String> {
        Some(format!("/model {}\n", model_id))
    }
}

struct CodexTerminalAdapter;

impl TerminalAdapter for CodexTerminalAdapter {
    fn start(&self, session: &Session) -> LaunchSpec {
        let mut args = vec!["--no-alt-screen".to_string()];
        if !session.folder_path.trim().is_empty() {
            args.push("-C".to_string());
            args.push(session.folder_path.clone());
        }
        if !session.current_model.trim().is_empty() {
            args.push("--model".to_string());
            args.push(session.current_model.clone());
        }

        LaunchSpec {
            cmd: "codex".to_string(),
            args,
            env: Vec::new(),
        }
    }

    fn resume(&self, session: &Session, native_id: &str) -> LaunchSpec {
        let mut spec = self.start(session);
        if !native_id.trim().is_empty() {
            spec.env
                .push(("CODEX_THREAD_ID".to_string(), native_id.to_string()));
        }
        spec
    }

    fn model_switch_input(&self, _model_id: &str) -> Option<String> {
        None
    }
}
