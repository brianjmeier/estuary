use std::collections::VecDeque;
use std::io::{self, Read, Write};
use std::sync::atomic::AtomicU8;
use std::sync::{Arc, Mutex};
use std::thread;

use anyhow::{Context, Result};
use portable_pty::{native_pty_system, CommandBuilder, MasterPty, PtySize};

use crate::keyboard::{detect_keyboard_protocol, KeyboardProtocol};
use crate::providers::LaunchSpec;

const OUTPUT_BUFFER_LIMIT: usize = 1024 * 1024;

#[derive(Clone)]
pub struct OutputController {
    state: Arc<Mutex<OutputState>>,
}

struct OutputState {
    paused: bool,
    buffer: VecDeque<u8>,
    buffered_len: usize,
    truncated: bool,
}

impl OutputController {
    pub fn new() -> Self {
        Self {
            state: Arc::new(Mutex::new(OutputState {
                paused: false,
                buffer: VecDeque::new(),
                buffered_len: 0,
                truncated: false,
            })),
        }
    }

    pub fn write(&self, data: &[u8]) -> Result<()> {
        let mut state = self.state.lock().expect("output state lock");
        if state.paused {
            for byte in data {
                state.buffer.push_back(*byte);
            }
            state.buffered_len += data.len();
            while state.buffered_len > OUTPUT_BUFFER_LIMIT {
                if state.buffer.pop_front().is_some() {
                    state.buffered_len -= 1;
                    state.truncated = true;
                }
            }
            return Ok(());
        }

        drop(state);
        let mut stdout = io::stdout().lock();
        stdout.write_all(data)?;
        stdout.flush()?;
        Ok(())
    }

    pub fn pause(&self) {
        let mut state = self.state.lock().expect("output state lock");
        state.paused = true;
    }

    pub fn resume(&self) -> Result<bool> {
        let mut state = self.state.lock().expect("output state lock");
        state.paused = false;
        let truncated = state.truncated;

        if state.buffer.is_empty() {
            state.truncated = false;
            return Ok(truncated);
        }

        let bytes = state.buffer.drain(..).collect::<Vec<_>>();
        state.buffered_len = 0;
        state.truncated = false;
        drop(state);

        let mut stdout = io::stdout().lock();
        stdout.write_all(&bytes)?;
        stdout.flush()?;
        Ok(truncated)
    }
}

pub struct PtySession {
    writer: Arc<Mutex<Box<dyn Write + Send>>>,
    master: Arc<Mutex<Box<dyn MasterPty + Send>>>,
    child: Box<dyn portable_pty::Child + Send + Sync>,
    pub pid: u32,
}

impl PtySession {
    pub fn spawn(
        spec: &LaunchSpec,
        cwd: &str,
        rows: u16,
        cols: u16,
        output: OutputController,
        keyboard: &KeyboardProtocol,
    ) -> Result<Self> {
        let pty_system = native_pty_system();
        let pair = pty_system
            .openpty(PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            })
            .context("open PTY")?;

        let mut command = CommandBuilder::new(&spec.cmd);
        for arg in &spec.args {
            command.arg(arg);
        }
        command.cwd(cwd);
        command.env("TERM", "xterm-256color");
        for (key, value) in &spec.env {
            command.env(key, value);
        }

        let child = pair
            .slave
            .spawn_command(command)
            .with_context(|| format!("spawn {}", spec.cmd))?;

        let writer = pair.master.take_writer().context("take PTY writer")?;
        let mut reader = pair
            .master
            .try_clone_reader()
            .context("clone PTY reader")?;

        let writer = Arc::new(Mutex::new(writer));
        let flags: Arc<AtomicU8> = keyboard.flags_ref();
        let output_clone = output.clone();

        thread::spawn(move || {
            let mut buf = [0u8; 16384];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) => break,
                    Ok(n) => {
                        let data = &buf[..n];
                        detect_keyboard_protocol(data, &flags);
                        let _ = output_clone.write(data);
                    }
                    Err(_) => break,
                }
            }
        });

        let pid = child.process_id().unwrap_or(0);

        Ok(Self {
            writer,
            master: Arc::new(Mutex::new(pair.master)),
            child,
            pid,
        })
    }

    pub fn write(&self, bytes: &[u8]) -> Result<()> {
        let mut writer = self.writer.lock().expect("pty writer lock");
        writer.write_all(bytes)?;
        writer.flush()?;
        Ok(())
    }

    pub fn resize(&self, rows: u16, cols: u16) -> Result<()> {
        if rows == 0 || cols == 0 {
            return Ok(());
        }
        let master = self.master.lock().expect("pty master lock");
        master.resize(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })?;
        Ok(())
    }

    pub fn try_wait(&mut self) -> Result<Option<i32>> {
        let status = self.child.try_wait()?;
        Ok(status.map(|status| status.exit_code() as i32))
    }

    pub fn kill(&mut self) -> Result<()> {
        self.child.kill()?;
        Ok(())
    }
}
