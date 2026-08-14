// Package scope provides minimal lexical scope and binding analysis over a
// goja AST. It is sufficient for the safety checks the deobfuscator needs:
// constant bindings, reference counts, and read-only-member-access detection.
//
// Semantics are simplified vs. eslint-scope / babel's binding analysis: the
// tree is rebuilt from scratch on each call (O(n), acceptable for obfuscated
// files ≤ a few MB). var and function declarations hoist to the nearest
// function/program scope; let/const bind in the enclosing block scope.
package scope

import (
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/jsast"
)

// BindingKind classifies how a binding was declared.
type BindingKind int

const (
	KindVar BindingKind = iota
	KindLet
	KindConst
	KindParam
	KindFunction // function declaration name
	KindCatch    // catch parameter
)

// Binding records a declaration and all its references.
type Binding struct {
	Name       string
	Kind       BindingKind
	Decl       ast.Node
	Scope      *Scope
	Refs       []*Reference
	Violations []*Reference // writes: assignment LHS, update operand, delete
}

// IsConstant reports whether a binding has no write references (babel
// "constant" semantics used by webcrack's safety checks).
func (b *Binding) IsConstant() bool { return len(b.Violations) == 0 }

// RefCount returns the total number of references.
func (b *Binding) RefCount() int { return len(b.Refs) + len(b.Violations) }

// Reference is one use of a binding.
type Reference struct {
	Ident  *ast.Identifier
	Scope  *Scope
	Parent ast.Node
}

// Kind classifies a scope.
type ScopeKind int

const (
	ScopeProgram ScopeKind = iota
	ScopeFunction
	ScopeBlock
	ScopeCatch
)

// Scope is a lexical scope.
type Scope struct {
	Kind     ScopeKind
	Node     ast.Node
	Parent   *Scope
	Bindings map[string]*Binding
}

// Tree is the result of Analyze: the root scope plus a node→scope map.
type Tree struct {
	Root   *Scope
	byNode map[ast.Node]*Scope
}

// ScopeOf returns the scope that contains n, or nil.
func (t *Tree) ScopeOf(n ast.Node) *Scope { return t.byNode[n] }

// GetBinding resolves name from s upward (var/function stop at block scopes).
func (t *Tree) GetBinding(s *Scope, name string) *Binding {
	for cur := s; cur != nil; cur = cur.Parent {
		if b, ok := cur.Bindings[name]; ok {
			return b
		}
	}
	return nil
}

// HasBinding reports whether name is bound in s (without walking up).
func (t *Tree) HasBinding(s *Scope, name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.Bindings[name]
	return ok
}

// GenerateUID returns a name not bound in s or any parent, based on base with a
// numeric suffix.
func (t *Tree) GenerateUID(s *Scope, base string) string {
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = base + itoa(i)
		}
		if t.GetBinding(s, name) == nil {
			return name
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Analyze builds the scope tree for prog.
func Analyze(prog *ast.Program) *Tree {
	if prog == nil {
		return &Tree{byNode: map[ast.Node]*Scope{}}
	}
	t := &Tree{byNode: map[ast.Node]*Scope{}}
	root := &Scope{Kind: ScopeProgram, Node: prog, Bindings: map[string]*Binding{}}
	t.Root = root
	t.byNode[prog] = root
	c := &collector{tree: t, stack: []*Scope{root}}
	jsast.Walk(prog, jsast.Visit{Enter: c.enter, Exit: c.exit})
	return t
}

type collector struct {
	tree  *Tree
	stack []*Scope
}

func (c *collector) cur() *Scope { return c.stack[len(c.stack)-1] }

func (c *collector) push(s *Scope) {
	c.stack = append(c.stack, s)
	c.tree.byNode[s.Node] = s
}

func (c *collector) pop() { c.stack = c.stack[:len(c.stack)-1] }

func (c *collector) enter(cur *jsast.Cursor) jsast.Action {
	n := cur.Node
	switch t := n.(type) {
	case *ast.FunctionLiteral:
		fnScope := &Scope{Kind: ScopeFunction, Node: n, Parent: c.cur(), Bindings: map[string]*Binding{}}
		c.push(fnScope)
		// Function name (for named function expressions) binds in the function's
		// own scope. FunctionDeclaration names are hoisted by the caller.
		if t.Name != nil {
			name := t.Name.Name.String()
			fnScope.Bindings[name] = &Binding{Name: name, Kind: KindFunction, Decl: t.Name, Scope: fnScope}
		}
		// Parameters.
		if t.ParameterList != nil {
			for _, b := range t.ParameterList.List {
				c.bindPattern(fnScope, b.Target, KindParam, b)
			}
			if t.ParameterList.Rest != nil {
				if id, ok := t.ParameterList.Rest.(*ast.Identifier); ok {
					fnScope.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: KindParam, Decl: id, Scope: fnScope}
				}
			}
		}
		// Hoist var declarations and function declarations in this function body.
		c.hoistVars(fnScope, t.Body, t.DeclarationList)
	case *ast.ArrowFunctionLiteral:
		fnScope := &Scope{Kind: ScopeFunction, Node: n, Parent: c.cur(), Bindings: map[string]*Binding{}}
		c.push(fnScope)
		if t.ParameterList != nil {
			for _, b := range t.ParameterList.List {
				c.bindPattern(fnScope, b.Target, KindParam, b)
			}
			if t.ParameterList.Rest != nil {
				if id, ok := t.ParameterList.Rest.(*ast.Identifier); ok {
					fnScope.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: KindParam, Decl: id, Scope: fnScope}
				}
			}
		}
	case *ast.BlockStatement:
		blkScope := &Scope{Kind: ScopeBlock, Node: n, Parent: c.cur(), Bindings: map[string]*Binding{}}
		c.push(blkScope)
	case *ast.CatchStatement:
		catchScope := &Scope{Kind: ScopeCatch, Node: n, Parent: c.cur(), Bindings: map[string]*Binding{}}
		c.push(catchScope)
		if t.Parameter != nil {
			c.bindPattern(catchScope, t.Parameter, KindCatch, nil)
		}
	case *ast.VariableStatement:
		c.declareBindings(c.cur(), t.List, KindVar)
	case *ast.LexicalDeclaration:
		k := KindLet
		if t.Token == token.CONST {
			k = KindConst
		}
		c.declareBindings(c.cur(), t.List, k)
	case *ast.VariableDeclaration:
		// VariableDeclaration appears in for-init and function DeclarationList;
		// treat as var.
		c.declareBindings(c.cur(), t.List, KindVar)
	case *ast.FunctionDeclaration:
		// Hoisted to enclosing function/program scope; declare there.
		if t.Function != nil && t.Function.Name != nil {
			name := t.Function.Name.Name.String()
			s := c.functionScope()
			if s.Bindings[name] == nil {
				s.Bindings[name] = &Binding{Name: name, Kind: KindFunction, Decl: n, Scope: s}
			}
		}
	case *ast.Identifier:
		// Reference (read) unless it's a declaration site.
		if c.isDeclSite(cur) {
			return jsast.VisitKeep
		}
		name := t.Name.String()
		b := c.tree.GetBinding(c.cur(), name)
		if b != nil {
			b.Refs = append(b.Refs, &Reference{Ident: t, Scope: c.cur(), Parent: cur.Parent})
		}
	case *ast.AssignExpression:
		// Record violations for identifier LHS.
		if id, ok := t.Left.(*ast.Identifier); ok && t.Operator == token.ASSIGN {
			c.recordViolation(id, cur)
		}
	case *ast.UnaryExpression:
		if (t.Operator == token.INCREMENT || t.Operator == token.DECREMENT) && !t.Postfix {
			if id, ok := t.Operand.(*ast.Identifier); ok {
				c.recordViolation(id, cur)
			}
		}
	}
	return jsast.VisitKeep
}

func (c *collector) exit(cur *jsast.Cursor) {
	switch cur.Node.(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral, *ast.BlockStatement, *ast.CatchStatement:
		c.pop()
	}
}

func (c *collector) functionScope() *Scope {
	for i := len(c.stack) - 1; i >= 0; i-- {
		if c.stack[i].Kind == ScopeFunction || c.stack[i].Kind == ScopeProgram {
			return c.stack[i]
		}
	}
	return c.cur()
}

func (c *collector) isDeclSite(cur *jsast.Cursor) bool {
	// An identifier is a declaration site when its parent is a Binding (target),
	// a FunctionLiteral name, a CatchStatement parameter, or a PropertyKeyed key.
	switch cur.Parent.(type) {
	case *ast.Binding, *ast.FunctionLiteral, *ast.CatchStatement:
		return true
	}
	// Property key in non-computed property.
	if pk, ok := cur.Parent.(*ast.PropertyKeyed); ok && !pk.Computed {
		if pk.Key == cur.Node {
			return true
		}
	}
	return false
}

func (c *collector) recordViolation(id *ast.Identifier, cur *jsast.Cursor) {
	name := id.Name.String()
	b := c.tree.GetBinding(c.cur(), name)
	if b != nil {
		b.Violations = append(b.Violations, &Reference{Ident: id, Scope: c.cur(), Parent: cur.Parent})
	}
}

func (c *collector) declareBindings(s *Scope, list []*ast.Binding, kind BindingKind) {
	// var declarations bind to the enclosing function/program scope (hoisting),
	// not the block scope.
	if kind == KindVar {
		s = c.functionScope()
	}
	for _, b := range list {
		if b.Target == nil {
			continue
		}
		c.bindPattern(s, b.Target, kind, b)
	}
}

func (c *collector) bindPattern(s *Scope, target ast.BindingTarget, kind BindingKind, decl *ast.Binding) {
	switch t := target.(type) {
	case *ast.Identifier:
		name := t.Name.String()
		if kind == KindVar {
			// var redeclaration: keep first binding (hoisted).
			if _, ok := s.Bindings[name]; ok {
				return
			}
		}
		s.Bindings[name] = &Binding{Name: name, Kind: kind, Decl: decl, Scope: s}
	case *ast.ArrayPattern:
		for _, el := range t.Elements {
			if id, ok := el.(*ast.Identifier); ok {
				s.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: kind, Decl: nil, Scope: s}
			}
		}
	case *ast.ObjectPattern:
		for _, prop := range t.Properties {
			if pk, ok := prop.(*ast.PropertyKeyed); ok {
				if id, ok := pk.Value.(*ast.Identifier); ok {
					s.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: kind, Decl: nil, Scope: s}
				}
			}
		}
	}
}

// hoistVars walks body for var statements and function declarations, binding
// them in fnScope. DeclarationList entries on FunctionLiteral are pre-collected
// by the parser (var declarations inside the function).
func (c *collector) hoistVars(fnScope *Scope, body *ast.BlockStatement, decls []*ast.VariableDeclaration) {
	for _, d := range decls {
		for _, b := range d.List {
			if b.Target == nil {
				continue
			}
			if id, ok := b.Target.(*ast.Identifier); ok {
				name := id.Name.String()
				if _, ok := fnScope.Bindings[name]; !ok {
					fnScope.Bindings[name] = &Binding{Name: name, Kind: KindVar, Decl: b, Scope: fnScope}
				}
			}
		}
	}
	if body != nil {
		c.hoistFromBlock(fnScope, body.List)
	}
}

// hoistFromBlock recursively collects var bindings and function declarations.
func (c *collector) hoistFromBlock(fnScope *Scope, stmts []ast.Statement) {
	for _, s := range stmts {
		switch t := s.(type) {
		case *ast.VariableStatement:
			for _, b := range t.List {
				if id, ok := b.Target.(*ast.Identifier); ok {
					if _, ok := fnScope.Bindings[id.Name.String()]; !ok {
						fnScope.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: KindVar, Decl: b, Scope: fnScope}
					}
				}
			}
		case *ast.FunctionDeclaration:
			if t.Function != nil && t.Function.Name != nil {
				name := t.Function.Name.Name.String()
				if _, ok := fnScope.Bindings[name]; !ok {
					fnScope.Bindings[name] = &Binding{Name: name, Kind: KindFunction, Decl: t, Scope: fnScope}
				}
			}
		case *ast.BlockStatement:
			c.hoistFromBlock(fnScope, t.List)
		case *ast.IfStatement:
			if b, ok := t.Consequent.(*ast.BlockStatement); ok {
				c.hoistFromBlock(fnScope, b.List)
			}
			if t.Alternate != nil {
				if b, ok := t.Alternate.(*ast.BlockStatement); ok {
					c.hoistFromBlock(fnScope, b.List)
				}
			}
		case *ast.ForStatement:
			if init, ok := t.Initializer.(*ast.ForLoopInitializerVarDeclList); ok {
				for _, b := range init.List {
					if id, ok := b.Target.(*ast.Identifier); ok {
						if _, ok := fnScope.Bindings[id.Name.String()]; !ok {
							fnScope.Bindings[id.Name.String()] = &Binding{Name: id.Name.String(), Kind: KindVar, Decl: b, Scope: fnScope}
						}
					}
				}
			}
			if b, ok := t.Body.(*ast.BlockStatement); ok {
				c.hoistFromBlock(fnScope, b.List)
			}
		case *ast.TryStatement:
			if t.Body != nil {
				c.hoistFromBlock(fnScope, t.Body.List)
			}
			if t.Finally != nil {
				c.hoistFromBlock(fnScope, t.Finally.List)
			}
		}
	}
}

// IsReadonlyMemberAccess reports whether every reference to b is a read-only
// member access (obj.key or obj["key"]) — never an assignment target, update,
// or delete. okMember filters which parent shapes count as a valid member read.
func (t *Tree) IsReadonlyMemberAccess(b *Binding, okMember func(parent ast.Node) bool) bool {
	if b == nil || !b.IsConstant() {
		return false
	}
	if len(b.Refs) == 0 {
		return false
	}
	for _, r := range b.Refs {
		if !okMember(r.Parent) {
			return false
		}
	}
	return true
}

// MemberAccessParent returns true if parent is a DotExpression or
// BracketExpression whose Left is the referenced identifier.
func MemberAccessParent(parent ast.Node) bool {
	switch p := parent.(type) {
	case *ast.DotExpression:
		_, isID := p.Left.(*ast.Identifier)
		return isID
	case *ast.BracketExpression:
		_, isID := p.Left.(*ast.Identifier)
		return isID
	}
	return false
}
