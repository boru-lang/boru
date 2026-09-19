package lang

import (
	"fmt"
	"testing"
)

// bytecode_storefn_test.go pins S1b-3: a fn VALUE stored in a container
// compiles, because the container mutators declare CompileStoresFn.
//
// `set`, `push`, `unshift` and `append` WRITE their operand into a container
// and never put it back on the tape, so a fn-valued operand is inert to the
// bytecode recorder. Without the declaration the recorder refused "function
// value reaches <word> (Stage 3)" — the blanket guard against a handler
// re-STEPPING a fn the VM has no tape for — and the whole program fell back.
//
// The declaration carries its own soundness discipline, and both halves are
// pinned here: a PURE fn literal rides as an inert const (and its body is
// stamped for the VM), while a CAPTURING or sub-registry fn declines at
// isInertConst and keeps the interpreter fallback, so a stored fn never
// loses its real binding. Every row was measured on the interpreter first.

type storeFnRow struct {
	label string
	src   string
}

var storeFnRows = []storeFnRow{
	// A pure named fn stored and read back: the store compiles, and the
	// read-back application runs on the VM (S1b-1's seam).
	{"a fn stored in a flex, its type read back", `def reg (flex {}) end def h fn [[n:Integer][Integer][n add 1]] end reg set 'cb' h/v drop end typeof (reg.cb)`},
	{"a fn stored in a flex, folded over a list", `def reg (flex {}) end def step fn [[a:Integer e:Integer][Integer][a add e]] end reg set 'f' step/v drop end 0 fold reg.f [1 2 3]`},
	{"a predicate stored in a flex, driving filter", `def reg (flex {}) end def p fn [[kv:Map][Boolean][kv.value gt 1]] end reg set 'f' p/v drop end filter reg.f [1 2 3]`},
	{"a module export stored in a flex", `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end def r (flex {}) end r set 'cb' M.inc/v drop end typeof (r.cb)`},
	{"a fn pushed onto a list, applied from it", `def f fn [[] [Integer] [def g fn [[x:Integer] [Integer] [x add 1]] def lst (push g/v []) end 41 (lst.0)/v apply]]  f`},
	{"lambdas pushed in a loop body", `def f fn [[] [Integer] [def fns [] end for [0 2] [def fns (push ([] => [i]) fns)] end size fns]]  f`},
	{"a fn unshifted onto a list", `def h fn [[n:Integer][Integer][n add 1]] end size (unshift h/v [])`},
	{"a fn appended to a flex list", `def h fn [[n:Integer][Integer][n add 1]] end size (valof ((flex []) append h/v))`},
	{"a fn set into a map returns the new map", `def h fn [[n:Integer][Integer][n add 1]] end typeof ({} set 'cb' h/v)`},
}

// storeFnDeclineRows are the soundness half: a CAPTURING fn is not
// const-bakeable, so the store DECLINES and the interpreter owns the
// program. The answer is what the declaration must never change.
var storeFnDeclineRows = []struct{ label, src, want string }{
	{"a capturing fn stored in a flex", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def reg (flex {}) end reg set 'cb' (mk 5) drop end each reg.cb [1 2 3]`, "[[6 7 8]]"},
	{"a capturing fn pushed onto a list", `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]] end def lst (push (mk 5) []) end each (lst.0) [1 2 3]`, "[[6 7 8]]"},
}

// TestStoreFnParity pins every row on both lanes — value, error code and
// error detail — and pins which rows take the bytecode path. A row marked
// compiles=false is a DECLINE, not a divergence: the interpreter still
// answers, which is exactly what makes the decline sound.
func TestStoreFnParity(t *testing.T) {
	for _, row := range storeFnRows {
		a, _ := New()
		gotI, errI := a.RunInterp(row.src)
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(row.src)
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) {
			t.Errorf("%s: compiled/interp disagree\n  compiled %v / [%s] %s\n  interp   %v / [%s] %s\n  %s",
				row.label, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI), row.src)
			continue
		}
		if !compiled {
			t.Errorf("%s: must take the compiled lane\n  %s", row.label, row.src)
		}
	}
}

// TestStoreFnCapturingDeclines pins the decline: the store REFUSES rather
// than baking a capturing fn as a const, and the interpreter answers. A
// decline is sound where a bake would not be — the stored fn must keep its
// real binding — so this is the boundary the declaration must not cross.
func TestStoreFnCapturingDeclines(t *testing.T) {
	for _, row := range storeFnDeclineRows {
		a, _ := New()
		prog, reason, _, cerr := a.CompileCheck(row.src)
		if cerr != nil {
			t.Fatalf("%s: check: %v", row.label, cerr)
		}
		if prog != nil {
			t.Errorf("%s: compiled — a capturing fn at a store slot must decline (it is not const-bakeable)\n  %s", row.label, row.src)
		} else if reason == "" {
			t.Errorf("%s: refused with no reason\n  %s", row.label, row.src)
		}
		b, _ := New()
		got, err := b.RunInterp(row.src)
		if err != nil || fmt.Sprint(got) != row.want {
			t.Errorf("%s: interpreter answers %v (%v), want %s\n  %s", row.label, got, err, row.want, row.src)
		}
	}
}
