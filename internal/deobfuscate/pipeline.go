package deobfuscate

import (
	"time"

	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// Pipeline runs the full deobfuscation stage on prog. Returns warnings.
// Mirrors webcrack's src/deobfuscate/index.ts ordering:
//
//	find string array → find rotator → find decoders → decode → remove helpers
//	→ fixpoint {mergeStrings, deadCode, controlFlow*}
func Pipeline(prog *ast.Program, timeout time.Duration) []string {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	tree := scope.Analyze(prog)
	d := FindAll(prog, tree)
	if d == nil {
		// No string array → nothing to do (webcrack's gate).
		return nil
	}
	decoded, warnings := DecodeAll(prog, d, timeout)
	_ = decoded
	// Remove helper nodes: string array declaration, rotator, decoders.
	if d.Array != nil && d.Array.Node != nil {
		removeNode(prog, d.Array.Node)
	}
	if d.Rotator != nil && d.Rotator.Stmt != nil {
		removeNode(prog, d.Rotator.Stmt)
	}
	for _, dec := range d.Decoders {
		if dec.Node != nil {
			removeNode(prog, dec.Node)
		}
	}
	// Remove wrapper alias declarations (var k = d).
	for _, w := range d.Wrappers {
		if w.Node != nil {
			removeNode(prog, w.Node)
		}
	}
	return warnings
}

// removeNode walks prog and removes the first statement matching target from
// its enclosing statement list (Program.Body or BlockStatement.List).
func removeNode(prog *ast.Program, target ast.Node) {
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if c.Node == target {
			c.Remove()
			return jsast.VisitStop
		}
		return jsast.VisitKeep
	}})
}
