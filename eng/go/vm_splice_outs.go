package eng

import core "github.com/boru-lang/boru/core/go"

// spliceOutsSeat seats a `do`'s results as the check pass stepped them
// (compiler.SigRef.SpliceOuts, NUR348): a literal body that reads a `word`
// value by value (`def w word [1 2] end do [w/v]`) hands back the splice
// marker, which the interpreter splices back at the `do` and fires — and
// the pass fired the marker it modelled, took it out of the call's results
// and recorded its expansion after the call. The results agree when they
// are as many as the pass's and every tape-coupled one is a splice
// rendering as the one the pass stepped at its position: the run's binding
// is the model's, and the recorded expansion is the interpreter's. Then the
// run is seated without its splices (ok). Anything else — no record,
// another count, another token, a splice over other tokens — does not
// agree, and the call's screen defers loudly, as it always has.
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
		if !core.IsSplice(v) || !core.IsSplice(want[i]) || core.CanonValue(v) != core.CanonValue(want[i]) {
			return nil, false
		}
	}
	return kept, true
}
