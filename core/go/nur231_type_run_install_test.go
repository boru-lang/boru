package core

import (
	"errors"
	"strings"
	"testing"
)

// passNode mints what the analysis pass mints for `def name (Integer gt
// (size s))`: a node whose refinement's bound is the pass's carrier.
func passNode(r *Registry, name string) *Type {
	placeholder := NewDepScalar(DepGT, NewCarrier(TInteger))
	di, _ := placeholder.AsDepScalar()
	n := r.Types.MintType(name, TInteger)
	installDepScalarUnifier(n, TInteger, di, name)
	n.SetTypeBody(placeholder)
	return n
}

// TestRunTypeInstallForwards pins OpBindTypeRun's core (NUR231's type half):
// the run installs the body it computed through the interpreter's own
// installer, and the pass's node — the one every compiled reference names —
// forwards to the node that install bound. Membership, unification and
// identity are then the run's: a fresh mint over the real bound, or the
// adopted Never of an empty interval the run computed.
func TestRunTypeInstallForwards(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	pass := passNode(r, "Rt")
	if !NewInteger(2).Is(pass) {
		t.Fatal("before the run, the pass's node admits: its bound is unknown")
	}
	if err := RunTypeInstall(r, &TypeRunInstallSpec{Name: "Rt", Node: pass}, NewDepScalar(DepGT, NewInteger(3))); err != nil {
		t.Fatal(err)
	}
	run := r.LookupTypeName("Rt")
	if run == nil || run == pass || ForwardedType(pass) != run {
		t.Fatalf("the name binds the run's node and the pass's forwards to it: %v %v %v", run, pass, ForwardedType(pass))
	}
	if NewInteger(2).Is(pass) || !NewInteger(5).Is(pass) {
		t.Error("the pass's node decides membership over the run's bound")
	}
	if _, ok := Unify(NewInteger(2), NewTypeLiteral(pass)); ok {
		t.Error("2 does not unify with the forwarded (Integer gt 3)")
	}
	if got, ok := UnifyR(NewInteger(5), NewTypeLiteral(pass), r); !ok || got.String() != "5" {
		t.Errorf("5 unifies with the forwarded (Integer gt 3): %v %v", got, ok)
	}
	if got := pass.Behavior().Format(NewInteger(5)); got != "5" {
		t.Errorf("rendering is the run node's: %q", got)
	}
	if !pass.Behavior().Equal(NewInteger(5), NewInteger(5)) {
		t.Error("equality is the run node's")
	}

	// An empty interval the run computed: the install adopts Never.
	empty := passNode(r, "Em")
	if err := RunTypeInstall(r, &TypeRunInstallSpec{Name: "Em", Node: empty}, NewTypeLiteral(TNever)); err != nil {
		t.Fatal(err)
	}
	if ForwardedType(empty).ID != TNever.ID || NewInteger(1).Is(empty) {
		t.Errorf("an empty interval forwards to Never: %v", ForwardedType(empty))
	}

	// The install's own refusal is the run's raise.
	if err := RunTypeInstall(r, &TypeRunInstallSpec{Name: "lower", Node: passNode(r, "Lw")}, NewDepScalar(DepGT, NewInteger(1))); err == nil ||
		!strings.Contains(err.Error(), "type names must start with a capital letter") {
		t.Errorf("the installer's refusal surfaces: %v", err)
	}
	// A spec without a node installs and forwards nothing.
	if err := RunTypeInstall(r, &TypeRunInstallSpec{Name: "Nn"}, NewDepScalar(DepGT, NewInteger(1))); err != nil || r.LookupTypeName("Nn") == nil {
		t.Errorf("a node-less spec still installs: %v", err)
	}
	// A node forwarded to itself is left alone.
	before := run.Behavior()
	forwardType(run, run)
	if run.Behavior() != before || ForwardedType(run) != run {
		t.Error("a self-forward changes nothing")
	}
}

// TestForwardedTypeStops pins the forward walk's two ends: a node with no
// forward is itself (a bare node without metadata too), and a cycle — which
// no install builds — stops after a bounded number of hops.
func TestForwardedTypeStops(t *testing.T) {
	if ForwardedType(nil) != nil {
		t.Error("nil forwards nowhere")
	}
	bare := &Value{}
	if ForwardedType(bare) != bare {
		t.Error("a node without metadata is itself")
	}
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	a, b := passNode(r, "Ca"), passNode(r, "Cb")
	a.ensureTMeta().RunForward = b
	b.ensureTMeta().RunForward = a
	if got := ForwardedType(a); got != a && got != b {
		t.Errorf("a cycle stops on one of its nodes: %v", got)
	}
}

// TestForwardingBehaviorOperands pins the forwarded unify's operand swap:
// only a bare node that forwards is replaced by the run's node; a value, and
// a node with no forward, pass through unchanged.
func TestForwardingBehaviorOperands(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	pass := passNode(r, "Op")
	if err := RunTypeInstall(r, &TypeRunInstallSpec{Name: "Op", Node: pass}, NewDepScalar(DepGT, NewInteger(3))); err != nil {
		t.Fatal(err)
	}
	if got := forwardedOperand(NewInteger(4)); got.String() != "4" {
		t.Errorf("a value is itself: %v", got)
	}
	if got := forwardedOperand(NewTypeLiteral(TString)); !IsBareTypeNode(got) || got.ID != TString.ID {
		t.Errorf("a node without a forward is itself: %v", got)
	}
	if got := forwardedOperand(NewTypeLiteral(pass)); got.ID != r.LookupTypeName("Op").ID {
		t.Errorf("a forwarded node is the run's: %v", got)
	}
	// The forwarded behaviour's Unify, driven with the node on either side.
	fb := forwardingBehavior{target: r.LookupTypeName("Op")}
	if _, err := fb.Unify(NewTypeLiteral(pass), NewInteger(9), r); err != nil {
		t.Errorf("9 is an (Integer gt 3): %v", err)
	}
	if _, err := fb.Unify(NewInteger(1), NewTypeLiteral(pass), r); err == nil {
		t.Error("1 is not an (Integer gt 3)")
	}
}

// TestHasUnknownRefinementWalks pins the walk that decides whether a type is
// the run's: a refinement over an unknown bound, through a union, a
// negation, a typed container's child or a named node's recorded body; a
// known bound, a plain type, a body-less node and a runaway nesting are not.
func TestHasUnknownRefinementWalks(t *testing.T) {
	unknown := NewDepScalar(DepGT, NewCarrier(TInteger))
	known := NewDepScalar(DepGT, NewInteger(1))
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		v    Value
		want bool
	}{
		{"unknown", unknown, true},
		{"known", known, false},
		{"union", NewDisjunct([]Value{NewTypeLiteral(TString), unknown}), true},
		{"known union", NewDisjunct([]Value{NewTypeLiteral(TString), known}), false},
		{"negation", NewNegation(unknown), true},
		{"typed list", NewValueRaw(TList, ChildTypeInfo{Child: unknown}), true},
		{"named", NewTypeLiteral(passNode(r, "Hn")), true},
		{"builtin", NewTypeLiteral(TInteger), false},
		{"scalar", NewInteger(3), false},
	} {
		if got := HasUnknownRefinement(tc.v); got != tc.want {
			t.Errorf("%s: HasUnknownRefinement = %v, want %v", tc.name, got, tc.want)
		}
	}
	deep := unknown
	for i := 0; i < 40; i++ {
		deep = NewNegation(deep)
	}
	if HasUnknownRefinement(deep) {
		t.Error("a runaway nesting stops the walk")
	}
	// A typed container's child in a signature slot is the run's too.
	ar, rec := analysingRegistry(t)
	if _, _, err := ResolveSigType(ar, NewValueRaw(TList, ChildTypeInfo{Child: unknown})); err != nil || rec.dependents != 1 {
		t.Errorf("a typed list over an unknown refinement is the run's to build: %v %d", err, rec.dependents)
	}
	if _, _, err := ResolveSigType(ar, NewValueRaw(TMap, ChildTypeInfo{Child: unknown})); err != nil || rec.dependents != 2 {
		t.Errorf("a typed map over an unknown refinement is the run's to build: %v %d", err, rec.dependents)
	}
}

// TestCombineOverUnknownBounds pins the intersection of refinements when a
// bound is one the pass does not know: which side is tighter is the run's,
// the unknown bound is kept (so the result stays a type the run installs),
// and emptiness is decided only over known bounds — `(Integer lt (size s))
// tand (Integer gt 5)` was Never at compile time whatever s held.
func TestCombineOverUnknownBounds(t *testing.T) {
	unk := func() *DepBound { return &DepBound{Value: NewCarrier(TInteger)} }
	kn := func(n int64) *DepBound { return &DepBound{Value: NewInteger(n)} }
	for _, tc := range []struct {
		name   string
		a, b   DepScalarInfo
		loUnk  bool
		hiUnk  bool
		nonNil bool
	}{
		{"unknown lo first", DepScalarInfo{Lo: unk()}, DepScalarInfo{Lo: kn(1)}, true, false, false},
		{"unknown lo second", DepScalarInfo{Lo: kn(1)}, DepScalarInfo{Lo: unk()}, true, false, false},
		{"unknown hi first", DepScalarInfo{Hi: unk()}, DepScalarInfo{Hi: kn(9)}, false, true, false},
		{"unknown hi second", DepScalarInfo{Hi: kn(9)}, DepScalarInfo{Hi: unk()}, false, true, false},
		{"unknown upper over known lower", DepScalarInfo{Hi: unk()}, DepScalarInfo{Lo: kn(5)}, false, true, true},
	} {
		out, ok := combineDepScalars(tc.a, tc.b)
		if !ok {
			t.Errorf("%s: an interval over an unknown bound is never proved empty", tc.name)
			continue
		}
		if tc.loUnk && (out.Lo == nil || depBoundKnown(out.Lo)) {
			t.Errorf("%s: the unknown lower bound is kept: %+v", tc.name, out.Lo)
		}
		if tc.hiUnk && (out.Hi == nil || depBoundKnown(out.Hi)) {
			t.Errorf("%s: the unknown upper bound is kept: %+v", tc.name, out.Hi)
		}
		if tc.nonNil && (out.Lo == nil || out.Hi == nil) {
			t.Errorf("%s: both sides survive: %+v", tc.name, out)
		}
	}
	if _, ok := combineDepScalars(DepScalarInfo{Hi: kn(2)}, DepScalarInfo{Lo: kn(5)}); ok {
		t.Error("over known bounds, (lt 2) tand (gt 5) is still empty")
	}
}

// TestNegationOverUnknownAdmits pins the complement's half of "the pass
// decides no membership over an unknown bound": the refinement admits every
// value, so its complement would refuse every value — the pass admits, and
// the run decides. Over a known bound the complement still decides.
func TestNegationOverUnknownAdmits(t *testing.T) {
	if _, ok := Unify(NewInteger(9), NewNegation(NewDepScalar(DepGT, NewCarrier(TInteger)))); !ok {
		t.Error("tnot over an unknown bound admits at check time")
	}
	if _, ok := Unify(NewInteger(9), NewNegation(NewDepScalar(DepGT, NewInteger(3)))); ok {
		t.Error("tnot (Integer gt 3) refuses 9")
	}
}

// TestTypedBindRunMembership pins OpBindTyped's run-time membership kind:
// the registry-armed Unify against the constraint the run holds — a named
// node (forwarded) or, inline, the constraint the run computed
// (RunTypedBindCons, describing it as rendered when the def named none).
func TestTypedBindRunMembership(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	spec := &TypedBindSpec{Kind: TypedBindRunMembership, Name: "v"}
	if _, err := RunTypedBind(r, spec, NewInteger(1)); err == nil || !strings.Contains(err.Error(), "has no constraint") {
		t.Errorf("a spec without its constraint is an internal fault: %v", err)
	}
	cons := NewDepScalar(DepGT, NewInteger(3))
	if got, err := RunTypedBindCons(r, spec, cons, NewInteger(5)); err != nil || got.String() != "5" {
		t.Errorf("5 binds under (Integer gt 3): %v %v", got, err)
	}
	_, err = RunTypedBindCons(r, spec, cons, NewInteger(2))
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "type_error" || !strings.Contains(ae.Detail, "def v: value 2 does not unify with declared type (Integer gt 3)") {
		t.Errorf("2 is refused, described by the run's constraint: %v", err)
	}
	named := &TypedBindSpec{Kind: TypedBindRunMembership, Name: "w", Describe: "Big"}
	if _, err := RunTypedBindCons(r, named, cons, NewInteger(2)); err == nil || !strings.Contains(err.Error(), "declared type Big") {
		t.Errorf("a named constraint keeps its name: %v", err)
	}
}

// TestMakeFieldErrorIsStructured pins NUR233: make's field refusal is a
// type_error on both lanes — a plain error was the interpreter's raise, and a
// compiled run books a plain error as a compiler defect. A refusal already
// structured keeps its own code.
func TestMakeFieldErrorIsStructured(t *testing.T) {
	fields := NewOrderedMap()
	fields.Set("x", NewDepScalar(DepGT, NewInteger(100)))
	src := NewOrderedMap()
	src.Set("x", NewInteger(3))
	_, err := MakeRecordR(RecordTypeInfo{Fields: fields}, NewMap(src), false, nil)
	var ae *BoruError
	if !errors.As(err, &ae) || ae.Code != "type_error" || !strings.HasPrefix(ae.Detail, `make: field "x": `) {
		t.Errorf("a field refusal is a type_error: %v", err)
	}
	inner := &BoruError{Code: "raise_error", Detail: "boom"}
	wrapped := makeFieldError("y", inner)
	if !errors.As(wrapped, &ae) || ae != inner || !strings.Contains(wrapped.Error(), `make: field "y"`) {
		t.Errorf("a structured refusal keeps its code: %v", wrapped)
	}
}

// TestWrittenBackTypeTwinInstallsNothing pins the replay side of the run-time
// install: the def's type twin is written back, so its replay pushes no
// binding and reserves nothing — OpBindTypeRun is the one install.
func TestWrittenBackTypeTwinInstallsNothing(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	node := passNode(r, "Wb")
	tr := BindTransition{Kind: BindTypeInstall, Name: "Wb", WrittenBack: true}
	if err := ApplyBindTwin(r, tr, DefEntry{TypeDef: node, Minted: true, Body: NewTypeLiteral(node)}); err != nil {
		t.Fatal(err)
	}
	if r.Defs.Has("Wb") {
		t.Error("a written-back type twin installs nothing")
	}
}
