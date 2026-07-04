// Package tty exposes a single helper to detect whether an io.Writer is
// attached to an interactive terminal. It is used by the CLI polling loops to
// switch between the dot-accumulation progress animation (tty) and a
// plain one-line-per-poll rendering (non-tty / CI logs).
package tty

import (
	"io"
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether w is a terminal. The implementation relies on
// type-asserting w to *os.File so it works with os.Stdout and os.Stderr. For
// any other writer (pipes, buffers, *bytes.Buffer in tests) it returns false.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
