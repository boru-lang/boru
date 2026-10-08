package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// stored_fn_proof_test.go pins the strict store-fn slot's payload proof
// (stored_fn_proof.go), the dyn-body backstop over a gradual stored fn
// (recordStoredFnDyn), and the run-time def dispatch (NoteRuntimeDefDispatch)
// — the 2026-09-26 mechanisms behind the sweep's `behave` × container and
// `fnsig` × module-export cells.

// storedFnBody is a capture-free fn value whose one signature runs body.
func storedFnBody(body ...core.Value) core.Value {
	return fnVal(core.Signature{Params: []core.FnParam{{Name: "t"}}, Impl: &core.BoruImpl{Body: body}})
}

// constMapWith is a const Map holding member under key.
func constMapWith(key string, member core.Value) core.Value {
	om := core.NewOrderedMap()
	om.Set(key, member)
	return core.NewMap(om)
}

func TestProvenFnDefConstAndLocal(t *testing.T) {
	es := NewEmitState()
	fn := storedFnBody(core.NewString("T"))
	fi := es.internUnpooled(fn)
	ii := es.internUnpooled(core.NewInteger(3))
	if !es.strictFnOperandProven(fn, ConstOperand(fi)) {
		t.Error("a baked const fn is an interpreter fn value")
	}
	if es.strictFnOperandProven(fn, ConstOperand(ii)) {
		t.Error("a const Integer is not a fn value")
	}
	if es.strictFnOperandProven(fn, ConstOperand(99)) {
		t.Error("an out-of-range const index proves nothing")
	}
	// A frame local proves only through its producing event.
	carrier := core.NewCarrier(core.TFunction)
	if es.strictFnOperandProven(carrier, localOperand(0)) {
		t.Error("a local with no producer (a param) is unproven")
	}
	es.producedBy[carrier.ID] = producer{seq: 7, idx: 1}
	if es.strictFnOperandProven(carrier, localOperand(0)) {
		t.Error("a local holding a non-first result is unproven")
	}
	es.producedBy[carrier.ID] = producer{seq: 7}
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{ConstOperand(fi)}}}
	es.frames[0] = []EmitEvent{{seq: 7, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}}}
	if !es.strictFnOperandProven(carrier, localOperand(0)) {
		t.Error("a promoted def of a factory's baked const fn is proven")
	}
	// Other operand kinds — a dynamic-scope read, a closure — never prove.
	if es.strictFnOperandProven(carrier, dynScopeOperand(fi)) {
		t.Error("a dynamic-scope read is unproven")
	}
	if es.strictFnOperandProven(carrier, EmitOperand{kind: opClosure}) {
		t.Error("a compiled closure is exactly what the proof refuses")
	}
}

func TestProvenFnDefUserCall(t *testing.T) {
	es := NewEmitState()
	fi := es.internUnpooled(storedFnBody(core.NewString("T")))
	es.fnRecs = []*fnUnitRec{
		{outOps: []EmitOperand{ConstOperand(fi)}},
		{outOps: []EmitOperand{{kind: opClosure, closureUnit: 0}}},
		{outOps: []EmitOperand{EventOperand(1, 0)}},
	}
	es.frames[0] = []EmitEvent{
		{seq: 1, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}},
		{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1, poly: &emitUserPolySpec{}}},
		{seq: 3, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 2}},
		{seq: 4, kind: evCallUser, uc: emitUserCall{unit: 1, nout: 1}},
		{seq: 5, kind: evCallUser, uc: emitUserCall{unit: 2, nout: 1}},
		{seq: 6, kind: evBranch},
	}
	v := core.NewCarrier(core.TFunction)
	for _, c := range []struct {
		name string
		op   EmitOperand
		want bool
	}{
		{"a factory returning a baked const fn", EventOperand(1, 0), true},
		{"a non-first result", EventOperand(1, 1), false},
		{"a poly user call", EventOperand(2, 0), false},
		{"a multi-result user call", EventOperand(3, 0), false},
		{"a factory returning a compiled closure", EventOperand(4, 0), false},
		{"a factory returning another call's result", EventOperand(5, 0), false},
		{"an event kind that yields no fn", EventOperand(6, 0), false},
		{"an event outside every frame", EventOperand(42, 0), false},
	} {
		if got := es.strictFnOperandProven(v, c.op); got != c.want {
			t.Errorf("%s: proven = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestProvenFnDefConstMemberRead(t *testing.T) {
	es := NewEmitState()
	fn := storedFnBody(core.NewString("T"))
	m := es.internUnpooled(constMapWith("c", fn))
	mi := es.internUnpooled(constMapWith("c", core.NewInteger(1)))
	key := es.internUnpooled(core.NewAtom("c"))
	read := func(seq int, c emitCall) EmitEvent { return EmitEvent{seq: seq, kind: evCall, call: c} }
	es.frames[0] = []EmitEvent{
		read(1, emitCall{word: "dot", nout: 1, ops: []EmitOperand{ConstOperand(m), ConstOperand(key)}}),
		read(2, emitCall{word: "size", nout: 1, ops: []EmitOperand{ConstOperand(m)}}),
		read(3, emitCall{word: "dot", nout: 1, ops: []EmitOperand{localOperand(0), ConstOperand(key)}}),
		read(4, emitCall{word: "dot", nout: 1, ops: []EmitOperand{ConstOperand(99), ConstOperand(key)}}),
		read(5, emitCall{word: "dot", nout: 1, ops: []EmitOperand{ConstOperand(mi), ConstOperand(key)}}),
		read(6, emitCall{word: "dot", nout: 1, dynApply: 1, ops: []EmitOperand{ConstOperand(m), ConstOperand(key)}}),
		read(7, emitCall{word: "get", nout: 2, ops: []EmitOperand{ConstOperand(m), ConstOperand(key)}}),
	}
	v := core.NewDynamicCarrier(core.TAny)
	for _, c := range []struct {
		name string
		seq  int
		want bool
	}{
		{"a member read over a const container", 1, true},
		{"a non-accessor word", 2, false},
		{"a receiver that is not a const", 3, false},
		{"an out-of-range const receiver", 4, false},
		{"a member that is no fn", 5, false},
		{"an apply riding the read", 6, false},
		{"a multi-result read", 7, false},
	} {
		if got := es.strictFnOperandProven(v, EventOperand(c.seq, 0)); got != c.want {
			t.Errorf("%s: proven = %v, want %v", c.name, got, c.want)
		}
	}
	// A carrier member (a factory's result stored in a map the pass holds
	// concretely) is no const fn: the proof needs the payload itself.
	carrierMember := es.internUnpooled(constMapWith("c", core.NewCarrier(core.TFunction)))
	es.frames[0] = append(es.frames[0], read(8, emitCall{word: "dot", nout: 1, ops: []EmitOperand{ConstOperand(carrierMember), ConstOperand(key)}}))
	if es.strictFnOperandProven(v, EventOperand(8, 0)) {
		t.Error("a carrier member is unproven")
	}
}

func TestStoredFnNeedsDynEnv(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{Args: []*core.Type{core.TAtom, core.TFunction}}
	atom := es.internUnpooled(core.NewAtom("canon"))
	data := es.internUnpooled(storedFnBody(core.NewString("T")))
	named := es.internUnpooled(storedFnBody(core.NewWord("k")))
	args := []core.Value{core.NewAtom("canon"), core.NewCarrier(core.TFunction)}
	if es.storedFnNeedsDynEnv(sig, args, []EmitOperand{ConstOperand(atom), ConstOperand(data)}) {
		t.Error("a pure-data stored body resolves no name: no DynEnv")
	}
	if !es.storedFnNeedsDynEnv(sig, args, []EmitOperand{ConstOperand(atom), ConstOperand(named)}) {
		t.Error("a stored body naming `k` needs the DynEnv mirror")
	}
	if !es.storedFnNeedsDynEnv(sig, args, []EmitOperand{ConstOperand(atom), localOperand(0)}) {
		t.Error("an unproven stored fn needs the DynEnv mirror")
	}
}

// storedFnSig is behave's atom form, as the recorder reads it.
func storedFnSig() *core.Signature {
	return &core.Signature{
		Args:          []*core.Type{core.TAtom, core.TFunction},
		QuoteArgs:     map[int]bool{0: true},
		CompileEffect: core.CompileStoresFn | core.CompileQuoteInert | core.CompileDynBody | core.CompileFnHandlerStrict,
	}
}

func TestRecordStoredFnDyn(t *testing.T) {
	// memberRead seats a const-member read of fn and returns its gradual
	// result, as the check pass hands it to behave.
	memberRead := func(es *EmitState, fn core.Value) core.Value {
		m := es.internUnpooled(constMapWith("c", fn))
		key := es.internUnpooled(core.NewAtom("c"))
		seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "dot", nout: 1, ops: []EmitOperand{ConstOperand(m), ConstOperand(key)}}})
		v := core.NewDynamicCarrier(core.TAny)
		es.producedBy[v.ID] = producer{seq: seq}
		return v
	}
	name := core.NewAtom("canon")

	es := NewEmitState()
	v := memberRead(es, storedFnBody(core.NewString("T")))
	if !recordStoredFnDyn(es, "behave", storedFnSig(), []core.Value{name, v}, nil, core.SrcPos{}) {
		t.Fatal("a proven gradual member fn records")
	}
	last := es.frames[0][len(es.frames[0])-1]
	if last.kind != evCall || last.call.word != "behave" || !last.call.poly || len(last.call.ops) != 2 {
		t.Errorf("recorded event = %+v, want a behave poly re-match over two operands", last)
	}
	if es.dynEnv {
		t.Error("a pure-data stored body must not arm DynEnv")
	}

	es = NewEmitState()
	v = memberRead(es, storedFnBody(core.NewWord("k")))
	if !recordStoredFnDyn(es, "behave", storedFnSig(), []core.Value{name, v}, nil, core.SrcPos{}) || !es.dynEnv {
		t.Error("a stored body that names something records under DynEnv")
	}

	// The declines, each leaving the event stream untouched.
	type decline struct {
		name string
		sig  *core.Signature
		args func(es *EmitState) []core.Value
		outs []core.Value
	}
	gradual := func(es *EmitState) []core.Value {
		return []core.Value{name, memberRead(es, storedFnBody(core.NewString("T")))}
	}
	withSig := func(mut func(s *core.Signature)) *core.Signature {
		s := storedFnSig()
		mut(s)
		return s
	}
	for _, c := range []decline{
		{"a code-body slot", withSig(func(s *core.Signature) { s.NoEvalArgs = map[int]bool{0: true} }), gradual, nil},
		{"a re-stepped result", withSig(func(s *core.Signature) { s.CompileEffect |= core.CompileResteps }), gradual, nil},
		{"a result-bearing word", storedFnSig(), gradual, []core.Value{core.NewInteger(1)}},
		{"two Function slots", withSig(func(s *core.Signature) { s.Args = []*core.Type{core.TFunction, core.TFunction} }), gradual, nil},
		{"no Function slot", withSig(func(s *core.Signature) { s.Args = []*core.Type{core.TAtom, core.TAny} }), gradual, nil},
		{"a concrete fn operand", storedFnSig(), func(*EmitState) []core.Value {
			return []core.Value{name, storedFnBody(core.NewString("T"))}
		}, nil},
		{"a strict (non-gradual) carrier", storedFnSig(), func(*EmitState) []core.Value {
			return []core.Value{name, core.NewCarrier(core.TFunction)}
		}, nil},
		{"a quoted operand the word does not declare inert", withSig(func(s *core.Signature) { s.CompileEffect &^= core.CompileQuoteInert }), gradual, nil},
		{"a quoted operand that is no inert const", storedFnSig(), func(es *EmitState) []core.Value {
			return []core.Value{core.NewCarrier(core.TAtom), memberRead(es, storedFnBody(core.NewString("T")))}
		}, nil},
		{"an operand of unknown provenance", storedFnSig(), func(*EmitState) []core.Value {
			return []core.Value{name, core.NewDynamicCarrier(core.TAny)}
		}, nil},
		{"an unproven payload (a flex member, a closure)", storedFnSig(), func(es *EmitState) []core.Value {
			seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "dot", nout: 1, ops: []EmitOperand{localOperand(0), localOperand(1)}}})
			v := core.NewDynamicCarrier(core.TAny)
			es.producedBy[v.ID] = producer{seq: seq}
			return []core.Value{name, v}
		}, nil},
	} {
		es := NewEmitState()
		args := c.args(es)
		before := len(es.frames[0])
		if recordStoredFnDyn(es, "behave", c.sig, args, c.outs, core.SrcPos{}) {
			t.Errorf("%s: recorded, want a decline", c.name)
			continue
		}
		if len(es.frames[0]) != before || es.dynEnv {
			t.Errorf("%s: a decline must leave the recorder untouched", c.name)
		}
	}
}

func TestStrictStoreSlotRequiresProvenFn(t *testing.T) {
	es := NewEmitState()
	sig := &core.Signature{Args: []*core.Type{core.TFunction}, CompileEffect: core.CompileStoresFn | core.CompileFnHandlerStrict}
	// A factory's typed Function result whose unit returns a compiled
	// closure: the handler would be handed a ClosurePayload.
	carrier := core.NewCarrier(core.TFunction)
	carrier.ID = core.GenerateID(core.IDPrefixForType(core.TFunction))
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{{kind: opClosure}}}}
	seq := es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	es.producedBy[carrier.ID] = producer{seq: seq}
	// A fn-typed result is a type body: its event says it produced one.
	es.eventInfo[seq] = eventFlags{typeOut: true}
	if _, ok := es.RecordCallOperands("compose", sig, []core.Value{carrier}); ok {
		t.Fatal("an unproven fn operand at a strict store slot must decline")
	}
	if es.Compilable || !strings.Contains(es.Reason, "not proven an interpreter fn value") {
		t.Errorf("decline reason = %q", es.Reason)
	}
	// The same slot over a factory's baked const fn records.
	es = NewEmitState()
	fi := es.internUnpooled(storedFnBody(core.NewString("T")))
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{ConstOperand(fi)}}}
	seq = es.appendEvent(EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}})
	es.producedBy[carrier.ID] = producer{seq: seq}
	es.eventInfo[seq] = eventFlags{typeOut: true}
	if ops, ok := es.RecordCallOperands("compose", sig, []core.Value{carrier}); !ok || len(ops) != 1 {
		t.Errorf("a proven fn operand records: ok=%v ops=%+v reason=%q", ok, ops, es.Reason)
	}
	if strictFnSlot(sig, 3) {
		t.Error("a position past the signature is no Function slot")
	}
}

func TestRecordCallArmsDynEnvForANamingStoredBody(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TFunction}, CompileEffect: core.CompileStoresFn | core.CompileDynBody}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es := NewEmitState()
	es.reg = r
	es.RecordCall("keep", sig, []core.Value{storedFnBody(core.NewWord("k"))}, nil, core.SrcPos{}, false, false)
	if !es.Compilable || !es.dynEnv {
		t.Errorf("a stored body naming `k` records under DynEnv (compilable=%v reason=%q dynEnv=%v)", es.Compilable, es.Reason, es.dynEnv)
	}
	es = NewEmitState()
	es.reg = r
	es.RecordCall("keep", sig, []core.Value{storedFnBody(core.NewString("T"))}, nil, core.SrcPos{}, false, false)
	if !es.Compilable || es.dynEnv {
		t.Errorf("a pure-data stored body records without DynEnv (compilable=%v reason=%q dynEnv=%v)", es.Compilable, es.Reason, es.dynEnv)
	}
}

func TestNoteRuntimeDefDispatch(t *testing.T) {
	// Inactive: nothing latches.
	es := NewEmitState()
	es.Compilable = false
	es.NoteRuntimeDefDispatch("T")
	if es.pendingRuntimeBindCall || len(es.runtimeDefNames) != 0 {
		t.Error("an inactive recorder latches nothing")
	}
	// Inside a compiled unit: the install would outlive the frame, so the
	// dispatch declines as the compile-time word it is.
	es = NewEmitState()
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	es.NoteRuntimeDefDispatch("T")
	if !es.pendingRuntimeBindCall || !es.runtimeDefInUnit || len(es.runtimeDefNames) != 0 {
		t.Errorf("a unit-scoped run-time def latches its decline: pending=%v inUnit=%v", es.pendingRuntimeBindCall, es.runtimeDefInUnit)
	}
	defSig := &core.Signature{Args: []*core.Type{core.TAtom}, Impl: core.Go(nil, core.RunInCheck())}
	es.RecordRuntimeDispatch("def", defSig, []core.Value{core.NewAtom("T")}, nil, core.SrcPos{})
	if es.Compilable || !strings.Contains(es.Reason, "compile-time word def") || es.pendingRuntimeBindCall || es.runtimeDefInUnit {
		t.Errorf("a unit-scoped run-time def must decline, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
	// At the root: the latch is armed and the name kept for Finalize.
	es = NewEmitState()
	es.NoteRuntimeDefDispatch("Zq")
	if !es.pendingRuntimeBindCall || len(es.runtimeDefNames) != 1 || es.dynEnv {
		t.Errorf("a root run-time def arms the dispatch latch alone: pending=%v names=%v dynEnv=%v", es.pendingRuntimeBindCall, es.runtimeDefNames, es.dynEnv)
	}
	if reason, blocked := es.finalizeBlocked(); blocked {
		t.Errorf("an unknown name part does not block: %q", reason)
	}
	// A part the pass registered after the def blocks the program.
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	es.reg = r
	r.RegisterPart("Zq")
	if reason, blocked := es.finalizeBlocked(); !blocked || !strings.Contains(reason, "name part") {
		t.Errorf("a later-registered part must block Finalize, got %q %v", reason, blocked)
	}
}

// TestTryRecordDynBodyRoutesAStoredFn pins the dyn-body backstop's store-fn
// arm: a CompileStoresFn word with no CallableSpec (behave) takes
// recordStoredFnDyn, never the clause-list or code-body arms — a strict
// carrier there declines, as it did before the arm existed.
func TestTryRecordDynBodyRoutesAStoredFn(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	done := r.Check.BeginCompilePass()
	defer done()
	if tryRecordDynBody(r, "behave", storedFnSig(), []core.Value{core.NewAtom("canon"), core.NewCarrier(core.TFunction)}, nil, core.SrcPos{}) {
		t.Error("a strict Function carrier is not the gradual operand the arm records")
	}
}

// TestStrictNativeFnResultProven pins the native-producer arm of the proof
// (Codex P2 on PR #511): a strict fn-handler native whose one declared result
// is a Function mints a Go-built FnDefInfo, so its result is proven at the
// next strict slot — as an event operand and through a promoted local — but
// its body is unknown, so provenFnDef still declines it (DynEnv arms).
func TestStrictNativeFnResultProven(t *testing.T) {
	es := NewEmitState()
	strict := &core.Signature{Returns: []*core.Type{core.TFunction}, CompileEffect: core.CompileFnHandlerStrict}
	plain := &core.Signature{Returns: []*core.Type{core.TFunction}}
	anyRet := &core.Signature{Returns: []*core.Type{core.TAny}, CompileEffect: core.CompileFnHandlerStrict}
	twoRet := &core.Signature{Returns: []*core.Type{core.TFunction, core.TFunction}, CompileEffect: core.CompileFnHandlerStrict}
	nilRet := &core.Signature{Returns: []*core.Type{nil}, CompileEffect: core.CompileFnHandlerStrict}
	call := func(seq int, c emitCall) EmitEvent { return EmitEvent{seq: seq, kind: evCall, call: c} }
	es.frames[0] = []EmitEvent{
		call(1, emitCall{word: "compose", sig: strict, nout: 1}),
		call(2, emitCall{word: "compose", sig: plain, nout: 1}),
		call(3, emitCall{word: "valof", sig: anyRet, nout: 1}),
		call(4, emitCall{word: "compose", sig: twoRet, nout: 1}),
		call(5, emitCall{word: "compose", sig: nilRet, nout: 1}),
		call(6, emitCall{word: "compose", nout: 1}),
		call(7, emitCall{word: "compose", sig: strict, nout: 2}),
		call(8, emitCall{word: "compose", sig: strict, nout: 1, dynApply: 1}),
		call(9, emitCall{word: "compose", sig: strict, nout: 1, live: true}),
		{seq: 10, kind: evCallUser, uc: emitUserCall{nout: 1}},
	}
	v := core.NewDynamicCarrier(core.TFunction)
	for _, c := range []struct {
		name string
		seq  int
		want bool
	}{
		{"a strict native with one Function result", 1, true},
		{"a native that is not strict", 2, false},
		{"a strict native returning Any", 3, false},
		{"two declared results", 4, false},
		{"an undeclared result type", 5, false},
		{"no signature", 6, false},
		{"a multi-result call", 7, false},
		{"an apply riding the call", 8, false},
		{"a live lookup", 9, false},
		{"a user call is not a native", 10, false},
		{"no such event", 99, false},
	} {
		if got := es.strictFnOperandProven(v, EventOperand(c.seq, 0)); got != c.want {
			t.Errorf("%s: proven = %v, want %v", c.name, got, c.want)
		}
	}
	if es.strictFnOperandProven(v, EventOperand(1, 1)) {
		t.Error("a non-first result is unproven")
	}
	if _, ok := es.provenFnDef(v, EventOperand(1, 0)); ok {
		t.Error("the wrapper's body is unknown: provenFnDef must decline it")
	}
	es.producedBy[v.ID] = producer{seq: 1}
	if !es.strictFnOperandProven(v, localOperand(0)) {
		t.Error("a promoted def of the native's result is proven")
	}
	es.producedBy[v.ID] = producer{seq: 1, idx: 1}
	if es.strictFnOperandProven(v, localOperand(0)) {
		t.Error("a promoted def of a non-first result is unproven")
	}
	delete(es.producedBy, v.ID)
	if es.strictFnOperandProven(v, localOperand(0)) {
		t.Error("a local with no producer is unproven")
	}
}

// TestValueRefsNameInterpolation pins the interpolation arms (Codex P1 on
// PR #511): a `${expr}` hole resolves names at run time, so an interpolated
// string or XML template counts as naming.
func TestValueRefsNameInterpolation(t *testing.T) {
	if !valueRefsName(core.Value{Data: core.InterpStringPayload{}}) {
		t.Error("an interpolated string counts as naming")
	}
	if !valueRefsName(core.Value{Data: core.XmlInterpPayload{}}) {
		t.Error("an XML template counts as naming")
	}
	if valueRefsName(core.NewString("plain")) {
		t.Error("a plain string names nothing")
	}
}
