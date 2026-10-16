package native

import (
	"fmt"
	"testing"
)

func TestFlexibleMatchPositional(t *testing.T) {
	// Positional match should always be preferred.
	vals := []Value{NewAtom("x"), NewList(nil)}
	ordered, ok := FlexibleMatch(vals, &Signature{Args: []*Type{TAtom, TList}, BarrierPos: -1})
	if !ok {
		t.Fatal("expected positional match")
	}
	_as0, _ := AsAtom(ordered[0])
	if _as0 != "x" {
		t.Errorf("expected atom x at [0], got %v", ordered[0])
	}
}

func TestFlexibleMatchNoPermutation(t *testing.T) {
	// Values in wrong positional order should NOT match — no permutation.
	vals := []Value{NewList(nil), NewAtom("x")}
	_, ok := FlexibleMatch(vals, &Signature{Args: []*Type{TAtom, TList}, BarrierPos: -1})
	if ok {
		t.Fatal("expected no match — arguments must not be permuted")
	}
}

func TestFlexibleMatchNoMatch(t *testing.T) {
	// No valid permutation exists.
	vals := []Value{NewAtom("a"), NewAtom("b")}
	_, ok := FlexibleMatch(vals, &Signature{Args: []*Type{TAtom, TList}, BarrierPos: -1})
	if ok {
		t.Fatal("expected no match for incompatible types")
	}
}

func TestFlexibleMatchPrefersLeastDisplacement(t *testing.T) {
	// When multiple permutations match, prefer fewest displacements.
	// [atom, atom, list] with types [atom, atom, list] — positional wins (0 displacements).
	vals := []Value{NewAtom("a"), NewAtom("b"), NewList(nil)}
	types := []*Type{TAtom, TAtom, TList}
	ordered, ok := FlexibleMatch(vals, &Signature{Args: types, BarrierPos: -1})
	if !ok {
		t.Fatal("expected match")
	}
	// Positional match: atoms should stay in original order.
	_as1, _ := AsAtom(ordered[0])
	if _as1 != "a" {
		_as2, _ := AsAtom(ordered[0])
		t.Errorf("[0] expected atom a, got %s", _as2)
	}
	_as3, _ := AsAtom(ordered[1])
	if _as3 != "b" {
		_as4, _ := AsAtom(ordered[1])
		t.Errorf("[1] expected atom b, got %s", _as4)
	}
}

// --- CompareSignatures tests ---

func TestCompareSignaturesZeroArgs(t *testing.T) {
	a := Signature{Args: nil, BarrierPos: -1}
	b := Signature{Args: nil, BarrierPos: -1}
	if got := CompareSignatures(&a, &b); got != 0 {
		t.Errorf("two zero-arg sigs should tie, got %d", got)
	}
}

func TestCompareSignaturesArgCountDominates(t *testing.T) {
	sig1 := Signature{Args: []*Type{TAny}, BarrierPos: -1}
	sig2 := Signature{Args: []*Type{TAny, TAny}, BarrierPos: -1}
	if c := CompareSignatures(&sig2, &sig1); c >= 0 {
		t.Errorf("2-arg should sort before 1-arg, got %d", c)
	}
}

func TestCompareSignaturesSpecificityBreaksTie(t *testing.T) {
	sigNarrow := Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: -1}
	sigWide := Signature{Args: []*Type{TScalar, TScalar}, BarrierPos: -1}
	if c := CompareSignatures(&sigNarrow, &sigWide); c >= 0 {
		t.Errorf("narrow (integer) should sort before wide (scalar), got %d", c)
	}
}

func TestCompareSignaturesMixedSpecificity(t *testing.T) {
	sig1 := Signature{Args: []*Type{TInteger, TAny}, BarrierPos: -1}
	sig2 := Signature{Args: []*Type{TAny, TAny}, BarrierPos: -1}
	if c := CompareSignatures(&sig1, &sig2); c >= 0 {
		t.Errorf("[integer,any] should sort before [any,any], got %d", c)
	}
}

func TestCompareSignaturesDeepType(t *testing.T) {
	deep := MintTestType("Number/Integer/Positive")
	sigDeep := Signature{Args: []*Type{deep}, BarrierPos: -1}
	sigShallow := Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	if c := CompareSignatures(&sigDeep, &sigShallow); c >= 0 {
		t.Errorf("deep type should sort before shallow type, got %d", c)
	}
}

// --- RankSignatures tests ---

func TestRankSignaturesEmpty(t *testing.T) {
	result := RankSignatures(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestRankSignaturesSingle(t *testing.T) {
	sigs := []Signature{{Args: []*Type{TAny}, BarrierPos: -1}}
	result := RankSignatures(sigs)
	if len(result) != 1 || result[0] != 0 {
		t.Errorf("expected [0], got %v", result)
	}
}

func TestRankSignaturesLongerFirst(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny}, BarrierPos: -1},
	}
	ranked := RankSignatures(sigs)
	want := []int{1, 2, 0}
	for i, idx := range ranked {
		if idx != want[i] {
			t.Errorf("rank[%d] = %d, want %d (ranked=%v)", i, idx, want[i], ranked)
		}
	}
}

func TestRankSignaturesNarrowerFirst(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TScalar, TScalar}, BarrierPos: -1},
		{Args: []*Type{TInteger, TInteger}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny}, BarrierPos: -1},
	}
	ranked := RankSignatures(sigs)
	if ranked[0] != 1 {
		t.Errorf("expected narrowest (integer,integer) first, got index %d", ranked[0])
	}
}

func TestRankSignaturesLengthBeatsSpecificity(t *testing.T) {
	deep := MintTestType("Number/Integer/Positive")
	sigs := []Signature{
		{Args: []*Type{deep, deep}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny}, BarrierPos: -1},
	}
	ranked := RankSignatures(sigs)
	if ranked[0] != 1 {
		t.Errorf("3-arg should beat 2-arg deep, got ranked=%v", ranked)
	}
}

func TestRankSignaturesStableForEqualOrder(t *testing.T) {
	// Two sigs that compare equal: stable sort preserves registration order.
	sig := Signature{Args: []*Type{TString}, BarrierPos: -1}
	sigs := []Signature{sig, sig}
	ranked := RankSignatures(sigs)
	if ranked[0] != 0 || ranked[1] != 1 {
		t.Errorf("expected stable order [0,1], got %v", ranked)
	}
}

func TestRankSignatures4To7Args(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny, TAny, TAny, TAny}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny, TAny, TAny}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny, TAny, TAny, TAny}, BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny, TAny, TAny, TAny, TAny}, BarrierPos: -1},
		{Args: []*Type{TInteger, TInteger, TInteger, TInteger, TInteger}, BarrierPos: -1},
	}
	ranked := RankSignatures(sigs)
	// 7-arg(3), 6-arg(2), 5-int(4), 5-any(1), 4-any(0)
	want := []int{3, 2, 4, 1, 0}
	for i, idx := range ranked {
		if idx != want[i] {
			t.Errorf("rank[%d] = %d, want %d (ranked=%v)", i, idx, want[i], ranked)
		}
	}
}

// --- MatchSignature priority tests ---

func dummyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return args, nil
}

func TestMatchSignaturePrefersMostSpecific(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TInteger, TInteger}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TScalar, TScalar}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	SortSignatures(sigs)
	stack := []Value{NewInteger(1), NewInteger(2)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	// integer,integer has highest specificity for integer args
	if !m.Sig.Args[0].Equal(TInteger) || !m.Sig.Args[1].Equal(TInteger) {
		t.Errorf("expected [integer,integer] match, got %v", m.Sig.Args)
	}
}

func TestMatchSignaturePrefersLonger(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	SortSignatures(sigs)
	stack := []Value{NewInteger(1), NewString("x"), NewBoolean(true)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if m.Sig.TotalArgs() != 3 {
		t.Errorf("expected 3-arg match, got %d args", m.Sig.TotalArgs())
	}
}

func TestMatchSignatureArgCountFilter(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TAny, TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	stack := []Value{NewInteger(1), NewInteger(2), NewInteger(3)}

	// Force 2-arg match via modifier
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: 2})
	if m == nil {
		t.Fatal("expected match")
	}
	if m.Sig.TotalArgs() != 2 {
		t.Errorf("expected 2-arg match with ArgCount filter, got %d", m.Sig.TotalArgs())
	}
}

func TestMatchSignatureSubtypeMatchesParent(t *testing.T) {
	// integer is a subtype of number; both match number pattern
	sigs := []Signature{
		{Args: []*Type{TNumber}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	stack := []Value{NewInteger(42)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("integer should match number pattern")
	}
}

func TestMatchSignatureParentDoesNotMatchChild(t *testing.T) {
	// A plain number value should NOT match an integer-only signature
	sigs := []Signature{
		{Args: []*Type{TInteger}, Impl: Go(dummyHandler), BarrierPos:

		// Create a value with type "number" (not "number/integer")
		-1},
	}

	v := NewInteger(42)
	SetParent(&v, TNumber) // override to plain number
	stack := []Value{v}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m != nil {
		t.Error("plain number should not match integer-only signature")
	}
}

func TestMatchSignatureNoMatchInsufficientStack(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TAny, TAny, TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	stack := []Value{NewInteger(1), NewInteger(2)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m != nil {
		t.Error("should not match 3-arg sig with only 2 stack values")
	}
}

func TestMatchSignatureExtraStackIgnored(t *testing.T) {
	sigs := []Signature{
		{Args: []*Type{TInteger}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	stack := []Value{NewString("extra"), NewString("extra2"), NewInteger(42)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("should match top-of-stack integer")
	}
	_as5, _ := AsInteger(m.Args[0])
	if _as5 != 42 {
		t.Errorf("expected arg 42, got %v", m.Args[0])
	}
}

func TestMatchSignatureNarrowVsWideHierarchy(t *testing.T) {
	// Test with multiple specificity levels:
	// boolean/true (3 parts) vs boolean (1 part) vs any (1 part)
	sigs := []Signature{
		{Args: []*Type{TAny}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TBoolean}, Impl: Go(dummyHandler), BarrierPos: -1},
		{Args: []*Type{TBoolean}, Impl: Go(dummyHandler), BarrierPos: -1},
	}
	SortSignatures(sigs)
	stack := []Value{NewBoolean(true)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(TBoolean) {
		t.Errorf("expected boolean/true match, got %v", m.Sig.Args[0])
	}
}

// --- MatchSignature with varying arg counts (1-7) ---

func TestMatchSignaturePriorityByArgCount(t *testing.T) {
	// Register signatures from 1 to 7 args, all TAny.
	// The longest matching signature should always win.
	for targetLen := 1; targetLen <= 7; targetLen++ {
		t.Run(fmt.Sprintf("%d_args", targetLen), func(t *testing.T) {
			var sigs []Signature
			for n := 1; n <= 7; n++ {
				args := make([]*Type, n)
				for i := range args {
					args[i] = TAny
				}
				sigs = append(sigs, Signature{Args: args, Impl: Go(dummyHandler), BarrierPos: -1})
			}
			SortSignatures(sigs)

			stack := make([]Value, targetLen)
			for i := range stack {
				stack[i] = NewInteger(int64(i + 1))
			}

			m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
			if m == nil {
				t.Fatal("expected match")
			}
			if m.Sig.TotalArgs() != targetLen {
				t.Errorf("with %d stack values, expected %d-arg match, got %d",
					targetLen, targetLen, m.Sig.TotalArgs())
			}
		})
	}
}

func TestMaxArgsLimit(t *testing.T) {
	// Exactly MaxArgs should be fine.
	args := make([]*Type, MaxArgs)
	for i := range args {
		args[i] = TAny
	}
	sig := Signature{Args: args, Impl: Go(dummyHandler), BarrierPos: -1}
	if sig.TotalArgs() != MaxArgs {
		t.Fatalf("expected %d args, got %d", MaxArgs, sig.TotalArgs())
	}
}

func TestMaxArgsExceededReturnsError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	registerIOWords(r)
	args := make([]*Type, MaxArgs+1)
	for i := range args {
		args[i] = TAny
	}
	r.Register("toobig", Signature{Args: args, Impl: Go(dummyHandler), BarrierPos: -1})
	if r.Err() == nil {
		t.Fatal("expected error for signature exceeding MaxArgs")
	}
}
