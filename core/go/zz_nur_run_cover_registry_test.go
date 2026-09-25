package core

// Core-suite pins for the NUR run's registry additions: the type-part
// snapshot / forget pair (NUR167), the strict named-call return count
// (NUR191), and RunPredicate's run-time memo and analysis-time real run
// (NUR102 / NUR141) with its purity screen. Positive and negative cases
// are paired throughout.

import (
	"errors"
	"strings"
	"testing"
)

// nrcRegisterNative registers a one-signature native word over args.
func nrcRegisterNative(r *Registry, name string, args []*Type, returns []*Type, fn func(args []Value) ([]Value, error), opts ...GoOpt) {
	r.RegisterNativeFunc(NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args: args,
			Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return fn(a)
			}, opts...),
			Returns:    returns,
			BarrierPos: -1,
		}},
	})
}

// nrcPredicate builds a one-param predicate fn value over input whose body
// is body.
func nrcPredicate(r *Registry, input *Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name:     "NrcPred",
		Registry: r,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: input}},
			Impl:       Boru(body),
			BarrierPos: 0,
		}},
	})
}

// --- TypePartsSnapshot / ForgetTypePartsSince (NUR167) ----------------------------

// Pins the snapshot/forget pair: a nil registry (or one with no type table)
// snapshots nil and forgets nothing; a part reserved before the snapshot is
// kept, a part reserved since whose type binding is still live is kept, and
// a part reserved since with no live binding is forgotten (and counted).
func TestNurRunTypePartsSnapshotAndForget(t *testing.T) {
	if (*Registry)(nil).TypePartsSnapshot() != nil || (&Registry{}).TypePartsSnapshot() != nil {
		t.Fatal("no type table: no snapshot")
	}
	if n := (*Registry)(nil).ForgetTypePartsSince(nil); n != 0 {
		t.Fatalf("nil registry forgets nothing, got %d", n)
	}
	if n := (&Registry{}).ForgetTypePartsSince(map[string]bool{}); n != 0 {
		t.Fatalf("no type table forgets nothing, got %d", n)
	}

	r := newTestRegistry(t)
	r.RegisterPart("NrcBefore")
	snap := r.TypePartsSnapshot()
	if !snap["NrcBefore"] {
		t.Fatalf("snapshot must carry the reserved part: %v", snap)
	}
	// The snapshot is a copy: a later reservation does not appear in it.
	r.RegisterPart("NrcLive")
	r.RegisterPart("NrcGone")
	if snap["NrcLive"] || snap["NrcGone"] {
		t.Fatal("snapshot must not alias the live part set")
	}
	r.Defs.PushType("NrcLive", r.Types.MintType("NrcLive", TInteger), NewTypeLiteral(TInteger))

	if n := r.ForgetTypePartsSince(snap); n != 1 {
		t.Fatalf("want exactly one part forgotten, got %d", n)
	}
	if !r.IsKnownPart("NrcBefore") {
		t.Fatal("a part in the snapshot must be kept")
	}
	if !r.IsKnownPart("NrcLive") {
		t.Fatal("a part still bound as a type must be kept")
	}
	if r.IsKnownPart("NrcGone") {
		t.Fatal("a part reserved since with no live binding must be forgotten")
	}
}

// --- CallBoruStrict / NamedFnReturnCount (NUR191) -----------------------------------

// Pins that a NAMED call through the CallBoru seam enforces the declared
// return COUNT (the frame's error, naming the fn), while the plain seam
// keeps its trim-don't-raise discipline and a matching count passes.
func TestNurRunCallBoruStrictReturnCount(t *testing.T) {
	r := newTestRegistry(t)
	twoForOne := &FnSig{Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(1), NewInteger(2)})}

	_, err := r.CallBoruStrict(twoForOne, nil, nil, "nrcTwo", SrcPos{Row: 4, Col: 1})
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "type_error" || !strings.Contains(ae.Detail, "nrcTwo") {
		t.Fatalf("strict call must raise the count error, got %v", err)
	}

	// Negative: the non-strict seam does not raise on the count.
	if _, err := r.CallBoruNamed(twoForOne, nil, nil, "nrcTwo"); err != nil {
		t.Fatalf("the plain seam must not raise the count: %v", err)
	}
	// Negative: the right count passes the strict seam.
	one := &FnSig{Returns: []*Type{TInteger}, Impl: Boru([]Value{NewInteger(1)})}
	out, err := r.CallBoruStrict(one, nil, nil, "nrcOne", SrcPos{})
	if err != nil || len(out) != 1 {
		t.Fatalf("a matching count must pass: %v %v", out, err)
	}

	// NamedFnReturnCount directly: undeclared returns and analysis mode
	// check nothing.
	if err := r.NamedFnReturnCount(&FnSig{}, "u", "", []Value{NewInteger(1), NewInteger(2)}, SrcPos{}); err != nil {
		t.Fatalf("undeclared returns are unchecked: %v", err)
	}
	r.Check.Mode = true
	t.Cleanup(func() { r.Check.Mode = false })
	if err := r.NamedFnReturnCount(twoForOne, "a", "", []Value{NewInteger(1), NewInteger(2)}, SrcPos{}); err != nil {
		t.Fatalf("a check-mode dispatch models the returns: %v", err)
	}
}

// --- predMemoKey / ClearPredMemo (NUR102) -------------------------------------------

// Pins predMemoKey: a constraint with an ID keys on it; one without an ID
// keys on its boru body's implementation identity; one without an ID whose
// first signature is not boru-bodied (or has none) cannot be keyed.
func TestNurRunPredMemoKey(t *testing.T) {
	cand := NewInteger(4)

	withID := NewFunction(FnDefInfo{Signatures: []Signature{{Impl: Boru([]Value{NewBoolean(true)})}}})
	withID.ID = "nrc-pred-id"
	if got := predMemoKey(withID, cand); !strings.HasPrefix(got, "nrc-pred-id|") {
		t.Fatalf("an ID'd constraint keys on its ID: %q", got)
	}

	boru := NewFunction(FnDefInfo{Signatures: []Signature{{Impl: Boru([]Value{NewBoolean(true)})}}})
	boru.ID = ""
	if got := predMemoKey(boru, cand); !strings.HasPrefix(got, "impl:") {
		t.Fatalf("an ID-less boru predicate keys on its impl: %q", got)
	}

	native := NewFunction(FnDefInfo{Signatures: []Signature{{Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return nil, nil
	})}}})
	native.ID = ""
	if got := predMemoKey(native, cand); got != "" {
		t.Fatalf("an ID-less native predicate cannot be keyed: %q", got)
	}
	bare := NewFunction(FnDefInfo{})
	bare.ID = ""
	if got := predMemoKey(bare, cand); got != "" {
		t.Fatalf("a signature-less predicate cannot be keyed: %q", got)
	}
}

// Pins the run-time memo: the second ask of the same predicate over the same
// candidate is answered without re-running the body (both verdicts), and
// ClearPredMemo forgets it so the next ask runs again.
func TestNurRunPredicateMemoAndClear(t *testing.T) {
	(*Registry)(nil).ClearPredMemo() // must not panic
	r := newTestRegistry(t)
	r.ClearPredMemo() // no memo yet: a no-op

	runs := 0
	nrcRegisterNative(r, "nrc-pred-count", []*Type{TInteger}, []*Type{TBoolean}, func(a []Value) ([]Value, error) {
		runs++
		n, _ := AsInteger(a[0])
		return []Value{NewBoolean(n%2 == 0)}, nil
	})
	even := nrcPredicate(r, TInteger, []Value{NewWord("nrc-pred-count"), NewWord("n")})

	for _, tc := range []struct {
		cand int64
		want bool
	}{{4, true}, {5, false}} {
		runs = 0
		for i := 0; i < 2; i++ {
			_, matched, err := r.RunPredicate(even, NewInteger(tc.cand))
			if err != nil || matched != tc.want {
				t.Fatalf("%d: got %v %v, want %v", tc.cand, matched, err, tc.want)
			}
		}
		if runs != 1 {
			t.Fatalf("%d: the memo must answer the second ask, body ran %d times", tc.cand, runs)
		}
	}
	r.ClearPredMemo()
	runs = 0
	if _, matched, err := r.RunPredicate(even, NewInteger(4)); err != nil || !matched || runs != 1 {
		t.Fatalf("after ClearPredMemo the body must run again: %v %v runs=%d", matched, err, runs)
	}
}

// --- RunPredicate under analysis (NUR141) -------------------------------------------

// Pins RunPredicate's analysis arms: a carrier candidate and an impure body
// (one reading a user def) are admitted without the verdict being trusted; a
// pure body over a concrete candidate runs for real, and admits only when
// that run errors — a pure body's clean refusal is a refusal.
func TestNurRunRunPredicateAnalysisArms(t *testing.T) {
	r := newTestRegistry(t)
	nrcRegisterNative(r, "nrc-pred-boom", []*Type{TInteger}, []*Type{TBoolean}, func([]Value) ([]Value, error) {
		return nil, errors.New("nrc predicate boom")
	})
	r.Defs.Push("nrclimit", NewBoolean(false))

	refuse := nrcPredicate(r, TInteger, []Value{NewBoolean(false)})
	impure := nrcPredicate(r, TInteger, []Value{NewWord("nrclimit")})
	raising := nrcPredicate(r, TInteger, []Value{NewWord("nrc-pred-boom"), NewWord("n")})

	done := r.Check.Begin()
	defer done()

	if _, matched, err := r.RunPredicate(refuse, NewCarrier(TInteger)); err != nil || !matched {
		t.Fatalf("a carrier candidate must be admitted: %v %v", matched, err)
	}
	if _, matched, err := r.RunPredicate(impure, NewInteger(3)); err != nil || !matched {
		t.Fatalf("an impure body must be admitted: %v %v", matched, err)
	}
	if out, matched, err := r.RunPredicate(raising, NewInteger(3)); err != nil || !matched || !ValuesEqual(out, NewInteger(3)) {
		t.Fatalf("a pure body whose run errors must admit the candidate: %v %v %v", out, matched, err)
	}
	// Negative: the pure refusal runs for real and refuses.
	if _, matched, err := r.RunPredicate(refuse, NewInteger(3)); err != nil || matched {
		t.Fatalf("a pure refusal must refuse under analysis: %v %v", matched, err)
	}
	if !r.Check.Mode {
		t.Fatal("the real run must restore check mode")
	}
}

// Pins predicateBodyPure's walk: the parameter and native words are pure,
// a user def is not, and the screen recurses into nested lists.
func TestNurRunPredicateBodyPure(t *testing.T) {
	r := newTestRegistry(t)
	nrcRegisterNative(r, "nrc-pure-native", []*Type{TInteger}, []*Type{TBoolean}, func([]Value) ([]Value, error) {
		return []Value{NewBoolean(true)}, nil
	})
	r.Defs.Push("nrcuserval", NewInteger(3))
	sig := func(body ...Value) *FnSig {
		return &FnSig{Params: []FnParam{{Name: "n", Type: TInteger}}, Impl: Boru(body)}
	}
	for _, tc := range []struct {
		name string
		sig  *FnSig
		want bool
	}{
		{"param and native", sig(NewWord("nrc-pure-native"), NewWord("n")), true},
		{"unbound word", sig(NewWord("nrc-unbound")), true},
		{"nested pure list", sig(NewList([]Value{NewWord("n"), NewInteger(1)})), true},
		{"user def", sig(NewWord("n"), NewWord("nrcuserval")), false},
		{"user def nested in a list", sig(NewList([]Value{NewWord("nrcuserval")})), false},
		{"no params", &FnSig{Impl: Boru([]Value{NewWord("nrcuserval")})}, false},
	} {
		if got := predicateBodyPure(r, tc.sig); got != tc.want {
			t.Errorf("%s: predicateBodyPure = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Pins userDefined: an unbound name and a native (Go-implemented) fn are
// not user definitions; a plain value def and a boru-bodied fn are.
func TestNurRunUserDefined(t *testing.T) {
	r := newTestRegistry(t)
	goImpl := Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) { return nil, nil })
	r.Defs.Push("nrcval", NewInteger(1))
	r.Defs.Push("nrcgofn", NewFunction(FnDefInfo{Signatures: []Signature{{Impl: goImpl}}}))
	r.Defs.Push("nrcborufn", NewFunction(FnDefInfo{Signatures: []Signature{
		{Impl: goImpl},
		{Impl: Boru([]Value{NewInteger(1)})},
	}}))

	if userDefined(r, "nrc-never-bound") {
		t.Error("an unbound name is not a user definition")
	}
	if userDefined(r, "nrcgofn") {
		t.Error("a native-only fn is not a user definition")
	}
	if !userDefined(r, "nrcval") {
		t.Error("a plain value def is a user definition")
	}
	if !userDefined(r, "nrcborufn") {
		t.Error("a fn with a boru body is a user definition")
	}
}
