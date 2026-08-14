package deobfuscate

import (
	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/jsast"
	"github.com/ejfkdev/jd/internal/scope"
)

// resolveWrappers finds var aliases (var k = decoder) and function wrappers
// (function w(a,b){ return decoder(b-938, a) }) of known decoders, and adds
// them to the Detection so DecodeAll can resolve k(...) calls too.
//
// This mirrors webcrack's inline-decoder-wrappers.ts (variable aliases +
// function aliases), but instead of inlining it records the alias names so
// collectCallSites can match them.
func resolveWrappers(prog *ast.Program, tree *scope.Tree, d *Detection) {
	if d == nil || len(d.Decoders) == 0 {
		return
	}
	decoderNames := make(map[string]bool)
	for _, dec := range d.Decoders {
		decoderNames[dec.Name] = true
	}
	// Fixpoint: resolve var aliases, then function wrappers, repeat until no
	// new aliases are found.
	for changed := true; changed; {
		changed = resolveVarAliases(prog, tree, d, decoderNames)
	}
}

// resolveVarAliases finds `var alias = knownDecoder` and records alias as a
// zero-offset wrapper.
func resolveVarAliases(prog *ast.Program, tree *scope.Tree, d *Detection, decoderNames map[string]bool) bool {
	changed := false
	jsast.Walk(prog, jsast.Visit{Enter: func(c *jsast.Cursor) jsast.Action {
		// Support var, const, and let declarations.
		var list []*ast.Binding
		switch t := c.Node.(type) {
		case *ast.VariableStatement:
			list = t.List
		case *ast.LexicalDeclaration:
			list = t.List
		default:
			return jsast.VisitKeep
		}
		for _, b := range list {
			id, ok := b.Target.(*ast.Identifier)
			if !ok {
				continue
			}
			if decoderNames[id.Name.String()] {
				continue
			}
			if initID, ok := b.Initializer.(*ast.Identifier); ok {
				if decoderNames[initID.Name.String()] {
					d.Wrappers = append(d.Wrappers, Wrapper{
						Name:      id.Name.String(),
						Target:    initID.Name.String(),
						AddOffset: 0,
						IndexArg:  0,
						KeyArg:    0,
						Node:      c.Node,
					})
					decoderNames[id.Name.String()] = true
					changed = true
				}
			}
		}
		return jsast.VisitKeep
	}})
	return changed
}
