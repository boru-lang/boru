package native

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// `behave` exposed four capability slots — compare, canon, nodify,
// unify — while the kernel could dispatch more. A boru type therefore
// could say how it ORDERS, RENDERS, PROJECTS and UNIFIES, but not
// whether it is EMPTY, when two of its values are the SAME, or how big
// it is. `size` was reachable only from Go; truthiness and deep
// equality were reachable from neither.
//
// These tests pin the three slots that close that gap, in both
// directions: an installed body owns the answer, and the untouched
// builtin keeps the kernel rule.

// bcRun runs src on a fresh registry and returns the whole stack.
func bcRun(t *testing.T, src string) []Value {
	t.Helper()
	out, err := w9Run(t, w9Reg(t), src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out
}

func bcTop(t *testing.T, src string) Value {
	t.Helper()
	out := bcRun(t, src)
	if len(out) == 0 {
		t.Fatalf("run %q: empty stack", src)
	}
	return out[len(out)-1]
}

func bcBool(t *testing.T, src string) bool {
	t.Helper()
	v := bcTop(t, src)
	b, err := core.AsBoolean(v)
	if err != nil {
		t.Fatalf("run %q: not a boolean: %v (%s)", src, err, v.String())
	}
	return b
}

// TestBehaveTruthySlot: a type overrides what it means in a boolean
// position, and the type it refines does not change.
func TestBehaveTruthySlot(t *testing.T) {
	// Level says "truthy iff above 5", overriding Integer's magnitude
	// rule in BOTH directions — 3 becomes falsy, and 0 would become
	// truthy if it were above the threshold (see the second program).
	src := `
def Level (refine Integer)
behave truthy/q (fn [[Level] [Boolean] [a gt 5]])
def lo:Level 3
def hi:Level 9
[(if lo [true] [false]) (if hi [true] [false]) (if 3 [true] [false])]`
	got := bcTop(t, src)
	elems, err := core.AsMutableList(got)
	if err != nil || len(elems) != 3 {
		t.Fatalf("expected a 3-element list, got %s (%v)", got.String(), err)
	}
	lo, _ := core.AsBoolean(elems[0])
	hi, _ := core.AsBoolean(elems[1])
	plain, _ := core.AsBoolean(elems[2])
	if lo {
		t.Error("Level 3 must be falsy — the installed body said so")
	}
	if !hi {
		t.Error("Level 9 must be truthy")
	}
	if !plain {
		t.Error("a plain Integer 3 must keep the magnitude rule — the override is per-type")
	}

	// Inverting the kernel rule the other way: zero truthy.
	if !bcBool(t, `
def Always (refine Integer)
behave truthy/q (fn [[Always] [Boolean] [true]])
def z:Always 0
if z [true] [false]`) {
		t.Error("a body returning true must win over the zero-is-falsy rule")
	}
}

// TestBehaveSizeSlot: `size` was a Go-only capability; the slot makes it
// reachable from boru.
func TestBehaveSizeSlot(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave size/q (fn [[Level] [Integer] [42]])
def x:Level 3
size x`)
	n, err := core.AsInteger(v)
	if err != nil || n != 42 {
		t.Errorf("size = %s (%v), want 42 from the installed body", v.String(), err)
	}
	// The builtin rule is untouched: a plain Integer sizes to its floored
	// magnitude.
	v = bcTop(t, "size 3")
	if n, _ := core.AsInteger(v); n != 3 {
		t.Errorf("plain size 3 = %s, want 3", v.String())
	}
}

// TestBehaveDeqSlotInstalls pins the slot's wiring. The dispatch point
// itself is covered in eng (capability_gaps_test.go) because the values
// that reach DeepEqual's terminal fall-through are not constructible
// from boru source today; what is tested here is that the slot
// validates, installs, and produces a working DeepEqualer.
func TestBehaveDeqSlotInstalls(t *testing.T) {
	r := w9Reg(t)
	if _, err := w9Run(t, r, `
def Tag (refine Ideal)
behave deq/q (fn [[Tag Tag] [Boolean] [true]])`); err != nil {
		t.Fatalf("installing the deq slot: %v", err)
	}
	ty := r.LookupTypeName("Tag")
	if ty == nil {
		t.Fatal("Tag was not installed")
	}
	de, ok := ty.Behavior().(core.DeepEqualer)
	if !ok {
		t.Fatal("the deq slot did not produce a DeepEqualer")
	}
	eq, err := de.DeepEqualValues(NewInteger(1), NewInteger(2))
	if err != nil {
		t.Fatalf("running the installed deq body: %v", err)
	}
	if !eq {
		t.Error("the installed body returns true, so DeepEqualValues must")
	}
}

// TestBehaveEqSlotInstalls pins the eq slot's wiring (NUR075): `eq` is
// extensible per type on deq's terms — the same shape, the same terminal
// dispatch point (core.ExactEqualer, covered in core's
// nur075_exact_equaler_test.go, since the values reaching the terminal are
// not constructible from boru source today) — and installing it leaves deq
// alone.
func TestBehaveEqSlotInstalls(t *testing.T) {
	r := w9Reg(t)
	if _, err := w9Run(t, r, `
def Tag (refine Ideal)
behave eq/q (fn [[Tag Tag] [Boolean] [true]])`); err != nil {
		t.Fatalf("installing the eq slot: %v", err)
	}
	ty := r.LookupTypeName("Tag")
	if ty == nil {
		t.Fatal("Tag was not installed")
	}
	ee, ok := ty.Behavior().(core.ExactEqualer)
	if !ok {
		t.Fatal("the eq slot did not produce an ExactEqualer")
	}
	eq, err := ee.ExactEqualValues(NewInteger(1), NewInteger(2))
	if err != nil || !eq {
		t.Errorf("the installed body returns true, so ExactEqualValues must: %v %v", eq, err)
	}
	// deq has no body of its own here: the wrapper declines it.
	if de, ok := ty.Behavior().(core.DeepEqualer); ok {
		if _, err := de.DeepEqualValues(NewInteger(1), NewInteger(2)); err == nil {
			t.Error("an eq-only wrapper must decline deq")
		}
	}
	// A wrapper with no eq body declines eq in turn.
	r2 := w9Reg(t)
	if _, err := w9Run(t, r2, `
def Tag (refine Ideal)
behave deq/q (fn [[Tag Tag] [Boolean] [true]])`); err != nil {
		t.Fatalf("installing the deq slot: %v", err)
	}
	if ee2, ok := r2.LookupTypeName("Tag").Behavior().(core.ExactEqualer); ok {
		if _, err := ee2.ExactEqualValues(NewInteger(1), NewInteger(2)); err == nil {
			t.Error("a deq-only wrapper must decline eq")
		}
	}
}

// TestBehaveNewSlotsRejectWrongShapes is the negative half — each
// validator must refuse a fn whose shape does not match the slot's
// contract, or a body would be installed that the kernel then calls
// with the wrong arity or return type.
func TestBehaveNewSlotsRejectWrongShapes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "truthy with two params",
			src: `def T (refine Integer)
behave truthy/q (fn [[T T] [Boolean] [true]])`,
			want: "truthy: fn must take 1 arg",
		},
		{
			name: "truthy returning a non-Boolean",
			src: `def T (refine Integer)
behave truthy/q (fn [[T] [Integer] [1]])`,
			want: "truthy: fn must return Boolean",
		},
		{
			name: "deq with one param",
			src: `def T (refine Integer)
behave deq/q (fn [[T] [Boolean] [true]])`,
			want: "deq: fn must take 2 args",
		},
		{
			name: "deq over two different types",
			src: `def T (refine Integer)
def U (refine Integer)
behave deq/q (fn [[T U] [Boolean] [true]])`,
			want: "deq: both params must be the same type",
		},
		{
			name: "deq returning a non-Boolean",
			src: `def T (refine Integer)
behave deq/q (fn [[T T] [Integer] [1]])`,
			want: "deq: fn must return Boolean",
		},
		{
			name: "eq with one param",
			src: `def T (refine Integer)
behave eq/q (fn [[T] [Boolean] [true]])`,
			want: "eq: fn must take 2 args",
		},
		{
			name: "eq over two different types",
			src: `def T (refine Integer)
def U (refine Integer)
behave eq/q (fn [[T U] [Boolean] [true]])`,
			want: "eq: both params must be the same type",
		},
		{
			name: "eq returning a non-Boolean",
			src: `def T (refine Integer)
behave eq/q (fn [[T T] [Integer] [1]])`,
			want: "eq: fn must return Boolean",
		},
		{
			name: "size with two params",
			src: `def T (refine Integer)
behave size/q (fn [[T T] [Integer] [1]])`,
			want: "size: fn must take 1 arg",
		},
		{
			name: "size returning a non-Integer",
			src: `def T (refine Integer)
behave size/q (fn [[T] [Boolean] [true]])`,
			want: "size: fn must return Integer",
		},
	}
	for _, c := range cases {
		_, err := w9Run(t, w9Reg(t), c.src)
		if err == nil {
			t.Errorf("%s: accepted, want %q", c.name, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q, want it to contain %q", c.name, err.Error(), c.want)
		}
	}
}

// TestBehaveNewSlotsUntypedParamsRejected covers the remaining validator
// arm: a param with no declared type gives the slot nothing to attach
// to.
func TestBehaveNewSlotsUntypedParamsRejected(t *testing.T) {
	for _, slot := range []string{"truthy", "size", "deq", "eq"} {
		fn := "(fn [[a] [Boolean] [true]])"
		if slot == "size" {
			fn = "(fn [[a] [Integer] [1]])"
		}
		if slot == "deq" || slot == "eq" {
			fn = "(fn [[a b] [Boolean] [true]])"
		}
		if _, err := w9Run(t, w9Reg(t), "behave "+slot+"/q "+fn); err == nil {
			t.Errorf("%s: an untyped param was accepted", slot)
		}
	}
}

// TestBehaveSlotsCoexist pins that the new slots are additive with the
// existing ones — `behave` is documented as accumulating capabilities on
// one wrapper, and a second install must not drop the first.
func TestBehaveSlotsCoexist(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave truthy/q (fn [[Level] [Boolean] [a gt 5]])
behave size/q   (fn [[Level] [Integer] [99]])
behave canon/q  (fn [[Level] [String]  ["<lvl>"]])
def x:Level 3
[(if x [true] [false]) (size x) (canon x)]`)
	elems, err := core.AsMutableList(v)
	if err != nil || len(elems) != 3 {
		t.Fatalf("expected a 3-element list, got %s (%v)", v.String(), err)
	}
	if b, _ := core.AsBoolean(elems[0]); b {
		t.Error("the truthy slot was lost when size/canon were installed")
	}
	if n, _ := core.AsInteger(elems[1]); n != 99 {
		t.Errorf("size = %s, want 99 — the size slot was lost", elems[1].String())
	}
	if s, _ := core.AsString(elems[2]); !strings.Contains(s, "lvl") {
		t.Errorf("canon = %q, want the installed rendering — the canon slot was lost", s)
	}
}

// TestUnknownBehaviorNamesEverySlot pins that the unknown-name error is
// derived from the table, not written out. The literal it replaced still
// said "compare, canon, nodify, unify" after three slots were added — a
// user asking `behave truthy/q` on a typo would have been told truthy
// does not exist.
func TestUnknownBehaviorNamesEverySlot(t *testing.T) {
	_, err := w9Run(t, w9Reg(t), `def T (refine Integer)
behave nope/q (fn [[T] [Boolean] [true]])`)
	if err == nil {
		t.Fatal("an unknown behavior name was accepted")
	}
	msg := err.Error()
	for name := range behaviors {
		if !strings.Contains(msg, name) {
			t.Errorf("the unknown-name error omits the installable slot %q: %s", name, msg)
		}
	}
	// And it must not advertise a name the table does not carry.
	if strings.Contains(msg, "nope;") {
		t.Errorf("the error echoes the bad name into the known list: %s", msg)
	}
}

// TestBehaveSizeDoesNotSwallowTheWalk pins the regression the PR #325
// review surfaced: the wrapper satisfies core.Sizer structurally whether
// or not a size body was installed, and Sizer has no decline channel —
// so installing ANY slot must not change `size` for a type that never
// installed one. Before the fix, `behave canon` alone turned an Integer
// refine's size from the magnitude rule into 0.
func TestBehaveSizeDoesNotSwallowTheWalk(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave canon/q (fn [[Level] [String] ["L"]])
def x:Level 7
size x`)
	if n, err := core.AsInteger(v); err != nil || n != 7 {
		t.Errorf("size = %s (%v), want 7 — a canon-only install must keep the magnitude rule", v.String(), err)
	}
}

// TestBehaveSizeCheckerDoesNotFold pins the checker half (PR #325
// review, Codex P1): sizeReturns folds a concrete container to its
// physical element count, which is only faithful when the kernel Sizer
// owns the dispatch. A `behave size` on a List refinement answers with
// its own body at runtime, so the static fold must yield to a carrier.
func TestBehaveSizeCheckerDoesNotFold(t *testing.T) {
	r := w9Reg(t)
	if _, err := w9Run(t, r, `
def Bag (refine List)
behave size/q (fn [[Bag] [Integer] [99]])`); err != nil {
		t.Fatalf("install: %v", err)
	}
	ty := r.LookupTypeName("Bag")
	if ty == nil {
		t.Fatal("Bag was not installed")
	}
	bag := w9List(NewInteger(1), NewInteger(2), NewInteger(3))
	bag.Parent = ty

	out := sizeReturns([]Value{bag}, r)
	if len(out) != 1 {
		t.Fatalf("sizeReturns = %v, want one value", out)
	}
	if IsConcrete(out[0]) {
		t.Errorf("the fold fired despite the size body: %s — runtime answers 99, "+
			"a folded 3 makes the checker model a size the runtime never reports",
			out[0].String())
	}

	// Both negatives: a plain list still folds (the fold is the point),
	// and a refinement WITHOUT a size body folds too — its Sizer owner
	// is still the kernel List node.
	plain := w9List(NewInteger(1), NewInteger(2))
	out = sizeReturns([]Value{plain}, r)
	if n, err := core.AsInteger(out[0]); err != nil || n != 2 {
		t.Errorf("plain-list fold regressed: %v (%v)", out[0].String(), err)
	}
	if _, err := w9Run(t, r, `def Sack (refine List)`); err != nil {
		t.Fatalf("Sack: %v", err)
	}
	sack := w9List(NewInteger(1), NewInteger(2))
	sack.Parent = r.LookupTypeName("Sack")
	out = sizeReturns([]Value{sack}, r)
	if n, err := core.AsInteger(out[0]); err != nil || n != 2 {
		t.Errorf("a body-less refinement must keep the fold: %v (%v)", out[0].String(), err)
	}
}

// ─── the make slot (NUR056) ───────────────────────────────────────────
//
// Construction was the eighth kernel operation and the only one with no
// opt-in: a type could say what it MEANS in every operation that consumed
// it and nothing about how it was BUILT — while a Go-side Ideal could
// already customise construction through Ideal.Instantiate, making this a
// Go-vs-boru asymmetry as well as a missing capability.

// TestBehaveMakeSlot: the installed constructor owns `make` for its type,
// and the type it refines is untouched.
func TestBehaveMakeSlot(t *testing.T) {
	v := bcTop(t, `
def Cel (refine Float)
behave make/q (fn [[Float] [Cel] [a sub 32.0 mul 0.5555]])
[(make Cel 212.0) (make Float "2.5")]`)
	elems, err := core.AsMutableList(v)
	if err != nil || len(elems) != 2 {
		t.Fatalf("expected a 2-element list, got %s (%v)", v.String(), err)
	}
	if f, _ := core.AsFloat(elems[0]); f < 99.9 || f > 100.1 {
		t.Errorf("make Cel = %s, want the constructor's answer (~99.99)", elems[0].String())
	}
	if f, _ := core.AsFloat(elems[1]); f != 2.5 {
		t.Errorf("make Float = %s — the untouched builtin must keep the kernel rule", elems[1].String())
	}
}

// The SOURCE is arbitrary — that is what makes this a constructor rather
// than a coercion, and why the slot reads its target from the fn's return
// type rather than from a param.
func TestBehaveMakeTakesAnySource(t *testing.T) {
	if !bcBool(t, `
def Tag (refine String)
behave make/q (fn [[Any] [Tag] ["fixed"]])
(make Tag 42) eq (make Tag ["a list"])`) {
		t.Error("a constructor must accept any source shape")
	}
}

// The result is TAGGED with the target type, so the constructed value is
// really a Tag and not a bare String that happens to look like one.
func TestBehaveMakeResultCarriesTheTargetType(t *testing.T) {
	if !bcBool(t, `
def Tag (refine String)
behave make/q (fn [[Any] [Tag] ["fixed"]])
(make Tag 1) is Tag`) {
		t.Error("the constructed value must carry the target type")
	}
}

// The capability outranks the Ideal REGISTRY — the Go-side construction
// hook it is the boru-side twin of — so an object type's constructor wins
// over the kind-level default.
func TestBehaveMakeOutranksTheIdealRegistry(t *testing.T) {
	if !bcBool(t, `
def P class {a: Integer}
behave make/q (fn [[Any] [P] [make P {a: 42}]])
(make P "ignored") dot a eq 42`) {
		t.Error("a class type's own constructor must outrank the Ideal default")
	}
}

// …and the body above proves the re-entrancy guard: it calls `make P`
// on its OWN type, which would loop forever without one. The inner call
// falls through to the kernel path, exactly as the render/nodify guards
// do for their slots.
func TestBehaveMakeReentrancyFallsToTheKernel(t *testing.T) {
	if !bcBool(t, `
def Q class {a: Integer}
behave make/q (fn [[Any] [Q] [make Q {a: 1}]])
(make Q "x") dot a eq 1`) {
		t.Error("a self-recursive constructor must fall through, not loop")
	}
}

// A body returning the target's BASE type is reparented rather than
// refused — the same courtesy the kernel's own newtype construction
// extends, so `[1.0]` is a legal body.
func TestBehaveMakeReparentsABaseTypedResult(t *testing.T) {
	if !bcBool(t, `
def C (refine Float)
behave make/q (fn [[String] [C] [1.0]])
(make C "anything") is C`) {
		t.Error("a base-typed result must be reparented to the target")
	}
}

// A body returning something else entirely is REFUSED. `make` is the
// language's narrowest construction boundary; letting a body smuggle an
// off-type value through it would make the capability a hole.
func TestBehaveMakeRefusesAnOffTypeResult(t *testing.T) {
	_, err := w9Run(t, w9Reg(t), `
def C (refine Float)
behave make/q (fn [[String] [C] ["not a float"]])
make C "x"`)
	if err == nil {
		t.Fatal("an off-type constructor result was accepted")
	}
	if !strings.Contains(err.Error(), "C") {
		t.Errorf("the refusal must name the target type: %v", err)
	}
}

// A RAISING body is the constructor's answer, not a decline: falling
// through to the kernel's coercion would produce a value the type's own
// constructor refused.
func TestBehaveMakeBodyErrorReachesTheCaller(t *testing.T) {
	_, err := w9Run(t, w9Reg(t), `
def C (refine Float)
behave make/q (fn [[String] [C] [raise bad_input "C needs a number"]])
make C "x"`)
	if err == nil {
		t.Fatal("a raising constructor must not fall through to the kernel")
	}
	if !strings.Contains(err.Error(), "C needs a number") {
		t.Errorf("the body's own error must reach the caller: %v", err)
	}
}

// Installing ANOTHER slot must not change construction — the wrapper
// satisfies core.Maker structurally whether or not a make body exists,
// so the empty case has to decline. This is the same regression the size
// slot hit (PR #325) and is the reason ErrNoMaker exists.
func TestBehaveMakeDoesNotSwallowConstruction(t *testing.T) {
	v := bcTop(t, `
def Level (refine Integer)
behave canon/q (fn [[Level] [String] ["<lvl>"]])
make Level 7`)
	if n, err := core.AsInteger(v); err != nil || n != 7 {
		t.Errorf("make = %s, want 7 — a slotless type must keep kernel construction", v.String())
	}
}

func TestBehaveMakeRejectsWrongShapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "two params",
			src: `def C (refine Float)
behave make/q (fn [[String String] [C] [1.0]])`,
			want: "make: fn must take 1 arg",
		},
		{
			name: "two returns",
			src: `def C (refine Float)
behave make/q (fn [[String] [C C] [1.0 1.0]])`,
			want: "make: fn must return exactly 1 value",
		},
		{
			name: "Any return names no type",
			src: `def C (refine Float)
behave make/q (fn [[String] [Any] [1.0]])`,
			want: "must declare a concrete return type",
		},
	} {
		_, err := w9Run(t, w9Reg(t), tc.src)
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not mention %q", tc.name, err, tc.want)
		}
	}
}

// The make slot reads its target from the RETURN type and never looks at
// the param — the one place a `behave` slot takes its target from
// anywhere but a parameter. Every other slot derives T from param 0, so
// changing the param type would retarget the install; here it must not.
func TestBehaveMakeTargetsItsReturnTypeNotItsParam(t *testing.T) {
	for _, param := range []string{"Any", "String", "Integer"} {
		src := `def C (refine Float)
behave make/q (fn [[` + param + `] [C] [1.0]])
(make C "x") is C`
		out, err := w9Run(t, w9Reg(t), src)
		if err != nil {
			t.Errorf("param %s: %v", param, err)
			continue
		}
		if b, _ := core.AsBoolean(out[len(out)-1]); !b {
			t.Errorf("param %s: the install must target C, whatever the source slot says", param)
		}
	}
}

// A refinement that installs a DIFFERENT slot still inherits its base's
// constructor: its own wrapper carries no make body, so it delegates to
// the Behavior it replaced rather than declining outright. Without that
// hand-off, adding `behave canon` to a subtype would silently remove the
// base's constructor — the same shape as the size-slot regression, one
// level up the chain.
func TestBehaveMakeInheritedThroughAWrappedRefinement(t *testing.T) {
	v := bcTop(t, `
def A (refine Float)
behave make/q (fn String A [1.0])
def B (refine A)
behave canon/q (fn B String ["<b>"])
[(make B "x") ((make B "x") is B)]`)
	elems, err := core.AsMutableList(v)
	if err != nil || len(elems) != 2 {
		t.Fatalf("expected a 2-element list, got %s (%v)", v.String(), err)
	}
	if r := elems[0].String(); !strings.Contains(r, "<b>") {
		t.Errorf("the subtype's own canon must render the result, got %s", r)
	}
	if b, _ := core.AsBoolean(elems[1]); !b {
		t.Error("the base's constructor must build a value of the SUBTYPE")
	}
}

// prevMaker is a Behavior that also constructs — the shape a Go-side host
// type would install directly.
type prevMaker struct {
	core.TypeBehavior
	built Value
}

func (p prevMaker) MakeValue(*core.Type, Value) (Value, error) { return p.built, nil }

// A wrapper carrying no make body hands construction to the Behavior it
// REPLACED, rather than declining outright.
//
// Not reachable from boru source: `behave` reuses an existing wrapper on
// the same type rather than nesting, and refuses builtin types outright,
// so no boru program can put a Maker underneath a wrapper. It is reachable
// the day a Go host installs a constructing Behavior on a user type and a
// boru `behave canon` wraps it — at which point declining here would
// silently drop that constructor, since makerCapability's walk moves on to
// the PARENT node and never revisits this one.
func TestBehaveMakeDelegatesToTheWrappedBehavior(t *testing.T) {
	built := NewFloat(7)
	u := &userBehavior{prev: prevMaker{TypeBehavior: core.DefaultBehavior, built: built}}
	out, err := u.MakeValue(core.TFloat, NewString("x"))
	if err != nil {
		t.Fatalf("delegation must reach the wrapped Maker: %v", err)
	}
	if f, _ := core.AsFloat(out); f != 7 {
		t.Errorf("got %s, want the wrapped Behavior's answer", out.String())
	}
	// …and with no Maker underneath, it declines so the parent-chain walk
	// continues rather than claiming the construction.
	plain := &userBehavior{prev: core.DefaultBehavior}
	if _, err := plain.MakeValue(core.TFloat, NewString("x")); !errors.Is(err, core.ErrNoMaker) {
		t.Errorf("a slotless wrapper must decline with ErrNoMaker, got %v", err)
	}
}
