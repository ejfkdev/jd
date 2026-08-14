// Package unminify applies readability-restoring AST transforms (the inverse
// of minification). Ported from webcrack's src/unminify/transforms.
package unminify

import (
	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/transform"
)

// Pipeline runs the unminify transforms to fixpoint on prog. Returns warnings
// (currently always nil; reserved for future transforms that can fail).
func Pipeline(prog *ast.Program) []string {
	ctx := &transform.Context{Prog: prog}
	transforms := allTransforms()
	transform.ApplyFixpoint(ctx, transforms, 20)
	return nil
}

func allTransforms() []transform.Transform {
	return []transform.Transform{
		// Prepare (normalization).
		&splitVariableDeclarations{},
		&sequence{},
		&blockStatements{},
		// Readability.
		&computedProperties{},
		&mergeStrings{},
		&unminifyBooleans{},
		&numberExpressions{},
		&voidToUndefined{},
		&rawLiterals{},
		&removeDoubleNot{},
		&logicalToIf{},
		&ternaryToIf{},
		&mergeElseIf{},
		&forToWhile{},
		&yoda{},
		&typeofUndefined{},
		&infinity{},
		&invertBooleanLogic{},
		&unaryExpressions{},
		&splitForLoopVars{},
	}
}
