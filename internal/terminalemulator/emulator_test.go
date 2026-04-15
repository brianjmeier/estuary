package emulator

import (
	"image/color"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
)

func TestEmulatorPreservesBasicSGRColor(t *testing.T) {
	emu := newTestEmulator(t, 12, 3)

	if _, err := emu.FeedOutput([]byte("\x1b[31mred\x1b[0m")); err != nil {
		t.Fatalf("FeedOutput() error = %v", err)
	}

	cell := emu.term.CellAt(0, 0)
	if cell == nil {
		t.Fatal("CellAt(0, 0) = nil")
	}
	if cell.Content != "r" {
		t.Fatalf("cell content = %q, want %q", cell.Content, "r")
	}
	if cell.Style.Fg != ansi.Red {
		t.Fatalf("cell foreground = %#v, want ansi.Red", cell.Style.Fg)
	}
}

func TestEmulatorPreservesIndexedAndTrueColorSGR(t *testing.T) {
	emu := newTestEmulator(t, 12, 3)

	if _, err := emu.FeedOutput([]byte("\x1b[38;5;202mI\x1b[0m\x1b[38;2;1;2;3mT\x1b[0m")); err != nil {
		t.Fatalf("FeedOutput() error = %v", err)
	}

	indexed := emu.term.CellAt(0, 0)
	if indexed == nil {
		t.Fatal("indexed cell = nil")
	}
	if indexed.Style.Fg != ansi.IndexedColor(202) {
		t.Fatalf("indexed foreground = %#v, want ansi.IndexedColor(202)", indexed.Style.Fg)
	}

	truecolor := emu.term.CellAt(1, 0)
	if truecolor == nil {
		t.Fatal("truecolor cell = nil")
	}
	assertRGB(t, truecolor.Style.Fg, 1, 2, 3)
}

func TestGetScreenReturnsANSIRenderedRows(t *testing.T) {
	emu := newTestEmulator(t, 12, 3)

	if _, err := emu.FeedOutput([]byte("\x1b[31mred\x1b[0m")); err != nil {
		t.Fatalf("FeedOutput() error = %v", err)
	}

	rows := emu.GetScreen().Rows
	if len(rows) != 3 {
		t.Fatalf("GetScreen() returned %d rows, want 3", len(rows))
	}
	if !strings.Contains(rows[0], "\x1b[31m") {
		t.Fatalf("GetScreen() first row did not preserve red ANSI style: %q", rows[0])
	}
	if got := ansi.Strip(rows[0]); !strings.HasPrefix(got, "red") {
		t.Fatalf("ansi.Strip(GetScreen()[0]) = %q, want red prefix", got)
	}
}

func TestOSCWindowTitleDoesNotRenderAsTerminalContent(t *testing.T) {
	emu := newTestEmulator(t, 40, 3)

	if _, err := emu.FeedOutput([]byte("\x1b]0;✳ Claude Code\a\x1b[38;2;215;119;87m ▐\x1b[39m")); err != nil {
		t.Fatalf("FeedOutput() error = %v", err)
	}

	row := ansi.Strip(emu.GetScreen().Rows[0])
	if strings.Contains(row, "Claude Code") {
		t.Fatalf("OSC window title leaked into terminal row: %q", row)
	}
	if !strings.HasPrefix(row, " ▐") {
		t.Fatalf("first row = %q, want logo prefix", row)
	}
}

func TestOSCWindowTitleFilterHandlesChunkBoundaries(t *testing.T) {
	emu := newTestEmulator(t, 40, 3)

	chunks := [][]byte{
		[]byte("\x1b]0;✳"),
		[]byte(" Claude Code"),
		[]byte("\a"),
		[]byte("ready"),
	}
	for _, chunk := range chunks {
		if _, err := emu.FeedOutput(chunk); err != nil {
			t.Fatalf("FeedOutput(%q) error = %v", chunk, err)
		}
	}

	row := ansi.Strip(emu.GetScreen().Rows[0])
	if strings.Contains(row, "Claude Code") {
		t.Fatalf("split OSC window title leaked into terminal row: %q", row)
	}
	if !strings.HasPrefix(row, "ready") {
		t.Fatalf("first row = %q, want ready prefix", row)
	}
}

func TestNonTitleOSCSequencesStillReachEmulator(t *testing.T) {
	emu := newTestEmulator(t, 40, 3)

	if _, err := emu.FeedOutput([]byte("\x1b]8;https://example.test;\alink\x1b]8;;\a")); err != nil {
		t.Fatalf("FeedOutput() error = %v", err)
	}

	cell := emu.term.CellAt(0, 0)
	if cell == nil {
		t.Fatal("CellAt(0, 0) = nil")
	}
	if cell.Link.URL != "https://example.test" {
		t.Fatalf("hyperlink URL = %q, want https://example.test", cell.Link.URL)
	}
}

func TestResizeUpdatesEmulatorAndPTYSize(t *testing.T) {
	emu := newTestEmulator(t, 10, 4)

	if err := emu.Resize(20, 6); err != nil {
		t.Fatalf("Resize() error = %v", err)
	}
	if got := emu.term.Width(); got != 20 {
		t.Fatalf("term.Width() = %d, want 20", got)
	}
	if got := emu.term.Height(); got != 6 {
		t.Fatalf("term.Height() = %d, want 6", got)
	}
	rows, cols, err := pty.Getsize(emu.pty)
	if err != nil {
		t.Fatalf("pty.Getsize() error = %v", err)
	}
	if rows != 6 || cols != 20 {
		t.Fatalf("pty size = %dx%d, want 6x20", rows, cols)
	}
}

func TestStartCommandNormalizesPTYEnv(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	emu := newTestEmulator(t, 10, 4)
	cmd := exec.Command(shell, "-c", "exit 0")
	cmd.Env = []string{
		"TERM=dumb",
		"COLORTERM=",
		"CODEX_THREAD_ID=adapter-thread",
	}

	if err := emu.StartCommand(cmd); err != nil {
		t.Fatalf("StartCommand() error = %v", err)
	}

	assertEnv(t, cmd.Env, "TERM", "xterm-256color")
	assertEnv(t, cmd.Env, "COLORTERM", "truecolor")
	assertEnv(t, cmd.Env, "CODEX_THREAD_ID", "adapter-thread")

	deadline := time.Now().Add(2 * time.Second)
	for !emu.IsProcessExited() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !emu.IsProcessExited() {
		t.Fatal("process did not exit")
	}
}

func newTestEmulator(t *testing.T, cols, rows int) *Emulator {
	t.Helper()
	emu, err := New(cols, rows)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = emu.Close()
	})
	return emu
}

func assertRGB(t *testing.T, got color.Color, wantR, wantG, wantB uint8) {
	t.Helper()
	if got == nil {
		t.Fatal("color = nil")
	}
	r, g, b, _ := got.RGBA()
	if uint8(r>>8) != wantR || uint8(g>>8) != wantG || uint8(b>>8) != wantB {
		t.Fatalf("color RGB = %d,%d,%d, want %d,%d,%d", uint8(r>>8), uint8(g>>8), uint8(b>>8), wantR, wantG, wantB)
	}
}

func assertEnv(t *testing.T, env []string, key, want string) {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if got := strings.TrimPrefix(entry, prefix); got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Fatalf("%s missing from env", key)
}
