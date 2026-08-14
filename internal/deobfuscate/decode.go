package deobfuscate

import (
	"time"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/token"
	"github.com/ejfkdev/jd/internal/codegen"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/sandbox"
)

// DecodeAll collects all decoder call sites, decodes them (hybrid: sandbox
// execution), and replaces them with string literals. Returns the number of
// call sites decoded and any warnings.
//
// The static path (rotation simulation + SIMPLE/BASE64/RC4) is not yet
// implemented; this version always uses the sandbox. When a call site cannot
// be decoded (sandbox error, non-literal args), it is left untouched.
func DecodeAll(prog *ast.Program, d *Detection, timeout time.Duration) (decoded int, warnings []string) {
	if d == nil || d.Array == nil || len(d.Decoders) == 0 {
		return 0, nil
	}

	// Collect call sites: CallExpression with identifier callee matching a
	// decoder name and all-literal args.
	calls := collectCallSites(prog, d)
	if len(calls) == 0 {
		return 0, nil
	}

	// Build sandbox setup: string array getter + rotator (if any) + decoders.
	setup := buildSetupCode(prog, d)
	if setup == "" {
		return 0, []string{"failed to extract decoder setup code"}
	}

	// Generate call expressions for the sandbox.
	callExprs := make([]string, 0, len(calls))
	for _, call := range calls {
		s := jsast.SliceSource(prog, call)
		if s == "" {
			s = codegen.StringOf(call)
		}
		callExprs = append(callExprs, s)
	}

	values, err := sandbox.Run(setup, callExprs, timeout)
	if err != nil {
		// Sandbox failure means the decoder setup is invalid (likely a
		// false-positive detection on non-obfuscated code). Silently skip —
		// the file is processed as-is by unminify.
		return 0, nil
	}

	// Replace each call site with a string literal.
	for i, call := range calls {
		if i >= len(values) || values[i] == nil {
			continue
		}
		s, ok := sandbox.DecodeString(values[i])
		if !ok {
			continue
		}
		lit := jsast.Str(codegen.QuoteString(s), s)
		replaceCallExpr(prog, call, lit)
		decoded++
	}
	return decoded, warnings
}

// collectCallSites walks prog and gathers all decoder calls with literal args.
func collectCallSites(prog *ast.Program, d *Detection) []*ast.CallExpression {
	// Match both decoder names and wrapper alias names.
	decoderNames := make(map[string]bool)
	for _, dec := range d.Decoders {
		decoderNames[dec.Name] = true
	}
	for _, w := range d.Wrappers {
		decoderNames[w.Name] = true
	}
	var sites []*ast.CallExpression
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		call, ok := c.Node.(*ast.CallExpression)
		if !ok {
			return jsast.VisitKeep
		}
		id, ok := call.Callee.(*ast.Identifier)
		if !ok || !decoderNames[id.Name.String()] {
			return jsast.VisitKeep
		}
		if !allLiteralArgs(call.ArgumentList) {
			return jsast.VisitKeep
		}
		sites = append(sites, call)
		return jsast.VisitSkip
	}})
	return sites
}

// allLiteralArgs reports whether every argument is a literal (string, number,
// boolean, null) or a unary-numeric expression.
func allLiteralArgs(args []ast.Expression) bool {
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		if !isLiteralArg(a) {
			return false
		}
	}
	return true
}

func isLiteralArg(e ast.Expression) bool {
	switch e.(type) {
	case *ast.StringLiteral, *ast.NumberLiteral, *ast.BooleanLiteral, *ast.NullLiteral:
		return true
	case *ast.UnaryExpression:
		// -123, +456
		u := e.(*ast.UnaryExpression)
		if u.Operator == token.MINUS || u.Operator == token.PLUS {
			if _, ok := u.Operand.(*ast.NumberLiteral); ok {
				return true
			}
		}
	}
	return false
}

// buildSetupCode extracts the string array getter, rotator and decoder functions
// as source text, sliced directly from the original source to preserve exact
// syntax (codegen can introduce subtle changes in complex decoder functions).
func buildSetupCode(prog *ast.Program, d *Detection) string {
	var parts []string
	// Order: string array → decoders → wrappers → rotator.
	if d.Array != nil && d.Array.Node != nil {
		if s := jsast.SliceSource(prog, d.Array.Node); s != "" {
			parts = append(parts, s)
		}
	}
	for _, dec := range d.Decoders {
		if dec.Node != nil {
			if s := jsast.SliceSource(prog, dec.Node); s != "" {
				parts = append(parts, s)
			}
		}
	}
	for _, w := range d.Wrappers {
		parts = append(parts, "var "+w.Name+"="+w.Target)
	}
	if d.Rotator != nil && d.Rotator.Stmt != nil {
		s := jsast.SliceSource(prog, d.Rotator.Stmt)
		if s == "" {
			s = codegen.StringOf(d.Rotator.Stmt)
		}
		if s != "" {
			// goja loses outer parens on IIFE: (function(){...})(args)
			// becomes function(){...}(args). Wrap in parens to be safe.
			if len(s) >= 8 && s[:8] == "function" {
				s = "(" + s + ")"
			}
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ";"
		}
		out += p
	}
	return out
}

// replaceCallExpr finds and replaces the given CallExpression node with lit
// in the program. It walks the tree and replaces the first match.
func replaceCallExpr(prog *ast.Program, target *ast.CallExpression, lit ast.Expression) {
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		if c.Node == target {
			c.Replace(lit)
			return jsast.VisitStop
		}
		return jsast.VisitKeep
	}})
}
