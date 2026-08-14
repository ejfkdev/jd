// Package deobfuscate implements the obfuscator.io deobfuscation pipeline:
// string-array detection, array-rotator detection, decoder detection, wrapper
// inlining, hybrid (static + sandbox) decoding, control-flow flattening,
// dead-code removal, and self-defending/debug-protection removal.
package deobfuscate

import (
	"github.com/dop251/goja/ast"
	"github.com/ejfkdev/jd/internal/scope"
)

// StringArray records the detected obfuscator.io string array getter.
type StringArray struct {
	Node         ast.Node        // the FunctionDeclaration or VariableStatement
	Name         string          // current (renamed) binding name
	OriginalName string          // original name before renaming
	Strs         []string        // the literal strings in the array
	ArrIdent     *ast.Identifier // the local array identifier inside the getter
}

// Rotator records the array-rotation IIFE (push/shift loop).
type Rotator struct {
	Stmt      ast.Statement   // the ExpressionStatement wrapping the IIFE
	ArrayFn   *ast.Identifier // the string-array function name
	BreakCond float64         // the break condition value (if statically known)
}

// Decoder records a decoder function that calls the string array.
type Decoder struct {
	Node     ast.Node // the FunctionDeclaration or VariableStatement
	Name     string   // current binding name
	Offset   int      // index offset (e.g. f -= 0xce → +0xce)
	Enc      int      // encoding: 0=simple, 1=base64, 2=rc4, 3=unknown
	Charset  string   // base64/rc4 charset (if detected)
	IndexArg int      // which param position is the index
	KeyArg   int      // which param position is the key (rc4)
}

// Wrapper records an alias of a decoder (var or function wrapper).
type Wrapper struct {
	Name      string   // alias name
	Target    string   // target decoder name
	AddOffset int      // additional offset on the index
	IndexArg  int      // which wrapper param feeds target's index slot
	KeyArg    int      // which wrapper param feeds target's key slot
	Node      ast.Node // the declaration node (for sandbox setup)
}

// Detection aggregates everything found in one scan.
type Detection struct {
	Array    *StringArray
	Rotator  *Rotator
	Decoders []*Decoder
	Wrappers []Wrapper
	Tree     *scope.Tree
}

// FindAll scans prog for the string array, rotator and decoders. Returns nil
// when no string array is found (the gate that determines whether deobfuscation
// runs at all, mirroring webcrack).
func FindAll(prog *ast.Program, tree *scope.Tree) *Detection {
	d := &Detection{Tree: tree}
	d.Array = findStringArray(prog, tree)
	if d.Array == nil {
		return nil
	}
	d.Rotator = findRotator(prog, tree, d.Array)
	d.Decoders = findDecoders(prog, tree, d.Array)
	resolveWrappers(prog, tree, d)
	return d
}
