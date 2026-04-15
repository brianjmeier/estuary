package emulator

import (
	"fmt"
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

	cmd           *exec.Cmd
	processExited bool
	onExit        func(string)

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
		id:   uuid.NewString(),
		term: vt.NewEmulator(cols, rows),
		pty:  ptmx,
		tty:  tty,
	}

	if err := e.resize(cols, rows); err != nil {
		_ = e.Close()
		return nil, err
	}

	go e.ptyReadLoop()

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
	return e.term.Write(data)
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
	return e.pty.Write(data)
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
