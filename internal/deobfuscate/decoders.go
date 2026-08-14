package deobfuscate

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// findDecoders scans for functions that call the string array into a local var
// and do a computed member access arr[expr] (with optional index shift). Each
// such function is a decoder. Also resolves var and function wrapper aliases.
func findDecoders(prog *ast.Program, tree *scope.Tree, arr *StringArray) []*Decoder {
	var decoders []*Decoder
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		var fl *ast.FunctionLiteral
		var declNode ast.Node
		switch t := c.Node.(type) {
		case *ast.FunctionDeclaration:
			fl = t.Function
			declNode = t
		case *ast.VariableStatement:
			if len(t.List) == 1 && t.List[0].Initializer != nil {
				if f, ok := t.List[0].Initializer.(*ast.FunctionLiteral); ok {
					fl = f
					declNode = t
				}
			}
		}
		if fl == nil {
			return jsast.VisitKeep
		}
		if d := matchDecoder(fl, arr, tree); d != nil {
			d.Node = declNode
			decoders = append(decoders, d)
		}
		return jsast.VisitKeep
	}})
	return decoders
}

// matchDecoder checks if fl calls the string array function (directly or via
// a local var) and does a computed member access arr[expr] (with optional
// index shift like `a -= offset`).
func matchDecoder(fl *ast.FunctionLiteral, arr *StringArray, tree *scope.Tree) *Decoder {
	if fl == nil || fl.Body == nil || fl.ParameterList == nil {
		return nil
	}
	// Detect: (a) a call to the string array function exists in the body, and
	// (b) a computed member access arr[expr] exists where arr is either a local
	// var holding the call result, or the call result itself.
	hasArrayCall := false
	var arrIdent *ast.Identifier
	offset := 0
	hasShift := false

	for _, s := range fl.Body.List {
		switch st := s.(type) {
		case *ast.VariableStatement, *ast.LexicalDeclaration:
			// Support both var and const/let declarations.
			var list []*ast.Binding
			if vs, ok := st.(*ast.VariableStatement); ok {
				list = vs.List
			} else if ld, ok := st.(*ast.LexicalDeclaration); ok {
				list = ld.List
			}
			for _, b := range list {
				if call, ok := b.Initializer.(*ast.CallExpression); ok {
					if id, ok := call.Callee.(*ast.Identifier); ok && id.Name.String() == arr.Name {
						hasArrayCall = true
						if bid, ok := b.Target.(*ast.Identifier); ok {
							arrIdent = bid
						}
					}
				}
			}
		case *ast.ExpressionStatement:
			// Index shift: param -= offset  (or += )
			if assign, ok := st.Expression.(*ast.AssignExpression); ok {
				if id, ok := assign.Left.(*ast.Identifier); ok {
					if be, ok := assign.Right.(*ast.BinaryExpression); ok {
						if nl, ok := be.Right.(*ast.NumberLiteral); ok {
							n, ok := toInt(nl.Value)
							if ok {
								switch be.Operator {
								case token.PLUS:
									if be.Left == id {
										offset = n
										hasShift = true
									}
								case token.MINUS:
									if be.Left == id {
										offset = -n
										hasShift = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Also check for inline calls like `return _0x123()[a]` (no local var).
	if !hasArrayCall {
		var foundCall bool
		jsast.Walk(fl, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
			if call, ok := c.Node.(*ast.CallExpression); ok {
				if id, ok := call.Callee.(*ast.Identifier); ok && id.Name.String() == arr.Name {
					foundCall = true
					return jsast.VisitStop
				}
			}
			return jsast.VisitKeep
		}})
		if !foundCall {
			return nil
		}
	}

	// Verify there's a computed member access. When arrIdent is set, look for
	// arrIdent[expr]; otherwise look for any BracketExpression whose Left is a
	// CallExpression to the string array.
	foundMember := false
	jsast.Walk(fl, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if be, ok := c.Node.(*ast.BracketExpression); ok {
			if arrIdent != nil {
				if id, ok := be.Left.(*ast.Identifier); ok && id.Name.String() == arrIdent.Name.String() {
					foundMember = true
					return jsast.VisitStop
				}
			} else {
				if call, ok := be.Left.(*ast.CallExpression); ok {
					if id, ok := call.Callee.(*ast.Identifier); ok && id.Name.String() == arr.Name {
						foundMember = true
						return jsast.VisitStop
					}
				}
			}
		}
		return jsast.VisitKeep
	}})
	if !foundMember {
		return nil
	}
	_ = hasShift
	return &Decoder{
		Name:     funcName(fl),
		Offset:   offset,
		Enc:      3, // unknown — sandbox will handle
		IndexArg: 0,
	}
}

// funcName returns the binding name of a function literal.
func funcName(fl *ast.FunctionLiteral) string {
	if fl.Name != nil {
		return fl.Name.Name.String()
	}
	return ""
}
