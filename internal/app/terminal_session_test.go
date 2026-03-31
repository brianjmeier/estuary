package app

import "testing"

func TestNeedsChromeRedraw(t *testing.T) {
	tests := []struct {
		name  string
		chunk []byte
		want  bool
	}{
		{name: "plain output", chunk: []byte("hello"), want: false},
		{name: "clear screen", chunk: []byte("\x1b[2J\x1b[H"), want: true},
		{name: "scrollback clear", chunk: []byte("\x1b[3J"), want: true},
		{name: "terminal reset", chunk: []byte("\x1bc"), want: true},
		{name: "alternate screen", chunk: []byte("\x1b[?1049h"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsChromeRedraw(tt.chunk); got != tt.want {
				t.Fatalf("needsChromeRedraw(%q) = %v, want %v", tt.chunk, got, tt.want)
			}
		})
	}
}

func TestTerminalSanitizerFiltersGlobalSequences(t *testing.T) {
	s := &terminalSanitizer{}
	chunk := []byte("\x1b[?2004h\x1b[>7u\x1b[?1004h\x1b[6n\x1b]10;?\x1b\\\x1b[1;24rhello")

	got, redraw := s.Filter(chunk)

	if string(got) != "hello" {
		t.Fatalf("filtered output = %q, want %q", got, "hello")
	}
	if !redraw {
		t.Fatal("expected redraw for filtered terminal-global sequences")
	}
}

func TestTerminalSanitizerKeepsRegularCSI(t *testing.T) {
	s := &terminalSanitizer{}
	chunk := []byte("\x1b[22;3Hprompt\x1b[?25h")

	got, redraw := s.Filter(chunk)

	if string(got) != string(chunk) {
		t.Fatalf("filtered output = %q, want %q", got, chunk)
	}
	if redraw {
		t.Fatal("did not expect redraw for regular cursor movement")
	}
}

func TestTerminalSanitizerHandlesSplitEscapeSequence(t *testing.T) {
	s := &terminalSanitizer{}

	got1, redraw1 := s.Filter([]byte("\x1b[?20"))
	got2, redraw2 := s.Filter([]byte("04hbody"))

	if len(got1) != 0 {
		t.Fatalf("first filtered output = %q, want empty", got1)
	}
	if redraw1 {
		t.Fatal("did not expect redraw until sequence completes")
	}
	if string(got2) != "body" {
		t.Fatalf("second filtered output = %q, want %q", got2, "body")
	}
	if !redraw2 {
		t.Fatal("expected redraw once split sequence completes")
	}
}
