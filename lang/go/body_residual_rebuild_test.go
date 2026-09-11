package lang

import (
	"fmt"
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
)

// The BODY-unit residual rebuild (the fifty-fourth increment). A body unit's
// RET seating lays results out IN PLACE, which cannot express a residual
// whose order is not the order its events produced it in — a full-stack
// shuffle over event results is exactly that shape, and it islanded to the
// interpreter. reconcileResults takes the same seatResidualRebuild the
// program residual has, so the shuffle folds and lowers natively.
//
// The frame sizing is what this pins, and it is a REGRESSION pin: the
// rebuild allocates spill temps, and the fn unit's NLocals write-back used
// to run BEFORE the residual seating — so the frame did not hold them and
// the VM read past its end ("index out of range"). Every unit's NLocals
// must cover every local its own code stores to.
func TestBodyResidualRebuildSizesTheFrame(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	p, reason, _, err := a.CompileCheck(`[10 20] each [drop (1 add 2) (3 add 4) 1 pick]`)
	if err != nil || p == nil {
		t.Fatalf("compile: %v (reason %q)", err, reason)
	}
	if len(p.Fns) == 0 {
		t.Fatal("no fn units — the body should compile to one")
	}
	for _, fn := range p.Fns {
		for pc, in := range fn.Code {
			if in.Op != compiler.OpStoreLocal && in.Op != compiler.OpPushLocal {
				continue
			}
			if int(in.Arg) >= fn.NLocals {
				t.Fatalf("unit %q pc=%d touches local %d with NLocals=%d — the frame does not hold "+
					"the slots its own code uses (the write-back ran before the residual seating)",
					fn.Name, pc, in.Arg, fn.NLocals)
			}
		}
	}
}

// Both directions of the rebuild's two extra screens, which are what make a
// fn RET different from the program residual: a RUNTIME-VARIABLE-count event
// cannot be spilled to one slot (the program residual absorbs such an event;
// a RET does not), and a residual that may carry a CALLABLE may not be
// re-pushed as data at all (NUR124's re-step rule).
func TestBodyResidualRebuildScreens(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"variadic branch result", `do [def b true  do [1 2 (if b [] [9 9])]]`, "no stream placement"},
		{"variadic, directly", `do [def zb true  do [1 (if zb [] [9 9])]]`, "result above a literal"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, cerr := a.RunCompiledStrict(tc.src); cerr == nil {
			t.Fatalf("%s: compiled, want a refusal — one spill slot cannot stand for a run of "+
				"a length the compiler does not know", tc.name)
		} else if !strings.Contains(cerr.Error(), tc.want) {
			t.Fatalf("%s: refused with %q, want %q", tc.name, cerr, tc.want)
		}
		b, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if _, ierr := b.RunInterp(tc.src); ierr != nil {
			t.Fatalf("%s: the interpreter must run it clean (a refusal row may not pin an "+
				"ill-formed program): %v", tc.name, ierr)
		}
	}
	// The CALLABLE screen: a body residual holding a produced closure is not
	// re-pushable as data, so the fold declines and the island stands
	// (NUR131's row, in a body this time). Parity is what matters.
	const src = `def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]] end [10] each [drop 5 (mk 3) 0 pick]`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want, ierr := a.RunInterp(src)
	if ierr != nil {
		t.Fatalf("interpreter: %v", ierr)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	got, _, cerr := b.RunCompiled(src)
	if cerr != nil {
		t.Fatalf("compiled: %v", cerr)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("a closure in a body residual: compiled %v, interpreted %v", got, want)
	}
}
