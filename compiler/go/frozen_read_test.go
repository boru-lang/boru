package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The freeze discipline's per-unit tables (unit_memo.go): what a unit BAKED
// (frozen — the compile failure's noun) and at which binding generation (bakes — the
// memo's staleness key). These pin the recording contract directly; the
// end-to-end rows live in lang/go/frozen_module_read_test.go and
// lang/go/analysis_order_test.go.

// openUnit puts the state inside an open unit, which is the only state in
// which NoteFrozenRead records at all.
func openUnit(es *EmitState, storedRef bool) *fnUnitRec {
	rec := &fnUnitRec{name: "u", storedRefUnit: storedRef}
	es.fnRecs = append(es.fnRecs, rec)
	es.openUnitRecs = append(es.openUnitRecs, len(es.fnRecs)-1)
	return rec
}

func TestNoteFrozenReadRecordsTheBakeKindAndGen(t *testing.T) {
	for _, c := range []struct {
		bake core.FrozenBake
		want string
	}{
		{core.FrozenBakeValue, "value"},
		{core.FrozenBakeType, "type"},
		{core.FrozenBakeCall, "call target"},
	} {
		es := NewEmitState()
		rec := openUnit(es, false)
		es.NoteFrozenRead("n", c.bake, 7)
		got, ok := rec.frozen["n"]
		if !ok {
			t.Fatalf("%v: an in-unit module read must record on the OPEN unit", c.bake)
		}
		if got.String() != c.want {
			t.Errorf("%v: compile failure would name %q, want %q", c.bake, got.String(), c.want)
		}
		if rec.bakes["n"] != 7 {
			t.Errorf("%v: the bake must carry the generation the read saw; got %d", c.bake, rec.bakes["n"])
		}
	}
}

// The invalid zero is DROPPED, not stored. A note that reached no classifier
// would otherwise decline a program while naming an artifact nothing froze —
// and "No Zero-Value Overload (CRITICAL)" forbids letting the zero stand in
// for a real member.
func TestNoteFrozenReadDropsTheInvalidZero(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("n", core.FrozenBakeNone, 1)
	if len(rec.frozen) != 0 || len(rec.bakes) != 0 {
		t.Error("an unclassified note must be dropped, not recorded as a default bake")
	}
}

// FIRST BAKE WINS, so the compile failure's text cannot depend on analysis order —
// and the generation kept is the first read's too.
func TestNoteFrozenReadKeepsTheFirstBake(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("n", core.FrozenBakeCall, 3)
	es.NoteFrozenRead("n", core.FrozenBakeValue, 4)
	if got := rec.frozen["n"]; got != core.FrozenBakeCall {
		t.Errorf("second note overwrote the first: got %v", got)
	}
	if rec.bakes["n"] != 3 {
		t.Errorf("second note overwrote the first generation: got %d", rec.bakes["n"])
	}
}

// The negatives, each of which must record NOTHING: no open unit (top level,
// where analysis order is program order), an empty name, a STORED-REF unit
// (whose rebind handling is NotifyNameRebound's own depHit arm), and an open
// unit index the state cannot resolve.
func TestNoteFrozenReadNegatives(t *testing.T) {
	top := NewEmitState()
	top.NoteFrozenRead("n", core.FrozenBakeValue, 1)
	if len(top.fnRecs) != 0 {
		t.Error("a TOP-LEVEL read must not record: the bake is the read the interpreter makes")
	}

	empty := NewEmitState()
	rec := openUnit(empty, false)
	empty.NoteFrozenRead("", core.FrozenBakeValue, 1)
	if len(rec.frozen) != 0 {
		t.Error("an empty name must not record")
	}

	stored := NewEmitState()
	srec := openUnit(stored, true)
	stored.NoteFrozenRead("n", core.FrozenBakeValue, 1)
	if len(srec.frozen) != 0 {
		t.Error("a stored-ref unit's read must stay out of the table")
	}

	dangling := NewEmitState()
	dangling.openUnitRecs = append(dangling.openUnitRecs, 5)
	dangling.NoteFrozenRead("n", core.FrozenBakeValue, 1)
	if len(dangling.fnRecs) != 0 {
		t.Error("an unresolvable open-unit index records nothing")
	}
}

// The latch is for ESCAPING units only: a returned closure, a stamped fn
// value, a fn-value closure body. Its reason names the bake, and it fires
// only with no unit open (a body-local def shadows independently).
func TestNotifyNameReboundNamesTheBakeForAnEscapingUnit(t *testing.T) {
	for _, mark := range []struct {
		what string
		mark func(*fnUnitRec)
	}{
		{"a returned closure (render)", func(r *fnUnitRec) { r.render = "fn […]" }},
		{"a stamped fn value (stampOnly)", func(r *fnUnitRec) { r.stampOnly = true }},
		{"a fn-value closure (lambdaUnit)", func(r *fnUnitRec) { r.lambdaUnit = true }},
	} {
		t.Run(mark.what, func(t *testing.T) {
			es := NewEmitState()
			rec := openUnit(es, false)
			es.NoteFrozenRead("g", core.FrozenBakeCall, 1)
			rec.finished = true
			mark.mark(rec)
			// Still inside the unit: a body-local def shadows independently.
			es.NotifyNameRebound("g")
			if !es.Compilable {
				t.Fatalf("a rebind with a unit still open must not decline; got %q", es.Reason)
			}
			es.openUnitRecs = es.openUnitRecs[:0]
			es.NotifyNameRebound("g")
			want := "module binding g rebound after a fn unit baked its call target"
			if es.Compilable || es.Reason != want {
				t.Errorf("want compile failure %q, got compilable=%v reason=%q", want, es.Compilable, es.Reason)
			}
		})
	}
}

// An ORDINARY unit's bake no longer declines: the memo re-records it at the
// next call site (TestStartFnCompileStaleHitRecompiles). This is the row
// that fails if the latch widens back to every unit.
func TestNotifyNameReboundLeavesAnOrdinaryUnitToTheMemo(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	rec.finished = true
	es.openUnitRecs = es.openUnitRecs[:0]
	es.NotifyNameRebound("k")
	if !es.Compilable {
		t.Errorf("a rebind of a name an ordinary unit baked is the memo's, not the latch's; got %q", es.Reason)
	}
}

// The escape is TRANSITIVE: a returned closure that CALLS a unit which baked
// the name escapes that bake with it — the closure value applies the callee
// through its frozen CALL_USER, which no memo refresh can reach.
func TestNotifyNameReboundWalksFromTheEscapingUnit(t *testing.T) {
	es := NewEmitState()
	callee := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	callee.finished = true
	es.openUnitRecs = es.openUnitRecs[:0]
	caller := &fnUnitRec{name: "mk", render: "fn […]", finished: true,
		frag: &EmitFragment{events: []EmitEvent{{kind: evCallUser, uc: emitUserCall{unit: 0}}}}}
	es.fnRecs = append(es.fnRecs, caller)
	es.NotifyNameRebound("k")
	want := "module binding k rebound after a fn unit baked its value"
	if es.Compilable || es.Reason != want {
		t.Errorf("want compile failure %q through the escaping caller, got compilable=%v reason=%q", want, es.Compilable, es.Reason)
	}
}

// An unfrozen name is untouched by a rebind — the latch is keyed on an actual
// bake, not on "reads a module ref".
func TestNotifyNameReboundIgnoresUnfrozenNames(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	rec.render = "fn […]"
	es.openUnitRecs = es.openUnitRecs[:0]
	es.NotifyNameRebound("never-read")
	if !es.Compilable {
		t.Errorf("a rebind of a name no unit baked must not decline; got %q", es.Reason)
	}
}

// rebindReachesModuleScope is NUR117's fix, and its three regimes are driven
// directly because each needs a suspension state the end-to-end rows cannot
// set on demand. The equality it tests — `suspended == keepModuleDepth +
// multiRunModuleDepth` — is what makes the fix ADDITIVE: any suspension that
// is not a leaking module-scope body run breaks it, so the latch can only gain
// the `do` / multi-run compile failures it is for.
func TestRebindReachesModuleScope(t *testing.T) {
	for _, c := range []struct {
		name                         string
		openUnits, susp, keep, multi int
		want                         bool
	}{
		{"top level, nothing open", 0, 0, 0, 0, true},
		{"a do body at module scope", 0, 1, 1, 0, true},
		{"an each body at module scope", 0, 1, 0, 1, true},
		{"a do inside a do", 0, 2, 2, 0, true},
		{"an each nested inside a do", 0, 2, 1, 0, false},
		{"a fn body (suspended, neither counter)", 0, 1, 0, 0, false},
		{"a do inside a FN body (publication gate declined)", 0, 2, 1, 0, false},
		{"a unit open", 1, 0, 0, 0, false},
		{"a unit open under a do body", 1, 1, 1, 0, false},
	} {
		es := NewEmitState()
		for i := 0; i < c.openUnits; i++ {
			openUnit(es, false)
		}
		es.suspended = c.susp
		es.keepModuleDepth = c.keep
		es.multiRunModuleDepth = c.multi
		if got := es.rebindReachesModuleScope(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	var nilES *EmitState
	if nilES.rebindReachesModuleScope() {
		t.Error("a nil recorder reaches no scope")
	}
}

// The latch must fire while the recorder is SUSPENDED by a do-body run — the
// whole of NUR117. Before the fix NotifyNameRebound returned at its Active()
// gate and never consulted the table.
func TestNotifyNameReboundFiresUnderADoBody(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	rec.finished, rec.stampOnly = true, true
	es.openUnitRecs = es.openUnitRecs[:0]

	// Suspended by a module-scope keep-defs run: the do body's defs leak.
	es.suspended, es.keepModuleDepth = 1, 1
	es.NotifyNameRebound("k")
	want := "module binding k rebound after a fn unit baked its value"
	if es.Compilable || es.Reason != want {
		t.Errorf("a rebind inside a do body must decline; got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// ...and must NOT fire when the suspension is a fn body, whose defs are
// frame-locals. This is the guard's original case and the row that fails if
// the counters are ever incremented unconditionally.
func TestNotifyNameReboundStaysQuietUnderAFnBody(t *testing.T) {
	es := NewEmitState()
	rec := openUnit(es, false)
	es.NoteFrozenRead("k", core.FrozenBakeValue, 1)
	rec.finished, rec.stampOnly = true, true
	es.openUnitRecs = es.openUnitRecs[:0]

	es.suspended = 1 // suspended, but by neither leaking-body counter
	es.NotifyNameRebound("k")
	if !es.Compilable {
		t.Errorf("a fn-body-local rebind must not decline; got %q", es.Reason)
	}
}
