package core

import (
	"strings"
	"testing"
)

// TestCurrentStackFiltersMarkers pins CurrentStack's structural-marker
// filter: markers and words to the left of the pointer are dropped so
// the result is the clean data stack, not the raw tape.
func TestCurrentStackFiltersMarkers(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := NewTop(r)
	e.Tape = NewTape([]Value{NewInteger(1), NewMark("m5"), NewWord("w5"), NewInteger(2)}, 0)
	e.Pointer = 4
	r.pushEngine(e)
	defer r.popEngine()
	out, ok := r.CurrentStack()
	if !ok || len(out) != 2 || !ValuesEqual(out[0], NewInteger(1)) || !ValuesEqual(out[1], NewInteger(2)) {
		t.Fatalf("CurrentStack must keep only resolved data, got ok=%v %v", ok, out)
	}
}

// TestRuntimeStampingLifecycle pins the detached-stamping arm/disarm
// group plus CanHostVM and the nil-registry scope id.
func TestRuntimeStampingLifecycle(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !r.CanHostVM() {
		t.Fatal("an idle registry must host a fresh VM run")
	}
	if (*Registry)(nil).CanHostVM() {
		t.Fatal("nil registry must not host a VM run")
	}
	if r.RuntimeStampingEnabled() {
		t.Fatal("fresh registry must be unarmed")
	}
	// Inheriting from an unarmed parent is a no-op.
	child, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	child.InheritRuntimeStamping(r)
	if child.RuntimeStampingEnabled() {
		t.Fatal("inheriting from an unarmed parent must stay unarmed")
	}
	(*Registry)(nil).EnableRuntimeStamping() // nil-receiver no-op
	r.EnableRuntimeStamping()
	if !r.RuntimeStampingEnabled() || r.stampLog == nil {
		t.Fatal("EnableRuntimeStamping must arm and allocate the stamp log")
	}
	log := r.stampLog
	r.EnableRuntimeStamping() // idempotent: keeps the existing log
	if r.stampLog != log {
		t.Fatal("re-arming must keep the existing stamp log")
	}
	child.InheritRuntimeStamping(r)
	if !child.RuntimeStampingEnabled() || child.stampLog != log {
		t.Fatal("inheriting must arm and SHARE the parent's stamp log")
	}
	r.DisableRuntimeStamping()
	if r.RuntimeStampingEnabled() {
		t.Fatal("DisableRuntimeStamping must disarm")
	}
	if r.stampLog != log {
		t.Fatal("disarming must leave the stamp log intact")
	}
	if (*Registry)(nil).AnalysisScopeID() != 0 {
		t.Fatal("nil registry scope id must be 0")
	}
}

// TestRegisterErrorArms pins the registration error paths: a signature
// over MaxArgs, a native with an invalid word name (both surfaced by
// Err), the RegisterCoreDefault non-builtin guard, and the reserved-
// literal arm of IsBuiltinWord.
func TestRegisterErrorArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tooMany := make([]*Type, MaxArgs+1)
	for i := range tooMany {
		tooMany[i] = TAny
	}
	r.Register("stage5-toomany", Signature{Args: tooMany})
	if e := r.Err(); e == nil || !strings.Contains(e.Error(), "max is") {
		t.Fatalf("over-MaxArgs registration must error, got %v", e)
	}
	r2, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2.RegisterNativeFunc(NativeFunc{Name: "BadName", Signatures: []Signature{{}}})
	if e := r2.Err(); e == nil {
		t.Fatal("invalid native name must error")
	}
	// RegisterCoreDefault refuses a name Register never installed.
	r2.RegisterCoreDefault("stage5-never-registered", Signature{})
	if r2.Lookup("stage5-never-registered") != nil {
		t.Fatal("RegisterCoreDefault on a non-builtin must install nothing")
	}
	if !r2.IsBuiltinWord("none") {
		t.Fatal("reserved literals are builtin words")
	}
}

// TestValToStringArms covers the remaining ValToString arms: the
// non-concrete fallback, floats, both boolean spellings, and the
// default (container) fallback.
func TestValToStringArms(t *testing.T) {
	carrier := NewCarrier(TInteger)
	if got := ValToString(carrier); got != carrier.String() {
		t.Fatalf("non-concrete must fall back to String(), got %q", got)
	}
	if got := ValToString(NewFloat(1.5)); got != "1.5" {
		t.Fatalf("float: got %q, want 1.5", got)
	}
	if got := ValToString(NewBoolean(true)); got != "true" {
		t.Fatalf("boolean true: got %q", got)
	}
	if got := ValToString(NewBoolean(false)); got != "false" {
		t.Fatalf("boolean false: got %q", got)
	}
	lst := NewList([]Value{NewInteger(1)})
	if got := ValToString(lst); got != lst.String() {
		t.Fatalf("default must fall back to String(), got %q", got)
	}
	if got := StoreKey(NewInteger(42)); got != "42" {
		t.Fatalf("integer store key: got %q, want 42", got)
	}
}

// TestIsKnownPartArms covers both positive arms: a builtin part and a
// dynamically registered one.
func TestIsKnownPartArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsKnownPart("Integer") {
		t.Fatal("builtin part Integer must be known")
	}
	if r.IsKnownPart("Stage5Part") {
		t.Fatal("unregistered part must be unknown")
	}
	r.RegisterPart("Stage5Part")
	if !r.IsKnownPart("Stage5Part") {
		t.Fatal("registered dynamic part must be known")
	}
}

// TestResolveTypeLiteralDefClassBinding pins the DefStacks fallback: a
// bare type literal whose name is def-bound to a richer ClassType value
// resolves to that binding.
func TestResolveTypeLiteralDefClassBinding(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	lit := NewTypeLiteral(TInteger)
	if name := TypeNameByID(lit.ID); name != "Integer" {
		t.Fatalf("precondition: TypeNameByID = %q, want Integer", name)
	}
	cv := NewClassType(TInteger, ClassTypeInfo{Fields: NewOrderedMap(), Name: "Class/Stage5"})
	r.Defs.Push("Integer", cv)
	defer r.Defs.Pop("Integer")
	got := ResolveTypeLiteralDef(lit, r)
	if !IsClassType(got) {
		t.Fatalf("literal must resolve to the class binding, got %s", got.String())
	}
}

// TestCallBoruArms drives CallBoru's remaining interpreter-path arms: a
// named List param gets quote-tagged, an unnamed param rides as an
// inert token prefix and is trimmed against declared returns (both the
// exact and the clamped trim), and body-created defs are cleaned up.
func TestCallBoruArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Named List param (un-quoted at the call site).
	sigList := &Signature{
		Params:     []FnParam{{Name: "xs", Type: TList}},
		Impl:       Boru([]Value{NewInteger(1)}),
		BarrierPos: 0,
	}
	lst := NewList([]Value{NewInteger(9)})
	lst.Quoted = false
	res, err := r.CallBoru(sigList, []Value{lst}, []CapturedBinding{{Name: "cap5", Value: NewInteger(2)}})
	if err != nil || len(res) != 1 || !ValuesEqual(res[0], NewInteger(1)) {
		t.Fatalf("named-list call: got %v, %v", res, err)
	}

	// Unnamed param + declared return: the leftover arg prefix is
	// trimmed (extra == unnamedCount).
	sigTrim := &Signature{
		Params:     []FnParam{{Type: TAny}},
		Returns:    []*Type{TInteger},
		Impl:       Boru([]Value{NewInteger(5)}),
		BarrierPos: 0,
	}
	res, err = r.CallBoru(sigTrim, []Value{NewInteger(9)}, nil)
	if err != nil || len(res) != 1 || !ValuesEqual(res[0], NewInteger(5)) {
		t.Fatalf("trim call: got %v, %v", res, err)
	}

	// extra > unnamedCount: the trim clamps to the unnamed prefix.
	sigClamp := &Signature{
		Params:     []FnParam{{Type: TAny}},
		Returns:    []*Type{TInteger},
		Impl:       Boru([]Value{NewInteger(5), NewInteger(7)}),
		BarrierPos: 0,
	}
	res, err = r.CallBoru(sigClamp, []Value{NewInteger(9)}, nil)
	if err != nil || len(res) != 2 || !ValuesEqual(res[0], NewInteger(5)) || !ValuesEqual(res[1], NewInteger(7)) {
		t.Fatalf("clamped trim call: got %v, %v", res, err)
	}

	// Body-created defs are popped back to the pre-call snapshot.
	r.RegisterNativeFunc(NativeFunc{
		Name: "stage-five-leak",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.Defs.Push("stage5$leaked", NewInteger(1))
				return nil, nil
			}),
			Returns:    []*Type{},
			BarrierPos: 0,
		}},
	})
	sigLeak := &Signature{
		Params:     []FnParam{{Name: "a", Type: TAny}},
		Impl:       Boru([]Value{NewWord("stage-five-leak")}),
		BarrierPos: 0,
	}
	if _, err := r.CallBoru(sigLeak, []Value{NewInteger(1)}, nil); err != nil {
		t.Fatalf("leak call: %v", err)
	}
	if _, ok := r.Defs.Top("stage5$leaked"); ok {
		t.Fatal("body-created def must be cleaned up after the call")
	}

	// The args-stack Pop guard: Pop errors only on a nil ArgsStack, which
	// a normal call can never produce (Push on the same receiver succeeded
	// at entry) — force it by nilling the stack from inside the body, and
	// the guard must surface the pop error since the run itself succeeded.
	rGuard, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rGuard.RegisterNativeFunc(NativeFunc{
		Name: "stage-five-nil-args",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				reg.Args = nil
				return nil, nil
			}),
			Returns:    []*Type{},
			BarrierPos: 0,
		}},
	})
	sigNil := &Signature{
		Params:     []FnParam{{Name: "a", Type: TAny}},
		Impl:       Boru([]Value{NewWord("stage-five-nil-args")}),
		BarrierPos: 0,
	}
	if _, err := rGuard.CallBoru(sigNil, []Value{NewInteger(1)}, nil); err == nil ||
		!strings.Contains(err.Error(), "argsstack") {
		t.Fatalf("a nil args stack must surface the pop error, got %v", err)
	}
}

// TestResolveTypedNameValueArms covers all four resolution arms: the
// non-word pass-through, a def-bound name, the builtin-type cascade
// arm, and the unresolved word.
func TestResolveTypedNameValueArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := (*Registry)(nil).ResolveTypedName("x"); ok {
		t.Fatal("nil registry must not resolve")
	}
	n := NewInteger(3)
	if v, name, ok := r.ResolveTypedNameValue(n); !ok || name != "" || !ValuesEqual(v, n) {
		t.Fatal("non-word input must pass through")
	}
	r.Defs.Push("stage5val", NewInteger(4))
	defer r.Defs.Pop("stage5val")
	if v, name, ok := r.ResolveTypedNameValue(NewWord("stage5val")); !ok || name != "stage5val" || !ValuesEqual(v, NewInteger(4)) {
		t.Fatal("def-bound word must resolve to its binding")
	}
	if v, name, ok := r.ResolveTypedNameValue(NewWord("Integer")); !ok || name != "Integer" || !IsBareTypeNode(v) {
		t.Fatal("builtin type name must resolve to its literal")
	}
	if _, name, ok := r.ResolveTypedNameValue(NewWord("Zz-stage5-missing")); ok || name != "Zz-stage5-missing" {
		t.Fatal("unknown word must fail with its name captured")
	}
}

// stage5Predicate builds a single-arg predicate constraint whose body is
// the given token run, with the given declared input type.
func stage5Predicate(r *Registry, input *Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name:     "Stage5Pred",
		Registry: r,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: input}},
			Impl:       Boru(body),
			BarrierPos: 0,
		}},
	})
}

// TestRunPredicateArms drives RunPredicate through every arm: malformed
// constraints, the check-mode short-circuit, the input-type gate (with
// its bare-type-literal skip), body errors, the arity contract, and the
// None / boolean-verdict / value-transform result protocol.
func TestRunPredicateArms(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Not a fn at all.
	if _, _, err := r.RunPredicate(NewInteger(1), NewInteger(2)); err == nil {
		t.Fatal("non-fn constraint must error")
	}
	// TFunction-shaped but no FnDefInfo payload.
	if _, _, err := r.RunPredicate(Value{Parent: TFunction}, NewInteger(2)); err == nil {
		t.Fatal("payload-less constraint must error")
	}
	// No own sig / wrong arity.
	if _, _, err := r.RunPredicate(NewFunction(FnDefInfo{}), NewInteger(2)); err == nil {
		t.Fatal("sig-less constraint must error")
	}
	twoArg := NewFunction(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}},
		Impl:   Boru([]Value{NewInteger(1)}),
	}}})
	if _, _, err := r.RunPredicate(twoArg, NewInteger(2)); err == nil {
		t.Fatal("two-arg predicate must error")
	}

	verdictTrue := stage5Predicate(r, TInteger, []Value{NewBoolean(true)})

	// Check-mode short-circuit: accept without running the body.
	r.Check.Mode = true
	out, matched, err := r.RunPredicate(verdictTrue, NewInteger(5))
	r.Check.Mode = false
	if err != nil || !matched || !ValuesEqual(out, NewInteger(5)) {
		t.Fatalf("check-mode must accept the candidate, got %v %v %v", out, matched, err)
	}

	// Input-type gate: a non-conforming candidate rejects without a run.
	out, matched, err = r.RunPredicate(verdictTrue, NewString("s"))
	if err != nil || matched || !ValuesEqual(out, NewString("s")) {
		t.Fatalf("gate must reject a non-conforming candidate, got %v %v %v", out, matched, err)
	}
	// A bare type literal skips the gate and runs the body.
	out, matched, err = r.RunPredicate(verdictTrue, NewTypeLiteral(TString))
	if err != nil || !matched {
		t.Fatalf("bare type literal must skip the gate, got %v %v %v", out, matched, err)
	}

	// Body error propagates.
	r.RegisterNativeFunc(NativeFunc{
		Name: "stage-five-boom",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				return nil, reg.BoruError("type_error", "stage5 boom", "stage-five-boom")
			}),
			Returns:    []*Type{},
			BarrierPos: 0,
		}},
	})
	boom := stage5Predicate(r, TInteger, []Value{NewWord("stage-five-boom")})
	if _, _, err := r.RunPredicate(boom, NewInteger(5)); err == nil {
		t.Fatal("a raising body must propagate its error")
	}

	// Return-arity contract: exactly one value.
	empty := stage5Predicate(r, TInteger, nil)
	if _, _, err := r.RunPredicate(empty, NewInteger(5)); err == nil {
		t.Fatal("a zero-value body must error")
	}

	// None: no match.
	noneBody := stage5Predicate(r, TInteger, []Value{NewNone()})
	if _, matched, err := r.RunPredicate(noneBody, NewInteger(5)); err != nil || matched {
		t.Fatalf("a None result must not match, got %v %v", matched, err)
	}

	// Boolean verdicts over a non-Boolean domain.
	verdictFalse := stage5Predicate(r, TInteger, []Value{NewBoolean(false)})
	if _, matched, err := r.RunPredicate(verdictFalse, NewInteger(5)); err != nil || matched {
		t.Fatalf("a false verdict must not match, got %v %v", matched, err)
	}
	out, matched, err = r.RunPredicate(verdictTrue, NewInteger(5))
	if err != nil || !matched || !ValuesEqual(out, NewInteger(5)) {
		t.Fatalf("a true verdict must pass the candidate through, got %v %v %v", out, matched, err)
	}

	// Value-transforming body: the output IS the new value.
	transform := stage5Predicate(r, TInteger, []Value{NewInteger(7)})
	out, matched, err = r.RunPredicate(transform, NewInteger(5))
	if err != nil || !matched || !ValuesEqual(out, NewInteger(7)) {
		t.Fatalf("a transforming body must return its output, got %v %v %v", out, matched, err)
	}
}

// TestHelpWordRecord pins the register-time half of on-demand help
// (NoteHelpWord / IsHelpWord): the hook RECORDS a name and nothing else, so a
// registration — including the user's own `def f fn […]` mid-check — costs a
// map insert rather than a synthetic example evaluation in the live registry.
func TestHelpWordRecord(t *testing.T) {
	// A nil receiver answers no rather than panicking: every describe site
	// reaches IsHelpWord before it has established it has a registry.
	if (*Registry)(nil).IsHelpWord("anything") {
		t.Error("a nil registry must claim no help words")
	}
	r := &Registry{}
	// The negative first: an unrecorded name is not eligible, so a word with a
	// build-time example snapshot never re-evaluates it.
	if r.IsHelpWord("never-registered") {
		t.Error("an unrecorded name must not be eligible for example generation")
	}
	r.NoteHelpWord("post-ready")  // lazily creates the map
	r.NoteHelpWord("post-ready2") // and reuses it
	if !r.IsHelpWord("post-ready") || !r.IsHelpWord("post-ready2") {
		t.Errorf("recorded names must be eligible, got %v", r.helpWords)
	}
	if r.IsHelpWord("still-not-registered") {
		t.Error("recording one name must not make every name eligible")
	}
}
