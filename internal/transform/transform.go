// Package transform provides the Transform abstraction and fixpoint runner
// shared by the deobfuscate and unminify pipelines.
package transform

import (
	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// Context carries the AST, source and scope tree through a transform pass.
type Context struct {
	Prog    *ast.Program
	Source  string
	Tree    *scope.Tree
	Changes int
}

// Transform is a single AST-to-AST transformation.
type Transform interface {
	Name() string
	Run(c *Context)
}

// Apply runs each transform in order, recomputing scope before each.
func Apply(c *Context, ts []Transform) {
	for _, t := range ts {
		c.Tree = scope.Analyze(c.Prog)
		c.Changes = 0
		t.Run(c)
	}
}

// ApplyFixpoint runs a set of transforms repeatedly until no transform reports
// changes, or maxPasses is reached. Scope is rebuilt before each pass.
func ApplyFixpoint(c *Context, ts []Transform, maxPasses int) {
	for pass := 0; pass < maxPasses; pass++ {
		c.Tree = scope.Analyze(c.Prog)
		c.Changes = 0
		for _, t := range ts {
			t.Run(c)
		}
		if c.Changes == 0 {
			return
		}
	}
}

// WalkVisit wraps a jsast.Visit into a Transform. Enter returns Action from the
// callback; changes counted when Replace/Remove/ReplaceStmts succeed.
type WalkVisit struct {
	N string
	V func(c *jsast.Cursor, ctx *Context) jsast.Action
}

func (w *WalkVisit) Name() string { return w.N }

func (w *WalkVisit) Run(c *Context) {
	before := c.Changes
	jsast.Walk(c.Prog, jsast.Visit{
		Enter: func(cur *jsast.Cursor) jsast.Action {
			return w.V(cur, c)
		},
	})
	if c.Changes > before {
		// changes recorded inside callback via RecordChange
	}
}

// RecordChange increments the change counter; transforms call this when they
// successfully mutate a node.
func RecordChange(c *Context) { c.Changes++ }
