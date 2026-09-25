package lang

import (
	"fmt"
	"strings"
	"testing"
)

// container_member_call_test.go pins the container-member calls
// (2026-09-22): a fn-valued container member — a produced closure stored by
// a map or list literal, or an element of an each-produced list — applied
// through a paren-bounded call, `(fs.b 10)`. Three things were in the way,
// each closed here:
//
//   - a map literal's computed closure member was CONST-FOLDED at the
//     check (a deterministic `(mk 1)`), and a closure's captured state is
//     nothing a const can carry, so the map's first read declined
//     "unannotated or opaque word dot"; the fold now leaves closures to the
//     recording eval, and OpMakeMap assembles the map at run time — the
//     list literal's path all along;
//   - the member-fn read tag recognised only a concrete fn member; a
//     fn-typed CARRIER member (the factory event's result) is one too;
//   - `each` over a fn VALUE callback typed its result as an untyped list;
//     it now carries the fn's one declared return, so `fs.2` over
//     `(each mk/v xs)` is a fn-typed lead the paren apply admits.

const cmcMk = `def mk fn [[k:Integer][Function][([n:Integer] => [n add k])]]  `

// TestContainerMemberCallParity: every row compiles and agrees on both lanes.
func TestContainerMemberCallParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// callbacks L60 and its neighbours: a map literal of produced closures
		{cmcMk + `def fs {a: (mk 1) b: (mk 2)}  (fs.b 10)`, "12", "the paren-bounded member call"},
		{cmcMk + `def fs {a: (mk 1) b: (mk 2)}  fs.b 10`, "12", "the tail form"},
		{cmcMk + `def fs {a: (mk 1) b: (mk 2)}  fs.b`, "fn (Integer)", "the bare read parks the closure"},
		{cmcMk + `def fs {a: (mk 1) b: (mk 2)}  def g fs.b end (g 10)`, "12", "the member def-bound and applied"},
		{cmcMk + `def fs {a: (mk 1) b: (mk 2)}  (fs.a 10) add (fs.b 10)`, "23", "two member calls in one expression"},
		// callbacks L73: an each-produced list of closures
		{cmcMk + `def fs (each mk/v [1 2 3])  (fs.2 10)`, "13", "an element of an each-produced list"},
		{cmcMk + `def fs (each mk/v [1 2 3])  (fs.0 10) add (fs.2 10)`, "24", "two element calls"},
		{cmcMk + `def fs (each mk/v [1 2 3])  typeof fs.1`, "Function", "the element's type"},
		{cmcMk + `def fs (each mk/v [])  size fs`, "0", "an empty each result"},
		{`def inc fn [[n:Integer][Integer][n add 1]] def fs (each inc/v [1 2 3]) (fs get 0) add 1`, "3", "a typed each result over a scalar-returning fn"},
		// module-composition L96: a module factory's closures in a map literal
		{`import module [def mk fn k:Integer Function [([n:Integer] => [n add k])] export "M" {mk: mk/v}] end def fs {a: (M.mk 1) b: (M.mk 2)} end (fs.b 10)`, "12", "a module export's closure as a map member"},
		// the list-literal twin, which compiled before and still does
		{cmcMk + `def fs [(mk 1) (mk 2)]  (fs.1 10)`, "12", "a list literal of produced closures"},
		// a concrete fn member, unchanged
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  (m.f 5)`, "6", "a concrete fn member"},
		{`def m {x: (1 add 2)}  m.x`, "3", "a computed non-fn member still folds"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestContainerMemberCallFlexFieldCompiles — the neighbour that used to
// DECLINE, with the interpreter's answer beside it: a flex registry's fn
// field. The flex SHAPE — the check pass's record of what `set` wrote — is
// not threaded on the compile pass (setFlexMapReturns keeps the legacy
// carrier there), so the read is dynamic Any and the paren lead had no fn
// type to admit it on. Since 2026-09-25 a paren lead that is the RESULT of
// a get/dot read the pass could not type is admitted to the guarded paren
// apply (core.EmitRecorder.ContainerReadResult): the op applies the runtime
// value and defers on a non-callable one, so callbacks L61 left the ledger.
func TestContainerMemberCallFlexFieldCompiles(t *testing.T) {
	rows := []struct{ src, want string }{
		{`def reg (flex {}) end def h fn [[n:Integer][Integer][n add 1]] end reg set 'cb' h/v drop end ((reg.cb) 5)`, "[6]"},
		{`def reg (flex {}) end def h fn [[n:Integer][Integer][n add 1]] end reg set 'cb' h/v drop end (reg.cb 5)`, "[6]"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled", c.src)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s", c.src, gotC, c.want)
		}
	}
}
