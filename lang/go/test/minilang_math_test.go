package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// The `m` mini-language evaluates a traditional maths formula whose variables
// are bound by the named params, backed by github.com/tabnas/expr. It ships
// built-in with boru:minilang.

const mImp = `import "boru:minilang"  `

// TestMiniMathEval pins the arithmetic, operator precedence/associativity, and
// the boru numeric coercion (integer domain vs float domain).
func TestMiniMathEval(t *testing.T) {
	cases := []struct {
		expr string
		want any
	}{
		{`mini math 'x*y-z^2' {x:3 y:4 z:2}`, int64(8)}, // 12 - 4
		{`mini math 'x*y-z^2' {x:3 y:4 z:2.0}`, "8.0"},  // a Float param → Float result
		{`mini math '2^3^2' {}`, int64(512)},            // right-assoc: 2^(3^2)
		{`mini math '-z^2' {z:3}`, int64(-9)},           // ^ tighter than unary -: -(9)
		{`mini math '(x+y)*z' {x:1 y:2 z:3}`, int64(9)}, // parens
		{`mini math 'x/y' {x:5 y:2}`, int64(2)},         // integer division truncates
		{`mini math 'x/y' {x:5.0 y:2}`, "2.5"},          // a Float operand → real division
		{`mini math 'x' {x:42}`, int64(42)},             // bare variable
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if got := runLast(t, a, mImp+c.expr); got != c.want {
				t.Errorf("%s => %v (%T), want %v", c.expr, got, got, c.want)
			}
		})
	}
}

// TestMiniMathDesugar proves `mini math` is sugar for the standard call.
func TestMiniMathDesugar(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	sugar := runLast(t, a, mImp+`mini math 'x*y-z^2' {x:3 y:4 z:2}`)
	desugared := runLast(t, a, mImp+`MiniLang.lang_math 'x*y-z^2' {x:3 y:4 z:2} end`)
	if sugar != desugared || sugar != int64(8) {
		t.Fatalf("sugar=%v desugared=%v must agree (8)", sugar, desugared)
	}
}

// TestMiniMathErrors pins the loud failures — and, critically, that malformed
// input is a clean error rather than the fatal stack overflow tabnas/expr's
// own evaluator hits on a degenerate AST.
func TestMiniMathErrors(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"unknown variable", `mini math 'x+q' {x:1}`, "mini_eval_error"},
		{"division by zero", `mini math 'x/y' {x:1 y:0}`, "mini_eval_error"},
		{"trailing operator", `mini math 'x +' {x:1}`, "mini_syntax_error"},
		{"unbalanced paren", `mini math '(x+y' {x:1 y:2}`, "mini_syntax_error"},
		{"non-numeric var", `mini math 'x' {x:'hi'}`, "mini_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			if _, err := a.Run(mImp + c.src); err == nil {
				t.Fatalf("%s: expected error, got nil", c.name)
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("%s: error %q should contain %q", c.name, err.Error(), c.want)
			}
		})
	}
}

// TestMiniMathInKinds confirms `m` is listed by MiniLang.kinds out of the box.
func TestMiniMathInKinds(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := runReference(t, a, mImp+`MiniLang.kinds`)
	if err != nil {
		t.Fatalf("kinds: %v", err)
	}
	if listed := fmt.Sprintf("%v", res[0]); !strings.Contains(listed, "m") {
		t.Errorf("MiniLang.kinds %v should contain m", listed)
	}
}
