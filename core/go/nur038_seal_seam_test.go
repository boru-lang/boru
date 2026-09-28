package core

import "testing"

// Seam coverage for the NUR038 statement-seal helpers: the arrival
// gate's call-vs-data classifiers. The end-to-end behaviour is pinned in
// lang/spec/fn-value.tsv §6 and lang/go/test/nur038_seal_test.go; these
// drive the classifier arms directly.

func TestNur038ReachFnWouldClaim(t *testing.T) {
	r := covRegistry(t, nil)
	r.Defs.Push("nsval", NewInteger(7))
	r.Defs.Push("nsfn", Value{Parent: TFunction, Data: FnDefInfo{Name: "nsfn"}})
	e := NewTop(r)
	anyFn := Value{Parent: TFunction, Data: FnDefInfo{Name: "a", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: -1},
	}}}
	intFn := Value{Parent: TFunction, Data: FnDefInfo{Name: "i", Signatures: []Signature{
		{Args: []*Type{TInteger}, BarrierPos: -1},
	}}}
	fnFn := Value{Parent: TFunction, Data: FnDefInfo{Name: "hof", Signatures: []Signature{
		{Args: []*Type{TFunction}, BarrierPos: -1},
	}}}
	stackOnly := Value{Parent: TFunction, Data: FnDefInfo{Name: "s", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: 0},
	}}}
	cases := []struct {
		name string
		fn   Value
		tok  Value
		want bool
	}{
		{"Any slot claims a literal", anyFn, NewInteger(5), true},
		{"typed slot claims a fitting literal", intFn, NewInteger(5), true},
		{"typed slot declines a misfit", intFn, NewList([]Value{NewInteger(99)}), false},
		{"end is a boundary", anyFn, NewEnd(), false},
		{"close paren is a boundary", anyFn, NewCloseParen(), false},
		{"forward marker is a boundary", anyFn, NewForward(ForwardInfo{}), false},
		{"plain value binding claims", anyFn, NewWord("nsval"), true},
		{"typed slot tests the binding", intFn, NewWord("nsval"), true},
		{"fn word is a barrier", anyFn, NewWord("nsfn"), false},
		{"/v word claims at a Function slot", fnFn, NewWordRef("nsfn"), true},
		{"reserved literal claims", anyFn, NewWord("true"), true},
		{"unbound word is no claim", anyFn, NewWord("ns-no-such"), false},
		{"a group is an optimistic claim", intFn, NewOpenParen(), true},
		{"a stack-only sig claims nothing forward", stackOnly, NewInteger(5), false},
		{"a sig-less value claims nothing", Value{Parent: TFunction, Data: FnDefInfo{Name: "z"}}, NewInteger(5), false},
		{"none is a reserved literal, like true", anyFn, NewWord("none"), true},
		{"a typed slot declines none", intFn, NewWord("none"), false},
		{"a 0-arg-only fn claims nothing, even a group", Value{Parent: TFunction, Data: FnDefInfo{Name: "p0", Signatures: []Signature{
			{BarrierPos: -1},
		}}}, NewOpenParen(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.Tape = NewTape([]Value{c.tok}, StackHeadroom)
			if got := e.ReachFnWouldClaim(c.fn, 0); got != c.want {
				t.Errorf("claim = %v, want %v", got, c.want)
			}
		})
	}
	// Past the end of the tape: nothing to claim.
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	if e.ReachFnWouldClaim(anyFn, 5) {
		t.Error("out-of-range index must not claim")
	}
}

func TestNur038FnValueWouldWiden(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	m := NewOrderedMap()
	m.Set("sep", NewString(","))
	optsMap := NewMap(m)
	// The concat shape: a 1-arg [List] overload plus a 2-arg [Map List]
	// overload whose MAP rides at sig[0] (the swap slot) — the completed
	// list redistributes to the stack side when the wider sig claims.
	concatLike := Value{Parent: TFunction, Data: FnDefInfo{Name: "c", Signatures: []Signature{
		{Args: []*Type{TMap, TList}, BarrierPos: -1},
		{Args: []*Type{TList}, BarrierPos: -1},
	}}}
	barriered := Value{Parent: TFunction, Data: FnDefInfo{Name: "b", Signatures: []Signature{
		{Args: []*Type{TInteger, TMap}, BarrierPos: 1},
	}}}
	cases := []struct {
		name      string
		fn        Value
		completed int
		tok       Value
		want      bool
	}{
		{"a wider sig claims the next token at its swap slot", concatLike, 1, optsMap, true},
		{"no sig wider than the completed count", concatLike, 2, optsMap, false},
		{"a token fitting no wider slot widens nothing", concatLike, 1, NewInteger(5), false},
		{"slots past the barrier are not forward-eligible", barriered, 1, optsMap, false},
		{"a statement boundary widens nothing", concatLike, 1, NewEnd(), false},
		{"a group is an optimistic widen", concatLike, 1, NewOpenParen(), true},
		// The PR-review P1 (the lambda twin-call CI failure): an
		// optimistic probe must NOT report widening when no overload is
		// wider than the completed count — `m.l 5 m.l 7` completes the
		// 1-arg lambda with the second REACH as the next token, and
		// skipping the seal there re-opened the swallow.
		{"an optimistic probe without a wider sig widens nothing", concatLike, 2, NewOpenParen(), false},
		{"a non-fn value widens nothing", NewInteger(1), 0, NewInteger(5), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e.Tape = NewTape([]Value{c.tok}, StackHeadroom)
			if got := e.fnValueWouldWiden(c.fn, c.completed, 0); got != c.want {
				t.Errorf("widen = %v, want %v", got, c.want)
			}
		})
	}
	// Past the end of the tape: nothing to widen into.
	e.Tape = NewTape([]Value{NewInteger(1)}, StackHeadroom)
	if e.fnValueWouldWiden(concatLike, 1, 5) {
		t.Error("out-of-range index must not widen")
	}
}

// arrivalGateEngine builds the mid-collection arrival state: a word
// callee with one collected arg beneath it, its Forward parked, and a
// reach-tagged 1-arg fn value at the pointer with a claimable token
// after it — the arm-3 shape (`w 1 <m.p> 7` at the moment the collapse
// arrives). The plan barriers normally close this window at plan time;
// the arrival gate is the RUNTIME safety net for windows planned before
// the collapse existed (the two phases can drift —
// design/FORWARD-COLLECTION-PHASES.10.md), so the seam drives the state
// directly.
func arrivalGateEngine(t *testing.T, callee string, sigs []Signature) *Engine {
	t.Helper()
	r := covRegistry(t, func(r *Registry) {
		InstallFnDef(r, callee, FnDefInfo{Name: callee, Signatures: sigs})
	})
	e := NewTop(r)
	tagged := Value{Parent: TFunction, Data: FnDefInfo{Name: "p", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: -1},
	}}}
	tagged.ReachGroup = true
	sig2 := &Signature{Args: []*Type{TAny, TAny}, BarrierPos: -1}
	e.Tape = NewTape([]Value{
		NewInteger(1),
		NewWord(callee),
		NewForward(ForwardInfo{FuncName: callee, FuncIndex: 1,
			CollectedArgs: 1, ExpectedArgs: 2, StackArgs: 0, Sig: sig2}),
		tagged,
		NewInteger(7),
	}, StackHeadroom)
	e.Pointer = 3
	return e
}

func TestNur038ArrivalGateCommitsAtCallHead(t *testing.T) {
	// The parked word has a 1-arg overload: the arriving call head
	// commits the collection with the arg it already claimed — the same
	// dispatch an explicit `end` before the dot-access would trigger.
	oneBody := Boru([]Value{NewWord("a")})
	e := arrivalGateEngine(t, "wboth", []Signature{
		{Params: []FnParam{{Name: "a", Type: TAny}}, Returns: []*Type{TAny},
			Impl: oneBody, BarrierPos: BarrierAllForward},
		{Params: []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
			Returns: []*Type{TAny}, Impl: oneBody, BarrierPos: BarrierAllForward},
	})
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	for i := 0; i < e.Tape.Len(); i++ {
		if IsForward(e.Tape.At(i)) {
			t.Fatal("the pending forward must be COMMITTED by the arriving call head")
		}
	}
}

func TestNur038ArrivalGateFallsBackToImplicitEnd(t *testing.T) {
	// The parked word has ONLY the 2-arg overload — nothing can commit
	// with one claimed arg, so the window resolves via implicitEnd (the
	// mismatch path) instead of swallowing the call head as data.
	e := arrivalGateEngine(t, "wtwo", []Signature{
		{Params: []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
			Returns: []*Type{TAny}, Impl: Boru([]Value{NewWord("a")}),
			BarrierPos: BarrierAllForward},
	})
	if err := e.stepLiteral(); err != nil {
		t.Fatalf("stepLiteral: %v", err)
	}
	// The window is resolved (forward removed), and the call head
	// survives at its own position AFTER the word — the next dispatch,
	// never collected data beneath it.
	for i := 0; i < e.Tape.Len(); i++ {
		if IsForward(e.Tape.At(i)) {
			t.Fatal("implicitEnd must resolve the pending forward")
		}
	}
	if !isFnDefValue(e.Tape.At(2)) {
		t.Fatalf("tape[2] = %v, want the surviving call head", e.Tape.At(2))
	}
}

func TestNur038SealClearedAtRunEntry(t *testing.T) {
	// The seal is per-run scratch: a prior run that armed it on its
	// final allowed step (eval_limit before the callee's re-step) must
	// not leak into the next program on a reused engine.
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.sealFnValue, e.sealFnValueIdx = true, 0
	res, err := e.Run([]Value{NewInteger(1)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("residual = %v, want [1]", res)
	}
	if e.sealFnValue {
		t.Error("a stale seal must be cleared at Run entry")
	}
}

func TestNur038PolyReachBoundStopsAtCallHead(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	tagged := Value{Parent: TFunction, Data: FnDefInfo{Name: "p", Signatures: []Signature{
		{Args: []*Type{TAny}, BarrierPos: -1},
	}}}
	tagged.ReachGroup = true

	// A tagged fn with a claim is the NEXT call — the scan stops before
	// it, exactly as a bare fn word would stop it.
	e.Tape = NewTape([]Value{NewInteger(0), tagged, NewInteger(7)}, StackHeadroom)
	e.Pointer = 0
	if n, ok := e.polyReachBound(); !ok || n != 0 {
		t.Errorf("bound = (%d,%v), want (0,true): the call head is a barrier", n, ok)
	}

	// The SAME value with nothing to claim is an operand and counts.
	e.Tape = NewTape([]Value{NewInteger(0), tagged, NewEnd()}, StackHeadroom)
	if n, ok := e.polyReachBound(); !ok || n != 1 {
		t.Errorf("bound = (%d,%v), want (1,true): a claim-less fn is data", n, ok)
	}
}

func TestNur038FnValueSigShapes(t *testing.T) {
	zero := Signature{}
	one := Signature{Args: []*Type{TAny}}
	fb := Signature{Fallback: true}

	// fnValueHasZeroArgSig: a real 0-arg sig counts, a Fallback does not,
	// and a non-fn value has none.
	if !fnValueHasZeroArgSig(Value{Parent: TFunction, Data: FnDefInfo{Name: "z", Signatures: []Signature{zero, one}}}) {
		t.Error("a real 0-arg overload must count")
	}
	if fnValueHasZeroArgSig(Value{Parent: TFunction, Data: FnDefInfo{Name: "f", Signatures: []Signature{one, fb}}}) {
		t.Error("a Fallback catch-all is not a property-read overload")
	}
	if fnValueHasZeroArgSig(NewInteger(1)) {
		t.Error("a non-fn value has no sigs")
	}

	// fnValueOnlyZeroArgSigs: all-0-arg is a property fn; any arg-taking
	// sig, a sig-less value, or a fallback-only value is not.
	if !FnValueOnlyZeroArgSigs(FnDefInfo{Name: "p", Signatures: []Signature{zero}}) {
		t.Error("an all-0-arg fn is a property read")
	}
	if !FnValueOnlyZeroArgSigs(FnDefInfo{Name: "pf", Signatures: []Signature{zero, fb}}) {
		t.Error("a synthetic Fallback does not disqualify a property fn")
	}
	if FnValueOnlyZeroArgSigs(FnDefInfo{Name: "m", Signatures: []Signature{zero, one}}) {
		t.Error("a mixed fn keeps the NUR035 deferral")
	}
	if FnValueOnlyZeroArgSigs(FnDefInfo{Name: "s"}) {
		t.Error("a sig-less value proves nothing")
	}
	if FnValueOnlyZeroArgSigs(FnDefInfo{Name: "fo", Signatures: []Signature{fb}}) {
		t.Error("a fallback-only value proves nothing")
	}
}

// TestPolyReachBoundCountsBuiltinTypeName — an unbound word that names a
// builtin type (`Integer`) steps to exactly one value, its type literal, so
// the reach bound counts it rather than reading it as unboundable
// (`convert Integer none` recorded no faithful-raise plan otherwise and the
// runtime no-match bailed, vm:poly-no-match; 2026-09-26). A genuinely
// unbound word still makes the bound untrustworthy.
func TestPolyReachBoundCountsBuiltinTypeName(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(0), NewWord("Integer"), NewEnd()}, StackHeadroom)
	e.Pointer = 0
	if n, ok := e.polyReachBound(); !ok || n != 1 {
		t.Errorf("bound = (%d,%v), want (1,true): a builtin type name is one type literal", n, ok)
	}
	e.Tape = NewTape([]Value{NewInteger(0), NewWord("zz-no-such-word"), NewEnd()}, StackHeadroom)
	if _, ok := e.polyReachBound(); ok {
		t.Error("an unbound non-type word keeps the bound untrustworthy")
	}
}
