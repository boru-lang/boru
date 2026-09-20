package lang

import (
	"fmt"
	"strings"
	"testing"
)

// NUR037's fn-local fn (the seventy-second increment): a code body — a
// `do` body, an `each` body — naming a fn the ENCLOSING fn's body defined
// declined the whole program, since every path the body could take resolved
// the name in the VM's registry, which never held the enclosing unit's
// local (its `def f fn …` compiled away, the unit reaching f by index). The
// def is placed now as a registry-visible install for the frame
// (placeFnLocalDef: the seventieth increment's `PUSH_CONST fn;
// BIND_DYN_SCOPE f` inside a unit, torn down at RET), so the body —
// compiled, islanded or interpreted — finds f where the interpreter does.
// A capturing local fn keeps the compile failure (its value is a closure the
// placement cannot bake).
func TestFnLocalFnPlacedForCodeBodies(t *testing.T) {
	const local = "def g fn [[][Integer][def f fn [[x:Integer][Integer][x add 1]] end "
	// placed: the local fn's def lowers as a placed install (BIND_DYN_SCOPE
	// in the disassembly); false where the interpreter's own rule leaves
	// nothing to place.
	compiled := []struct {
		src, want string
		placed    bool
	}{
		{local + "do [f 5]]] end g", "[6]", true},
		{"def g fn [[][List][def f fn [[x:Integer][Integer][x add 1]] end [1 2] each [f]]] end g", "[[2 3]]", true},
		{"def g fn [[][List][def f fn [[x:Integer][Integer][x add 1]] end [1 2] each [f 1 add]]] end g", "[[3 4]]", true},
		{"def g fn [[][List][def f fn [[x:Integer][Integer][x add 1]] end def h fn [[x:Integer][Integer][x add 10]] end [1 2] each [f h]]] end g", "[[12 13]]", true},
		// Two bodies over one install; a nested body; a loop around the body.
		{local + "do [f 5] do [f 6] add]] end g", "[13]", true},
		{local + "do [do [f 5]]]] end g", "[6]", true},
		{local + "for 2 [do [f 5] drop] 9]] end g", "[9]", true},
		// Called twice: the install is the frame's, torn down at RET.
		{local + "do [f 5]]] end g g", "[6 6]", true},
		// A module-scope f of a DISJOINT signature shadowed for the frame
		// (the local pushes a fresh entry above it) and restored after it,
		// whichever side is defined first, and across two calls.
		{local + "do [f 5]]] end def f fn [[s:String][String][s]] end g f \"a\"", "[6 a]", true},
		{"def f fn [[s:String][String][s]] end " + local + "do [f 5]]] end g g f \"a\"", "[6 6 a]", true},
		// A module-scope f of an OVERLAPPING signature is not shadowed: the
		// in-body def REPLACES it in place (InstallDef's overlap rule —
		// core/go/core_helpers.go), so the body's f is the module binding,
		// nothing is placed, and the module-level read after g sees the
		// replacement — as the interpreter does.
		{local + "do [f 5]]] end def f fn [[x:Integer][Integer][x add 100]] end f 1 g f 1", "[101 6 2]", false},
		// An in-body undef of the local before any read: the def is never
		// reached by a body, so it compiles away with its undef — the
		// interpreter's residual.
		{local + "do [undef f 5]]] end g", "[5]", false},
		// An in-body undef AFTER the read: the pass's registry holds no f by
		// the time the body's dispatch is recorded, so the fn-local predicate
		// does not fire — the body's call commits by index and the undef
		// compiles away, as on main before this increment.
		{local + "do [f 5 undef f]]] end g", "[6]", false},
		// Defined inside an arm the model cannot decide: the seventieth
		// increment placed it already, and the body's dispatch routes.
		{"def m {e: true} end def g fn [[][Integer][if (m \"e\" get) [def f fn [[x:Integer][Integer][x add 1]] end] [] do [f 5]]] end g", "[6]", true},
		// Two locals named by one ISLANDED body (an error handler's), both
		// placed (review of #468: only the first was, and the island raised
		// undefined_word for the second); a unit-level redefinition of the
		// local between two bodies, each placed at its site.
		{"def g fn [[][List][def f fn [[x:Integer][Integer][x add 1]] end def h fn [[s:String][String][s]] end do [raise x \"e\"] error [[f 5 h \"b\"]]]] end g", "[[6 'b']]", true},
		{local + "do [f 5] def f fn [[x:Integer][Integer][x add 10]] end do [f 5] add]] end g", "[21]", true},
		// The local's name is ALSO a speculative family's at module scope
		// (the seventieth's), of a DISJOINT signature: the local is placed
		// itself (the frames are searched before the family shortcut), the
		// body's routed dispatch resolves it live, and the module read after
		// g resolves the family's. An OVERLAPPING signature here is NUR149's.
		{"def m {e: true} end if (m \"e\" get) [def f fn [[x:Integer][Integer][x add 100]] end] [] end def g fn [[][String][def f fn [[s:String][String][s]] end do [f \"a\"]]] end g f 1", "[a 101]", true},
	}
	for _, c := range compiled {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile, got reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "BIND_DYN_SCOPE") != c.placed {
			t.Errorf("%q: the local fn's def placed=%v, want %v:\n%s", c.src, !c.placed, c.placed, dis)
		}
		gotC, ran, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !ran {
			t.Errorf("%q: the compiled run stays compiled", c.src)
		}
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q: got %s, want %s", c.src, got, c.want)
		}
	}
	declined := []struct{ src, reason string }{
		// A capturing local fn: its value is a closure the placement cannot
		// bake.
		{"def g fn [[k:Integer][Integer][def f fn [[x:Integer][Integer][x add k]] end do [f 5]]] end g 10", "code-body names fn-local fn `f` at `do`"},
		// The current binding is a CLOSED body's redefinition (review of
		// #468): the unit's def event is the original's, not the current
		// binding's, so there is no install to place — declined, where
		// stamping the stale event installed the original (6 for 15).
		{local + "do [def f fn [[x:Integer][Integer][x add 10]] end] do [raise x \"e\"] error [f 5]]] end g", "code-body names fn-local fn `f` at `do`"},
		// The same rule where the redefining body is the reading one: the
		// body's def replaces the local in place (the overlap rule) before
		// the site records, so the current binding is the closed body's
		// (55 and 111 answered by index before the review; declined now).
		{local + "do [def f fn [[x:Integer][Integer][x add 50]] end f 5]]] end g", "code-body names fn-local fn `f` at `do`"},
		{local + "do [def f fn [[x:Integer][Integer][x add 50]] end f 5] f 6 add]] end g", "code-body names fn-local fn `f` at `do`"},
		// A VALUE read of the local (review of #468): the closure returned
		// the fn as data where the interpreter dispatches the returned
		// value (uncalled_function); `/u` the same.
		{"def g fn [[][Any][def f fn [[x:Integer][Integer][x add 1]] end do [f/v]]] end g", "code-body names fn-local fn `f` at `do`"},
		{"def g fn [[][Any][def f fn [[x:Integer][Integer][x add 1]] end do [f/u 5]]] end g", "code-body names fn-local fn `f` at `do`"},
	}
	for _, c := range declined {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: want the compile failure %q…, got compiled=%v reason=%q", c.src, c.reason, prog != nil, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}
