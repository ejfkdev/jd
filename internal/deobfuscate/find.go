package deobfuscate

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// findStringArray detects the obfuscator.io string-array getter function.
// Returns nil if not found.
func findStringArray(prog *ast.Program, tree *scope.Tree) *StringArray {
	var result *StringArray
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if result != nil {
			return jsast.VisitSkip
		}
		// Form: function name() { var arr = [strs...]; return (name = function(){return arr})(); }
		// Form: function name() { var arr = [strs...]; name = function(){return arr;}; return name(); }
		switch t := c.Node.(type) {
		case *ast.FunctionDeclaration:
			if sa := matchStringArrayFn(t.Function, tree); sa != nil {
				sa.Node = t
				result = sa
				return jsast.VisitSkip
			}
		case *ast.VariableStatement:
			// var name = function() { ... } form
			if len(t.List) == 1 && t.List[0].Initializer != nil {
				if fl, ok := t.List[0].Initializer.(*ast.FunctionLiteral); ok {
					if sa := matchStringArrayFn(fl, tree); sa != nil {
						sa.Node = t
						sa.OriginalName = nameOf(t.List[0].Target)
						sa.Name = sa.OriginalName
						result = sa
						return jsast.VisitSkip
					}
				}
			}
			// Simple array: var arr = [strs...] only referenced as arr[numeric]
			if sa := matchSimpleArray(t, tree); sa != nil {
				sa.Node = t
				result = sa
				return jsast.VisitSkip
			}
		case *ast.LexicalDeclaration:
			// const/let arr = [...]  (simple array form only)
			if sa := matchSimpleArrayLex(t, tree); sa != nil {
				sa.Node = t
				result = sa
				return jsast.VisitSkip
			}
		}
		return jsast.VisitKeep
	}})
	return result
}

// matchStringArrayFn checks if fl is a string-array getter:
//
//	function() { var arr = ["strs"]; return (name = function(){return arr})(); }
//
// or
//
//	function() { var arr = ["strs"]; name = function(){return arr;}; return name(); }
func matchStringArrayFn(fl *ast.FunctionLiteral, tree *scope.Tree) *StringArray {
	if fl == nil || fl.ParameterList != nil && len(fl.ParameterList.List) != 0 {
		return nil
	}
	if fl.Body == nil || len(fl.Body.List) < 2 {
		return nil
	}
	stmts := fl.Body.List
	// First statement: var/const/let arr = [string literals...]
	var bindings []*ast.Binding
	switch d := stmts[0].(type) {
	case *ast.VariableStatement:
		bindings = d.List
	case *ast.LexicalDeclaration:
		bindings = d.List
	default:
		return nil
	}
	if len(bindings) != 1 {
		return nil
	}
	b := bindings[0]
	arrIdent, ok := b.Target.(*ast.Identifier)
	if !ok {
		return nil
	}
	arrLit, ok := b.Initializer.(*ast.ArrayLiteral)
	if !ok || arrLit == nil {
		return nil
	}
	strs := extractStringArray(arrLit)
	if strs == nil {
		return nil
	}
	// Last statement: return (name = function(){return arr})();  OR  return name();
	ret, ok := stmts[len(stmts)-1].(*ast.ReturnStatement)
	if !ok || ret.Argument == nil {
		return nil
	}
	// Check the two valid forms.
	if !validateStringArrayBody(stmts, arrIdent) {
		return nil
	}
	name := ""
	if fl.Name != nil {
		name = fl.Name.Name.String()
	}
	return &StringArray{
		Name:         name,
		OriginalName: name,
		Strs:         strs,
		ArrIdent:     arrIdent,
	}
}

// validateStringArrayBody checks that the function body matches one of the
// two obfuscator.io getter forms.
func validateStringArrayBody(stmts []ast.Statement, arrIdent *ast.Identifier) bool {
	if len(stmts) < 2 {
		return false
	}
	last := stmts[len(stmts)-1]
	ret, ok := last.(*ast.ReturnStatement)
	if !ok {
		return false
	}
	// Form 1: return (name = function(){return arr})();  — call of an assignment
	if call, ok := ret.Argument.(*ast.CallExpression); ok {
		if assign, ok := call.Callee.(*ast.AssignExpression); ok && assign.Operator == token.ASSIGN {
			if innerFn, ok := assign.Right.(*ast.FunctionLiteral); ok {
				return returnsIdentifier(innerFn, arrIdent)
			}
		}
	}
	// Form 2: return name();  preceded by  name = function(){return arr}
	if call, ok := ret.Argument.(*ast.CallExpression); ok {
		if id, ok := call.Callee.(*ast.Identifier); ok {
			// The preceding statement must be name = function(){return arr}
			for i := len(stmts) - 2; i >= 0; i-- {
				if es, ok := stmts[i].(*ast.ExpressionStatement); ok {
					if assign, ok := es.Expression.(*ast.AssignExpression); ok && assign.Operator == token.ASSIGN {
						if lid, ok := assign.Left.(*ast.Identifier); ok && lid.Name.String() == id.Name.String() {
							if innerFn, ok := assign.Right.(*ast.FunctionLiteral); ok {
								return returnsIdentifier(innerFn, arrIdent)
							}
						}
					}
				}
			}
		}
	}
	return false
}

// returnsIdentifier reports whether fl's body is a single return statement
// returning the given identifier.
func returnsIdentifier(fl *ast.FunctionLiteral, id *ast.Identifier) bool {
	if fl == nil || fl.Body == nil || len(fl.Body.List) != 1 {
		return false
	}
	ret, ok := fl.Body.List[0].(*ast.ReturnStatement)
	if !ok || ret.Argument == nil {
		return false
	}
	rid, ok := ret.Argument.(*ast.Identifier)
	return ok && rid.Name == id.Name
}

// extractStringArray extracts string literals from an array literal, returning
// nil if any element is not a string literal or undefined.
func extractStringArray(arr *ast.ArrayLiteral) []string {
	if arr == nil {
		return nil
	}
	out := make([]string, 0, len(arr.Value))
	for _, el := range arr.Value {
		if el == nil {
			out = append(out, "undefined")
			continue
		}
		if sl, ok := el.(*ast.StringLiteral); ok {
			out = append(out, sl.Value.String())
		} else if u, ok := el.(*ast.UnaryExpression); ok && u.Operator == token.VOID {
			out = append(out, "undefined")
		} else {
			return nil
		}
	}
	return out
}

// matchSimpleArray detects `var arr = [strs...]` where arr is only referenced
// via arr[<numeric literal < len>]. Returns a StringArray with inlined data.
func matchSimpleArray(vs *ast.VariableStatement, tree *scope.Tree) *StringArray {
	if len(vs.List) != 1 {
		return nil
	}
	return matchSimpleArrayBinding(vs.List[0], tree)
}

// matchSimpleArrayLex is the const/let variant.
func matchSimpleArrayLex(ld *ast.LexicalDeclaration, tree *scope.Tree) *StringArray {
	if len(ld.List) != 1 {
		return nil
	}
	return matchSimpleArrayBinding(ld.List[0], tree)
}

func matchSimpleArrayBinding(b *ast.Binding, tree *scope.Tree) *StringArray {
	if b == nil {
		return nil
	}
	arrIdent, ok := b.Target.(*ast.Identifier)
	if !ok {
		return nil
	}
	arrLit, ok := b.Initializer.(*ast.ArrayLiteral)
	if !ok || arrLit == nil {
		return nil
	}
	strs := extractStringArray(arrLit)
	if strs == nil {
		return nil
	}
	binding := tree.GetBinding(tree.Root, arrIdent.Name.String())
	if binding == nil {
		return nil
	}
	if !binding.IsConstant() {
		return nil
	}
	if len(binding.Refs) == 0 {
		return nil
	}
	for _, ref := range binding.Refs {
		be, ok := ref.Parent.(*ast.BracketExpression)
		if !ok {
			return nil
		}
		leftID, ok := be.Left.(*ast.Identifier)
		if !ok || leftID.Name.String() != arrIdent.Name.String() {
			return nil
		}
		nl, ok := be.Member.(*ast.NumberLiteral)
		if !ok {
			return nil
		}
		idx, ok := toInt(nl.Value)
		if !ok || idx < 0 || idx >= len(strs) {
			return nil
		}
	}
	return &StringArray{
		Name:         arrIdent.Name.String(),
		OriginalName: arrIdent.Name.String(),
		Strs:         strs,
		ArrIdent:     arrIdent,
	}
}

// toInt extracts an int from a goja NumberLiteral value.
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// nameOf extracts the identifier name from a BindingTarget.
func nameOf(target ast.BindingTarget) string {
	if id, ok := target.(*ast.Identifier); ok {
		return id.Name.String()
	}
	return ""
}
