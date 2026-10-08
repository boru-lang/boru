package core

import (
	"strings"
	"testing"
)

// The runaway taxonomy (design/legacy/TCO-STAGED.10.ignore Stage 4a): with full
// frame replacement, an infinite TAIL loop consumes CPU, not memory —
// it must trip the step-count guard (evaluation_limit), never the tape
// ceiling (tape_exhausted). Before TCO the same loop was misclassified
// as a memory problem.
//
// Pure-kernel construction: InstallFnDef registers `spin` whose body
// is the unconditional self tail call `spin n`. Under replacement the
// parked frame tails are deleted before their tokens ever dispatch, so
// no lang-layer words (__pa, undef, if) are needed — the loop never
// terminates and never unwinds.

func TestInfiniteTailLoopTripsEvaluationLimitNotTape(t *testing.T) {
	t.Parallel()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.InitRootContext()
	// A tape ceiling tiny enough that ~100 nested frames would blow
	// it, against a step budget large enough for thousands of
	// replaced iterations: whichever guard trips names the resource
	// the loop actually consumes.
	r.TapeConfig = TapeConfig{InitialSize: 512, MaxGrows: 1, GrowthFactor: 2.0}

	InstallFnDef(r, "spin", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewOpenParen(), NewWord("spin"), NewWord("n"), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})

	e := NewTop(r)
	e.stepLimit = 50_000

	_, err = e.Run([]Value{NewWord("spin"), NewInteger(0)})
	if err == nil {
		t.Fatal("infinite tail loop returned nil error")
	}
	if strings.Contains(err.Error(), "tape_exhausted") {
		t.Fatalf("infinite tail loop tripped the MEMORY guard: %v — full replacement should keep the tape O(1) and let the step limit name the real resource", err)
	}
	if !strings.Contains(err.Error(), "evaluation_limit") {
		t.Fatalf("infinite tail loop error = %v, want evaluation_limit", err)
	}
	if r.TCO.Replaced == 0 {
		t.Error("no frames were replaced — the loop survived on tape headroom alone, so this test pinned nothing")
	}

	// The taxonomy's other half: the SAME loop with TCO disabled is a
	// memory runaway and must trip the tape ceiling instead.
	r2, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2.InitRootContext()
	r2.TapeConfig = TapeConfig{InitialSize: 512, MaxGrows: 1, GrowthFactor: 2.0}
	r2.TCO.Disable = true
	InstallFnDef(r2, "spin", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       Boru([]Value{NewOpenParen(), NewWord("spin"), NewWord("n"), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
	e2 := NewTop(r2)
	e2.stepLimit = 50_000
	_, err = e2.Run([]Value{NewWord("spin"), NewInteger(0)})
	if err == nil || !strings.Contains(err.Error(), "tape_exhausted") {
		t.Fatalf("disabled-TCO runaway err = %v, want tape_exhausted", err)
	}
}
