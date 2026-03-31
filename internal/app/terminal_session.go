package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/brianmeier/estuary/internal/domain"
	"github.com/brianmeier/estuary/internal/habitats"
	"github.com/brianmeier/estuary/internal/handoff"
	"github.com/brianmeier/estuary/internal/prereq"
	"github.com/brianmeier/estuary/internal/providers"
	estuarypty "github.com/brianmeier/estuary/internal/pty"
	"github.com/brianmeier/estuary/internal/sessions"
	"github.com/brianmeier/estuary/internal/store"
)

// TerminalSession is the PTY-first runtime. It manages a raw terminal loop
// where a native provider process (Claude Code or Codex) runs in a PTY, with
// Estuary chrome (header + footer) rendered via ANSI scrolling regions.
//
// The palette (Ctrl+K) is a short-lived Bubble Tea program layered on top.
type TerminalSession struct {
	ctx        context.Context
	cwd        string
	store      *store.Store
	prober     *prereq.Prober
	sessions   *sessions.Service
	handoffSvc *handoff.Service
	ptyMgr     *estuarypty.Manager
	adapters   map[domain.Habitat]providers.TerminalAdapter
	theme      Theme

	// Active state — mutated during the run loop.
	session     domain.Session
	sessionList []domain.Session
	ptySess     *estuarypty.Session
	ptyRecordID string // primary key in pty_sessions table
	overlay     *estuarypty.Overlay
}

// NewTerminalSession creates the PTY-first runtime. It does not start a
// session; call Run() to enter the terminal loop.
func NewTerminalSession(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) (*TerminalSession, error) {
	settings, err := st.LoadAppSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}

	sessionSvc := sessions.NewService(st)
	sessionList, err := sessionSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	return &TerminalSession{
		ctx:        ctx,
		cwd:        cwd,
		store:      st,
		prober:     prober,
		sessions:   sessionSvc,
		handoffSvc: handoff.NewService(st),
		ptyMgr:     estuarypty.NewManager(),
		adapters: map[domain.Habitat]providers.TerminalAdapter{
			domain.HabitatClaude: &providers.ClaudeTerminalAdapter{},
			domain.HabitatCodex:  &providers.CodexTerminalAdapter{},
		},
		theme:       ThemeByName(settings.Theme),
		sessionList: sessionList,
	}, nil
}

// Run enters the raw terminal loop. It blocks until the PTY process exits,
// the user quits, or the context is cancelled.
func (ts *TerminalSession) Run() error {
	// Find an existing session for this directory, or create a fresh one.
	session, err := ts.findOrCreateSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	ts.session = session
	if err := ts.store.TouchSession(ts.ctx, session.ID); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	// Ensure this session is at the top of the list.
	ts.sessionList = prependSession(session, ts.sessionList)

	// Put stdin in raw mode; restore on exit.
	stdinFd := int(os.Stdin.Fd())
	savedState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(stdinFd, savedState) //nolint:errcheck

	// Get terminal dimensions.
	rows, cols := terminalSize()

	// Set up the ANSI overlay (scrolling region + chrome).
	ts.overlay = estuarypty.NewOverlay(os.Stdout, rows, cols)
	ts.overlay.Setup()
	ts.renderChrome()

	// Spawn the native provider in a PTY.
	if err := ts.spawnPTY(ts.overlay.ContentRows(), cols); err != nil {
		return err
	}

	// Forward PTY output to stdout with pause/resume around palette opens.
	fwd := newOutputForwarder(ts.ptySess.Output, os.Stdout, ts.renderChrome)
	go fwd.run()

	// Handle SIGWINCH (terminal resize).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			r, c := terminalSize()
			ts.overlay.Resize(r, c)
			_ = ts.ptyMgr.Resize(ts.ptySess.ID, ts.overlay.ContentRows(), c)
			ts.renderChrome()
		}
	}()

	// Main input loop — runs until quit or PTY exit.
	return ts.inputLoop(stdinFd, savedState, fwd)
}

// findOrCreateSession reopens the most recent session for the cwd when one
// exists, and creates a fresh one otherwise.
func (ts *TerminalSession) findOrCreateSession() (domain.Session, error) {
	existing, found, err := ts.sessions.FindForFolder(ts.ctx, ts.cwd)
	if err != nil {
		return domain.Session{}, err
	}
	if found {
		return existing, nil
	}

	session, _, err := ts.sessions.CreateCurrent(ts.ctx, ts.cwd)
	return session, err
}

// inputLoop reads from stdin and routes each byte sequence to the PTY or to
// Estuary controls (Ctrl+K → palette, Ctrl+C → quit).
func (ts *TerminalSession) inputLoop(stdinFd int, savedState *term.State, fwd *outputForwarder) error {
	buf := make([]byte, 256)
	for {
		// Check if the PTY process has exited.
		select {
		case <-ts.ptySess.Done:
			return ts.handlePTYExit(stdinFd, savedState, fwd)
		case <-ts.ctx.Done():
			return ts.ctx.Err()
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil
		}
		data := buf[:n]

		switch {
		case isCtrlK(data):
			fwd.Pause()
			ts.overlay.Teardown()
			term.Restore(stdinFd, savedState) //nolint:errcheck

			action := ts.runPalette()

			// Model picker runs as a second alt-screen program while the terminal
			// is still restored and PTY output is still paused.
			if action.Kind == "model" {
				target, ok := ts.runModelPicker()
				term.MakeRaw(stdinFd) //nolint:errcheck
				ts.overlay.Setup()
				if ok {
					if err := ts.switchModel(target); err != nil {
						fwd.Resume()
						return err
					}
					fwd.swapOutput(ts.ptySess.Output)
				}
				fwd.Resume()
				ts.renderChrome()
				continue
			}

			term.MakeRaw(stdinFd) //nolint:errcheck
			ts.overlay.Setup()

			prevPTY := ts.ptySess
			if err := ts.handlePaletteAction(action); err != nil {
				fwd.Resume()
				return err
			}
			if ts.ptySess != prevPTY {
				fwd.swapOutput(ts.ptySess.Output)
			}
			fwd.Resume()
			ts.renderChrome()

		case isCtrlC(data):
			return nil

		default:
			_ = ts.ptyMgr.Write(ts.ptySess.ID, data)
		}
	}
}

// handlePTYExit is called when the native process exits. It persists the exit
// state and waits for the user to reconnect or quit.
func (ts *TerminalSession) handlePTYExit(stdinFd int, savedState *term.State, fwd *outputForwarder) error {
	exitCode := ts.ptySess.ExitCode()

	// Persist the PTY session as exited.
	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, exitCode)
	_ = ts.store.UpdateSessionStatus(ts.ctx, ts.session.ID, domain.SessionStatusIdle, ts.session.NativeSessionID)
	_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "pty.exited", map[string]any{"exit_code": exitCode})

	ts.overlay.RenderBar(
		ts.buildBar(fmt.Sprintf("session ended (exit %d)  ·  r reconnect  ·  ^C quit", exitCode)),
		ts.buildDivider(),
	)

	// Wait for user to press 'r' (reconnect) or Ctrl+C (quit).
	buf := make([]byte, 4)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil
		}
		data := buf[:n]
		switch {
		case isCtrlC(data):
			return nil
		case len(data) == 1 && (data[0] == 'r' || data[0] == 'R' || data[0] == '\r'):
			// Reconnect: spawn fresh PTY for the current session.
			_, cols := terminalSize()
			if err := ts.spawnPTY(ts.overlay.ContentRows(), cols); err != nil {
				ts.overlay.RenderBar(
					ts.buildBar(fmt.Sprintf("reconnect failed: %v  ·  ^C quit", err)),
					ts.buildDivider(),
				)
				continue
			}
			fwd.swapOutput(ts.ptySess.Output)
			fwd.Resume()
			ts.renderChrome()
			return ts.inputLoop(stdinFd, savedState, fwd)
		}
	}
}

// spawnPTY starts the native provider process in a PTY. It chooses between
// resume (if NativeSessionID is set) and fresh start.
func (ts *TerminalSession) spawnPTY(rows, cols int) error {
	adapter, ok := ts.adapters[ts.session.CurrentHabitat]
	if !ok {
		return fmt.Errorf("no terminal adapter for provider %q", ts.session.CurrentHabitat)
	}

	strategy := domain.AttachStrategyFresh
	var cmd string
	var args []string

	if ts.session.NativeSessionID != "" {
		cmd, args, _ = adapter.ResumeArgs(ts.session, ts.session.NativeSessionID)
		strategy = domain.AttachStrategyResume
	} else {
		cmd, args, _ = adapter.StartArgs(ts.session)
	}

	ptySess, err := ts.ptyMgr.Spawn(ts.ctx, estuarypty.SpawnOpts{
		ID:   ts.session.ID,
		Cmd:  cmd,
		Args: args,
		Cwd:  ts.session.FolderPath,
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return fmt.Errorf("spawn %s: %w", ts.session.CurrentHabitat, err)
	}
	ts.ptySess = ptySess

	// Persist PTY process metadata.
	ts.ptyRecordID = uuid.NewString()
	pid := 0
	if ptySess.Cmd.Process != nil {
		pid = ptySess.Cmd.Process.Pid
	}
	_ = ts.store.SavePTYSession(ts.ctx, domain.TerminalSession{
		ID:              ts.ptyRecordID,
		SessionID:       ts.session.ID,
		Provider:        ts.session.CurrentHabitat,
		PID:             pid,
		AttachStrategy:  strategy,
		NativeSessionID: ts.session.NativeSessionID,
		Status:          domain.ProviderRuntimeStatusRunning,
		StartedAt:       time.Now().UTC(),
	})
	_ = ts.store.TouchSession(ts.ctx, ts.session.ID)

	return nil
}

// runPalette opens the Ctrl+K palette as a short-lived Bubble Tea alt-screen
// program and returns the action the user selected.
func (ts *TerminalSession) runPalette() PaletteAction {
	palette := NewPaletteModel(ts.sessionList, ts.theme)
	p := tea.NewProgram(palette, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return PaletteAction{}
	}
	if pm, ok := final.(PaletteModel); ok {
		return pm.Result()
	}
	return PaletteAction{}
}

// handlePaletteAction acts on the result of a palette selection.
func (ts *TerminalSession) handlePaletteAction(action PaletteAction) error {
	switch action.Kind {
	case "quit":
		return fmt.Errorf("quit")

	case "new":
		session, _, err := ts.sessions.CreateCurrent(ts.ctx, ts.cwd)
		if err != nil {
			return nil // non-fatal: keep running current session
		}
		ts.sessionList = prependSession(session, ts.sessionList)
		return ts.switchSession(session)

	case "session":
		for _, s := range ts.sessionList {
			if s.ID == action.SessionID {
				return ts.switchSession(s)
			}
		}

	case "reconnect":
		return ts.reconnectPTY()

	case "probe":
		go func() { ts.prober.ProbeAll(ts.ctx) }()

	case "theme":
		if ts.theme.Name == "dark" {
			ts.theme = LightTheme()
		} else {
			ts.theme = DarkTheme()
		}
		settings, _ := ts.store.LoadAppSettings(ts.ctx)
		settings.Theme = ts.theme.Name
		_ = ts.store.SaveAppSettings(ts.ctx, settings)
	}
	return nil
}

// switchSession stops the current PTY, saves its state as idle, and starts a
// new PTY for the target session.
func (ts *TerminalSession) switchSession(target domain.Session) error {
	// Mark the old session as idle before switching.
	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.store.UpdateSessionStatus(ts.ctx, ts.session.ID, domain.SessionStatusIdle, ts.session.NativeSessionID)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)

	ts.session = target
	_, cols := terminalSize()
	if err := ts.spawnPTY(ts.overlay.ContentRows(), cols); err != nil {
		return err
	}
	_ = ts.store.TouchSession(ts.ctx, target.ID)
	return nil
}

// reconnectPTY restarts the current native session.
// If a NativeSessionID is stored, it uses ResumeArgs; otherwise it uses
// StartArgs for a fresh start. This handles the simple same-provider reconnect
// case (e.g. after a crash). For cross-provider or model switching, use switchModel.
func (ts *TerminalSession) reconnectPTY() error {
	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)
	_, cols := terminalSize()
	return ts.spawnPTY(ts.overlay.ContentRows(), cols)
}

// switchModel changes the model (and optionally provider) of the current session.
// It generates a HandoffPacket for context continuity, updates the session record,
// spawns a new PTY, and injects the handoff text into stdin after a short delay.
func (ts *TerminalSession) switchModel(target domain.ModelDescriptor) error {
	switchType := domain.SwitchTypeSameProvider
	if target.Habitat != ts.session.CurrentHabitat {
		switchType = domain.SwitchTypeCrossProvider
	}

	// Generate a handoff packet from the current conversation state.
	// Failures here are non-fatal — we fall back to a plain fresh start.
	packet, err := ts.handoffSvc.Generate(ts.ctx, ts.session, target.ID, target.Habitat, switchType)
	if err != nil {
		_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "handoff.error", map[string]any{"error": err.Error()})
	}

	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)

	// Update the session to reflect the new model and provider.
	// Clear NativeSessionID so spawnPTY uses StartArgs, not ResumeArgs.
	ts.session.CurrentModel = target.ID
	ts.session.CurrentHabitat = target.Habitat
	ts.session.NativeSessionID = ""
	_ = ts.store.UpdateSession(ts.ctx, ts.session)

	_, cols := terminalSize()
	if err := ts.spawnPTY(ts.overlay.ContentRows(), cols); err != nil {
		return err
	}

	// Inject handoff context via PTY stdin after a short delay so the provider
	// CLI has time to print its startup output. Guard against a second model
	// switch racing the goroutine: only write if this PTY session is still active.
	if packet.ID != "" {
		injectionText := handoff.InjectionText(packet)
		spawnedSess := ts.ptySess
		go func() {
			time.Sleep(3 * time.Second)
			if ts.ptySess != spawnedSess {
				return
			}
			_ = ts.ptyMgr.Write(spawnedSess.ID, []byte(injectionText+"\n"))
		}()
		_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "handoff.injected",
			map[string]any{"packet_id": packet.ID, "switch_type": string(switchType)})
	}

	ts.renderChrome()
	return nil
}

// runModelPicker opens the model picker as a short-lived Bubble Tea program
// and returns the chosen model descriptor and whether a choice was made.
func (ts *TerminalSession) runModelPicker() (domain.ModelDescriptor, bool) {
	picker := NewModelPickerModel(habitats.SupportedModels(), ts.session.CurrentModel, ts.theme)
	p := tea.NewProgram(picker, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return domain.ModelDescriptor{}, false
	}
	if pm, ok := final.(ModelPickerModel); ok {
		return pm.Chosen()
	}
	return domain.ModelDescriptor{}, false
}

// renderChrome renders the single-row status bar at the top of the terminal.
func (ts *TerminalSession) renderChrome() {
	ts.overlay.RenderBar(ts.buildBar(""), ts.buildDivider())
}

// buildBar constructs a full-width lipgloss-styled status bar string.
// Every character uses an explicit background (BGSurface) so the bar renders
// correctly regardless of the terminal's own background colour.
func (ts *TerminalSession) buildBar(notice string) string {
	t := ts.theme
	_, cols := terminalSize()
	bg := t.BGSurface

	// Helper: foreground on the bar background.
	s := func(fg lipgloss.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg).Background(bg)
	}
	accent := s(t.AccentWater).Bold(true)
	muted := s(t.FGMuted)
	normal := s(t.FGPrimary)
	fill := lipgloss.NewStyle().Background(bg)

	// Provider-specific color for the habitat badge.
	habitatColor := t.HabitatClaude
	if ts.session.CurrentHabitat == domain.HabitatCodex {
		habitatColor = t.HabitatCodex
	}
	habitat := s(habitatColor).Bold(true)

	logo := accent.Render("◆ estuary")
	sep := muted.Render(" · ")
	hint := muted.Render(" ^K ")

	var center string
	if notice != "" {
		center = muted.Render(notice)
	} else {
		dot := muted.Render("○")
		if ts.ptySess != nil {
			dot = habitat.Render("●")
		}
		center = dot + fill.Render(" ") +
			normal.Render(ts.session.CurrentModel) + sep +
			habitat.Render(string(ts.session.CurrentHabitat)) + sep +
			muted.Render(shortDir(ts.session.FolderPath))
	}

	left := fill.Render(" ") + logo + fill.Render("  ") + center
	leftW := lipgloss.Width(left)
	hintW := lipgloss.Width(hint)
	gap := cols - leftW - hintW
	if gap < 1 {
		gap = 1
	}
	return left + fill.Render(strings.Repeat(" ", gap)) + hint
}

func (ts *TerminalSession) buildDivider() string {
	_, cols := terminalSize()
	if cols < 1 {
		cols = 1
	}
	return lipgloss.NewStyle().
		Foreground(ts.theme.BorderSoft).
		Background(ts.theme.BGCanvas).
		Render(strings.Repeat("─", cols))
}

// shortDir replaces the home directory prefix with ~ for display.
func shortDir(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// terminalSize returns the current terminal rows and cols with safe defaults.
func terminalSize() (rows, cols int) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 24, 80
	}
	return h, w
}

// prependSession adds s to the front of the list, deduplicating by ID.
func prependSession(s domain.Session, list []domain.Session) []domain.Session {
	out := []domain.Session{s}
	for _, existing := range list {
		if existing.ID != s.ID {
			out = append(out, existing)
		}
	}
	return out
}

// isCtrlK returns true if data is the Ctrl+K byte sequence.
func isCtrlK(data []byte) bool { return len(data) == 1 && data[0] == 0x0b }

// isCtrlC returns true if data is the Ctrl+C byte sequence.
func isCtrlC(data []byte) bool { return len(data) == 1 && data[0] == 0x03 }

// outputForwarder copies PTY output to a writer with pause/resume so the
// palette can take the terminal without visual corruption.
type outputForwarder struct {
	mu           sync.Mutex
	output       <-chan []byte
	out          *os.File
	pauseCh      chan struct{}
	resumeCh     chan struct{}
	redrawChrome func()
	sanitizer    terminalSanitizer
}

func newOutputForwarder(output <-chan []byte, out *os.File, redrawChrome func()) *outputForwarder {
	return &outputForwarder{
		output:       output,
		out:          out,
		pauseCh:      make(chan struct{}, 1),
		resumeCh:     make(chan struct{}, 1),
		redrawChrome: redrawChrome,
	}
}

func (f *outputForwarder) run() {
	for {
		select {
		case chunk, ok := <-f.output:
			if !ok {
				return
			}
			filtered, redraw := f.sanitizer.Filter(chunk)
			if len(filtered) > 0 {
				f.out.Write(filtered) //nolint:errcheck
			}
			if f.redrawChrome != nil && (redraw || needsChromeRedraw(filtered)) {
				f.redrawChrome()
			}
		case <-f.pauseCh:
			// Drain output while paused to prevent the PTY read loop from blocking.
			for {
				select {
				case <-f.resumeCh:
					goto resumed
				case <-f.output:
					// discard while paused
				}
			}
		resumed:
		}
	}
}

// swapOutput replaces the channel the forwarder reads from (used after reconnect).
func (f *outputForwarder) swapOutput(ch <-chan []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.output = ch
}

// Pause stops forwarding PTY output to the terminal.
func (f *outputForwarder) Pause() { f.pauseCh <- struct{}{} }

// Resume restarts forwarding PTY output. Must be called after Pause.
func (f *outputForwarder) Resume() { f.resumeCh <- struct{}{} }

type terminalSanitizer struct {
	pending []byte
}

func (s *terminalSanitizer) Filter(chunk []byte) ([]byte, bool) {
	data := append(append([]byte(nil), s.pending...), chunk...)
	s.pending = s.pending[:0]

	out := make([]byte, 0, len(data))
	redraw := false

	for i := 0; i < len(data); {
		if data[i] != 0x1b {
			out = append(out, data[i])
			i++
			continue
		}
		if i+1 >= len(data) {
			s.pending = append(s.pending, data[i:]...)
			break
		}

		switch data[i+1] {
		case '[':
			end, ok := findCSIEnd(data, i+2)
			if !ok {
				s.pending = append(s.pending, data[i:]...)
				return out, redraw
			}
			seq := data[i : end+1]
			params := string(seq[2 : len(seq)-1])
			final := seq[len(seq)-1]
			if drop, seqRedraw := shouldDropCSI(params, final); drop {
				redraw = redraw || seqRedraw
				i = end + 1
				continue
			}
			out = append(out, seq...)
			i = end + 1
		case ']':
			end, ok := findOSCEnd(data, i+2)
			if !ok {
				s.pending = append(s.pending, data[i:]...)
				return out, redraw
			}
			seq := data[i:end]
			content := string(seq[2:])
			if shouldDropOSC(content) {
				redraw = true
				i = end
				continue
			}
			out = append(out, seq...)
			i = end
		case 'c':
			redraw = true
			i += 2
		default:
			out = append(out, data[i], data[i+1])
			i += 2
		}
	}

	return out, redraw
}

func findCSIEnd(data []byte, start int) (int, bool) {
	for i := start; i < len(data); i++ {
		if data[i] >= 0x40 && data[i] <= 0x7e {
			return i, true
		}
	}
	return 0, false
}

func findOSCEnd(data []byte, start int) (int, bool) {
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

func shouldDropCSI(params string, final byte) (bool, bool) {
	switch final {
	case 'h', 'l':
		switch params {
		case "?6", "?1004", "?2004", "?2026":
			return true, true
		}
	case 'n':
		if params == "6" {
			return true, false
		}
	case 'r':
		return true, true
	case 'u':
		if strings.HasPrefix(params, ">") || strings.HasPrefix(params, "<") {
			return true, false
		}
	}
	return false, false
}

func shouldDropOSC(content string) bool {
	return strings.HasPrefix(content, "10;?") || strings.HasPrefix(content, "11;?")
}

func needsChromeRedraw(chunk []byte) bool {
	return strings.Contains(string(chunk), "\x1b[2J") ||
		strings.Contains(string(chunk), "\x1b[J") ||
		strings.Contains(string(chunk), "\x1b[3J") ||
		strings.Contains(string(chunk), "\x1bc") ||
		strings.Contains(string(chunk), "\x1b[?1049h") ||
		strings.Contains(string(chunk), "\x1b[?1049l")
}
