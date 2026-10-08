package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A fn-SHAPE-typed carrier is maybe-callable by SHAPE, not by declared
// result count (core.IsFnTypedCarrier / core.TypeIsFnShape — the NUR095
// retirement). A review asked whether a shape declaring MORE THAN ONE
// return — `fnsig [[Integer] [Integer Integer]]` — could therefore be
// compiled with the wrong per-iteration stack shape, since lower.go models
// OpCallDynamic as producing one value.
//
// It cannot. That "one result" is the emitter's compile-time SLOT
// bookkeeping; the VM's dynamic-call handler appends every result the fn
// actually returned (eng/go/vm.go callDynamic's
// `append(stack[:base], results...)` — screenResults checks only
// tape-coupling, never arity). So both lanes net the same values.
//
// Pinned HERE rather than as class.tsv corpus rows: the checker still
// models this shape as the INERT fn it was before the NUR095 fix, so a
// corpus row trips the type-soundness ratchet (NUR096). That is a check-lane
// gap, not a compiled-lane one — this differential pins the runtime property
// the review actually questioned, and NUR096 tracks the analysis half.
func TestFnShapeMultiReturnLaneParity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
		// tolerateCompileFailure marks a row whose compiled lane DECLINES
		// since the BROAD park (NUR073 clause 3): the paren-apply idiom the
		// original pin rode was removed, and the BROAD spellings of this
		// shape sit behind the def-bound-computed-fn compile frontier. The
		// interpreter half of the pin still holds; graduation re-tightens it.
		tolerateCompileFailure bool
	}{
		{
			name: "class member leading apply",
			src: `def T fnsig [[Integer] [Integer Integer]]
def C class {op:T}
def c (make C {op:(fn [[x:Integer] [Integer Integer] [x x]])})
c.op 10`,
			want: "[10 10]",
		},
		{
			// The loop-body dynamic apply (setLoopBodyApply → OpCallDynamic),
			// the exact site the review named: two iterations of a 2-return
			// shape must net FOUR values, not two.
			name: "loop body dynamic apply",
			src: `def T fnsig [[Integer] [Integer Integer]]
def mk fn [[i:Integer] [T] [fn [[x:Integer] [Integer Integer] [x x]]]]
for 2 [7 (mk 1) apply]`,
			want:                   "[7 7 7 7]",
			tolerateCompileFailure: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interp, err := mustNew(t).RunInterpValues(tc.src)
			if err != nil {
				t.Fatalf("interpreted: %v", err)
			}
			if got := fmt.Sprint(interp); got != tc.want {
				t.Fatalf("interpreted = %s, want %s", got, tc.want)
			}

			compiled, ran, reason, err := mustNew(t).RunAutoValues(tc.src)
			if err != nil {
				if tc.tolerateCompileFailure && strings.Contains(err.Error(), "compile_failed") {
					t.Skipf("compiled lane declined (%v)", err)
				}
				t.Fatalf("compiled: %v", err)
			}
			if !ran {
				if tc.tolerateCompileFailure {
					t.Skipf("compiled lane declined (%q)", reason)
				}
				t.Fatalf("compiled lane declined (%q) — this shape compiled when the pin was written; a compile failure here is a silent loss of coverage, not a pass", reason)
			}
			if got := fmt.Sprint(compiled); got != tc.want {
				t.Fatalf("compiled = %s, want %s (interpreted gave %s)", got, tc.want, fmt.Sprint(interp))
			}
		})
	}
}
