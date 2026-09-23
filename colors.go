package main

import (
	"os"

	"golang.org/x/term"
)

// Colors, one per role, like the web app: dim for time and hints, red for
// failures, lime for good news. Off when the output is not a terminal or
// NO_COLOR is set.
type palette struct{ on bool }

func colorsFor(f *os.File) palette {
	return palette{on: os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(f.Fd()))}
}

func (p palette) paint(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) dim(s string) string  { return p.paint("90", s) }
func (p palette) red(s string) string  { return p.paint("31", s) }
func (p palette) lime(s string) string { return p.paint("92", s) }
func (p palette) bold(s string) string { return p.paint("1", s) }
