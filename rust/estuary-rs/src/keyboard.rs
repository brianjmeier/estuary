use crossterm::event::{Event, KeyCode, KeyEvent, KeyModifiers};
use std::sync::atomic::{AtomicU8, Ordering};
use std::sync::Arc;

pub struct KeyboardProtocol {
    flags: Arc<AtomicU8>,
}

impl KeyboardProtocol {
    pub fn new() -> Self {
        Self {
            flags: Arc::new(AtomicU8::new(0)),
        }
    }

    pub fn flags_ref(&self) -> Arc<AtomicU8> {
        Arc::clone(&self.flags)
    }

    pub fn is_enhanced(&self) -> bool {
        self.flags.load(Ordering::SeqCst) > 0
    }
}

pub fn detect_keyboard_protocol(data: &[u8], flags: &AtomicU8) {
    let len = data.len();
    let mut i = 0;

    while i + 3 < len {
        if data[i] != 0x1b || data[i + 1] != b'[' {
            i += 1;
            continue;
        }
        i += 2;
        if i >= len {
            break;
        }

        match data[i] {
            b'>' => {
                i += 1;
                let mut num: u8 = 0;
                let mut has_digits = false;
                while i < len && data[i].is_ascii_digit() {
                    num = num.saturating_mul(10).saturating_add(data[i] - b'0');
                    has_digits = true;
                    i += 1;
                }
                if has_digits && i < len && data[i] == b'u' {
                    flags.store(num.max(1), Ordering::SeqCst);
                    i += 1;
                }
            }
            b'<' => {
                while i < len && data[i] != b'u' {
                    i += 1;
                }
                if i < len && data[i] == b'u' {
                    flags.store(0, Ordering::SeqCst);
                    i += 1;
                }
            }
            _ => {}
        }
    }
}

fn xterm_modifier(mods: KeyModifiers) -> u8 {
    let mut value = 0;
    if mods.contains(KeyModifiers::SHIFT) {
        value += 1;
    }
    if mods.contains(KeyModifiers::ALT) {
        value += 2;
    }
    if mods.contains(KeyModifiers::CONTROL) {
        value += 4;
    }
    if mods.contains(KeyModifiers::SUPER) {
        value += 8;
    }
    if mods.contains(KeyModifiers::HYPER) {
        value += 16;
    }
    if mods.contains(KeyModifiers::META) {
        value += 32;
    }
    if value > 0 {
        value + 1
    } else {
        0
    }
}

fn needs_csi_u(mods: KeyModifiers) -> bool {
    if mods.intersects(KeyModifiers::SUPER | KeyModifiers::HYPER | KeyModifiers::META) {
        return true;
    }
    let ctrl = mods.contains(KeyModifiers::CONTROL);
    let shift = mods.contains(KeyModifiers::SHIFT);
    let alt = mods.contains(KeyModifiers::ALT);
    (ctrl && shift) || (ctrl && alt) || (alt && shift)
}

fn csi_u(codepoint: u32, modifier: u8) -> Vec<u8> {
    if modifier > 0 {
        format!("\x1b[{};{}u", codepoint, modifier).into_bytes()
    } else {
        format!("\x1b[{}u", codepoint).into_bytes()
    }
}

fn modified_special_key_csi(suffix: u8, modifier: u8) -> Vec<u8> {
    format!("\x1b[1;{}{}", modifier, suffix as char).into_bytes()
}

fn modified_special_key_tilde(code: u16, modifier: u8) -> Vec<u8> {
    format!("\x1b[{};{}~", code, modifier).into_bytes()
}

pub fn key_to_bytes(key: &KeyEvent, inner_enhanced: bool) -> Vec<u8> {
    let mods = key.modifiers;
    let xmod = xterm_modifier(mods);

    match key.code {
        KeyCode::Char(c) => {
            if needs_csi_u(mods) {
                return csi_u(c.to_ascii_lowercase() as u32, xmod);
            }

            if mods.contains(KeyModifiers::CONTROL) {
                if c.is_ascii_alphabetic() {
                    vec![(c.to_ascii_lowercase() as u8) & 0x1f]
                } else if c == ' ' {
                    vec![0x00]
                } else {
                    csi_u(c as u32, xmod)
                }
            } else if mods.contains(KeyModifiers::ALT) {
                let mut bytes = vec![0x1b];
                let mut buf = [0u8; 4];
                bytes.extend_from_slice(c.encode_utf8(&mut buf).as_bytes());
                bytes
            } else {
                let mut buf = [0u8; 4];
                c.encode_utf8(&mut buf).as_bytes().to_vec()
            }
        }
        KeyCode::Enter => {
            if xmod > 0 || inner_enhanced {
                csi_u(13, xmod)
            } else {
                vec![b'\r']
            }
        }
        KeyCode::Backspace => {
            if xmod > 0 || inner_enhanced {
                csi_u(127, xmod)
            } else {
                vec![0x7f]
            }
        }
        KeyCode::Tab => {
            if inner_enhanced || xmod > 0 {
                csi_u(9, xmod)
            } else {
                vec![b'\t']
            }
        }
        KeyCode::BackTab => {
            if inner_enhanced || xmod > 0 {
                csi_u(9, if xmod > 0 { xmod } else { 2 })
            } else {
                vec![0x1b, b'[', b'Z']
            }
        }
        KeyCode::Esc => {
            if xmod > 0 || inner_enhanced {
                csi_u(27, xmod)
            } else {
                vec![0x1b]
            }
        }
        KeyCode::Up => {
            if xmod > 0 {
                modified_special_key_csi(b'A', xmod)
            } else {
                vec![0x1b, b'[', b'A']
            }
        }
        KeyCode::Down => {
            if xmod > 0 {
                modified_special_key_csi(b'B', xmod)
            } else {
                vec![0x1b, b'[', b'B']
            }
        }
        KeyCode::Right => {
            if xmod > 0 {
                modified_special_key_csi(b'C', xmod)
            } else {
                vec![0x1b, b'[', b'C']
            }
        }
        KeyCode::Left => {
            if xmod > 0 {
                modified_special_key_csi(b'D', xmod)
            } else {
                vec![0x1b, b'[', b'D']
            }
        }
        KeyCode::Home => {
            if xmod > 0 {
                modified_special_key_csi(b'H', xmod)
            } else {
                vec![0x1b, b'[', b'H']
            }
        }
        KeyCode::End => {
            if xmod > 0 {
                modified_special_key_csi(b'F', xmod)
            } else {
                vec![0x1b, b'[', b'F']
            }
        }
        KeyCode::PageUp => {
            if xmod > 0 {
                modified_special_key_tilde(5, xmod)
            } else {
                vec![0x1b, b'[', b'5', b'~']
            }
        }
        KeyCode::PageDown => {
            if xmod > 0 {
                modified_special_key_tilde(6, xmod)
            } else {
                vec![0x1b, b'[', b'6', b'~']
            }
        }
        KeyCode::Delete => {
            if xmod > 0 {
                modified_special_key_tilde(3, xmod)
            } else {
                vec![0x1b, b'[', b'3', b'~']
            }
        }
        KeyCode::Insert => {
            if xmod > 0 {
                modified_special_key_tilde(2, xmod)
            } else {
                vec![0x1b, b'[', b'2', b'~']
            }
        }
        KeyCode::F(n) => match n {
            1 => {
                if xmod > 0 {
                    modified_special_key_csi(b'P', xmod)
                } else {
                    vec![0x1b, b'O', b'P']
                }
            }
            2 => {
                if xmod > 0 {
                    modified_special_key_csi(b'Q', xmod)
                } else {
                    vec![0x1b, b'O', b'Q']
                }
            }
            3 => {
                if xmod > 0 {
                    modified_special_key_csi(b'R', xmod)
                } else {
                    vec![0x1b, b'O', b'R']
                }
            }
            4 => {
                if xmod > 0 {
                    modified_special_key_csi(b'S', xmod)
                } else {
                    vec![0x1b, b'O', b'S']
                }
            }
            5 => modified_special_key_tilde(15, xmod.max(1)),
            6 => modified_special_key_tilde(17, xmod.max(1)),
            7 => modified_special_key_tilde(18, xmod.max(1)),
            8 => modified_special_key_tilde(19, xmod.max(1)),
            9 => modified_special_key_tilde(20, xmod.max(1)),
            10 => modified_special_key_tilde(21, xmod.max(1)),
            11 => modified_special_key_tilde(23, xmod.max(1)),
            12 => modified_special_key_tilde(24, xmod.max(1)),
            _ => Vec::new(),
        },
        _ => Vec::new(),
    }
}

pub fn event_to_bytes(event: &Event, inner_enhanced: bool) -> Vec<u8> {
    match event {
        Event::Key(key) => key_to_bytes(key, inner_enhanced),
        Event::Paste(text) => text.as_bytes().to_vec(),
        _ => Vec::new(),
    }
}
