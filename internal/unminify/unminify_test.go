package unminify

import (
	"strings"
	"testing"

	"github.com/dop251/goja/parser"
	"github.com/ejfkdev/jd/internal/codegen"
)

func TestMergeStrings(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `var a = "hello" + " world";`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, `"hello world"`) {
		t.Errorf("expected merged string in: %s", out)
	}
}

func TestUnminifyBooleans(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `var a = !0; var b = !1;`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "true") {
		t.Errorf("expected true in: %s", out)
	}
	if !contains(out, "false") {
		t.Errorf("expected false in: %s", out)
	}
}

func TestComputedProperties(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `console["log"]("hi");`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "console.log") {
		t.Errorf("expected console.log in: %s", out)
	}
}

func TestNumberExpressions(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `var a = 1 + 2;`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "a=3") {
		t.Errorf("expected a=3 in: %s", out)
	}
}

func TestVoidToUndefined(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `var a = void 0;`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "undefined") {
		t.Errorf("expected undefined in: %s", out)
	}
}

func TestSequenceSplit(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `a(), b(), c();`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "a();") || !contains(out, "b();") || !contains(out, "c();") {
		t.Errorf("expected split statements in: %s", out)
	}
}

func TestSplitVariableDeclarations(t *testing.T) {
	prog, _ := parser.ParseFile(nil, "", `var a = 1, b = 2;`, 0)
	Pipeline(prog)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModeCompact})
	if !contains(out, "var a=1") || !contains(out, "var b=2") {
		t.Errorf("expected split declarations in: %s", out)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
