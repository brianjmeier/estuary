package ptyenv

import "strings"

const (
	// DefaultTerm matches Beehive's PTY contract and advertises ANSI color support.
	DefaultTerm = "xterm-256color"
	// DefaultColorTerm gives modern CLIs a truecolor hint without forcing color output.
	DefaultColorTerm = "truecolor"
)

// Build returns an environment for PTY-backed provider processes.
//
// Parent variables are preserved, adapter-specific variables override matching
// parent variables, and terminal capability variables are normalized last so
// provider UIs can emit ANSI color confidently.
func Build(parent, adapter []string) []string {
	out := make([]string, 0, len(parent)+len(adapter)+2)
	index := make(map[string]int, len(parent)+len(adapter)+2)

	add := func(entry string) {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			out = append(out, entry)
			return
		}
		if i, found := index[key]; found {
			out[i] = entry
			return
		}
		index[key] = len(out)
		out = append(out, entry)
	}

	for _, entry := range parent {
		add(entry)
	}
	for _, entry := range adapter {
		add(entry)
	}

	add("TERM=" + DefaultTerm)
	add("COLORTERM=" + DefaultColorTerm)

	return out
}
