package core

import (
	"strings"
	"testing"
)

// constructRecorder is the inactive recorder keeping what the refinement
// constructors and install sites tell the compile pass (NUR231): the run-time
// construct latch, the remembered consts, and the declines.
type constructRecorder struct {
	inactiveEmit
	constructs int
	remembered []Value
	declined   []string
}

func (c *constructRecorder) NoteRuntimeConstruct()          { c.constructs++ }
func (c *constructRecorder) RememberOriginal(v Value)       { c.remembered = append(c.remembered, v) }
func (c *constructRecorder) MarkUncompilable(reason string) { c.declined = append(c.declined, reason) }

func analysingRegistry(t *testing.T) (*Registry, *constructRecorder) {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true
	rec := &constructRecorder{}
	r.Check.Emit = rec
	return r, rec
}

// zzFormatBehavior is a base whose Formatter reads a VALUE of the base, the
// way Bytes' does — it has no refinement to read.
type zzFormatBehavior struct{ defaultBehavior }

func (zzFormatBehavior) Format(Value) string { return "Zz<?>" }

// TestRefinementBaseIsDeclared pins NUR009's capability: a type is a
// refinement base because it DECLARED itself one, not because a resolver
// lists it — core's six leaves are declared, a subtype refines as its
// declaring ancestor, an undeclared type is no base, and a type declared
// later (basic's Bytes, here a fresh node) becomes one.
func TestRefinementBaseIsDeclared(t *testing.T) {
	for _, tc := range []struct{ t, want *Type }{
		{TInteger, TInteger}, {TFloat, TFloat}, {TNumber, TNumber},
		{TString, TString}, {TBoolean, TBoolean}, {TAtom, TAtom},
		{TList, nil}, {TMap, nil}, {TScalar, nil}, // undeclared: no base
	} {
		if got := canonicalBaseType(tc.t); got != tc.want {
			t.Errorf("canonicalBaseType(%s) = %v, want %v", tc.t, got, tc.want)
		}
	}
	// A subtype refines as its declaring ancestor: 'abc' is a ProperString.
	if got := canonicalBaseType(NewString("abc").Parent); got != TString {
		t.Errorf("a String subtype's base = %v, want String", got)
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	zz := r.Types.MintType("Zz", TScalar)
	child := r.Types.MintType("ZzChild", zz)
	if canonicalBaseType(zz) != nil || canonicalBaseType(child) != nil {
		t.Fatal("an undeclared type is no refinement base")
	}
	DeclareRefinementBase(zz)
	DeclareRefinementBase(nil) // a failed registration declares nothing
	if canonicalBaseType(zz) != zz || canonicalBaseType(child) != zz {
		t.Fatalf("a declared type is its own base and its subtypes': %v %v", canonicalBaseType(zz), canonicalBaseType(child))
	}
	if dep := NewDepScalar(DepGT, NewValueRaw(child, IntPayload{N: 1})); !dep.IsDepScalar() || dep.Parent != zz {
		t.Fatalf("a bound under a declared base builds a refinement over it: %v", dep)
	}
}

// TestRefinementRendersOverAFormatterBase pins the display half of NUR009: a
// refinement renders in the comparison vocabulary even when its base has a
// Formatter (Bytes' printed `Bytes<?>` for every Bytes refinement, and the
// compile pass's const pool, keyed on that rendering, merged two of them).
// A plain value of the base still renders through the Formatter.
func TestRefinementRendersOverAFormatterBase(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	zz := r.Types.MintTypeWithBehavior("Zz", TScalar, zzFormatBehavior{})
	DeclareRefinementBase(zz)
	dep := NewValueRaw(zz, DepScalarInfo{Hi: &DepBound{Value: NewInteger(7)}})
	if got := dep.String(); got != "(Zz lt 7)" {
		t.Errorf("a refinement over a Formatter base renders %q, want (Zz lt 7)", got)
	}
	if got := NewValueRaw(zz, IntPayload{N: 1}).String(); got != "Zz<?>" {
		t.Errorf("a plain value of the base renders %q, want its Formatter's Zz<?>", got)
	}
}

// TestRefinementConstructorsNoteUnknownBounds pins NUR231 at the
// constructors: over a KNOWN bound the refinement is a const (remembered for
// the stripped-operand recovery); over a bound the pass does not know (a
// carrier — a computed one) the constructor latches a run-time construct and
// remembers nothing, and `between` decides no empty interval from it.
func TestRefinementConstructorsNoteUnknownBounds(t *testing.T) {
	r, rec := analysingRegistry(t)
	gtSig := MakeDepScalarSig("gt", DepGT)
	gt := gtSig.DispatchHandler()
	intLit := NewTypeLiteral(TInteger)

	if _, err := gt([]Value{NewInteger(3), intLit}, nil, nil, r); err != nil {
		t.Fatal(err)
	}
	if rec.constructs != 0 || len(rec.remembered) != 1 {
		t.Fatalf("a known bound is a const: constructs=%d remembered=%d", rec.constructs, len(rec.remembered))
	}
	out, err := gt([]Value{NewCarrier(TInteger), intLit}, nil, nil, r)
	if err != nil || len(out) != 1 || !out[0].IsDepScalar() {
		t.Fatalf("an unknown bound still builds the refinement the pass reads: %v %v", out, err)
	}
	if rec.constructs != 1 || len(rec.remembered) != 1 {
		t.Fatalf("an unknown bound latches a run-time construct: constructs=%d remembered=%d", rec.constructs, len(rec.remembered))
	}

	// between: a known empty interval is Never; an unknown bound decides
	// nothing and is constructed at run time.
	out, err = BetweenHandler([]Value{NewInteger(5), NewInteger(1), intLit}, nil, nil, r)
	if err != nil || !IsBareTypeNode(out[0]) || !out[0].Is(TNever) {
		t.Fatalf("between over known crossed bounds is Never: %v %v", out, err)
	}
	out, err = BetweenHandler([]Value{NewInteger(1), NewCarrier(TInteger), intLit}, nil, nil, r)
	if err != nil || !out[0].IsDepScalar() || rec.constructs != 2 {
		t.Fatalf("between over an unknown bound is a run-time refinement, not Never: %v %v constructs=%d", out, err, rec.constructs)
	}
	// Outside a registry (a bare handler call) nothing is noted.
	if out, err = BetweenHandler([]Value{NewInteger(1), NewInteger(3), intLit}, nil, nil, nil); err != nil || !out[0].IsDepScalar() {
		t.Fatalf("between without a registry: %v %v", out, err)
	}
}

// TestUnknownBoundDecidesNothing pins the membership half of NUR231: a
// carrier bound orders below every value, so a verdict over it was the
// lattice's — `Integer lte (size s)` refused 3 at check time whatever s
// held. An unknown bound admits (gradually); a known one still decides.
func TestUnknownBoundDecidesNothing(t *testing.T) {
	unknown := &DepBound{Value: NewCarrier(TInteger)}
	for _, lower := range []bool{true, false} {
		if !depBoundCheck(unknown, lower, NewInteger(3)) {
			t.Errorf("an unknown bound (lower=%v) decides nothing: it must admit", lower)
		}
	}
	if depBoundCheck(&DepBound{Value: NewInteger(3)}, true, NewInteger(3)) {
		t.Error("a known strict lower bound 3 must refuse 3")
	}
	if depBoundCheck(&DepBound{Value: NewInteger(3)}, false, NewInteger(5)) {
		t.Error("a known strict upper bound 3 must refuse 5")
	}
}

// TestRefinementConstOnlyOverKnownBounds pins the const gate: a refinement
// bakes only when every bound it carries is itself a const.
func TestRefinementConstOnlyOverKnownBounds(t *testing.T) {
	known := NewDepScalar(DepGT, NewInteger(3))
	if !IsInertConst(known) {
		t.Error("a refinement over a known bound is a const")
	}
	if IsInertConst(NewDepScalar(DepGT, NewCarrier(TInteger))) {
		t.Error("a refinement over an unknown bound is no const")
	}
	half := NewValueRaw(TInteger, DepScalarInfo{
		Lo: &DepBound{Inclusive: true, Value: NewInteger(1)},
		Hi: &DepBound{Inclusive: true, Value: NewCarrier(TInteger)},
	})
	if IsInertConst(half) {
		t.Error("an interval with one unknown side is no const")
	}
}

// TestUnknownRefinementDeclines pins the sites that would bake a refinement
// over an unknown bound: a type install and an inline signature type decline
// the compile; over a known bound neither does, and nothing declines outside
// an analysis pass. The inline signature type's slot is the refinement's own
// base — whichever type declared itself one — not a hand-listed five (an
// inline Bytes refinement was a wildcard, NUR009).
func TestUnknownRefinementDeclines(t *testing.T) {
	DeclineUnknownRefinement(nil, "nothing") // no registry: no-op
	plain, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	DeclineUnknownRefinement(plain, "an interpreter run") // not analysing: no-op

	r, rec := analysingRegistry(t)
	if err := InstallType(r, "Kn", NewDepScalar(DepGT, NewInteger(3))); err != nil {
		t.Fatal(err)
	}
	if len(rec.declined) != 0 {
		t.Fatalf("a type over a known bound declines nothing: %q", rec.declined)
	}
	if err := InstallType(r, "Un", NewDepScalar(DepGT, NewCarrier(TInteger))); err != nil {
		t.Fatal(err)
	}
	if len(rec.declined) != 1 || !strings.Contains(rec.declined[0], "type Un refines over a computed bound") ||
		!strings.Contains(rec.declined[0], "(NUR231)") {
		t.Fatalf("a type over an unknown bound declines: %q", rec.declined)
	}

	kind, pat, err := ResolveSigType(r, NewDepScalar(DepLT, NewInteger(9)))
	if err != nil || kind != TInteger || pat == nil || !pat.IsDepScalar() || len(rec.declined) != 1 {
		t.Fatalf("an inline refinement's slot is its base, the refinement its pattern: %v %v %v %q", kind, pat, err, rec.declined)
	}
	if _, _, err = ResolveSigType(r, NewDepScalar(DepLT, NewCarrier(TInteger))); err != nil ||
		len(rec.declined) != 2 || !strings.Contains(rec.declined[1], "an inline signature type refines over a computed bound") {
		t.Fatalf("an inline signature type over an unknown bound declines: %v %q", err, rec.declined)
	}
	zz := r.Types.MintTypeWithBehavior("Zz", TScalar, zzFormatBehavior{})
	DeclareRefinementBase(zz)
	kind, pat, err = ResolveSigType(r, NewValueRaw(zz, DepScalarInfo{Lo: &DepBound{Value: NewInteger(1)}}))
	if err != nil || kind != zz || pat == nil {
		t.Fatalf("a declared base's inline refinement slots at that base, not TAny: %v %v %v", kind, pat, err)
	}
}
