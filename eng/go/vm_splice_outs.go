package eng

import core "github.com/boru-lang/boru/core/go"

// spliceOutsSeat seats a `do`'s results as the check pass stepped them
// (compiler.SigRef.SpliceOuts, NUR348): a literal body that reads a `word`
// value by value (`def w word [1 2] end do [w/v]`) hands back the splice
// marker, which the interpreter splices back at the `do` and fires — and
// the pass fired the marker it modelled, took it out of the call's results
// and recorded its expansion after the call. The results agree when they
// are as many as the pass's and every tape-coupled one is a splice the SAME
// as the one the pass stepped at its position (sameSpliceToken): the run's
// binding is the model's, and the recorded expansion is the interpreter's.
// Then the run is seated without its splices (ok). Anything else — no
// record, another count, another token, a splice over other tokens or over
// a payload this host cannot prove the same — does not agree, and the
// call's screen defers loudly, as it always has.
func spliceOutsSeat(want, got []core.Value) ([]core.Value, bool) {
	if want == nil || len(want) != len(got) {
		return nil, false
	}
	kept := make([]core.Value, 0, len(got))
	for i, v := range got {
		if !tapeCoupled(got[i : i+1]) {
			if core.IsSplice(want[i]) {
				return nil, false
			}
			kept = append(kept, v)
			continue
		}
		if !core.IsSplice(v) || !sameSpliceToken(v, want[i]) {
			return nil, false
		}
	}
	return kept, true
}

// sameSpliceToken reports whether a and b are provably the same token for
// the tape's step: a splice over the same payload, a plain list of the same
// tokens, a word of the same name and modifiers, or a scalar leaf whose
// canonical rendering is its value (an Integer, a string, a Boolean, an
// Atom) of the same type. A rendering is no identity for anything
// else — two closures of one name and signature over different captures
// both render `fn f(Integer)` — so any other token, a carrier, a flex, a
// map, a fn value, a Float, is never the same here: the run defers
// (review of #522).
func sameSpliceToken(a, b core.Value) bool {
	if a.Parent == nil || b.Parent == nil || !a.Parent.Equal(b.Parent) || a.Quoted != b.Quoted || a.Eval != b.Eval ||
		a.Carrier || b.Carrier || a.Dynamic || b.Dynamic {
		return false
	}
	if ai, err := core.AsSplice(a); err == nil {
		bi, err := core.AsSplice(b)
		return err == nil && sameSpliceToken(ai.Data, bi.Data)
	}
	if aw, ok := a.Data.(core.WordInfo); ok {
		bw, ok := b.Data.(core.WordInfo)
		return ok && aw == bw
	}
	if al, ok := a.Data.(core.ListPayload); ok {
		bl, ok := b.Data.(core.ListPayload)
		if !ok || len(al.Elems) != len(bl.Elems) {
			return false
		}
		for i := range al.Elems {
			if !sameSpliceToken(al.Elems[i], bl.Elems[i]) {
				return false
			}
		}
		return true
	}
	return core.IsConcrete(a) && core.IsConcrete(b) && renderIsValue(a.Parent) && core.CanonValue(a) == core.CanonValue(b)
}

// renderIsValue reports whether a value of type t renders injectively —
// its canonical text is its value: an Integer, a string, a Boolean, an
// Atom.
func renderIsValue(t *core.Type) bool {
	for _, leaf := range []*core.Type{core.TInteger, core.TString, core.TBoolean, core.TAtom} {
		if t.ConformsTo(leaf) {
			return true
		}
	}
	return false
}
