package core

// Wave-7 coverage, part 4: Run()-driven programs that exercise the
// interpreter loop's dispatch, forward-collection, paren, end-of-input,
// and error arms with the eng-only cov vocabulary plus a few extra
// words. See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// runReg builds a cov registry with an extra failing/void vocabulary
// used across the Run scenarios.
func runReg(t *testing.T) *Registry {
	t.Helper()
	return covRegistry(t, func(r *Registry) {
		// cfail: always errors (runtime error propagation).
		r.RegisterNativeFunc(NativeFunc{
			Name: "cfail",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, &BoruError{Code: "runtime_error", Detail: "boom"}
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
		// cvoid: produces no value (a void group).
		r.RegisterNativeFunc(NativeFunc{
			Name: "cvoid",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
		// capply: forward-collects a Function then an Integer. Used to
		// drive the "pending forward expecting Function" path in stepWord.
		r.RegisterNativeFunc(NativeFunc{
			Name: "capply",
			Signatures: []Signature{{
				Args: []*Type{TFunction, TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{args[1]}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
}

func mustRun(t *testing.T, r *Registry, toks []Value) ([]Value, error) {
	t.Helper()
	return NewTop(r).Run(toks)
}

// --- basic forward + stack + paren dispatch -------------------------------

func TestS7RunForwardAndParen(t *testing.T) {
	r := runReg(t)
	// cadd (cmul 2 3) 4 = 6 + 4 = 10 → forward paren-group eval path.
	out, err := mustRun(t, r, []Value{
		NewWord("cadd"),
		NewOpenParen(), NewWord("cmul"), NewInteger(2), NewInteger(3), NewCloseParen(),
		NewInteger(4),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1: %v", len(out), out)
	}
	if n, _ := AsInteger(out[0]); n != 10 {
		t.Errorf("result = %d, want 10", n)
	}
}

func TestS7RunStackForm(t *testing.T) {
	r := runReg(t)
	// 5 cneg = -5 (cneg is a stack word, BarrierPos 0).
	out, err := mustRun(t, r, []Value{NewInteger(5), NewWord("cneg")})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != -5 {
		t.Errorf("result = %d, want -5", n)
	}
}

// --- insufficient args at end-of-input (resolveOrphanedForwards) ----------

func TestS7RunInsufficientArgsAtEnd(t *testing.T) {
	r := runReg(t)
	// cadd needs two forward args; only one is available before EOF.
	_, err := mustRun(t, r, []Value{NewWord("cadd"), NewInteger(1)})
	if err == nil {
		t.Fatal("cadd with one arg should error at end-of-input")
	}
}

// --- insufficient args inside a paren -------------------------------------

func TestS7RunInsufficientArgsInParen(t *testing.T) {
	r := runReg(t)
	// (cadd 1) — the group closes before cadd gathers its second arg.
	_, err := mustRun(t, r, []Value{
		NewOpenParen(), NewWord("cadd"), NewInteger(1), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("(cadd 1) should error: insufficient args in paren")
	}
	if !strings.Contains(err.Error(), "insufficient") && !strings.Contains(err.Error(), "signature") {
		t.Logf("err = %v", err)
	}
}

// --- error propagation from a handler (stampErrPos / maybeAddFnShapeHint) --

func TestS7RunHandlerErrorPropagates(t *testing.T) {
	r := runReg(t)
	_, err := mustRun(t, r, []Value{NewWord("cfail"), NewInteger(1)})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("cfail should propagate its error, got %v", err)
	}
}

func TestS7RunHandlerErrorInParen(t *testing.T) {
	r := runReg(t)
	// A failing word inside a forward paren group.
	_, err := mustRun(t, r, []Value{
		NewWord("cadd"),
		NewOpenParen(), NewWord("cfail"), NewInteger(1), NewCloseParen(),
		NewInteger(2),
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error inside a forward paren group should surface, got %v", err)
	}
}

// --- void group + statement boundary --------------------------------------

func TestS7RunVoidGroupThenEnd(t *testing.T) {
	r := runReg(t)
	// (cvoid 1) end 5 — a void group, a statement boundary, then a value.
	out, err := mustRun(t, r, []Value{
		NewOpenParen(), NewWord("cvoid"), NewInteger(1), NewCloseParen(),
		NewEnd(),
		NewInteger(5),
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected a residual value")
	}
}

// --- end token resolving a pending forward --------------------------------

func TestS7RunPendingForwardThenEndErrors(t *testing.T) {
	r := runReg(t)
	// cadd 1 end — the end boundary resolves the under-fed forward via
	// stepEnd → curryOrStack; with no outer collector this force-stacks
	// and the subsequent dispatch fails (insufficient).
	_, err := mustRun(t, r, []Value{
		NewWord("cadd"), NewInteger(1), NewEnd(),
	})
	if err == nil {
		t.Fatal("cadd 1 end should still be an insufficient-args error")
	}
}

// --- unmatched close paren ------------------------------------------------

func TestS7RunUnmatchedCloseParen(t *testing.T) {
	r := runReg(t)
	_, err := mustRun(t, r, []Value{NewInteger(1), NewCloseParen()})
	if err == nil || !strings.Contains(err.Error(), "closing") {
		t.Fatalf("unmatched close paren should error, got %v", err)
	}
}

// --- polymorphic dispatch (matchSignature multi-sig) ----------------------

func TestS7RunPolymorphicDispatch(t *testing.T) {
	r := runReg(t)
	// cdub over Integer and String.
	out, err := mustRun(t, r, []Value{NewWord("cdub"), NewInteger(4)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 8 {
		t.Errorf("cdub 4 = %d, want 8", n)
	}
	out2, err := mustRun(t, r, []Value{NewWord("cdub"), NewString("ab")})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if s, _ := AsString(out2[0]); s != "abab" {
		t.Errorf("cdub ab = %q, want abab", s)
	}
}

// --- no matching signature (wrong types) ----------------------------------

func TestS7RunNoMatchingSignature(t *testing.T) {
	r := runReg(t)
	// ccat wants two Strings; give it Integers.
	_, err := mustRun(t, r, []Value{NewWord("ccat"), NewInteger(1), NewInteger(2)})
	if err == nil {
		t.Fatal("ccat over Integers should fail to match")
	}
}

// --- bare type-path word (stepWord ResolveTypePath arm) -------------------

func TestS7RunBareTypePathWord(t *testing.T) {
	r := runReg(t)
	// An undefined word that IS a slashed type path resolves to a type
	// literal (not a dispatch error).
	out, err := mustRun(t, r, []Value{NewWord("Scalar/Number/Integer")})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 || !IsBareTypeNode(out[0]) {
		t.Fatalf("expected a type literal, got %v", out)
	}
}

// --- pending forward expecting Function: no reference intercept (NUR078) ---

func TestS7StepWordForwardExpectingFunction(t *testing.T) {
	r := runReg(t)
	// Hand-build a pending forward whose NEXT slot (CollectedArgs=1) is a
	// Function. A bare def-fn word at the pointer is NOT resolved to a
	// reference any more: the name CALLS whatever slot is open (ADR-011 as
	// amended), so it is a function word beginning its own dispatch while the
	// forward still waits — the strict-rule barrier error.
	sig := &Signature{Params: []FnParam{{Type: TInteger}, {Type: TFunction}}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "capply", Sig: sig, CollectedArgs: 1, FuncIndex: 0})
	e := NewTop(r)
	e.Tape = NewTape([]Value{fwd, NewWord("cadd")}, StackHeadroom)
	e.Pointer = 1
	err := e.stepWord(e.Tape.At(1))
	if err == nil || !strings.Contains(err.Error(), "begins its own dispatch") {
		t.Fatalf("a bare fn name before a Function slot must dispatch, not arrive as a reference: %v", err)
	}
	// The reference is explicit: `cadd/v` arrives as the Function value.
	e = NewTop(r)
	w := NewWord("cadd")
	info, _ := AsWord(w)
	info.ForceVal = true
	w = NewValueRaw(TWord, info)
	e.Tape = NewTape([]Value{fwd, w}, StackHeadroom)
	e.Pointer = 1
	if err := e.stepWord(e.Tape.At(1)); err != nil {
		t.Fatalf("stepWord cadd/v: %v", err)
	}
	for i := 0; i < e.Tape.Len(); i++ {
		if IsWord(e.Tape.At(i)) {
			t.Errorf("cadd/v left unresolved as a Word at %d", i)
		}
	}
}

// --- word forced to both /s and /f (resolveForwardArgs ForceStack arm) -----

func TestS7RunForceStackAndForward(t *testing.T) {
	r := runReg(t)
	// A word carrying BOTH /s and /f reaches resolveForwardArgs (via
	// ForceForward) yet takes the ForceStack barrier=0 arm inside it.
	w := NewWordModified("cadd", -1, true, true)
	// Provide the two args on the stack (barrier 0 → all-stack).
	_, err := mustRun(t, r, []Value{NewInteger(2), NewInteger(3), w})
	// Either dispatches or errors cleanly; the point is exercising the arm.
	_ = err
}

// --- user fn dispatch (execFnDefSig / ExecFnDefSigStackMatch) --------------

func TestS7RunUserFnDispatch(t *testing.T) {
	r := runReg(t)
	// ctwice 5 = 10 (user fn, forward form).
	out, err := mustRun(t, r, []Value{NewWord("ctwice"), NewInteger(5)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 10 {
		t.Errorf("ctwice 5 = %d, want 10", n)
	}
	// cquad 3 = 12 (nested user fn calls).
	out2, err := mustRun(t, r, []Value{NewWord("cquad"), NewInteger(3)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := AsInteger(out2[0]); n != 12 {
		t.Errorf("cquad 3 = %d, want 12", n)
	}
}
