package core

import (
	"strings"
	"testing"
)

func nur069Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

// NUR069: the CallBoru dispatch path enforces a fn's declared return
// contract, which it previously did not — it trimmed to arity and asked
// nothing about type, so a module export returning the wrong shape passed
// silently on the interpreter while a compiled unit's RET raised.
//
// These exercise enforceCallBoruReturns directly. The end-to-end module
// spelling is pinned in lang/spec/record.tsv, but that suite is another
// module's, and core/go's standalone gate wants its own coverage of every
// arm here — including the two that only a DIRECT call can reach.

func TestNUR069EnforceReturnsAcceptsConforming(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{Returns: []*Type{TInteger}}
	if err := r.enforceCallBoruReturns(sig, "f", []Value{NewInteger(1)}); err != nil {
		t.Fatalf("a conforming return must pass: %v", err)
	}
}

func TestNUR069EnforceReturnsRejectsWrongType(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{Returns: []*Type{TInteger}}
	err := r.enforceCallBoruReturns(sig, "f", []Value{NewString("nope")})
	if err == nil {
		t.Fatal("a String under a declared [Integer] must be declined")
	}
	if !strings.Contains(err.Error(), "expected Integer") {
		t.Fatalf("the error must name the declared type, got %v", err)
	}
}

// An UNDECLARED return keeps the historical flow-through — the residual IS
// the return — matching the frame path, which emits no ReturnCheck at all
// in that case.
func TestNUR069EnforceReturnsSkipsUndeclared(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{}
	if err := r.enforceCallBoruReturns(sig, "f", []Value{NewString("anything")}); err != nil {
		t.Fatalf("an undeclared return must not be checked: %v", err)
	}
}

// COUNT is deliberately not enforced, only the types of the positions
// PRESENT: a lambda auto-declaring [Any] over a side-effect body, and a
// guard predicate signalling with a bare residual, both legitimately
// return fewer values than they declare. Fewer results than declared
// therefore checks the tail that exists rather than raising on arity.
func TestNUR069EnforceReturnsChecksOnlyPresentPositions(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{Returns: []*Type{TString, TInteger}}
	// One result against two declared returns pairs HEAD-to-head, as the
	// frame path does: the value is matched against declared position 0.
	if err := r.enforceCallBoruReturns(sig, "f", []Value{NewString("x")}); err != nil {
		t.Fatalf("a short result must check only what is there: %v", err)
	}
	if err := r.enforceCallBoruReturns(sig, "f", []Value{NewInteger(7)}); err == nil {
		t.Fatal("…and must still decline the wrong type in that position")
	}
	// More results than declared: the surplus sits at the BOTTOM, so the
	// declared positions pair with the TOP of the residual.
	over := []Value{NewBoolean(true), NewString("x"), NewInteger(1)}
	if err := r.enforceCallBoruReturns(sig, "f", over); err != nil {
		t.Fatalf("surplus at the bottom must be skipped, not misaligned: %v", err)
	}
}

// Zero results with a declared return: nothing to check, and raising
// would be an arity verdict this path does not make.
func TestNUR069EnforceReturnsEmptyResult(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{Returns: []*Type{TInteger}}
	if err := r.enforceCallBoruReturns(sig, "f", nil); err != nil {
		t.Fatalf("an empty residual must not raise: %v", err)
	}
}

// A predicate's declared return is NOT its contract: the protocol signals
// "doesn't match" with a None or Boolean-false residual whatever the body
// declares, so enforcement is suppressed while a predicate body is in
// flight. A counter rather than a bool, so a predicate whose body triggers
// another predicate restores the outer state.
func TestNUR069EnforceReturnsSuppressedInsidePredicate(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{Returns: []*Type{TInteger}}
	bad := []Value{NewNone()}
	if err := r.enforceCallBoruReturns(sig, "Positive", bad); err == nil {
		t.Fatal("outside a predicate, None under [Integer] is a violation")
	}
	r.predicateCalls++
	if err := r.enforceCallBoruReturns(sig, "Positive", bad); err != nil {
		t.Fatalf("inside a predicate the same residual is the PROTOCOL: %v", err)
	}
	r.predicateCalls--
	if err := r.enforceCallBoruReturns(sig, "Positive", bad); err == nil {
		t.Fatal("the suppression must not outlive the predicate call")
	}
}

// The end-to-end half: a CallBoru dispatch whose BODY returns the wrong
// type now fails the call, where it used to flow through. This is the arm
// the module spelling exercises in lang/spec/record.tsv, driven here
// directly so core/go's standalone gate covers the error return itself.
func TestNUR069CallBoruRejectsNonConformingReturn(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TString},
		Impl:    Boru([]Value{NewWord("n")}),
	}
	_, err := r.CallBoru(sig, []Value{NewInteger(6)}, nil)
	if err == nil {
		t.Fatal("an Integer body under a declared [String] must fail the call")
	}
	if !strings.Contains(err.Error(), "expected String") {
		t.Fatalf("the error must name the declared return, got %v", err)
	}
}

// …and the conforming twin, so the row above is proved to be about the
// TYPE rather than about CallBoru declining everything.
func TestNUR069CallBoruAcceptsConformingReturn(t *testing.T) {
	r := nur069Reg(t)
	sig := &FnSig{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewWord("n")}),
	}
	out, err := r.CallBoru(sig, []Value{NewInteger(6)}, nil)
	if err != nil {
		t.Fatalf("a conforming body must pass: %v", err)
	}
	if got, _ := AsInteger(out[len(out)-1]); got != 6 {
		t.Fatalf("result = %v, want 6", out)
	}
}
