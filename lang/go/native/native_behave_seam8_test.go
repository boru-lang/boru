package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- fake Behaviors used as the `prev` slot on a userBehavior ---

// w8Plain implements only TypeBehavior — it is NOT a Comparer,
// Nodifier, or Unifier. Used to drive the "prev doesn't satisfy the
// capability interface" fall-through branches (ErrNoComparer /
// ErrNoNodifier / ErrNoUnifier).
type w8Plain struct{}

func (w8Plain) Match(v Value, t *Type) bool { return false }
func (w8Plain) Format(v Value) string       { return "w8plain-format" }
func (w8Plain) Equal(a, b Value) bool       { return false }

// w8Cap implements TypeBehavior plus every optional capability, so it
// drives the "delegate to prev" branches.
type w8Cap struct{}

func (w8Cap) Match(v Value, t *Type) bool     { return true }
func (w8Cap) Format(v Value) string           { return "w8cap-format" }
func (w8Cap) Equal(a, b Value) bool           { return true }
func (w8Cap) Compare(a, b Value) (int, error) { return 7, nil }
func (w8Cap) Nodify(v Value) (Value, error)   { return NewString("w8cap-nodify"), nil }
func (w8Cap) Unify(a, b Value, _ *core.Registry) (Value, *core.UnifyError) {
	return NewString("w8cap-unify"), nil
}

// dropBody is a token body that pushes one value then drops it, so the
// engine leaves an empty stack — reaching the "body produced no result"
// guards in each run*Body helper.
func w8DropBody() []Value { return []Value{NewInteger(1), NewWord("drop")} }

// --- validate* sig helpers (direct calls) ---

func TestW8ValidateCompareSigNilParamTypes(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: nil}, {Type: nil}},
		Returns: []*Type{TInteger},
	}
	if _, err := validateCompareSig(sig); err == nil {
		t.Fatal("compare: nil param types must error")
	}
}

func TestW8ValidateCanonSigNilParamType(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: nil}},
		Returns: []*Type{TString},
	}
	if _, err := validateCanonSig(sig); err == nil {
		t.Fatal("canon: nil param type must error")
	}
}

func TestW8ValidateNodifySigWrongReturns(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TInteger}},
		Returns: []*Type{}, // must be exactly one
	}
	if _, err := validateNodifySig(sig); err == nil {
		t.Fatal("nodify: zero returns must error")
	}
}

func TestW8ValidateNodifySigNilParamType(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: nil}},
		Returns: []*Type{TAny},
	}
	if _, err := validateNodifySig(sig); err == nil {
		t.Fatal("nodify: nil param type must error")
	}
}

func TestW8ValidateUnifySigWrongReturns(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: TInteger}, {Type: TInteger}},
		Returns: []*Type{}, // must be exactly one
	}
	if _, err := validateUnifySig(sig); err == nil {
		t.Fatal("unify: zero returns must error")
	}
}

func TestW8ValidateUnifySigNilParamTypes(t *testing.T) {
	sig := FnSig{
		Params:  []FnParam{{Type: nil}, {Type: nil}},
		Returns: []*Type{TAny},
	}
	if _, err := validateUnifySig(sig); err == nil {
		t.Fatal("unify: nil param types must error")
	}
}

// --- extractFnDefInfo error paths ---

func TestW8ExtractFnDefInfoNilParent(t *testing.T) {
	if _, err := extractFnDefInfo(Value{}); err == nil {
		t.Fatal("extractFnDefInfo: nil-Parent value must error")
	}
}

func TestW8ExtractFnDefInfoBadPayload(t *testing.T) {
	// A Function-typed value whose payload is a StrPayload, not FnDefInfo.
	v := Value{Parent: TFunction, Data: NewString("not-a-fndefinfo").Data}
	if _, err := extractFnDefInfo(v); err == nil {
		t.Fatal("extractFnDefInfo: Function value with wrong payload must error")
	}
}

// --- behaveHandler error paths ---

func TestW8BehaveHandlerFnNoSignatures(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fnVal := Value{Parent: TFunction, Data: core.FnDefInfo{}} // no signatures
	_, herr := behaveHandler([]Value{NewAtom("compare"), fnVal}, nil, nil, r)
	if herr == nil {
		t.Fatal("behave: fn with no signatures must error")
	}
}

func TestW8BehaveHandlerTargetNil(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Install a temporary behavior whose validate returns (nil, nil) so
	// the handler reaches the "could not infer target type" guard.
	behaviors["w8fake"] = behaviorEntry{
		validate: func(sig core.FnSig) (*core.Type, error) { return nil, nil },
		install:  func(u *userBehavior, body []core.Value) {},
	}
	t.Cleanup(func() { delete(behaviors, "w8fake") })

	fnVal := Value{Parent: TFunction, Data: core.FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Type: TInteger}},
			Returns: []*Type{TInteger},
		}},
	}}
	_, herr := behaveHandler([]Value{NewAtom("w8fake"), fnVal}, nil, nil, r)
	if herr == nil {
		t.Fatal("behave: nil target type must error")
	}
}

// --- userBehavior.Match / Equal delegation + default ---

func TestW8UserBehaviorMatchEqual(t *testing.T) {
	withPrev := &userBehavior{prev: w8Cap{}}
	if !withPrev.Match(NewInteger(1), TInteger) {
		t.Error("Match should delegate to prev (true)")
	}
	if !withPrev.Equal(NewInteger(1), NewInteger(1)) {
		t.Error("Equal should delegate to prev (true)")
	}

	noPrev := &userBehavior{}
	// prev == nil -> DefaultBehavior. Just assert no panic and a stable
	// result for equal integers.
	_ = noPrev.Match(NewInteger(1), TInteger)
	if !noPrev.Equal(NewInteger(1), NewInteger(1)) {
		t.Error("default Equal of identical integers should be true")
	}
}

// --- userBehavior.Format ---

func TestW8UserBehaviorFormatDefault(t *testing.T) {
	// canonBody empty, prev nil -> DefaultBehavior.Format.
	u := &userBehavior{}
	if got := u.Format(NewInteger(42)); got == "" {
		t.Error("default Format should render something for 42")
	}
}

// --- userBehavior.Compare ---

func TestW8UserBehaviorCompareDelegates(t *testing.T) {
	u := &userBehavior{prev: w8Cap{}} // no compareBody, prev is a Comparer
	n, err := u.Compare(NewInteger(1), NewInteger(2))
	if err != nil {
		t.Fatalf("Compare delegate: %v", err)
	}
	if n != 7 {
		t.Errorf("Compare should delegate to prev (got %d, want 7)", n)
	}
}

func TestW8UserBehaviorCompareNoComparer(t *testing.T) {
	u := &userBehavior{prev: w8Plain{}} // prev not a Comparer, no body
	_, err := u.Compare(NewInteger(1), NewInteger(2))
	if err != core.ErrNoComparer {
		t.Errorf("Compare should return ErrNoComparer, got %v", err)
	}
}

// --- userBehavior.runCompareBody error paths ---

func TestW8RunCompareBodyNoRegistry(t *testing.T) {
	u := &userBehavior{typeName: "Foo", compareBody: []Value{NewInteger(0)}}
	if _, err := u.runCompareBody(NewInteger(1), NewInteger(2)); err == nil {
		t.Fatal("runCompareBody with nil registry must error")
	}
}

func TestW8RunCompareBodyEmptyResult(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", compareBody: w8DropBody()}
	if _, err := u.runCompareBody(NewInteger(1), NewInteger(2)); err == nil {
		t.Fatal("runCompareBody with empty result must error")
	}
}

func TestW8RunCompareBodyNonInteger(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", compareBody: []Value{NewString("x")}}
	if _, err := u.runCompareBody(NewInteger(1), NewInteger(2)); err == nil {
		t.Fatal("runCompareBody returning a String must error")
	}
}

func TestW8RunCompareBodyIntegerNilData(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// An Integer-typed value with nil Data: ConformsTo(Integer) is true
	// but AsInteger fails, reaching the AsInteger-error guard.
	u := &userBehavior{registry: r, typeName: "Foo", compareBody: []Value{{Parent: TInteger}}}
	if _, err := u.runCompareBody(NewInteger(1), NewInteger(2)); err == nil {
		t.Fatal("runCompareBody with Integer-typed nil-Data result must error")
	}
}

// --- userBehavior.Nodify ---

func TestW8UserBehaviorNodifyDelegates(t *testing.T) {
	u := &userBehavior{prev: w8Cap{}} // no nodifyBody, prev is a Nodifier
	out, err := u.Nodify(NewInteger(1))
	if err != nil {
		t.Fatalf("Nodify delegate: %v", err)
	}
	if s, _ := AsString(out); s != "w8cap-nodify" {
		t.Errorf("Nodify should delegate to prev, got %v", out)
	}
}

func TestW8UserBehaviorNodifyNoNodifier(t *testing.T) {
	u := &userBehavior{prev: w8Plain{}} // prev not a Nodifier, no body
	if _, err := u.Nodify(NewInteger(1)); err != core.ErrNoNodifier {
		t.Errorf("Nodify should return ErrNoNodifier, got %v", err)
	}
}

func TestW8UserBehaviorNodifyReentry(t *testing.T) {
	u := &userBehavior{nodifyBody: []Value{NewInteger(0)}, inNodify: true}
	out, err := u.Nodify(NewInteger(9))
	if err != nil {
		t.Fatalf("Nodify re-entry: %v", err)
	}
	if n, _ := AsInteger(out); n != 9 {
		t.Errorf("Nodify re-entry should pass value through, got %v", out)
	}
}

// --- userBehavior.Unify ---

func TestW8UserBehaviorUnifyDelegates(t *testing.T) {
	u := &userBehavior{prev: w8Cap{}} // no unifyBody, prev is a Unifier
	out, uerr := u.Unify(NewInteger(1), NewInteger(2), nil)
	if uerr != nil {
		t.Fatalf("Unify delegate: %v", uerr)
	}
	if s, _ := AsString(out); s != "w8cap-unify" {
		t.Errorf("Unify should delegate to prev, got %v", out)
	}
}

func TestW8UserBehaviorUnifyNoUnifier(t *testing.T) {
	u := &userBehavior{prev: w8Plain{}} // prev not a Unifier, no body
	if _, uerr := u.Unify(NewInteger(1), NewInteger(2), nil); uerr != core.ErrNoUnifier {
		t.Errorf("Unify should return ErrNoUnifier, got %v", uerr)
	}
}

func TestW8UserBehaviorUnifyReentryDelegates(t *testing.T) {
	u := &userBehavior{unifyBody: []Value{NewInteger(0)}, prev: w8Cap{}, inUnify: true}
	out, uerr := u.Unify(NewInteger(1), NewInteger(2), nil)
	if uerr != nil {
		t.Fatalf("Unify re-entry delegate: %v", uerr)
	}
	if s, _ := AsString(out); s != "w8cap-unify" {
		t.Errorf("Unify re-entry should delegate to prev Unifier, got %v", out)
	}
}

func TestW8UserBehaviorUnifyReentryNoUnifier(t *testing.T) {
	u := &userBehavior{unifyBody: []Value{NewInteger(0)}, prev: w8Plain{}, inUnify: true}
	if _, uerr := u.Unify(NewInteger(1), NewInteger(2), nil); uerr != core.ErrNoUnifier {
		t.Errorf("Unify re-entry without prev Unifier should return ErrNoUnifier, got %v", uerr)
	}
}

// --- userBehavior.runUnifyBody error paths ---

func TestW8RunUnifyBodyNoRegistry(t *testing.T) {
	u := &userBehavior{typeName: "Foo", unifyBody: []Value{NewInteger(0)}}
	if _, uerr := u.Unify(NewInteger(1), NewInteger(2), nil); uerr == nil {
		t.Fatal("runUnifyBody with nil registry must error")
	}
}

func TestW8RunUnifyBodyEmptyResult(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", unifyBody: w8DropBody()}
	if _, uerr := u.Unify(NewInteger(1), NewInteger(2), nil); uerr == nil {
		t.Fatal("runUnifyBody with empty result must error")
	}
}

// --- userBehavior.runNodifyBody error paths ---

func TestW8RunNodifyBodyNoRegistry(t *testing.T) {
	u := &userBehavior{typeName: "Foo", nodifyBody: []Value{NewInteger(0)}}
	if _, err := u.Nodify(NewInteger(1)); err == nil {
		t.Fatal("runNodifyBody with nil registry must error")
	}
}

func TestW8RunNodifyBodyEmptyResult(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", nodifyBody: w8DropBody()}
	if _, err := u.Nodify(NewInteger(1)); err == nil {
		t.Fatal("runNodifyBody with empty result must error")
	}
}

// --- userBehavior.runCanonBody error paths ---

func TestW8RunCanonBodyNoRegistry(t *testing.T) {
	u := &userBehavior{typeName: "Foo", canonBody: []Value{NewString("x")}}
	if _, err := u.runCanonBody(NewInteger(1)); err == nil {
		t.Fatal("runCanonBody with nil registry must error")
	}
}

func TestW8RunCanonBodyEmptyResult(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", canonBody: w8DropBody()}
	if _, err := u.runCanonBody(NewInteger(1)); err == nil {
		t.Fatal("runCanonBody with empty result must error")
	}
}

func TestW8RunCanonBodyNonString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	u := &userBehavior{registry: r, typeName: "Foo", canonBody: []Value{NewInteger(5)}}
	if _, err := u.runCanonBody(NewInteger(1)); err == nil {
		t.Fatal("runCanonBody returning an Integer must error")
	}
}

func TestW8RunCanonBodyStringNilData(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A String-typed value with nil Data: ConformsTo(String) is true but
	// AsString fails, reaching the AsString-error guard.
	u := &userBehavior{registry: r, typeName: "Foo", canonBody: []Value{{Parent: TString}}}
	if _, err := u.runCanonBody(NewInteger(1)); err == nil {
		t.Fatal("runCanonBody with String-typed nil-Data result must error")
	}
}
