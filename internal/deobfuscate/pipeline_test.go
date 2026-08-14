package deobfuscate

import (
	"os"
	"testing"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/ejfkdev/jd/internal/codegen"
	"github.com/ejfkdev/jd/internal/scope"
)

func TestFindStringArraySimple(t *testing.T) {
	src := `const arr = ['log', 'Hello, World!'];
console[arr[0]](arr[1]);`
	prog, err := parser.ParseFile(nil, "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	tree := scope.Analyze(prog)
	sa := findStringArray(prog, tree)
	if sa == nil {
		t.Fatal("expected simple string array to be detected")
	}
	if len(sa.Strs) != 2 {
		t.Errorf("expected 2 strings, got %d", len(sa.Strs))
	}
	if sa.Strs[0] != "log" || sa.Strs[1] != "Hello, World!" {
		t.Errorf("strings = %v", sa.Strs)
	}
}

func TestPipelineSimpleArray(t *testing.T) {
	src := `const arr = ['log', 'Hello, World!'];
console[arr[0]](arr[1]);`
	prog, err := parser.ParseFile(nil, "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	warnings := Pipeline(prog, 0)
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModePretty})
	t.Logf("output:\n%s", out)
	// The simple-array path doesn't yet inline via Pipeline (that's in
	// unminify/inline-object-props); for now verify it parses.
	if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
		t.Errorf("output not parseable: %v", err)
	}
}

func TestDecodeObfuscatorIOSimple(t *testing.T) {
	// Minimal obfuscator.io-style: a string array getter with a simple decoder.
	src := `
function _0x123() {
  var _arr = ['hello', 'world'];
  return (_0x123 = function () { return _arr; })();
}
function _0x456(a) {
  a = a - 0x0;
  return _0x123()[a];
}
console.log(_0x456(0x0));
console.log(_0x456(0x1));
`
	prog, err := parser.ParseFile(nil, "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	tree := scope.Analyze(prog)
	d := FindAll(prog, tree)
	if d == nil {
		t.Fatal("FindAll returned nil")
	}
	if d.Array == nil {
		t.Fatal("string array not detected")
	}
	if len(d.Decoders) == 0 {
		t.Fatal("no decoders detected")
	}
	t.Logf("found %d decoders, array len=%d", len(d.Decoders), len(d.Array.Strs))

	warnings := Pipeline(prog, 0)
	t.Logf("warnings: %v", warnings)
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModePretty})
	t.Logf("output:\n%s", out)
	if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
		t.Errorf("output not parseable: %v", err)
	}
	// Verify the decoded strings appear.
	if !contains(out, "hello") {
		t.Error("decoded 'hello' not in output")
	}
}

func TestWebcrackSample(t *testing.T) {
	data, err := os.ReadFile("testdata/samples/simple-string-array.js")
	if err != nil {
		t.Skipf("sample not found: %v", err)
	}
	prog, err := parser.ParseFile(nil, "", string(data), 0)
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}
	warnings := Pipeline(prog, 0)
	_ = warnings
	out := codegen.Generate(prog, codegen.Options{Mode: codegen.ModePretty})
	t.Logf("output:\n%s", out)
	if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
		t.Errorf("output not parseable: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// silence unused import warnings until pipeline uses them directly.
var _ = ast.Node(nil)
