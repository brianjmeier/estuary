package emulator

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/google/uuid"

	"github.com/brianmeier/estuary/internal/ptyenv"
)

// Emulator owns a provider PTY and the virtual terminal screen state rendered
// into Estuary's Bubble Tea layout.
type Emulator struct {
	mu sync.RWMutex
	id string

	term *vt.Emulator

	pty, tty *os.File
	input    io.Writer

	cmd           *exec.Cmd
	processExited bool
	onExit        func(string)
	pendingOutput []byte

	closeOnce sync.Once
}

// EmittedFrame is a rendered terminal frame. Rows contain ANSI SGR sequences
// emitted by x/vt so foreground, background, and text attributes survive.
type EmittedFrame struct {
	Rows []string
}

// New creates a PTY-backed terminal emulator with the requested pane size.
func New(cols, rows int) (*Emulator, error) {
	if cols <= 0 || rows <= 0 {
		return nil, ErrInvalidSize
	}

	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}

	e := &Emulator{
		id:    uuid.NewString(),
		term:  vt.NewEmulator(cols, rows),
		pty:   ptmx,
		tty:   tty,
		input: ptmx,
	}

	if err := e.resize(cols, rows); err != nil {
		_ = e.Close()
		return nil, err
	}

	go e.ptyReadLoop()
	go e.terminalResponseLoop()

	return e, nil
}

func (e *Emulator) ID() string {
	return e.id
}

// SetSize sets the terminal size.
func (e *Emulator) SetSize(cols, rows int) error {
	return e.Resize(cols, rows)
}

// Resize updates both the child PTY size and x/vt's screen model.
func (e *Emulator) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return ErrInvalidSize
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	return e.resize(cols, rows)
}

func (e *Emulator) resize(cols, rows int) error {
	if e.pty == nil {
		return ErrPTYNotInitialized
	}
	if err := pty.Setsize(e.pty, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
		X:    uint16(cols * 8),
		Y:    uint16(rows * 16),
	}); err != nil {
		return err
	}
	e.term.Resize(cols, rows)
	return nil
}

// SetFrameRate is kept for compatibility with the legacy embedded terminal
// wrapper. Rendering cadence is owned by the Bubble Tea model.
func (e *Emulator) SetFrameRate(_ int) {}

// GetScreen returns the current rendered screen as ANSI strings.
func (e *Emulator) GetScreen() EmittedFrame {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return EmittedFrame{Rows: renderRows(e.term.Render(), e.term.Height())}
}

// FeedOutput parses raw provider output bytes into the terminal screen model.
func (e *Emulator) FeedOutput(data []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.term == nil {
		return 0, ErrPTYNotInitialized
	}
	filtered := e.filterProviderOutput(data)
	if len(filtered) == 0 {
		return len(data), nil
	}
	if _, err := e.term.Write(filtered); err != nil {
		return 0, err
	}
	return len(data), nil
}

// SetOnExit sets a callback that is called when the child process exits.
func (e *Emulator) SetOnExit(callback func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onExit = callback
}

// IsProcessExited returns true after the child process exits.
func (e *Emulator) IsProcessExited() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.processExited
}

// StartCommand starts a provider process attached to this emulator's PTY.
func (e *Emulator) StartCommand(cmd *exec.Cmd) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.pty == nil || e.tty == nil {
		return ErrPTYNotInitialized
	}

	cmd.Env = ptyenv.Build(os.Environ(), cmd.Env)
	cmd.Stdout = e.tty
	cmd.Stdin = e.tty
	cmd.Stderr = e.tty

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true

	e.cmd = cmd
	e.processExited = false

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start terminal command: %w", err)
	}

	go e.monitorProcess()
	return nil
}

func (e *Emulator) monitorProcess() {
	if e.cmd == nil {
		return
	}

	_ = e.cmd.Wait()

	e.mu.Lock()
	e.processExited = true
	onExit := e.onExit
	id := e.id
	e.mu.Unlock()

	if onExit != nil {
		onExit(id)
	}
}

// Write sends keyboard/input bytes to the child PTY.
func (e *Emulator) Write(data []byte) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.pty == nil {
		return 0, ErrPTYNotInitialized
	}
	return e.writeInput(data)
}

// SendKey sends a key sequence to the child PTY.
func (e *Emulator) SendKey(key string) error {
	_, err := e.Write([]byte(key))
	return err
}

// SendMouse sends a basic SGR mouse event to the child PTY.
func (e *Emulator) SendMouse(button int, x, y int, pressed bool) error {
	action := "M"
	if !pressed {
		action = "m"
	}
	_, err := e.Write([]byte(fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x+1, y+1, action)))
	return err
}

// Close shuts down the PTY and releases terminal resources.
func (e *Emulator) Close() error {
	var err error
	e.closeOnce.Do(func() {
		if e.term != nil {
			err = e.term.Close()
		}
		if e.tty != nil {
			if closeErr := e.tty.Close(); err == nil {
				err = closeErr
			}
		}
		if e.pty != nil {
			if closeErr := e.pty.Close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

func (e *Emulator) ptyReadLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := e.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			_, _ = e.FeedOutput(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (e *Emulator) terminalResponseLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := e.term.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			e.mu.RLock()
			_, _ = e.writeInput(chunk)
			e.mu.RUnlock()
		}
		if err != nil {
			return
		}
	}
}

func (e *Emulator) writeInput(data []byte) (int, error) {
	if e.input == nil {
		return 0, ErrPTYNotInitialized
	}
	return e.input.Write(data)
}

func renderRows(rendered string, rows int) []string {
	out := strings.Split(rendered, "\n")
	if len(out) > rows {
		return out[:rows]
	}
	for len(out) < rows {
		out = append(out, "")
	}
	return out
}

func (e *Emulator) filterProviderOutput(data []byte) []byte {
	if len(e.pendingOutput) > 0 {
		merged := make([]byte, 0, len(e.pendingOutput)+len(data))
		merged = append(merged, e.pendingOutput...)
		merged = append(merged, data...)
		data = merged
		e.pendingOutput = nil
	}

	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		if data[i] != 0x1b {
			out = append(out, data[i])
			i++
			continue
		}
		if i+1 >= len(data) {
			e.pendingOutput = append(e.pendingOutput, data[i:]...)
			return out
		}
		if data[i+1] != ']' {
			out = append(out, data[i])
			i++
			continue
		}

		end, ok := oscEnd(data, i+2)
		if !ok {
			e.pendingOutput = append(e.pendingOutput, data[i:]...)
			return out
		}
		seq := data[i:end]
		if !isTitleOSC(seq) {
			out = append(out, seq...)
		}
		i = end
	}

	return out
}

func oscEnd(data []byte, start int) (int, bool) {
	for i := start; i < len(data); i++ {
		switch data[i] {
		case 0x07:
			return i + 1, true
		case 0x1b:
			if i+1 >= len(data) {
				return 0, false
			}
			if data[i+1] == '\\' {
				return i + 2, true
			}
		}
	}
	return 0, false
}

func isTitleOSC(seq []byte) bool {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != ']' {
		return false
	}
	body := seq[2:]
	if len(body) > 0 && body[len(body)-1] == 0x07 {
		body = body[:len(body)-1]
	} else if len(body) >= 2 && body[len(body)-2] == 0x1b && body[len(body)-1] == '\\' {
		body = body[:len(body)-2]
	}
	cmd, _, _ := bytes.Cut(body, []byte(";"))
	return bytes.Equal(cmd, []byte("0")) || bytes.Equal(cmd, []byte("1")) || bytes.Equal(cmd, []byte("2"))
}
