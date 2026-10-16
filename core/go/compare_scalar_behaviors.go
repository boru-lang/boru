package core

import (
	"math"
	"math/big"
	"strings"
)

// Comparer implementations for the kernel scalar types and Word. Each embeds
// defaultBehavior so Match/Format/Equal stay at the kernel default;
// the only addition is the Compare method, which makes the type
// orderable for `lt`/`gt`/`lte`/`gte`/`sort`. Descendants of a
// scalar (e.g. Integer < Number, EmptyString < String) inherit the
// Comparer via the lattice walk in CompareValues — no per-subtype
// registration needed.
//
// The Scalar root itself carries scalarCompareBehavior — the
// cross-branch comparator. It orders values from different branches
// (e.g. Integer-vs-String) by a fixed branch precedence and is
// reached only when the LCA walk finds no branch-level Comparer.

// numberCompareBehavior compares any pair of values rooted under
// Scalar/Number (Integer, Float, or their dep variants) via the
// canonical float promotion in AsNumber.
type numberCompareBehavior struct{ defaultBehavior }

// formatDelegate marks every scalar Comparer Behavior in this file
// as "Format is the kernel default". Value.String walks the parent
// chain looking for a *real* Format override; tagged behaviours are
// skipped so the kernel renderer runs unchanged. See
// formatDelegatesToDefault in value.go for the dispatch logic.
func (numberCompareBehavior) formatDelegate()  {}
func (stringCompareBehavior) formatDelegate()  {}
func (booleanCompareBehavior) formatDelegate() {}
func (atomCompareBehavior) formatDelegate()    {}
func (scalarCompareBehavior) formatDelegate()  {}
func (wordCompareBehavior) formatDelegate()    {}

// litVsConcreteOrder applies the type-literal-first rule at the top
// of every family Comparer: when exactly one side is a bare type
// literal (Data==nil, !Carrier) and the other carries a concrete
// payload, the type literal sorts first.
//
// This places every type literal strictly BELOW every concrete value
// in the same family — including the family's zero-valued inhabitant
// (`Integer < 0`, `String < ""`, `Boolean < false`). Without this
// rule the family Comparer would read the type literal's nil payload
// as a zero value and tie with `0` / `""` / `false`, putting two
// distinct lattice nodes in the same equivalence class and
// violating the strict-total-order property.
//
// Returns (sign, true) when exactly one side is a literal. Returns
// (0, false) when both are literals or both concrete — the caller
// then picks the appropriate branch (litVsLitOrder for both-
// literal, the family's value compare for both-concrete).
func litVsConcreteOrder(a, b Value) (int, bool) {
	aLit := DataOf(&a) == nil && !IsCarrier(&a)
	bLit := DataOf(&b) == nil && !IsCarrier(&b)
	if aLit && !bLit {
		return -1, true
	}
	if !aLit && bLit {
		return 1, true
	}
	return 0, false
}

// litVsLitOrder orders two bare type literals by lattice node
// position via compareTypes (Rank → depth → name → ID). Used by the
// family Comparers when both inputs are type literals — in that
// case the value-payload compare would tie (both read as zero) so
// we fall back to lattice identity.
//
// The two values are by-value copies of their lattice nodes, so &a
// and &b ARE the nodes for compareTypes' purposes.
func litVsLitOrder(a, b Value) int {
	return compareTypes(&a, &b)
}

// scalarCmpPrologue is the shared preamble every per-family scalar
// Comparer opens with, in the load-bearing order:
//
//  1. DepScalar opt-out — DepScalar values share a scalar Parent with
//     concrete values but carry a DepScalarInfo payload rather than a
//     scalar; they have no family ordering. ErrNoComparer signals
//     "I don't apply" so CompareValues falls through to the lattice +
//     structural compare (which tie-breaks on canonical form).
//  2. Type-literal-first — a bare type literal sorts strictly below
//     every concrete inhabitant of the same family.
//  3. Literal-vs-literal — two bare literals order by lattice node
//     (Rank → depth → name → ID), since the value-payload compare
//     would tie on zeros.
//
// done=true means the prologue settled the comparison (result/err are
// final); done=false hands over to the family's own value comparison.
// Keeping the sequence in ONE place stops the per-family copies from
// drifting (a missed DepScalar guard silently compares zero values).
func scalarCmpPrologue(a, b Value) (result int, done bool, err error) {
	if a.IsDepScalar() || b.IsDepScalar() {
		return 0, true, ErrNoComparer
	}
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c, true, nil
	}
	if DataOf(&a) == nil && DataOf(&b) == nil {
		return litVsLitOrder(a, b), true, nil
	}
	return 0, false, nil
}

func (numberCompareBehavior) Compare(a, b Value) (int, error) {
	if c, done, err := scalarCmpPrologue(a, b); done {
		return c, err
	}
	// Arbitrary-precision operand on either side: compare EXACTLY via
	// apd.Decimal so cross-leaf magnitude equality holds (1 == 0d1 == 1.0
	// == 0d1.0) with no binary error. A Float that is not an exact decimal
	// keeps its true binary value, so e.g. Float 0.1 ≠ BigDecimal 0d0.1 —
	// the mathematically honest result. A non-finite Float opposite a Big
	// value takes the IEEE total-order slot (NaN greatest, ±Inf extremes).
	if numIsBig(a) || numIsBig(b) {
		if r, ok := compareNonFiniteFloat(a, b); ok {
			return r, nil
		}
		ar, aok := toRatExact(a)
		br, bok := toRatExact(b)
		if aok && bok {
			if c := ar.Cmp(br); c != 0 {
				return c, nil
			}
			// Magnitude tie — apply the signed-zero totalOrder slot here
			// too, or transitivity breaks across the 0 / 0d0 / -0.0
			// triangle (-0.0 < 0 and 0 = 0d0 force -0.0 < 0d0). Only a
			// Float -0.0 counts as negative zero: big.Rat has no signed
			// zero, so a BigDecimal "-0" spelling (apd's Negative flag
			// surviving Text('f') → "-0") cannot leak through — every
			// BigDecimal/BigInteger/Integer zero slots as +0.
			return signedZeroOrder(a, b), nil
		}
		return 0, ErrNoComparer
	}
	// Two int64-backed Integers compare EXACTLY as int64. Projecting them
	// through float64 (the cross-leaf path below) silently rounds any
	// magnitude above 2^53, which collapses distinct integers to "equal"
	// and breaks the sort order. Only the Integer/Integer case is exact in
	// int64; a Float on either side keeps the float projection, where the
	// float is already the honest representation of that operand.
	if ParentOf(&a).ConformsTo(TInteger) && ParentOf(&b).ConformsTo(TInteger) {
		ai, _ := AsInteger(a)
		bi, _ := AsInteger(b)
		switch {
		case ai < bi:
			return -1, nil
		case ai > bi:
			return 1, nil
		default:
			return 0, nil
		}
	}
	af, _ := AsNumber(a)
	bf, _ := AsNumber(b)
	// IEEE-754 has no native order for NaN, but `cmp`/`tcmp`/`sort` need a
	// TOTAL order or the sort comparator violates transitivity (the bug
	// this fixes: NaN previously fell through to 0, comparing "equal" to
	// everything, which silently left lists unsorted). Per the IEEE
	// totalOrder predicate (§5.10), NaN sorts after every non-NaN value
	// (greatest); two NaNs tie. The relational words lt/lte/gt/gte do NOT
	// use this slot — they apply the IEEE *unordered* rule (always false)
	// via numericUnordered in compare.go — so this affects only the total
	// order that sort and the collection words consume.
	aNaN, bNaN := math.IsNaN(af), math.IsNaN(bf)
	if aNaN || bNaN {
		switch {
		case aNaN && bNaN:
			return 0, nil
		case aNaN:
			return 1, nil
		default:
			return -1, nil
		}
	}
	switch {
	case af < bf:
		return -1, nil
	case af > bf:
		return 1, nil
	default:
		// Magnitude tie. IEEE totalOrder (§5.10) places -0 strictly
		// before +0, so the signed zeros must not tie in the total order
		// (they did before NUR013 — `sort [0.0 -0.0]` was a no-op).
		// Only a Float -0.0 counts as negative zero; an Integer 0 slots
		// as +0. The relational words lt/lte/gt/gte do NOT see this
		// order — the signedZeroPair guard in compare.go keeps them on
		// the IEEE §5.11 rule (-0 == +0), and eq/deq compare floats via
		// Go ==, where -0.0 == 0.0 already holds.
		return signedZeroOrder(a, b), nil
	}
}

// signedZeroOrder is the totalOrder (§5.10) tiebreak for a magnitude-
// equal numeric pair: the Float negative zero sorts strictly before
// every positive zero (Float 0.0, Integer 0, BigInteger/BigDecimal
// zeros), and any other magnitude-equal pair — including two -0.0s —
// ties. Callers invoke it only after the magnitudes compared equal, so
// a non-zero pair always hits the aNeg == bNeg (both false) arm.
func signedZeroOrder(a, b Value) int {
	aNeg, bNeg := isNegZeroFloat(&a), isNegZeroFloat(&b)
	switch {
	case aNeg == bNeg:
		return 0
	case aNeg:
		return -1
	default:
		return 1
	}
}

// isNegZeroFloat reports whether v is the Float negative zero — the
// ONLY value that slots as -0 in the total order. Integer 0 and the
// Big zeros (0n0, 0d0 — including a BigDecimal negative-zero spelling)
// are not Floats and slot as +0.
func isNegZeroFloat(v *Value) bool {
	if !ParentOf(v).ConformsTo(TFloat) {
		return false
	}
	f, err := AsFloat(*v)
	return err == nil && f == 0 && math.Signbit(f)
}

// numIsBig reports whether v is an arbitrary-precision numeric leaf.
func numIsBig(v Value) bool {
	return ParentOf(&v).ConformsTo(TBigInteger) || ParentOf(&v).ConformsTo(TBigDecimal)
}

// toRatExact projects a finite numeric value to an exact *big.Rat:
// Integer/BigInteger are exact, BigDecimal parses its plain-decimal text
// exactly (e.g. 0d0.1 → 1/10), and a finite Float uses big.Rat.SetFloat64
// which captures its EXACT binary value (so Float 0.1 → ...00055.../2^… ≠
// 1/10). Comparing in this domain gives the mathematically honest
// cross-leaf result — matching Python's `Decimal('0.1') == 0.1 → False`.
// Returns false for a non-finite Float (handled by compareNonFiniteFloat)
// or a non-number.
func toRatExact(v Value) (*big.Rat, bool) {
	switch {
	case ParentOf(&v).ConformsTo(TBigDecimal):
		d, err := AsBigDecimal(v)
		if err != nil {
			return nil, false
		}
		// SetString fails for non-finite text ("Infinity"/"NaN"), so a
		// non-finite BigDecimal correctly returns ok=false here.
		r, ok := new(big.Rat).SetString(d.Text('f'))
		return r, ok
	case ParentOf(&v).ConformsTo(TBigInteger):
		n, err := AsBigInteger(v)
		if err != nil {
			return nil, false
		}
		return new(big.Rat).SetInt(n), true
	case ParentOf(&v).ConformsTo(TFloat):
		f, err := AsFloat(v)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return new(big.Rat).SetFloat64(f), true
	case ParentOf(&v).ConformsTo(TInteger):
		n, err := AsInteger(v)
		if err != nil {
			return nil, false
		}
		return new(big.Rat).SetInt64(n), true
	}
	return nil, false
}

// compareNonFiniteFloat handles a comparison where exactly one operand is
// a non-finite Float (NaN / ±Inf) and the other is finite, giving the
// non-finite value its IEEE total-order slot: NaN sorts after everything
// (greatest), +Inf above any finite value, -Inf below. Returns ok=false
// when neither operand is a non-finite Float.
func compareNonFiniteFloat(a, b Value) (int, bool) {
	if r, nf := nonFiniteRank(a); nf {
		return r, true // a is non-finite, b is finite
	}
	if r, nf := nonFiniteRank(b); nf {
		return -r, true // b is non-finite, a is finite
	}
	return 0, false
}

// nonFiniteRank returns +1 for NaN/+Inf, -1 for -Inf, and ok=false when v
// is not a non-finite Float.
func nonFiniteRank(v Value) (int, bool) {
	if !ParentOf(&v).ConformsTo(TFloat) {
		return 0, false
	}
	f, err := AsFloat(v)
	if err != nil {
		return 0, false
	}
	switch {
	case math.IsNaN(f):
		return 1, true
	case math.IsInf(f, 1):
		return 1, true
	case math.IsInf(f, -1):
		return -1, true
	}
	return 0, false
}

// stringCompareBehavior orders strings lexicographically (UTF-8 byte
// order via strings.Compare).
type stringCompareBehavior struct{ defaultBehavior }

func (stringCompareBehavior) Compare(a, b Value) (int, error) {
	if c, done, err := scalarCmpPrologue(a, b); done {
		return c, err
	}
	as, _ := AsString(a)
	bs, _ := AsString(b)
	return strings.Compare(as, bs), nil
}

// booleanCompareBehavior orders booleans false < true.
type booleanCompareBehavior struct{ defaultBehavior }

func (booleanCompareBehavior) Compare(a, b Value) (int, error) {
	if c, done, err := scalarCmpPrologue(a, b); done {
		return c, err
	}
	ab, _ := AsBoolean(a)
	bb, _ := AsBoolean(b)
	switch {
	case ab == bb:
		return 0, nil
	case !ab:
		return -1, nil
	default:
		return 1, nil
	}
}

// atomCompareBehavior orders atoms lexicographically by name.
type atomCompareBehavior struct{ defaultBehavior }

func (atomCompareBehavior) Compare(a, b Value) (int, error) {
	if c, done, err := scalarCmpPrologue(a, b); done {
		return c, err
	}
	as, _ := AsAtom(a)
	bs, _ := AsAtom(b)
	return strings.Compare(as, bs), nil
}

// wordCompareBehavior orders Word values lexicographically by their
// rendered form. Word — like String and Atom — is a name-like type
// whose deliberate comparison basis is the text itself; this is one
// of the few places Value.String is used as an ordering key.
type wordCompareBehavior struct{ defaultBehavior }

func (wordCompareBehavior) Compare(a, b Value) (int, error) {
	// The DepScalar arm of the prologue is unreachable for Word-family
	// pairs (DepScalar payloads carry scalar Parents, never Word), so
	// sharing the full prologue is a no-op there and keeps the five
	// family Comparers uniform.
	if c, done, err := scalarCmpPrologue(a, b); done {
		return c, err
	}
	return strings.Compare(a.String(), b.String()), nil
}

// scalarCompareBehavior is the Comparer on the abstract Scalar root.
// It gives cross-family scalar pairs (e.g. Integer-vs-String) a defined
// order by their unified lattice Rank — the same key CompareValues'
// fallback uses, so the Scalar root needs no private branch ladder and,
// since the bare Scalar root literal carries a real Rank, it never has
// to bail.
//
// CompareValues reaches it only for cross-family pairs: the LCA walk
// stops at a branch root's own Comparer (Number/String/Boolean/Atom,
// and the Micron family Comparer for Pathon/Emailon/Urlon pairs) for
// any same-family pair. The one same-Rank pair that does reach here is
// DepScalar-vs-DepScalar (their wrapper Comparers opt out), settled by
// comparePathons' canonical-form branch below.
type scalarCompareBehavior struct{ defaultBehavior }

func (scalarCompareBehavior) Compare(a, b Value) (int, error) {
	// Cross-family scalar pairs (e.g. Boolean-vs-Integer) and the
	// same-type DepScalar case both land here. For cross-family pairs
	// the Rank discriminator must own the result — applying the type-
	// literal-first rule here would override Rank (e.g. `true cmp
	// Integer` should stay -1 via Rank, not flip to +1 because
	// Integer is a literal).
	if c := compareTypes(ValueType(a), ValueType(b)); c != 0 {
		return c, nil
	}
	// Same scalar type with no family Comparer of its own — DepScalar
	// pairs order by canonical form via comparePathons' first branch.
	return comparePathons(a, b), nil
}

// comparePathons orders two Pathon values by the NATURAL total order
// (NUR012): volume/drive first, then absolute paths before relative
// ones, then segment by segment in FORWARD lexical order, and finally
// shorter-prefix-first — so a directory sorts immediately before its
// contents (`a`, `a/b`, `a/z`, `b/a`), the order a filesystem walk
// prints. This replaced the historical reverse-lex order, whose
// rationale was argued nowhere. scalarCompareBehavior routes the
// Pathon-vs-Pathon case here.
func comparePathons(a, b Value) int {
	// DepScalar pair: order by canonical form (forward lex on the
	// rendered "(base op bound)" string). They have no numeric
	// ordering of their own — their Comparers signal ErrNoComparer
	// so we land here — and the spec's "tie-breaks on canonical
	// form" rule wants forward lex, not the Pathon-reverse rule.
	if a.IsDepScalar() && b.IsDepScalar() {
		return strings.Compare(a.String(), b.String())
	}
	// Type-literal-first rule for the Pathon family: the bare `Pathon`
	// type literal sorts strictly below every concrete path value.
	// Done here (not in scalarCompareBehavior) so the rule applies
	// only inside the Pathon family — cross-family scalar pairs route
	// through scalarCompareBehavior's Rank-only path.
	if c, ok := litVsConcreteOrder(a, b); ok {
		return c
	}
	ap, aerr := AsPathon(a)
	bp, berr := AsPathon(b)
	if aerr != nil || berr != nil {
		// Two type literals (both Data==nil) or two non-Pathon
		// concretes — fall back to lattice identity then to
		// rendered form.
		if DataOf(&a) == nil && DataOf(&b) == nil {
			return litVsLitOrder(a, b)
		}
		return strings.Compare(a.String(), b.String())
	}
	// 1. Volume/drive — driveless "" sorts first, drive/UNC paths group
	//    by volume before anything else.
	if c := strings.Compare(ap.Volume, bp.Volume); c != 0 {
		return c
	}
	// 2. Absolute paths sort before relative ones.
	switch {
	case ap.Abs && !bp.Abs:
		return -1
	case !ap.Abs && bp.Abs:
		return 1
	}
	// 3. Segment-by-segment FORWARD lexical comparison. Comparing the
	//    parts directly, rather than the "/"-joined render, keeps the
	//    separator byte from skewing the order.
	n := len(ap.Parts)
	if len(bp.Parts) < n {
		n = len(bp.Parts)
	}
	for i := 0; i < n; i++ {
		if c := strings.Compare(ap.Parts[i], bp.Parts[i]); c != 0 {
			return c
		}
	}
	// 4. Shared prefix — the shorter path (the directory) sorts first.
	switch {
	case len(ap.Parts) < len(bp.Parts):
		return -1
	case len(ap.Parts) > len(bp.Parts):
		return 1
	}
	return 0
}

// init attaches the scalar Comparers to their owning kernel types.
// The Builtin TypeTable has been populated by the typetable.go init
// at this point (same package, lexicographic file order isn't
// guaranteed but Builtin is a package-level var initialised via the
// init() in typetable.go which Go runs before this file's init
// unless there's a dependency — see below).
//
// Run order: package vars are initialised before any init() runs,
// so Builtin is constructed (and TNumber/TString/etc. are non-nil)
// before this init fires. We assign Behaviors directly on the *Type
// values; CompareValues consults Behavior at call time.
func init() {
	TNumber.ensureTMeta().Behavior = numberCompareBehavior{}
	TString.ensureTMeta().Behavior = stringCompareBehavior{}
	TBoolean.ensureTMeta().Behavior = booleanCompareBehavior{}
	TAtom.ensureTMeta().Behavior = atomCompareBehavior{}
	TScalar.ensureTMeta().Behavior = scalarCompareBehavior{}
	TWord.ensureTMeta().Behavior = wordCompareBehavior{}
}

// Exported spellings of the family-Comparer doctrine helpers, for
// family Comparers that live OUTSIDE the kernel (the Micron family's,
// in basic/go — the first external family Comparer). The rule they
// implement is documented in CLAUDE.md "Comparison & Ordering": every
// family Comparer opens with the type-literal-first probe; Pathon
// pairs keep the verbatim segment order.

// LitVsConcreteOrder is litVsConcreteOrder for external Comparers.
func LitVsConcreteOrder(a, b Value) (int, bool) { return litVsConcreteOrder(a, b) }

// LitVsLitOrder is litVsLitOrder for external Comparers.
func LitVsLitOrder(a, b Value) int { return litVsLitOrder(a, b) }

// ComparePathons is comparePathons for external Comparers — the
// kernel-owned Pathon segment order (the payload stays kernel).
func ComparePathons(a, b Value) int { return comparePathons(a, b) }
