package compiler

// The quoted-operand compilation compile failure used to be lifted for `set`/`del`
// by setDelKernelSig (NUR057), a binding-identity key that admitted two
// classes under those two names: a LOCKED Go registration, and a
// BORU-BODIED open-words extension. Both are gone. `set`/`del`'s sixteen
// quoted-receiver signatures DECLARE CompileQuoteKey, read by quotedKeySig
// at both gates — the recorder consults a contract about the operand where
// it used to consult a word's name.
//
// The key could go because neither of its classes was carrying what its
// comment claimed:
//
//   - The LOCKED half was argued as "a registration identity no runtime
//     construction can counterfeit". It is not: a re-dispatch wrapper
//     copies the whole signature off the wrapped one, so it inherits
//     Locked (and CompileEffect), and NormalizeSig rebuilds the QuoteArgs
//     the constructor cleared. Its unit pin of the day asserted against a
//     HAND-BUILT bodiless sig, never a real usurp wrapper, so the claim was
//     never tested where it mattered. What actually excludes a wrapper is
//     the RunInCheckMode screen that precedes the exemption at both call
//     sites: a wrapper must be steppable by the carrier compiler, so every
//     constructor builds it `Go(handler, RunInCheck())`.
//   - The BORU-BODIED half was unreachable. A user-fn signature carries an
//     FnFrame, and `case sig.FnFrame() != nil` sits above the quoted-operand
//     arm in EmitState's switch (and above the quoted-operand line in
//     recordPolyCall), so an open-words extension of set/del never reached
//     the exemption. Deleting that half alone leaves the whole corpus
//     byte-identical.
//
// What the LOCKED half WAS carrying is unconditional admission, and that is
// load-bearing: it did not ask whether the quoted key was an inert const.
// An override's delegation to the base overload passes the key through a
// carrier (`set (k) v (m as FlexMap)` with `k` an `Atom/q` param), and
// lang/spec/as.tsv:52-54 are exactly that shape. Declaring CompileQuoteInert
// — whose admission requires IsInertConst — declined those three rows and put
// the compile-compile-failure ceiling back from 113 to 116. CompileQuoteKey exists to
// name that difference: a KEY the handler reads needs no const bake, where a
// LITERAL the handler consumes does.
//
// These pins hold the replacement: quotedKeySig's arms including the carrier
// key, the declaration branch of quoteOperandInertOK it is deliberately NOT
// routed through, and the screen that keeps a counterfeit off both.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// declaringRegistry registers a kernel-shaped word that declares
// CompileQuoteInert over one quoted Atom key, mirroring set/del's
// quoted-receiver overloads.
func declaringRegistry(t *testing.T) (*core.Registry, *core.FnDefInfo) {
	t.Helper()
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zz-quoted",
		Signatures: []core.Signature{{
			Args:          []*core.Type{core.TAtom, core.TAny, core.TMap},
			QuoteArgs:     map[int]bool{0: true},
			Impl:          core.Go(noop),
			Returns:       []*core.Type{core.TMap},
			CompileEffect: core.CompileQuoteInert,
		}},
	})
	fd := r.Lookup("zz-quoted")
	if fd == nil || len(fd.Signatures) == 0 {
		t.Fatal("registration not found")
	}
	return r, fd
}

func TestQuotedOperandDeclarationArms(t *testing.T) {
	r, fd := declaringRegistry(t)
	kernel := &fd.Signatures[0]
	inert := []core.Value{core.NewAtom("a"), core.NewInteger(1), core.NewMap(nil)}

	if !quoteOperandInertOK(r, "zz-quoted", kernel, inert) {
		t.Error("a declaring sig over inert quoted operands must be admitted")
	}
	// The declaration buys the door; the operand still has to be
	// const-bakeable to walk through it.
	dyn := []core.Value{core.NewDynamicCarrier(core.TAtom), core.NewInteger(1), core.NewMap(nil)}
	if quoteOperandInertOK(r, "zz-quoted", kernel, dyn) {
		t.Error("a non-inert quoted operand must decline")
	}
	// Without the declaration a core word does not ride this branch: that is
	// the whole difference between a contract and a name.
	plain := *kernel
	plain.CompileEffect = 0
	if quoteOperandInertOK(r, "zz-quoted", &plain, inert) {
		t.Error("an undeclared core word must not ride the exemption")
	}
	// Degenerate arms.
	if quoteOperandInertOK(r, "zz-quoted", nil, inert) {
		t.Error("nil sig must decline")
	}
	noQuote := *kernel
	noQuote.QuoteArgs = nil
	if quoteOperandInertOK(r, "zz-quoted", &noQuote, inert) {
		t.Error("a sig with no quoted operand has nothing to exempt")
	}
}

// TestQuotedOperandCounterfeitScreen runs every runtime wrapper constructor
// over the declaring word and pins both the outcome (the wrapper never rides
// the exemption) and the reason (RunInCheckMode, since the wrapper does
// inherit the flag and does present a quoted operand).
func TestQuotedOperandCounterfeitScreen(t *testing.T) {
	r, fd := declaringRegistry(t)
	inert := []core.Value{core.NewAtom("a"), core.NewInteger(1), core.NewMap(nil)}
	src := core.NewFunction(*fd)

	sawInheritedFlag := false
	for _, tc := range []struct {
		name string
		wrap func(core.Value) (core.Value, bool)
	}{
		{"usurp", core.UsurpFunction},
		{"stack-args", core.ForceStackFunction},
		{"forward-args", core.ForceForwardFunction},
		{"force-arity", func(v core.Value) (core.Value, bool) { return core.ForceArityFunction(v, 3) }},
	} {
		wrapped, ok := tc.wrap(src)
		if !ok {
			t.Fatalf("%s: the wrapper must build over a Function", tc.name)
		}
		wd, ok := wrapped.Data.(core.FnDefInfo)
		if !ok {
			t.Fatalf("%s: the wrapper must carry an FnDefInfo", tc.name)
		}
		sigs := wd.OwnSigs()
		if len(sigs) == 0 {
			t.Fatalf("%s: the wrapper must carry a signature", tc.name)
		}
		for i := range sigs {
			ws := &sigs[i]
			if quoteOperandInertOK(r, "zz-quoted", ws, inert) {
				t.Errorf("%s sig %d: the wrapper rode the declaration exemption", tc.name, i)
			}
			if hasUncoveredQuoteArg(ws) {
				// It inherited the flag AND presents a quoted operand: only
				// the RunInCheck screen is standing between it and the
				// exemption. A constructor that dropped RunInCheck would make
				// the declaration counterfeitable.
				if ws.CompileEffect.Has(core.CompileQuoteInert) {
					sawInheritedFlag = true
				}
				if !ws.RunInCheckMode() {
					t.Errorf("%s sig %d: presents a quoted operand with no RunInCheck screen — the exemption is now counterfeitable",
						tc.name, i)
				}
			}
		}
	}
	if !sawInheritedFlag {
		t.Error("no wrapper inherited flag+QuoteArgs — if that is now true by construction, this test's premise (and the comment above it) needs rewriting, not deleting")
	}
}

// TestQuotedKeySigArms pins the predicate the two gates actually read, and
// the one property that distinguishes it from quoteOperandInertOK: a KEY
// operand need not be an inert const.
func TestQuotedKeySigArms(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	noop := func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
		return nil, nil
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "zz-key",
		Signatures: []core.Signature{{
			Args:          []*core.Type{core.TAtom, core.TAny, core.TMap},
			QuoteArgs:     map[int]bool{0: true},
			Impl:          core.Go(noop),
			Returns:       []*core.Type{core.TMap},
			CompileEffect: core.CompileQuoteKey,
		}},
	})
	fd := r.Lookup("zz-key")
	if fd == nil || len(fd.Signatures) == 0 {
		t.Fatal("registration not found")
	}
	key := &fd.Signatures[0]

	if !quotedKeySig(key) {
		t.Error("a declaring sig must be admitted")
	}
	if quotedKeySig(nil) {
		t.Error("nil sig must decline")
	}
	undeclared := *key
	undeclared.CompileEffect = 0
	if quotedKeySig(&undeclared) {
		t.Error("an undeclared sig must decline — the declaration is the key")
	}
	// The flags are not interchangeable in either direction: quotedKeySig
	// reads only its own, so a CompileQuoteInert declarer does not ride this
	// gate (that is what keeps `raise` out of the poly path).
	inertOnly := *key
	inertOnly.CompileEffect = core.CompileQuoteInert
	if quotedKeySig(&inertOnly) {
		t.Error("CompileQuoteInert must not satisfy the key gate")
	}

	// THE POINT. A carrier-delivered key — an `Atom/q` param passed through a
	// paren, which is how an open-words override delegates to the base
	// overload — is admitted here and DECLINED by the inert-const predicate.
	// If these two ever agree, lang/spec/as.tsv:52-54 are about to regress.
	carrierKey := []core.Value{core.NewDynamicCarrier(core.TAtom), core.NewInteger(1), core.NewMap(nil)}
	if !quotedKeySig(key) {
		t.Error("the key gate must not consult the operand at all")
	}
	if quoteOperandInertOK(r, "zz-key", key, carrierKey) {
		t.Error("the inert-const predicate is expected to decline a carrier key — if it now accepts one, this test's premise changed")
	}
}
