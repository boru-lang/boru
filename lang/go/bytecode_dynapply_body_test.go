package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestClosureBodyUnappliedFnValueSound pins the fix for an off-corpus MISCOMPILE the
// langspec differential is blind to: a captured/param comparator applied as
// `(a b comp)` inside a higher-order CLOSURE body leaves the residual [a, b, comp]
// with the fn VALUE on top. The closure's top-taking handler (each/fold/scan) was
// trimming to that top and mapping every element to the UNAPPLIED comp —
// `[1 2] each [(x x comp)]` interpreted to [0 0] but COMPILED to [fn, fn]. The fix
// refuses such a closure body (sound interpreter fallback) — mirroring the fn-body
// unapplied-fn-value refusal and resolveDynamicApply's main-residual refusals — so
// compile == interpret holds. A SOLE inert fn-reference body still compiles (it maps
// every element to the reference, which is a concrete const, not an unapplied apply).
func TestClosureBodyUnappliedFnValueSound(t *testing.T) {
	// compile == interpret MUST hold (fallback allowed) for the comparator-apply
	// shapes — the exact off-corpus miscompile and its siblings across each/fold/scan.
	// The captured-comparator apply now LOWERS natively (OpCallDynTrailTop), so it
	// must compile WITHOUT a fallback island AND RunCompiledStrict == Run — the
	// off-corpus gate for the trailing-fn-value lowering's exact arg-ordering
	// (`(x 2 comp)` binds the comparator's first param to the TOP arg, like the
	// interpreter's paren). want is the program RESIDUAL (a slice), single wrapped.
	strict := []struct{ name, src, want string }{
		{"each captured comparator apply",
			`def f fn [[comp:Function][List][ [1 2] each [ var [[x] (x x comp) ] ] ]] cmp/v f`, "[[0 0]]"},
		{"fold captured comparator apply",
			`def f fn [[comp:Function][Integer][ 0 fold [ var [[acc x] (x x comp) ] ] [1 2] ]] cmp/v f`, "[0]"},
		{"scan captured comparator apply",
			`def f fn [[comp:Function][List][ scan [ var [[acc x] (acc x comp) ] ] [3 1 2] ]] cmp/v f`, "[[3 -1 1]]"},
		{"each comparator apply, asymmetric args (arg-order gate)",
			`def f fn [[comp:Function][List][ [3 1] each [ var [[x] (x 2 comp) ] ] ]] cmp/v f`, "[[1 -1]]"},
		{"fn-body comparator apply",
			`def f fn [[comp:Function a:Integer b:Integer][Integer][ (a b comp) ]] 5 3 cmp/v f`, "[-1]"},
		{"3-arg trailing apply",
			`def g fn [[f:Function][Integer][ def a 1 def b 2 def c 3 (a b c f) ]] def add3 fn [[x:Integer y:Integer z:Integer][Integer][(x add (y add z))]] add3/v g`, "[6]"},
		{"lambda (compiled-closure) comparator apply",
			`def f fn [[comp:Function][List][ [3 1] each [ var [[x] (x 2 comp) ] ] ]] ([b:Any a:Any] => [(a cmp b)]) f`, "[[1 -1]]"},
		{"trailing apply with a DYNAMIC arg (comp still the fn, not the leading dynamic)",
			`def f fn [[comp:Function][List][ def arr (flex [5 3]) [0 1] each [ var [[x] ((arr get x) 4 comp) ] ] ]] cmp/v f`, "[[1 -1]]"},
		// The comparator apply BOUND TO A DEF-LOCAL (`def c (a b comp)`) then consumed
		// by `if (c gt 0)` — an INTERMEDIATE apply, not the body's trailing residual.
		// Recorded as a RecordDynApply EVENT so it seats like any computed result; this
		// is the def-bound-dynamic-apply leaf that gates 8/11 comparison sorts.
		{"def-bound comparator apply feeding an if (2 dynamic args)",
			`def f fn [[comp:Function][List][ def arr (flex [3 1 2]) [0 1] each [ var [[i] def c ((arr get i) (arr get (i add 1)) comp) if (c gt 0) [9] [0] ] ] ]] cmp/v f`, "[[9 0]]"},
		{"def-bound comparator apply, result used twice",
			`def f fn [[comp:Function][Integer][ def c (5 3 comp) (c add c) ]] cmp/v f`, "[2]"},
		// A branch ARM whose result is an ENCLOSING-scope value-def (`[g]` / `[c]`) — the
		// value lives on the PARENT sim, unreachable from the arm's own fragment sim, so
		// planValueDefLocals must PROMOTE it (a fragResult that is not fragInternal) and
		// lowerFragment re-resolves the arm's captured opEvent out to the local. Clears
		// the comb sort + the Stage-3 "branch reads enclosing computation" cluster.
		{"branch arm result is an enclosing computed value-def",
			`def f fn [[n:Integer][Integer][ def g (n add 5) def gg (if (g lt 1) [1] [g]) gg ]] 3 f`, "[8]"},
		{"branch arm result is an enclosing dynApply value-def",
			`def f fn [[comp:Function][Integer][ def c (5 3 comp) if (c gt 0) [c] [0] ]] cmp/v f`, "[1]"},
	}
	for _, c := range strict {
		t.Run("strict/"+c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, _ := a.CompileCheck(c.src)
			if prog == nil {
				t.Fatalf("must compile natively, refused: %q", reason)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("%s must compile native (no island)", c.name)
			}
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("RunCompiledStrict: %v", err)
			}
			b, _ := New()
			want, _ := b.RunInterp(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}

	// A SOLE inert fn-reference body (mapping every element to the reference) is a
	// concrete const, NOT an unapplied apply — it must NOT be over-refused into a
	// wrong result; compile == interpret (fallback allowed).
	t.Run("sound/sole inert fn-ref body", func(t *testing.T) {
		src := `[1 2] each [cmp/v]`
		a, _ := New()
		got, _, err := a.RunCompiled(src)
		if noteCompileDefect(t, src, got, err) {
			return
		}
		b, _ := New()
		want, werr := b.RunInterp(src)
		if (err == nil) != (werr == nil) {
			t.Fatalf("error mismatch: compiled=%v interp=%v", err, werr)
		}
		if err == nil && fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("compiled %v != interpreter %v", got, want)
		}
	})
}

// The trailing-apply QUOTE discipline (probe-found off-corpus divergence,
// 2026-07-17): a compiled LOCAL push carries the stored value VERBATIM —
// including the construction-time quote of a `/v` reference — while the
// interpreter's per-read word substitution strips one quote level before the
// paren auto-apply. callDynTrailTop now strips Quoted from the applied copy
// (the op stands for a READ-substituted arrival), and RecordDynApply fences
// an INLINE-quoted fn (the interpreter keeps that paren un-collapsed).
func TestTrailingApplyQuoteDiscipline(t *testing.T) {
	// The miscompile shape: a captured `/v` comparator applied in an each
	// body compiled [[1 1]] (the still-quoted fn islanded as inert) vs the
	// interpreter's [[3 3]].
	mustCompileWithParity(t,
		`def g fn [[c:Function][List][[1 2] each [(1 2 c)]]] g (([a:Integer b:Integer]=>[a add b])/v)`,
		"[[3 3]]")
	// A quote-wrapped ARGUMENT: the param read strips one level, so it
	// applies identically.
	mustCompileWithParity(t,
		`def g fn [[c:Function][Integer][(1 2 c)]] g (quote (fn [[a:Integer b:Integer][Integer][a add b]]))`,
		"[3]")
	// An INLINE quote at the trailing position stays inert in BOTH engines
	// (no read strips it): the record fence keeps the paren un-collapsed.
	mustCompileWithParity(t,
		`(1 2 (quote (fn [[a:Integer b:Integer][Integer][a add b]])))`,
		"[1 2 fn (Integer, Integer)]")
	// An EVENT-provenance fn (a direct call result) has NO read substitution,
	// so its runtime quote state is unknowable at CHECK time: a QUOTED
	// return stays inert in the interpreter while an UNQUOTED return
	// auto-applies. CALL_DYN_TRAIL_KEEPQ decides that at RUN time and was
	// always the right op; what the refusal was really missing is the
	// callee's ARITY, without which KeepQ would consume the whole window.
	// A factory returning one anonymous lambda now yields it (the eleventh
	// increment reads it off the baked const's own signature), so both
	// polarities compile and the window trims to the callee's arity.
	mustCompileWithParity(t,
		`def choose fn [[][Function][quote (fn [[a:Integer z:Integer][Integer][a add z]])]] (1 2 choose)`,
		"[1 2 fn (Integer, Integer)]")
	mustCompileWithParity(t,
		`def mk fn [[][Function][fn [[a:Integer z:Integer][Integer][a add z]]]] (1 2 (mk))`,
		"[3]")
	// The trim: a 1-arg callee under a 2-wide window takes the TOP value
	// and the deeper one survives, as the interpreter leaves it.
	mustCompileWithParity(t,
		`def mk fn [[][Function][fn [[a:Integer][Integer][a add 1]]]] (1 2 (mk))`,
		"[1 3]")
	// More params than the window has values: the interpreter leaves the fn
	// UNAPPLIED, and nothing here models that — the refusal stands.
	mustRefuseWithParity(t,
		`def mk fn [[][Function][fn [[a:Integer z:Integer y:Integer][Integer][a add z]]]] (1 2 (mk))`,
		"trailing fn-value apply with fewer args than the callee's arity")
	// A factory whose ARMS return different lambdas has no ONE provable
	// shape, so the arity stays unknown and the standing refusal holds —
	// the row that keeps this graduation honest.
	mustRefuseWithParity(t,
		`def choose fn [[c:Boolean][Function][if c [quote (fn [[a:Integer z:Integer][Integer][a add z]])] [fn [[a:Integer z:Integer][Integer][a mul z]]]]] (1 2 (choose false))`,
		"trailing fn-value apply over a call result (runtime quote state unknown)")
}
