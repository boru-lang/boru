package lang

import (
	"fmt"
	"testing"
)

// restep_flex_parity_test.go pins the last five non-NUR runtime defers
// (test/go/langspec/runtime_defers.tsv, 2026-09-26) end to end: each program
// COMPILES and its compiled run is byte-identical to the interpreter's —
// values, error code, detail and position.
//
//   - A word node read out of a list RE-STEPS (edge-quote-1.tsv L28,
//     edge-quote-3.tsv L56): the check pass re-steps it too and the read emits
//     nothing, so the word's own dispatch — a trap, or a call over the
//     forward tokens and the stack — is what compiles.
//   - `set` on a flex member (flex.tsv L228/L230/L236) commits the overload
//     the runtime lands on through the member's recorded write shape: a
//     FlexMap member (one result) and, the negative side, a class instance
//     stored in the flex (no result) both keep parity.
func TestReStepWordAndFlexMemberSetParity(t *testing.T) {
	const tw2 = `def tw2 (macro [[e] [quote [unquote e add unquote e]]]) `
	cases := []string{
		// The re-stepped word node.
		`quote [add 1 2] get 0`,
		tw2 + `macroexpand (tw2 7) get 1`,
		`quote [add 1 2] get 0 5 6`,
		`3 4 quote [add 1 2] get 0`,
		`2 quote [dup] get 0`,
		`quote [add 1 2] dot 0`,
		`[quote [add 1 2]] get 0 get 0`,
		`def q (quote [add 1 2]) q get 0`,
		`def x 5 quote [x] get 0`,
		`quote [Integer] get 0`,
		`quote [add 1 2] get 1`,
		// The flex member set.
		`def f (flex {}) set a {b:1} f end set b 9 f.a end f.a.b`,
		`def f (flex []) push {x:1} f end set x 9 (f get 0) end (f get 0).x`,
		`def h (flex {z:1}) def f (flex {}) set a h f end set z 9 f.a end h.z`,
		`def f (flex [{a:1}]) set a 5 (f get 0) end f`,
		`def f (flex [[1]]) push 2 (f get 0) end f`,
		`def f (flex [{a:1}]) set 0 5 f end (f get 0) add 1`,
		`def f (flex [{a:1}]) append [3] f end (f get 1) add 1`,
		`def f (flex []) (f get 0)`,
		// A class instance in the flex: the recorded shape keeps set's
		// no-result overload.
		`def P class {x:1} end def f (flex {}) set a (make P {}) f end set x 9 f.a end f.a.x`,
		`def P class {x:1} end def f (flex []) push (make P {}) f end set x 9 (f get 0) end (f get 0).x`,
		`def P class {x:1} end def f (flex {}) set a (make P {}) f end 3 4 set x 9 f.a end add 1`,
	}
	for _, src := range cases {
		gotI, eI := mustNew(t).RunInterp(src)
		gotC, ran, reason, eC := mustNew(t).RunCompiledReason(src)
		if !ran || reason != "" {
			t.Errorf("%q: must compile, got reason %q (%v)", src, reason, eC)
			continue
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(eC) != fmt.Sprint(eI) {
			t.Errorf("%q: compiled %v / %v, interpreted %v / %v", src, gotC, eC, gotI, eI)
		}
	}
}
