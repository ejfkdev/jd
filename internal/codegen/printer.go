package codegen

import (
	"strconv"
	"strings"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/dop251/goja/unistring"
)

type printer struct {
	sb      strings.Builder
	mode    Mode
	indent  string
	newline string
	level   int
	source  string
	dirty   func(ast.Node) bool
}

func (p *printer) gen(n ast.Node) {
	switch t := n.(type) {
	case *ast.Program:
		p.program(t)
	case *ast.BadStatement:
		if p.source != "" {
			start, end := int(t.Idx0()), int(t.Idx1())
			if start >= 1 && end >= start && end-1 <= len(p.source) {
				p.sb.WriteString(p.source[start-1 : end-1])
			}
		}
	default:
		if n != nil {
			if stmt, ok := n.(ast.Statement); ok {
				p.stmt(stmt)
				return
			}
			if expr, ok := n.(ast.Expression); ok {
				p.expr(expr, 0)
				return
			}
		}
	}
}

// rawSlice attempts to emit n verbatim from the original source. Returns true
// if the node was untouched and a valid Idx range exists in source.
func (p *printer) rawSlice(n ast.Node) bool {
	if p.source == "" || p.dirty == nil || n == nil {
		return false
	}
	if p.dirty(n) {
		return false
	}
	start, end := int(n.Idx0()), int(n.Idx1())
	if start < 1 || end < start || end-1 > len(p.source) {
		return false
	}
	p.sb.WriteString(p.source[start-1 : end-1])
	return true
}

// --- program & statements -------------------------------------------------

func (p *printer) program(prog *ast.Program) {
	for _, s := range prog.Body {
		p.stmt(s)
		p.lineBreak()
	}
}

func (p *printer) lineBreak() {
	if p.mode == ModePretty {
		p.sb.WriteString(p.newline)
		p.writeIndent()
	}
}

func (p *printer) writeIndent() {
	for i := 0; i < p.level; i++ {
		p.sb.WriteString(p.indent)
	}
}

func (p *printer) space() {
	if p.mode == ModePretty {
		p.sb.WriteByte(' ')
	}
}

// opSpace writes a space around binary/unary/assign operators in both modes
// to avoid token-merging ambiguity (e.g. f - -1 → f--1).
func (p *printer) opSpace() {
	p.sb.WriteByte(' ')
}

func (p *printer) sep() {
	p.sb.WriteByte(';')
}

func (p *printer) stmt(s ast.Statement) {
	switch t := s.(type) {
	case *ast.BlockStatement:
		p.block(t)
	case *ast.ExpressionStatement:
		p.expr(t.Expression, 0)
		p.sep()
	case *ast.VariableStatement:
		p.varStmt(t, "var")
	case *ast.LexicalDeclaration:
		kw := "let"
		if t.Token == token.CONST {
			kw = "const"
		}
		p.varStmt2(t.List, kw)
	case *ast.EmptyStatement:
		p.sep()
	case *ast.IfStatement:
		p.ifStmt(t)
	case *ast.WhileStatement:
		p.sb.WriteString("while")
		p.space()
		p.sb.WriteByte('(')
		p.expr(t.Test, 0)
		p.sb.WriteByte(')')
		p.space()
		p.bodyStmt(t.Body)
	case *ast.DoWhileStatement:
		p.sb.WriteString("do")
		p.space()
		p.bodyStmt(t.Body)
		p.space()
		p.sb.WriteString("while")
		p.space()
		p.sb.WriteByte('(')
		p.expr(t.Test, 0)
		p.sb.WriteByte(')')
		p.sep()
	case *ast.ForStatement:
		p.forStmt(t)
	case *ast.ForInStatement:
		p.forInOf(t.Into, t.Source, t.Body, false)
	case *ast.ForOfStatement:
		p.forInOf(t.Into, t.Source, t.Body, true)
	case *ast.ReturnStatement:
		p.sb.WriteString("return")
		if t.Argument != nil {
			p.sb.WriteByte(' ')
			p.expr(t.Argument, 0)
		}
		p.sep()
	case *ast.ThrowStatement:
		p.sb.WriteString("throw ")
		p.expr(t.Argument, 0)
		p.sep()
	case *ast.BranchStatement:
		if t.Token == token.BREAK {
			p.sb.WriteString("break")
		} else {
			p.sb.WriteString("continue")
		}
		if t.Label != nil {
			p.sb.WriteByte(' ')
			p.sb.WriteString(t.Label.Name.String())
		}
		p.sep()
	case *ast.LabelledStatement:
		p.sb.WriteString(t.Label.Name.String())
		p.sb.WriteByte(':')
		p.space()
		p.stmt(t.Statement)
	case *ast.SwitchStatement:
		p.switchStmt(t)
	case *ast.TryStatement:
		p.tryStmt(t)
	case *ast.DebuggerStatement:
		p.sb.WriteString("debugger")
		p.sep()
	case *ast.FunctionDeclaration:
		p.functionLit(t.Function)
		p.sep()
	case *ast.ClassDeclaration:
		p.classLit(t.Class)
	case *ast.WithStatement:
		p.sb.WriteString("with")
		p.space()
		p.sb.WriteByte('(')
		p.expr(t.Object, 0)
		p.sb.WriteByte(')')
		p.space()
		p.bodyStmt(t.Body)
	default:
		if expr, ok := s.(ast.Expression); ok {
			p.expr(expr, 0)
			p.sep()
		}
	}
}

func (p *printer) varStmt(t *ast.VariableStatement, kw string) {
	p.varStmt2(t.List, kw)
}

func (p *printer) varStmt2(list []*ast.Binding, kw string) {
	p.sb.WriteString(kw)
	p.sb.WriteByte(' ')
	for i, b := range list {
		if i > 0 {
			p.sb.WriteString(", ")
		}
		p.binding(b)
	}
	p.sep()
}

func (p *printer) binding(b *ast.Binding) {
	p.pattern(b.Target)
	if b.Initializer != nil {
		p.space()
		p.sb.WriteByte('=')
		p.space()
		p.expr(b.Initializer, precAssign)
	}
}

func (p *printer) pattern(t ast.BindingTarget) {
	switch tt := t.(type) {
	case *ast.Identifier:
		p.sb.WriteString(tt.Name.String())
	case *ast.ArrayPattern:
		p.sb.WriteByte('[')
		for i, e := range tt.Elements {
			if i > 0 {
				p.sb.WriteString(", ")
			}
			if e != nil {
				p.expr(e, precAssign)
			}
		}
		p.sb.WriteByte(']')
	case *ast.ObjectPattern:
		p.sb.WriteByte('{')
		for i, prop := range tt.Properties {
			if i > 0 {
				p.sb.WriteString(", ")
			}
			p.property(prop, true)
		}
		p.sb.WriteByte('}')
	}
}

func (p *printer) ifStmt(t *ast.IfStatement) {
	p.sb.WriteString("if")
	p.space()
	p.sb.WriteByte('(')
	p.expr(t.Test, 0)
	p.sb.WriteByte(')')
	p.space()
	p.bodyStmt(t.Consequent)
	if t.Alternate != nil {
		p.space()
		p.sb.WriteString("else")
		// else-if chain without braces:
		if _, ok := t.Alternate.(*ast.IfStatement); ok {
			p.sb.WriteByte(' ')
			p.stmt(t.Alternate)
			return
		}
		p.space()
		p.bodyStmt(t.Alternate)
	}
}

func (p *printer) forStmt(t *ast.ForStatement) {
	p.sb.WriteString("for")
	p.space()
	p.sb.WriteByte('(')
	if t.Initializer != nil {
		p.forInit(t.Initializer)
	}
	p.sep()
	p.space()
	if t.Test != nil {
		p.expr(t.Test, 0)
	}
	p.sep()
	p.space()
	if t.Update != nil {
		p.expr(t.Update, 0)
	}
	p.sb.WriteByte(')')
	p.space()
	p.bodyStmt(t.Body)
}

func (p *printer) forInit(init ast.ForLoopInitializer) {
	switch it := init.(type) {
	case *ast.ForLoopInitializerExpression:
		p.expr(it.Expression, 0)
	case *ast.ForLoopInitializerVarDeclList:
		p.sb.WriteString("var ")
		for i, b := range it.List {
			if i > 0 {
				p.sb.WriteString(", ")
			}
			p.binding(b)
		}
	case *ast.ForLoopInitializerLexicalDecl:
		kw := "let"
		if it.LexicalDeclaration.Token == token.CONST {
			kw = "const"
		}
		p.sb.WriteString(kw)
		p.sb.WriteByte(' ')
		for i, b := range it.LexicalDeclaration.List {
			if i > 0 {
				p.sb.WriteString(", ")
			}
			p.binding(b)
		}
	}
}

func (p *printer) forInOf(into ast.ForInto, source ast.Expression, body ast.Statement, isOf bool) {
	p.sb.WriteString("for")
	p.space()
	p.sb.WriteByte('(')
	switch it := into.(type) {
	case *ast.ForIntoVar:
		if it.Binding != nil {
			p.sb.WriteString("var ")
			p.binding(it.Binding)
		}
	case *ast.ForDeclaration:
		if it.IsConst {
			p.sb.WriteString("const ")
		} else {
			p.sb.WriteString("let ")
		}
		p.pattern(it.Target)
	case *ast.ForIntoExpression:
		p.expr(it.Expression, 0)
	}
	p.space()
	if isOf {
		p.sb.WriteString("of")
	} else {
		p.sb.WriteString("in")
	}
	p.sb.WriteByte(' ')
	p.expr(source, 0)
	p.sb.WriteByte(')')
	p.space()
	p.bodyStmt(body)
}

func (p *printer) switchStmt(t *ast.SwitchStatement) {
	p.sb.WriteString("switch")
	p.space()
	p.sb.WriteByte('(')
	p.expr(t.Discriminant, 0)
	p.sb.WriteByte(')')
	p.space()
	p.sb.WriteByte('{')
	p.level++
	for _, cs := range t.Body {
		p.lineBreak()
		if cs.Test == nil {
			p.sb.WriteString("default:")
		} else {
			p.sb.WriteString("case ")
			p.expr(cs.Test, 0)
			p.sb.WriteByte(':')
		}
		p.level++
		for _, s := range cs.Consequent {
			p.lineBreak()
			p.stmt(s)
		}
		p.level--
	}
	p.level--
	p.lineBreak()
	p.sb.WriteByte('}')
}

func (p *printer) tryStmt(t *ast.TryStatement) {
	p.sb.WriteString("try")
	p.space()
	if t.Body != nil {
		p.block(t.Body)
	}
	if t.Catch != nil {
		p.space()
		p.sb.WriteString("catch")
		if t.Catch.Parameter != nil {
			p.space()
			p.sb.WriteByte('(')
			p.pattern(t.Catch.Parameter)
			p.sb.WriteByte(')')
		}
		p.space()
		if t.Catch.Body != nil {
			p.block(t.Catch.Body)
		}
	}
	if t.Finally != nil {
		p.space()
		p.sb.WriteString("finally")
		p.space()
		p.block(t.Finally)
	}
}

func (p *printer) block(b *ast.BlockStatement) {
	if b == nil || len(b.List) == 0 {
		p.sb.WriteString("{}")
		return
	}
	p.sb.WriteByte('{')
	p.level++
	for _, s := range b.List {
		p.lineBreak()
		p.stmt(s)
	}
	p.level--
	p.lineBreak()
	p.sb.WriteByte('}')
}

// bodyStmt prints a statement that is the body of a control structure,
// wrapping single-statement bodies in braces when in pretty mode.
func (p *printer) bodyStmt(s ast.Statement) {
	if b, ok := s.(*ast.BlockStatement); ok {
		p.block(b)
		return
	}
	if p.mode == ModePretty {
		p.sb.WriteByte('{')
		p.level++
		p.lineBreak()
		p.stmt(s)
		p.level--
		p.lineBreak()
		p.sb.WriteByte('}')
		return
	}
	p.stmt(s)
}

// --- expressions ----------------------------------------------------------

// Precedence levels (higher binds tighter). Loosely based on escodegen.
const (
	precSeq        = 1
	precAssign     = 2
	precCond       = 3
	precCoalesce   = 4
	precLogicalOr  = 5
	precLogicalAnd = 6
	precBitOr      = 7
	precBitXor     = 8
	precBitAnd     = 9
	precEquality   = 10
	precRelational = 11
	precShift      = 12
	precAdd        = 13
	precMul        = 14
	precExp        = 15
	precUnary      = 16
	precPostfix    = 17
	precCall       = 18
	precPrimary    = 19
	precNoParen    = 0
)

func (p *printer) expr(e ast.Expression, ctx int) {
	if e == nil {
		return
	}
	// raw-slice preservation for unchanged subtrees.
	if p.rawSlice(e) {
		return
	}
	switch t := e.(type) {
	case *ast.BadExpression:
		// Parser recovery artifact (usually a regex or number.member access
		// that goja couldn't parse). Emit the original source text via Idx,
		// but wrap number-dot-identifier patterns in parens to avoid syntax
		// errors (e.g. 1024.toFixed → (1024).toFixed).
		if p.source != "" {
			start, end := int(t.Idx0()), int(t.Idx1())
			if start >= 1 && end >= start && end-1 <= len(p.source) {
				s := p.source[start-1 : end-1]
				s = fixNumberDotAccess(s)
				p.sb.WriteString(s)
				return
			}
		}
	case *ast.Identifier:
		p.sb.WriteString(t.Name.String())
	case *ast.StringLiteral:
		p.sb.WriteString(t.Literal)
	case *ast.NumberLiteral:
		p.sb.WriteString(t.Literal)
	case *ast.BooleanLiteral:
		p.sb.WriteString(t.Literal)
	case *ast.NullLiteral:
		p.sb.WriteString("null")
	case *ast.RegExpLiteral:
		p.sb.WriteString(t.Literal)
	case *ast.ThisExpression:
		p.sb.WriteString("this")
	case *ast.SuperExpression:
		p.sb.WriteString("super")
	case *ast.MetaProperty:
		if t.Meta != nil {
			p.sb.WriteString(t.Meta.Name.String())
		}
		p.sb.WriteByte('.')
		if t.Property != nil {
			p.sb.WriteString(t.Property.Name.String())
		}
	case *ast.BinaryExpression:
		p.binary(t, ctx)
	case *ast.AssignExpression:
		p.assign(t, ctx)
	case *ast.UnaryExpression:
		p.unary(t, ctx)
	case *ast.SequenceExpression:
		p.sequence(t, ctx)
	case *ast.ConditionalExpression:
		p.conditional(t, ctx)
	case *ast.CallExpression:
		p.call(t, ctx)
	case *ast.NewExpression:
		p.newExpr(t, ctx)
	case *ast.DotExpression:
		_, isOptLeft := t.Left.(*ast.Optional)
		_, isOptChainLeft := t.Left.(*ast.OptionalChain)
		if isOptLeft || isOptChainLeft {
			p.expr(t.Left, precCall)
			p.sb.WriteString("?.")
		} else {
			// NumberLiteral followed by .method needs parens: (1024).toFixed
			// to avoid being parsed as a decimal point.
			if _, isNum := t.Left.(*ast.NumberLiteral); isNum {
				p.sb.WriteByte('(')
				p.expr(t.Left, precCall)
				p.sb.WriteByte(')')
			} else {
				p.expr(t.Left, precCall)
			}
			p.sb.WriteByte('.')
		}
		p.sb.WriteString(t.Identifier.Name.String())
	case *ast.BracketExpression:
		_, isOptLeft := t.Left.(*ast.Optional)
		_, isOptChainLeft := t.Left.(*ast.OptionalChain)
		if isOptLeft || isOptChainLeft {
			p.expr(t.Left, precCall)
			p.sb.WriteString("?.[")
		} else {
			p.expr(t.Left, precCall)
			p.sb.WriteByte('[')
		}
		p.expr(t.Member, 0)
		p.sb.WriteByte(']')
	case *ast.Optional:
		// Optional is a wrapper marking short-circuiting on the next member
		// access; the ?. is emitted by DotExpression/BracketExpression when
		// their Left is an Optional. Just recurse.
		p.expr(t.Expression, ctx)
	case *ast.OptionalChain:
		// OptionalChain is the entry of an optional chain; its inner expression
		// carries the dots/brackets. Recurse.
		p.expr(t.Expression, ctx)
	case *ast.ArrayLiteral:
		p.arrayLit(t)
	case *ast.ObjectLiteral:
		p.objectLit(t, ctx)
	case *ast.FunctionLiteral:
		if ctx >= precCall {
			p.sb.WriteByte('(')
			defer p.sb.WriteByte(')')
		}
		p.functionLit(t)
	case *ast.ArrowFunctionLiteral:
		if ctx >= precAssign {
			p.sb.WriteByte('(')
			defer p.sb.WriteByte(')')
		}
		p.arrowLit(t)
	case *ast.TemplateLiteral:
		p.templateLit(t)
	case *ast.SpreadElement:
		p.sb.WriteString("...")
		p.expr(t.Expression, precAssign)
	case *ast.YieldExpression:
		p.sb.WriteString("yield")
		if t.Delegate {
			p.sb.WriteByte('*')
		}
		if t.Argument != nil {
			p.sb.WriteByte(' ')
			p.expr(t.Argument, precAssign)
		}
	case *ast.AwaitExpression:
		p.sb.WriteString("await ")
		p.expr(t.Argument, precUnary)
	case *ast.ClassLiteral:
		if ctx >= precCall {
			p.sb.WriteByte('(')
			defer p.sb.WriteByte(')')
		}
		p.classLit(t)
	}
}

// memberBracket prints obj[member].
func (p *printer) memberBracket(left, member ast.Expression) {
	p.expr(left, precCall)
	p.sb.WriteByte('[')
	p.expr(member, 0)
	p.sb.WriteByte(']')
}

func (p *printer) binary(t *ast.BinaryExpression, ctx int) {
	op := binOpStr(t.Operator)
	rightAssoc := t.Operator == token.EXPONENT
	prec := binPrec(t.Operator)
	lhsCtx := prec
	if rightAssoc {
		lhsCtx = prec + 1
	}
	rhsCtx := prec + 1
	if rightAssoc {
		rhsCtx = prec
	}
	p.expr(t.Left, lhsCtx)
	p.opSpace()
	p.sb.WriteString(op)
	p.opSpace()
	p.expr(t.Right, rhsCtx)
	_ = ctx
}

func (p *printer) assign(t *ast.AssignExpression, ctx int) {
	// Assignment has very low precedence; wrap when the parent context is
	// tighter (e.g. `a || b = c` → `a || (b = c)`).
	if ctx > precAssign {
		p.sb.WriteByte('(')
		defer p.sb.WriteByte(')')
	}
	p.expr(t.Left, precAssign+1)
	p.opSpace()
	p.sb.WriteString(assignOpStr(t.Operator))
	p.opSpace()
	p.expr(t.Right, precAssign+1)
}

func (p *printer) unary(t *ast.UnaryExpression, ctx int) {
	op := unaryOpStr(t.Operator)
	if t.Postfix {
		p.expr(t.Operand, precPostfix)
		p.sb.WriteString(op)
	} else {
		p.sb.WriteString(op)
		// Avoid merging: `typeof x`, `void 0`, `delete x`, `! x`.
		if op[len(op)-1] >= 'a' && op[len(op)-1] <= 'z' {
			p.sb.WriteByte(' ')
		}
		p.expr(t.Operand, precUnary)
	}
	_ = ctx
}

func (p *printer) sequence(t *ast.SequenceExpression, ctx int) {
	needParen := ctx > precSeq
	if needParen {
		p.sb.WriteByte('(')
	}
	for i, e := range t.Sequence {
		if i > 0 {
			p.sb.WriteString(", ")
		}
		p.expr(e, precSeq+1)
	}
	if needParen {
		p.sb.WriteByte(')')
	}
}

func (p *printer) conditional(t *ast.ConditionalExpression, ctx int) {
	needParen := ctx > precCond
	if needParen {
		p.sb.WriteByte('(')
	}
	p.expr(t.Test, precCond+1)
	p.space()
	p.sb.WriteByte('?')
	p.space()
	p.expr(t.Consequent, precCond+1)
	p.space()
	p.sb.WriteByte(':')
	p.space()
	p.expr(t.Alternate, precCond+1)
	if needParen {
		p.sb.WriteByte(')')
	}
}

func (p *printer) call(t *ast.CallExpression, ctx int) {
	// Callees that need parenthesising: assignment, conditional, sequence,
	// function literals, object literals, etc. — any expression whose natural
	// precedence is below call/member. Without parens, `a = f()` parses as a
	// call of `(a=f)`() which is wrong.
	switch t.Callee.(type) {
	case *ast.AssignExpression, *ast.ConditionalExpression, *ast.SequenceExpression,
		*ast.FunctionLiteral, *ast.ArrowFunctionLiteral, *ast.ObjectLiteral,
		*ast.NewExpression, *ast.UnaryExpression:
		p.sb.WriteByte('(')
		p.expr(t.Callee, 0)
		p.sb.WriteByte(')')
	default:
		p.expr(t.Callee, precCall)
	}
	p.sb.WriteByte('(')
	for i, a := range t.ArgumentList {
		if i > 0 {
			p.sb.WriteString(", ")
		}
		p.expr(a, precSeq+1)
	}
	p.sb.WriteByte(')')
	_ = ctx
}

func (p *printer) newExpr(t *ast.NewExpression, ctx int) {
	p.sb.WriteString("new ")
	p.expr(t.Callee, precCall)
	// goja's NewExpression always has a parenthesised arg list when present;
	// ArgumentList is nil only for "new Foo".
	if t.ArgumentList != nil {
		p.sb.WriteByte('(')
		for i, a := range t.ArgumentList {
			if i > 0 {
				p.sb.WriteString(", ")
			}
			p.expr(a, precSeq+1)
		}
		p.sb.WriteByte(')')
	}
	_ = ctx
}

func (p *printer) arrayLit(t *ast.ArrayLiteral) {
	p.sb.WriteByte('[')
	for i, v := range t.Value {
		if i > 0 {
			p.sb.WriteString(", ")
		}
		if v != nil {
			p.expr(v, precSeq+1)
		}
	}
	p.sb.WriteByte(']')
}

func (p *printer) objectLit(t *ast.ObjectLiteral, ctx int) {
	// Wrap in parens when in statement position to avoid block ambiguity.
	if ctx == 0 {
		// Statement position: caller (ExpressionStatement) doesn't pass ctx=0
		// in a way that distinguishes this; the safe approach is to always wrap
		// object literals that appear at the top of an ExpressionStatement.
		// The bodyStmt/stmt path wraps via expr(_,0) so we guard here.
	}
	needParen := ctx >= precPrimary
	if needParen {
		p.sb.WriteByte('(')
	}
	p.sb.WriteByte('{')
	for i, prop := range t.Value {
		if prop == nil {
			continue
		}
		if i > 0 {
			p.sb.WriteString(", ")
		}
		p.property(prop, false)
	}
	p.sb.WriteByte('}')
	if needParen {
		p.sb.WriteByte(')')
	}
}

func (p *printer) property(prop ast.Property, inPattern bool) {
	switch t := prop.(type) {
	case *ast.SpreadElement:
		p.sb.WriteString("...")
		p.expr(t.Expression, precAssign)

	case *ast.PropertyKeyed:
		// async method shorthand: async key() {} — async prefix goes before key.
		if t.Kind == ast.PropertyKindMethod {
			if fl, ok := t.Value.(*ast.FunctionLiteral); ok && fl.Async {
				p.sb.WriteString("async ")
			}
		}
		if t.Kind == ast.PropertyKindGet {
			p.sb.WriteString("get ")
		} else if t.Kind == ast.PropertyKindSet {
			p.sb.WriteString("set ")
		}
		if t.Computed {
			p.sb.WriteByte('[')
			p.expr(t.Key, 0)
			p.sb.WriteByte(']')
		} else {
			p.expr(t.Key, precPrimary)
		}
		if t.Kind == ast.PropertyKindValue {
			p.sb.WriteByte(':')
			p.space()
			p.expr(t.Value, precSeq+1)
		} else if t.Kind == ast.PropertyKindMethod {
			p.sb.WriteByte('(')
			if fl, ok := t.Value.(*ast.FunctionLiteral); ok {
				p.paramList(fl.ParameterList)
			}
			p.sb.WriteByte(')')
			p.space()
			if fl, ok := t.Value.(*ast.FunctionLiteral); ok {
				if fl.Body != nil {
					p.block(fl.Body)
				}
			}
		} else {
			// get/set: key() { body }
			if fl, ok := t.Value.(*ast.FunctionLiteral); ok {
				p.sb.WriteByte('(')
				p.paramList(fl.ParameterList)
				p.sb.WriteByte(')')
				p.space()
				if fl.Body != nil {
					p.block(fl.Body)
				}
			}
		}
	case *ast.PropertyShort:
		p.sb.WriteString(t.Name.Name.String())
		if t.Initializer != nil {
			p.space()
			p.sb.WriteByte('=')
			p.space()
			p.expr(t.Initializer, precAssign)
		}
	}
}

func (p *printer) functionLit(t *ast.FunctionLiteral) {
	if t.Async {
		p.sb.WriteString("async ")
	}
	p.sb.WriteString("function")
	if t.Generator {
		p.sb.WriteByte('*')
	}
	if t.Name != nil {
		p.sb.WriteByte(' ')
		p.sb.WriteString(t.Name.Name.String())
	}
	p.sb.WriteByte('(')
	p.paramList(t.ParameterList)
	p.sb.WriteByte(')')
	p.space()
	if t.Body != nil {
		p.block(t.Body)
	}
}

func (p *printer) paramList(pl *ast.ParameterList) {
	if pl == nil {
		return
	}
	for i, b := range pl.List {
		if i > 0 {
			p.sb.WriteString(", ")
		}
		if b.Target != nil {
			p.pattern(b.Target)
		}
		if b.Initializer != nil {
			p.space()
			p.sb.WriteByte('=')
			p.space()
			p.expr(b.Initializer, precAssign)
		}
	}
	if pl.Rest != nil {
		if len(pl.List) > 0 {
			p.sb.WriteString(", ")
		}
		p.sb.WriteString("...")
		p.expr(pl.Rest, precAssign)
	}
}

func (p *printer) arrowLit(t *ast.ArrowFunctionLiteral) {
	if t.Async {
		p.sb.WriteString("async ")
	}
	if t.ParameterList != nil && len(t.ParameterList.List) == 1 && t.ParameterList.List[0].Initializer == nil && t.ParameterList.Rest == nil {
		// Single-param, no default: omit parens ONLY for a simple identifier.
		// Destructuring patterns ({a, b} or [a, b]) require parens.
		if _, isID := t.ParameterList.List[0].Target.(*ast.Identifier); isID {
			p.pattern(t.ParameterList.List[0].Target)
		} else {
			p.sb.WriteByte('(')
			p.paramList(t.ParameterList)
			p.sb.WriteByte(')')
		}
	} else if t.ParameterList != nil {
		p.sb.WriteByte('(')
		p.paramList(t.ParameterList)
		p.sb.WriteByte(')')
	} else {
		p.sb.WriteString("()")
	}
	p.space()
	p.sb.WriteString("=>")
	p.space()
	switch b := t.Body.(type) {
	case *ast.BlockStatement:
		p.block(b)
	case *ast.ExpressionBody:
		// Arrow function concise body returning an object literal needs parens
		// to avoid being parsed as a block: x => ({a: 1}), not x => {a: 1}.
		if _, isObj := b.Expression.(*ast.ObjectLiteral); isObj {
			p.sb.WriteByte('(')
			p.expr(b.Expression, 0)
			p.sb.WriteByte(')')
		} else {
			p.expr(b.Expression, precAssign)
		}
	}
}

func (p *printer) templateLit(t *ast.TemplateLiteral) {
	if t.Tag != nil {
		p.expr(t.Tag, precCall)
	}
	p.sb.WriteByte('`')
	for i, el := range t.Elements {
		if el != nil {
			p.sb.WriteString(el.Literal)
		}
		if i < len(t.Expressions) {
			p.sb.WriteString("${")
			p.expr(t.Expressions[i], 0)
			p.sb.WriteByte('}')
		}
	}
	p.sb.WriteByte('`')
}

func (p *printer) classLit(t *ast.ClassLiteral) {
	p.sb.WriteString("class")
	if t.Name != nil {
		p.sb.WriteByte(' ')
		p.sb.WriteString(t.Name.Name.String())
	}
	if t.SuperClass != nil {
		p.space()
		p.sb.WriteString("extends")
		p.space()
		p.expr(t.SuperClass, precAssign)
	}
	p.space()
	p.sb.WriteByte('{')
	p.level++
	for _, el := range t.Body {
		p.lineBreak()
		switch e := el.(type) {
		case *ast.MethodDefinition:
			if e.Static {
				p.sb.WriteString("static ")
			}
			if e.Body != nil && e.Body.Async {
				p.sb.WriteString("async ")
			}
			if e.Kind == ast.PropertyKindGet {
				p.sb.WriteString("get ")
			} else if e.Kind == ast.PropertyKindSet {
				p.sb.WriteString("set ")
			}
			if e.Computed {
				p.sb.WriteByte('[')
				p.expr(e.Key, 0)
				p.sb.WriteByte(']')
			} else {
				p.expr(e.Key, precPrimary)
			}
			if e.Body != nil {
				p.sb.WriteByte('(')
				p.paramList(e.Body.ParameterList)
				p.sb.WriteByte(')')
				p.space()
				p.block(e.Body.Body)
			}
		case *ast.FieldDefinition:
			if e.Static {
				p.sb.WriteString("static ")
			}
			if e.Computed {
				p.sb.WriteByte('[')
				p.expr(e.Key, 0)
				p.sb.WriteByte(']')
			} else {
				p.expr(e.Key, precPrimary)
			}
			if e.Initializer != nil {
				p.space()
				p.sb.WriteByte('=')
				p.space()
				p.expr(e.Initializer, precAssign)
			}
			p.sep()
		case *ast.ClassStaticBlock:
			p.sb.WriteString("static")
			p.space()
			if e.Block != nil {
				p.block(e.Block)
			}
		}
	}
	p.level--
	p.lineBreak()
	p.sb.WriteByte('}')
}

// --- operator tables ------------------------------------------------------

func binPrec(op token.Token) int {
	switch op {
	case token.LOGICAL_OR:
		return precLogicalOr
	case token.LOGICAL_AND:
		return precLogicalAnd
	case token.COALESCE:
		return precCoalesce
	case token.OR:
		return precBitOr
	case token.EXCLUSIVE_OR:
		return precBitXor
	case token.AND:
		return precBitAnd
	case token.EQUAL, token.STRICT_EQUAL, token.NOT_EQUAL, token.STRICT_NOT_EQUAL:
		return precEquality
	case token.LESS, token.GREATER, token.LESS_OR_EQUAL, token.GREATER_OR_EQUAL,
		token.IN, token.INSTANCEOF:
		return precRelational
	case token.SHIFT_LEFT, token.SHIFT_RIGHT, token.UNSIGNED_SHIFT_RIGHT:
		return precShift
	case token.PLUS, token.MINUS:
		return precAdd
	case token.MULTIPLY, token.SLASH, token.REMAINDER:
		return precMul
	case token.EXPONENT:
		return precExp
	}
	return precPrimary
}

func binOpStr(op token.Token) string {
	switch op {
	case token.PLUS:
		return "+"
	case token.MINUS:
		return "-"
	case token.MULTIPLY:
		return "*"
	case token.SLASH:
		return "/"
	case token.REMAINDER:
		return "%"
	case token.EXPONENT:
		return "**"
	case token.EQUAL:
		return "=="
	case token.STRICT_EQUAL:
		return "==="
	case token.NOT_EQUAL:
		return "!="
	case token.STRICT_NOT_EQUAL:
		return "!=="
	case token.LESS:
		return "<"
	case token.GREATER:
		return ">"
	case token.LESS_OR_EQUAL:
		return "<="
	case token.GREATER_OR_EQUAL:
		return ">="
	case token.LOGICAL_AND:
		return "&&"
	case token.LOGICAL_OR:
		return "||"
	case token.COALESCE:
		return "??"
	case token.AND:
		return "&"
	case token.OR:
		return "|"
	case token.EXCLUSIVE_OR:
		return "^"
	case token.SHIFT_LEFT:
		return "<<"
	case token.SHIFT_RIGHT:
		return ">>"
	case token.UNSIGNED_SHIFT_RIGHT:
		return ">>>"
	case token.IN:
		return "in"
	case token.INSTANCEOF:
		return "instanceof"
	}
	return "?"
}

func assignOpStr(op token.Token) string {
	switch op {
	case token.ASSIGN:
		return "="
	case token.ADD_ASSIGN:
		return "+="
	case token.SUBTRACT_ASSIGN:
		return "-="
	case token.MULTIPLY_ASSIGN:
		return "*="
	case token.QUOTIENT_ASSIGN:
		return "/="
	case token.REMAINDER_ASSIGN:
		return "%="
	case token.EXPONENT_ASSIGN:
		return "**="
	case token.LOGICAL_AND_ASSIGN:
		return "&&="
	case token.LOGICAL_OR_ASSIGN:
		return "||="
	case token.COALESCE_ASSIGN:
		return "??="
	case token.SHIFT_LEFT_ASSIGN:
		return "<<="
	case token.SHIFT_RIGHT_ASSIGN:
		return ">>="
	case token.UNSIGNED_SHIFT_RIGHT_ASSIGN:
		return ">>>="
	case token.AND_ASSIGN:
		return "&="
	case token.OR_ASSIGN:
		return "|="
	case token.EXCLUSIVE_OR_ASSIGN:
		return "^="
	}
	return "="
}

func unaryOpStr(op token.Token) string {
	switch op {
	case token.NOT:
		return "!"
	case token.BITWISE_NOT:
		return "~"
	case token.PLUS:
		return "+"
	case token.MINUS:
		return "-"
	case token.TYPEOF:
		return "typeof"
	case token.VOID:
		return "void"
	case token.DELETE:
		return "delete"
	case token.INCREMENT:
		return "++"
	case token.DECREMENT:
		return "--"
	}
	return ""
}

// --- string builder helpers ----------------------------------------------

// String helpers for unistring.
func usString(s unistring.String) string { return s.String() }

// quotedString produces a double-quoted JS string literal for s, escaping
// control characters and special quotes. Used by transforms that build new
// string literals from decoded values.
func quotedString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		case 0:
			b.WriteString("\\0")
		default:
			if r < 0x20 {
				b.WriteString("\\u")
				h := strconv.FormatInt(int64(r), 16)
				for len(h) < 4 {
					h = "0" + h
				}
				b.WriteString(h)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// QuoteString returns a JS string literal for s, to be assigned to a
// StringLiteral.Literal field.
func QuoteString(s string) string { return quotedString(s) }

// fixNumberDotAccess wraps number-dot-identifier patterns in parens to avoid
// syntax errors: "1024.toFixed" → "(1024).toFixed".
func fixNumberDotAccess(s string) string {
	// Quick check: if no digit followed by '.', skip.
	hasPattern := false
	for i := 0; i < len(s)-1; i++ {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] == '.' {
			hasPattern = true
			break
		}
	}
	if !hasPattern {
		return s
	}
	var out []byte
	i := 0
	for i < len(s) {
		// Find a number literal (digits, possibly with 0x prefix or decimal point).
		if (s[i] >= '0' && s[i] <= '9') || (s[i] == '0' && i+1 < len(s) && (s[i+1] == 'x' || s[i+1] == 'X')) {
			start := i
			// Consume the number: 0x hex, or decimal digits with optional fractional/exponent.
			if s[i] == '0' && i+1 < len(s) && (s[i+1] == 'x' || s[i+1] == 'X') {
				i += 2
				for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || (s[i] >= 'a' && s[i] <= 'f') || (s[i] >= 'A' && s[i] <= 'F')) {
					i++
				}
			} else {
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
			}
			// Check if followed by .identifier (but not a decimal point).
			// A decimal point followed by a digit is part of the number;
			// a dot followed by an identifier letter is member access.
			if i < len(s) && s[i] == '.' && i+1 < len(s) && ((s[i+1] >= 'a' && s[i+1] <= 'z') || (s[i+1] >= 'A' && s[i+1] <= 'Z') || s[i+1] == '_') {
				out = append(out, '(')
				out = append(out, s[start:i]...)
				out = append(out, ')')
				continue
			}
			out = append(out, s[start:i]...)
			continue
		}
		out = append(out, s[i])
		i++
	}
	return string(out)
}
