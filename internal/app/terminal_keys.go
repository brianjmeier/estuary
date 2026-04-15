package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func encodeTerminalKey(msg tea.KeyMsg) ([]byte, bool) {
	if msg.Alt && len(msg.Runes) > 0 {
		return append([]byte{0x1b}, []byte(string(msg.Runes))...), true
	}

	switch msg.Type {
	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			return nil, false
		}
		return []byte(string(msg.Runes)), true
	case tea.KeySpace:
		return []byte(" "), true
	}

	switch msg.String() {
	case "enter":
		return []byte{'\r'}, true
	case "tab":
		return []byte{'\t'}, true
	case "shift+tab":
		return []byte("\x1b[Z"), true
	case "backspace":
		return []byte{0x7f}, true
	case "delete":
		return []byte("\x1b[3~"), true
	case "esc":
		return []byte{0x1b}, true
	case "up":
		return []byte("\x1b[A"), true
	case "down":
		return []byte("\x1b[B"), true
	case "right":
		return []byte("\x1b[C"), true
	case "left":
		return []byte("\x1b[D"), true
	case "home":
		return []byte("\x1b[H"), true
	case "end":
		return []byte("\x1b[F"), true
	case "pgup", "pageup":
		return []byte("\x1b[5~"), true
	case "pgdown", "pagedown":
		return []byte("\x1b[6~"), true
	case "insert":
		return []byte("\x1b[2~"), true
	case "f1":
		return []byte("\x1bOP"), true
	case "f2":
		return []byte("\x1bOQ"), true
	case "f3":
		return []byte("\x1bOR"), true
	case "f4":
		return []byte("\x1bOS"), true
	case "f5":
		return []byte("\x1b[15~"), true
	case "f6":
		return []byte("\x1b[17~"), true
	case "f7":
		return []byte("\x1b[18~"), true
	case "f8":
		return []byte("\x1b[19~"), true
	case "f9":
		return []byte("\x1b[20~"), true
	case "f10":
		return []byte("\x1b[21~"), true
	case "f11":
		return []byte("\x1b[23~"), true
	case "f12":
		return []byte("\x1b[24~"), true
	}

	if strings.HasPrefix(msg.String(), "ctrl+") {
		key := strings.TrimPrefix(msg.String(), "ctrl+")
		if len(key) == 1 {
			r := rune(strings.ToLower(key)[0])
			if r >= 'a' && r <= 'z' {
				return []byte{byte(r - 'a' + 1)}, true
			}
		}
		switch key {
		case "@", "space":
			return []byte{0x00}, true
		case "[":
			return []byte{0x1b}, true
		case "\\":
			return []byte{0x1c}, true
		case "]":
			return []byte{0x1d}, true
		case "^":
			return []byte{0x1e}, true
		case "_":
			return []byte{0x1f}, true
		}
	}

	return nil, false
}
