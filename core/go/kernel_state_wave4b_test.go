package core

import (
	"fmt"
	"strings"
	"testing"
)

// Wave 4b coverage: the scoped-context stack (contextstack.go), the
// static-checker state surface (check.go — diagnostics, isolation,
// dynamic-scope rescue, context-type tracking), canonical rendering
// (canon.go), and the concrete-payload accessors (util.go).

// --- contextstack.go -------------------------------------------------------------

func TestContextStackLifecycle(t *testing.T) {
	cs := NewContextStack()
	if cs.Depth() != 0 || cs.Top() != nil || cs.TopData() != nil {
		t.Fatal("fresh stack not empty")
	}
	cs.Pop() // no-op on empty

	parent := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{"k": NewInteger(1)}}
	cs.Push(parent)
	if cs.Depth() != 1 {
		t.Fatalf("depth = %d", cs.Depth())
	}
	top := cs.Top()
	if top == nil || top.Prototype != parent {
		t.Error("pushed child does not chain to its parent")
	}
	// A freshly-pushed layer carries a NIL Data map, deliberately: every
	// context write goes through CowSet, which replaces the layer rather than
	// mutating it, so the map was never read from OR written to. The VM pushes
	// one of these per nested body invocation, so allocating it was pure cost
	// (see ContextStack.Push). What must hold is the OBSERVABLE contract — it
	// reads as empty, and it is writable on demand.
	if n := len(cs.TopData()); n != 0 {
		t.Errorf("a fresh layer must read as empty, got %d keys", n)
	}
	if _, ok := top.Get("nope"); ok {
		t.Error("a fresh layer must miss an unset key rather than resolve it")
	}
	// The parent's keys still resolve through the prototype chain: a nil own
	// map must not break lookup, which is the whole point of the layer.
	if v, ok := top.Get("k"); !ok || fmt.Sprint(v) != "1" {
		t.Errorf("parent key must resolve through the prototype chain, got %v %v", v, ok)
	}
	// Set is the one path that writes a layer in place, so it must allocate on
	// demand rather than panic on the nil map.
	top.Set("own", NewInteger(2))
	if v, ok := top.Get("own"); !ok || fmt.Sprint(v) != "2" {
		t.Errorf("Set on a nil-map layer must allocate and store, got %v %v", v, ok)
	}
	cs.PushExisting(parent)
	if cs.Depth() != 2 || cs.Top() != parent {
		t.Error("PushExisting did not append verbatim")
	}
	cs.PushExisting(nil) // ignored
	if cs.Depth() != 2 {
		t.Error("nil PushExisting changed the stack")
	}
	cs.Pop()
	if cs.Depth() != 1 || cs.Top() != top {
		t.Error("pop did not restore the child layer")
	}

	// Snapshot / Restore round-trip.
	snap := cs.Snapshot()
	cs.Push(top)
	cs.Push(top)
	if cs.Depth() != 3 {
		t.Fatal("pushes lost")
	}
	cs.Restore(snap)
	if cs.Depth() != 1 || cs.Top() != top {
		t.Error("restore did not roll back")
	}

	// Nil receivers are safe everywhere.
	var nilCS *ContextStack
	nilCS.Push(parent)
	nilCS.PushExisting(parent)
	nilCS.Pop()
	nilCS.Restore(nil)
	nilCS.UpdateChain(parent, parent)
	if nilCS.Depth() != 0 || nilCS.Top() != nil || nilCS.Snapshot() != nil {
		t.Error("nil ContextStack not inert")
	}
}

func TestContextStackUpdateChain(t *testing.T) {
	cs := NewContextStack()
	origRoot := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}}
	cs.PushExisting(origRoot)
	cs.Push(origRoot) // child whose Prototype chain reaches origRoot

	newRoot := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{"cow": NewInteger(1)}}
	cs.UpdateChain(origRoot, newRoot)
	// The direct entry swaps; the child's prototype pointer rewrites.
	snap := cs.Snapshot()
	if snap[0] != newRoot {
		t.Error("direct entry not swapped")
	}
	if snap[1].Prototype != newRoot {
		t.Error("child prototype not rewritten")
	}
}

// TestContextStackUpdateChainNoSelfCycle pins the guard that keeps a second
// context frame from being a crash rather than a fix.
//
// The shape is two NESTED stack entries: a child layer pushed over a parent
// that is itself a child of origRoot (`inner → outer → origRoot`), which is
// exactly what a second context frame produces. Sibling entries are not
// enough and were the first repro attempt — with both pointing straight at
// origRoot each walk relinks in one step and never advances far enough to
// meet newRoot, so the guard looked unnecessary. It is the nesting that
// exposes it:
//
//	scan from the top — inner's walk relinks outer.Prototype to newRoot;
//	then outer's own walk runs, steps through the freshly-relinked pointer
//	into newRoot, finds newRoot.Prototype == origRoot (true by construction,
//	newRoot being the COW of origRoot) and sets newRoot.Prototype = newRoot.
//
// Every later missing-key lookup then walks that self-loop forever,
// surfacing as `fatal error: stack overflow` — which recover() cannot catch
// and nothing can rescue.
//
// Asserted structurally (the prototype pointer) AND behaviourally (a real
// missing-key walk terminates), because the pointer assertion alone would
// still pass if some future relink produced a longer cycle.
//
// This is a unit test rather than a boru program because the compiled path
// currently pushes only one such entry, so the cycle is latent — see
// design/verse-report-defects-investigation.0.md §B, where a patch that added
// the second entry turned it into a default-path crash from a documented
// idiom. The guard has to be in place BEFORE that patch is attempted again,
// which means its test cannot depend on that patch existing.
func TestContextStackUpdateChainNoSelfCycle(t *testing.T) {
	origRoot := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{"seed": NewInteger(1)}}

	// Nested layers: inner → outer → origRoot, both on the stack. One entry
	// alone cannot trigger this, and neither can two siblings.
	outer := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}, Prototype: origRoot}
	inner := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{}, Prototype: outer}

	cs := NewContextStack()
	cs.PushExisting(outer)
	cs.PushExisting(inner)

	newRoot := &StoreInstanceInfo{TypeName: "Ideal/Store", Data: map[string]Value{"cow": NewInteger(2)}, Prototype: origRoot}
	cs.UpdateChain(origRoot, newRoot)

	if newRoot.Prototype == newRoot {
		t.Fatal("newRoot.Prototype was relinked to itself — the self-cycle is " +
			"back, and any later missing-key lookup is now an uncatchable " +
			"fatal error: stack overflow")
	}
	if newRoot.Prototype != origRoot {
		t.Errorf("newRoot must keep its COW parent as prototype, got %p", newRoot.Prototype)
	}
	// The layer that actually pointed at origRoot must be relinked onto the
	// new root; the one above it keeps pointing at its own parent.
	if outer.Prototype != newRoot {
		t.Errorf("outer not relinked: got %p want %p", outer.Prototype, newRoot)
	}
	if inner.Prototype != outer {
		t.Errorf("inner's own parent link must be untouched: got %p want %p",
			inner.Prototype, outer)
	}

	// Behavioural half: walking every chain for an absent key terminates.
	// A bounded walk is the assertion — an unguarded self-cycle never exits,
	// so exceeding the bound IS the failure.
	for _, entry := range cs.Snapshot() {
		steps := 0
		for p := entry; p != nil; p = p.Prototype {
			if steps++; steps > 16 {
				t.Fatalf("prototype chain does not terminate — cycle at %p", p)
			}
		}
	}
}

// --- check.go ---------------------------------------------------------------------

func TestAddDiagnosticSeverityAndFnBody(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: "x"})
	if got := r.Check.Diagnostics[0].Severity; got != SeverityFor("undefined_word") {
		t.Errorf("severity defaulted to %q", got)
	}
	// Inside a named fn body the finding is attributed.
	r.Check.FnBodyDepth = 1
	r.Check.FnNameStack = []string{"outer", "inner"}
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: "y", Word: "y"})
	d := r.Check.Diagnostics[1]
	if !d.FnBody || d.FnName != "inner" {
		t.Errorf("fn-body attribution = %+v", d)
	}
	// Recursive re-entry suppression drops emergent dispatch errors only.
	r.Check.SuppressBodyErrors = 1
	before := len(r.Check.Diagnostics)
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "no_signature", Detail: "z"})
	if len(r.Check.Diagnostics) != before {
		t.Error("suppressed dispatch error recorded")
	}
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "unused_def", Detail: "w"})
	if len(r.Check.Diagnostics) != before+1 {
		t.Error("warning suppressed alongside errors")
	}
	r.Check.SuppressBodyErrors = 0

	// TruncateDiagnostics keeps only the prefix; out-of-range is a no-op.
	n := len(r.Check.Diagnostics)
	r.Check.TruncateDiagnostics(n) // no-op
	if len(r.Check.Diagnostics) != n {
		t.Error("in-range no-op truncated")
	}
	r.Check.TruncateDiagnostics(1)
	if len(r.Check.Diagnostics) != 1 {
		t.Error("truncate did not drop the tail")
	}
	// Nil receiver safety.
	var nilC *CheckState
	nilC.AddDiagnostic(CheckDiagnostic{Code: "x"})
	nilC.TruncateDiagnostics(0)
	if nilC.IsActive() {
		t.Error("nil CheckState active")
	}
}

func TestContextTypeTracking(t *testing.T) {
	r := newTestRegistry(t)
	// Outside check mode the recorder is inert and lookups miss.
	r.Check.RecordContextSet("k", NewCarrier(TInteger))
	if _, ok := r.Check.LookupContextType("k"); ok {
		t.Error("inactive record landed")
	}
	done := r.Check.Begin()
	defer done()
	r.Check.RecordContextSet("k", NewCarrier(TInteger))
	v, ok := r.Check.LookupContextType("k")
	if !ok || !v.Parent.Equal(TInteger) {
		t.Errorf("recorded carrier = %v, %v", v, ok)
	}
	// A second write joins the carriers.
	r.Check.RecordContextSet("k", NewCarrier(TString))
	v, _ = r.Check.LookupContextType("k")
	if v.Parent.Equal(TInteger) {
		t.Errorf("joined carrier still plain Integer: %v", v)
	}
	// Empty keys are ignored; misses degrade to Any.
	r.Check.RecordContextSet("", NewCarrier(TInteger))
	v, ok = r.Check.LookupContextType("absent")
	if ok || !v.Parent.Equal(TAny) {
		t.Errorf("miss = %v, %v", v, ok)
	}
	var nilC *CheckState
	if _, ok := nilC.LookupContextType("k"); ok {
		t.Error("nil lookup hit")
	}
}

func TestDynamicScopeRescue(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// binder `f` binds "x" and calls reader `g`, whose body read of "x"
	// was flagged undefined — the rescue drops it.
	r.Check.FnNameStack = []string{"f"}
	r.Check.RecordFnBinder("x")
	r.Check.FnNameStack = nil
	r.Check.RecordCallEdge("f", "g")
	r.Check.FnBodyDepth = 1
	r.Check.FnNameStack = []string{"g"}
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: "x undefined", Word: "x"})
	// An unrelated reader keeps its diagnostic — no binder reaches it.
	r.Check.FnNameStack = []string{"h"}
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "undefined_word", Detail: "x undefined", Word: "x"})
	r.Check.FnNameStack = nil
	r.Check.FnBodyDepth = 0

	r.RescueForwardRefDiagnostics()
	var kept []string
	for _, d := range r.Check.Diagnostics {
		kept = append(kept, d.FnName)
	}
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].FnName != "h" {
		t.Errorf("rescue kept %v, want only h's finding", kept)
	}

	// callReaches follows transitive edges and detects recursion only
	// via a real cycle.
	r.Check.RecordCallEdge("a", "b")
	r.Check.RecordCallEdge("b", "c")
	if !r.Check.callReaches("a", "c") {
		t.Error("transitive reach missed")
	}
	if r.Check.callReaches("c", "a") {
		t.Error("reverse reach claimed")
	}
	if r.Check.callReaches("a", "a") {
		t.Error("non-recursive fn reaches itself")
	}
	r.Check.RecordCallEdge("c", "a")
	if !r.Check.callReaches("a", "a") {
		t.Error("recursion cycle missed")
	}
	// dynamicScopeReachable guards.
	if r.Check.DynamicScopeReachable("x", "") {
		t.Error("empty reader rescued")
	}
	if r.Check.DynamicScopeReachable("neverbound", "g") {
		t.Error("unbound name rescued")
	}
	// Engine-internal names and top-level binds are ignored.
	r.Check.RecordFnBinder("_internal")
	r.Check.FnNameStack = []string{"f"}
	r.Check.RecordFnBinder("$dollar")
	r.Check.FnNameStack = nil
	r.Check.RecordFnBinder("toplevel")
	if r.Check.FnBinders["_internal"] != nil || r.Check.FnBinders["$dollar"] != nil ||
		r.Check.FnBinders["toplevel"] != nil {
		t.Error("ignored binder names recorded")
	}
}

func TestUnusedDefDiagnosticsW4b(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.RecordDef("used", SrcPos{Row: 1})
	r.Check.RecordDef("unused", SrcPos{Row: 2})
	r.Check.RecordDef("_hidden", SrcPos{Row: 3}) // ignored
	r.Check.RecordUse("used")
	r.Check.EmitUnusedDefDiagnostics()
	var found int
	for _, d := range r.Check.Diagnostics {
		if d.Code == "unused_def" {
			found++
			if d.Word != "unused" {
				t.Errorf("flagged %q", d.Word)
			}
		}
	}
	if found != 1 {
		t.Errorf("unused_def count = %d, want 1", found)
	}
	var nilC *CheckState
	nilC.EmitUnusedDefDiagnostics()
}

// --- canon.go ---------------------------------------------------------------------

func TestCanonStringForms(t *testing.T) {
	// Plain content keeps single quotes.
	if got := CanonValue(NewString("hello")); got != "'hello'" {
		t.Errorf("plain = %q", got)
	}
	// A single quote switches to double quotes.
	if got := CanonValue(NewString("it's")); got != `"it's"` {
		t.Errorf("single-quote = %q", got)
	}
	// Both quote kinds / escapes fall back to backslash escaping.
	got := CanonValue(NewString("a'b\"c\n\td\r\\e"))
	if !strings.HasPrefix(got, "'") || !strings.Contains(got, `\'`) ||
		!strings.Contains(got, `\n`) || !strings.Contains(got, `\t`) ||
		!strings.Contains(got, `\r`) || !strings.Contains(got, `\\`) {
		t.Errorf("escaped = %q", got)
	}
}

func TestCanonValueShapes(t *testing.T) {
	if CanonValue(NewNone()) != "none" {
		t.Error("none canon broken")
	}
	if CanonValue(NewInteger(-3)) != "-3" {
		t.Error("integer canon broken")
	}
	if CanonValue(NewBoolean(true)) != "true" || CanonValue(NewBoolean(false)) != "false" {
		t.Error("boolean canon broken")
	}
	if CanonValue(NewAtom("ok")) != "ok/q" {
		t.Error("atom canon broken")
	}
	if got := CanonValue(NewTypeLiteral(TInteger)); got != "Integer" {
		t.Errorf("type literal canon = %q", got)
	}
	// Quoted lists wrap in (quote …) so the flag round-trips.
	q := NewList([]Value{NewInteger(1)})
	q.Quoted = true
	if got := CanonValue(q); got != "(quote [1])" {
		t.Errorf("quoted list canon = %q", got)
	}
	if got := CanonValue(NewList([]Value{NewInteger(1), NewInteger(2)})); got != "[1 2]" {
		t.Errorf("list canon = %q", got)
	}
	if got := CanonValue(mapOf("a", NewInteger(1))); !strings.HasPrefix(got, "{") {
		t.Errorf("map canon = %q", got)
	}
	// Flex containers carry their flexness.
	if got := CanonValue(NewFlexList([]Value{NewInteger(1)})); got != "(flex [1])" {
		t.Errorf("flex list canon = %q", got)
	}
	fm := NewOrderedMap()
	fm.Set("a", NewInteger(1))
	if got := CanonValue(NewFlexMap(fm)); !strings.HasPrefix(got, "(flex {") {
		t.Errorf("flex map canon = %q", got)
	}
	// A bounded Type canons as the /t suffix sugar.
	if got := CanonValue(NewBoundedType(TMap)); got != "Map/t" {
		t.Errorf("bounded canon = %q", got)
	}
	// DepScalars canon via their render.
	if got := CanonValue(NewDepScalar(DepGT, NewInteger(1))); !strings.Contains(got, "gt") {
		t.Errorf("depscalar canon = %q", got)
	}
	// Canon over a stack joins with spaces.
	if got := Canon([]Value{NewInteger(1), NewAtom("a")}); got != "1 a/q" {
		t.Errorf("stack canon = %q", got)
	}
}

func TestCanonReachForms(t *testing.T) {
	// Plain dotted surface.
	plain := NewReachFromKeys(NewWord("m"), []Value{NewWord("a"), NewWord("b")})
	if got := CanonValue(plain); got != "m.a.b" {
		t.Errorf("plain reach = %q", got)
	}
	// Strict step, computed key, string key, receiverless lens, quoting.
	mixed := NewReach(ReachInfo{
		Receiver: []Value{NewWord("m")},
		Segments: []ReachSeg{
			{Getr: true, KeyLit: NewWord("x")},
			{Computed: true, KeyExpr: []Value{NewWord("k")}},
			{KeyLit: NewString("lit")},
		},
	})
	got := CanonValue(mixed)
	if !strings.Contains(got, "!.x") || !strings.Contains(got, ".(k)") || !strings.Contains(got, "'lit'") {
		t.Errorf("mixed reach = %q", got)
	}
	lens := NewReach(ReachInfo{Segments: []ReachSeg{{KeyLit: NewWord("name")}}})
	if got := CanonValue(lens); got != "$.name" {
		t.Errorf("lens = %q", got)
	}
	multi := NewReach(ReachInfo{
		Receiver: []Value{NewWord("f"), NewInteger(1)},
		Segments: []ReachSeg{{KeyLit: NewWord("k")}},
	})
	if got := CanonValue(multi); got != "(f 1).k" {
		t.Errorf("multi-receiver = %q", got)
	}
	quoted := plain
	quoted.Quoted = true
	if got := CanonValue(quoted); got != "(codequote m.a.b)" {
		t.Errorf("quoted reach = %q", got)
	}
	// A nested paren token renders recursively.
	nested := NewReach(ReachInfo{
		Receiver: []Value{NewParenExpr([]Value{NewWord("g"), NewInteger(2)})},
		Segments: []ReachSeg{{KeyLit: NewWord("k")}},
	})
	if got := CanonValue(nested); got != "(g 2).k" {
		t.Errorf("paren receiver = %q", got)
	}
}

// --- util.go accessors ---------------------------------------------------------

func TestConcreteAccessorsRejectDepScalars(t *testing.T) {
	dep := NewDepScalar(DepGT, NewInteger(1))
	if _, err := dep.AsConcreteInteger(); err == nil {
		t.Error("DepScalar read as concrete integer")
	}
	sdep := NewDepScalar(DepLT, NewString("z"))
	if _, err := sdep.AsConcreteString(); err == nil {
		t.Error("DepScalar read as concrete string")
	}
	fdep := NewDepScalar(DepGTE, NewFloat(1))
	if _, err := fdep.AsConcreteFloat(); err == nil {
		t.Error("DepScalar read as concrete float")
	}
	bdep := NewDepScalar(DepLTE, NewBoolean(true))
	if _, err := bdep.AsConcreteBoolean(); err == nil {
		t.Error("DepScalar read as concrete boolean")
	}
	adep := NewDepScalar(DepGT, NewAtom("a"))
	if _, err := adep.AsConcreteAtom(); err == nil {
		t.Error("DepScalar read as concrete atom")
	}
	// The happy paths still unwrap.
	if n, err := NewInteger(4).AsConcreteInteger(); err != nil || n != 4 {
		t.Error("integer accessor broken")
	}
	if s, err := NewString("x").AsConcreteString(); err != nil || s != "x" {
		t.Error("string accessor broken")
	}
	// Atoms satisfy the textual-content contract of AsConcreteString.
	if s, err := NewAtom("tag").AsConcreteString(); err != nil || s != "tag" {
		t.Error("atom-as-string accessor broken")
	}
	if f, err := NewFloat(2.5).AsConcreteFloat(); err != nil || f != 2.5 {
		t.Error("float accessor broken")
	}
	if b, err := NewBoolean(true).AsConcreteBoolean(); err != nil || !b {
		t.Error("boolean accessor broken")
	}
	if a, err := NewAtom("z").AsConcreteAtom(); err != nil || a != "z" {
		t.Error("atom accessor broken")
	}
}

func TestMapFieldAccessors(t *testing.T) {
	m := NewOrderedMap()
	m.Set("s", NewString("v"))
	m.Set("n", NewInteger(3))
	m.Set("b", NewBoolean(true))
	m.Set("f", NewFloat(1.5))

	if s, ok := MapFieldString(m, "s"); !ok || s != "v" {
		t.Error("MapFieldString hit broken")
	}
	if _, ok := MapFieldString(m, "n"); ok {
		t.Error("MapFieldString type-mismatch hit")
	}
	if _, ok := MapFieldString(m, "zz"); ok {
		t.Error("MapFieldString miss hit")
	}
	if _, ok := MapFieldString(nil, "s"); ok {
		t.Error("nil map hit")
	}
	if n, ok := MapFieldInteger(m, "n"); !ok || n != 3 {
		t.Error("MapFieldInteger hit broken")
	}
	if _, ok := MapFieldInteger(m, "s"); ok {
		t.Error("MapFieldInteger type-mismatch hit")
	}
	if _, ok := MapFieldInteger(nil, "n"); ok {
		t.Error("nil map integer hit")
	}
	if b, ok := MapFieldBoolean(m, "b"); !ok || !b {
		t.Error("MapFieldBoolean hit broken")
	}
	if _, ok := MapFieldBoolean(m, "s"); ok {
		t.Error("MapFieldBoolean type-mismatch hit")
	}
	if _, ok := MapFieldBoolean(nil, "b"); ok {
		t.Error("nil map boolean hit")
	}
	if f, ok := MapFieldFloat(m, "f"); !ok || f != 1.5 {
		t.Error("MapFieldFloat hit broken")
	}
	if _, ok := MapFieldFloat(m, "n"); ok {
		t.Error("MapFieldFloat integer hit")
	}
	if _, ok := MapFieldFloat(nil, "f"); ok {
		t.Error("nil map float hit")
	}
}

func TestRequireConcreteListAndMap(t *testing.T) {
	if _, err := RequireConcreteList(NewList([]Value{NewInteger(1)}), "op"); err != nil {
		t.Errorf("concrete list rejected: %v", err)
	}
	if _, err := RequireConcreteList(NewTypeLiteral(TList), "op"); err == nil {
		t.Error("type literal accepted as list")
	}
	if _, err := RequireConcreteList(NewCarrier(TList), "op"); err == nil {
		t.Error("carrier accepted as list")
	}
	if _, err := RequireConcreteList(NewInteger(1), "op"); err == nil {
		t.Error("integer accepted as list")
	}
	if _, err := RequireConcreteMap(mapOf("a", NewInteger(1)), "op"); err != nil {
		t.Errorf("concrete map rejected: %v", err)
	}
	if _, err := RequireConcreteMap(NewTypeLiteral(TMap), "op"); err == nil {
		t.Error("type literal accepted as map")
	}
	if _, err := RequireConcreteMap(NewCarrier(TMap), "op"); err == nil {
		t.Error("carrier accepted as map")
	}
	if _, err := RequireConcreteMap(NewRecordType(NewOrderedMap()), "op"); err == nil {
		t.Error("record type accepted as concrete map")
	}
}
