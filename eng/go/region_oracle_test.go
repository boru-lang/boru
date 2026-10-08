package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The COLLECT oracle at the seam: a hand-built descriptor, a live registry
// and the operand stack the lowering would have left, driven through every
// outcome. The corpus lane (test/go/langspec/region_oracle_test.go) is the
// measurement; this pins that each outcome is the one the file doc names.

// oracleWorld is region_desc.go's `k` pair as a fixture: `w` takes two Any
// operands forward, `k` is a module binding, and the descriptor is the
// dispatch `w k 1` — slot 0 the live word `k`, slot 1 the const `1`, both
// claimed. The stack is what the lowering pushed: sig position 0 (k's value
// at record time, 5) on top, position 1 (the 1) beneath.
func oracleWorld(t *testing.T) (*vmContext, *core.Registry, *compiler.RegionDesc, []core.Value) {
	t.Helper()
	reg := seam7Reg(t)
	reg.Register("w", core.Signature{Args: []*core.Type{core.TAny, core.TAny}, BarrierPos: 2})
	reg.Defs.Push("k", core.NewInteger(5))
	d := &compiler.RegionDesc{Lead: compiler.LeadWord, Word: "w", Pos: core.SrcPos{Row: 1, Col: 1}, NFwd: 2,
		Slots: []compiler.SlotDesc{
			{Source: compiler.SlotWordRef, Token: core.NewWord("k")},
			{Source: compiler.SlotConst, Token: core.NewInteger(1)},
		}}
	vc := &vmContext{p: &compiler.Program{Regions: []compiler.RegionDesc{*d}}, r: reg}
	return vc, reg, d, []core.Value{core.NewInteger(1), core.NewInteger(5)}
}

func oracleRun(t *testing.T, vc *vmContext, reg *core.Registry, d *compiler.RegionDesc, stack []core.Value) core.RegionOracleEvent {
	t.Helper()
	var got []core.RegionOracleEvent
	disarm := reg.ArmRegionOracleHook(func(ev core.RegionOracleEvent) { got = append(got, ev) })
	defer disarm()
	if err := vc.collectOracle(vc.p, nil, 0, d, stack, reg, nil); err != nil {
		t.Fatalf("collectOracle: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the oracle reports exactly once per execution, got %d", len(got))
	}
	if got[0].Word != d.Word || got[0].NFwd != d.NFwd || got[0].Pos != d.Pos {
		t.Errorf("the event names the descriptor: %+v", got[0])
	}
	return got[0]
}

func TestRegionOracleReproducesTheKPair(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	if ev := oracleRun(t, vc, reg, d, stack); ev.Outcome != "reproduced" {
		t.Fatalf("k bound to the value the lowering pushed: %+v", ev)
	}
}

// The value half: k rebound to another VALUE after the record. The walk
// still claims two slots (a value is collected), but the live binding is not
// what the lowering pushed — the frozen-read hazard, seen at run time.
func TestRegionOracleSeesARebindOfTheValue(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	reg.Defs.Push("k", core.NewInteger(9))
	ev := oracleRun(t, vc, reg, d, stack)
	if ev.Outcome != "diverged-value" || !strings.Contains(ev.Detail, "resolves to 9 where the lowering pushed 5") {
		t.Fatalf("a rebound value must diverge by value: %+v", ev)
	}
	// …and an UNBOUND live word is a disagreement too, not a crash.
	reg.Defs.Pop("k")
	reg.Defs.Pop("k")
	ev = oracleRun(t, vc, reg, d, stack)
	if ev.Outcome != "diverged-value" || !strings.Contains(ev.Detail, "<unbound>") {
		t.Fatalf("an unbound live word diverges by value: %+v", ev)
	}
}

// The extent half: k rebound to a FN. A word bound to a dispatching
// definition is SPECULATIVE to the scan — at run time it dispatches rather
// than arriving as a value, and the interpreter strands the forward (the
// `k` pair's second spelling) — so the live claim ends at slot 0 where the
// record claimed two: the record claims MORE than the walk, the miscompile
// direction.
func TestRegionOracleSeesARebindToABarrier(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	reg.Register("k", core.Signature{Args: nil, Returns: []*core.Type{core.TInteger}, BarrierPos: 0})
	ev := oracleRun(t, vc, reg, d, stack)
	if ev.Outcome != "over-claimed" {
		t.Fatalf("k as a barrier: the record over-claims: %+v", ev)
	}
}

// The other direction: the record stopped short of what the walk claims.
// NFwd 1 over the same two-slot region, with k a value: the scan claims 2.
func TestRegionOracleSeesAnUnderClaim(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	d.NFwd = 1
	ev := oracleRun(t, vc, reg, d, stack)
	if ev.Outcome != "under-claimed" || !strings.Contains(ev.Detail, "the scans claimed [2]") {
		t.Fatalf("a record shorter than the live claim under-claims: %+v", ev)
	}
}

// A walk the host cannot drive — a paren group beyond the claim — reports a
// decline, and an unbound LEAD reports that; neither is an error.
func TestRegionOracleDeclinesAndUnbound(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	d.NFwd = 1
	d.Slots[1] = compiler.SlotDesc{Source: compiler.SlotNone, Token: core.NewOpenParen()}
	if ev := oracleRun(t, vc, reg, d, stack); ev.Outcome != "declined" {
		t.Fatalf("a group the host cannot evaluate declines: %+v", ev)
	}
	d.Word = "nosuchword"
	if ev := oracleRun(t, vc, reg, d, stack); ev.Outcome != "unbound" {
		t.Fatalf("an unbound lead reports so: %+v", ev)
	}
}

// The candidate set comes from the CALL the oracle precedes — the program
// tables name exactly what the dispatch used — and only a hand-built program
// with nothing after the op falls back to a live lookup. Driven over each
// call kind's table with a one-op code window.
func TestRegionOracleCandidatesFollowTheCall(t *testing.T) {
	_, reg, d, _ := oracleWorld(t)
	sig := &core.Signature{Args: []*core.Type{core.TInteger}, BarrierPos: 1}
	p := &compiler.Program{
		Sigs:      []compiler.SigRef{{Word: "min", Sig: sig}},
		PolyRefs:  []compiler.PolyRef{{Word: "w"}},
		Fns:       []compiler.CompiledFn{{Name: "w"}},
		UserPolys: []compiler.UserPolyRef{{Word: "w", Sigs: []core.Signature{*sig, *sig}}, {Word: "w"}},
	}
	code := func(op compiler.Opcode, arg int32) []compiler.Instr {
		return []compiler.Instr{{Op: compiler.OpCollect}, {Op: op, Arg: arg}}
	}
	// A baked native sig is the ONE candidate, whatever the word's live binding.
	if got := oracleCandidates(p, code(compiler.OpCallNative, 0), 0, d, reg); len(got) != 1 || got[0] != sig {
		t.Errorf("CALL_NATIVE: the SigRef's own signature, got %v", got)
	}
	// A poly ref resolves its word in its registry (here the running one).
	if got := oracleCandidates(p, code(compiler.OpCallNativePoly, 0), 0, d, reg); len(got) != 1 || got[0].BarrierPos != 2 {
		t.Errorf("CALL_NATIVE_POLY: w's live signatures, got %v", got)
	}
	// A user unit resolves the descriptor's word in the unit's registry.
	for _, op := range []compiler.Opcode{compiler.OpCallUser, compiler.OpTailCallUser} {
		if got := oracleCandidates(p, code(op, 0), 0, d, reg); len(got) != 1 || got[0].BarrierPos != 2 {
			t.Errorf("%v: w's live signatures, got %v", op, got)
		}
	}
	// A user poly's stored arm table is the set; an empty table resolves live.
	if got := oracleCandidates(p, code(compiler.OpCallUserPoly, 0), 0, d, reg); len(got) != 2 || got[0] != &p.UserPolys[0].Sigs[0] {
		t.Errorf("CALL_USER_POLY: the stored arm table, got %v", got)
	}
	if got := oracleCandidates(p, code(compiler.OpCallUserPoly, 1), 0, d, reg); len(got) != 1 || got[0].BarrierPos != 2 {
		t.Errorf("CALL_USER_POLY with no stored table: w's live signatures, got %v", got)
	}
	// Nothing after the op, or an op that is not a call: the live lookup.
	if got := oracleCandidates(p, code(compiler.OpDrop, 0), 0, d, reg); len(got) != 1 || got[0].BarrierPos != 2 {
		t.Errorf("a non-call successor: the live lookup, got %v", got)
	}
	if got := oracleCandidates(p, nil, 0, d, reg); len(got) != 1 {
		t.Errorf("no successor: the live lookup, got %v", got)
	}
	// A unit that names its OWN registry resolves there.
	other := seam7Reg(t)
	other.Register("w", core.Signature{Args: []*core.Type{core.TAny}, BarrierPos: 1})
	p.Fns[0].Reg = other
	if got := oracleCandidates(p, code(compiler.OpCallUser, 0), 0, d, reg); len(got) != 1 || got[0].BarrierPos != 1 {
		t.Errorf("CALL_USER into a module unit: the unit's registry, got %v", got)
	}
	// A FN-LOCAL fn: no registry holds the word, so the unit's own params
	// are the signature — all forward; a unit with no args names nothing.
	local := &compiler.RegionDesc{Word: "helper", NFwd: 1, Slots: []compiler.SlotDesc{{Source: compiler.SlotConst, Token: core.NewInteger(5)}}}
	p.Fns = append(p.Fns, compiler.CompiledFn{Name: "helper", NArgs: 1, Params: []*core.Type{core.TInteger}}, compiler.CompiledFn{Name: "noargs"})
	if got := oracleCandidates(p, code(compiler.OpCallUser, 1), 0, local, reg); len(got) != 1 || got[0].BarrierPos != 1 || got[0].Args[0] != core.TInteger {
		t.Errorf("a fn-local unit: its params as one forward signature, got %v", got)
	}
	if got := oracleCandidates(p, code(compiler.OpCallUser, 2), 0, local, reg); got != nil {
		t.Errorf("a fn-local unit with no args names no signature, got %v", got)
	}
}

// Where the live scan stops short of the record, the slot it stopped at
// decides between a real over-claim and the host's own limit: a word or a
// scalar is presentable, a compound literal or a group is not.
func TestRegionOracleStopPresentable(t *testing.T) {
	list := core.NewList([]core.Value{core.NewInteger(1)})
	for _, c := range []struct {
		tok  core.Value
		want bool
	}{
		{core.NewWord("k"), true},
		{core.NewInteger(5), true},
		{core.NewString("s"), true},
		{core.NewOpenParen(), false},
		{list, false},
		{core.NewTypeLiteral(core.TInteger), false},
	} {
		if got := oracleStopPresentable(c.tok); got != c.want {
			t.Errorf("presentable(%s) = %v, want %v", core.CanonValue(c.tok), got, c.want)
		}
	}
}

// The VM's own invariants: a stack shorter than the claim, and a region
// index the program does not have. Both are VM errors, never outcomes.
func TestRegionOracleVMInvariants(t *testing.T) {
	vc, reg, d, _ := oracleWorld(t)
	if err := vc.collectOracle(vc.p, nil, 0, d, []core.Value{core.NewInteger(5)}, reg, nil); err == nil || !strings.Contains(err.Error(), "COLLECT underflow at w") {
		t.Fatalf("a short stack is an underflow: %v", err)
	}
	// Through the opcode: an index past the table, then a descriptor the
	// main code can drive (NFwd 0 needs no operands) — the case arm runs the
	// oracle and the program continues to its end.
	bad := &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpCollect, Arg: 3}}}
	if _, err := RunProgram(bad, reg); err == nil || !strings.Contains(err.Error(), "COLLECT region index out of range") {
		t.Fatalf("an index past the region table is a VM error: %v", err)
	}
	// Through the opcode again: the underflow above is the one error the
	// arm returns, and it fails the run as the VM's own invariant.
	short := &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpCollect, Arg: 0}}, Regions: []compiler.RegionDesc{*d}}
	if _, err := RunProgram(short, reg); err == nil || !strings.Contains(err.Error(), "COLLECT underflow at w") {
		t.Fatalf("the arm surfaces the oracle's invariant as the run's error: %v", err)
	}
	d.NFwd = 0
	prog := &compiler.Program{Code: []compiler.Instr{{Op: compiler.OpCollect, Arg: 0}}, Regions: []compiler.RegionDesc{*d}}
	var got []core.RegionOracleEvent
	disarm := reg.ArmRegionOracleHook(func(ev core.RegionOracleEvent) { got = append(got, ev) })
	defer disarm()
	if _, err := RunProgram(prog, reg); err != nil {
		t.Fatalf("the oracle never fails the run: %v", err)
	}
	if len(got) != 1 || got[0].Outcome != "under-claimed" {
		t.Fatalf("the opcode drives the oracle and continues: %+v", got)
	}
}

// The window the walk is given: a claimed slot presents the operand the
// lowering pushed (stack[top-i] for slot i), a live word slot keeps its
// token, and a slot beyond the claim keeps its token.
func TestRegionOracleWindowPresentsOperands(t *testing.T) {
	_, _, d, stack := oracleWorld(t)
	d.Slots = append(d.Slots, compiler.SlotDesc{Source: compiler.SlotNone, Token: core.NewWord("tail")})
	d.Slots[1] = compiler.SlotDesc{Source: compiler.SlotLocal, Idx: 0, Token: core.NewWord("b")}
	win := regionOracleWindow(d, stack)
	if len(win) != 3 || !core.IsWord(win[0]) || core.CanonValue(win[1]) != "1" || !core.IsWord(win[2]) {
		t.Fatalf("window = %v: want [word k, the pushed 1, word tail]", win)
	}
}

// A zero-argument candidate is a zero-length claim (found in review of
// #458): the scan loop skipped it, so a lead whose only signature takes
// nothing left maxFwd at -1 and the miss branch indexed the stop slot with
// it — a panic the VM would recover as internal_error in place of an
// event. A record of nothing forward against it is exact; a record of one
// slot is an over-claim the scans report as [0].
func TestRegionOracleZeroArgCandidate(t *testing.T) {
	vc, reg, d, _ := oracleWorld(t)
	reg.Register("z", core.Signature{Args: nil, BarrierPos: 0})
	d.Word, d.NFwd = "z", 0
	d.Slots = []compiler.SlotDesc{{Source: compiler.SlotConst, Token: core.NewInteger(1)}}
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1)}); ev.Outcome != "reproduced" {
		t.Errorf("a zero-arg lead claims exactly the recorded nothing: %+v", ev)
	}
	d.NFwd = 1
	ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1)})
	if ev.Outcome != "over-claimed" || !strings.Contains(ev.Detail, "the scans claimed [0]") {
		t.Errorf("a record claiming one slot over a zero-arg lead over-claims: %+v", ev)
	}
}

// Agreement is the `eq` word's rule, not structure (found in review of
// #458): a live word slot bound to a DISTINCT container with equal
// contents diverges — an identity-sensitive word over the pushed one
// (`set` on a flex, `eq` on any container) would act on the wrong object
// where the interpreter's live lookup takes the binding's. The same
// instance, rebound, still reproduces.
func TestRegionOracleContainerIdentity(t *testing.T) {
	vc, reg, d, _ := oracleWorld(t)
	one := core.NewFlexList([]core.Value{core.NewInteger(1)})
	reg.Defs.Push("k", one)
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), one}); ev.Outcome != "reproduced" {
		t.Errorf("the same flex instance, pushed and bound, reproduces: %+v", ev)
	}
	twin := core.NewFlexList([]core.Value{core.NewInteger(1)})
	ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), twin})
	if ev.Outcome != "diverged-value" {
		t.Errorf("a distinct flex instance with equal contents is a divergence: %+v", ev)
	}
	// An immutable list is a container too: `eq` is identity there as well.
	lit := core.NewList([]core.Value{core.NewInteger(2)})
	reg.Defs.Push("k", lit)
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), core.NewList([]core.Value{core.NewInteger(2)})}); ev.Outcome != "diverged-value" {
		t.Errorf("a distinct immutable list with equal contents is a divergence: %+v", ev)
	}
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), lit}); ev.Outcome != "reproduced" {
		t.Errorf("the same immutable list reproduces: %+v", ev)
	}
	// Identity by ID first: a value that carries one (a carrier's or a
	// clone's provenance ID) agrees with itself before any equality is
	// asked.
	tagged := core.NewInteger(7)
	tagged.ID = "T_same-provenance"
	reg.Defs.Push("k", tagged)
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), tagged}); ev.Outcome != "reproduced" {
		t.Errorf("the same ID is the same value: %+v", ev)
	}
	// A REFINED container (`def S (refine FlexMap) def w:S (flex {a:1})`):
	// ExactEqual does not reach its identity (NUR142), the oracle's rule
	// does — the same object under the same tag reproduces; the same
	// store under another tag is another value.
	sub := reg.Types.MintType("S", core.TFlexList)
	refined := core.ReparentValue(one, sub)
	reg.Defs.Push("k", refined)
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), refined}); ev.Outcome != "reproduced" {
		t.Errorf("a refined flex list, the same object under the same tag, reproduces: %+v", ev)
	}
	if ev := oracleRun(t, vc, reg, d, []core.Value{core.NewInteger(1), one}); ev.Outcome != "diverged-value" {
		t.Errorf("the same store under another tag is another value: %+v", ev)
	}
}

// The scan stops short at a slot this host cannot present as the runtime
// does — a list literal the arrival loop evaluates on delivery — and the
// record claims past it: the host's limit, reported as a decline, never as
// an over-claim (the miscompile direction) the host has no evidence for.
func TestRegionOracleDeclinesAtACompoundStop(t *testing.T) {
	vc, reg, d, _ := oracleWorld(t)
	reg.Register("w2", core.Signature{Args: []*core.Type{core.TInteger, core.TInteger}, BarrierPos: 2})
	lit := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	d.Word, d.NFwd = "w2", 2
	d.Slots = []compiler.SlotDesc{
		{Source: compiler.SlotConst, Token: core.NewInteger(1)},
		{Source: compiler.SlotConst, Token: lit},
	}
	ev := oracleRun(t, vc, reg, d, []core.Value{lit, core.NewInteger(1)})
	if ev.Outcome != "declined" || !strings.Contains(ev.Detail, "the scans claimed [1]") {
		t.Errorf("a claim past a compound stop is the host's limit, not the record's error: %+v", ev)
	}
}

// The lead's modifiers ride the descriptor (RegionDesc.Mods, review of
// #460) and the oracle hands them to the walk as the record's own walk had
// them: the `k` pair with `/f` on the lead reproduces exactly as the plain
// lead does (TestRegionOracleReproducesTheKPair). The collection walk the
// oracle runs reads the lead's NAME; the modifiers decide the plan-level
// match, which the routed op runs and pins the contrast of
// (TestDispatchGenericReviewGuards, "the lead's modifiers are honoured").
func TestRegionOracleWalksWithTheLeadModifiers(t *testing.T) {
	vc, reg, d, stack := oracleWorld(t)
	d.Mods = &core.WordInfo{Name: "w", ArgCount: -1, ForceForward: true}
	if ev := oracleRun(t, vc, reg, d, stack); ev.Outcome != "reproduced" {
		t.Fatalf("`w/f k 1` over an all-forward overload reproduces the two-slot claim: %+v", ev)
	}
}
