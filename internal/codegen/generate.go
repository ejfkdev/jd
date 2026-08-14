// Package codegen converts a goja AST back into JavaScript source text. It
// supports pretty (indented) and compact (single-line, no comments) modes,
// and can preserve untouched subtrees byte-for-byte via raw-slice fallback.
package codegen

import (
	"github.com/dop251/goja/ast"
)

// Mode selects output formatting.
type Mode int

const (
	ModePretty Mode = iota
	ModeCompact
)

// Options controls generation.
type Options struct {
	Mode    Mode
	Indent  string // pretty-mode indent unit (default two spaces)
	Newline string // pretty-mode line separator (default "\n")
	Source  string // original source; when set, unchanged subtrees with valid
	// Idx ranges are emitted verbatim (raw-slice preservation).
	Dirty func(ast.Node) bool // reports whether n (or an ancestor) was mutated
}

// Generate produces JavaScript source for n.
func Generate(n ast.Node, o Options) string {
	p := printer{
		mode:    o.Mode,
		indent:  o.Indent,
		newline: o.Newline,
		source:  o.Source,
		dirty:   o.Dirty,
	}
	if p.indent == "" {
		p.indent = "  "
	}
	if p.newline == "" {
		p.newline = "\n"
	}
	p.gen(n)
	return p.sb.String()
}

// Program is a convenience that generates a whole program with default
// pretty options (no raw-slice preservation; use Generate for that).
func Program(prog *ast.Program, o Options) string {
	return Generate(prog, o)
}

// StringOf is a compact-mode shorthand for a single expression/statement.
func StringOf(n ast.Node) string {
	return Generate(n, Options{Mode: ModeCompact})
}
