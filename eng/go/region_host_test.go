package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The descriptor adapter, driven through core's three kernel entry points.
//
// collect_seam_host_test.go already proves the seam is SATISFIABLE from
// outside core, with a hand-written window and stub evaluations. This file
// asserts the production host: that it walks a real RegionDesc, that its
// classifications answer from the LIVE registry rather than from anything
// frozen, and that every evaluation declines in the one way a caller can
// act on.
//
// The negative half is the point of the file. An adapter that answered an
// evaluation with a plausible guess would be a miscompile with no symptom;
// declining is what makes an un-drivable collection a DEFERRAL instead.

func regionDesc(toks ...core.Value) *compiler.RegionDesc {
	slots := make([]compiler.SlotDesc, 0, len(toks))
	for _, t := range toks {
		s := compiler.SlotDesc{Token: t}
		if core.IsWord(t) {
			s.Source = compiler.SlotWordRef
		} else {
			s.Source = compiler.SlotConst
		}
		slots = append(slots, s)
	}
	return &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "w", NFwd: len(slots), Slots: slots}
}

func newHost(t *testing.T, toks ...core.Value) (*regionHost, *core.Registry) {
	t.Helper()
	reg, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return newRegionHost(reg, regionDesc(toks...)), reg
}

// The window IS the slots' tokens, in written order, and it is a real Tape —
// so the gap-buffer splice and live-length semantics the kernel relies on are
// the interpreter's own, not a second implementation to keep correct.
func TestRegionHostWindowIsTheSlotTokens(t *testing.T) {
	a, b := core.NewInteger(1), core.NewWord("k")
	h, _ := newHost(t, a, b)

	w := h.Window()
	if w.Len() != 2 {
		t.Fatalf("window length %d, want 2", w.Len())
	}
	if core.CanonValue(w.At(0)) != core.CanonValue(a) || core.CanonValue(w.At(1)) != core.CanonValue(b) {
		t.Errorf("window = [%s %s], want the slots' tokens in written order",
			core.CanonValue(w.At(0)), core.CanonValue(w.At(1)))
	}
	// Live-length, spliceable: the kernel re-reads Len() after every mutation,
	// so a window that could not grow or shrink would break the walk rather
	// than merely differ from it.
	w.Splice(1, 1, core.NewInteger(7), core.NewInteger(8))
	if w.Len() != 3 {
		t.Errorf("after splice length %d, want 3", w.Len())
	}
	w.Remove(0)
	if w.Len() != 2 || core.CanonValue(w.At(0)) != "7" {
		t.Errorf("after remove: len %d, head %s", w.Len(), core.CanonValue(w.At(0)))
	}
}

// A descriptor with no slots is a legitimate shape, not a defect: a dispatch
// that claimed nothing forward still has a region, and the host must seat on
// it without a special case.
func TestRegionHostEmptyDescriptor(t *testing.T) {
	h, _ := newHost(t)
	if got := h.Window().Len(); got != 0 {
		t.Errorf("empty descriptor gave a window of %d", got)
	}
}

// Every evaluation declines, and declines the SAME way, so one predicate at
// the call site tells "this host cannot finish" from a collection error the
// kernel raised. The two demand opposite responses — defer versus surface —
// and a caller that conflated them would report an internal decline to the
// user as their own program's error.
func TestRegionHostEvaluationsDecline(t *testing.T) {
	h, _ := newHost(t, core.NewInteger(1))

	if err := h.EvalGroupAt(0); !RegionCannotEval(err) {
		t.Errorf("EvalGroupAt = %v, want the decline", err)
	}
	if _, err := h.EvalInterp(core.NewString("x")); !RegionCannotEval(err) {
		t.Errorf("EvalInterp = %v, want the decline", err)
	}
	if _, err := h.EvalXml(core.NewString("x")); !RegionCannotEval(err) {
		t.Errorf("EvalXml = %v, want the decline", err)
	}
	if _, err := h.ExpandSugarAt(core.NewInteger(1), 0, 0, nil); !RegionCannotEval(err) {
		t.Errorf("ExpandSugarAt = %v, want the decline", err)
	}
	if _, err := h.ExpandSugarTokens(core.SugarInfo{}, core.NewInteger(1), false); !RegionCannotEval(err) {
		t.Errorf("ExpandSugarTokens = %v, want the decline", err)
	}
	// The negative half: an unrelated error is NOT the decline. Without this
	// the predicate could be `err != nil` and every real raise would be
	// swallowed as a deferral.
	if RegionCannotEval(errors.New("some other failure")) {
		t.Error("an unrelated error must not read as the decline")
	}
	if RegionCannotEval(nil) {
		t.Error("nil must not read as the decline")
	}
}

// ScratchParenSpan reuses one buffer, per the seam's contract that the result
// is valid only until the caller splices. The assertion is that the CONTENT is
// right and the buffer is REUSED — a fresh allocation per call would pass a
// content-only check while costing an allocation on a collection's hottest
// path.
func TestRegionHostScratchSpanReuses(t *testing.T) {
	h, _ := newHost(t)
	// The span is the interpreter's shape: the items BETWEEN paren markers.
	// A span without them is what the kernel splices in for a ParenExpr, so
	// it would never meet an OpenParen on its next step — the first corpus
	// walk of the COLLECT oracle spun forever on exactly that.
	first := h.ScratchParenSpan([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if len(first) != 4 || !core.IsOpenParen(first[0]) || !core.IsCloseParen(first[3]) ||
		core.CanonValue(first[1]) != "1" || core.CanonValue(first[2]) != "2" {
		t.Fatalf("span = %v, want ( 1 2 )", first)
	}
	second := h.ScratchParenSpan([]core.Value{core.NewInteger(3)})
	if len(second) != 3 || core.CanonValue(second[1]) != "3" {
		t.Fatalf("second span = %v, want ( 3 )", second)
	}
	if &first[:1][0] != &second[0] {
		t.Error("the span buffer must be reused, not reallocated per call")
	}
}

// FlowInterrupted reads the LIVE registry rather than answering false by
// construction. Nothing this host runs can raise flow control today, so the
// honest answer happens to be false — but reading the flag is what keeps that
// true if it ever changes, and the paired assertion is what proves it is read
// rather than hardcoded.
func TestRegionHostFlowInterruptedIsLive(t *testing.T) {
	h, reg := newHost(t)
	if h.FlowInterrupted() {
		t.Error("a quiescent registry must not report flow control")
	}
	reg.FlowCtrl = core.FlowBreak
	if !h.FlowInterrupted() {
		t.Error("FlowInterrupted must read the live registry, not a constant")
	}
	reg.FlowCtrl = core.FlowNone
	if h.FlowInterrupted() {
		t.Error("clearing the flag must clear the answer")
	}
}

// TestRegionHostResolvesFnCarrierUnderAnalysis pins the seat's analysis
// arm (S1b-2): a name def-bound to a computed fn's CARRIER — held in the
// per-pass side table, not in Defs — resolves under an analysis pass and
// only there, exactly as the engine's own seat resolves it.
func TestRegionHostResolvesFnCarrierUnderAnalysis(t *testing.T) {
	h, reg := newHost(t, core.NewWord("f"))
	core.NoteCheckFnCarrierBind(reg, "f", core.NewCarrier(core.TFunction))
	if _, ok := h.DefTop("f"); ok {
		t.Fatal("outside analysis the table is not consulted")
	}
	end := reg.Check.Begin()
	defer end()
	if v, ok := h.DefTop("f"); !ok || !v.Carrier || !v.Parent.ConformsTo(core.TFunction) {
		t.Errorf("under analysis the table-bound carrier resolves: %v/%v", v, ok)
	}
	if _, ok := h.DefTop("zz"); ok {
		t.Error("an unbound name misses")
	}
}

// THE CENTRAL PROPERTY: a word slot is resolved LIVE, so the same descriptor
// answers differently as the binding moves. That is the whole reason the
// model keeps a word slot as SlotWordRef instead of freezing its value, and
// it is what OpCollect will exist to do — asserted here on the host, before
// any opcode executes one.
func TestRegionHostResolvesWordSlotsLive(t *testing.T) {
	h, reg := newHost(t, core.NewWord("k"))

	if _, ok := h.DefTop("k"); ok {
		t.Fatal("k must start unbound")
	}
	reg.Defs.Push("k", core.NewInteger(5))
	if v, ok := h.DefTop("k"); !ok || core.CanonValue(v) != "5" {
		t.Fatalf("DefTop after bind = %v/%v, want 5", core.CanonValue(v), ok)
	}
	reg.Defs.Push("k", core.NewInteger(9))
	if v, _ := h.DefTop("k"); core.CanonValue(v) != "9" {
		t.Errorf("DefTop after rebind = %s, want 9 — the slot must not be frozen", core.CanonValue(v))
	}
	if !reg.Defs.Pop("k") {
		t.Fatal("pop")
	}
	if v, _ := h.DefTop("k"); core.CanonValue(v) != "5" {
		t.Errorf("DefTop after undef = %s, want 5", core.CanonValue(v))
	}
}

// The classifications delegate to core's shared free functions rather than
// restating the rules. The assertion is that the answers MOVE when the
// registry moves: an adapter that hardcoded false/nil would satisfy an
// unbound-only check, which is what makes the bound half load-bearing rather
// than decorative.
func TestRegionHostClassificationsFollowTheRegistry(t *testing.T) {
	h, reg := newHost(t, core.NewWord("f"))
	tok := core.NewWord("f")

	// Negative half: nothing is bound, so nothing classifies.
	if h.IsFnWordBarrier(tok) {
		t.Error("an unbound word is not an fn-word barrier")
	}
	if h.LookupWord("f") != nil {
		t.Error("an unbound word must not resolve to a fn")
	}

	// Positive half: bind `f` as a fn and BOTH answers must change. This is
	// the half a hardcoded implementation fails.
	reg.Register("f", core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: 1})
	if !h.IsFnWordBarrier(tok) {
		t.Error("a bound fn word IS a barrier — the classification must follow the registry")
	}
	if got := h.LookupWord("f"); got == nil || got.Name != "f" {
		t.Errorf("LookupWord after binding = %v, want the fn", got)
	}
	// …and `/v` opts out, which is the exact subtlety the seam exists to keep
	// in one implementation. A host that read only "is it bound" would miss it.
	vTok := core.NewWord("f")
	if wi, err := core.AsWord(vTok); err == nil {
		wi.ForceVal = true
		vTok.Data = wi
	}
	if h.IsFnWordBarrier(vTok) {
		t.Error("`f/v` parks the fn as data and must NOT be a barrier")
	}

	// StaticForwardType is PURE — no registry, no window — which is why it is
	// the one classification that cannot drift between hosts. Asserted against
	// core's own answer rather than a transcribed constant.
	gotV, gotK := h.StaticForwardType(core.NewInteger(1))
	wantV, wantK := core.StaticForwardTypeOf(core.NewInteger(1))
	if core.CanonValue(gotV) != core.CanonValue(wantV) || gotK != wantK {
		t.Errorf("StaticForwardType = %s/%v, want core's own answer %s/%v",
			core.CanonValue(gotV), gotK, core.CanonValue(wantV), wantK)
	}
}

// reachFnValue builds a reach-collapsed named fn VALUE — a Function carrying
// FnDefInfo with the transient ReachGroup tag — which is the only shape the
// two reach classifications look past their opening guards for. Without it
// ReachFnWouldClaimOn returns false at its FnDefInfo assert and
// ReachCallHeadBarrierOn at its ReachGroup guard, so a host that hardcoded
// either to false would pass every test that only ever showed it a literal.
func reachFnValue(name string) core.Value {
	fd := core.FnDefInfo{
		Name:       name,
		Signatures: []core.Signature{{Args: []*core.Type{core.TAny}, BarrierPos: 1}},
	}
	v := core.NewValueRaw(core.TFunction, fd)
	v.ReachGroup = true
	return v
}

// The three kernel entry points, driven end to end through this host over a
// real descriptor, each asserting what it CLAIMED rather than that it ran.
//
// The fn is hand-built rather than looked up, for the reason the seam-host
// test builds one too: a bare core registry carries no natives (they are the
// lang layer's), and a test that SKIPPED here would prove nothing about the
// property it names.
func TestRegionHostDrivesTheKernelLoops(t *testing.T) {
	h, _ := newHost(t, core.NewInteger(1), core.NewInteger(2))
	fn := &core.FnDefInfo{
		Name: "pair",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger, core.TInteger},
			BarrierPos: 2,
		}},
	}

	if err := h.Collected(core.CollectForward(h, fn, core.WordInfo{Name: "pair", ArgCount: -1}, 0)); err != nil {
		t.Fatalf("CollectForward over a descriptor window: %v", err)
	}
	if got := h.Window().Len(); got != 2 {
		t.Errorf("the plan walk changed the window length to %d; a walk over plain "+
			"literals has nothing to rewrite", got)
	}

	// The candidate scan must CLAIM both integer slots and record where it took
	// them. `fwd` starts at zero and only increments, so asserting `fwd >= 0`
	// would be unfalsifiable — the count and the positions are the assertion.
	positions := []int{-1, -1}
	fwd, specAt := core.CollectCandidateScan(h, &fn.Signatures[0], 2, positions, 0, false, false)
	if fwd != 2 {
		t.Errorf("CollectCandidateScan claimed fwd=%d, want 2 — both slots match Integer", fwd)
	}
	if positions[0] != 0 || positions[1] != 1 {
		t.Errorf("positions = %v, want [0 1] — the window indexes the scan took", positions)
	}
	if specAt != -1 {
		t.Errorf("specAt = %d, want -1 — no slot here is speculative", specAt)
	}

	// The paired negative: a signature the tokens do NOT satisfy must claim
	// less. Without it, a scan that claimed everything unconditionally would
	// pass the positive half.
	strHost, _ := newHost(t, core.NewInteger(1), core.NewInteger(2))
	strSig := core.Signature{Args: []*core.Type{core.TString, core.TString}, BarrierPos: 2}
	strPos := []int{-1, -1}
	if got, _ := core.CollectCandidateScan(strHost, &strSig, 2, strPos, 0, false, false); got == 2 {
		t.Error("a String signature must not claim two Integer slots")
	}
}

// The ARRIVAL decision, driven past its completed-collection guard.
//
// CollectArrival returns ArrivalCollect immediately when CollectedArgs >=
// ExpectedArgs, so a zero ForwardInfo never reaches the window at all — the
// loop would be "driven" only in the sense that the function was called. The
// ForwardInfo below has a real deficit, which is what makes the walk ask this
// host anything.
//
// This is the loop the design records as having had ZERO samples on both
// sides of the Stage-2 CPU gate, so no benchmark or differential in the tree
// exercises it; a purpose-built driver is the only coverage it gets from
// outside core.
func TestRegionHostDrivesArrival(t *testing.T) {
	sig := &core.Signature{Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 2}

	// A value that MATCHES the pending slot fills it.
	h, _ := newHost(t, core.NewInteger(7))
	fwd := core.ForwardInfo{FuncName: "pair", Sig: sig, CollectedArgs: 0, ExpectedArgs: 2}
	before := h.Window().Len()
	if v := core.CollectArrival(h, fwd, 0); v != core.ArrivalCollect {
		t.Errorf("verdict = %v, want ArrivalCollect — an Integer fills an Integer slot", v)
	}
	if got := h.Window().Len(); got != before {
		t.Errorf("the arrival decision changed the window length %d -> %d for a plain "+
			"literal; only the /q and /v rewrites may mutate", before, got)
	}

	// The paired negative, and the half that proves the guard was passed at
	// all: a value that does NOT match the slot ends the collection instead.
	// A zero ForwardInfo would have returned ArrivalCollect for both.
	sh, _ := newHost(t, core.NewString("no"))
	sfwd := core.ForwardInfo{FuncName: "pair", Sig: sig, CollectedArgs: 0, ExpectedArgs: 2}
	if v := core.CollectArrival(sh, sfwd, 0); v != core.ArrivalImplicitEnd {
		t.Errorf("verdict = %v, want ArrivalImplicitEnd — a String does not fill an Integer slot", v)
	}
}

// A group token in the window is the shape that makes the decline REACHABLE
// through a kernel loop rather than only by direct call — the plan walk asks
// the host to evaluate it, and this host cannot. That is the exact path a
// routed dispatch would take to a deferral, so it is pinned end to end rather
// than at the method.
func TestRegionHostDeclineSurfacesThroughTheWalk(t *testing.T) {
	h, _ := newHost(t, core.NewOpenParen(), core.NewInteger(7))
	fn := &core.FnDefInfo{
		Name: "one",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TAny},
			BarrierPos: 1,
		}},
	}
	err := h.Collected(core.CollectForward(h, fn, core.WordInfo{Name: "one", ArgCount: -1}, 0))
	if !RegionCannotEval(err) {
		t.Fatalf("CollectForward over a group = %v, want the decline: a viable overload "+
			"consumes the position, so the walk must ask this host to evaluate it", err)
	}
}

// The two REACH classifications, shown a shape that gets past their guards.
//
// Both open by asserting the value is a reach-collapsed fn — ReachFnWouldClaimOn
// on its FnDefInfo payload, ReachCallHeadBarrierOn on the ReachGroup tag — so a
// test that only ever passed a Word or an Integer proves nothing: an adapter
// hardcoding either to false would pass it. Each is asserted against core's own
// answer over the same window, which is what pins the DELEGATION rather than a
// transcribed verdict.
func TestRegionHostReachClassificationsSeeARealFn(t *testing.T) {
	fnVal := reachFnValue("g")
	h, _ := newHost(t, fnVal, core.NewInteger(1))

	got := h.ReachFnWouldClaim(fnVal, 1)
	want := core.ReachFnWouldClaimOn(h.Window(), h.reg, fnVal, 1)
	if got != want {
		t.Errorf("ReachFnWouldClaim = %v, want core's own answer %v", got, want)
	}
	if !got {
		t.Error("a reach-collapsed fn with a forward sig and a following literal must claim it")
	}

	viable := []core.ViableSig{{Sig: &fnVal.Data.(core.FnDefInfo).Signatures[0], Barrier: 1}}
	gotHead := h.IsReachCallHead(fnVal, viable, 0, 0)
	wantHead := core.ReachCallHeadBarrierOn(h.Window(), h.reg, fnVal, viable, 0, 0)
	if gotHead != wantHead {
		t.Errorf("IsReachCallHead = %v, want core's own answer %v", gotHead, wantHead)
	}

	// The paired negative, and the reason the guards exist: a plain Word is not
	// a reach-collapsed fn, so neither classification looks past its guard.
	plain := core.NewWord("g")
	if h.ReachFnWouldClaim(plain, 1) {
		t.Error("a plain word is not a reach-collapsed fn and claims nothing")
	}
	if h.IsReachCallHead(plain, viable, 0, 0) {
		t.Error("a plain word is not a reach call head")
	}
}

// The window's growth ceiling, surfaced as the decline.
//
// core.Tape.Splice consumes its tokens BEFORE attempting to grow and returns
// early when the ceiling is hit, leaving the window short with only a latch to
// say so — its own comment defers to "the next step", which the interpreter
// has and this host does not. Without Collected, a walk that spliced past the
// ceiling would return nil over a truncated window and the caller would route
// against it.
func TestRegionHostSurfacesWindowExhaustion(t *testing.T) {
	h, _ := newHost(t, core.NewInteger(1))

	// A clean window is not exhausted, so the wrapper is transparent.
	if err := h.Collected(nil); err != nil {
		t.Errorf("Collected over a healthy window = %v, want nil", err)
	}
	// A real error passes through unchanged rather than being relabelled.
	sentinel := errors.New("a real collection failure")
	if err := h.Collected(sentinel); !errors.Is(err, sentinel) {
		t.Errorf("Collected must not swallow a real error, got %v", err)
	}
	// Exhaust the window: splice past the growth ceiling.
	big := make([]core.Value, h.win.MaxCap()+1)
	for i := range big {
		big[i] = core.NewInteger(int64(i))
	}
	h.win.Splice(0, 1, big...)
	if !h.win.Exhausted() {
		t.Fatal("the window did not latch exhaustion; the fixture no longer reaches the ceiling")
	}
	if err := h.Collected(nil); !RegionCannotEval(err) {
		t.Errorf("Collected over an exhausted window = %v, want the decline", err)
	}
}
