package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestParenBoundedTrailingApply(t *testing.T) {
	// `(5 (cmk1))` — the closure is the LAST value in the paren; the
	// check pass records a paren-bounded dyn-apply event.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewOpenParen(),
			core.NewInteger(5),
			core.NewOpenParen(), core.NewWord("cmk1"), core.NewCloseParen(),
			core.NewCloseParen(),
		}
	}, "105")
}

func TestParenBoundedApplyFeedsCall(t *testing.T) {
	// The paren-bounded closure inside a consumer's window. Under the BROAD
	// park (NUR073 clause 3) the inner collapse PLACES the closure instead of
	// eagerly applying it against the 5, so cdub collects the 5 first (→ 10)
	// and the placed closure dispatches at its next pointer encounter against
	// cdub's result (→ 110). Pre-BROAD the eager inner apply gave cdub 105
	// (→ 210); that order is gone with the re-step.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cdub"),
			core.NewOpenParen(),
			core.NewInteger(5),
			core.NewOpenParen(), core.NewWord("cmk1"), core.NewCloseParen(),
			core.NewCloseParen(),
		}
	}, "110")
}

func TestParenLeadingDynamicFailsToCompile(t *testing.T) {
	// `((cany) 3)` — a leading DYNAMIC value before args inside a paren
	// is the unsound reorder shape: check declines; interpreter runs.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewOpenParen(),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
			core.NewInteger(3),
			core.NewCloseParen(),
		}
	}, "7 | 3")
}

// --- user-fn poly re-match -------------------------------------------------------------

func TestCompiledDisjunctArgDispatch(t *testing.T) {
	// The same join consumed by cdub, end to end.
	runTolerant(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cdub"),
			core.NewOpenParen(),
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
			core.NewInteger(1),
			core.NewString("s"),
			core.NewCloseParen(),
		}
	}, "'ss'")
}

// --- dispatch error paths ----------------------------------------------------------------
