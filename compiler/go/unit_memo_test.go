package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The binding-sensitive unit memo, the body re-run environment and the
// residual-order hazard (unit_memo.go), driven directly. The end-to-end rows
// are lang/go/analysis_order_test.go and lang/go/frozen_module_read_test.go;
// these pin the arms those rows cannot reach on demand — the in-flight
// exemption, the reachability walk's every edge, the environment's declines
// and its type carry-over, and the hazard's fragment keying.

// memoState arms a recording state on a fresh registry with the top-level
// unit open, as CompileCheck does before the first record.
func memoState(t *testing.T) (*EmitState, *core.Registry) {
	t.Helper()
	r := newTestRegistry(t)
	es := NewEmitState()
	es.reg = r
	es.Compilable = true
	return es, r
}

// --- the memo ---------------------------------------------------------------

// A finished unit whose bake's generation has moved is STALE: StartFnCompile
// allocates a fresh unit for the call site and re-points the key at it, while
// the stale unit stays in fnRecs for the sites that already reference it.
func TestStartFnCompileStaleHitRecompiles(t *testing.T) {
	es, r := memoState(t)
	r.Defs.Push("k", core.NewInteger(5))
	unit, finish, ok := es.StartFnCompile("key", "f", r, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok || unit != 0 || finish == nil {
		t.Fatalf("first StartFnCompile: unit=%d finish=%v ok=%v", unit, finish != nil, ok)
	}
	es.NoteFrozenRead("k", core.FrozenBakeValue, r.Defs.Gen("k"))
	finish(nil)

	// Same bindings: a hit, no re-analysis.
	if u, fin, ok := es.StartFnCompile("key", "f", r, nil, nil, nil, nil, false, core.SrcPos{}); !ok || u != 0 || fin != nil {
		t.Fatalf("unchanged binding must hit the memo: unit=%d finish=%v ok=%v", u, fin != nil, ok)
	}
	// The rebind moves k's generation: the next site gets a fresh unit.
	r.Defs.Replace("k", core.NewInteger(9))
	u, fin, ok := es.StartFnCompile("key", "f", r, nil, nil, nil, nil, false, core.SrcPos{})
	if !ok || u != 1 || fin == nil {
		t.Fatalf("a moved generation must miss: unit=%d finish=%v ok=%v", u, fin != nil, ok)
	}
	if es.fnUnits["key"] != 1 || len(es.fnRecs) != 2 {
		t.Errorf("the key must re-point at the fresh unit and the stale one stay: fnUnits=%v recs=%d", es.fnUnits, len(es.fnRecs))
	}
	fin(nil)
}

// An UNFINISHED unit is never stale, whatever its bakes say: in-flight
// recursion must reuse the in-flight unit.
func TestUnitStaleSparesAnInFlightUnit(t *testing.T) {
	es, r := memoState(t)
	r.Defs.Push("k", core.NewInteger(5))
	unit, finish, _ := es.StartFnCompile("key", "f", r, nil, nil, nil, nil, false, core.SrcPos{})
	es.NoteFrozenRead("k", core.FrozenBakeValue, r.Defs.Gen("k"))
	r.Defs.Replace("k", core.NewInteger(9))
	if es.unitStale(unit) {
		t.Error("an open unit's bakes are incomplete; it must be reused, not recompiled")
	}
	if u, fin, ok := es.StartFnCompile("key", "f", r, nil, nil, nil, nil, false, core.SrcPos{}); !ok || u != unit || fin != nil {
		t.Errorf("the recursive call must hit the in-flight unit: unit=%d finish=%v", u, fin != nil)
	}
	finish(nil)
	if !es.unitStale(unit) {
		t.Error("once finished, the moved generation makes it stale")
	}
}

// Staleness is TRANSITIVE through every kind of unit reference: a caller
// that never read the name is stale when a unit it references is.
func TestUnitStaleWalksEveryReference(t *testing.T) {
	es, r := memoState(t)
	r.Defs.Push("k", core.NewInteger(5))
	stale := &fnUnitRec{name: "callee", finished: true, reg: r, bakes: map[string]int64{"k": r.Defs.Gen("k") - 1}}
	fresh := &fnUnitRec{name: "fresh", finished: true, reg: r}
	es.fnRecs = append(es.fnRecs, stale, fresh) // 0: stale, 1: fresh
	for _, c := range []struct {
		what string
		rec  *fnUnitRec
		want bool
	}{
		{"a user call", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}, true},
		{"a poly arm", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: -1, poly: &emitUserPolySpec{units: []int{1, 0}}}}}}}, true},
		{"a closure operand of a call", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opClosure, closureUnit: 0}}}}}}}, true},
		{"a closure in the residual", &fnUnitRec{finished: true, outOps: []EmitOperand{{kind: opClosure, closureUnit: 0}}}, true},
		{"a closure in the ret prefix", &fnUnitRec{finished: true, retPrefix: []EmitOperand{{kind: opClosure, closureUnit: 0}}}, true},
		{"inside a branch's then arm", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evBranch, br: &emitBranch{then: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}}}}}, true},
		{"inside a branch's else arm", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evBranch, br: &emitBranch{els: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}}}}}, true},
		{"inside a branch's condition", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evBranch, br: &emitBranch{condFrag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}}}}}, true},
		{"inside a loop body", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evLoop, loop: &emitLoop{body: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}}}}}, true},
		{"a loop body's inert residual", &fnUnitRec{finished: true, frag: &EmitFragment{residualOps: []EmitOperand{{kind: opClosure, closureUnit: 0}}}}, true},
		{"a loop body's apply args", &fnUnitRec{finished: true, frag: &EmitFragment{applyArgs: []EmitOperand{{kind: opClosure, closureUnit: 0}}}}, true},
		{"a loop body's apply fn", &fnUnitRec{finished: true, frag: &EmitFragment{applyFn: &EmitOperand{kind: opClosure, closureUnit: 0}}}, true},
		{"only the fresh unit", &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 1}}}}}, false},
		{"no references at all", &fnUnitRec{finished: true}, false},
	} {
		es.fnRecs = append(es.fnRecs[:2], c.rec)
		if got := es.unitStale(2); got != c.want {
			t.Errorf("%s: stale=%v, want %v", c.what, got, c.want)
		}
	}
	// Out-of-range and nil records are never stale, and the walk terminates
	// on a cycle.
	es.fnRecs = append(es.fnRecs[:2], nil)
	if es.unitStale(2) || es.unitStale(99) || es.unitStale(-1) {
		t.Error("a nil or absent record is not stale")
	}
	cyc := &fnUnitRec{finished: true, frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 2}}}}}
	es.fnRecs = append(es.fnRecs[:2], cyc)
	if es.unitStale(2) {
		t.Error("a self-referencing fresh unit is not stale")
	}
	// A bake with no registry to compare against reads as stale (the
	// conservative direction).
	es.reg = nil
	es.fnRecs = append(es.fnRecs[:2], &fnUnitRec{finished: true, bakes: map[string]int64{"k": 1}})
	if !es.unitStale(2) {
		t.Error("with no registry at all, a baked unit cannot be proven fresh")
	}
}

// The escaping predicate names exactly the three flags.
func TestUnitEscapes(t *testing.T) {
	if unitEscapes(nil) || unitEscapes(&fnUnitRec{closure: true}) || unitEscapes(&fnUnitRec{storedRefUnit: true}) {
		t.Error("an ordinary, closure or stored-ref unit does not escape")
	}
	if !unitEscapes(&fnUnitRec{render: "x"}) || !unitEscapes(&fnUnitRec{stampOnly: true}) || !unitEscapes(&fnUnitRec{lambdaUnit: true}) {
		t.Error("a returned closure, a stamped value and a fn-value closure all escape")
	}
}

// --- the body re-run environment --------------------------------------------

// publishBodyStart's guards, and the fixed-point rule: a later round keeps
// the first round's start.
func TestPublishBodyStart(t *testing.T) {
	es, r := memoState(t)
	first, second := core.NewDefTable(), core.NewDefTable()
	es.publishBodyStart("", r, first, false)
	es.publishBodyStart("b", r, nil, false)
	if len(es.bodyStarts) != 0 {
		t.Fatal("an empty id or a nil table publishes nothing")
	}
	resume := es.Suspend()
	es.publishBodyStart("b", r, first, false)
	resume()
	if len(es.bodyStarts) != 0 {
		t.Fatal("a close that finds the recorder suspended publishes nothing")
	}
	es.publishBodyStart("b", r, first, false)
	es.publishBodyStart("b", r, second, true)
	if es.bodyStarts["b"].defs != first {
		t.Error("a later fixed-point round must keep the first round's start")
	}
	es.publishBodyStart("b", r, second, false)
	if es.bodyStarts["b"].defs != second {
		t.Error("a new dispatch of the same body replaces the start")
	}
	var nilES *EmitState
	nilES.publishBodyStart("b", r, first, false)
}

// takeBodyEnv claims exactly once, only for a leaking body with a value id.
func TestTakeBodyEnv(t *testing.T) {
	es, r := memoState(t)
	body := core.NewList([]core.Value{core.NewInteger(1)})
	body.ID = "body-1" // ids are minted inside a pass; the test supplies one
	es.publishBodyStart(body.ID, r, core.NewDefTable(), false)
	if es.takeBodyEnv(body, core.CallableSpec{}) != nil {
		t.Error("a body that neither keeps nor multi-runs defs needs no environment")
	}
	if es.takeBodyEnv(core.Value{}, core.CallableSpec{BodyOnceKeepsDefs: true}) != nil {
		t.Error("an id-less body has nothing published")
	}
	env := es.takeBodyEnv(body, core.CallableSpec{BodyMultiRunKeepsDefs: true})
	if env == nil || !env.multi || env.reg != r {
		t.Fatalf("a multi-run body takes its start: %+v", env)
	}
	if es.takeBodyEnv(body, core.CallableSpec{BodyOnceKeepsDefs: true}) != nil {
		t.Error("the start is claimed exactly once")
	}
	var nilES *EmitState
	if nilES.takeBodyEnv(body, core.CallableSpec{BodyOnceKeepsDefs: true}) != nil {
		t.Error("a nil recorder has no environment")
	}
}

// A once-run environment swaps the START table in and the leaked table back.
func TestBodyRunEnvOnce(t *testing.T) {
	_, r := memoState(t)
	r.Defs.Push("k", core.NewInteger(5))
	start := r.Defs.Clone()
	r.Defs.Replace("k", core.NewInteger(9)) // the leak
	env := &bodyRunEnv{start: start, reg: r}
	prev, ok := env.enter(r)
	if !ok || prev == nil {
		t.Fatalf("enter: prev=%v ok=%v", prev, ok)
	}
	if v, _ := r.Defs.Top("k"); v.String() != "5" {
		t.Errorf("inside the environment the START binding is live: k=%v", v)
	}
	if r.Defs == start {
		t.Error("the environment is a CLONE of the start, so a compile cannot mutate the published start")
	}
	env.exit(r, prev)
	if v, _ := r.Defs.Top("k"); v.String() != "9" {
		t.Errorf("after exit the leaked binding is back: k=%v", v)
	}
	// Guards: a nil env, a foreign registry, and a nil start all enter nothing.
	var nilEnv *bodyRunEnv
	if p, ok := nilEnv.enter(r); p != nil || !ok {
		t.Error("a nil environment enters nothing")
	}
	other := newTestRegistry(t)
	if p, ok := env.enter(other); p != nil || !ok {
		t.Error("an environment taken on another registry enters nothing")
	}
	if p, ok := (&bodyRunEnv{reg: r}).enter(r); p != nil || !ok {
		t.Error("an environment with no start enters nothing")
	}
	nilEnv.exit(r, prev)
	env.exit(r, nil)
	env.exit(nil, prev)
}

// A multi-run environment replaces every name the body rebound with the JOIN
// of its start and end carriers, leaves unchanged and type names alone, and
// declines a name the body unbound or rebound to a function value.
func TestBodyRunEnvMulti(t *testing.T) {
	_, r := memoState(t)
	r.Defs.Push("k", core.NewInteger(5))
	r.Defs.Push("same", core.NewInteger(1))
	r.Defs.PushTypeAdopted("T", core.TInteger, core.NewTypeLiteral(core.TInteger))
	start := r.Defs.Clone()
	r.Defs.Replace("k", core.NewString("s")) // rebound per element
	r.Defs.Replace("T", core.NewTypeLiteral(core.TString))
	env := &bodyRunEnv{start: start, reg: r, multi: true}
	prev, ok := env.enter(r)
	if !ok {
		t.Fatal("a value rebind joins rather than declines")
	}
	k, _ := r.Defs.Top("k")
	if core.IsConcrete(k) || !k.Carrier {
		t.Errorf("a rebound name reads as a carrier inside the environment, got %v", k)
	}
	if s, _ := r.Defs.Top("same"); s.String() != "1" || s.Carrier {
		t.Errorf("an unchanged name keeps its start binding: %v", s)
	}
	if e, _ := r.Defs.TopEntry("T"); e.TypeDef != core.TInteger {
		t.Errorf("a type name keeps its start binding: %v", e.TypeDef)
	}
	env.exit(r, prev)

	// The declines.
	r2 := newTestRegistry(t)
	r2.Defs.Push("k", core.NewInteger(5))
	start2 := r2.Defs.Clone()
	r2.Defs.Pop("k")
	if _, ok := (&bodyRunEnv{start: start2, reg: r2, multi: true}).enter(r2); ok {
		t.Error("a name the body unbinds declines")
	}
	r3 := newTestRegistry(t)
	r3.Defs.Push("k", core.NewInteger(5))
	start3 := r3.Defs.Clone()
	r3.Defs.Replace("k", core.NewFunction(core.FnDefInfo{Name: "k"}))
	if _, ok := (&bodyRunEnv{start: start3, reg: r3, multi: true}).enter(r3); ok {
		t.Error("a name rebound to a function value declines")
	}
}

// A type the body installed is carried over from the leaked table — minted
// as minted, adopted as adopted — so the re-run re-defines it over its
// surviving lattice part instead of tripping the parts conflict.
func TestBodyRunEnvCarriesInstalledTypes(t *testing.T) {
	_, r := memoState(t)
	start := r.Defs.Clone()
	minted := r.Types.MintType("Big", core.TInteger)
	r.Defs.PushType("Big", minted, core.NewTypeLiteral(minted))
	r.Defs.PushTypeAdopted("Alias", core.TInteger, core.NewTypeLiteral(core.TInteger))
	r.Defs.Push("v", core.NewInteger(1)) // a value install is NOT carried
	env := &bodyRunEnv{start: start, reg: r}
	prev, _ := env.enter(r)
	if e, ok := r.Defs.TopEntry("Big"); !ok || !e.Minted || e.TypeDef != minted {
		t.Errorf("a minted type carries over minted: %+v (present=%v)", e, ok)
	}
	if e, ok := r.Defs.TopEntry("Alias"); !ok || e.Minted || e.TypeDef != core.TInteger {
		t.Errorf("an adopted alias carries over adopted: %+v (present=%v)", e, ok)
	}
	if r.Defs.Has("v") {
		t.Error("a value the body installed is restored to absent: that is the whole point")
	}
	env.exit(r, prev)
}

// bindCarrier keeps a carrier, widens a concrete value to its type, keeps a
// container's shape, and reads a root literal as the gradual Any.
func TestBindCarrier(t *testing.T) {
	c := core.NewCarrier(core.TString)
	if got := bindCarrier(c); !got.Carrier || got.Parent != c.Parent {
		t.Error("a carrier is itself")
	}
	if got := bindCarrier(core.NewInteger(5)); !got.Carrier || core.IsConcrete(got) || !got.Parent.Equal(core.TInteger) {
		t.Errorf("a scalar widens to its type carrier: %v", got)
	}
	if got := bindCarrier(core.NewList(nil)); !got.Parent.Equal(core.TList) || got.Data == nil {
		t.Errorf("a list keeps its container shape: %v", got)
	}
	if got := bindCarrier(core.NewMap(nil)); !got.Parent.Equal(core.TMap) || got.Data == nil {
		t.Errorf("a map keeps its container shape: %v", got)
	}
	if got := bindCarrier(core.Value{}); !got.Dynamic || !got.Carrier {
		t.Errorf("a root literal is the gradual Any: %v", got)
	}
}

// --- the residual-order hazard ----------------------------------------------

// The hazard is keyed by FRAGMENT: a read and a later bind in one fragment
// mark that fragment (and every open enclosing one that had read the name);
// a sibling fragment that merely shares the name is untouched, and a read in
// a nested fragment never marks the enclosing residual.
func TestResidualReadHazardIsFragmentKeyed(t *testing.T) {
	es, _ := memoState(t)
	v := core.NewInteger(5)
	v.ID = "v-k"
	es.defReads = map[string]string{v.ID: "k"}
	dyn := dynScopeOperand(0)

	// Root: no fragment, a read then a bind.
	es.noteFragRead("k")
	es.noteBindHazard("k")
	if got := es.residualReadHazard(v, dyn, nil); !strings.Contains(got, "residual read of `k` precedes its rebind") {
		t.Errorf("root read-before-bind must decline, got %q", got)
	}

	// An enclosing fragment U that read k, a nested fragment N that binds it:
	// U is marked (its read precedes the bind), N is not (it never read).
	endU := es.beginFragment()
	es.noteFragRead("k")
	endN := es.beginFragment()
	es.noteBindHazard("k")
	endN()
	nested := es.captured
	endU()
	outer := es.captured
	if es.residualReadHazard(v, dyn, outer) == "" {
		t.Error("the enclosing fragment read k before a nested bind: its residual re-push is hazardous")
	}
	if es.residualReadHazard(v, dyn, nested) != "" {
		t.Error("the nested fragment never read k: nothing to re-push")
	}

	// A sibling fragment: a read in A, a bind in B — B's residual is safe,
	// and so is A's (A closed before the bind).
	endA := es.beginFragment()
	es.noteFragRead("z")
	endA()
	a := es.captured
	endB := es.beginFragment()
	es.noteBindHazard("z")
	endB()
	b := es.captured
	z := core.NewInteger(1)
	z.ID = "v-z"
	es.defReads[z.ID] = "z"
	if es.residualReadHazard(z, dyn, a) != "" || es.residualReadHazard(z, dyn, b) != "" {
		t.Error("sibling fragments do not share a hazard")
	}

	// A bind with no prior read marks nothing; a read AFTER the bind is safe.
	endC := es.beginFragment()
	es.noteBindHazard("w")
	es.noteFragRead("w")
	endC()
	w := core.NewInteger(2)
	w.ID = "v-w"
	es.defReads[w.ID] = "w"
	if es.residualReadHazard(w, dyn, es.captured) != "" {
		t.Error("a read after the bind re-pushes the rebound value, which is the interpreter's")
	}
}

// The operand kinds: only a live lookup or a loop-carried slot are re-pushed
// late; a const or event is not, whatever the tables say. The dyn-scope name
// comes from the value's DynFrom tag when defReads has none.
func TestResidualReadHazardOperandKinds(t *testing.T) {
	es, _ := memoState(t)
	v := core.NewInteger(5)
	v.ID = "v-k"
	es.defReads = map[string]string{v.ID: "k"}
	es.noteFragRead("k")
	es.noteStoreHazard("k", 3)
	if es.residualReadHazard(v, ConstOperand(0), nil) != "" || es.residualReadHazard(v, EventOperand(1, 0), nil) != "" {
		t.Error("a const or event residual is not re-pushed late")
	}
	if got := es.residualReadHazard(v, localOperand(3), nil); !strings.Contains(got, "`k`") {
		t.Errorf("the stored slot's residual re-push is hazardous and names the def: %q", got)
	}
	if es.residualReadHazard(v, localOperand(4), nil) != "" {
		t.Error("another slot is untouched")
	}
	anon := core.NewInteger(7)
	anon.ID = "v-anon"
	if got := es.residualReadHazard(anon, localOperand(3), nil); !strings.Contains(got, "a loop-carried def") {
		t.Errorf("a slot read with no name falls back to the generic noun: %q", got)
	}
	tagged := core.NewCarrier(core.TInteger)
	tagged.SetDynFrom("k")
	if es.residualReadHazard(tagged, dynScopeOperand(0), nil) == "" {
		t.Error("a DynFrom-tagged read names its binding without a defReads entry")
	}
	unnamed := core.NewInteger(8)
	unnamed.ID = "v-unnamed"
	if es.residualReadHazard(unnamed, dynScopeOperand(0), nil) != "" {
		t.Error("an unnameable dyn-scope read cannot be judged, so it passes")
	}
	// A store with no prior read of the name marks nothing.
	es2, _ := memoState(t)
	es2.noteStoreHazard("q", 1)
	if len(es2.storeHazard) != 0 || len(es2.bindHazard) != 0 {
		t.Error("a store the fragment never read before marks nothing")
	}
	// Nil-receiver safety across the whole family.
	var nilES *EmitState
	nilES.noteFragRead("k")
	nilES.noteBindHazard("k")
	nilES.noteStoreHazard("k", 0)
	if nilES.curFrag() != 0 || nilES.residualReadHazard(v, dyn(), nil) != "" {
		t.Error("a nil recorder tracks nothing")
	}
	es.noteFragRead("")
	es.noteBindHazard("")
	es.noteStoreHazard("", 0)
}

func dyn() EmitOperand { return dynScopeOperand(0) }

// forkForProbe carries the open fragment ids and the hazard tables, so a
// probe judges a residual exactly as the real compile will.
func TestForkForProbeCarriesTheHazardTables(t *testing.T) {
	es, _ := memoState(t)
	end := es.beginFragment()
	es.noteFragRead("k")
	es.noteBindHazard("k")
	es.noteStoreHazard("k", 2)
	p := es.forkForProbe()
	end()
	if p.curFrag() != es.fragSeq || p.fragSeq != es.fragSeq {
		t.Errorf("the probe continues the fragment counter from the real state: probe=%d real=%d", p.fragSeq, es.fragSeq)
	}
	if !p.fragReads[readKey{"k", 1}] || !p.bindHazard[readKey{"k", 1}] || !p.storeHazard[slotKey{2, 1}] {
		t.Error("the probe must see the real state's reads and hazards")
	}
	// The top-level computed fn value-defs too, as a clone: a closure body's
	// unit snapshots them at open, and a def-bound fn read at its tail is
	// admitted to the dynamic-scope rescue only through that snapshot.
	es.rootComputedBindIDs = map[string]bool{"a5": true}
	p = es.forkForProbe()
	if !p.rootComputedBindIDs["a5"] {
		t.Error("the probe must see the real state's root computed binds")
	}
	p.rootComputedBindIDs["z"] = true
	if es.rootComputedBindIDs["z"] {
		t.Error("the probe's copy must not write through to the real state")
	}
}

// residualStands is the ONE compile failure site the four residual settlements share
// (a fn unit's finish, a branch arm, both loop arms): an unresolved residual
// declines as "<what> of unknown provenance" under the caller's prefix, a
// resolved one under the residual-order hazard, and a safe one stands. The
// compile failure-site census counts call sites, so the arms are pinned here rather
// than through four separate end-to-end shapes.
func TestResidualStandsIsTheSharedCompileFailureSite(t *testing.T) {
	var nilES *EmitState
	if nilES.residualStands("fn f: ", core.Value{}, EmitOperand{}, true, nil, "body result") {
		t.Error("a nil recorder settles nothing")
	}

	es := NewEmitState()
	es.Compilable = true
	v := core.NewInteger(1)
	v.ID = "v-stands"
	if !es.residualStands("fn f: ", v, ConstOperand(0), true, nil, "body result") || !es.Compilable {
		t.Errorf("a resolved, hazard-free residual must stand; got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
	if es.residualStands("if: then-branch ", v, EmitOperand{}, false, nil, "result") || es.Compilable ||
		es.Reason != "if: then-branch result of unknown provenance" {
		t.Errorf("an unresolved residual must decline under the caller's prefix; got compilable=%v reason=%q", es.Compilable, es.Reason)
	}

	es = NewEmitState()
	es.Compilable = true
	es.defReads = map[string]string{"v-stands": "k"}
	es.bindHazard = map[readKey]bool{{"k", 0}: true}
	if es.residualStands("for: ", v, dynScopeOperand(0), true, nil, "body result") || es.Compilable ||
		!strings.Contains(es.Reason, "for: residual read of `k` precedes its rebind") {
		t.Errorf("a hazardous residual must decline under the hazard's reason; got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// A nil unit record is skipped by both walks, not dereferenced: the
// reachability primitive returns on a nil receiver, and the escaping-latch
// walk returns at a nil index a caller's CALL_USER names. Production never
// appends a nil record; the guards are the no-panic contract (ADR-005)
// for the direct-drive callers that may.
func TestUnitWalksSkipANilRecord(t *testing.T) {
	es := NewEmitState()
	called := 0
	es.forEachUnitRef(nil, func(int) { called++ })
	if called != 0 {
		t.Errorf("a nil record references no unit; the walk visited %d", called)
	}

	caller := &fnUnitRec{name: "mk", render: "fn […]", finished: true,
		frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}
	es.fnRecs = []*fnUnitRec{nil, caller}
	if _, hit := es.frozenInEscapingUnit("k"); hit {
		t.Error("a nil callee holds no bake; the escaping walk must report none")
	}
}
