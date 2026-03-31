package pty

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Session is a live PTY-backed process.
type Session struct {
	ID     string
	PTY    *os.File
	Cmd    *exec.Cmd
	Output chan []byte
	Done   chan struct{}

	mu       sync.Mutex
	exitCode int
}

// ExitCode returns the process exit code after Done is closed.
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// SpawnOpts configures a PTY session.
type SpawnOpts struct {
	ID   string
	Cmd  string
	Args []string
	Env  []string // extra env vars merged with os.Environ()
	Cwd  string
	Rows int
	Cols int
}

// Manager owns all active PTY sessions.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

// Spawn starts a new PTY process and begins streaming its output to Session.Output.
func (m *Manager) Spawn(ctx context.Context, opts SpawnOpts) (*Session, error) {
	rows, cols := opts.Rows, opts.Cols
	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}

	cmd := exec.CommandContext(ctx, opts.Cmd, opts.Args...)
	cmd.Env = append(os.Environ(), opts.Env...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		return nil, fmt.Errorf("pty spawn %q: %w", opts.Cmd, err)
	}

	s := &Session{
		ID:     opts.ID,
		PTY:    ptmx,
		Cmd:    cmd,
		Output: make(chan []byte, 512),
		Done:   make(chan struct{}),
	}

	go s.readLoop()

	m.mu.Lock()
	m.sessions[opts.ID] = s
	m.mu.Unlock()

	return s, nil
}

func (s *Session) readLoop() {
	defer close(s.Done)

	buf := make([]byte, 4096)
	for {
		n, err := s.PTY.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case s.Output <- chunk:
			default:
				// Channel full — drop to avoid blocking the read loop.
			}
		}
		if err != nil {
			break
		}
	}

	if err := s.Cmd.Wait(); err == nil {
		// clean exit
	}
	if s.Cmd.ProcessState != nil {
		s.mu.Lock()
		s.exitCode = s.Cmd.ProcessState.ExitCode()
		s.mu.Unlock()
	}
}

// Write sends input bytes into the PTY (i.e., to the child process's stdin).
func (m *Manager) Write(id string, data []byte) error {
	s, ok := m.get(id)
	if !ok {
		return fmt.Errorf("pty session %q not found", id)
	}
	_, err := s.PTY.Write(data)
	return err
}

// Resize updates the PTY window size and notifies the child process via SIGWINCH.
func (m *Manager) Resize(id string, rows, cols int) error {
	s, ok := m.get(id)
	if !ok {
		return fmt.Errorf("pty session %q not found", id)
	}
	return pty.Setsize(s.PTY, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// Close kills the process and releases the PTY.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}

	s.PTY.Close()
	if s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
	}
	return nil
}

// Session returns the active session by ID.
func (m *Manager) Session(id string) (*Session, bool) {
	return m.get(id)
}

func (m *Manager) get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}
