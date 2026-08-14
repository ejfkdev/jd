// Package jsast provides traversal, mutation and cloning helpers over the
// goja AST (github.com/dop251/goja/ast).
//
// goja nodes are concrete structs held in typed fields of their parents.
// Because those fields have heterogeneous types (interface fields like
// ast.Expression, concrete-pointer fields like *ast.BlockStatement, and
// slices of pointers like []*ast.Binding), the walker does not try to
// expose a single typed accessor. Instead, each child node carries its own
// set/remove closures bound to the exact field it lives in.
package jsast

import (
	"fmt"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/file"
)

// Action controls traversal.
type Action int

const (
	VisitKeep Action = iota // descend (default)
	VisitSkip               // do not descend into children
	VisitStop               // halt the whole walk
)

// Visit holds optional enter/exit callbacks.
type Visit struct {
	Enter func(*Cursor) Action
	Exit  func(*Cursor)
}

// Cursor is the visit context for one node.
type Cursor struct {
	Node      ast.Node
	Parent    ast.Node
	Ancestors []ast.Node
	acc       accessor
	replace   ast.Node
	removed   bool
}

// slotKind classifies what a slot holds, so compatible() can answer without
// reflection.
type slotKind int

const (
	kindUnknown slotKind = iota
	kindExprSlot
	kindStmtSlot
	kindStmtListSlot
	kindReadOnly
)

// accessor writes a child back into its parent field. set/remove operate on
// ast.Node and internally assert to the concrete slot type.
type accessor struct {
	canSet    bool
	canRemove bool
	kind      slotKind
	set       func(ast.Node) // writes replacement (asserts internally)
	remove    func()         // deletes from list or clears field
}

// multiStmt is a sentinel for "replace one statement with many".
type multiStmt []ast.Statement

func (multiStmt) Idx0() file.Idx { return 0 }
func (multiStmt) Idx1() file.Idx { return 0 }

// Replace replaces the current node with n. Returns an error if n is not
// assignable to the slot (e.g. putting an expression into a statement slot).
func (c *Cursor) Replace(n ast.Node) error {
	if !c.acc.canSet || !compatible(c.acc, n) {
		return fmt.Errorf("jsast: cannot replace %T with %T", c.Node, n)
	}
	c.replace = n
	c.removed = false
	return nil
}

// ReplaceStmts replaces a statement (in a stmt-list slot) with multiple
// statements. Returns false if the slot does not support expansion.
func (c *Cursor) ReplaceStmts(stmts []ast.Statement) bool {
	if !c.acc.canSet {
		return false
	}
	c.replace = multiStmt(stmts)
	c.removed = false
	return true
}

// Remove marks the current node for removal. Returns false if the slot does
// not support removal.
func (c *Cursor) Remove() bool {
	if !c.acc.canRemove {
		return false
	}
	c.removed = true
	c.replace = nil
	return true
}

// compatible reports whether n may fill the slot behind acc.
func compatible(acc accessor, n ast.Node) bool {
	if ms, ok := n.(multiStmt); ok {
		return len(ms) > 0 && (acc.kind == kindStmtSlot || acc.kind == kindStmtListSlot)
	}
	switch n.(type) {
	case ast.Expression:
		return acc.kind == kindExprSlot || acc.kind == kindStmtListSlot
	case ast.Statement:
		return acc.kind == kindStmtSlot || acc.kind == kindStmtListSlot
	}
	return false
}

// Walk performs depth-first traversal of root.
func Walk(root ast.Node, v Visit) {
	if root == nil {
		return
	}
	walkNode(root, nil, nil, v, accessor{})
}

func walkNode(n ast.Node, parent ast.Node, ancestors []ast.Node, v Visit, acc accessor) Action {
	if n == nil {
		return VisitKeep
	}
	cur := &Cursor{Node: n, Parent: parent, Ancestors: ancestors, acc: acc}
	var action Action
	if v.Enter != nil {
		action = v.Enter(cur)
	}
	if action == VisitStop {
		// Still apply pending mutation (Replace/Remove requested in the same
		// Enter call) before halting.
		if v.Exit != nil {
			v.Exit(cur)
		}
		applyPending(cur)
		return VisitStop
	}
	if action != VisitSkip {
		if stop := visitChildren(n, ancestors, v); stop {
			if v.Exit != nil {
				v.Exit(cur)
			}
			applyPending(cur)
			return VisitStop
		}
	}
	if v.Exit != nil {
		v.Exit(cur)
	}
	applyPending(cur)
	return VisitKeep
}

func applyPending(cur *Cursor) {
	if cur.removed && cur.acc.remove != nil {
		cur.acc.remove()
		return
	}
	if cur.replace != nil && cur.acc.set != nil {
		cur.acc.set(cur.replace)
	}
}

// --- accessor builders ---------------------------------------------------

func accExpr(set func(ast.Expression), remove func()) accessor {
	return accessor{
		canSet: true, canRemove: remove != nil, kind: kindExprSlot,
		set: func(n ast.Node) {
			if e, ok := n.(ast.Expression); ok {
				set(e)
			}
		},
		remove: remove,
	}
}

func accNullableExpr(set func(ast.Expression)) accessor {
	return accExpr(set, func() { set(nil) })
}

func accStmt(set func(ast.Statement)) accessor {
	return accessor{
		canSet: true, canRemove: true, kind: kindStmtSlot,
		set: func(n ast.Node) {
			if ms, ok := n.(multiStmt); ok && len(ms) > 0 {
				set(ms[0])
				return
			}
			if s, ok := n.(ast.Statement); ok {
				set(s)
			}
		},
		remove: func() { set(&ast.EmptyStatement{}) },
	}
}

func accNullableStmt(set func(ast.Statement)) accessor {
	return accessor{
		canSet: true, canRemove: true, kind: kindStmtSlot,
		set: func(n ast.Node) {
			if ms, ok := n.(multiStmt); ok && len(ms) > 0 {
				set(ms[0])
				return
			}
			if s, ok := n.(ast.Statement); ok {
				set(s)
			}
		},
		remove: func() { set(nil) },
	}
}

func accExprElem(list *[]ast.Expression, idx int) accessor {
	return accessor{
		canSet: true, canRemove: true, kind: kindExprSlot,
		set: func(n ast.Node) {
			if e, ok := n.(ast.Expression); ok {
				(*list)[idx] = e
			}
		},
		remove: func() { *list = append((*list)[:idx], (*list)[idx+1:]...) },
	}
}

func accStmtElem(list *[]ast.Statement, idx int) accessor {
	return accessor{
		canSet: true, canRemove: true, kind: kindStmtListSlot,
		set: func(n ast.Node) {
			if ms, ok := n.(multiStmt); ok {
				tail := append([]ast.Statement(nil), (*list)[idx+1:]...)
				head := append([]ast.Statement(nil), (*list)[:idx]...)
				*list = append(append(head, ms...), tail...)
				return
			}
			if s, ok := n.(ast.Statement); ok {
				(*list)[idx] = s
			}
		},
		remove: func() { *list = append((*list)[:idx], (*list)[idx+1:]...) },
	}
}

// accReadOnly marks a child that cannot be mutated from this walker (e.g.
// concrete-typed pointer fields where type change isn't supported in place).
func accReadOnly() accessor {
	return accessor{canSet: false, canRemove: false, kind: kindReadOnly}
}

// child is a (node, accessor) pair returned by children().
type child struct {
	node ast.Node
	acc  accessor
}

// visitChildren dispatches statement lists (handled directly) then per-type
// children.
func visitChildren(n ast.Node, ancestors []ast.Node, v Visit) (stop bool) {
	ancestors = append(ancestors, n)
	// Statement-list fields: enumerate inline so removal/replace-multi work.
	switch list := n.(type) {
	case *ast.Program:
		visitStmtSlice(&list.Body, n, ancestors, v)
		return false
	case *ast.BlockStatement:
		visitStmtSlice(&list.List, n, ancestors, v)
		return false
	case *ast.CaseStatement:
		visitStmtSlice(&list.Consequent, n, ancestors, v)
		return false
	}
	for _, c := range children(n) {
		if c.node == nil {
			continue
		}
		if walkNode(c.node, n, ancestors, v, c.acc) == VisitStop {
			return true
		}
	}
	return false
}

func visitStmtSlice(list *[]ast.Statement, parent ast.Node, ancestors []ast.Node, v Visit) {
	for i := 0; i < len(*list); i++ {
		// capture i for the accessor.
		idx := i
		acc := accStmtElem(list, idx)
		if walkNode((*list)[idx], parent, ancestors, v, acc) == VisitStop {
			return
		}
		// After removal the slice shrank; don't advance i past the new element.
		if idx >= len(*list) || (*list)[idx] == nil {
			// removed via nullable path; nothing to recheck.
		}
	}
}

// children returns the non-list child nodes of n.
func children(n ast.Node) []child {
	var out []child
	switch t := n.(type) {
	// --- statements ---
	case *ast.ExpressionStatement:
		out = append(out, child{t.Expression, accExpr(func(e ast.Expression) { t.Expression = e }, nil)})
	case *ast.VariableStatement:
		for i := range t.List {
			out = append(out, child{t.List[i], accReadOnly()})
		}
	case *ast.LexicalDeclaration:
		for i := range t.List {
			out = append(out, child{t.List[i], accReadOnly()})
		}
	case *ast.VariableDeclaration:
		for i := range t.List {
			out = append(out, child{t.List[i], accReadOnly()})
		}
	case *ast.Binding:
		out = append(out, child{t.Target, accReadOnly()})
		if t.Initializer != nil {
			out = append(out, child{t.Initializer, accExpr(func(e ast.Expression) { t.Initializer = e }, nil)})
		}
	case *ast.IfStatement:
		out = append(out, child{t.Test, accExpr(func(e ast.Expression) { t.Test = e }, nil)})
		out = append(out, child{t.Consequent, accStmt(func(s ast.Statement) { t.Consequent = s })})
		if t.Alternate != nil {
			out = append(out, child{t.Alternate, accNullableStmt(func(s ast.Statement) { t.Alternate = s })})
		}
	case *ast.WhileStatement:
		out = append(out, child{t.Test, accExpr(func(e ast.Expression) { t.Test = e }, nil)})
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
	case *ast.DoWhileStatement:
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
		out = append(out, child{t.Test, accExpr(func(e ast.Expression) { t.Test = e }, nil)})
	case *ast.ForStatement:
		forInitChildren(&out, t.Initializer)
		if t.Test != nil {
			out = append(out, child{t.Test, accNullableExpr(func(e ast.Expression) { t.Test = e })})
		}
		if t.Update != nil {
			out = append(out, child{t.Update, accNullableExpr(func(e ast.Expression) { t.Update = e })})
		}
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
	case *ast.ForInStatement:
		forIntoChildren(&out, t.Into)
		out = append(out, child{t.Source, accExpr(func(e ast.Expression) { t.Source = e }, nil)})
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
	case *ast.ForOfStatement:
		forIntoChildren(&out, t.Into)
		out = append(out, child{t.Source, accExpr(func(e ast.Expression) { t.Source = e }, nil)})
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
	case *ast.ReturnStatement:
		if t.Argument != nil {
			out = append(out, child{t.Argument, accNullableExpr(func(e ast.Expression) { t.Argument = e })})
		}
	case *ast.ThrowStatement:
		out = append(out, child{t.Argument, accExpr(func(e ast.Expression) { t.Argument = e }, nil)})
	case *ast.LabelledStatement:
		out = append(out, child{t.Statement, accStmt(func(s ast.Statement) { t.Statement = s })})
	case *ast.WithStatement:
		out = append(out, child{t.Object, accExpr(func(e ast.Expression) { t.Object = e }, nil)})
		out = append(out, child{t.Body, accStmt(func(s ast.Statement) { t.Body = s })})
	case *ast.TryStatement:
		if t.Body != nil {
			out = append(out, child{t.Body, accReadOnly()})
		}
		if t.Catch != nil {
			out = append(out, child{t.Catch, accReadOnly()})
		}
		if t.Finally != nil {
			out = append(out, child{t.Finally, accReadOnly()})
		}
	case *ast.CatchStatement:
		if t.Body != nil {
			out = append(out, child{t.Body, accReadOnly()})
		}
	case *ast.SwitchStatement:
		out = append(out, child{t.Discriminant, accExpr(func(e ast.Expression) { t.Discriminant = e }, nil)})
		for _, cs := range t.Body {
			out = append(out, child{cs, accReadOnly()})
		}
	case *ast.FunctionDeclaration:
		out = append(out, child{t.Function, accReadOnly()})
	case *ast.FunctionLiteral:
		if t.Name != nil {
			out = append(out, child{t.Name, accReadOnly()})
		}
		for _, b := range params(t.ParameterList) {
			out = append(out, b)
		}
		if t.Body != nil {
			out = append(out, child{t.Body, accReadOnly()})
		}
	case *ast.ArrowFunctionLiteral:
		for _, b := range params(t.ParameterList) {
			out = append(out, b)
		}
		switch b := t.Body.(type) {
		case *ast.BlockStatement:
			out = append(out, child{b, accReadOnly()})
		case *ast.ExpressionBody:
			out = append(out, child{b.Expression, accExpr(func(e ast.Expression) { b.Expression = e }, nil)})
		}
	case *ast.ClassDeclaration:
		out = append(out, child{t.Class, accReadOnly()})
	case *ast.ClassLiteral:
		if t.SuperClass != nil {
			out = append(out, child{t.SuperClass, accNullableExpr(func(e ast.Expression) { t.SuperClass = e })})
		}
		for _, el := range t.Body {
			out = append(out, child{el, accReadOnly()})
		}
	case *ast.ClassStaticBlock:
		if t.Block != nil {
			out = append(out, child{t.Block, accReadOnly()})
		}
	case *ast.MethodDefinition:
		if t.Computed {
			out = append(out, child{t.Key, accExpr(func(e ast.Expression) { t.Key = e }, nil)})
		}
		out = append(out, child{t.Body, accReadOnly()})
	case *ast.FieldDefinition:
		if t.Computed {
			out = append(out, child{t.Key, accExpr(func(e ast.Expression) { t.Key = e }, nil)})
		}
		if t.Initializer != nil {
			out = append(out, child{t.Initializer, accNullableExpr(func(e ast.Expression) { t.Initializer = e })})
		}
	// --- expressions ---
	case *ast.BinaryExpression:
		out = append(out, child{t.Left, accExpr(func(e ast.Expression) { t.Left = e }, nil)})
		out = append(out, child{t.Right, accExpr(func(e ast.Expression) { t.Right = e }, nil)})
	case *ast.AssignExpression:
		out = append(out, child{t.Left, accExpr(func(e ast.Expression) { t.Left = e }, nil)})
		out = append(out, child{t.Right, accExpr(func(e ast.Expression) { t.Right = e }, nil)})
	case *ast.UnaryExpression:
		out = append(out, child{t.Operand, accExpr(func(e ast.Expression) { t.Operand = e }, nil)})
	case *ast.SequenceExpression:
		for i := range t.Sequence {
			out = append(out, child{t.Sequence[i], accExprElem(&t.Sequence, i)})
		}
	case *ast.ConditionalExpression:
		out = append(out, child{t.Test, accExpr(func(e ast.Expression) { t.Test = e }, nil)})
		out = append(out, child{t.Consequent, accExpr(func(e ast.Expression) { t.Consequent = e }, nil)})
		out = append(out, child{t.Alternate, accExpr(func(e ast.Expression) { t.Alternate = e }, nil)})
	case *ast.CallExpression:
		out = append(out, child{t.Callee, accExpr(func(e ast.Expression) { t.Callee = e }, nil)})
		for i := range t.ArgumentList {
			out = append(out, child{t.ArgumentList[i], accExprElem(&t.ArgumentList, i)})
		}
	case *ast.NewExpression:
		out = append(out, child{t.Callee, accExpr(func(e ast.Expression) { t.Callee = e }, nil)})
		for i := range t.ArgumentList {
			out = append(out, child{t.ArgumentList[i], accExprElem(&t.ArgumentList, i)})
		}
	case *ast.DotExpression:
		out = append(out, child{t.Left, accExpr(func(e ast.Expression) { t.Left = e }, nil)})
	case *ast.BracketExpression:
		out = append(out, child{t.Left, accExpr(func(e ast.Expression) { t.Left = e }, nil)})
		out = append(out, child{t.Member, accExpr(func(e ast.Expression) { t.Member = e }, nil)})
	case *ast.Optional:
		out = append(out, child{t.Expression, accReadOnly()})
	case *ast.OptionalChain:
		out = append(out, child{t.Expression, accReadOnly()})
	case *ast.ArrayLiteral:
		for i := range t.Value {
			out = append(out, child{t.Value[i], accExprElem(&t.Value, i)})
		}
	case *ast.SpreadElement:
		out = append(out, child{t.Expression, accReadOnly()})
	case *ast.ObjectLiteral:
		for i := range t.Value {
			out = append(out, child{t.Value[i], accReadOnly()})
		}
	case *ast.PropertyKeyed:
		if t.Computed {
			out = append(out, child{t.Key, accExpr(func(e ast.Expression) { t.Key = e }, nil)})
		}
		out = append(out, child{t.Value, accExpr(func(e ast.Expression) { t.Value = e }, nil)})
	case *ast.PropertyShort:
		if t.Initializer != nil {
			out = append(out, child{t.Initializer, accNullableExpr(func(e ast.Expression) { t.Initializer = e })})
		}
	case *ast.TemplateLiteral:
		if t.Tag != nil {
			out = append(out, child{t.Tag, accNullableExpr(func(e ast.Expression) { t.Tag = e })})
		}
		for i := range t.Expressions {
			out = append(out, child{t.Expressions[i], accExprElem(&t.Expressions, i)})
		}
		// leaves: literals, Identifier-in-expr-position, ThisExpression,
		// EmptyStatement, DebuggerStatement, BadExpression, BadStatement,
		// PrivateIdentifier, SuperExpression, MetaProperty, YieldExpression,
		// AwaitExpression.
	}
	return out
}

func params(pl *ast.ParameterList) []child {
	if pl == nil {
		return nil
	}
	var out []child
	for i := range pl.List {
		out = append(out, child{pl.List[i], accReadOnly()})
	}
	if pl.Rest != nil {
		out = append(out, child{pl.Rest, accExpr(func(e ast.Expression) { pl.Rest = e }, nil)})
	}
	return out
}

func forInitChildren(out *[]child, init ast.ForLoopInitializer) {
	switch it := init.(type) {
	case *ast.ForLoopInitializerExpression:
		*out = append(*out, child{it.Expression, accExpr(func(e ast.Expression) { it.Expression = e }, nil)})
	case *ast.ForLoopInitializerVarDeclList:
		for i := range it.List {
			*out = append(*out, child{it.List[i], accReadOnly()})
		}
	case *ast.ForLoopInitializerLexicalDecl:
		for i := range it.LexicalDeclaration.List {
			*out = append(*out, child{it.LexicalDeclaration.List[i], accReadOnly()})
		}
	}
}

func forIntoChildren(out *[]child, into ast.ForInto) {
	switch it := into.(type) {
	case *ast.ForIntoVar:
		if it.Binding != nil {
			*out = append(*out, child{it.Binding, accReadOnly()})
		}
	case *ast.ForDeclaration:
		*out = append(*out, child{it.Target, accReadOnly()})
	case *ast.ForIntoExpression:
		*out = append(*out, child{it.Expression, accExpr(func(e ast.Expression) { it.Expression = e }, nil)})
	}
}
