package codegen

import (
	"strings"
	"testing"

	"github.com/dop251/goja/parser"
)

func TestRoundTripPretty(t *testing.T) {
	cases := []string{
		`var a = 1;`,
		`var a = 1, b = 2;`,
		`function f(x) { return x + 1; }`,
		`var f = function(a, b) { return a - b; };`,
		`if (a) { b(); } else { c(); }`,
		`while (true) { break; }`,
		`for (var i = 0; i < 10; i++) { sum += i; }`,
		`for (var k in obj) { print(k); }`,
		`for (var v of arr) { print(v); }`,
		`switch (x) { case 1: a(); break; case 2: b(); break; default: c(); }`,
		`try { f(); } catch (e) { g(e); } finally { h(); }`,
		`var obj = { a: 1, b: "x", "c-d": 2, 0: 3 };`,
		`var arr = [1, 2, [3, 4], { x: 5 }];`,
		`a.b.c["d"].e();`,
		`new Foo(1, 2).bar();`,
		`x = a ? b : c;`,
		`a = b = c = 0;`,
		`f(g(h()));`,
		`x && y || z;`,
		`!a && ~b + +c;`,
		`(a, b, c);`,
		`x["y"][0].z;`,
		`var f = (a) => a + 1;`,
		`var f = (a, b) => { return a + b; };`,
		`label: for (;;) { break label; }`,
		`throw new Error("oops");`,
		`do { x(); } while (y);`,
		`typeof a === "string";`,
		`a?.b?.c;`,
		"`tag${x}tail`;",
		`class C extends B { method() { super.method(); } static get x() { return 1; } field = 2; }`,
		`var f = async (x) => await g(x);`,
	}
	for _, src := range cases {
		prog, err := parser.ParseFile(nil, "", src, 0)
		if err != nil {
			t.Errorf("parse %q: %v", src, err)
			continue
		}
		out := Generate(prog, Options{Mode: ModePretty})
		// The output must be re-parseable.
		if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
			t.Errorf("round-trip parse failed for %q\noutput:\n%s\nerr: %v", src, out, err)
		}
	}
}

func TestCompactMode(t *testing.T) {
	src := `var a = 1;
var b = "hello";
if (a) {
  f(a, b);
}`
	prog, err := parser.ParseFile(nil, "", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := Generate(prog, Options{Mode: ModeCompact})
	if contains(out, "\n") {
		t.Errorf("compact output contains newlines: %q", out)
	}
	if _, err := parser.ParseFile(nil, "", out, 0); err != nil {
		t.Errorf("compact round-trip failed: %v\noutput: %q", err, out)
	}
}

func TestQuoteString(t *testing.T) {
	cases := map[string]string{
		"hello":       `"hello"`,
		`with"quote`:  `"with\"quote"`,
		"line\nbreak": `"line\nbreak"`,
		"tab\tchar":   `"tab\tchar"`,
		`back\slash`:  `"back\\slash"`,
	}
	for in, want := range cases {
		if got := QuoteString(in); got != want {
			t.Errorf("QuoteString(%q) = %q, want %q", in, got, want)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
