package unminify

import (
	"strconv"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/codegen"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/transform"
)

// --- computed-properties: console["log"] → console.log ---

type computedProperties struct{}

func (computedProperties) Name() string { return "computed-properties" }

func (computedProperties) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		be, ok := cur.Node.(*ast.BracketExpression)
		if !ok {
			return jsast.VisitKeep
		}
		sl, ok := be.Member.(*ast.StringLiteral)
		if !ok {
			return jsast.VisitKeep
		}
		name := sl.Value.String()
		if !isValidIdentifier(name) {
			return jsast.VisitKeep
		}
		// Replace a.b["c"] with a.b.c (collapse nested dot access).
		dot := &ast.DotExpression{Left: be.Left, Identifier: ast.Identifier{Name: sl.Value}}
		cur.Replace(dot)
		transform.RecordChange(c)
		return jsast.VisitKeep
	}})
}

// --- merge-strings: "a" + "b" → "ab" ---

type mergeStrings struct{}

func (mergeStrings) Name() string { return "merge-strings" }

func (mergeStrings) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		be, ok := cur.Node.(*ast.BinaryExpression)
		if !ok || be.Operator != token.PLUS {
			return jsast.VisitKeep
		}
		l, lok := be.Left.(*ast.StringLiteral)
		r, rok := be.Right.(*ast.StringLiteral)
		if !lok || !rok {
			return jsast.VisitKeep
		}
		merged := l.Value.String() + r.Value.String()
		lit := jsast.Str(codegen.QuoteString(merged), merged)
		cur.Replace(lit)
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- unminify-booleans: !0 → true, !1 → false, ![] → false, !![] → true ---

type unminifyBooleans struct{}

func (unminifyBooleans) Name() string { return "unminify-booleans" }

func (unminifyBooleans) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		u, ok := cur.Node.(*ast.UnaryExpression)
		if !ok || u.Operator != token.NOT || u.Postfix {
			return jsast.VisitKeep
		}
		// !0 → true, !1 → false
		if nl, ok := u.Operand.(*ast.NumberLiteral); ok {
			if v, ok := toFloat(nl.Value); ok {
				if v == 0 {
					cur.Replace(&ast.BooleanLiteral{Literal: "true", Value: true})
					transform.RecordChange(c)
					return jsast.VisitSkip
				}
				if v == 1 {
					cur.Replace(&ast.BooleanLiteral{Literal: "false", Value: false})
					transform.RecordChange(c)
					return jsast.VisitSkip
				}
			}
		}
		// ![] → false
		if _, ok := u.Operand.(*ast.ArrayLiteral); ok {
			cur.Replace(&ast.BooleanLiteral{Literal: "false", Value: false})
			transform.RecordChange(c)
			return jsast.VisitSkip
		}
		return jsast.VisitKeep
	}})
}

// --- number-expressions: constant fold numeric binary/unary ---

type numberExpressions struct{}

func (numberExpressions) Name() string { return "number-expressions" }

func (numberExpressions) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		be, ok := cur.Node.(*ast.BinaryExpression)
		if !ok {
			return jsast.VisitKeep
		}
		lf, lok := numericValue(be.Left)
		rf, rok := numericValue(be.Right)
		if !lok || !rok {
			return jsast.VisitKeep
		}
		result, ok := foldBinary(be.Operator, lf, rf)
		if !ok {
			return jsast.VisitKeep
		}
		lit := jsast.Num(formatNumber(result), result)
		cur.Replace(lit)
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- void-to-undefined: void 0 → undefined ---

type voidToUndefined struct{}

func (voidToUndefined) Name() string { return "void-to-undefined" }

func (voidToUndefined) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		u, ok := cur.Node.(*ast.UnaryExpression)
		if !ok || u.Operator != token.VOID || u.Postfix {
			return jsast.VisitKeep
		}
		// Only void 0 → undefined (scope-safe-ish; the real check is whether
		// "undefined" is not shadowed, which we skip for simplicity).
		if nl, ok := u.Operand.(*ast.NumberLiteral); ok {
			if v, ok := toFloat(nl.Value); ok && v == 0 {
				cur.Replace(jsast.Ident("undefined"))
				transform.RecordChange(c)
				return jsast.VisitSkip
			}
		}
		return jsast.VisitKeep
	}})
}

// --- raw-literals: strip hex/escapes in number literals (0x1→1) ---
// goja's AST preserves Literal text; this transform normalizes hex numbers to
// decimal for readability.

type rawLiterals struct{}

func (rawLiterals) Name() string { return "raw-literals" }

func (rawLiterals) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		nl, ok := cur.Node.(*ast.NumberLiteral)
		if !ok {
			return jsast.VisitKeep
		}
		// Don't convert hex/octal literals that are followed by a member
		// access (e.g. 0x400.toFixed() → 1024.toFixed() is a syntax error).
		// 0x400.toFixed() is valid JS, but 1024.toFixed() is not.
		if _, ok := cur.Parent.(*ast.DotExpression); ok {
			return jsast.VisitKeep
		}
		if _, ok := cur.Parent.(*ast.BracketExpression); ok {
			return jsast.VisitKeep
		}
		if v, ok := toFloat(nl.Value); ok {
			newLit := formatNumber(v)
			if newLit != nl.Literal {
				nl.Literal = newLit
				transform.RecordChange(c)
			}
		}
		return jsast.VisitKeep
	}})
}

// --- remove-double-not: !!x → Boolean(x) simplified to x (for boolean coercion) ---
// Conservative: only collapses !!literal and !!boolean.

type removeDoubleNot struct{}

func (removeDoubleNot) Name() string { return "remove-double-not" }

func (removeDoubleNot) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		outer, ok := cur.Node.(*ast.UnaryExpression)
		if !ok || outer.Operator != token.NOT || outer.Postfix {
			return jsast.VisitKeep
		}
		inner, ok := outer.Operand.(*ast.UnaryExpression)
		if !ok || inner.Operator != token.NOT || inner.Postfix {
			return jsast.VisitKeep
		}
		// !!true → true, !!false → false
		if bl, ok := inner.Operand.(*ast.BooleanLiteral); ok {
			cur.Replace(bl)
			transform.RecordChange(c)
			return jsast.VisitSkip
		}
		return jsast.VisitKeep
	}})
}

// --- logical-to-if: a && b() → if (a) b(); a || b() → if (!a) b() ---

type logicalToIf struct{}

func (logicalToIf) Name() string { return "logical-to-if" }

func (logicalToIf) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		es, ok := cur.Node.(*ast.ExpressionStatement)
		if !ok {
			return jsast.VisitKeep
		}
		le, ok := es.Expression.(*ast.BinaryExpression)
		if !ok {
			return jsast.VisitKeep
		}
		if le.Operator != token.LOGICAL_AND && le.Operator != token.LOGICAL_OR {
			return jsast.VisitKeep
		}
		var test ast.Expression
		if le.Operator == token.LOGICAL_AND {
			test = le.Left
		} else {
			// a || b → if (!a) b
			test = &ast.UnaryExpression{Operator: token.NOT, Operand: le.Left}
		}
		stmt := &ast.IfStatement{Test: test, Consequent: &ast.ExpressionStatement{Expression: le.Right}}
		cur.Replace(stmt)
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- ternary-to-if: a ? b : c (statement) → if (a) b else c ---

type ternaryToIf struct{}

func (ternaryToIf) Name() string { return "ternary-to-if" }

func (ternaryToIf) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		es, ok := cur.Node.(*ast.ExpressionStatement)
		if !ok {
			return jsast.VisitKeep
		}
		ce, ok := es.Expression.(*ast.ConditionalExpression)
		if !ok {
			return jsast.VisitKeep
		}
		stmt := &ast.IfStatement{
			Test:       ce.Test,
			Consequent: &ast.ExpressionStatement{Expression: ce.Consequent},
		}
		if ce.Alternate != nil {
			stmt.Alternate = &ast.ExpressionStatement{Expression: ce.Alternate}
		}
		cur.Replace(stmt)
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- merge-else-if: else { if (...) } → else if (...) ---

type mergeElseIf struct{}

func (mergeElseIf) Name() string { return "merge-else-if" }

func (mergeElseIf) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		ifs, ok := cur.Node.(*ast.IfStatement)
		if !ok || ifs.Alternate == nil {
			return jsast.VisitKeep
		}
		blk, ok := ifs.Alternate.(*ast.BlockStatement)
		if !ok || len(blk.List) != 1 {
			return jsast.VisitKeep
		}
		innerIf, ok := blk.List[0].(*ast.IfStatement)
		if !ok {
			return jsast.VisitKeep
		}
		ifs.Alternate = innerIf
		transform.RecordChange(c)
		return jsast.VisitKeep
	}})
}

// --- for-to-while: for(;;) → while(true), for(;test;) → while(test) ---

type forToWhile struct{}

func (forToWhile) Name() string { return "for-to-while" }

func (forToWhile) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		f, ok := cur.Node.(*ast.ForStatement)
		if !ok {
			return jsast.VisitKeep
		}
		if f.Initializer != nil || f.Update != nil {
			return jsast.VisitKeep
		}
		var test ast.Expression = &ast.BooleanLiteral{Literal: "true", Value: true}
		if f.Test != nil {
			test = f.Test
		}
		cur.Replace(&ast.WhileStatement{Test: test, Body: f.Body})
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- yoda: 5 === x → x === 5 (flip literal-first comparisons) ---

type yoda struct{}

func (yoda) Name() string { return "yoda" }

func (yoda) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		be, ok := cur.Node.(*ast.BinaryExpression)
		if !ok || !be.Comparison {
			return jsast.VisitKeep
		}
		if isLiteral(be.Left) && !isLiteral(be.Right) {
			be.Left, be.Right = be.Right, be.Left
			transform.RecordChange(c)
		}
		return jsast.VisitKeep
	}})
}

// --- typeof-undefined: typeof a === "undefined" normalization (no-op for now) ---

type typeofUndefined struct{}

func (typeofUndefined) Name() string { return "typeof-undefined" }

func (typeofUndefined) Run(c *transform.Context) {
	// Stub: full implementation requires scope check for "undefined" shadowing.
}

// --- infinity: 1/0 → Infinity, -1/0 → -Infinity ---

type infinity struct{}

func (infinity) Name() string { return "infinity" }

func (infinity) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		be, ok := cur.Node.(*ast.BinaryExpression)
		if !ok || be.Operator != token.SLASH {
			return jsast.VisitKeep
		}
		ln, lok := numericValue(be.Left)
		rn, rok := numericValue(be.Right)
		if !lok || !rok || rn != 0 {
			return jsast.VisitKeep
		}
		if ln == 1 {
			cur.Replace(jsast.Ident("Infinity"))
			transform.RecordChange(c)
		} else if ln == -1 {
			cur.Replace(&ast.UnaryExpression{Operator: token.MINUS, Operand: jsast.Ident("Infinity")})
			transform.RecordChange(c)
		}
		return jsast.VisitKeep
	}})
}

// --- invert-boolean-logic: !(a == b) → a != b, De Morgan ---

type invertBooleanLogic struct{}

func (invertBooleanLogic) Name() string { return "invert-boolean-logic" }

func (invertBooleanLogic) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		u, ok := cur.Node.(*ast.UnaryExpression)
		if !ok || u.Operator != token.NOT || u.Postfix {
			return jsast.VisitKeep
		}
		be, ok := u.Operand.(*ast.BinaryExpression)
		if !ok {
			return jsast.VisitKeep
		}
		// !(a == b) → a != b
		inverted := invertOperator(be.Operator)
		if inverted != be.Operator {
			be.Operator = inverted
			cur.Replace(be)
			transform.RecordChange(c)
			return jsast.VisitSkip
		}
		return jsast.VisitKeep
	}})
}

func invertOperator(op token.Token) token.Token {
	switch op {
	case token.EQUAL:
		return token.NOT_EQUAL
	case token.STRICT_EQUAL:
		return token.STRICT_NOT_EQUAL
	case token.NOT_EQUAL:
		return token.EQUAL
	case token.STRICT_NOT_EQUAL:
		return token.STRICT_EQUAL
	}
	return op
}

// --- unary-expressions: drop no-op statement-level void/!/typeof ---

type unaryExpressions struct{}

func (unaryExpressions) Name() string { return "unary-expressions" }

func (unaryExpressions) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		es, ok := cur.Node.(*ast.ExpressionStatement)
		if !ok {
			return jsast.VisitKeep
		}
		u, ok := es.Expression.(*ast.UnaryExpression)
		if !ok || u.Postfix {
			return jsast.VisitKeep
		}
		if u.Operator == token.VOID || u.Operator == token.NOT || u.Operator == token.TYPEOF {
			cur.Replace(jsast.ExprStmt(u.Operand))
			transform.RecordChange(c)
			return jsast.VisitSkip
		}
		return jsast.VisitKeep
	}})
}

// --- split-for-loop-vars: hoist unused for-init vars ---

type splitForLoopVars struct{}

func (splitForLoopVars) Name() string { return "split-for-loop-vars" }

func (splitForLoopVars) Run(c *transform.Context) {
	// Stub: requires scope analysis of which vars are used in test/update.
}

// --- block-statements: wrap single-statement bodies in blocks ---

type blockStatements struct{}

func (blockStatements) Name() string { return "block-statements" }

func (blockStatements) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		// Wrap if/while/for/do-while bodies that aren't BlockStatements.
		switch t := cur.Node.(type) {
		case *ast.IfStatement:
			t.Consequent = wrapBlock(t.Consequent)
			if t.Alternate != nil {
				if _, ok := t.Alternate.(*ast.BlockStatement); !ok {
					if _, ok := t.Alternate.(*ast.IfStatement); !ok {
						t.Alternate = wrapBlock(t.Alternate)
					}
				}
			}
		case *ast.WhileStatement:
			t.Body = wrapBlock(t.Body)
		case *ast.ForStatement:
			t.Body = wrapBlock(t.Body)
		case *ast.DoWhileStatement:
			t.Body = wrapBlock(t.Body)
		}
		return jsast.VisitKeep
	}})
}

func wrapBlock(s ast.Statement) ast.Statement {
	if s == nil {
		return s
	}
	if _, ok := s.(*ast.BlockStatement); ok {
		return s
	}
	return jsast.Block(s)
}

// --- sequence: split comma expressions into statements ---

type sequence struct{}

func (sequence) Name() string { return "sequence" }

func (sequence) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		es, ok := cur.Node.(*ast.ExpressionStatement)
		if !ok {
			return jsast.VisitKeep
		}
		seq, ok := es.Expression.(*ast.SequenceExpression)
		if !ok || len(seq.Sequence) <= 1 {
			return jsast.VisitKeep
		}
		stmts := make([]ast.Statement, len(seq.Sequence))
		for i, e := range seq.Sequence {
			stmts[i] = jsast.ExprStmt(e)
		}
		cur.ReplaceStmts(stmts)
		transform.RecordChange(c)
		return jsast.VisitSkip
	}})
}

// --- split-variable-declarations: var a=1,b=2 → var a=1; var b=2 ---

type splitVariableDeclarations struct{}

func (splitVariableDeclarations) Name() string { return "split-variable-declarations" }

func (splitVariableDeclarations) Run(c *transform.Context) {
	jsast.Walk(c.Prog, jsast.Visit{Enter: func(cur *jsast.Cursor) jsast.Action {
		switch t := cur.Node.(type) {
		case *ast.VariableStatement:
			if len(t.List) <= 1 {
				return jsast.VisitKeep
			}
			stmts := make([]ast.Statement, len(t.List))
			for i, b := range t.List {
				stmts[i] = &ast.VariableStatement{List: []*ast.Binding{b}}
			}
			cur.ReplaceStmts(stmts)
			transform.RecordChange(c)
			return jsast.VisitSkip
		case *ast.LexicalDeclaration:
			if len(t.List) <= 1 {
				return jsast.VisitKeep
			}
			stmts := make([]ast.Statement, len(t.List))
			for i, b := range t.List {
				stmts[i] = &ast.LexicalDeclaration{Token: t.Token, List: []*ast.Binding{b}}
			}
			cur.ReplaceStmts(stmts)
			transform.RecordChange(c)
			return jsast.VisitSkip
		}
		return jsast.VisitKeep
	}})
}

// --- helpers ---

func isValidIdentifier(name string) bool {
	if len(name) == 0 {
		return false
	}
	c := name[0]
	if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$') {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$') {
			return false
		}
	}
	return true
}

func isLiteral(e ast.Expression) bool {
	switch e.(type) {
	case *ast.StringLiteral, *ast.NumberLiteral, *ast.BooleanLiteral, *ast.NullLiteral:
		return true
	}
	return false
}

func numericValue(e ast.Expression) (float64, bool) {
	switch t := e.(type) {
	case *ast.NumberLiteral:
		return toFloat(t.Value)
	case *ast.UnaryExpression:
		if t.Postfix {
			return 0, false
		}
		v, ok := numericValue(t.Operand)
		if !ok {
			return 0, false
		}
		switch t.Operator {
		case token.MINUS:
			return -v, true
		case token.PLUS:
			return v, true
		case token.BITWISE_NOT:
			return float64(^int(v)), true
		}
	}
	return 0, false
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func foldBinary(op token.Token, l, r float64) (float64, bool) {
	switch op {
	case token.PLUS:
		return l + r, true
	case token.MINUS:
		return l - r, true
	case token.MULTIPLY:
		return l * r, true
	case token.SLASH:
		if r == 0 {
			return 0, false
		}
		return l / r, true
	case token.REMAINDER:
		if r == 0 {
			return 0, false
		}
		// JS % uses float64 modulo
		return l - r*float64(int(l/r)), true
	case token.AND:
		return float64(int(l) & int(r)), true
	case token.OR:
		return float64(int(l) | int(r)), true
	case token.EXCLUSIVE_OR:
		return float64(int(l) ^ int(r)), true
	case token.SHIFT_LEFT:
		return float64(int(l) << (uint(r) & 31)), true
	case token.SHIFT_RIGHT:
		return float64(int(l) >> (uint(r) & 31)), true
	case token.UNSIGNED_SHIFT_RIGHT:
		return float64(int(uint32(int(l))) >> (uint(r) & 31)), true
	}
	return 0, false
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
