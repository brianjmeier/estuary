use anyhow::Result;
use std::io::{self, Write};
use std::process::Command;

use crate::types::ChromeState;

pub trait HostChrome {
    fn apply(&mut self, state: &ChromeState) -> Result<()>;
    fn clear(&mut self) -> Result<()>;
}

pub fn detect_host_chrome() -> Box<dyn HostChrome> {
    let pane_id = std::env::var("TMUX_PANE").unwrap_or_default();
    if !pane_id.trim().is_empty() {
        return Box::new(TmuxChrome {
            pane_id,
            last_title: String::new(),
        });
    }

    Box::new(OscTitleChrome {
        last_title: String::new(),
    })
}

fn sanitize_title(title: &str) -> String {
    let title = title.replace('\n', " ").replace('\r', " ");
    let title = title.trim();
    if title.is_empty() {
        "estuary".to_string()
    } else {
        title.to_string()
    }
}

fn format_title(state: &ChromeState) -> String {
    if let Some(notice) = state.notice.as_ref().filter(|notice| !notice.trim().is_empty()) {
        return sanitize_title(&format!("◆ estuary | {}", notice));
    }

    sanitize_title(&format!(
        "◆ estuary | {} | {} | {}",
        state.model, state.habitat, state.folder
    ))
}

struct TmuxChrome {
    pane_id: String,
    last_title: String,
}

impl HostChrome for TmuxChrome {
    fn apply(&mut self, state: &ChromeState) -> Result<()> {
        let title = format_title(state);
        if title == self.last_title {
            return Ok(());
        }
        self.last_title = title.clone();
        let _ = Command::new("tmux")
            .args(["select-pane", "-t", &self.pane_id, "-T", &title])
            .status();
        Ok(())
    }

    fn clear(&mut self) -> Result<()> {
        self.last_title.clear();
        Ok(())
    }
}

struct OscTitleChrome {
    last_title: String,
}

impl HostChrome for OscTitleChrome {
    fn apply(&mut self, state: &ChromeState) -> Result<()> {
        let title = format_title(state);
        if title == self.last_title {
            return Ok(());
        }
        self.last_title = title.clone();
        let mut stdout = io::stdout().lock();
        write!(stdout, "\x1b]2;{}\x07", title)?;
        stdout.flush()?;
        Ok(())
    }

    fn clear(&mut self) -> Result<()> {
        self.last_title.clear();
        let mut stdout = io::stdout().lock();
        write!(stdout, "\x1b]2;estuary\x07")?;
        stdout.flush()?;
        Ok(())
    }
}
