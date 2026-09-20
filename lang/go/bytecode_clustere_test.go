package lang

import (
	"fmt"
	"testing"
)

// TestFnValueApplyInBody pins the fn-body part of cluster E (broad miscompile
// hunt): applying a runtime Function VALUE inside a fn body — `(fnv 100)` where
// fnv is a Function-typed param — was not lowered to a dynamic apply
// (resolveDynamicApply runs only for the MAIN program residual, not fn bodies). The
// compiler pushed [fnv, 100] WITHOUT applying, so RET saw 2 values and raised
// "expected 1 return value, got 2" where the interpreter applies fnv → 5. The fn
// finish now declines a user-fn count mismatch whose residual carries a Function/
// FnDef value → fall back → compile==interpret.
//
// The remaining cluster-E cases (a /v-deferred map field auto-invoked on `.field`,
// and a nested-factory apply in the MAIN residual) are separate resolveDynamicApply
// gaps — see design/legacy/MISCOMPILE-HUNT-FINDINGS.0.ignore.
func TestFnValueApplyInBody(t *testing.T) {
	apply := []struct{ name, src string }{
		{"fn-param apply", `def apply1 fn [[fnv:Function][Integer][(fnv 100)]] (apply1 ([y:Integer] => [5]))`},
		{"fn-param apply 2-arg", `def ap2 fn [[f:Function n:Integer][Integer][(f n)]] (ap2 ([y:Integer]=>[y add 1]) 10)`},
		// A factory whose returned closure is def-bound then applied — the
		// def-bound-closure apply is a separate resolveDynamicApply gap, so it falls
		// back; pin only that it MATCHES the interpreter (not native compilation).
		{"factory def-bound apply", `def mk fn [[x:Integer][Function][([y:Integer]=>[x add y])]] def add5 (mk 5) (add5 10)`},
	}
	for _, c := range apply {
		t.Run("apply/"+c.name, func(t *testing.T) {
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

	// NEGATIVE: my change declines only a COUNT-MISMATCH residual carrying a Function;
	// a plain fn (no fn-value) and a fn whose body's Function count MATCHES its
	// declared [Function] return must STILL compile natively (RunCompiledStrict).
	compiles := []struct{ name, src, want string }{
		{"plain fn no fn-value", `def g fn [[n:Integer][Integer][n add 1]] (g 5)`, "[6]"},
		{"fn computing with its result (no leaked fn-value)", `def twice fn [[n:Integer][Integer][n mul 2]] (twice (twice 3))`, "[12]"},
	}
	for _, c := range compiles {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("must compile natively, error: %v", err)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
