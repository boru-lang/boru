package core

import "testing"

// Stage-5 coverage for signature.go: NormalizeSig's legacy-Patterns
// derivation, MatchSignature's ArgCount / flex-fail / map-pattern arms,
// FlexibleMatch guards, SigTypeMatches' ascription / dynamic / disjunct-
// carrier / Options arms, the TypeArgs matcher, rejectsTypeLiteral's
// carve-outs, and the CompareSignatures / RankSignatures ordering.

func TestWt5NormalizeSigLegacyPatterns(t *testing.T) {
	s := &Signature{
		Args:     []*Type{TInteger},
		Patterns: map[int]Value{0: NewInteger(0)},
	}
	NormalizeSig(s)
	if len(s.Params) != 1 || s.Params[0].Pattern == nil {
		t.Fatal("legacy Patterns must be derived into Params")
	}
	if p, ok := SigPattern(s, 0); !ok || !ValuesEqual(p, NewInteger(0)) {
		t.Fatalf("normalized pattern = %v %v", p, ok)
	}
}

func TestWt5MatchSignatureArgCountAndFlexFail(t *testing.T) {
	sigs := []Signature{{Args: []*Type{TInteger, TInteger}}}
	stack := []Value{NewInteger(1), NewInteger(2)}
	// An explicit ArgCount modifier that disagrees with the sig arity
	// skips the sig.
	if MatchSignature(sigs, stack, WordInfo{ArgCount: 1}) != nil {
		t.Fatal("ArgCount mismatch must skip the sig")
	}
	// A type mismatch fails FlexibleMatch and continues.
	sigsInt := []Signature{{Args: []*Type{TInteger}}}
	if MatchSignature(sigsInt, []Value{NewString("s")}, WordInfo{ArgCount: -1}) != nil {
		t.Fatal("flex-fail must yield no match")
	}
}

func TestWt5MatchSignatureMapPatternArms(t *testing.T) {
	sigs := []Signature{{Args: []*Type{TMap}, Patterns: map[int]Value{0: wt5KindMap("api")}}}
	// Subset match: extra keys allowed, declared pairs must be present.
	if MatchSignature(sigs, []Value{wt5KindMap("api")}, WordInfo{ArgCount: -1}) == nil {
		t.Fatal("matching structural map must be accepted")
	}
	if MatchSignature(sigs, []Value{wt5KindMap("db")}, WordInfo{ArgCount: -1}) != nil {
		t.Fatal("mismatching structural map must be rejected")
	}
}

func TestWt5FlexibleMatchGuards(t *testing.T) {
	sig := &Signature{Args: []*Type{TInteger}}
	if _, ok := FlexibleMatch(nil, sig); ok {
		t.Fatal("short value slice must not match")
	}
	if _, ok := FlexibleMatch([]Value{NewString("s")}, sig); ok {
		t.Fatal("positional type mismatch must not match")
	}
	if vals, ok := FlexibleMatch([]Value{NewInteger(1)}, sig); !ok || len(vals) != 1 {
		t.Fatal("conforming value must match")
	}
}

func TestWt5SigTypeMatchesAscription(t *testing.T) {
	// `v as T` dispatches as if tagged with the ascribed ancestor.
	v := NewInteger(5)
	(&v).SetAscribed(TNumber)
	if !SigTypeMatches(&v, TNumber) {
		t.Fatal("ascribed value must match at the ascribed type")
	}
	// The view is the ascribed tag, not the concrete one.
	w := NewInteger(5)
	(&w).SetAscribed(TString)
	if SigTypeMatches(&w, TInteger) {
		t.Fatal("ascription must override the concrete tag for matching")
	}
}

func TestWt5SigTypeMatchesDynamicDisjoint(t *testing.T) {
	// A dynamic carrier fails only provably-disjoint slots — the tand
	// probe path (neither side conforms to the other).
	v := NewCarrier(TInteger)
	SetDynamic(&v, true)
	if SigTypeMatches(&v, TString) {
		t.Fatal("dynamic(Integer) must fail a provably-disjoint String slot")
	}
	if !SigTypeMatches(&v, TNumber) {
		t.Fatal("dynamic(Integer) must match a Number slot")
	}
}

func TestWt5SigTypeMatchesDisjunctCarrier(t *testing.T) {
	// A strict disjunct carrier matches iff EVERY alternative matches.
	all := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TFloat)})
	all.Carrier = true
	if !SigTypeMatches(&all, TNumber) {
		t.Fatal("all-alternatives-match disjunct carrier must match")
	}
	mixed := NewDisjunct([]Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	mixed.Carrier = true
	if SigTypeMatches(&mixed, TInteger) {
		t.Fatal("a failing alternative must fail the whole carrier")
	}
}

func TestWt5SigTypeMatchesOptionsArms(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	// An Options type body matches an Options slot.
	if !SigTypeMatches(addrOf(NewOptionsType(fields)), TOptions) {
		t.Fatal("Options type body must match an Options slot")
	}
	// A plain concrete map matches an Options slot.
	if !SigTypeMatches(addrOf(wt5KindMap("api")), TOptions) {
		t.Fatal("concrete map must match an Options slot")
	}
	// An Options-tagged carrier matches a Map-family slot.
	if !SigTypeMatches(addrOf(NewCarrier(TOptions)), TMap) {
		t.Fatal("Options carrier must match a Map slot")
	}
}

func TestWt5SigArgMatchesTypeArgsArms(t *testing.T) {
	anySlot := &Signature{Args: []*Type{TAny}, TypeArgs: map[int]bool{0: true}}
	// Carriers are values, not types — rejected at TypeArgs slots.
	if SigArgMatches(anySlot, 0, addrOf(NewCarrier(TInteger))) {
		t.Fatal("carrier must be rejected at a TypeArgs slot")
	}
	// A bare type literal denotes its lattice node.
	if !SigArgMatches(anySlot, 0, addrOf(NewTypeLiteral(TInteger))) {
		t.Fatal("bare type literal must be accepted at a TypeArgs slot")
	}
	// DepScalar bodies are constraints, not bare literals.
	if SigArgMatches(anySlot, 0, addrOf(NewDepScalar(DepGT, NewInteger(1)))) {
		t.Fatal("DepScalar body must be rejected at a TypeArgs slot")
	}
	// A structural type body matches via its lattice family.
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	mapSlot := &Signature{Args: []*Type{TMap}, TypeArgs: map[int]bool{0: true}}
	if !SigArgMatches(mapSlot, 0, addrOf(NewRecordType(fields))) {
		t.Fatal("record type body must be accepted at a Map TypeArgs slot")
	}
	// A concrete non-type value is not a type.
	if SigArgMatches(anySlot, 0, addrOf(NewInteger(5))) {
		t.Fatal("concrete scalar must be rejected at a TypeArgs slot")
	}
}

func TestWt5RejectsTypeLiteralArms(t *testing.T) {
	if rejectsTypeLiteral(NewCarrier(TInteger), TInteger) {
		t.Fatal("carriers are values — never rejected here")
	}
	if rejectsTypeLiteral(NewTypeLiteral(TNone), TNone) {
		t.Fatal("the None literal is the canonical TNone inhabitant")
	}
	if rejectsTypeLiteral(NewTypeLiteral(TInteger), TType) {
		t.Fatal("type literals are the canonical Type inhabitants")
	}
	if !rejectsTypeLiteral(NewTypeLiteral(TInteger), TInteger) {
		t.Fatal("a type literal at a concrete-payload slot must be rejected")
	}
}

func TestWt5CompareSignaturesCoreDefaultAndBarrier(t *testing.T) {
	core := &Signature{Args: []*Type{TInteger}, CoreDefault: true}
	plain := &Signature{Args: []*Type{TInteger}}
	if CompareSignatures(core, plain) != 1 {
		t.Fatal("CoreDefault must sort strictly last")
	}
	if CompareSignatures(plain, core) != -1 {
		t.Fatal("non-CoreDefault must sort before CoreDefault")
	}

	// A piped (intermediate) barrier is an extra dispatch constraint.
	withBarrier := &Signature{Args: []*Type{TInteger, TInteger}, BarrierPos: 1}
	noBarrier := &Signature{Args: []*Type{TInteger, TInteger}}
	if CompareSignatures(withBarrier, noBarrier) != -1 {
		t.Fatal("barrier sig must sort before its barrier-less twin")
	}
	if CompareSignatures(noBarrier, withBarrier) != 1 {
		t.Fatal("barrier-less sig must sort after its barrier twin")
	}
}

func TestWt5RankSignatures(t *testing.T) {
	sigs := []Signature{
		{},                        // 0-arg fallback
		{Args: []*Type{TInteger}}, // most specific: longest arity
	}
	order := RankSignatures(sigs)
	if len(order) != 2 || order[0] != 1 || order[1] != 0 {
		t.Fatalf("RankSignatures = %v, want [1 0]", order)
	}
}
