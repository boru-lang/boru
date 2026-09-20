package compiler

import core "github.com/boru-lang/boru/core/go"

// Region capture, PHASE B — completing a Phase-A capture against the
// recorder's own operand model (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// Phase A (region_record.go) records what the region CONTAINS: its extent,
// its written-order tokens, each token's static quote modifier, and — for a
// word — the finished SlotWordRef source, because a word's binding is
// resolved live and there is nothing a later pass could learn. Every other
// slot is left at the invalid SlotNone, because its source comes from the
// recorder's operand model and no tape index is in scope there.
//
// THIS IS NOT THE JOIN THREE MODELS WERE REVERTED FOR. Those tried to match
// a written-order slot against an already-RESOLVED operand by searching for
// it. The rule makes searching unnecessary: matching fills sig positions from
// the FORWARD tokens in written order, then fills every remaining position
// from the value stack. So for the leading positions the two orders are the
// SAME order, and slot i is sig position i — a direct index, not a search.
//
// What the reverted models actually got wrong was ASSUMING that. Measured
// over the corpus, the prefix rule holds at 99.83% and the residue is real:
// a `none` word rendering against a `None` value, a record type against its
// expanded object form, and a handful whose slots and args genuinely
// disagree. The recorder holds both the slots and the operands here, so the
// correspondence costs one comparison per claimed slot and becomes a CHECKED
// precondition. Where it stops, the claim stops: that is NFwd.
//
// Nothing here interns, allocates a const, or otherwise touches a program
// table. Phase A must stay side-effect-free because a region is offered on
// EVERY execution of its dispatch (a capture that interned would append a
// fresh const per loop iteration — es.intern never pools compounds); Phase B
// runs once per RECORDED call, and it only copies indices the operand model
// has already assigned.

// completeRegion claims the Phase-A capture for this dispatch and fills the
// sources of the slots it actually claimed, reporting the completed
// descriptor. A miss is ordinary and returns nil: not every recorded call has
// a region (a stack-only dispatch never reaches forward collection), and not
// every region becomes a recorded call (`def` captures and is never recorded,
// being a check-mode word).
//
// args are in SIGNATURE order and ops are their operands, index for index —
// RecordCallOperands' own contract.
func (es *EmitState) completeRegion(word string, pos core.SrcPos, args []core.Value, ops []EmitOperand) *RegionDesc {
	if es == nil {
		return nil
	}
	return es.completeOffer(es.takePendingRegion(word, pos))(args, ops)
}

// completeHeldRegion is completeRegion for a record that HOLDS its offer — a
// user-fn call, whose ReturnsFn took the offer out of the pool at entry
// (HoldRegion). Only such a record may complete a held offer: a native
// record under the same (word, row, col) — a module arm's `MathUtil.min`
// beneath a main program's `Lib.min`, both at 2:1 — completes from the pool,
// where its own offer is, and can never take the outer call's (the review
// finding on #457). A holder whose hold found nothing completes nothing.
func (es *EmitState) completeHeldRegion(word string, pos core.SrcPos, args []core.Value, ops []EmitOperand) *RegionDesc {
	if es == nil {
		return nil
	}
	return es.completeOffer(es.claimHeldRegion(word, pos))(args, ops)
}

// completeOffer fills the sources of the slots the dispatch actually took
// forward, over the offer a claim handed back; a miss completes nothing.
func (es *EmitState) completeOffer(off pendingRegion, ok bool) func(args []core.Value, ops []EmitOperand) *RegionDesc {
	return func(args []core.Value, ops []EmitOperand) *RegionDesc {
		d := off.desc
		if !ok || d == nil || len(d.Slots) == 0 {
			return nil
		}
		return es.fillOffer(off, args, ops)
	}
}

// fillOffer is the completion proper: a COPY of the offered descriptor with
// the claimed prefix's sources filled, NFwd where the claim stopped.
func (es *EmitState) fillOffer(off pendingRegion, args []core.Value, ops []EmitOperand) *RegionDesc {
	d := off.desc
	// The capture is keyed by position and re-offered on every execution, so
	// completing in place would mutate a descriptor an earlier completion may
	// already have stamped. Work on a copy; the slots are copied with it.
	// The lead's reachability is decided here, by the same rule the slots
	// below use for a word: a binding pushed inside the enclosing fn is one
	// no live lookup finds where the body runs (found in review of #460 — a
	// body-local callee routed, and the op looked up a name the run-time
	// def stack does not hold), and so is a lead the dispatch registry
	// itself does not hold — a module native reached through its wrapper,
	// whose inner signature is dispatched from the caller's registry where
	// only the namespace is bound (review of #461).
	leadHeld := off.reg != nil && off.reg.Lookup(d.Word) != nil
	out := &RegionDesc{Lead: d.Lead, Word: d.Word, Pos: d.Pos, Mods: d.Mods, Reg: off.reg,
		LeadLocal: !leadHeld || fnScopedWord(off.reg, core.NewWord(d.Word)),
		Slots:     append([]SlotDesc(nil), d.Slots...)}
	n := len(out.Slots)
	if len(args) < n {
		n = len(args)
	}
	for i := 0; i < n; i++ {
		if !es.slotIsOperand(out.Slots[i], off.reg, args[i]) {
			break
		}
		src, idx, resIdx, ok := regionSourceOf(ops[i])
		if !ok {
			break
		}
		// A word slot is already finished, and finishing it AGAIN from the
		// operand is the frozen-class mistake this model exists to decline:
		// the operand is the binding the word had during the pass, and the
		// whole point of SlotWordRef is that the next execution may find a
		// different one (region_desc.go's `k` pair).
		//
		// EXCEPT when the name is FN-SCOPED, and that exception is the fn-unit
		// hazard rather than an exception to the rule. Inside a fn body the
		// analysis binds params and body-local defs into the def stack so the
		// body can be analysed, so they resolve here — but the emitted body
		// reads them from the FRAME or bakes them, and at run time the def
		// stack holds no such binding. Live re-derivation would miss the name
		// or, worse, find an outer binding of the same one.
		//
		// A MODULE-scope name read from inside a fn body is not in that class
		// and must stay live: it is exactly region_desc.go's `k` pair, the
		// shape OpCollect exists to answer.
		//
		// AND a word whose operand is a FRAME SLOT lives in the frame at run
		// time whatever scope its name has. The fn-scoped test alone missed a
		// class the COLLECT oracle found on its first corpus run
		// (eng/go/region_oracle.go): a TOP-LEVEL loop iterator — `for 6 [if
		// (eq i 3) …]` — is bound by the loop's analysis so it resolves here,
		// has no enclosing fn so it is not fn-scoped, and is never replayed by
		// a twin, so the run-time def stack holds no `i` at all; a live
		// re-derivation would miss where the emitted code reads the slot.
		// The operand says where the value lives, so the slot says the same.
		if out.Slots[i].Source == SlotWordRef {
			if src == SlotLocal {
				out.Slots[i].Source, out.Slots[i].Idx, out.Slots[i].ResIdx = src, idx, resIdx
			} else if fnScopedWord(off.reg, out.Slots[i].Token) {
				// A frame-bound name whose operand does not say which slot —
				// a body-local `def`, promoted only after completion: stop.
				break
			}
		} else {
			out.Slots[i].Source, out.Slots[i].Idx, out.Slots[i].ResIdx = src, idx, resIdx
		}
		out.NFwd = i + 1
	}
	return out
}

// slotIsOperand reports whether written-order slot s is the operand that
// filled its sig position — the checked half of the prefix rule.
//
// The comparison is by VALUE IDENTITY, not by structural equality, and that
// is the point: a forward-collected operand IS the tape token the capture
// recorded, the same instance, so its ID matches. Structural equality would
// admit a coincidence — `add 1 1` fills sig position 1 from the stack with a
// value equal to the forward token at written position 1 — and a coincidental
// match would extend the claim past what the dispatch really took forward.
//
// A WORD slot is the one indirection. Its operand is the BINDING, not the
// word token, so the comparison resolves the name through the def stack
// exactly as the matcher's own patternsOk does before unifying. An unbound
// word never corresponds: it cannot have supplied an operand.
//
// The registry is the DISPATCH's, carried on the Phase-A offer, not es.reg.
// es.reg is the last registry BindRegistry saw, which after a call into a
// boru-implemented module is that module's sub-registry — resolving a later
// main-registry dispatch's words there finds nothing, and the claim stops
// short of operands that did come forward.
//
// An EMPTY id never corresponds, and that guard is load-bearing rather than
// defensive. core.NewValueRaw stamps an id only for a payload-less value or
// one minted inside the check pass, so a value built outside a pass carries
// "" — and two of those would compare EQUAL, which is the coincidental match
// this function exists to decline, in its worst form: it would extend the
// claim over a slot the dispatch never took. Tape tokens do carry ids today
// (the parser stamps them, measured), so the guard costs nothing; what it
// buys is that the failure mode is an UNDER-claim, which defers, rather than
// an over-claim, which miscompiles.
func (es *EmitState) slotIsOperand(s SlotDesc, reg *core.Registry, arg core.Value) bool {
	if arg.ID == "" {
		return false
	}
	if s.Source == SlotWordRef {
		if reg == nil {
			return false
		}
		wi, err := core.AsWord(s.Token)
		if err != nil {
			return false
		}
		// A LIVE READ of the word (NoteLiveRead — a generalised name's read
		// seated with its own identity) is the slot's operand as surely as
		// the binding itself: it is a read of exactly this word, minted at
		// this token.
		if es.liveReadIDs[arg.ID] && es.defReads[arg.ID] == wi.Name {
			return true
		}
		top, ok := reg.Defs.Top(wi.Name)
		return ok && top.ID == arg.ID
	}
	return s.Token.ID == arg.ID
}

// fnScopedWord reports whether a word token names a binding that lives inside
// an enclosing fn — a param, a loop iterator, or a body-local `def` — rather
// than at module scope.
//
// The test is the closure-capture rule, verbatim and deliberately: Registry's
// own doc for FnBaselines says a referenced name with Defs.Depth(name) >
// baseline[name] lives inside an enclosing fn, and depth == baseline means it
// lives at module/global scope and stays dynamic. Captures and descriptors are
// asking the same question — will this name still be reachable by a live
// lookup where the body actually runs — so they must not answer it two ways.
//
// An empty baseline stack means the dispatch is at top level, where nothing is
// fn-scoped.
func fnScopedWord(reg *core.Registry, tok core.Value) bool {
	if reg == nil || len(reg.FnBaselines) == 0 {
		return false
	}
	wi, err := core.AsWord(tok)
	if err != nil {
		return false
	}
	return reg.Defs.Depth(wi.Name) > reg.FnBaselines[len(reg.FnBaselines)-1][wi.Name]
}

// regionSourceOf translates one operand of the recorder's model into the
// descriptor's slot source. The two enumerations are deliberately separate
// types — the operand model describes an EMISSION, the slot source describes
// what OpCollect reads — but for the kinds a region can carry they correspond
// exactly, so the translation is a table rather than a reinterpretation.
//
// opDynScope and opDataScope decline, and the count is the argument: they are
// runtime dynamic-scope lookups by name, 12 and 0 occurrences in the corpus
// (region_desc.go's SlotSource doc records the same measurement declining a
// member for them). A bounded, countable decline beats an enumeration member
// nothing fills.
func regionSourceOf(op EmitOperand) (SlotSource, int, int, bool) {
	switch op.kind {
	case opConst:
		return SlotConst, op.idx, 0, true
	case opLocal:
		return SlotLocal, op.idx, 0, true
	case opType:
		return SlotType, op.idx, 0, true
	case opEvent:
		return SlotEvent, op.idx, op.resIdx, true
	default:
		return SlotNone, 0, 0, false
	}
}
