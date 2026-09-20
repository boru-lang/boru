package lang

import (
	"fmt"
	"testing"
)

// TestMultiOverloadGradualArg pins cluster C (broad miscompile hunt): a gradual-Any
// arg (static type exactly Any, e.g. laundered through an [Any] fn) to a
// MULTI-overload user fn is an ambiguous dispatch. The checker committed to one
// overload, whose CALL_USER param guard then RAISED at runtime when the value
// matched a SIBLING overload — but the interpreter runtime-re-matches and
// dispatches the sibling (`(g (id 5))` → 'i' interpreted, signature_error
// compiled). User-fn overloads have no OpCallNativePoly re-match. Now declined →
// fall back → compile==interpret.
func TestMultiOverloadGradualArg(t *testing.T) {
	gradual := []struct{ name, src string }{
		{"uniform-return laundered Integer",
			`def id fn [[x:Any][Any][x]] def g fn [[a:Integer][String]['i'] [a:String][String]['s']] (g (id 5))`},
		{"laundered via List get",
			`def head1 fn [[xs:List][Any][xs get 0]] def g fn [[a:Integer][String]['i'] [a:String][String]['s']] (g (head1 [5]))`},
		{"getr-Any to multi-overload",
			`def f fn [[s:String] [Integer] [1] [n:Integer] [Integer] [2]] (f ({b:2} getr 'b'))`},
	}
	for _, c := range gradual {
		t.Run("gradual/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src) // allows fallback
			if err != nil {
				t.Fatalf("RunCompiled error: %v", err)
			}
			b, _ := New()
			want, werr := b.RunInterp(c.src)
			if werr != nil {
				t.Fatalf("interpreter error: %v", werr)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}

	// NEGATIVE: a SINGLE-overload gradual call (the param guard handles it) and a
	// MULTI-overload call with a CONCRETE arg (not Dynamic → unambiguous) must STILL
	// compile natively. Pins that the cluster-C compile failure did not over-fire.
	compiles := []struct{ name, src, want string }{
		{"single-overload gradual match", `def id fn [[x:Any][Any][x]] def f fn [[n:Integer] [Integer] [n add 1]] f (id 41)`, "[42]"},
		{"multi-overload concrete Integer", `def g fn [[a:Integer][String]['i'] [a:String][String]['s']] (g 5)`, "[i]"},
		{"multi-overload concrete String", `def g fn [[a:Integer][String]['i'] [a:String][String]['s']] (g "x")`, "[s]"},
	}
	for _, c := range compiles {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("must compile natively, error: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) || fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s (interp %v)", got, c.want, want)
			}
		})
	}
}
