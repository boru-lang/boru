package compiler

import core "github.com/boru-lang/boru/core/go"

// CaptureRegionSlots reads the tokens of the region starting at `from` into
// written-order SlotDescs (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// It READS the window and never evaluates: no group is run, no sugar
// lowered, no forward collected. That is deliberate and it is why this can
// take a CollectWindow without a collectHost — the recorder's job is to
// record what is THERE, not to decide what a dispatch would claim. Deciding
// is OpCollect's, at run time, against the live binding set.
//
// A WORD slot is finished here, as SlotWordRef. That is not an optimisation:
// a word's binding is resolved LIVE on every execution (see SlotWordRef, and
// the `k` pair in region_desc.go's type doc), so there is nothing for a
// lowerer to add and nothing a later pass could learn. The name is in Token,
// which is where OpCollect reads it.
//
// Every OTHER slot is captured with its Source left at SlotNone — the
// INVALID zero, not a valid-looking SlotConst — because its source does come
// from the recorder's own operand model: which values are interned, which
// are frame locals, which are prior events, which paren spans became
// fragments. The lowerer fills those in, and a slot that reaches execution
// still holding SlotNone is a missed initialisation rather than a silent
// reference to constant 0.
//
// This function stays PURE, which is why the word case belongs here and the
// rest does not: interning is not idempotent for compounds (es.intern never
// pools a list or a map, so two source literals stay two constants), and a
// region is offered to the recorder on EVERY execution of its dispatch. A
// capture that interned would grow the const table once per loop iteration.
// SlotWordRef costs nothing and cannot drift.
//
// What this establishes besides the word slots is the region's EXTENT and
// its written order, which are properties of the tape and nothing else.
//
// Returns nil for an empty region, so a caller can distinguish "no region"
// from "a region of zero slots" without a second call.
func CaptureRegionSlots(w core.CollectWindow, from int) []SlotDesc {
	n := core.RegionSlotCount(w, from)
	if n == 0 {
		return nil
	}
	if from < 0 {
		from = 0
	}
	slots := make([]SlotDesc, 0, n)
	for i := from; i < from+n; i++ {
		tok := w.At(i)
		d := SlotDesc{Quote: quoteOfToken(tok), Token: tok}
		if core.IsWord(&tok) {
			d.Source = SlotWordRef
		}
		slots = append(slots, d)
	}
	return slots
}

// quoteOfToken reads the STATIC dispatch-control modifier a token carries.
//
// Unlike a slot's class, a modifier is syntax: it is on the token, no
// binding can move it, and it is therefore sound to freeze at record time —
// the one half of a slot's description that genuinely is record-time-final.
func quoteOfToken(v core.Value) SlotQuote {
	if core.IsAtom(v) {
		// A /q slot has already become an Atom by the time it is on the
		// tape: the quote is not a pending instruction, it is done.
		return QuoteAtom
	}
	if core.IsDispatchMod(&v) {
		// /v and /q are distinct facts on ONE marker type and mean different
		// things downstream: /v disables the call, /q treats the RESULT as
		// data. Collapsing them loses the distinction the payload carries.
		if info, ok := core.DataOf(&v).(core.DispatchModInfo); ok && info.Quote {
			return QuoteData
		}
		return QuoteValue
	}
	return QuoteNone
}
