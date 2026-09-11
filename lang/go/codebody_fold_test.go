package lang

import (
	"fmt"
	"strings"
	"testing"
)

// codebody_fold_test.go is the whole-program half of the fifty-second
// increment: FoldFullStack is exact PER UNIT, not only for the top one.
//
// The fold bakes the recorder's simulated stack as the runtime stack, and the
// old gate read that as "the top frame of the TOP UNIT". The unit half was
// inherited from where the machinery was first needed, not from the argument:
// a compiled body unit has its own stack discipline exactly as the top unit
// does, and `depth` inside a body counts the BODY's stack on both lanes
// (`[10 20] each [1 2 3 depth]` is 4, not 4-plus-the-collection). What the
// argument genuinely cannot survive is an open FRAGMENT above the unit's root
// frame, whose events are not reconciled into any scope's residual yet — so
// that is what the gate tests now.

func cbfRun(t *testing.T, src string) (ran, island bool, gotC, gotI string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, _, _, _ := a.CompileCheck(src)
	island = prog != nil && strings.Contains(prog.Disassemble(), "FALLBACK")
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vC, ran, _ := b.RunCompiled(src)
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vI, _ := c.RunInterp(src)
	return ran, island, fmt.Sprint(vC), fmt.Sprint(vI)
}

// TestCodeBodyFoldRunsNative — the ledger row and its family. Each compiled
// WITH an interpreter island before; each is native now, and each agrees.
func TestCodeBodyFoldRunsNative(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// THE ledger row (frontier-fullstack.tsv's code-body occurrence).
		{`[10 20] each [drop 1 2 3 1 pick]`, "[[2 2]]"},
		{`[10 20] each [drop 1 2 3 1 roll]`, "[[2 2]]"},
		{`[10 20] each [drop 1 2 3 depth]`, "[[3 3]]"},
		// depth counts the BODY's own stack, and the element is in it —
		// which is the fact that makes the per-unit model exact rather than
		// merely convenient.
		{`[10 20] each [1 2 3 depth]`, "[[4 4]]"},
		{`[10 20] each [depth]`, "[[1 1]]"},
		{`[10 20] each [1 2 1 pick]`, "[[1 1]]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, island, gotC, gotI := cbfRun(t, tc.src)
			if !ran {
				t.Fatal("must compile and run natively")
			}
			if island {
				t.Error("the fold must elide the dispatch, not island it")
			}
			if gotI != tc.want {
				t.Fatalf("the interpreter ORACLE moved: %s, want %s", gotI, tc.want)
			}
			if gotC != gotI {
				t.Errorf("compiled %s, interpreted %s", gotC, gotI)
			}
		})
	}
}

// TestTopLevelFoldUnchanged — the shapes that already folded must keep
// folding, and the NUR131 callable screen must keep declining. A widening
// that quietly took either would be a regression the ledger cannot see.
func TestTopLevelFoldUnchanged(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`1 2 3 1 pick`, "[1 2 3 2]"},
		{`do [1 2 3 1 pick]`, "[1 2 3 2]"},
		{`1 2 3 depth`, "[1 2 3 3]"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			ran, island, gotC, gotI := cbfRun(t, tc.src)
			if !ran || island {
				t.Fatalf("must still compile natively (ran=%v island=%v)", ran, island)
			}
			if gotI != tc.want || gotC != gotI {
				t.Errorf("compiled %s, interpreted %s, want %s", gotC, gotI, tc.want)
			}
		})
	}
}

// TestCodeBodyFoldKeepsTheCallableRefusal — NUR131's screen is per-entry, not
// per-unit, so widening the unit gate must not reopen it: a produced closure
// shuffled inside a body is still data to the fold and still refuses.
func TestCodeBodyFoldKeepsTheCallableRefusal(t *testing.T) {
	const src = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end 5 (mk 3) 0 pick`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, _ := a.CompileCheck(src)
	if prog != nil {
		t.Fatalf("a produced closure in the shuffle must keep its refusal:\n%s", prog.Disassemble())
	}
	if reason == "" {
		t.Error("the refusal must name a reason")
	}
}
