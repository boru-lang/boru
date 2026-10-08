package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestEachFoldGradualCollection pins cluster D (broad miscompile hunt): a
// higher-order word (each/fold/scan/...) over a gradual-Any collection — a value
// of static type exactly Any, e.g. laundered through an [Any]-returning fn — has
// an AMBIGUOUS List-vs-Map overload the checker can't statically commit and a
// code-body word can't poly-re-match. It used to bake the Map overload and error
// "expected concrete map" at runtime where the interpreter iterates the runtime
// List (a compile==interpret VIOLATION). Now tryRecordClosure declines
// (MarkUncompilable) → the program falls back → compile==interpret.
func TestEachFoldGradualCollection(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	gradual := []struct{ name, src string }{
		{"each over gradual list", `def mk fn [[][Any][[1 2 3]]] (each [mul 2] (mk))`},
		{"fold over gradual list", `def id fn [[x:Any][Any][x]] (fold [add] (id [1 2 3]) 0)`},
		{"each over gradual (1 elem)", `def id fn [[x:Any][Any][x]] (each [add 1] (id [1]))`},
		{"scan over gradual list", `def id fn [[x:Any][Any][x]] (scan [add] (id [1 2 3]) 0)`},
		{"for-each over gradual list", `def id fn [[x:Any][Any][x]] (for-each [add 1] (id [1 2]))`},
	}
	for _, c := range gradual {
		t.Run("gradual/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, err := a.RunCompiled(c.src)
			if noteCompileDefect(t, c.src, got, err) {
				return
			}
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

	// NEGATIVE: a CONCRETE collection is not Dynamic, so the guard does not fire —
	// these must STILL compile natively (RunCompiledStrict forces full compilation)
	// and match the interpreter. Pins that the compile failure did not over-fire.
	concrete := []struct{ name, src string }{
		{"each concrete list", `def xs [1 2 3] (each [mul 2] xs)`},
		{"fold concrete list", `(fold [add] [1 2 3] 0)`},
		{"each list-of-maps lambda", `def people [{age:30} {age:40}] (each [([p:Any] => [p.age]) apply] people)`},
		{"filter concrete list", `def xs [1 2 3] (filter [gt 1] xs)`},
	}
	for _, c := range concrete {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("concrete higher-order must compile, declined: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("concrete %s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}
}
