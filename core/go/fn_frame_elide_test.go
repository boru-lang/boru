package core

import "testing"

// Synthetic-tape tests for the shell-variant elision mechanism: the
// scanned tail's effects must equal the parked marker's effects (the
// truncation, the state pops, the teardown), the marker must vanish, and
// the shell — the frame's open paren, ReturnCheck, and close paren — must
// survive.

func TestElideTailFrameRunsTeardownAndKeepsShell(t *testing.T) {
	f := newProbeFixture(t)

	// Frame entry state, as buildFnBodyHandler would have pushed it:
	// a baseline, an args list, and the param binding the tail undefs.
	f.r.PushFnBaseline(f.r.Defs.Snapshot())
	if err := f.r.Args.Push(NewList([]Value{NewInteger(1)})); err != nil {
		t.Fatal(err)
	}
	InstallDef(f.r, "n", NewInteger(1))
	// A body-local def ABOVE the snapshot the __DC marker carries —
	// eager DefCleanup must truncate it exactly like the parked one.
	snap := f.r.Defs.Snapshot()
	InstallDef(f.r, "loc", NewInteger(9))
	dc := frameMarker(f.r, snap, "n")

	// (ₘ 1 f __DC __RC )
	tokens := []Value{
		NewFrameOpen(f.meta), NewInteger(1), NewWord("f"),
		dc, f.rc, NewCloseParen(),
	}
	e := NewTop(f.r)
	e.Tape = NewTape(tokens, 8)
	e.Pointer = 2

	scan, ok := e.probeTailCall([]int{1}, 1)
	if !ok {
		t.Fatal("probe declined the canonical shape")
	}
	if err := e.elideTailFrame(scan); err != nil {
		t.Fatalf("elideTailFrame: %v", err)
	}

	// State: all three per-call effects happened.
	if f.r.TopFnBaseline() != nil {
		t.Error("FnBaseline not popped")
	}
	if _, ok, _ := f.r.Args.Top(); ok {
		t.Error("Args entry not popped")
	}
	if _, bound := f.r.Defs.Top("loc"); bound {
		t.Error("body-local def not truncated by the eager DefCleanup")
	}
	if _, bound := f.r.Defs.Top("n"); bound {
		t.Error("param binding not torn down by the eager teardown")
	}

	// Tape: marker gone, shell intact — (ₘ 1 f __RC )
	isInt := func(v Value) bool { return v.Parent.Equal(TInteger) && IsConcrete(v) }
	want := []func(Value) bool{IsFrameOpen, isInt, IsWord, IsReturnCheck, IsCloseParen}
	if e.Tape.Len() != len(want) {
		t.Fatalf("tape has %d tokens after elision, want %d", e.Tape.Len(), len(want))
	}
	for i, pred := range want {
		if !pred(e.Tape.At(i)) {
			t.Errorf("tape[%d] = %v, wrong token class", i, e.Tape.At(i))
		}
	}
	if e.Pointer != 2 {
		t.Errorf("pointer moved to %d; ahead-of-pointer edits must not move it", e.Pointer)
	}
}

func TestElideTailFrameNoReturnCheck(t *testing.T) {
	f := newProbeFixture(t)
	f.r.PushFnBaseline(f.r.Defs.Snapshot())
	if err := f.r.Args.Push(NewList(nil)); err != nil {
		t.Fatal(err)
	}
	dc := frameMarker(f.r, f.r.Defs.Snapshot())

	// (ₘ f __DC )  — no declared returns, no params.
	tokens := []Value{
		NewFrameOpen(f.meta), NewWord("f"), dc, NewCloseParen(),
	}
	e := NewTop(f.r)
	e.Tape = NewTape(tokens, 8)
	e.Pointer = 1

	scan, ok := e.probeTailCall(nil, 0)
	if !ok {
		t.Fatal("probe declined")
	}
	if err := e.elideTailFrame(scan); err != nil {
		t.Fatal(err)
	}
	// Shell without RC: (ₘ f )
	if e.Tape.Len() != 3 || !IsFrameOpen(e.Tape.At(0)) || !IsWord(e.Tape.At(1)) || !IsCloseParen(e.Tape.At(2)) {
		t.Fatalf("tape after elision = %d tokens, want (ₘ f )", e.Tape.Len())
	}
}

func TestTCOEligibleGates(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)
	// Every scan needs a real DefCleanup token for the coverage walk;
	// the snapshot is taken with no bindings installed, so truncation
	// is empty unless a case installs one.
	snap := f.r.Defs.Snapshot()
	e.Tape = NewTape([]Value{NewDefCleanup(DefCleanupInfo{Snapshot: snap, Registry: f.r})}, 4)
	base := frameTailScan{Meta: f.meta, TailStart: 0}
	sig := &Signature{Impl: &BoruImpl{FnFrame: f.meta}}
	muts := f.r.Defs.Mutations()

	if !e.tcoEligible(base, sig, muts) {
		t.Fatal("baseline scan must be eligible")
	}
	// Kill switch.
	f.r.TCO.Disable = true
	if e.tcoEligible(base, sig, muts) {
		t.Error("Disable must gate elision off")
	}
	f.r.TCO.Disable = false
	// A different overload's frame (the mutual shape) is eligible
	// since Stage 4b — which tape treatment it gets is decided by
	// ValuesBelow + returnsConform, not here.
	other := frameTailScan{Meta: &FnFrameMeta{Name: "g"}, TailStart: 0}
	if !e.tcoEligible(other, sig, muts) {
		t.Error("a different overload's frame must be eligible (mutual tail calls, Stage 4b)")
	}
	// Generic fns decline — caller frame or callee, either side.
	genMeta := &FnFrameMeta{Name: "h", HasGen: true}
	if e.tcoEligible(frameTailScan{Meta: genMeta, TailStart: 0}, sig, muts) {
		t.Error("a generic enclosing frame must decline")
	}
	if e.tcoEligible(base, &Signature{Impl: &BoruImpl{FnFrame: genMeta}}, muts) {
		t.Error("a generic callee must decline")
	}
	// Binding mutation during arg auto-eval.
	if e.tcoEligible(base, sig, muts-1) {
		t.Error("binding mutations during auto-eval must decline")
	}
	// A name's spelling no longer matters: the marker tears every name
	// down the same way (UninstallFrameBinding), so a builtin-shadowing
	// param the callee rebinds is eligible. (The stepped `undef name`
	// tail used to send such names down undef's reserved-word arm, and
	// the gate declined them to match it.)
	shadowScan := base
	shadowScan.Names = []string{"add"}
	if f.r.builtinWords == nil {
		f.r.builtinWords = map[string]bool{}
	}
	f.r.builtinWords["add"] = true
	shadowSig := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{Name: "f", InstallNames: []string{"add"}}}}
	if !e.tcoEligible(shadowScan, shadowSig, muts) {
		t.Error("a builtin-shadowing teardown name the callee rebinds must be eligible")
	}
}

func TestTCOEligibleNameCoverage(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)
	snap := f.r.Defs.Snapshot()
	e.Tape = NewTape([]Value{NewDefCleanup(DefCleanupInfo{Snapshot: snap, Registry: f.r})}, 4)
	muts := f.r.Defs.Mutations()

	// A torn-down param the callee rebinds is covered…
	scan := frameTailScan{Meta: f.meta, TailStart: 0, Names: []string{"n"}}
	rebinds := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{Name: "g", InstallNames: []string{"n"}}}}
	if !e.tcoEligible(scan, rebinds, muts) {
		t.Error("a param the callee rebinds must be eligible")
	}
	// …but a callee that does NOT rebind it must decline: a dynamic
	// read of the caller's param anywhere in the callee chain would
	// see undefined where nesting shows the live binding.
	noRebind := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{Name: "g"}}}
	if e.tcoEligible(scan, noRebind, muts) {
		t.Error("a torn-down param the callee does not rebind must decline")
	}

	// Body-local bindings above the frame's snapshot: same rule via
	// the DefCleanup truncation walk. The recursive-local-fn idiom and
	// loop-carried base-branch reads depend on these staying live.
	InstallDef(f.r, "tmp", NewInteger(1))
	muts = f.r.Defs.Mutations()
	clean := frameTailScan{Meta: f.meta, TailStart: 0}
	if e.tcoEligible(clean, noRebind, muts) {
		t.Error("a body-local the callee does not rebind must decline")
	}
	rebindsTmp := &Signature{Impl: &BoruImpl{FnFrame: &FnFrameMeta{Name: "g", InstallNames: []string{"tmp"}}}}
	if !e.tcoEligible(clean, rebindsTmp, muts) {
		t.Error("a body-local the callee rebinds must be eligible")
	}
}

func TestTCOEligibleDeclinesForeignRegistryFrames(t *testing.T) {
	// A frame whose DefCleanup carries a DIFFERENT registry than the
	// executing engine's (a handler closed over a module sub-registry
	// splicing onto this tape) must decline: the eager teardown pops
	// e.registry's per-call stacks, which such a frame never pushed.
	f := newProbeFixture(t)
	e := NewTop(f.r)
	foreign, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e.Tape = NewTape([]Value{NewDefCleanup(DefCleanupInfo{Snapshot: foreign.Defs.Snapshot(), Registry: foreign})}, 4)
	scan := frameTailScan{Meta: f.meta, TailStart: 0}
	if e.tcoEligible(scan, &Signature{Impl: &BoruImpl{FnFrame: f.meta}}, f.r.Defs.Mutations()) {
		t.Error("a foreign-registry frame must decline")
	}
}

func TestReturnsConform(t *testing.T) {
	f := newProbeFixture(t)
	e := NewTop(f.r)

	// Helper: an engine whose tape holds one ReturnCheck with the
	// given caller returns; scan.RCIdx points at it.
	scanWithCallerRC := func(returns []*Type) frameTailScan {
		rc := NewReturnCheck(ReturnCheckInfo{FuncName: "caller", Returns: returns})
		e.Tape = NewTape([]Value{rc}, 4)
		return frameTailScan{RCIdx: 0}
	}

	cases := []struct {
		name   string
		caller []*Type // nil = caller declares no returns (RCIdx -1)
		callee []*Type
		want   bool
	}{
		{"caller unchecked admits anything", nil, nil, true},
		{"caller unchecked, callee checked", nil, []*Type{TInteger}, true},
		{"identical returns", []*Type{TInteger}, []*Type{TInteger}, true},
		{"widening callee (Integer into Number)", []*Type{TNumber}, []*Type{TInteger}, true},
		{"narrowing callee (Number into Integer)", []*Type{TInteger}, []*Type{TNumber}, false},
		{"checked caller, unchecked callee", []*Type{TInteger}, nil, false},
		{"count mismatch", []*Type{TInteger}, []*Type{TInteger, TInteger}, false},
		{"anonymous placeholder Any against stricter caller", []*Type{TInteger}, []*Type{TAny}, false},
		{"Any caller accepts Any callee", []*Type{TAny}, []*Type{TAny}, true},
	}
	for _, tc := range cases {
		var scan frameTailScan
		if tc.caller == nil {
			scan = frameTailScan{RCIdx: -1}
		} else {
			scan = scanWithCallerRC(tc.caller)
		}
		sig := &Signature{Returns: tc.callee}
		if got := e.returnsConform(scan, sig); got != tc.want {
			t.Errorf("%s: returnsConform = %v, want %v", tc.name, got, tc.want)
		}
	}
}
