// cloner.go registers a generic AST cloner with internal/jsast that clones
// any node by generating its source and re-parsing. This covers exotic node
// types not handled by jsast.CloneExpr/CloneStmt's hand-written table.
package codegen

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"

	"github.com/ejfkdev/jd/internal/jsast"
)

func init() {
	jsast.SetGenericCloner(func(n ast.Node) ast.Node {
		if n == nil {
			return nil
		}
		src := Generate(n, Options{Mode: ModeCompact})
		// Try as an expression first, fall back to a program.
		if expr, ok := n.(ast.Expression); ok {
			if fl, err := parser.ParseFile(nil, "", "("+src+")", 0); err == nil && len(fl.Body) == 1 {
				if es, ok := fl.Body[0].(*ast.ExpressionStatement); ok {
					// Parenthesized; unwrap.
					_ = expr
					return es.Expression
				}
			}
		}
		if prog, err := parser.ParseFile(nil, "", src, 0); err == nil && len(prog.Body) == 1 {
			return prog.Body[0]
		}
		return n
	})
}
