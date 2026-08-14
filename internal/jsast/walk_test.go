package jsast

import (
	"testing"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja/token"
)

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	p, err := parser.ParseFile(nil, "", src, 0)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return p
}

// TestVisitCount verifies the walker visits every expected node kind.
func TestVisitCount(t *testing.T) {
	prog := mustParse(t, `var a = 1 + 2; if (a) { f(a); } else { g(); }`)
	var count int
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		count++
		return VisitKeep
	}})
	if count < 14 {
		t.Fatalf("expected at least 14 node visits, got %d", count)
	}
}

// TestReplaceExpression replaces a+b with a in `f(a+b)`.
func TestReplaceExpression(t *testing.T) {
	prog := mustParse(t, `f(a + b);`)
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		if be, ok := c.Node.(*ast.BinaryExpression); ok && be.Operator == token.PLUS {
			if err := c.Replace(be.Left); err != nil {
				t.Fatalf("replace: %v", err)
			}
		}
		return VisitKeep
	}})
	call := prog.Body[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
	if len(call.ArgumentList) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.ArgumentList))
	}
	if _, ok := call.ArgumentList[0].(*ast.Identifier); !ok {
		t.Fatalf("expected Identifier after replace, got %T", call.ArgumentList[0])
	}
}

// TestRemoveStatement removes the if-statement.
func TestRemoveStatement(t *testing.T) {
	prog := mustParse(t, `if (x) {} y();`)
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		if _, ok := c.Node.(*ast.IfStatement); ok {
			if !c.Remove() {
				t.Fatal("Remove returned false for stmt-list element")
			}
		}
		return VisitKeep
	}})
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	if _, ok := prog.Body[0].(*ast.ExpressionStatement); !ok {
		t.Fatalf("expected ExpressionStatement, got %T", prog.Body[0])
	}
}

// TestReplaceStmts splices two statements in place of one.
func TestReplaceStmts(t *testing.T) {
	prog := mustParse(t, `a(); b();`)
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		if es, ok := c.Node.(*ast.ExpressionStatement); ok {
			if call, ok := es.Expression.(*ast.CallExpression); ok {
				if id, ok := call.Callee.(*ast.Identifier); ok && id.Name == "a" {
					c.ReplaceStmts([]ast.Statement{
						ExprStmt(Call(Ident("x"))),
						ExprStmt(Call(Ident("y"))),
					})
				}
			}
		}
		return VisitKeep
	}})
	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 statements after splice, got %d", len(prog.Body))
	}
}

// TestCloneFunction verifies CloneExpr produces an independent copy.
func TestCloneFunction(t *testing.T) {
	prog := mustParse(t, `(function(a, b){ return a + b; })(1, 2)`)
	var orig *ast.FunctionLiteral
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		if f, ok := c.Node.(*ast.FunctionLiteral); ok {
			orig = f
		}
		return VisitKeep
	}})
	if orig == nil {
		t.Fatal("function literal not found")
	}
	clone := CloneExpr(orig).(*ast.FunctionLiteral)
	if clone == orig {
		t.Fatal("clone is not independent")
	}
	if clone.ParameterList == orig.ParameterList {
		t.Fatal("param list not cloned")
	}
}

// TestSliceSource verifies Idx-based source slicing.
func TestSliceSource(t *testing.T) {
	prog := mustParse(t, `foo("bar");`)
	var lit *ast.StringLiteral
	Walk(prog, Visit{Enter: func(c *Cursor) Action {
		if s, ok := c.Node.(*ast.StringLiteral); ok {
			lit = s
		}
		return VisitKeep
	}})
	if lit == nil {
		t.Fatal("string literal not visited")
	}
	if got := SliceSource(prog, lit); got != `"bar"` {
		t.Fatalf("slice source: got %q", got)
	}
}
