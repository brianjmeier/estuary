package app

import (
	"bytes"
	"testing"
)

func TestFormatChromeTitleUsesNotice(t *testing.T) {
	got := formatChromeTitle(chromeState{
		Model:   "gpt-5.4",
		Habitat: "codex",
		Folder:  "~/work",
		Notice:  "output truncated while command mode was open",
	})

	want := "◆ estuary | output truncated while command mode was open"
	if got != want {
		t.Fatalf("formatChromeTitle() = %q, want %q", got, want)
	}
}

func TestOscTitleChromeWritesOSCSequence(t *testing.T) {
	var buf bytes.Buffer
	chrome := &oscTitleChrome{out: &buf}
	chrome.Apply(chromeState{
		Model:   "claude-sonnet-4-6",
		Habitat: "claude",
		Folder:  "~/repo",
	})

	got := buf.String()
	want := "\x1b]2;◆ estuary | claude-sonnet-4-6 | claude | ~/repo\a"
	if got != want {
		t.Fatalf("OSC title output = %q, want %q", got, want)
	}
}

func TestOutputForwarderBuffersWhilePaused(t *testing.T) {
	var out bytes.Buffer
	fwd := newOutputForwarder(nil, &out, nil)
	fwd.Pause()

	fwd.appendBuffered([]byte("hello"))
	fwd.appendBuffered([]byte(" world"))

	if truncated := fwd.Resume(); truncated {
		t.Fatal("Resume() returned truncated=true, want false")
	}
	if got := out.String(); got != "hello world" {
		t.Fatalf("buffered output = %q, want %q", got, "hello world")
	}
}

func TestOutputForwarderTruncatesOldestBufferedBytes(t *testing.T) {
	var out bytes.Buffer
	fwd := newOutputForwarder(nil, &out, nil)
	fwd.maxBuffered = 5
	fwd.Pause()

	fwd.appendBuffered([]byte("hello"))
	fwd.appendBuffered([]byte(" world"))

	if truncated := fwd.Resume(); !truncated {
		t.Fatal("Resume() returned truncated=false, want true")
	}
	if got := out.String(); got != "world" {
		t.Fatalf("buffered output = %q, want %q", got, "world")
	}
}

func TestRunningInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if runningInsideTmux() {
		t.Fatal("expected tmux mode to be disabled with empty TMUX")
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if !runningInsideTmux() {
		t.Fatal("expected tmux mode to be enabled when TMUX is set")
	}
}

func TestIsCtrlKSupportsCSIUEncoding(t *testing.T) {
	if !isCtrlK([]byte("\x1b[107;5u")) {
		t.Fatal("expected Ctrl+K CSI-u encoding to be recognized")
	}
	if isCtrlK([]byte("\x1b[107;6u")) {
		t.Fatal("did not expect non-Ctrl-only modifier sequence to match Ctrl+K")
	}
}

func TestIsCtrlCSupportsCSIUEncoding(t *testing.T) {
	if !isCtrlC([]byte("\x1b[99;5u")) {
		t.Fatal("expected Ctrl+C CSI-u encoding to be recognized")
	}
}

func TestIsLeaderRuneMatchesUpperAndLowercase(t *testing.T) {
	if !isLeaderRune([]byte("m"), 'm') {
		t.Fatal("expected lowercase leader rune to match")
	}
	if !isLeaderRune([]byte("M"), 'm') {
		t.Fatal("expected uppercase leader rune to match")
	}
}
