mod chrome;
mod handoff;
mod keyboard;
mod providers;
mod store;
mod terminal;
mod types;

use std::io::{self, Write};
use std::path::{Path, PathBuf};
use std::time::Duration;

use anyhow::{Context, Result};
use crossterm::event::{self, Event, KeyCode, KeyEvent, KeyEventKind, KeyModifiers};
use crossterm::terminal::{disable_raw_mode, enable_raw_mode};
use uuid::Uuid;

use chrome::{detect_host_chrome, HostChrome};
use handoff::{build_packet, injection_text};
use keyboard::{event_to_bytes, KeyboardProtocol};
use providers::{adapter_for, LaunchSpec};
use store::Store;
use terminal::{OutputController, PtySession};
use types::{habitat_for_model, AttachStrategy, ChromeState, Habitat, Session, SwitchType, SUPPORTED_MODELS};

fn main() {
    if let Err(err) = run() {
        eprintln!("estuary-rs: {err:#}");
        std::process::exit(1);
    }
}

fn run() -> Result<()> {
    let cwd = current_dir()?;
    let mut app = App::new(cwd)?;
    app.run()
}

struct App {
    store: Store,
    chrome: Box<dyn HostChrome>,
    output: OutputController,
    keyboard: KeyboardProtocol,
    current_session: Session,
    current_pty: PtySession,
    current_pty_record_id: String,
}

impl App {
    fn new(cwd: PathBuf) -> Result<Self> {
        let store = Store::open_default()?;
        let folder = Store::canonical_folder_path(&cwd)?;
        let current_session = if let Some(session) = store.find_recent_session_for_folder(&folder)? {
            store.touch_session(&session.id)?;
            session
        } else {
            store.create_session(&folder, "claude-sonnet-4-6", Habitat::Claude)?
        };

        let output = OutputController::new();
        let keyboard = KeyboardProtocol::new();
        let mut chrome = detect_host_chrome();
        apply_session_chrome(&mut *chrome, &current_session, None)?;

        let (rows, cols) = terminal_size();
        let (current_pty, current_pty_record_id) =
            spawn_for_session(&store, &output, &keyboard, &current_session, rows, cols, AttachStrategy::Fresh, None)?;

        Ok(Self {
            store,
            chrome,
            output,
            keyboard,
            current_session,
            current_pty,
            current_pty_record_id,
        })
    }

    fn run(&mut self) -> Result<()> {
        enable_raw_mode().context("enable raw mode")?;
        let result = self.event_loop();
        disable_raw_mode().ok();
        let _ = self.chrome.clear();
        result
    }

    fn event_loop(&mut self) -> Result<()> {
        loop {
            if let Some(exit_code) = self.current_pty.try_wait()? {
                self.store.close_pty_session(&self.current_pty_record_id, exit_code)?;
                self.store.set_session_status(&self.current_session.id, "idle")?;
                apply_session_chrome(
                    &mut *self.chrome,
                    &self.current_session,
                    Some(format!("session ended ({exit_code}) · Ctrl+K r reconnect · Ctrl+K q quit")),
                )?;
                self.handle_exited_session()?;
                continue;
            }

            if !event::poll(Duration::from_millis(100))? {
                continue;
            }

            match event::read()? {
                Event::Key(key) if key.kind == KeyEventKind::Press => {
                    if is_ctrl_k(&key) {
                        if self.handle_leader()? {
                            break;
                        }
                        continue;
                    }

                    let bytes = event_to_bytes(&Event::Key(key), self.keyboard.is_enhanced());
                    if !bytes.is_empty() {
                        self.current_pty.write(&bytes)?;
                    }
                }
                Event::Paste(text) => {
                    let bytes = event_to_bytes(&Event::Paste(text), self.keyboard.is_enhanced());
                    if !bytes.is_empty() {
                        self.current_pty.write(&bytes)?;
                    }
                }
                Event::Resize(cols, rows) => {
                    self.current_pty.resize(rows, cols)?;
                }
                _ => {}
            }
        }

        Ok(())
    }

    fn handle_exited_session(&mut self) -> Result<()> {
        loop {
            if !event::poll(Duration::from_millis(100))? {
                continue;
            }

            match event::read()? {
                Event::Key(key) if key.kind == KeyEventKind::Press => {
                    if key.code == KeyCode::Char('c') && key.modifiers.contains(KeyModifiers::CONTROL)
                    {
                        return Ok(());
                    }

                    if is_ctrl_k(&key) && self.handle_leader()? {
                        return Ok(());
                    }
                }
                _ => {}
            }
        }
    }

    fn handle_leader(&mut self) -> Result<bool> {
        self.output.pause();
        print!("\r\n[estuary] leader: ? help · s session · m model · r reconnect · q quit\r\n");
        io::stdout().flush()?;

        let action = if event::poll(Duration::from_secs(1))? {
            match event::read()? {
                Event::Key(key) if key.kind == KeyEventKind::Press => Some(key),
                _ => None,
            }
        } else {
            None
        };

        let should_quit = match action {
            Some(key) => match key.code {
                KeyCode::Char('?') => {
                    self.show_help()?;
                    false
                }
                KeyCode::Char('s') | KeyCode::Char('S') => {
                    self.switch_session_prompt()?;
                    false
                }
                KeyCode::Char('m') | KeyCode::Char('M') => {
                    self.switch_model_prompt()?;
                    false
                }
                KeyCode::Char('r') | KeyCode::Char('R') => {
                    self.reconnect_current_session()?;
                    false
                }
                KeyCode::Char('q') | KeyCode::Char('Q') => true,
                _ => false,
            },
            None => false,
        };

        let truncated = self.output.resume()?;
        if truncated {
            apply_session_chrome(
                &mut *self.chrome,
                &self.current_session,
                Some("output truncated while command mode was open".to_string()),
            )?;
        } else {
            apply_session_chrome(&mut *self.chrome, &self.current_session, None)?;
        }

        Ok(should_quit)
    }

    fn reconnect_current_session(&mut self) -> Result<()> {
        self.replace_pty(AttachStrategy::Fresh, None)
    }

    fn switch_session_prompt(&mut self) -> Result<()> {
        disable_raw_mode()?;
        let sessions = self.store.list_sessions()?;
        println!("\nEstuary sessions:");
        for (index, session) in sessions.iter().enumerate() {
            println!(
                "  {}. {}  [{} / {}]",
                index + 1,
                session.folder_path,
                session.current_habitat,
                session.current_model
            );
        }
        println!("\nChoose a session number (blank cancels):");
        print!("> ");
        io::stdout().flush()?;

        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        enable_raw_mode()?;

        let Some(choice) = input.trim().parse::<usize>().ok() else {
            return Ok(());
        };
        let Some(target) = sessions.get(choice.saturating_sub(1)).cloned() else {
            return Ok(());
        };
        if target.id == self.current_session.id {
            return Ok(());
        }

        self.store.set_session_status(&self.current_session.id, "idle")?;
        self.current_session = target;
        self.store.touch_session(&self.current_session.id)?;
        apply_session_chrome(&mut *self.chrome, &self.current_session, None)?;
        self.replace_pty(AttachStrategy::Resume, None)
    }

    fn switch_model_prompt(&mut self) -> Result<()> {
        disable_raw_mode()?;
        println!("\nSwitch model:");
        for (index, model) in SUPPORTED_MODELS.iter().enumerate() {
            println!("  {}. {} ({})", index + 1, model.label, model.id);
        }
        println!("\nChoose a model number (blank cancels):");
        print!("> ");
        io::stdout().flush()?;

        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        enable_raw_mode()?;

        let Some(choice) = input.trim().parse::<usize>().ok() else {
            return Ok(());
        };
        let Some(target) = SUPPORTED_MODELS.get(choice.saturating_sub(1)) else {
            return Ok(());
        };

        self.switch_model(target.id, target.habitat)
    }

    fn switch_model(&mut self, target_model: &str, target_provider: Habitat) -> Result<()> {
        let switch_type = if target_provider == self.current_session.current_habitat {
            SwitchType::SameProvider
        } else {
            SwitchType::CrossProvider
        };

        let packet = build_packet(
            &self.store,
            &self.current_session,
            target_model,
            target_provider,
            switch_type,
            None,
        )?;

        let adapter = adapter_for(self.current_session.current_habitat);
        if target_provider == self.current_session.current_habitat {
            if let Some(command) = adapter.model_switch_input(target_model) {
                self.current_pty.write(command.as_bytes())?;
                self.current_pty
                    .write(format!("{}\n", injection_text(&packet)).as_bytes())?;
                self.current_session.current_model = target_model.to_string();
                self.store.update_session(&self.current_session)?;
                apply_session_chrome(
                    &mut *self.chrome,
                    &self.current_session,
                    Some("model switched in-place".to_string()),
                )?;
                return Ok(());
            }
        }

        self.current_session.current_model = target_model.to_string();
        self.current_session.current_habitat = target_provider;
        self.current_session.native_session_id.clear();
        self.store.update_session(&self.current_session)?;
        self.replace_pty(AttachStrategy::Handoff, Some(&packet))?;
        self.current_pty
            .write(format!("{}\n", injection_text(&packet)).as_bytes())?;
        Ok(())
    }

    fn replace_pty(
        &mut self,
        attach_strategy: AttachStrategy,
        handoff_packet: Option<&types::HandoffPacket>,
    ) -> Result<()> {
        let _ = self.current_pty.kill();
        let _ = self.store.close_pty_session(&self.current_pty_record_id, 0);

        let (rows, cols) = terminal_size();
        let handoff_id = handoff_packet.map(|packet| packet.checkpoint.id.as_str());
        let (pty, record_id) = spawn_for_session(
            &self.store,
            &self.output,
            &self.keyboard,
            &self.current_session,
            rows,
            cols,
            attach_strategy,
            handoff_id,
        )?;
        self.current_pty = pty;
        self.current_pty_record_id = record_id;
        self.store.set_session_status(&self.current_session.id, "active")?;
        apply_session_chrome(&mut *self.chrome, &self.current_session, None)?;
        Ok(())
    }

    fn show_help(&mut self) -> Result<()> {
        disable_raw_mode()?;
        println!("\nEstuary leader help");
        println!("  Ctrl+K ?  show this help");
        println!("  Ctrl+K s  switch session");
        println!("  Ctrl+K m  switch model");
        println!("  Ctrl+K r  reconnect current session");
        println!("  Ctrl+K q  quit Estuary");
        println!("\nPress Enter to continue...");
        io::stdout().flush()?;

        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        enable_raw_mode()?;
        Ok(())
    }
}

fn spawn_for_session(
    store: &Store,
    output: &OutputController,
    keyboard: &KeyboardProtocol,
    session: &Session,
    rows: u16,
    cols: u16,
    attach_strategy: AttachStrategy,
    handoff_packet_id: Option<&str>,
) -> Result<(PtySession, String)> {
    let adapter = adapter_for(session.current_habitat);
    let spec: LaunchSpec = if attach_strategy == AttachStrategy::Resume && !session.native_session_id.is_empty() {
        adapter.resume(session, &session.native_session_id)
    } else {
        adapter.start(session)
    };
    let pty = PtySession::spawn(&spec, &session.folder_path, rows, cols, output.clone(), keyboard)?;
    let record_id = Uuid::new_v4().to_string();
    store.save_pty_session(&record_id, session, attach_strategy, pty.pid, handoff_packet_id)?;
    Ok((pty, record_id))
}

fn apply_session_chrome(chrome: &mut dyn HostChrome, session: &Session, notice: Option<String>) -> Result<()> {
    let folder = Path::new(&session.folder_path)
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or(&session.folder_path)
        .to_string();
    chrome.apply(&ChromeState {
        model: session.current_model.clone(),
        habitat: session.current_habitat,
        folder,
        notice,
    })
}

fn terminal_size() -> (u16, u16) {
    match crossterm::terminal::size() {
        Ok((cols, rows)) if cols > 0 && rows > 0 => (rows, cols),
        _ => (24, 80),
    }
}

fn current_dir() -> Result<PathBuf> {
    std::env::current_dir().context("resolve current directory")
}

fn is_ctrl_k(key: &KeyEvent) -> bool {
    matches!(key.code, KeyCode::Char('k') | KeyCode::Char('K'))
        && key.modifiers.contains(KeyModifiers::CONTROL)
}
