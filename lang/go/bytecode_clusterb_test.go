package lang

import (
	"fmt"
	"testing"
)

// TestTypedDefRefinement pins cluster B (broad miscompile hunt): a typed-def whose
// constraint is a REFINEMENT — a predicate/DepScalar subset (`def x:(Integer gt 10)
// v`) or a bare-refine newtype (`def x:Pos v`) — has its predicate validation and
// reparent done ONLY by the interpreter's defTypedHandler. The bytecode value-def
// lowering folds `def x:Pos n` to a bare `x≡n` alias keeping the base tag, so a
// compiled `def x:(Integer gt 10) 5` bound x=5 where the interpreter raises, and
// `def x:Pos n` reported typeof Integer / failed a [Pos] return-check. Now declined
// (no compiled store-with-reparent) → fall back → compile==interpret.
func TestTypedDefRefinement(t *testing.T) {
	refine := []struct{ name, src string }{
		{"top-level predicate bad value", `def x:(Integer gt 10) 5 x`},
		{"top-level named predicate bad", `def Big (Integer gt 10) def y:Big 5 y`},
		{"predicate valid value", `def x:(Integer gt 10) 15 x`},
		{"fn-body newtype typeof", `def Pos (refine Integer) def g fn [[n:Integer][Type][def x:Pos n (typeof x)]] (g 5)`},
		{"fn-body newtype [Pos] return", `def Pos (refine Integer) def g fn [[n:Integer][Pos][def x:Pos n x]] (g 5)`},
		{"newtype into Pos param", `def Pos (refine Integer) def need fn [[p:Pos][Integer][p]] def x:Pos (3 add 3) (need x)`},
	}
	for _, c := range refine {
		t.Run("refine/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, _, errC := a.RunCompiled(c.src) // allows fallback
			b, _ := New()
			want, errI := b.RunInterp(c.src)
			// Same outcome (both a value, or both the same error).
			if (errC == nil) != (errI == nil) {
				t.Fatalf("error mismatch: compiled err=%v, interp err=%v", errC, errI)
			}
			if errC == nil && fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
		})
	}

	// NEGATIVE: a PLAIN base-type alias (Integer/Map) is not a refinement, so it
	// must STILL compile natively (RunCompiledStrict) — the compile failure must not
	// over-fire onto every typed-def.
	plain := []struct{ name, src, want string }{
		{"Integer alias", `def x:Integer 5 x`, "[5]"},
		{"Map alias", `def m:Map {a:1} (m size)`, "[1]"},
		{"String alias", `def s:String "hi" (s size)`, "[2]"},
		// A STATIC refine-newtype value reparents via the const pool — it must STILL
		// compile (only DYNAMIC newtype values lose the reparent). Pins the narrow gate.
		{"top-level static newtype", `def Pt refine Integer def p:Pt 5 (typeof p)`, "[Pt]"},
		{"fn-body static newtype", `def Pos (refine Integer) def g fn [[][Type][def x:Pos 5 (typeof x)]] (g)`, "[Pos]"},
	}
	for _, c := range plain {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("plain typed-def must compile natively, error: %v", err)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
