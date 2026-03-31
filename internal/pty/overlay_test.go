package pty

import (
	"bytes"
	"strings"
	"testing"
)

func TestOverlayContentRows(t *testing.T) {
	o := NewOverlay(new(bytes.Buffer), 30, 100)
	want := 30 - HeaderRows - FooterRows
	if got := o.ContentRows(); got != want {
		t.Errorf("ContentRows() = %d, want %d", got, want)
	}
}

func TestOverlaySetupSetsScrollRegion(t *testing.T) {
	var buf bytes.Buffer
	o := NewOverlay(&buf, 24, 80)
	o.Setup()

	out := buf.String()
	// HeaderRows=2, FooterRows=0: content is rows 3..24
	if !strings.Contains(out, "\033[3;24r") {
		t.Errorf("Setup() output %q does not contain expected scroll region \\033[3;24r", out)
	}
}

func TestOverlaySetupEnablesOriginMode(t *testing.T) {
	var buf bytes.Buffer
	o := NewOverlay(&buf, 24, 80)
	o.Setup()

	out := buf.String()
	if !strings.Contains(out, "\033[?6h") {
		t.Errorf("Setup() output %q missing origin mode enable \\033[?6h", out)
	}
}

func TestOverlayRenderBarPreservesCursor(t *testing.T) {
	var buf bytes.Buffer
	o := NewOverlay(&buf, 24, 80)
	o.RenderBar("◆ estuary  claude-sonnet-4-6  ^K", "divider")

	out := buf.String()
	if !strings.Contains(out, "\0337") {
		t.Error("RenderBar() missing cursor save escape \\0337")
	}
	if !strings.Contains(out, "\0338") {
		t.Error("RenderBar() missing cursor restore escape \\0338")
	}
	if !strings.Contains(out, "claude-sonnet-4-6") {
		t.Error("RenderBar() missing content in output")
	}
	if !strings.Contains(out, "divider") {
		t.Error("RenderBar() missing divider content in output")
	}
}

func TestOverlayRenderBarTogglesOriginMode(t *testing.T) {
	var buf bytes.Buffer
	o := NewOverlay(&buf, 24, 80)
	o.RenderBar("test content", "divider")

	out := buf.String()
	// Must disable origin mode to reach row 1, then re-enable it.
	if !strings.Contains(out, "\033[?6l") {
		t.Error("RenderBar() missing origin mode disable \\033[?6l")
	}
	if !strings.Contains(out, "\033[?6h") {
		t.Error("RenderBar() missing origin mode enable \\033[?6h")
	}
}

func TestOverlayResize(t *testing.T) {
	var buf bytes.Buffer
	o := NewOverlay(&buf, 24, 80)
	buf.Reset()

	o.Resize(40, 120)
	out := buf.String()
	// After resize to 40 rows: content = rows 3..40 (HeaderRows=2, FooterRows=0)
	if !strings.Contains(out, "\033[3;40r") {
		t.Errorf("Resize() output %q missing expected scroll region \\033[3;40r", out)
	}
	if o.ContentRows() != 40-HeaderRows-FooterRows {
		t.Errorf("ContentRows after Resize() = %d, want %d", o.ContentRows(), 40-HeaderRows-FooterRows)
	}
}
