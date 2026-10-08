package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// nur207_gradual_fn_test.go pins the recorder half of NUR207 (2026-09-26):
// the parking-member fold (tryFoldParkedMemberFn), the branch flag an arm
// that leaves a fn sets (armsLeaveFn) and the read poison over it, the proof
// that a carrier not typed Function holds a fn (gradualHoldsFn,
// eventLeavesFn), the gradual claims at a def (noteClosureShapeBind's member
// arm, memberLambdaShape), the user-call decline over such a carrier, and
// `apply`'s pending registration over a gradual lead.

func n207Lambda(params ...core.FnParam) core.Value {
	return core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{
		Params: params, BarrierPos: len(params), Impl: core.Boru([]core.Value{core.NewInteger(42)}),
	}}})
}

func TestParkedMemberFn(t *testing.T) {
	lam := n207Lambda()
	named := core.NewFunction(core.FnDefInfo{Name: "one", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	quoted := lam
	quoted.Quoted = true
	applied := n207Lambda()
	fd := applied.Data.(core.FnDefInfo)
	fd.Applied = true
	applied.Data = fd
	macro := n207Lambda()
	fd = macro.Data.(core.FnDefInfo)
	fd.Macro = true
	macro.Data = fd
	capturing := n207Lambda()
	fd = capturing.Data.(core.FnDefInfo)
	fd.Captured = []core.CapturedBinding{{Name: "k", Value: core.NewInteger(1)}}
	capturing.Data = fd
	fallback := n207Lambda()
	fd = fallback.Data.(core.FnDefInfo)
	fd.Signatures = append(fd.Signatures, core.Signature{Fallback: true})
	fallback.Data = fd
	noSigs := core.NewFunction(core.FnDefInfo{Anonymous: true})
	if !parkedMemberFn(lam) {
		t.Error("an anonymous capture-free only-0-arg lambda parks at every value landing")
	}
	for name, v := range map[string]core.Value{
		"named (fires at its landing)": named, "arg-taking": n207Lambda(core.FnParam{Type: core.TInteger}),
		"quoted": quoted, "applied": applied, "macro": macro, "capturing": capturing,
		"a fallback overload": fallback, "no signature": noSigs, "not a fn": core.NewInteger(1),
		"a carrier": core.NewCarrier(core.TFunction),
	} {
		if parkedMemberFn(v) {
			t.Errorf("%s: not a parking member", name)
		}
	}
}

func TestTryFoldParkedMemberFn(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()
	lam := n207Lambda()
	om := core.NewOrderedMap()
	om.Set("f", lam)
	om.Set("s", n207Lambda(core.FnParam{Type: core.TInteger}))
	m := core.NewMap(om)
	key := core.NewAtom("f")
	outs := []core.Value{core.NewDynamicCarrier(core.TAny)}
	if tryFoldParkedMemberFn(reg, "getr", []core.Value{key, m}, outs) {
		t.Error("getr raises on a miss: not the fold's word")
	}
	if tryFoldParkedMemberFn(reg, "dot", []core.Value{core.NewAtom("s"), m}, outs) {
		t.Error("an arg-taking member is the arrival model's")
	}
	if tryFoldParkedMemberFn(reg, "dot", []core.Value{core.NewAtom("zz"), m}, outs) {
		t.Error("a miss folds nothing")
	}
	if tryFoldParkedMemberFn(reg, "dot", []core.Value{key, m}, nil) {
		t.Error("no out to rewrite")
	}
	if !tryFoldParkedMemberFn(reg, "dot", []core.Value{key, m}, outs) {
		t.Fatal("the parking member of a concrete container folds")
	}
	if _, isFn := outs[0].Data.(core.FnDefInfo); !isFn || outs[0].Carrier {
		t.Errorf("the out is the member value itself, got %v", outs[0])
	}
	es.Compilable = false
	outs = []core.Value{core.NewDynamicCarrier(core.TAny)}
	if tryFoldParkedMemberFn(reg, "get", []core.Value{core.NewString("f"), m}, outs) {
		t.Error("an inactive recorder folds nothing")
	}
}

func TestArmsLeaveFnAndReadPoison(t *testing.T) {
	es := NewEmitState()
	lam := n207Lambda()
	named := core.NewFunction(core.FnDefInfo{Name: "one", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}})
	two := core.NewInteger(2)
	for name, c := range map[string]struct {
		b    core.BranchRecord
		want bool
	}{
		"a parking lambda arm":        {core.BranchRecord{ThenValue: &lam, ElsValue: &two}, true},
		"an arg-taking arm":           {core.BranchRecord{ElsStk: []core.Value{n207Lambda(core.FnParam{Type: core.TInteger})}}, true},
		"a body leaving a lambda":     {core.BranchRecord{ThenStk: []core.Value{lam}}, true},
		"a fn carrier arm":            {core.BranchRecord{ThenStk: []core.Value{core.NewCarrier(core.TFunction)}}, true},
		"a named 0-arg arm fires":     {core.BranchRecord{ThenValue: &named, ElsValue: &two}, false},
		"data arms":                   {core.BranchRecord{ThenValue: &two, ElsStk: []core.Value{core.NewInteger(3)}}, false},
		"an unproven gradual carrier": {core.BranchRecord{ThenStk: []core.Value{core.NewDynamicCarrier(core.TAny)}}, false},
	} {
		if got := es.armsLeaveFn(c.b); got != c.want {
			t.Errorf("%s: armsLeaveFn = %v, want %v", name, got, c.want)
		}
	}
	// A nested branch's merge is a carrier the inner flag proves.
	es.producedBy = map[string]producer{"inner": {seq: 4}}
	es.eventInfo[4] = eventFlags{armLeavesFn: true}
	inner := core.NewCarrier(core.TAny)
	inner.ID = "inner"
	if !es.armsLeaveFn(core.BranchRecord{ThenValue: &inner}) {
		t.Error("a nested branch that leaves a fn leaves it through the outer arm")
	}
	// The read of a def bound to such a result poisons the placement gate.
	es.noteMayBeFnRead("inner", "g")
	if es.armReadCompileFailure != "" {
		t.Fatal("a read of a value no def marked poisons nothing")
	}
	es.noteFnLeavingBind(inner)
	es.noteMayBeFnRead("inner", "g")
	if !strings.Contains(es.armReadCompileFailure, "read of `g`") || !strings.Contains(es.armReadCompileFailure, "NUR207") {
		t.Errorf("the read declines under NUR207, got %q", es.armReadCompileFailure)
	}
	es.armReadCompileFailure = ""
	es.noteMayBeFnRead("nobody", "g")
	if es.armReadCompileFailure != "" {
		t.Error("an unproduced read poisons nothing")
	}
}

func TestGradualHoldsFn(t *testing.T) {
	es := NewEmitState()
	// seq 7: a user call whose unit (1) returns a branch (seq 5) with a fn arm;
	// seq 9: a user call whose unit (0) builds the closure unit 2.
	es.fnRecs = []*fnUnitRec{
		{outOps: []EmitOperand{{kind: opClosure, closureUnit: 2}}},
		{outOps: []EmitOperand{{kind: opEvent, idx: 5}}},
		{nParams: 0},
	}
	es.frames[0] = append(es.frames[0],
		EmitEvent{seq: 7, kind: evCallUser, uc: emitUserCall{nout: 1, unit: 1}},
		EmitEvent{seq: 9, kind: evCallUser, uc: emitUserCall{nout: 1, unit: 0}})
	es.eventInfo[5] = eventFlags{armLeavesFn: true}
	es.producedBy = map[string]producer{"viaBranch": {seq: 7}, "viaClosure": {seq: 9}, "plain": {seq: 3}}
	mk := func(id string, t *core.Type) core.Value {
		v := core.NewCarrier(t)
		v.ID = id
		return v
	}
	if !es.gradualHoldsFn(mk("viaBranch", core.TAny)) || !es.gradualHoldsFn(mk("viaClosure", core.TAny)) {
		t.Error("an Any result proven to hold a fn")
	}
	quoted := mk("viaClosure", core.TAny)
	quoted.Quoted = true
	for name, v := range map[string]core.Value{
		"no producer": mk("plain", core.TAny), "no id": mk("", core.TAny), "quoted": quoted,
		"fn-typed (the strict paths own it)": mk("viaClosure", core.TFunction), "concrete": core.NewInteger(1),
	} {
		if es.gradualHoldsFn(v) {
			t.Errorf("%s: not a gradual fn holder", name)
		}
	}
	if es.eventLeavesFn(3, 9) {
		t.Error("the depth bound ends the walk")
	}
}

func TestMemberLambdaShape(t *testing.T) {
	s, ok := memberLambdaShape(n207Lambda(core.FnParam{Type: core.TInteger}, core.FnParam{Type: core.TString}))
	if !ok || s.Arity != 2 || s.Params[0] != core.TInteger || s.Params[1] != core.TString {
		t.Errorf("the member's one signature is the claim, got %+v/%v", s, ok)
	}
	pat := core.NewInteger(0)
	quoted := n207Lambda()
	quoted.Quoted = true
	twoSigs := n207Lambda()
	fd := twoSigs.Data.(core.FnDefInfo)
	fd.Signatures = append(fd.Signatures, fd.Signatures[0])
	twoSigs.Data = fd
	bodiless := core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{}}})
	for name, v := range map[string]core.Value{
		"a pattern param":  n207Lambda(core.FnParam{Type: core.TInteger, Pattern: &pat}),
		"an optional":      n207Lambda(core.FnParam{Type: core.TInteger, Optional: true}),
		"a /q param":       n207Lambda(core.FnParam{Type: core.TAtom, Quote: true}),
		"a named fn":       core.NewFunction(core.FnDefInfo{Name: "inc", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}}),
		"two overloads":    twoSigs,
		"no body":          bodiless,
		"quoted":           quoted,
		"not a fn":         core.NewInteger(1),
		"a fallback":       core.NewFunction(core.FnDefInfo{Anonymous: true, Signatures: []core.Signature{{Fallback: true}}}),
		"a module wrapper": core.NewFunction(core.FnDefInfo{Anonymous: true, Module: "M", Signatures: []core.Signature{{Impl: core.Boru([]core.Value{core.NewInteger(1)})}}}),
	} {
		if _, ok := memberLambdaShape(v); ok {
			t.Errorf("%s: claims no shape", name)
		}
	}
}

func TestNoteClosureShapeBindMemberClaim(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()
	read := core.NewDynamicCarrier(core.TAny)
	es.NoteMemberFnRead(read.ID, n207Lambda(core.FnParam{Type: core.TInteger}))
	es.noteClosureShapeBind(read)
	if s, ok := reg.Check.FnShapeOf(read.ID); !ok || s.Arity != 1 {
		t.Errorf("a def of a pinpointed member read claims the member's shape, got %+v/%v", s, ok)
	}
	unfit := core.NewDynamicCarrier(core.TAny)
	es.NoteMemberFnRead(unfit.ID, core.NewFunction(core.FnDefInfo{Name: "inc"}))
	es.noteClosureShapeBind(unfit)
	if _, ok := reg.Check.FnShapeOf(unfit.ID); ok {
		t.Error("a member the claim cannot describe claims nothing")
	}
	data := core.NewDynamicCarrier(core.TInteger)
	es.NoteMemberFnRead(data.ID, n207Lambda())
	es.noteClosureShapeBind(data)
	if _, ok := reg.Check.FnShapeOf(data.ID); ok {
		t.Error("a carrier whose bound excludes a fn claims nothing")
	}
}

func TestRecordUserCallDeclinesGradualFnArg(t *testing.T) {
	for _, c := range []struct {
		param   *core.Type
		decline bool
	}{{core.TAny, true}, {core.TFunction, false}} {
		es, _, done := beginRegionPass(t)
		arg := core.NewDynamicCarrier(core.TAny)
		unit, _, ok := es.StartFnCompile("g", "g", nil, []core.Value{arg}, []*core.Type{c.param}, []string{"h"}, nil, false, core.SrcPos{})
		if !ok {
			t.Fatal("StartFnCompile declined")
		}
		es.fnRecs[unit].paramTypes = []*core.Type{c.param}
		seq := es.appendEvent(EmitEvent{kind: evBranch, br: &emitBranch{}})
		es.setProduced(arg, seq)
		es.eventInfo[seq] = eventFlags{armLeavesFn: true}
		es.RecordUserCall(unit, "g", []core.Value{arg}, nil, core.SrcPos{Row: 1, Col: 3}, core.SrcPos{Row: 1, Col: 1})
		declined := !es.Compilable && strings.Contains(es.Reason, "parameter not typed Function (NUR207)")
		if declined != c.decline {
			t.Errorf("param %s: declined=%v (%q), want %v", c.param, declined, es.Reason, c.decline)
		}
		done()
	}
}

func TestApplyOverGradualLeadIsPending(t *testing.T) {
	es := NewEmitState()
	es.units = []*emitUnit{{}}
	lead := core.NewDynamicCarrier(core.TAny)
	lead.ID = "lead"
	es.producedBy = map[string]producer{"lead": {seq: 3}}
	es.eventInfo[3] = eventFlags{}
	sig := &core.Signature{Args: []*core.Type{core.TFunction}}
	if !es.recordCallElided("apply", sig, []core.Value{lead}, []core.Value{lead}, core.SrcPos{}) {
		t.Fatal("the apply over a structured producer's gradual lead is owned here")
	}
	if pa := es.units[0].pendingApply; len(pa) != 1 || pa[0].id != "lead" {
		t.Errorf("the pending apply is registered on the open unit, got %v", pa)
	}
	// A GENERIC producer's result is RecordCall's de-collision, not this arm.
	es.eventInfo[3] = eventFlags{generic: true}
	es.units[0].pendingApply = nil
	es.recordCallElided("apply", sig, []core.Value{lead}, []core.Value{lead}, core.SrcPos{})
	if len(es.units[0].pendingApply) != 0 {
		t.Error("a generic producer registers no pending apply")
	}
}

func TestNoteFnLeavingBind(t *testing.T) {
	es, reg, done := beginRegionPass(t)
	defer done()
	// seq 9: a user call whose unit (0) builds the closure unit 1; seq 7: a
	// user call whose unit (2) returns a branch (seq 5) with a fn arm.
	es.fnRecs = []*fnUnitRec{
		{outOps: []EmitOperand{{kind: opClosure, closureUnit: 1}}},
		{nParams: 0},
		{outOps: []EmitOperand{{kind: opEvent, idx: 5}}},
	}
	es.frames[0] = append(es.frames[0],
		EmitEvent{seq: 9, kind: evCallUser, uc: emitUserCall{nout: 1, unit: 0}},
		EmitEvent{seq: 7, kind: evCallUser, uc: emitUserCall{nout: 1, unit: 2}})
	es.eventInfo[5] = eventFlags{armLeavesFn: true}
	es.producedBy = map[string]producer{"claimed": {seq: 9}, "unclaimed": {seq: 7}, "fnTyped": {seq: 7}, "branch": {seq: 5}}
	carrier := func(id string, ty *core.Type) core.Value {
		v := core.NewDynamicCarrier(ty)
		v.ID = id
		return v
	}
	claimed := carrier("claimed", core.TAny)
	reg.Check.NoteFnShape(claimed, core.FnShape{Arity: 0})
	fnTyped := core.NewCarrier(core.TFunction)
	fnTyped.ID = "fnTyped"
	for _, v := range []core.Value{claimed, carrier("unclaimed", core.TAny), fnTyped, carrier("branch", core.TAny), {}} {
		es.noteFnLeavingBind(v)
	}
	want := map[string]bool{"unclaimed": true, "branch": true}
	for id := range es.fnLeavingBinds {
		if !want[id] {
			t.Errorf("%s: marked, but a claim or the fn-carrier paths model its reads", id)
		}
	}
	for id := range want {
		if !es.fnLeavingBinds[id] {
			t.Errorf("%s: a bound value read as data while it may hold a fn must be marked", id)
		}
	}
	if _, ok := NewEmitState().claimedShape("claimed"); ok {
		t.Error("no registry: no claim")
	}
}
