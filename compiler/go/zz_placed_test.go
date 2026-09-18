package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestS6aDynOutNativeOKFnValuedArgDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := &core.Signature{Args: []*core.Type{core.TFunction}}
	args := []core.Value{core.NewInteger(1)}
	outs := []core.Value{core.NewDynamicCarrier(core.TAny)}
	if dynOutNativeOK(r, "s6adyn", sig, args, outs) {
		t.Error("a Function-typed arg slot must refuse the dyn-out bake")
	}
}

// --- isModuleInnerSig ---------------------------------------------------------

func TestS6aTryRecordPolyQuotedNonGetDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6apolyq",
		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyq").Signatures[0]
	if tryRecordPoly(r, "s6apolyq", sig, []core.Value{core.NewAtom("k")}, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("an UNDECLARED quoted-operand word must not poly")
	}
}

// TestS6aTryRecordPolyQuotedDeclarationAdmits is the POSITIVE twin of the
// decline test above, and the pair now turns on a DECLARATION rather than a
// word's name. `set`/`del` used to be admitted by setDelKernelSig (NUR057),
// a by-name key; their quoted-receiver overloads declare CompileQuoteKey
// instead, so the gate reads a contract about the operand. The two tests are
// therefore the same signature shape under two arbitrary names: the one that
// declares records the poly, the one that does not declines.
//
// The flag is CompileQuoteKey and not CompileQuoteInert on purpose. Reading
// the wider quoteOperandInertOK here admits every CompileQuoteInert declarer,
// `raise` among them, and a poly-recorded `raise` loses its divergence (the
// poly event carries no sig) — measured, and the reason this gate tests the
// narrow declaration.
func TestS6aTryRecordPolyQuotedDeclarationAdmits(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6apolyd",
		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
			CompileEffect: core.CompileQuoteKey,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyd").Signatures[0]
	if !tryRecordPoly(r, "s6apolyd", sig, []core.Value{core.NewAtom("k")}, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("a sig DECLARING CompileQuoteKey must poly")
	}
}

// TestS6aTryRecordPolyQuotedDelNameAloneDeclines is the negative that the
// old by-name key could not express: an identically-shaped sig registered
// under the name `del` but carrying NO declaration must decline. The name
// is not the contract, and a regression that restored a name test would
// pass the admit test above while failing here.
func TestS6aTryRecordPolyQuotedDelNameAloneDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "del",
		Signatures: []core.Signature{{
			Args:      []*core.Type{core.TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("del").Signatures[0]
	if tryRecordPoly(r, "del", sig, []core.Value{core.NewAtom("k")}, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("the name `del` alone must no longer admit — the declaration is the key")
	}
}

func TestS6aTryRecordPolyFnValuedArgDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6apolyf",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TFunction},
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}),
			Returns: []*core.Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyf").Signatures[0]
	if tryRecordPoly(r, "s6apolyf", sig, []core.Value{core.NewInteger(1)}, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("a Function-typed arg slot must not poly")
	}
}

func TestS6aTryRecordPolyReservedLiteralNoBinding(t *testing.T) {
	// "true" is IsBuiltinWord (reserved literal) in every registry but has
	// no Lookup binding — the fn == nil arm.
	r := newTestRegistry(t)
	armEmit(r)
	if tryRecordPoly(r, "true", &core.Signature{}, nil, []core.Value{}, core.SrcPos{}, true, nil, false, nil) {
		t.Error("a reserved literal with no fn binding must not poly")
	}
}

// --- TryRecordFallback ---------------------------------------------------------

func TestS6aTryRecordFallbackForeignSigDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	registerIslandWord(t, r, "s6afb0", core.CompileFallbackBody, 1, -1)
	foreign := &core.Signature{CompileEffect: core.CompileFallbackBody}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	if TryRecordFallback(r, "s6afb0", foreign, []core.Value{core.NewInteger(1)}, outs, core.SrcPos{}) {
		t.Error("a sig that is not the word's registered binding must decline")
	}
}

func TestS6aTryRecordFallbackTypeNodeAfterThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb1", core.CompileFallbackBody, 2, -1)
	args := []core.Value{core.NewCarrier(core.TInteger), core.NewTypeLiteral(core.TInteger)}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	if TryRecordFallback(r, "s6afb1", sig, args, outs, core.SrcPos{}) {
		t.Error("a type operand after a threaded arg must decline")
	}
}

func TestS6aTryRecordFallbackBakedAfterThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb2", core.CompileFallbackBody, 2, -1)
	args := []core.Value{core.NewCarrier(core.TInteger), core.NewInteger(5)}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	if TryRecordFallback(r, "s6afb2", sig, args, outs, core.SrcPos{}) {
		t.Error("a baked arg after a threaded arg must decline")
	}
}

func TestS6aTryRecordFallbackTwoThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb3", core.CompileFallbackBody, 2, -1)
	args := []core.Value{core.NewCarrier(core.TInteger), core.NewCarrier(core.TString)}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	if TryRecordFallback(r, "s6afb3", sig, args, outs, core.SrcPos{}) {
		t.Error("two threaded args must decline (multi-thread islands unsupported)")
	}
}

func TestS6aTryRecordFallbackBarrierClamp(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb4", core.CompileFallbackBody, 1, -1)
	sig.BarrierPos = 99 // out-of-range barrier clamps to TotalArgs
	args := []core.Value{core.NewInteger(5)}
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	// All-baked, fallback-body word: forward-eligible constraint holds
	// (1 baked arg <= clamped barrier 1) — the attempt proceeds past the
	// clamp; whether the recorder accepts is not the point here.
	TryRecordFallback(r, "s6afb4", sig, args, outs, core.SrcPos{})
	if sig.BarrierPos != 99 {
		t.Error("the clamp must be local, not written back to the sig")
	}
}

// A CONCRETE fn-valued arg (FnDefInfo in .Data) under a NON-Function sig slot
// reaches the args-data guard (past the ArgTypes-Function check) and declines
// the dyn-out bake / the poly record.
func TestS6aDynOutNativeOKFnValueDataDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := &core.Signature{Args: []*core.Type{core.TAny}}
	args := []core.Value{core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "s6afn"})}
	outs := []core.Value{core.NewDynamicCarrier(core.TAny)}
	if dynOutNativeOK(r, "s6adynv", sig, args, outs) {
		t.Error("a concrete fn-valued arg must refuse the dyn-out bake")
	}
}

func TestS6aTryRecordPolyFnValueDataDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s6apolyv",
		Signatures: []core.Signature{
			{Args: []*core.Type{core.TAny}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
			{Args: []*core.Type{core.TAny, core.TAny}, Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return nil, nil
			}), Returns: []*core.Type{core.TAny}, BarrierPos: -1},
		},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyv").Signatures[0]
	args := []core.Value{core.NewValueRaw(core.TFunction, core.FnDefInfo{Name: "s6afn2"})}
	if tryRecordPoly(r, "s6apolyv", sig, args, []core.Value{core.NewDynamicCarrier(core.TAny)}, core.SrcPos{}, false, nil, false, nil) {
		t.Error("a concrete fn-valued arg must not poly")
	}
}

func TestDriftWindowVariadicOperandDeclines(t *testing.T) {
	// The dynamic top is produced by a VARIADIC event — a fixed-width window
	// cannot carry it, so the fence declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	dyn := core.NewDynamicCarrier(core.TAny)
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzvar", nout: 1}})
	es.setProducedAt(dyn, seq, 0)
	f := es.eventInfo[seq]
	f.variadicResult = true
	es.eventInfo[seq] = f
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(5), dyn, core.NewWord("add"), core.NewInteger(1)}, core.StackHeadroom)
	e.Pointer = 2
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("a variadic-event operand must decline the window")
	}
}

func TestDriftWindowUnresolvableOperandDeclines(t *testing.T) {
	// A window operand with no compiled home (a plain carrier: no producing
	// event, no local, no const materialisation) — resolveOperand declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	dyn := core.NewDynamicCarrier(core.TAny)
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zzev", nout: 1}})
	es.setProducedAt(dyn, seq, 0)
	orphan := core.NewCarrier(core.TInteger) // non-dynamic carrier, no event, no orig
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{orphan, dyn, core.NewWord("add"), core.NewInteger(1)}, core.StackHeadroom)
	e.Pointer = 2
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("an unresolvable window operand must decline the window")
	}
}

func TestRecordCallRefusalComputedRangeList(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)
	lst := core.NewCarrier(core.TList)
	lst.ID = core.GenerateID(core.IDPrefixForType(core.TList))
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: wordMakeList, nout: 1, makeList: true}})
	es.setProduced(lst, seq)
	if !es.recordCallRefusal("for", &core.Signature{}, []core.Value{lst}, nil, core.SrcPos{}, false, false) {
		t.Fatal("the computed-range-list arm must classify the refusal")
	}
	if es.Compilable || !containsStr(es.Reason, "computed range list") {
		t.Errorf("want the computed-range-list reason, got compilable=%v reason=%q", es.Compilable, es.Reason)
	}
}

// SplitEventRegionBind's guard-owned decline arms (S9.1), white-box: the
// parked-fn screen (a Function-typed idx-0 result — the spilled rest would
// auto-apply it in the interpreter) and eventBySeq's closed-frame miss (a
// producing seq recorded in a fragment frame that has since closed).
func TestSplitEventRegionBindDeclines(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	es := r.Check.Emit.(*EmitState)
	es.BindRegistry(r)

	// A 2-out call event whose idx-0 result is Function-typed.
	fnOut := core.NewCarrier(core.TFunction)
	fnOut.ID = core.GenerateID(core.IDPrefixForType(core.TFunction))
	other := core.NewCarrier(core.TString)
	other.ID = core.GenerateID(core.IDPrefixForType(core.TString))
	seq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zz", nout: 2}})
	es.setProduced(fnOut, seq)
	es.producedBy[other.ID] = producer{seq: seq, idx: 1}
	if _, ok := es.SplitEventRegionBind("zzf", fnOut); ok {
		t.Error("a Function-typed first result must decline (parked-fn screen)")
	}

	// A seq living in a CLOSED fragment frame: record the event inside a
	// fragment, close it, then ask for the split — eventBySeq misses.
	closeFrag := es.beginFragment()
	iOut := core.NewCarrier(core.TInteger)
	iOut.ID = core.GenerateID(core.IDPrefixForType(core.TInteger))
	fseq := es.appendEvent(EmitEvent{kind: evCall, call: emitCall{word: "zz2", nout: 2}})
	es.setProduced(iOut, fseq)
	closeFrag()
	es.TakeFragment()
	if _, ok := es.SplitEventRegionBind("zzg", iOut); ok {
		t.Error("a closed-fragment producer must decline (eventBySeq miss)")
	}
}

func TestRecordDispatchOutcomeFnLocalFnRefusal(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	sig := bodySig(core.CompileFallbackBody, nil)
	recordDispatchOutcome(r, "for-each", sig, bodyArgs("step"), nil, core.SrcPos{}, r)
	if es.Compilable {
		t.Fatal("fn-local fn body word must latch the program uncompilable")
	}
	if !strings.Contains(es.Reason, "fn-local fn `step`") ||
		!strings.Contains(es.Reason, "for-each") {
		t.Errorf("refusal reason = %q; want the fn-local-fn reason naming step and for-each", es.Reason)
	}
}

func TestRecordDispatchOutcomeFnLocalFnAlreadyProducedSkips(t *testing.T) {
	// A dispatch a structured ReturnsFn hook already recorded (case's
	// branch-chain desugar) lowered its clause bodies as inline events —
	// not a leak path; the guard must not refuse it.
	r := newTestRegistry(t)
	es := armEmit(r)
	fnLocalBind(r, "step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	out := core.NewInteger(7)
	es.producedBy[out.ID] = producer{}
	sig := bodySig(core.CompileFallbackBody, nil)
	recordDispatchOutcome(r, "case", sig, bodyArgs("step"), []core.Value{out}, core.SrcPos{}, r)
	if strings.Contains(es.Reason, "fn-local fn") {
		t.Errorf("already-produced dispatch hit the fn-local-fn refusal: %q", es.Reason)
	}
}

func TestRecordDispatchOutcomeModuleScopeNotFnLocalRefused(t *testing.T) {
	// The module-scope twin of the refusal test: whatever else the
	// recording chain decides, the fn-local-fn reason must not fire.
	r := newTestRegistry(t)
	es := armEmit(r)
	r.Defs.Push("step", core.NewFunction(core.FnDefInfo{Name: "step"}))
	sig := bodySig(core.CompileFallbackBody, nil)
	recordDispatchOutcome(r, "for-each", sig, bodyArgs("step"), nil, core.SrcPos{}, r)
	if strings.Contains(es.Reason, "fn-local fn") {
		t.Errorf("module-scope callback hit the fn-local-fn refusal: %q", es.Reason)
	}
}

func TestS6aTryFoldModuleConstNondeterministicDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	// A real Ideal/Module descriptor — isModuleFamilyValue now keys on
	// the kernel-declared TModule (Equal, not the retired string-path
	// probe), so a minted look-alike no longer reaches the fold.
	modVal := core.NewModuleInstance(core.ModuleDesc{ID: "s6am"})
	sig := statefulSig(core.CompileModuleFold, func(n int) []core.Value {
		return []core.Value{core.NewInteger(int64(n))}
	})
	outs := []core.Value{core.NewCarrier(core.TInteger)}
	if tryFoldModuleConst(r, "s6amfold", sig, []core.Value{modVal}, outs) {
		t.Error("a nondeterministic module read must not const-fold")
	}
}

// --- dynOutNativeOK -----------------------------------------------------------

func TestDriftWindowOutOfRangePosition(t *testing.T) {
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(1)}, core.StackHeadroom)
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{0, 999}) {
		t.Error("an out-of-range matched position must decline the window")
	}
}

func TestDriftWindowNonContiguousDeclines(t *testing.T) {
	// Matched operands not directly under the word (a bystander token between
	// the top operand and the word) — the contiguity fence declines.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	dyn := core.NewDynamicCarrier(core.TAny)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(5), dyn, core.NewInteger(9), core.NewWord("add"), core.NewInteger(1)}, core.StackHeadroom)
	e.Pointer = 3
	w, sig := zzDriftWord()
	if tryRecordDriftWindow(e, w, sig, []int{1, 0}) {
		t.Error("a non-contiguous matched span must decline the window")
	}
}
