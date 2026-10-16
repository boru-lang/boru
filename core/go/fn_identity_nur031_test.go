package core

import "testing"

// NUR031: a function's `deq` is CONTENT equality — parameters, returns and
// body — while `eq` stays reference identity per NUR011. Before this,
// `f/v deq f/v` was false: a function was not deq to ITSELF, which made
// ADR-015's round-trip unsatisfiable for the kind, since a value that is
// not deq to itself cannot be deq to anything re-parsed from its canon.

func nur031Fn(t *testing.T, r *Registry, name string, body []Value) Value {
	t.Helper()
	return NewFunction(FnDefInfo{Name: name, Registry: r, Signatures: []Signature{{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru(body),
	}}})
}

func TestNUR031FnDeqIsReflexive(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	if !DeepEqual(f, f) {
		t.Fatal("a function must be deq to itself — ADR-015's prerequisite")
	}
}

// Content equality ignores the BINDING NAME, which is what made two
// references to one function compare unequal. The name is how a function
// was reached, not part of what it is.
// Two separately-minted payloads, so the identity short-circuit cannot
// answer this — it reaches canon, which is where the name-independence has
// to hold.
func TestNUR031FnDeqIgnoresTheBindingName(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	f := nur031Fn(t, r, "f", body)
	a := nur031Fn(t, r, "a", body)
	if sameFnIdentity(*f.Data().(*FnDefInfo), *a.Data().(*FnDefInfo)) {
		t.Fatal("two constructions are two identities — the canon path must answer")
	}
	if !DeepEqual(f, a) {
		t.Fatal("the same function under another name must be deq")
	}
}

// …but it does NOT ignore the body. This is the negative that the earlier
// Impl-walking attempt got wrong: after InstallFnDef a signature's Impl is
// a Go handler, so walking it found no body and every installed function
// read as identical — a false POSITIVE. canonFnDef still sees the body.
func TestNUR031FnDeqDiscriminatesTheBody(t *testing.T) {
	r, _ := NewRegistry()
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	h := nur031Fn(t, r, "h", []Value{NewInteger(2)})
	if DeepEqual(f, h) {
		t.Fatal("different bodies must NOT be deq")
	}
}

// The DEFINING MODULE is part of the content: boru resolves a function's
// free words in the module that defined it, so two identical bodies from
// different modules do not mean the same thing.
func TestNUR031FnDeqSeparatesDefiningModules(t *testing.T) {
	r1, _ := NewRegistry()
	r2, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	if DeepEqual(nur031Fn(t, r1, "f", body), nur031Fn(t, r2, "f", body)) {
		t.Fatal("identical bodies in different modules must NOT be deq")
	}
}

// eq stays REFERENCE identity: two independently-written functions with the
// same content are deq but not eq. That distinction is the reason content
// addressing answers only half of this record.
func TestNUR031EqStaysReferenceIdentity(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	f := nur031Fn(t, r, "f", body)
	g := nur031Fn(t, r, "g", body)
	if !DeepEqual(f, g) {
		t.Fatal("identical content is deq")
	}
	if ExactEqual(f, g) {
		t.Fatal("…but two independently-written functions are not eq")
	}
	if !ExactEqual(f, f) {
		t.Fatal("a function is eq to itself")
	}
}

// The record's own case: one function reached through two names. `def a
// (f/v)` and `def b (f/v)` name one function, so `a/v eq b/v` is true —
// and getting there means surviving the reach, which rebuilds the dispatch
// aggregate (and its Signatures slice) separately for each name.
func TestNUR031IdentitySurvivesRebinding(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	InstallFnDef(r, "f", FnDefInfo{Signatures: []Signature{{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewInteger(1)}),
	}}})
	fr, ok := ResolveRef(r, "f")
	if !ok {
		t.Fatal("ResolveRef f")
	}
	// What `def a (f/v)` / `def b (f/v)` leave behind: the reached value
	// bound under two further names.
	r.Defs.Push("a", fr)
	r.Defs.Push("b", fr)
	ar, _ := ResolveRef(r, "a")
	br, _ := ResolveRef(r, "b")
	if !ExactEqual(ar, br) {
		t.Fatal("two names for one function must be eq")
	}
	if !ExactEqual(ar, fr) {
		t.Fatal("…and eq to the function they were reached from")
	}
	// The aggregate really was rebuilt per name — otherwise this test would
	// pass on the old Signatures-array key and prove nothing.
	if &ar.Data().(*FnDefInfo).Signatures[0] == &br.Data().(*FnDefInfo).Signatures[0] {
		t.Fatal("expected per-name aggregates with distinct Signatures arrays")
	}
}

// A payload with no token has no identity. Every FnDefInfo built as a
// literal outside NewFunction is one — a compile-time carrier, a probe —
// and answering true would make them all eq to each other.
func TestNUR031UntokenedPayloadsAreNotReferenceEqual(t *testing.T) {
	a := FnDefInfo{Name: "a"}
	if sameFnIdentity(a, a) {
		t.Fatal("a payload with no identity token is not reference-equal")
	}
	if tokened, ok := NewFunction(a).Data().(*FnDefInfo); !ok || !sameFnIdentity(*tokened, *tokened) {
		t.Fatal("…but NewFunction mints one, and a fn is eq to itself")
	}
}

// The bucketed collection scans (unique / member / indices / group) must
// agree with deq, or `unique` keeps what `deq` calls a duplicate. Before
// this, DeqKey classified a function DeqNeverEqual — filed nowhere,
// matching nothing — which was correct only while deq rejected functions.
func TestNUR031FnDeqKeyMatchesDeq(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	f := nur031Fn(t, r, "f", body)
	g := nur031Fn(t, r, "g", body)
	h := nur031Fn(t, r, "h", []Value{NewInteger(2)})
	if _, c := DeqKey(f); c != DeqKeyed {
		t.Fatalf("a function must be DeqKeyed, got class %d", c)
	}
	if deqFam(f) != "F" {
		t.Fatalf("a function must scan in the fn family, got %q", deqFam(f))
	}
	// Deq-equal functions share a bucket; a different body does not.
	fk, _ := DeqKey(f)
	gk, _ := DeqKey(g)
	hk, _ := DeqKey(h)
	if fk != gk {
		t.Error("deq-equal functions must share a bucket")
	}
	if fk == hk {
		t.Error("a different body must not share a bucket")
	}
	// End to end through the index — what `unique` actually calls.
	var idx DeqIndex
	idx.Add(f)
	if got := idx.FirstMatch(g); got != 0 {
		t.Errorf("FirstMatch(deq-equal fn) = %d, want 0", got)
	}
	if got := idx.FirstMatch(h); got != -1 {
		t.Errorf("FirstMatch(different fn) = %d, want -1", got)
	}
}

// The same agreement for the declared TYPE values, the other kind this
// record moved out of the never-equal class.
func TestNUR031TypeValueDeqKeyMatchesDeq(t *testing.T) {
	// A disjunction — `def E enum ['a' 'b']`'s shape — is the cheapest
	// declared type body to build here; the arm keys on the type body,
	// not on which declarer produced it.
	pv := NewDisjunct([]Value{NewString("a"), NewString("b")})
	qv := NewEnum([]Value{NewString("a"), NewString("b")})
	if !IsTypeBody(&pv) || !IsTypeBody(&qv) {
		t.Fatal("fixture: both values must be type bodies")
	}
	if !DeepEqual(pv, pv) {
		t.Fatal("a declared type must be deq to itself")
	}
	if DeepEqual(pv, qv) {
		t.Fatal("…and not to a different declaration")
	}
	pk, c := DeqKey(pv)
	if c != DeqKeyed {
		t.Fatalf("a declared type must be DeqKeyed, got class %d", c)
	}
	if qk, _ := DeqKey(qv); pk == qk {
		t.Error("two declarations must not share a bucket")
	}
	var idx DeqIndex
	idx.Add(pv)
	if got := idx.FirstMatch(pv); got != 0 {
		t.Errorf("FirstMatch(self) = %d, want 0", got)
	}
	if got := idx.FirstMatch(qv); got != -1 {
		t.Errorf("FirstMatch(other declaration) = %d, want -1", got)
	}
}

// A compiled CLOSURE is the VM's spelling of a fn value, and it must
// answer deq the way the interpreter's fn value does — by content. It
// reaches the type-body arm rather than the fn arm (its payload is a
// ClosurePayload, not a FnDefInfo), and the comparison lands on its
// Render, which is the fn's canon: the same string fnStructurallyEqual
// compares. Without this the two engines would disagree about `deq` on
// the same function.
func TestNUR031CompiledClosureDeqMatchesTheInterpreter(t *testing.T) {
	a := NewValueRaw(TFunction, &ClosurePayload{Unit: 1, Render: "fn [[][Integer][1]]"})
	twin := NewValueRaw(TFunction, &ClosurePayload{Unit: 1, Render: "fn [[][Integer][1]]"})
	other := NewValueRaw(TFunction, &ClosurePayload{Unit: 2, Render: "fn [[][Integer][2]]"})
	if !DeepEqual(a, a) {
		t.Fatal("a closure must be deq to itself")
	}
	if !DeepEqual(a, twin) {
		t.Fatal("…and to a closure of the same function — content, like the interpreter")
	}
	if DeepEqual(a, other) {
		t.Fatal("…but not to a closure of a different function")
	}
}

// A function against a NON-function: the fn arms claim the pair as soon as
// either side is one, so they must answer for the mixed case too rather
// than falling through to an arm that reads a FnDefInfo as something else.
func TestNUR031FnAgainstNonFn(t *testing.T) {
	r, _ := NewRegistry()
	f := nur031Fn(t, r, "f", []Value{NewInteger(1)})
	other := NewInteger(1)
	if ExactEqual(f, other) || ExactEqual(other, f) {
		t.Fatal("a function is not eq to a non-function, in either order")
	}
	if DeepEqual(f, other) || DeepEqual(other, f) {
		t.Fatal("…nor deq to one")
	}
}

// ─── PR #377 review (Codex) ───────────────────────────────────────────

// CAPTURES are part of a function's content. canonFnDef renders params,
// returns and body — not the closure environment — so two closures built
// by one factory over different arguments render identically and behave
// entirely differently. Without this, `(make-adder 5)` and
// `(make-adder 9)` were deq, and `unique` would discard one of them.
func TestNUR031ClosureCapturesAreContent(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	mk := func(captured Value) FnDefInfo {
		fd, _ := FnDefCopy(nur031Fn(t, r, "adder", body).Data())
		fd.ident = nil // two separate constructions, so canon must answer
		fd.Captured = []CapturedBinding{{Name: "x", Value: captured}}
		return fd
	}
	five, nine := mk(NewInteger(5)), mk(NewInteger(9))
	if fnStructurallyEqual(five, nine) {
		t.Fatal("closures over different captures must NOT be deq")
	}
	if !fnStructurallyEqual(five, mk(NewInteger(5))) {
		t.Fatal("…but equal captures over one body must be")
	}
	// A differing capture NAME counts too, not just the value.
	renamed := mk(NewInteger(5))
	renamed.Captured = []CapturedBinding{{Name: "y", Value: NewInteger(5)}}
	if fnStructurallyEqual(five, renamed) {
		t.Fatal("a differently-NAMED capture is a different environment")
	}
	// And an absent environment is not the same as a present one.
	bare := mk(NewInteger(5))
	bare.Captured = nil
	if fnStructurallyEqual(five, bare) {
		t.Fatal("a closure is not deq to the same body with no captures")
	}
}

// The DEFINING SCOPE is content, and a nil Registry is a scope like any
// other — "wherever this is running", which is not a named module. An
// earlier `both non-nil` guard admitted the mixed pair, so a
// module-owned fn and a locally defined one with identical text compared
// deq although their free words resolve in different registries.
func TestNUR031NilAndModuleScopesAreDifferentContent(t *testing.T) {
	r, _ := NewRegistry()
	body := []Value{NewInteger(1)}
	owned, _ := FnDefCopy(nur031Fn(t, r, "f", body).Data())
	local := owned
	local.ident = nil
	local.Registry = nil
	owned.ident = nil
	if fnStructurallyEqual(owned, local) {
		t.Fatal("a module-owned fn and a locally defined one are different content")
	}
	other := local
	if !fnStructurallyEqual(local, other) {
		t.Fatal("…while two locally defined ones still compare on content")
	}
}

// A dispatch aggregate that UNIONS several stacked entries is a
// composite no single authored function identifies: its lower overloads
// come from entries the top one never saw. Inheriting the top entry's
// token there would make two such composites eq although their deeper
// overloads run different code.
func TestNUR031CompositeAggregateHasNoIdentity(t *testing.T) {
	tok := &fnIdent{}
	one := []FnDefInfo{{ident: tok}}
	if aggregateIdent(one) != tok {
		t.Fatal("a sole entry IS the function — its token rides the aggregate")
	}
	two := []FnDefInfo{{ident: tok}, {ident: &fnIdent{}}}
	if aggregateIdent(two) != nil {
		t.Fatal("a multi-entry union must not inherit the top entry's identity")
	}
	if aggregateIdent(nil) != nil {
		t.Fatal("an empty entry list has no identity either")
	}
}
