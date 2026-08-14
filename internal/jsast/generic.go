// Generic clone fallback and operator helpers.
package jsast

import (
	"fmt"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
)

// genericCloner is injected by internal/codegen once available, to clone nodes
// that the hand-written CloneExpr/CloneStmt table does not cover. Without
// injection, CloneGeneric returns the node unchanged (transforms only clone
// node shapes they understand).
var genericCloner func(ast.Node) ast.Node

// SetGenericCloner is called by the codegen package during init.
func SetGenericCloner(f func(ast.Node) ast.Node) { genericCloner = f }

// CloneGeneric delegates to the injected generic cloner, or returns n
// unchanged when none is registered.
func CloneGeneric(n ast.Node) ast.Node {
	if genericCloner != nil {
		return genericCloner(n)
	}
	return n
}

// assignOp maps a string operator to its token.Token.
func assignOp(op string) token.Token {
	switch op {
	case "=":
		return token.ASSIGN
	case "+=":
		return token.ADD_ASSIGN
	case "-=":
		return token.SUBTRACT_ASSIGN
	case "*=":
		return token.MULTIPLY_ASSIGN
	case "/=":
		return token.QUOTIENT_ASSIGN
	case "%=":
		return token.REMAINDER_ASSIGN
	case "**=":
		return token.EXPONENT_ASSIGN
	case "&&=":
		return token.LOGICAL_AND_ASSIGN
	case "||=":
		return token.LOGICAL_OR_ASSIGN
	case "??=":
		return token.COALESCE_ASSIGN
	case "<<=":
		return token.SHIFT_LEFT_ASSIGN
	case ">>=":
		return token.SHIFT_RIGHT_ASSIGN
	case ">>>=":
		return token.UNSIGNED_SHIFT_RIGHT_ASSIGN
	case "&=":
		return token.AND_ASSIGN
	case "|=":
		return token.OR_ASSIGN
	case "^=":
		return token.EXCLUSIVE_OR_ASSIGN
	}
	panic(fmt.Sprintf("jsast: unknown assignment operator %q", op))
}

// binOp maps a string operator to its token.Token.
func binOp(op string) token.Token {
	switch op {
	case "+":
		return token.PLUS
	case "-":
		return token.MINUS
	case "*":
		return token.MULTIPLY
	case "/":
		return token.SLASH
	case "%":
		return token.REMAINDER
	case "**":
		return token.EXPONENT
	case "==":
		return token.EQUAL
	case "===":
		return token.STRICT_EQUAL
	case "!=":
		return token.NOT_EQUAL
	case "!==":
		return token.STRICT_NOT_EQUAL
	case "<":
		return token.LESS
	case ">":
		return token.GREATER
	case "<=":
		return token.LESS_OR_EQUAL
	case ">=":
		return token.GREATER_OR_EQUAL
	case "&&":
		return token.LOGICAL_AND
	case "||":
		return token.LOGICAL_OR
	case "??":
		return token.COALESCE
	case "&":
		return token.AND
	case "|":
		return token.OR
	case "^":
		return token.EXCLUSIVE_OR
	case "<<":
		return token.SHIFT_LEFT
	case ">>":
		return token.SHIFT_RIGHT
	case ">>>":
		return token.UNSIGNED_SHIFT_RIGHT
	case "in":
		return token.IN
	case "instanceof":
		return token.INSTANCEOF
	}
	panic(fmt.Sprintf("jsast: unknown binary operator %q", op))
}
