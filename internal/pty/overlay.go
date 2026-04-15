package pty

import (
	"fmt"
	"io"
)

// HeaderRows is the number of rows reserved for Estuary chrome at the top.
const (
	HeaderRows = 2
	FooterRows = 0
)

// Overlay manages the ANSI scrolling region and chrome rendering.
//
// The reserved top rows hold Estuary chrome. Everything below is the PTY
// content area; child-terminal coordinates are translated by the app layer.
type Overlay struct {
	out  io.Writer
	rows int
	cols int
}

func NewOverlay(out io.Writer, rows, cols int) *Overlay {
	return &Overlay{out: out, rows: rows, cols: cols}
}

// Setup constrains scrolling to the content area and clears the terminal.
// Call once after entering raw mode.
func (o *Overlay) Setup() {
	o.writeScrollRegion()
	fmt.Fprintf(o.out, "\033[2J") // clear screen
	fmt.Fprintf(o.out, "\033[%d;1H", HeaderRows+1)
}

// Teardown resets the scroll region. Call before exiting raw mode.
func (o *Overlay) Teardown() {
	fmt.Fprintf(o.out, "\033[r") // reset scroll region to full terminal
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
// It saves the cursor, writes both lines using absolute addressing, then
// restores the cursor so the child app keeps control of the content area.
func (o *Overlay) RenderBar(bar, divider string) {
	fmt.Fprintf(o.out, "\0337")            // DECSC: save cursor + attributes
	fmt.Fprintf(o.out, "\033[1;1H\033[2K") // row 1: erase + write bar
	fmt.Fprintf(o.out, "%s", bar)
	fmt.Fprintf(o.out, "\033[2;1H\033[2K") // row 2: erase + write divider
	fmt.Fprintf(o.out, "%s", divider)
	fmt.Fprintf(o.out, "\0338") // DECRC: restore cursor
}

func (o *Overlay) writeScrollRegion() {
	contentTop := HeaderRows + 1
	contentBot := o.rows - FooterRows
	fmt.Fprintf(o.out, "\033[%d;%dr", contentTop, contentBot)
}
