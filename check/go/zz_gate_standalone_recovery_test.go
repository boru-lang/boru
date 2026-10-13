package check

// Standalone-gate tests for check_recovery.go's remaining arms and the
// dyn-body forwarder in dispatch_hooks.go: the forward-drift guard's
// written-operand exemption (NUR362, driven end to end through a real
// forward collection), the const-fold's escaped-flow decline, the exported
// check-state share, the recovered poly's layout publication, the module
// native's poly owner, the dyn-body recovery, the single-overload user-fn
// recovery over an imprecise carrier and its barrier refusal, the compile
// pass's runtime-rematch exemption from no_signature, and the stranded
// type-call advisory.
//
// Armed answers come from the test doubles the recovery suite already uses
// (zzRecEmit / zzArmEmit, zzAssumeReg / zzAssumeCall in
// zz_cover_recovery_test.go) and from DispatchBraid / CheckBraid slot
// swaps restored with defer.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// gsWithID stamps a fresh value identity on v (a recorded operand needs
// one).
func gsWithID(v core.Value) core.Value {
	core.SetID(&v, core.GenerateID(core.IDPrefixForType(v.Parent)))
	return v
}

// DeclineForwardStackDrift's written-operand exemption (NUR362): a word
// whose OWN forward collection took its dynamic top from AFTER the word is
// re-stepped with every operand beneath it, and the literal after it is
// what the collection left there — the interpreter re-collects nothing, so
// the all-stack re-step is no drift. Driven end to end: `1 zgsg (zgsh) 7`
// over `zgsg [a:Integer | b:Integer]` with zgsh answering dynamic(Any). A
// spy around the installed guard records the re-step's forward split and
// the guard's verdict. The pairing form — the same operands already on the
// stack, no split — is TestZZCoverForwardStackDriftFailsToCompile's
// decline.
func TestGateStandaloneForwardDriftWrittenOperandsExempt(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsg", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 1,
			Returns: []*core.Type{core.TInteger}, Impl: zzGoInt(),
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsh", Signatures: []core.Signature{{
			Returns: []*core.Type{core.TAny}, BarrierPos: -1, Impl: zzGoInt(),
			ReturnsFn: func([]core.Value, *core.Registry) []core.Value {
				return []core.Value{core.NewDynamicCarrier(core.TAny)}
			},
		}}})
	})
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	prev := core.CheckBraid.DeclineForwardStackDrift
	splits, verdicts := []int{}, []bool{}
	core.CheckBraid.DeclineForwardStackDrift = func(e *core.Engine, sig *core.Signature, positions []int) bool {
		got := prev(e, sig, positions)
		if w, err := core.AsWord(e.Tape.Ref(e.Pointer)); err == nil && w.Name == "zgsg" {
			splits = append(splits, e.ForwardSplit())
			verdicts = append(verdicts, got)
		}
		return got
	}
	defer func() { core.CheckBraid.DeclineForwardStackDrift = prev }()

	out, err := core.NewTop(r).Run([]core.Value{
		core.NewInteger(1), core.NewWord("zgsg"),
		core.NewOpenParen(), core.NewWord("zgsh"), core.NewCloseParen(),
		core.NewInteger(7),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(splits) != 1 || splits[0] != 1 {
		t.Fatalf("the guard must be asked once, at the re-step of a 1-operand forward collection; splits = %v", splits)
	}
	if verdicts[0] {
		t.Error("a re-step over written operands is no drift: the guard must not decline")
	}
	if es.hasUncomp("forward operand accounting") {
		t.Errorf("no drift compile failure may be latched, marks = %v", es.uncomp)
	}
	if len(out) != 2 {
		t.Errorf("residual = %v, want zgsg's result and the trailing 7", out)
	}
}

// concreteEvalOnce declines a run whose break/continue ESCAPED the
// expression (a container member's run, NUR358): that is no constant, and
// the scratch run takes the signal so it stays the recorded run's to raise.
// A pure expression is the folding form it pairs with.
func TestGateStandaloneConcreteEvalEscapedFlow(t *testing.T) {
	r := xmlReg(t)
	done := r.Check.Begin()
	defer done()
	e := core.NewTop(r)

	if got, ok := concreteEvalOnce(e, []core.Value{core.NewWord("cadd"), core.NewInteger(1), core.NewInteger(2)}); !ok {
		t.Fatalf("a pure expression folds, got (%v, %v)", got, ok)
	}
	if _, ok := concreteEvalOnce(e, []core.Value{core.NewWord("cbreak")}); ok {
		t.Error("an escaping break is no constant")
	}
	if r.FlowCtrl != core.FlowNone {
		t.Errorf("the scratch run's escaped signal must be taken, FlowCtrl = %v", r.FlowCtrl)
	}
	if !r.Check.Mode {
		t.Error("check mode must be restored after the scratch run")
	}
}

// ShareCheckStateFrom is the exported share the compiler's cross-registry
// closure compile uses: while it holds, the owner registry runs on the
// caller's check state; the restore puts the owner's own back. A caller not
// in check mode shares nothing.
func TestGateStandaloneShareCheckStateFromExport(t *testing.T) {
	owner := newTestRegistry(t)
	caller := newTestRegistry(t)
	own := owner.Check

	idle := ShareCheckStateFrom(owner, caller)
	if owner.Check != own {
		t.Error("a caller outside check mode shares nothing")
	}
	idle()

	done := caller.Check.Begin()
	defer done()
	restore := ShareCheckStateFrom(owner, caller)
	if owner.Check != caller.Check {
		t.Fatal("the owner must run on the caller's check state")
	}
	restore()
	if owner.Check != own {
		t.Error("the restore must put the owner's own check state back")
	}
}

// gsProbe is a poly no-match probe double: no spec, a called word.
type gsProbe struct{}

func (gsProbe) Spec(*core.FnDefInfo, []core.Value) *core.PolyNoMatchSpec { return nil }
func (gsProbe) Uncalled() bool                                           { return false }

// recoverPoly publishes the window's exact layout for the record when a
// candidate's barrier stops its forward collection short of the written
// operands (core.BarrierBars, NUR362): the flat re-match is not the
// interpreter's there, so the layout rides to the record. A barrier that
// reaches every written operand publishes none. The publication is undone
// once the record returns.
func TestGateStandaloneRecoverPolyPublishesBarredLayout(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsbarq", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 1,
			Returns: []*core.Type{core.TInteger}, Impl: zzGoInt(),
		}}})
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsopenq", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 2,
			Returns: []*core.Type{core.TInteger}, Impl: zzGoInt(),
		}}})
	})
	done := r.Check.Begin()
	defer done()
	zzArmEmit(r)

	var layout *core.DispatchLayout
	prev := DispatchBraid.TryRecordPoly
	DispatchBraid.TryRecordPoly = func(reg *core.Registry, _ string, _ *core.Signature, args, _ []core.Value, _ core.SrcPos, _ bool, _ *core.Registry, _ bool, _ *core.PolyNoMatchSpec) bool {
		layout = reg.Check.LayoutFor(args)
		return true
	}
	defer func() { DispatchBraid.TryRecordPoly = prev }()

	record := func(word string) bool {
		layout = nil
		a1, a2 := gsWithID(core.NewInteger(1)), gsWithID(core.NewInteger(2))
		e := core.NewTop(r)
		e.Tape = core.NewTape([]core.Value{core.NewWord(word), a1, a2}, core.StackHeadroom)
		e.Pointer = 0
		fn := r.Lookup(word)
		outs := []core.Value{core.NewCarrier(core.TInteger)}
		return recoverPoly(e, gsProbe{}, core.WordInfo{Name: word, ArgCount: -1}, fn, &fn.Signatures[0],
			[]core.Value{a1, a2}, outs, []int{1, 2}, 0, core.SrcPos{Row: 1, Col: 1}, r, false)
	}

	if !record("zgsbarq") {
		t.Fatal("the armed poly record must take the barred window")
	}
	if layout == nil || layout.NFwd != 2 {
		t.Fatalf("a barred window's exact layout must ride to the record, got %+v", layout)
	}
	if r.Check.CurLayout != nil {
		t.Error("the layout publication must be undone once the record returns")
	}
	if !record("zgsopenq") {
		t.Fatal("the armed poly record must take the open window")
	}
	if layout != nil {
		t.Errorf("a barrier reaching every written operand publishes no layout, got %+v", layout)
	}
}

// recoveryPolyOwner: a module NATIVE reached from outside as a fn value has
// its home passed beside it, where the word IS a builtin — the poly re-match
// runs over that home. A foreign fn whose name is no builtin there (a rebound
// copy), and a fn with no home (core.FnHome resolves the dispatching
// registry), re-match over the dispatching registry.
func TestGateStandaloneRecoveryPolyOwnerForeignNative(t *testing.T) {
	home := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsnetq", Signatures: []core.Signature{{
			Args: []*core.Type{core.TInteger}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1, Impl: zzGoInt(),
		}}})
	})
	r := newTestRegistry(t)
	e := core.NewTop(r)

	native := &core.FnDefInfo{Name: "zgsnetq", Registry: home}
	nativeHome, _ := core.FnHome(r, native)
	if got := recoveryPolyOwner(e, core.WordInfo{Name: "zgsnetq"}, nativeHome); got != home {
		t.Error("a foreign module native re-matches over its home registry")
	}
	if got := recoveryPolyOwner(e, core.WordInfo{Name: "zgsrebound"}, nativeHome); got != r {
		t.Error("a name that is no builtin of the home re-matches over the dispatching registry")
	}
	unhomed, _ := core.FnHome(r, &core.FnDefInfo{Name: "zgsnetq"})
	if got := recoveryPolyOwner(e, core.WordInfo{Name: "zgsnetq"}, unhomed); got != r {
		t.Error("a fn with no home re-matches over the dispatching registry")
	}
}

// The dyn-body recovery (S1a): a CompileDynBody word over a STRICT-Any
// operand never reaches the poly recorder (a code-body word), but the
// dyn-body recorder re-matches the word's own overloads at run time — so
// the recovery offers it the WIDEST satisfiable window with its results
// computed suspended, and splices them when the recorder takes it. A
// declined offer falls through to the ordinary declines; a word without
// the body effect is never offered.
func TestGateStandaloneDynBodyRecovery(t *testing.T) {
	type offer struct {
		word string
		args []core.Value
		outs []core.Value
	}
	var offers []offer
	accept := true
	prev := DispatchBraid.TryRecordDynBody
	DispatchBraid.TryRecordDynBody = func(_ *core.Registry, word string, _ *core.Signature, args, outs []core.Value, _ core.SrcPos) bool {
		offers = append(offers, offer{word, args, outs})
		return accept
	}
	defer func() { DispatchBraid.TryRecordDynBody = prev }()

	dynSig := func(effect core.CompileEffect, callable *core.CallableSpec) core.Signature {
		return core.Signature{
			Args: []*core.Type{core.TList}, Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			Callable: callable, CompileEffect: effect, Impl: zzGoInt(),
		}
	}
	reg := func(t *testing.T) *core.Registry {
		return covRegistry(t, func(r *core.Registry) {
			r.RegisterNativeFunc(core.NativeFunc{Name: "zgsdynq", Signatures: []core.Signature{
				dynSig(core.CompileDynBody, &core.CallableSpec{BodyPos: 0, BodyOut: 1}),
			}})
			r.RegisterNativeFunc(core.NativeFunc{Name: "zgsplainq", Signatures: []core.Signature{
				dynSig(core.CompileDefault, nil),
			}})
		})
	}
	run := func(t *testing.T, word string) (*core.Engine, *zzRecEmit) {
		r := reg(t)
		done := r.Check.Begin()
		t.Cleanup(done)
		es := zzArmEmit(r)
		e, _ := zzAssumeCall(t, r, word, []core.Value{core.NewCarrier(core.TAny), core.NewWord(word)}, 1, nil)
		return e, es
	}

	e, es := run(t, "zgsdynq")
	if len(offers) != 1 || offers[0].word != "zgsdynq" || len(offers[0].args) != 1 || !isAnyCarrier(offers[0].args[0]) {
		t.Fatalf("the dyn-body recorder must be offered the Any-operand window once, got %+v", offers)
	}
	if len(es.uncomp) != 0 {
		t.Errorf("a taken dyn-body record declines nothing, marks = %v", es.uncomp)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Carrier || core.IDOf(e.Tape.Ref(0)) != core.IDOf(&offers[0].outs[0]) {
		t.Errorf("the recorded results must be spliced in the window's place, tape len %d", e.Tape.Len())
	}

	offers, accept = nil, false
	_, es = run(t, "zgsdynq")
	if len(offers) != 1 {
		t.Fatalf("the declined offer is still made once, got %d", len(offers))
	}
	if !es.hasUncomp("unmatched dispatch recovered at zgsdynq") {
		t.Errorf("a declined dyn-body offer falls through to the ordinary decline, marks = %v", es.uncomp)
	}

	offers, accept = nil, true
	run(t, "zgsplainq")
	if len(offers) != 0 {
		t.Errorf("a word without the body effect is never offered to the dyn-body recorder, got %+v", offers)
	}
}

// gsUserFnReg is zzAssumeReg plus two single-overload boru user fns with a
// ReturnsFn (the guarded-CALL_USER recoverable class): `zgsuserint` over a
// NOMINAL Integer param and `zgsuserpred` over a predicate-typed one.
func gsUserFnReg(t *testing.T) *core.Registry {
	t.Helper()
	r := zzAssumeReg(t)
	pred := covPredicateType(t, r, "ZgsUserPred")
	for name, pt := range map[string]*core.Type{"zgsuserint": core.TInteger, "zgsuserpred": pred} {
		r.RegisterNativeFunc(core.NativeFunc{Name: name, Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "x", Type: pt}},
			Returns:    []*core.Type{core.TInteger},
			ReturnsFn:  zzIntCarrierFn,
			Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("x"), core.NewWord("x"))),
			BarrierPos: -1,
		}}})
	}
	if err := r.Err(); err != nil {
		t.Fatalf("register: %v", err)
	}
	return r
}

func gsPickReturnsFn(s *core.Signature) bool { return s.ReturnsFn != nil && !s.Fallback }

// An IMPRECISE carrier (a String tag a narrowing settled on) into a
// single-overload user fn whose param is NOMINAL recovers as the guarded
// CALL_USER: the entry check asks exactly the interpreter's question, so
// the recovery splices the fn's results and declines nothing. A
// CONSTRAINED (predicate) param is a value-sensitive check the nominal
// guard cannot enforce: the same dispatch declines and is reported.
func TestGateStandaloneImpreciseCarrierNominalUserFn(t *testing.T) {
	r := gsUserFnReg(t)
	done := r.Check.Begin()
	defer done()
	es := zzArmEmit(r)

	e, _ := zzAssumeCall(t, r, "zgsuserint",
		[]core.Value{core.NewCarrier(core.TString), core.NewWord("zgsuserint")}, 1, gsPickReturnsFn)
	if len(es.uncomp) != 0 {
		t.Errorf("the nominal recovery must not decline, marks = %v", es.uncomp)
	}
	if out := e.Tape.At(0); e.Tape.Len() != 1 || !out.Carrier || !out.Parent.Equal(core.TInteger) {
		t.Errorf("the sole sig's results must be spliced, tape len %d", e.Tape.Len())
	}
	if zzHasDiag(r, "no_signature") {
		t.Error("a recovered nominal dispatch is not reported")
	}

	zzAssumeCall(t, r, "zgsuserpred",
		[]core.Value{core.NewCarrier(core.TString), core.NewWord("zgsuserpred")}, 1, gsPickReturnsFn)
	if !es.hasUncomp("unmatched dispatch recovered at zgsuserpred") {
		t.Errorf("a constrained param keeps the decline, marks = %v", es.uncomp)
	}
	if !zzHasDiag(r, "no_signature") {
		t.Error("the declined constrained dispatch is reported")
	}
}

// soleSigParamsNominal: only a single-overload user fn whose every param
// type is nominal qualifies. A multi-overload fn, a generic fn (its param
// is a type variable the generic lane binds) and a constrained param do
// not.
func TestGateStandaloneSoleSigParamsNominal(t *testing.T) {
	r := gsUserFnReg(t)
	pick := func(word string) (*core.Signature, *core.FnDefInfo) {
		fn := r.Lookup(word)
		for i := range fn.Signatures {
			if gsPickReturnsFn(&fn.Signatures[i]) {
				return &fn.Signatures[i], fn
			}
		}
		t.Fatalf("%s: no ReturnsFn signature", word)
		return nil, nil
	}

	sig, fn := pick("zgsuserint")
	if !soleSigParamsNominal(sig, fn) {
		t.Error("a nominal single-overload user fn qualifies")
	}
	generic := *fn
	generic.Gen = &core.GenSpecInfo{}
	if soleSigParamsNominal(sig, &generic) {
		t.Error("a generic fn has no nominal param to guard")
	}
	multi := *fn
	multi.Signatures = append(append([]core.Signature(nil), fn.Signatures...), *sig)
	if soleSigParamsNominal(sig, &multi) {
		t.Error("a multi-overload fn is not single-overload recoverable")
	}
	if sig, fn := pick("zgsuserpred"); soleSigParamsNominal(sig, fn) {
		t.Error("a constrained param is not nominal")
	}
}

// recoverUserFn refuses a window whose written operands reach past the
// signature's barrier (NUR362: `g (h) 7` over `g [a:Integer | b:Integer]`
// raises): that window is the recovery's shortfall fill, not the
// interpreter's collection, so no call is recorded and the dispatch cursor
// is left alone. The same signature over one written operand and one stack
// operand — within the barrier — records the guarded call.
func TestGateStandaloneRecoverUserFnBarrier(t *testing.T) {
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{Name: "zgsbaruser", Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TInteger}},
			Returns:    []*core.Type{core.TInteger},
			ReturnsFn:  zzIntCarrierFn,
			Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("a"), core.NewWord("b"))),
			BarrierPos: 1,
		}}})
	})
	done := r.Check.Begin()
	defer done()
	fn := r.Lookup("zgsbaruser")
	var sig *core.Signature
	for i := range fn.Signatures {
		if gsPickReturnsFn(&fn.Signatures[i]) {
			sig = &fn.Signatures[i]
		}
	}
	if sig == nil {
		t.Fatal("zgsbaruser: no ReturnsFn signature")
	}
	w := core.WordInfo{Name: "zgsbaruser", ArgCount: -1}
	pos := core.SrcPos{Row: 2, Col: 3}

	// Both operands written after the word: barred.
	one, two := core.NewInteger(1), core.NewInteger(2)
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewWord("zgsbaruser"), one, two}, core.StackHeadroom)
	e.Pointer = 0
	r.Check.CurCallWord = "zgsprior"
	if recoverUserFn(e, w, pos, sig, fn, []core.Value{one, two}, 0, []int{1, 2}) {
		t.Fatal("a window written past the barrier must not record a call")
	}
	if r.Check.CurCallWord != "zgsprior" || e.Tape.Len() != 3 {
		t.Errorf("a refused window leaves the cursor and the tape alone (cursor %q, tape len %d)", r.Check.CurCallWord, e.Tape.Len())
	}

	// One stack operand beneath, one written after: within the barrier.
	e = core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{one, core.NewWord("zgsbaruser"), two}, core.StackHeadroom)
	e.Pointer = 1
	if !recoverUserFn(e, w, pos, sig, fn, []core.Value{one, two}, 1, []int{0, 2}) {
		t.Fatal("a window within the barrier records the guarded call")
	}
	if r.Check.CurCallWord != "zgsbaruser" || r.Check.CurCallPos != pos {
		t.Errorf("the recovered call publishes its word cursor, got %q at %+v", r.Check.CurCallWord, r.Check.CurCallPos)
	}
	if e.Tape.Len() != 1 || !e.Tape.At(0).Parent.Equal(core.TInteger) {
		t.Errorf("the recovered call's results replace the window, tape len %d", e.Tape.Len())
	}
}

// On a COMPILE pass a dispatch the recorder compiles to a RUNTIME REMATCH
// is not a decided failure: the live values can match and the program then
// continues, so no no_signature mirror is reported for it. When the
// rematch declines and the program's recorder latches the decline, the
// same dispatch IS reported (as a runtime mirror).
func TestGateStandaloneRematchIsNoDecidedFailure(t *testing.T) {
	run := func(rematch bool) (*core.Registry, *zzRecEmit) {
		r := zzAssumeReg(t)
		done := r.Check.Begin()
		t.Cleanup(done)
		es := zzArmEmit(r)
		es.rematchOK = rematch
		r.Check.ProgramEmit = es
		r.Check.Compiling = true
		zzAssumeCall(t, r, "zzr-one", []core.Value{core.NewCarrier(core.TString), core.NewWord("zzr-one")}, 1, nil)
		return r, es
	}

	r, es := run(true)
	if !es.rematched || len(es.uncomp) != 0 {
		t.Fatalf("the imprecise dispatch must compile to the rematch (rematched %v, marks %v)", es.rematched, es.uncomp)
	}
	if zzHasDiag(r, "no_signature") {
		t.Error("a runtime rematch is no decided failure: nothing reported")
	}

	r, es = run(false)
	if !es.hasUncomp("unmatched dispatch recovered at zzr-one") {
		t.Fatalf("a declined rematch latches the decline, marks = %v", es.uncomp)
	}
	var mirror bool
	for _, d := range r.Check.Diagnostics {
		if d.Code == "no_signature" && d.RuntimeMirror {
			mirror = true
		}
	}
	if !mirror {
		t.Error("the decided decline is reported as a runtime-mirror no_signature")
	}
}

// gsFnTypeNode mints a type node whose declared content is a FUNCTION value
// — what `def <Capitalised> <fn body>` binds.
func gsFnTypeNode(r *core.Registry, name string) core.Value {
	node := r.Types.MintType(name, core.TFunction)
	node.SetTypeBody(core.NewFunction(core.FnDefInfo{Signatures: []core.Signature{{
		Params:  []core.FnParam{{Name: "x", Type: core.TInteger}},
		Returns: []*core.Type{core.TBoolean},
		Impl:    core.Boru([]core.Value{core.NewBoolean(true)}),
	}}}))
	return core.NewTypeLiteral(node)
}

// noteStrandedTypeCall (§5.1): a node whose declared content is a fn,
// immediately followed by a non-type value, is the forward-form call `I 5`
// that never ran — reported once per source defect, under the written name
// (the token's source spelling, else the node's own name). A node stranded
// last, a node whose content is no fn, and two adjacent nodes are quiet.
func TestGateStandaloneStrandedTypeCall(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	e := core.NewTop(r)
	node := gsFnTypeNode(r, "Zgsi")
	five := core.NewInteger(5)

	noteStrandedTypeCall(e, []core.Value{node, five})
	noteStrandedTypeCall(e, []core.Value{node, five}) // a re-analysed body surfaces it again
	var words []string
	for _, d := range r.Check.Diagnostics {
		if d.Code == "stranded_type_call" {
			words = append(words, d.Word)
		}
	}
	if len(words) != 1 || words[0] != node.String() {
		t.Fatalf("stranded_type_call = %v, want one under the node's name %q", words, node.String())
	}

	alias := node
	alias.SetPos(core.SrcPos{Row: 4, Col: 1, Src: "Zgsj"})
	noteStrandedTypeCall(e, []core.Value{alias, five})
	if d := r.Check.Diagnostics[len(r.Check.Diagnostics)-1]; d.Code != "stranded_type_call" || d.Word != "Zgsj" ||
		!strings.Contains(d.Detail, "'Zgsj' names a type") {
		t.Errorf("an aliased spelling is reported as written, got %+v", d)
	}

	before := len(r.Check.Diagnostics)
	plain := r.Types.MintType("ZgsAlias", core.TInteger)
	plain.SetTypeBody(core.NewTypeLiteral(core.TInteger))
	for name, residual := range map[string][]core.Value{
		"stranded last":      {five, node},
		"content is no fn":   {core.NewTypeLiteral(plain), five},
		"builtin type":       {core.NewTypeLiteral(core.TInteger), five},
		"two adjacent nodes": {node, core.NewTypeLiteral(core.TInteger)},
	} {
		noteStrandedTypeCall(e, residual)
		if len(r.Check.Diagnostics) != before {
			t.Errorf("%s: must stay quiet", name)
			before = len(r.Check.Diagnostics)
		}
	}
}
