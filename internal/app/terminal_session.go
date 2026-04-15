package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
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

const pausedOutputLimit = 1 << 20

// TerminalSession is the PTY-first runtime. The child process owns the full
// terminal surface and scrollback; Estuary chrome is kept out-of-band via
// host title updates and a minimal Ctrl+K leader flow.
type TerminalSession struct {
	ctx        context.Context
	cwd        string
	store      *store.Store
	prober     *prereq.Prober
	sessions   *sessions.Service
	handoffSvc *handoff.Service
	ptyMgr     *estuarypty.Manager
	adapters   map[domain.Habitat]providers.TerminalAdapter
	trace      *terminalTrace
	chrome     hostChrome

	session     domain.Session
	sessionList []domain.Session
	ptySess     *estuarypty.Session
	ptyRecordID string
}

// NewTerminalSession creates the PTY-first runtime. It does not start a
// session; call Run() to enter the terminal loop.
func NewTerminalSession(ctx context.Context, cwd string, st *store.Store, prober *prereq.Prober) (*TerminalSession, error) {
	sessionSvc := sessions.NewService(st)
	sessionList, err := sessionSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	trace, err := newTerminalTraceFromEnv()
	if err != nil {
		return nil, err
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
		trace:       trace,
		chrome:      newHostChrome(os.Stdout),
		sessionList: sessionList,
	}, nil
}

// Run enters the raw terminal loop. It blocks until the PTY process exits,
// the user quits, or the context is cancelled.
func (ts *TerminalSession) Run() error {
	if ts.trace != nil {
		defer ts.trace.Close() //nolint:errcheck
	}
	defer ts.chrome.Clear()

	session, err := ts.findOrCreateSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	ts.session = session
	if err := ts.store.TouchSession(ts.ctx, session.ID); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	ts.sessionList = prependSession(session, ts.sessionList)
	ts.renderChrome("")

	stdinFd := int(os.Stdin.Fd())
	savedState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(stdinFd, savedState) //nolint:errcheck

	rows, cols := terminalSize()
	if err := ts.spawnPTY(rows, cols, domain.AttachStrategyFresh, ""); err != nil {
		return err
	}

	fwd := newOutputForwarder(ts.ptySess.Output, os.Stdout, ts.trace)
	go fwd.run()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			r, c := terminalSize()
			_ = ts.ptyMgr.Resize(ts.ptySess.ID, r, c)
			ts.renderChrome("")
		}
	}()

	return ts.inputLoop(stdinFd, savedState, fwd)
}

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

func (ts *TerminalSession) inputLoop(stdinFd int, savedState *term.State, fwd *outputForwarder) error {
	buf := make([]byte, 256)
	for {
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
		if ts.trace != nil {
			ts.trace.Logf("stdin raw=%s", traceBytes(data))
		}

		if isCtrlK(data) {
			quit, err := ts.handleLeaderMode(stdinFd, savedState, fwd)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
			continue
		}

		if len(data) > 0 {
			if ts.trace != nil {
				ts.trace.Logf("stdin forwarded=%s", traceBytes(data))
			}
			_ = ts.ptyMgr.Write(ts.ptySess.ID, data)
		}
	}
}

func (ts *TerminalSession) handleLeaderMode(stdinFd int, savedState *term.State, fwd *outputForwarder) (bool, error) {
	fwd.Pause()
	ts.renderChrome("leader: ? help · s session · m model · r reconnect · q quit")

	data, ok, err := readInputWithTimeout(stdinFd, time.Second)
	if err != nil {
		truncated := fwd.Resume()
		ts.renderResumeNotice(truncated)
		return false, nil
	}
	if !ok {
		truncated := fwd.Resume()
		ts.renderResumeNotice(truncated)
		return false, nil
	}
	if ts.trace != nil {
		ts.trace.Logf("leader raw=%s", traceBytes(data))
	}

	prevPTY := ts.ptySess
	var quit bool

	switch {
	case isQuestionMark(data):
		if err := ts.showLeaderHelp(stdinFd, savedState); err != nil {
			truncated := fwd.Resume()
			ts.renderResumeNotice(truncated)
			return false, err
		}
	case isLeaderRune(data, 's'):
		if err := ts.promptSwitchSession(stdinFd, savedState); err != nil {
			truncated := fwd.Resume()
			ts.renderResumeNotice(truncated)
			return false, err
		}
	case isLeaderRune(data, 'm'):
		if err := ts.promptSwitchModel(stdinFd, savedState); err != nil {
			truncated := fwd.Resume()
			ts.renderResumeNotice(truncated)
			return false, err
		}
	case isLeaderRune(data, 'r'):
		if err := ts.reconnectPTY(); err != nil {
			truncated := fwd.Resume()
			ts.renderResumeNotice(truncated)
			return false, err
		}
	case isLeaderRune(data, 'q'):
		quit = true
	}

	if ts.ptySess != prevPTY {
		fwd.swapOutput(ts.ptySess.Output)
	}
	truncated := fwd.Resume()
	ts.renderResumeNotice(truncated)

	return quit, nil
}

func (ts *TerminalSession) handlePTYExit(stdinFd int, savedState *term.State, fwd *outputForwarder) error {
	exitCode := ts.ptySess.ExitCode()

	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, exitCode)
	_ = ts.store.UpdateSessionStatus(ts.ctx, ts.session.ID, domain.SessionStatusIdle, ts.session.NativeSessionID)
	_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "pty.exited", map[string]any{"exit_code": exitCode})

	ts.renderExitNotice(fmt.Sprintf("session ended (exit %d) · Ctrl+K r reconnect · Ctrl+K q quit", exitCode))

	buf := make([]byte, 32)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return nil
		}
		data := buf[:n]
		if isCtrlK(data) {
			prevPTY := ts.ptySess
			quit, err := ts.handleLeaderMode(stdinFd, savedState, fwd)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
			if ts.ptySess != prevPTY {
				return ts.inputLoop(stdinFd, savedState, fwd)
			}
			ts.renderExitNotice(fmt.Sprintf("session ended (exit %d) · Ctrl+K r reconnect · Ctrl+K q quit", exitCode))
		}
	}
}

func (ts *TerminalSession) spawnPTY(rows, cols int, strategy domain.AttachStrategy, handoffPacketID string) error {
	adapter, ok := ts.adapters[ts.session.CurrentHabitat]
	if !ok {
		return fmt.Errorf("no terminal adapter for provider %q", ts.session.CurrentHabitat)
	}

	strategyUsed := strategy
	var cmd string
	var args []string
	var env []string

	switch strategy {
	case domain.AttachStrategyHandoff:
		cmd, args, env = adapter.HandoffArgs(ts.session, "")
	case domain.AttachStrategyResume:
		if ts.session.NativeSessionID != "" {
			cmd, args, env = adapter.ResumeArgs(ts.session, ts.session.NativeSessionID)
		} else {
			cmd, args, env = adapter.StartArgs(ts.session)
			strategyUsed = domain.AttachStrategyFresh
		}
	default:
		if ts.session.NativeSessionID != "" {
			cmd, args, env = adapter.ResumeArgs(ts.session, ts.session.NativeSessionID)
			strategyUsed = domain.AttachStrategyResume
		} else {
			cmd, args, env = adapter.StartArgs(ts.session)
		}
	}

	ptySess, err := ts.ptyMgr.Spawn(ts.ctx, estuarypty.SpawnOpts{
		ID:   ts.session.ID,
		Cmd:  cmd,
		Args: args,
		Env:  env,
		Cwd:  ts.session.FolderPath,
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return fmt.Errorf("spawn %s: %w", ts.session.CurrentHabitat, err)
	}
	ts.ptySess = ptySess

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
		AttachStrategy:  strategyUsed,
		NativeSessionID: ts.session.NativeSessionID,
		HandoffPacketID: handoffPacketID,
		Status:          domain.ProviderRuntimeStatusRunning,
		StartedAt:       time.Now().UTC(),
	})
	_ = ts.store.TouchSession(ts.ctx, ts.session.ID)
	ts.renderChrome("")

	return nil
}

func (ts *TerminalSession) switchSession(target domain.Session) error {
	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.store.UpdateSessionStatus(ts.ctx, ts.session.ID, domain.SessionStatusIdle, ts.session.NativeSessionID)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)

	ts.session = target
	ts.sessionList = prependSession(target, ts.sessionList)
	rows, cols := terminalSize()
	if err := ts.spawnPTY(rows, cols, domain.AttachStrategyFresh, ""); err != nil {
		return err
	}
	_ = ts.store.TouchSession(ts.ctx, target.ID)
	return nil
}

func (ts *TerminalSession) reconnectPTY() error {
	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)
	rows, cols := terminalSize()
	return ts.spawnPTY(rows, cols, domain.AttachStrategyFresh, "")
}

func (ts *TerminalSession) switchModel(target domain.ModelDescriptor) error {
	switchType := domain.SwitchTypeSameProvider
	if target.Habitat != ts.session.CurrentHabitat {
		switchType = domain.SwitchTypeCrossProvider
	}

	packet, err := ts.handoffSvc.Generate(ts.ctx, ts.session, target.ID, target.Habitat, switchType)
	if err != nil {
		_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "handoff.error", map[string]any{"error": err.Error()})
	}

	adapter := ts.adapters[ts.session.CurrentHabitat]
	if target.Habitat == ts.session.CurrentHabitat {
		if input := adapter.ModelSwitchInput(target.ID); input != "" {
			ts.session.CurrentModel = target.ID
			_ = ts.store.UpdateSession(ts.ctx, ts.session)
			_ = ts.ptyMgr.Write(ts.ptySess.ID, []byte(input))
			if packet.ID != "" {
				injectionText := handoff.InjectionText(packet)
				spawnedSess := ts.ptySess
				go func() {
					time.Sleep(500 * time.Millisecond)
					if ts.ptySess != spawnedSess {
						return
					}
					_ = ts.ptyMgr.Write(spawnedSess.ID, []byte(injectionText+"\n"))
				}()
				_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "handoff.injected",
					map[string]any{"packet_id": packet.ID, "switch_type": string(switchType)})
			}
			ts.renderChrome("model switched")
			return nil
		}
	}

	_ = ts.store.ClosePTYSession(ts.ctx, ts.ptyRecordID, 0)
	_ = ts.ptyMgr.Close(ts.ptySess.ID)

	ts.session.CurrentModel = target.ID
	ts.session.CurrentHabitat = target.Habitat
	ts.session.NativeSessionID = ""
	_ = ts.store.UpdateSession(ts.ctx, ts.session)

	rows, cols := terminalSize()
	if err := ts.spawnPTY(rows, cols, domain.AttachStrategyHandoff, packet.ID); err != nil {
		return err
	}

	if packet.ID != "" {
		injectionText := handoff.InjectionText(packet)
		spawnedSess := ts.ptySess
		go func() {
			time.Sleep(2 * time.Second)
			if ts.ptySess != spawnedSess {
				return
			}
			_ = ts.ptyMgr.Write(spawnedSess.ID, []byte(injectionText+"\n"))
		}()
		_ = ts.store.AppendEvent(ts.ctx, ts.session.ID, "handoff.injected",
			map[string]any{"packet_id": packet.ID, "switch_type": string(switchType)})
	}

	ts.renderChrome("model switched")
	return nil
}

func (ts *TerminalSession) promptSwitchSession(stdinFd int, savedState *term.State) error {
	sessions, err := ts.sessions.List(ts.ctx)
	if err != nil {
		return err
	}
	ts.sessionList = sessions
	lines := []string{"Estuary sessions:", ""}
	for idx, session := range sessions {
		current := ""
		if session.ID == ts.session.ID {
			current = " (current)"
		}
		lines = append(lines, fmt.Sprintf("%d. %s  [%s / %s]%s", idx+1, shortDir(session.FolderPath), session.CurrentHabitat, session.CurrentModel, current))
	}
	lines = append(lines, "", "Choose a session number and press Enter. Leave blank to cancel.")

	input, err := ts.promptForLine(stdinFd, savedState, lines)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input) == "" {
		return nil
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(sessions) {
		return nil
	}

	target := sessions[choice-1]
	if target.ID == ts.session.ID {
		return nil
	}
	return ts.switchSession(target)
}

func (ts *TerminalSession) promptSwitchModel(stdinFd int, savedState *term.State) error {
	models := habitats.SupportedModels()
	lines := []string{"Supported models:", ""}
	for idx, model := range models {
		current := ""
		if model.ID == ts.session.CurrentModel && model.Habitat == ts.session.CurrentHabitat {
			current = " (current)"
		}
		lines = append(lines, fmt.Sprintf("%d. %s  [%s]%s", idx+1, model.ID, model.Habitat, current))
	}
	lines = append(lines, "", "Choose a model number and press Enter. Leave blank to cancel.")

	input, err := ts.promptForLine(stdinFd, savedState, lines)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input) == "" {
		return nil
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(models) {
		return nil
	}

	return ts.switchModel(models[choice-1])
}

func (ts *TerminalSession) showLeaderHelp(stdinFd int, savedState *term.State) error {
	_, err := ts.promptForLine(stdinFd, savedState, []string{
		"Estuary leader keys:",
		"",
		"Ctrl+K ?  help",
		"Ctrl+K s  switch session",
		"Ctrl+K m  switch model",
		"Ctrl+K r  reconnect current session",
		"Ctrl+K q  quit Estuary",
		"",
		"Press Enter to continue.",
	})
	return err
}

func (ts *TerminalSession) promptForLine(stdinFd int, savedState *term.State, lines []string) (string, error) {
	if err := term.Restore(stdinFd, savedState); err != nil {
		return "", err
	}

	fmt.Fprintln(os.Stdout)
	for _, line := range lines {
		fmt.Fprintln(os.Stdout, line)
	}
	fmt.Fprint(os.Stdout, "> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')

	if _, rawErr := term.MakeRaw(stdinFd); rawErr != nil && err == nil {
		err = rawErr
	}
	return strings.TrimRight(input, "\r\n"), err
}

func (ts *TerminalSession) renderChrome(notice string) {
	ts.chrome.Apply(chromeState{
		Model:   ts.session.CurrentModel,
		Habitat: ts.session.CurrentHabitat,
		Folder:  shortDir(ts.session.FolderPath),
		Notice:  notice,
	})
}

func (ts *TerminalSession) renderResumeNotice(truncated bool) {
	if truncated {
		ts.renderChrome("output truncated while command mode was open")
		return
	}
	ts.renderChrome("")
}

func (ts *TerminalSession) renderExitNotice(notice string) {
	ts.renderChrome(notice)
	fmt.Fprintf(os.Stdout, "\r\n[estuary] %s\r\n", notice)
}

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

func runningInsideTmux() bool {
	return strings.TrimSpace(os.Getenv("TMUX")) != ""
}

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

func isCtrlK(data []byte) bool { return isCtrlRune(data, 'k') }

func isCtrlC(data []byte) bool { return isCtrlRune(data, 'c') }

func isCtrlRune(data []byte, r rune) bool {
	if len(data) == 1 && data[0] == byte(r&0x1f) {
		return true
	}
	return matchesCSIUCtrl(data, r)
}

func isLeaderRune(data []byte, r rune) bool {
	if len(data) != 1 {
		return false
	}
	return data[0] == byte(r) || data[0] == byte(unicode.ToUpper(r))
}

func isQuestionMark(data []byte) bool {
	return len(data) == 1 && data[0] == '?'
}

func matchesCSIUCtrl(data []byte, r rune) bool {
	if len(data) < 6 || data[0] != 0x1b || data[1] != '[' || data[len(data)-1] != 'u' {
		return false
	}

	body := string(data[2 : len(data)-1])
	parts := strings.Split(body, ";")
	if len(parts) != 2 {
		return false
	}

	codepoint, err := strconv.Atoi(parts[0])
	if err != nil || codepoint != int(r) {
		return false
	}

	modifiers, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	return modifiers == 5
}

type outputForwarder struct {
	mu          sync.Mutex
	output      <-chan []byte
	out         io.Writer
	trace       *terminalTrace
	paused      bool
	buffer      []byte
	truncated   bool
	maxBuffered int
}

func newOutputForwarder(output <-chan []byte, out io.Writer, trace *terminalTrace) *outputForwarder {
	return &outputForwarder{
		output:      output,
		out:         out,
		trace:       trace,
		maxBuffered: pausedOutputLimit,
	}
}

func (f *outputForwarder) run() {
	for {
		ch := f.currentOutput()
		chunk, ok := <-ch
		if !ok {
			return
		}
		if f.trace != nil {
			f.trace.Logf("stdout raw=%s", traceBytes(chunk))
		}

		f.mu.Lock()
		if f.paused {
			f.appendBuffered(chunk)
			f.mu.Unlock()
			continue
		}
		f.mu.Unlock()

		_, _ = f.out.Write(chunk)
	}
}

func (f *outputForwarder) currentOutput() <-chan []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.output
}

func (f *outputForwarder) swapOutput(ch <-chan []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.output = ch
}

func (f *outputForwarder) Pause() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused = true
}

func (f *outputForwarder) Resume() bool {
	f.mu.Lock()
	buffer := append([]byte(nil), f.buffer...)
	truncated := f.truncated
	f.buffer = f.buffer[:0]
	f.truncated = false
	f.paused = false
	f.mu.Unlock()

	if len(buffer) > 0 {
		_, _ = f.out.Write(buffer)
	}
	return truncated
}

func (f *outputForwarder) appendBuffered(chunk []byte) {
	if len(chunk) >= f.maxBuffered {
		f.buffer = append(f.buffer[:0], chunk[len(chunk)-f.maxBuffered:]...)
		f.truncated = true
		return
	}

	if overflow := len(f.buffer) + len(chunk) - f.maxBuffered; overflow > 0 {
		f.buffer = append([]byte(nil), f.buffer[overflow:]...)
		f.truncated = true
	}
	f.buffer = append(f.buffer, chunk...)
}

func readInputWithTimeout(stdinFd int, timeout time.Duration) ([]byte, bool, error) {
	if !fdReady(stdinFd, timeout) {
		return nil, false, nil
	}
	buf := make([]byte, 32)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		return nil, false, err
	}
	return buf[:n], true, nil
}

func fdReady(fd int, timeout time.Duration) bool {
	var readfds unix.FdSet
	fdSet(fd, &readfds)
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	n, err := unix.Select(fd+1, &readfds, nil, nil, &tv)
	return err == nil && n > 0
}

func fdSet(fd int, set *unix.FdSet) {
	set.Bits[fd/64] |= 1 << (uint(fd) % 64)
}
