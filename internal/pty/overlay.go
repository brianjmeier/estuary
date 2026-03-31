package pty

import (
	"fmt"
	"io"
)

// HeaderRows is the number of rows reserved for Estuary chrome at the top.
// The scroll region starts at HeaderRows+1, so Claude Code's absolute cursor
// position 1 maps to actual terminal row HeaderRows+1 via origin mode.
const (
	HeaderRows = 2
	FooterRows = 0
)

// Overlay manages the ANSI scrolling region, origin mode, and chrome rendering.
//
// The reserved top row (row 1) holds the status bar. Everything below is the
// PTY content area. Origin mode (DECOM, \033[?6h) makes the child process's
// absolute cursor addresses relative to the scroll region, so Claude Code's
// row 1 maps to actual terminal row 2 — preventing any overlap with the bar.
type Overlay struct {
	out  io.Writer
	rows int
	cols int
}

func NewOverlay(out io.Writer, rows, cols int) *Overlay {
	return &Overlay{out: out, rows: rows, cols: cols}
}

// Setup switches to the alternate screen buffer, sets the scroll region,
// enables origin mode, and clears the content area. Call once after entering
// raw mode.
func (o *Overlay) Setup() {
	fmt.Fprintf(o.out, "\033[?1049h")       // switch to alternate screen buffer
	o.writeScrollRegion()
	fmt.Fprintf(o.out, "\033[?6h")          // DECOM on: cursor addresses relative to scroll region
	fmt.Fprintf(o.out, "\033[2J")           // clear screen
	fmt.Fprintf(o.out, "\033[1;1H")         // cursor to top of scroll region (absolute row HeaderRows+1)
}

// Teardown resets origin mode, scroll region, and returns to the main screen
// buffer. Call before exiting raw mode.
func (o *Overlay) Teardown() {
	fmt.Fprintf(o.out, "\033[?6l") // DECOM off
	fmt.Fprintf(o.out, "\033[r")   // reset scroll region to full terminal
	fmt.Fprintf(o.out, "\033[?1049l") // return to main screen buffer
}

// Resize updates dimensions and resets the scroll region. Call on SIGWINCH.
func (o *Overlay) Resize(rows, cols int) {
	o.rows = rows
	o.cols = cols
	o.writeScrollRegion()
}

// ContentRows returns the number of rows available for the PTY child process.
// The child is spawned with exactly this many rows.
func (o *Overlay) ContentRows() int {
	return o.rows - HeaderRows - FooterRows
}

// Cols returns the current terminal column count.
func (o *Overlay) Cols() int { return o.cols }

// RenderBar writes the status bar (row 1) and a divider line (row 2) to the
// reserved header area.
//
// It saves the cursor, disables origin mode so the header rows are reachable,
// writes both lines, re-enables origin mode, then restores the cursor.
func (o *Overlay) RenderBar(bar, divider string) {
	fmt.Fprintf(o.out, "\0337")            // DECSC: save cursor + attributes
	fmt.Fprintf(o.out, "\033[?6l")         // DECOM off: absolute addressing
	fmt.Fprintf(o.out, "\033[1;1H\033[2K") // row 1: erase + write bar
	fmt.Fprintf(o.out, "%s", bar)
	fmt.Fprintf(o.out, "\033[2;1H\033[2K") // row 2: erase + write divider
	fmt.Fprintf(o.out, "%s", divider)
	fmt.Fprintf(o.out, "\033[?6h")         // DECOM on: back to origin-mode addressing
	fmt.Fprintf(o.out, "\0338")            // DECRC: restore cursor
}

func (o *Overlay) writeScrollRegion() {
	contentTop := HeaderRows + 1
	contentBot := o.rows - FooterRows
	fmt.Fprintf(o.out, "\033[%d;%dr", contentTop, contentBot)
}
