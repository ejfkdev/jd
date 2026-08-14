package deobfuscate

import (
	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// findRotator detects the array-rotation IIFE: an ExpressionStatement whose
// expression is a CallExpression (IIFE) called with the string-array function
// as one of its arguments. The IIFE body contains a while/for infinite loop
// with push/shift rotation. We capture the whole statement for sandbox setup.
func findRotator(prog *ast.Program, tree *scope.Tree, arr *StringArray) *Rotator {
	if arr == nil {
		return nil
	}
	var result *Rotator
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if result != nil {
			return jsast.VisitSkip
		}
		es, ok := c.Node.(*ast.ExpressionStatement)
		if !ok {
			return jsast.VisitKeep
		}
		call, ok := es.Expression.(*ast.CallExpression)
		if !ok {
			return jsast.VisitKeep
		}
		// IIFE: callee is a FunctionLiteral, and the string-array function is
		// passed as an argument (directly or as identifier reference).
		fl, ok := call.Callee.(*ast.FunctionLiteral)
		if !ok {
			return jsast.VisitKeep
		}
		hasArrayRef := false
		for _, arg := range call.ArgumentList {
			if id, ok := arg.(*ast.Identifier); ok && id.Name.String() == arr.Name {
				hasArrayRef = true
				break
			}
		}
		if !hasArrayRef {
			// Also check if the IIFE body references the string array function.
			jsast.Walk(fl, jsast.Visit{Enter: func(ic *jsast.Cursor) jsast.Action {
				if id, ok := ic.Node.(*ast.Identifier); ok && id.Name.String() == arr.Name {
					hasArrayRef = true
					return jsast.VisitStop
				}
				return jsast.VisitKeep
			}})
		}
		if !hasArrayRef {
			return jsast.VisitKeep
		}
		// Verify it has an infinite loop with push/shift.
		if !hasPushShiftLoop(fl) {
			return jsast.VisitKeep
		}
		result = &Rotator{
			Stmt:    es,
			ArrayFn: jsast.Ident(arr.Name),
		}
		return jsast.VisitSkip
	}})
	return result
}

// hasPushShiftLoop checks if fl contains a while/for/do-while loop (the
// rotator's infinite loop). We don't strictly require push/shift to be present
// since the sandbox executes the rotator code as-is; the loop + string-array
// reference is enough to identify the rotator IIFE.
func hasPushShiftLoop(fl *ast.FunctionLiteral) bool {
	found := false
	jsast.Walk(fl, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		switch c.Node.(type) {
		case *ast.WhileStatement, *ast.ForStatement, *ast.DoWhileStatement:
			found = true
			return jsast.VisitStop
		}
		return jsast.VisitKeep
	}})
	return found
}
