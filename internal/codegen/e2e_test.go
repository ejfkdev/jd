package codegen

import (
	"strings"
	"testing"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/jsast"
)

// TestWalkReplaceRegenerate exercises jsast walker + codegen together: replace
// a+b with a in f(a+b), then regenerate and re-parse.
func TestWalkReplaceRegenerate(t *testing.T) {
	src := `var x = f(a + b);`
	prog, _ := parser.ParseFile(nil, "", src, 0)
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if be, ok := c.Node.(*ast.BinaryExpression); ok && be.Operator == token.PLUS {
			c.Replace(be.Left)
		}
		return jsast.VisitKeep
	}})
	out := Generate(prog, Options{Mode: ModePretty})
	if !strings.Contains(out, "f(a)") {
		t.Fatalf("expected f(a) in output, got:\n%s", out)
	}
	if strings.Contains(out, "a + b") {
		t.Fatalf("a+b still present:\n%s", out)
	}
	if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
		t.Fatalf("regenerated output not parseable: %v\n%s", err, out)
	}
}
