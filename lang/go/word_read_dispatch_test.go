package lang

// NUR123 (measured 2026-09-05). A bare read of a frame binding holding a fn
// is a WORD dispatch on the interpreter — stepWord routes a bound FnDefInfo
// through Registry.Lookup under the binding name: a 0-arg fn fires, an n-arg
// fn collects from the tokens after it and the frame's stack below it, a
// no-match raises `cannot call `g`` — while the check model bound a CARRIER
// the emitter lowered as a slot push: `def f fn [[g:Function][Any][g]]  f
// ([] => [42])` answered `fn` for the interpreter's 42, and the identity fn
// over a 0-arg lambda the same, both on the DEFAULT lane with exit 0. The
// engine now notes each such read (NoteWordRead), the unit's finish arms the
// whole-frame replay over the residual with the binding names, and the VM
// re-steps a fn-valued read as the WORD through the interpreter's own
// dispatch (CompiledFn.DynFrameWords). A fn-typed read the replay cannot
// seat declines; a gradual one keeps the slot push it always had.

import (
	"fmt"
	"strings"
	"testing"
)

// TestGradualFnParamMapLiteralCheckIsClean pins NUR125 (resolved 2026-09-05):
// the check pass over a map literal whose value is a bare read of a gradual
// param bound to a fn PANICKED (recovered as internal_error, both lanes,
// exit 1). The check run binds a concrete lambda argument by pushing it as
// a plain def — a FnDefInfo no installDef gave a runner — and the literal's
// const-fold sub-run dispatched the bare read by name into a signature
// whose DispatchHandler is nil. execMatch now raises internal_error on a
// nil runner (ADR-005: an error, never a panic), the fold declines on it,
// and the literal records normally: the check pass is clean, and both lanes
// answer the interpreter's value (the compiled lane through the compile failure
// pinned above). Boru.Check is the entry point that reached the fold;
// CompileCheck never did, so the compile failure rows alone could not pin it.
func TestGradualFnParamMapLiteralCheckIsClean(t *testing.T) {
	for _, src := range []string{
		`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: (k)}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: k}]]  h 5  h ([] => [42])`,
		`def h fn [[k:Any n:Integer][Any][{a: k, b: n}]]  h ([] => [42]) 1`,
	} {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		res, cerr := a.Check(src)
		if cerr != nil || len(res.Diagnostics) != 0 {
			t.Errorf("%q: check must be clean: err=%v diags=%v", src, cerr, res.Diagnostics)
		}
	}
	// the negative twin: a gradual param whose fn does not match its read
	// stays the interpreter's no_signature diagnostic, not a clean pass
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := `def h fn [[k:Any][Any][{a: k}]]  h (z:Integer => [z])`
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("%q: check error: %v", src, cerr)
	}
	if len(res.Diagnostics) != 1 || !strings.Contains(fmt.Sprint(res.Diagnostics[0]), "cannot call `k`") {
		t.Errorf("%q: want one no_signature diagnostic naming k, got %v", src, res.Diagnostics)
	}
}
