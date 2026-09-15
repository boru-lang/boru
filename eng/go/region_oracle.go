package eng

import (
	"fmt"
	"strconv"
	"strings"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// The COLLECT oracle — the first EXECUTION of a region descriptor
// (design/FULL-COMPILATION.0.md §6.2, §6.5; the OpCollect doc).
//
// Every descriptor in Program.Regions was recorded by the check pass and
// completed by Phase B against the operands the lowering pushed; nothing
// has ever READ one at run time. Before OpDispatchGeneric routes a dispatch
// through a descriptor instead of beside one, the lane needs to know how
// often the live walk — the kernel's own collection routine over the
// descriptor host, against the registry the dispatch actually sees — comes
// to the same claim the record made. This is that measurement, taken the
// way the twins were: execute the table while nothing depends on it, report
// through a hook, and count over the whole corpus.
//
// WHAT THE WALK IS GIVEN. The window is the descriptor's slots: a slot the
// claim took forward (i < NFwd) presents the VALUE the lowering pushed for
// it — stack[top-i] is signature position i, and over the leading positions
// slot i IS signature position i (region_complete.go's index rule) — except
// a live word slot (SlotWordRef), which stays the word TOKEN so the walk
// resolves it against the live def stack exactly as the interpreter would;
// a slot beyond the claim stays its raw token. So a const, a frame local or
// a prior result is fed in as the operand it became, and only the live
// class is re-derived — which is the one class whose answer can move.
//
// WHAT IS CHECKED. Two things, independently: the EXTENT — some live
// signature of the lead's live binding claims exactly NFwd forward slots
// (the per-candidate scan, over that signature's own forward limit) — and
// the VALUES of the live word slots — each resolves, live, to the value the
// lowering pushed. A rebound module name is the shape that fails the
// second, or the first when the rebind turned a value into a barrier
// (region_desc.go's `k` pair). A walk the host cannot drive (a paren group
// or an interpolation beyond the claim, which the host declines to
// evaluate) reports "declined": nothing is known and nothing is claimed.
//
// It never changes the run. The outcome goes to core's region-oracle hook
// and the call opcode after it executes as it would have.

// regionOracleWindow builds the host's window for a descriptor from the
// operand stack the lowering left (see the file doc). The stack is read,
// never written.
func regionOracleWindow(d *compiler.RegionDesc, stack []core.Value) []core.Value {
	toks := make([]core.Value, len(d.Slots))
	top := len(stack) - 1
	for i := range d.Slots {
		s := &d.Slots[i]
		if i < d.NFwd && s.Source != compiler.SlotWordRef {
			toks[i] = stack[top-i]
		} else {
			toks[i] = s.Token
		}
	}
	return toks
}

// regOr is the registry a call resolves in: the one its program table names
// (a module fn's own sub-registry) or, when it names none, the running one.
func regOr(r, running *core.Registry) *core.Registry {
	if r != nil {
		return r
	}
	return running
}

// oracleCandidates is the signature set the walk is checked against: the
// CALL the oracle precedes says exactly what the dispatch used, and the
// program tables carry it — the baked SigRef of a CALL_NATIVE, a poly ref's
// word in its owner registry, a user unit's word in the unit's registry, a
// user poly's stored arm table. A live LOOKUP by the descriptor's word alone
// would be wrong twice over: a namespaced member (`MathUtil.sqrt`) records
// under its member name, which the running registry does not bind, and a
// member that shares a core word's name (`BinUtil.reverse`) would resolve to
// the WRONG binding — both measured on the first corpus walk (3705 unbound,
// 23 false over-claims). With no call after it (a hand-built program) the
// live lookup is all there is. The empty set means the lead is unbound.
func oracleCandidates(p *compiler.Program, code []compiler.Instr, pc int, d *compiler.RegionDesc, running *core.Registry) []*core.Signature {
	fnSigs := func(fn *core.FnDefInfo) []*core.Signature {
		if fn == nil {
			return nil
		}
		out := make([]*core.Signature, len(fn.Signatures))
		for i := range fn.Signatures {
			out[i] = &fn.Signatures[i]
		}
		return out
	}
	if pc+1 < len(code) {
		next := code[pc+1]
		switch next.Op {
		case compiler.OpCallNative:
			return []*core.Signature{p.Sigs[next.Arg].Sig}
		case compiler.OpCallNativePoly:
			pr := &p.PolyRefs[next.Arg]
			return fnSigs(regOr(pr.Reg, running).Lookup(pr.Word))
		case compiler.OpCallUser, compiler.OpTailCallUser:
			fn := &p.Fns[next.Arg]
			if sigs := fnSigs(regOr(fn.Reg, running).Lookup(d.Word)); len(sigs) > 0 {
				return sigs
			}
			// A FN-LOCAL fn (`def helper fn […]` inside a body) is a frame
			// binding the run-time registry never holds; the unit itself
			// says what the dispatch took — its params, all of them forward.
			if fn.NArgs > 0 && len(fn.Params) >= fn.NArgs {
				return []*core.Signature{{Args: fn.Params[:fn.NArgs], BarrierPos: fn.NArgs}}
			}
			return nil
		case compiler.OpCallUserPoly:
			up := &p.UserPolys[next.Arg]
			if len(up.Sigs) > 0 {
				out := make([]*core.Signature, len(up.Sigs))
				for i := range up.Sigs {
					out[i] = &up.Sigs[i]
				}
				return out
			}
			return fnSigs(regOr(up.Reg, running).Lookup(up.Word))
		}
	}
	return fnSigs(running.Lookup(d.Word))
}

// oracleStopPresentable reports whether the slot the live scan stopped at
// is one the descriptor host presents as the runtime does — a word (resolved
// live) or a scalar (the token is the value). A compound literal is not:
// the arrival loop evaluates an active list and converts a quotation on
// delivery, and a group is an evaluation this host declines outright.
func oracleStopPresentable(tok core.Value) bool {
	if core.IsWord(tok) {
		return true
	}
	if _, kind := core.StaticForwardTypeOf(tok); kind != core.FwdValue {
		return false
	}
	return !tok.Parent.ConformsTo(core.TList) && !tok.Parent.ConformsTo(core.TMap)
}

// oracleSameValue is the agreement test for a live word slot: is the value
// the lowering pushed the OBJECT the name is bound to? Identity first; then
// the `eq` word's own rule (core.ExactEqual) — a scalar by value, a closure
// or a handle by identity, a container by identity AND tag — with the
// container family asked directly (core.SameContainer), because
// ExactEqual's family fold does not reach a REFINED container (`def S
// (refine FlexMap) def w:S …` is not eq to itself, NUR142) and the oracle's
// question about it has an answer. Never by structure: two containers with
// equal contents are two objects, and an identity-sensitive word over the
// pushed one (`set` on a flex, `eq` on any) would act on the wrong object
// where the interpreter's live lookup takes the binding's (found in review
// of #458 — the first draft fell back to ValuesEqual, which is structural).
// Never by rendering either, which would walk a container's whole payload
// on every execution of a loop body that names it.
func oracleSameValue(live, pushed core.Value) bool {
	if live.ID != "" && live.ID == pushed.ID {
		return true
	}
	if core.HasContainerIdentity(live) || core.HasContainerIdentity(pushed) {
		return core.SameContainer(live, pushed)
	}
	return core.ExactEqual(live, pushed)
}

// collectOracle executes one OpCollect: walks the descriptor live and reports
// the outcome (never an error of the program's; the only errors are the
// VM's own invariants). code and pc locate the op, so the call after it can
// name the signatures the walk is checked against (oracleCandidates).
func (vc *vmContext) collectOracle(p *compiler.Program, code []compiler.Instr, pc int, d *compiler.RegionDesc, stack []core.Value, reg *core.Registry, curDebug []core.SrcPos) error {
	if len(stack) < d.NFwd {
		return vmErrAt(curDebug, pc, "COLLECT underflow at "+d.Word)
	}
	ev := core.RegionOracleEvent{Word: d.Word, Pos: d.Pos, NFwd: d.NFwd}
	sigs := oracleCandidates(p, code, pc, d, reg)
	if len(sigs) == 0 {
		ev.Outcome, ev.Detail = "unbound", "no binding for "+d.Word+" where the call resolves"
		reg.NoteRegionOracle(ev)
		return nil
	}
	// The plan walk wants the fn; a synthetic one over the candidate set
	// gives it exactly the signatures the dispatch could take.
	fn := &core.FnDefInfo{Name: d.Word, Signatures: make([]core.Signature, len(sigs))}
	for i, s := range sigs {
		fn.Signatures[i] = *s
	}
	h := newRegionHostOver(reg, regionOracleWindow(d, stack))
	w := core.WordInfo{Name: d.Word, ArgCount: -1}
	if d.Mods != nil {
		w = *d.Mods
	}
	if err := h.Collected(core.CollectForward(h, fn, w, 0)); err != nil {
		// A decline (the host cannot evaluate a group or an interpolation,
		// or the window hit its ceiling) and a kernel raise are the same
		// fact to the oracle: nothing is known about this walk.
		ev.Outcome, ev.Detail = "declined", err.Error()
		reg.NoteRegionOracle(ev)
		return nil
	}
	// The extent: does some live signature claim exactly the recorded NFwd?
	// The scan runs over the signature's OWN forward limit, which for a lead
	// carrying no /s or /f modifier is its BarrierPos (effectiveForwardLimit
	// in core) — and a lead that carried /s would have no region at all.
	// Which signature the lowering baked is not known here, so the check is
	// existential; the direction of a miss is what is reported, because the
	// two directions are not the same fact: a record that claims LESS than
	// the live walk (Phase B stopped early — a type name, a paren) is the
	// safe under-claim, a record that claims MORE is the miscompile shape.
	var claims []string
	exact, maxFwd := false, -1
	for _, sig := range sigs {
		// A zero-argument signature claims nothing forward — a ZERO-LENGTH
		// claim, counted like any other. The first draft skipped it, so a
		// lead whose only signature takes nothing left maxFwd at -1 and
		// the miss branch below indexed the stop slot with it (found in
		// review of #458).
		fwd := 0
		if n := sig.TotalArgs(); n > 0 {
			positions := make([]int, n)
			var specAt int
			fwd, specAt = core.CollectCandidateScan(h, sig, sig.BarrierPos, positions, 0, false, false)
			// A SPECULATIVE slot — a word bound to a dispatching
			// definition, which the scan counts optimistically because at
			// run time the token DISPATCHES rather than arriving as a value
			// (the parked forward carries it as its stop condition) — ends
			// the live claim: nothing from that slot on arrives as an
			// operand of this dispatch. That is the `k` pair's second
			// spelling, where the record pushed k's value and the
			// interpreter meets a barrier.
			if specAt >= 0 && specAt < fwd {
				fwd = specAt
			}
		}
		claims = append(claims, strconv.Itoa(fwd))
		if fwd == d.NFwd {
			exact = true
		}
		if fwd > maxFwd {
			maxFwd = fwd
		}
	}
	if !exact {
		ev.Outcome = "over-claimed"
		if maxFwd > d.NFwd {
			ev.Outcome = "under-claimed"
		} else if maxFwd < len(d.Slots) && !oracleStopPresentable(d.Slots[maxFwd].Token) {
			// The scan stopped short at a slot this host cannot PRESENT as
			// the runtime does — a list or map literal the arrival loop
			// evaluates or converts on delivery, a group. That is the
			// host's limit, not the record's error: nothing is known.
			ev.Outcome = "declined"
		}
		ev.Detail = fmt.Sprintf("no signature of %s claims exactly %d forward slot(s); the scans claimed [%s]",
			d.Word, d.NFwd, strings.Join(claims, " "))
		reg.NoteRegionOracle(ev)
		return nil
	}
	// The values: every live word slot inside the claim resolves to what the
	// lowering pushed for that position.
	top := len(stack) - 1
	for i := 0; i < d.NFwd; i++ {
		s := &d.Slots[i]
		if s.Source != compiler.SlotWordRef {
			continue
		}
		// A SlotWordRef token is a word by Phase A's construction; an
		// unbound name resolves to nothing, which is a disagreement too.
		wi, _ := core.AsWord(s.Token)
		pushed := stack[top-i]
		live, ok := reg.Defs.Top(wi.Name)
		if ok && oracleSameValue(live, pushed) {
			continue
		}
		got := "<unbound>"
		if ok {
			got = core.CanonValue(live)
		}
		ev.Outcome = "diverged-value"
		ev.Detail = fmt.Sprintf("live word slot %s resolves to %s where the lowering pushed %s", wi.Name, got, core.CanonValue(pushed))
		reg.NoteRegionOracle(ev)
		return nil
	}
	ev.Outcome = "reproduced"
	reg.NoteRegionOracle(ev)
	return nil
}
