// Helpers over the goja AST: source slicing, deep cloning of the common
// expression/statement nodes, and small node constructors.
package jsast

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/unistring"
)

// SliceSource returns the source text of n within prog (prog.File.Source()).
// Returns "" when n or prog.File is nil.
func SliceSource(prog *ast.Program, n ast.Node) string {
	if prog == nil || prog.File == nil || n == nil {
		return ""
	}
	src := prog.File.Source()
	start := int(n.Idx0())
	end := int(n.Idx1())
	if start < 1 || end < start || end > len(src) {
		return ""
	}
	return src[start-1 : end-1]
}

// Ident returns an *ast.Identifier with the given name.
func Ident(name string) *ast.Identifier {
	return &ast.Identifier{Name: unistring.String(name)}
}

// Str returns a *ast.StringLiteral. Literal is the quoted raw form; Value is
// the unescaped string.
func Str(literal, value string) *ast.StringLiteral {
	return &ast.StringLiteral{Literal: literal, Value: unistring.String(value)}
}

// Num returns a *ast.NumberLiteral from a numeric value.
func Num(literal string, value interface{}) *ast.NumberLiteral {
	return &ast.NumberLiteral{Literal: literal, Value: value}
}

// Member returns a non-computed DotExpression obj.name.
func Member(obj ast.Expression, name string) *ast.DotExpression {
	return &ast.DotExpression{Left: obj, Identifier: *Ident(name)}
}

// Call returns callee(args...).
func Call(callee ast.Expression, args ...ast.Expression) *ast.CallExpression {
	return &ast.CallExpression{Callee: callee, ArgumentList: args}
}

// Assign returns left op right.
func Assign(op string, left, right ast.Expression) *ast.AssignExpression {
	return &ast.AssignExpression{Operator: assignOp(op), Left: left, Right: right}
}

// Binary returns left op right.
func Binary(op string, left, right ast.Expression) *ast.BinaryExpression {
	return &ast.BinaryExpression{Operator: binOp(op), Left: left, Right: right}
}

// ExprStmt wraps e in an ExpressionStatement.
func ExprStmt(e ast.Expression) *ast.ExpressionStatement {
	return &ast.ExpressionStatement{Expression: e}
}

// Block wraps stmts in a BlockStatement.
func Block(stmts ...ast.Statement) *ast.BlockStatement {
	return &ast.BlockStatement{List: stmts}
}

// CloneExpr returns a deep copy of e. It hand-clones the node types that
// transforms need to duplicate (identifiers, literals, binary/unary/assign/
// conditional/sequence/call/member/array/object/function literals). Exotic
// nodes fall through to CloneGeneric (codegen + reparse), which is always
// correct.
func CloneExpr(e ast.Expression) ast.Expression {
	switch t := e.(type) {
	case *ast.Identifier:
		c := *t
		return &c
	case *ast.StringLiteral:
		c := *t
		return &c
	case *ast.NumberLiteral:
		c := *t
		return &c
	case *ast.BooleanLiteral:
		c := *t
		return &c
	case *ast.NullLiteral:
		c := *t
		return &c
	case *ast.RegExpLiteral:
		c := *t
		return &c
	case *ast.ThisExpression:
		c := *t
		return &c
	case *ast.BinaryExpression:
		return &ast.BinaryExpression{
			Operator: t.Operator, Comparison: t.Comparison,
			Left: CloneExpr(t.Left), Right: CloneExpr(t.Right),
		}
	case *ast.AssignExpression:
		return &ast.AssignExpression{
			Operator: t.Operator,
			Left:     CloneExpr(t.Left), Right: CloneExpr(t.Right),
		}
	case *ast.UnaryExpression:
		return &ast.UnaryExpression{
			Operator: t.Operator, Idx: t.Idx, Postfix: t.Postfix,
			Operand: CloneExpr(t.Operand),
		}
	case *ast.ConditionalExpression:
		return &ast.ConditionalExpression{
			Test:       CloneExpr(t.Test),
			Consequent: CloneExpr(t.Consequent),
			Alternate:  CloneExpr(t.Alternate),
		}
	case *ast.SequenceExpression:
		seq := make([]ast.Expression, len(t.Sequence))
		for i, e := range t.Sequence {
			seq[i] = CloneExpr(e)
		}
		return &ast.SequenceExpression{Sequence: seq}
	case *ast.CallExpression:
		args := make([]ast.Expression, len(t.ArgumentList))
		for i, a := range t.ArgumentList {
			args[i] = CloneExpr(a)
		}
		return &ast.CallExpression{Callee: CloneExpr(t.Callee), ArgumentList: args}
	case *ast.NewExpression:
		args := make([]ast.Expression, len(t.ArgumentList))
		for i, a := range t.ArgumentList {
			args[i] = CloneExpr(a)
		}
		return &ast.NewExpression{Callee: CloneExpr(t.Callee), ArgumentList: args}
	case *ast.DotExpression:
		return &ast.DotExpression{Left: CloneExpr(t.Left), Identifier: t.Identifier}
	case *ast.BracketExpression:
		return &ast.BracketExpression{
			Left: CloneExpr(t.Left), Member: CloneExpr(t.Member),
		}
	case *ast.ArrayLiteral:
		vals := make([]ast.Expression, len(t.Value))
		for i, v := range t.Value {
			vals[i] = CloneExpr(v)
		}
		return &ast.ArrayLiteral{Value: vals}
	case *ast.ObjectLiteral:
		props := make([]ast.Property, len(t.Value))
		for i, p := range t.Value {
			props[i] = cloneProperty(p)
		}
		return &ast.ObjectLiteral{Value: props}
	case *ast.FunctionLiteral:
		return CloneFunction(t)
	case *ast.ArrowFunctionLiteral:
		return &ast.ArrowFunctionLiteral{
			ParameterList: cloneParamList(t.ParameterList),
			Body:          cloneBody(t.Body),
			Async:         t.Async,
		}
	}
	if c := CloneGeneric(e); c != nil {
		if e, ok := c.(ast.Expression); ok {
			return e
		}
	}
	return e
}

func cloneProperty(p ast.Property) ast.Property {
	switch t := p.(type) {
	case *ast.PropertyKeyed:
		return &ast.PropertyKeyed{
			Key: CloneExpr(t.Key), Kind: t.Kind,
			Value: CloneExpr(t.Value), Computed: t.Computed,
		}
	case *ast.PropertyShort:
		c := *t
		if t.Initializer != nil {
			c.Initializer = CloneExpr(t.Initializer)
		}
		return &c
	}
	return p
}

func CloneFunction(f *ast.FunctionLiteral) *ast.FunctionLiteral {
	return &ast.FunctionLiteral{
		Name:          cloneIdentPtr(f.Name),
		ParameterList: cloneParamList(f.ParameterList),
		Body:          cloneBlock(f.Body),
		Async:         f.Async, Generator: f.Generator,
	}
}

func cloneParamList(pl *ast.ParameterList) *ast.ParameterList {
	if pl == nil {
		return nil
	}
	list := make([]*ast.Binding, len(pl.List))
	for i, b := range pl.List {
		list[i] = cloneBinding(b)
	}
	out := &ast.ParameterList{Opening: pl.Opening, Closing: pl.Closing, List: list}
	if pl.Rest != nil {
		out.Rest = CloneExpr(pl.Rest)
	}
	return out
}

func cloneBinding(b *ast.Binding) *ast.Binding {
	if b == nil {
		return nil
	}
	out := &ast.Binding{}
	if b.Target != nil {
		out.Target = b.Target // patterns: keep as-is for read-only transforms
	}
	if b.Initializer != nil {
		out.Initializer = CloneExpr(b.Initializer)
	}
	return out
}

func cloneBlock(b *ast.BlockStatement) *ast.BlockStatement {
	if b == nil {
		return nil
	}
	list := make([]ast.Statement, len(b.List))
	for i, s := range b.List {
		list[i] = CloneStmt(s)
	}
	return &ast.BlockStatement{List: list}
}

func cloneBody(b ast.ConciseBody) ast.ConciseBody {
	switch t := b.(type) {
	case *ast.BlockStatement:
		return cloneBlock(t)
	case *ast.ExpressionBody:
		return &ast.ExpressionBody{Expression: CloneExpr(t.Expression)}
	}
	return b
}

func cloneIdentPtr(id *ast.Identifier) *ast.Identifier {
	if id == nil {
		return nil
	}
	c := *id
	return &c
}

// CloneStmt clones a statement. Covers common statement types; others fall
// through to CloneGeneric.
func CloneStmt(s ast.Statement) ast.Statement {
	switch t := s.(type) {
	case *ast.BlockStatement:
		return cloneBlock(t)
	case *ast.ExpressionStatement:
		return &ast.ExpressionStatement{Expression: CloneExpr(t.Expression)}
	case *ast.ReturnStatement:
		out := &ast.ReturnStatement{}
		if t.Argument != nil {
			out.Argument = CloneExpr(t.Argument)
		}
		return out
	case *ast.IfStatement:
		out := &ast.IfStatement{Test: CloneExpr(t.Test), Consequent: CloneStmt(t.Consequent)}
		if t.Alternate != nil {
			out.Alternate = CloneStmt(t.Alternate)
		}
		return out
	case *ast.EmptyStatement:
		c := *t
		return &c
	case *ast.BranchStatement:
		c := *t
		return &c
	case *ast.VariableStatement:
		list := make([]*ast.Binding, len(t.List))
		for i, b := range t.List {
			list[i] = cloneBinding(b)
		}
		return &ast.VariableStatement{List: list}
	}
	if c := CloneGeneric(s); c != nil {
		if st, ok := c.(ast.Statement); ok {
			return st
		}
	}
	return s
}
