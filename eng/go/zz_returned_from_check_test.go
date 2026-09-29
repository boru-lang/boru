package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestW8TryRecordUnmatchedTrapZeroArgSig(t *testing.T) {
	// A 0-arg non-fallback sig is courtesy-dispatchable → returns false.
	r := covRegistry(t, nil)
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	e.Tape = core.NewTape([]core.Value{core.NewInteger(1)}, core.StackHeadroom)
	e.Pointer = 0
	fn := &core.FnDefInfo{Signatures: []core.Signature{{BarrierPos: -1}}}
	if e.TryRecordUnmatchedDispatchTrap(core.WordInfo{Name: "w8z", ArgCount: -1}, fn, core.SrcPos{}) {
		t.Error("a 0-arg sig should make the trap decline (false)")
	}
}

// TestW8DispatchRematchPermutedStackTakesDefiniteTrap — a permuted CONCRETE
// stack window never reaches the rematch gates: the stack positions fill the
// window first (resolvedIndicesBefore), every value is runtime-identical, so
// the DEFINITE trap serialises the interpreter's error — reorder hint
// included — with no runtime rebuild needed.
func TestW8DispatchRematchPermutedStackTakesDefiniteTrap(t *testing.T) {
	r := covRegistry(t, nil)
	r.RegisterNativeFunc(core.NativeFunc{Name: "w8rh", Signatures: []core.Signature{{
		Args: []*core.Type{core.TInteger, core.TString}, BarrierPos: -1,
		Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
			return nil, nil
		}),
	}}})
	done := w8ArmCompile(t, r)
	defer done()
	e := core.NewTop(r)
	// The operands written after the word are concrete: a CARRIER there is
	// a value the interpreter's match examines first and its report names,
	// which sends the failure to the rematch (NUR329,
	// withGradualWrittenOperands) — no longer this definite-trap arm.
	e.Tape = core.NewTape([]core.Value{
		core.NewInteger(1), core.NewString("s"), // prefix: permutes (Integer, String)
		core.NewWord("w8rh"),
		core.NewInteger(3), core.NewInteger(2),
	}, core.StackHeadroom)
	e.Pointer = 2
	fn := r.Lookup("w8rh")
	if fn == nil {
		t.Fatal("w8rh not registered")
	}
	if !e.TryRecordUnmatchedDispatchTrap(core.WordInfo{Name: "w8rh", ArgCount: -1}, fn, core.SrcPos{}) {
		t.Error("a concrete permuted stack window must serialise the definite trap")
	}
}
