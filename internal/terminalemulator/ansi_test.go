package emulator

import (
	"strings"
	"testing"
)

func TestSplitCSIParamPartsSupportsColonSeparatedParams(t *testing.T) {
	got := splitCSIParamParts("48:5:109")
	want := []string{"48", "5", "109"}
	if len(got) != len(want) {
		t.Fatalf("splitCSIParamParts() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitCSIParamParts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestANSIEscapeUsesReverseModeCode(t *testing.T) {
	seq := string(ANSIEscape(ColWhite.SetMode(ModeReverse), ColBlack))
	if !strings.Contains(seq, "\x1b[7m") {
		t.Fatalf("ANSIEscape() = %q, want reverse mode \\x1b[7m", seq)
	}
	if strings.Contains(seq, "\x1b[6m") {
		t.Fatalf("ANSIEscape() = %q, did not expect \\x1b[6m", seq)
	}
}
