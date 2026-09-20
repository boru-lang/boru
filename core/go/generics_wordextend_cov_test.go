package core

import (
	"strings"
	"testing"
)

// Tests for generic-schema instantiation (generics_instantiate.go /
// generics.go), the open-words merge (word_extend.go), and closure
// capture analysis (fn_capture.go).

// buildBoxSchema builds `def Box gen [T] refine Record {value:T}` by
// hand: mints the placeholder, builds the record body around it, and
// installs the schema through InstallType (the same route `def` takes).
func buildBoxSchema(t *testing.T, r *Registry, name string, params []GenParam) *TypeSchemaInfo {
	t.Helper()
	spec := &GenSpecInfo{Params: params}
	PushGenBindings(r, spec)
	fields := NewOrderedMap()
	fields.Set("value", NewTypeLiteral(spec.Params[0].Node))
	body := NewRecordType(fields)
	PopGenBindings(r, spec)

	info := &TypeSchemaInfo{Params: spec.Params, Body: body, Kind: SchemaRecord}
	if err := InstallType(r, name, NewTypeSchema(TType, info)); err != nil {
		t.Fatalf("InstallType %s: %v", name, err)
	}
	return info
}

func TestInstantiateSchemaBasicAndMemo(t *testing.T) {
	r := newTestRegistry(t)
	info := buildBoxSchema(t, r, "CovBox", []GenParam{{Name: "T"}})

	inst, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)})
	if err != nil {
		t.Fatalf("InstantiateSchema: %v", err)
	}
	ri, err := AsRecordType(inst)
	if err != nil {
		t.Fatalf("instantiation is not a record type: %v", err)
	}
	fv, ok := ri.Fields.Get("value")
	if !ok {
		t.Fatal("instantiated record lost its field")
	}
	if !strings.Contains(fv.String(), "Integer") {
		t.Errorf("substituted field = %v, want Integer", fv)
	}

	// Memoisation: a second instantiation with the same args returns
	// the SAME body (const-interning; teq relies on identity).
	inst2, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)})
	if err != nil {
		t.Fatalf("second InstantiateSchema: %v", err)
	}
	ri2, _ := AsRecordType(inst2)
	if ri.Fields != ri2.Fields {
		t.Error("memo missed: same instantiation minted a fresh body")
	}
	// A DIFFERENT arg gets a different instantiation.
	inst3, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TString)})
	if err != nil {
		t.Fatalf("string InstantiateSchema: %v", err)
	}
	ri3, _ := AsRecordType(inst3)
	if ri3.Fields == ri.Fields {
		t.Error("distinct type args shared one memo entry")
	}
}

func TestInstantiateSchemaArityErrors(t *testing.T) {
	r := newTestRegistry(t)
	info := buildBoxSchema(t, r, "CovBoxA", []GenParam{{Name: "T"}})

	// Too many args.
	_, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger), NewTypeLiteral(TString)})
	if err == nil || !strings.Contains(err.Error(), "arity_mismatch") {
		t.Errorf("too many args: err = %v, want arity_mismatch", err)
	}
	// Too few args, no default.
	_, err = InstantiateSchema(r, info, nil)
	if err == nil || !strings.Contains(err.Error(), "arity_mismatch") {
		t.Errorf("missing arg: err = %v, want arity_mismatch", err)
	}
	// A schema with no minted node declines.
	if _, err := InstantiateSchema(r, &TypeSchemaInfo{}, nil); err == nil {
		t.Error("un-minted schema instantiated")
	}
}

func TestInstantiateSchemaDefault(t *testing.T) {
	r := newTestRegistry(t)
	info := buildBoxSchema(t, r, "CovBoxD", []GenParam{
		{Name: "T", HasDefault: true, Default: NewTypeLiteral(TString)},
	})
	inst, err := InstantiateSchema(r, info, nil)
	if err != nil {
		t.Fatalf("default instantiation: %v", err)
	}
	ri, _ := AsRecordType(inst)
	fv, _ := ri.Fields.Get("value")
	if !strings.Contains(fv.String(), "String") {
		t.Errorf("defaulted field = %v, want String", fv)
	}
}

func TestInstantiateSchemaBound(t *testing.T) {
	r := newTestRegistry(t)
	info := buildBoxSchema(t, r, "CovBoxB", []GenParam{
		{Name: "T", HasBound: true, Bound: NewTypeLiteral(TNumber)},
	})
	// Integer satisfies extends Number.
	if _, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TInteger)}); err != nil {
		t.Errorf("Integer against Number bound: %v", err)
	}
	// String violates it.
	_, err := InstantiateSchema(r, info, []Value{NewTypeLiteral(TString)})
	if err == nil || !strings.Contains(err.Error(), "constraint_violation") {
		t.Errorf("String against Number bound: err = %v, want constraint_violation", err)
	}
}

func TestInstantiateSchemaDeferredOnTypeParam(t *testing.T) {
	r := newTestRegistry(t)
	info := buildBoxSchema(t, r, "CovBoxF", []GenParam{{Name: "T"}})

	// An arg that still contains a placeholder defers to a GenInstRef.
	spec := &GenSpecInfo{Params: []GenParam{{Name: "U"}}}
	PushGenBindings(r, spec)
	paramLit := NewTypeLiteral(spec.Params[0].Node)
	inst, err := InstantiateSchema(r, info, []Value{paramLit})
	PopGenBindings(r, spec)
	if err != nil {
		t.Fatalf("deferred instantiation: %v", err)
	}
	if !IsGenInstRef(inst) {
		t.Fatalf("got %v, want a deferred GenInstRef", inst)
	}
	ref, err := AsGenInstRef(inst)
	if err != nil || len(ref.Args) != 1 {
		t.Fatalf("AsGenInstRef: %v / %+v", err, ref)
	}
	// Negative accessor.
	if _, err := AsGenInstRef(NewInteger(1)); err == nil {
		t.Error("AsGenInstRef(1) should error")
	}
	if IsGenInstRef(NewInteger(1)) {
		t.Error("IsGenInstRef(1) = true")
	}
}

func TestContainsTypeParam(t *testing.T) {
	r := newTestRegistry(t)
	spec := &GenSpecInfo{Params: []GenParam{{Name: "T"}}}
	PushGenBindings(r, spec)
	lit := NewTypeLiteral(spec.Params[0].Node)
	PopGenBindings(r, spec)

	if !containsTypeParam(lit) {
		t.Error("placeholder literal not detected")
	}
	if containsTypeParam(NewTypeLiteral(TInteger)) {
		t.Error("Integer literal misread as a placeholder")
	}
	// Nested in a record body.
	fields := NewOrderedMap()
	fields.Set("v", lit)
	if !containsTypeParam(NewRecordType(fields)) {
		t.Error("record holding a placeholder not detected")
	}
	// Nested in a disjunct.
	dj := NewDisjunct([]Value{NewTypeLiteral(TInteger), lit})
	if !containsTypeParam(dj) {
		t.Error("disjunct holding a placeholder not detected")
	}
	// A concrete value has no placeholders.
	if containsTypeParam(NewInteger(1)) {
		t.Error("concrete integer misread as a placeholder")
	}
}

func TestSubstituteTypeParamsWordAndRecord(t *testing.T) {
	r := newTestRegistry(t)
	bindings := map[string]Value{"T": NewTypeLiteral(TInteger)}

	// A raw word naming a parameter substitutes by name.
	got, err := SubstituteTypeParams(r, NewWord("T"), bindings, nil)
	if err != nil {
		t.Fatalf("SubstituteTypeParams word: %v", err)
	}
	if !strings.Contains(got.String(), "Integer") {
		t.Errorf("word substitution = %v, want Integer", got)
	}
	// A word naming nothing passes through.
	same, err := SubstituteTypeParams(r, NewWord("unrelated"), bindings, nil)
	if err != nil {
		t.Fatalf("unrelated word: %v", err)
	}
	w, _ := AsWord(same)
	if w.Name != "unrelated" {
		t.Errorf("unrelated word rewritten to %v", same)
	}
	// A concrete scalar passes through unchanged.
	n, err := SubstituteTypeParams(r, NewInteger(9), bindings, nil)
	if err != nil {
		t.Fatalf("scalar: %v", err)
	}
	if got, _ := AsInteger(n); got != 9 {
		t.Errorf("scalar rewritten to %v", n)
	}
}

// --- word_extend.go -----------------------------------------------------

func TestIsSealedWord(t *testing.T) {
	for _, w := range []string{"def", "make", "word"} {
		if !IsSealedWord(w) {
			t.Errorf("%q should be sealed", w)
		}
	}
	if IsSealedWord("cadd") {
		t.Error("cadd should not be sealed")
	}
}

func TestInstallWordExtensionMergeAndDispatch(t *testing.T) {
	r := covRegistry(t, nil)
	base := r.Lookup("cadd")
	if base == nil {
		t.Fatal("cadd not registered")
	}
	if !HasLockedSigs(base) {
		t.Fatal("native registration should carry locked sigs")
	}

	// R1: extending a core word requires a nominal anchor this scope
	// owns — mint a program-owned String subtype and anchor on it.
	prefab := r.Types.MintRefinePrefab(TString)
	if err := InstallType(r, "CovStr", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("InstallType CovStr: %v", err)
	}
	entry0, _ := r.Defs.TopEntry("CovStr")
	covStrT := entry0.TypeDef
	if covStrT == nil {
		t.Fatal("CovStr minted no type")
	}

	// The def-merge path (compileFnSigs) reads Params, so the extension
	// authors named params like a boru `fn [[a:CovStr b:String] …]` def.
	ext := FnDefInfo{
		Name: "cadd",
		Signatures: []Signature{{
			Params: []FnParam{{Name: "a", Type: covStrT}, {Name: "b", Type: TString}},
			Impl: Boru([]Value{
				NewOpenParen(), NewWord("ccat"), NewWord("a"), NewWord("b"), NewCloseParen(),
			}),
			Returns: []*Type{TString}, BarrierPos: BarrierAllForward,
		}},
	}
	if err := InstallWordExtension(r, "cadd", ext); err != nil {
		t.Fatalf("InstallWordExtension: %v", err)
	}

	// The clone is recognisable via the provenance helper.
	entry, ok := r.Defs.TopEntry("cadd")
	if !ok {
		t.Fatal("no def entry for the clone")
	}
	clone, isExt := IsWordExtension(entry.Body)
	if !isExt {
		t.Fatal("clone not recognised by IsWordExtension")
	}
	if clone.Extends != "cadd" {
		t.Errorf("clone.Extends = %q", clone.Extends)
	}

	// Dispatch: the anchored overload works, the locked Integer one remains.
	out, err := NewTop(r).Run([]Value{NewWord("cadd"), ReparentValue(NewString("a"), covStrT), NewString("b")})
	if err != nil {
		t.Fatalf("extended string call: %v", err)
	}
	if !strings.Contains(out[0].String(), "ab") {
		t.Errorf("string overload result = %v", out[0])
	}
	out, err = NewTop(r).Run([]Value{NewWord("cadd"), NewInteger(2), NewInteger(3)})
	if err != nil {
		t.Fatalf("locked int call after merge: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 5 {
		t.Errorf("int overload result = %v", out[0])
	}

	// Negative: IsWordExtension on a non-extension.
	if _, isExt := IsWordExtension(NewInteger(1)); isExt {
		t.Error("integer misread as a word extension")
	}
}

func TestInstallWordExtensionCompileFailures(t *testing.T) {
	r := covRegistry(t, nil)
	ext := FnDefInfo{Name: "def", Signatures: []Signature{{
		Args:    []*Type{TString},
		Impl:    Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil }),
		Returns: []*Type{}, BarrierPos: -1,
	}}}
	// Sealed word.
	if err := InstallWordExtension(r, "def", ext); err == nil || !strings.Contains(err.Error(), "sealed") {
		t.Errorf("sealed word: err = %v", err)
	}
	// Unknown base word.
	if err := InstallWordExtension(r, "no_such_base", ext); err == nil {
		t.Error("extension of unknown word accepted")
	}
	// An attempt to claim a locked kernel tuple dies at ADMISSION now
	// (R1): the all-kernel tuple has no owned anchor, so the compile failure is
	// extend_owner — the replacement can no longer even be attempted.
	// (The locked_signature arm remains reachable for non-builtin
	// locked-bearing words — see TestMergeExtensionSigsLockedCollision.)
	collide := FnDefInfo{Name: "cadd", Signatures: []Signature{{
		Args: []*Type{TInteger, TInteger},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewInteger(0)}, nil
		}),
		Returns: []*Type{TInteger}, BarrierPos: -1,
	}}}
	if err := InstallWordExtension(r, "cadd", collide); err == nil || !strings.Contains(err.Error(), "extend_owner") {
		t.Errorf("kernel-tuple claim: err = %v", err)
	}
}

// TestMergeExtensionSigsLockedCollision pins the R2 replacement-compile failure
// arm directly: a tuple equal to a LOCKED signature can never replace
// it, whatever the anchor situation.
func TestMergeExtensionSigsLockedCollision(t *testing.T) {
	r := covRegistry(t, nil)
	base := &FnDefInfo{Name: "wlock", Signatures: []Signature{{
		Args: []*Type{TInteger}, Locked: true, BarrierPos: 0,
	}}}
	incoming := []Signature{{Args: []*Type{TInteger}, BarrierPos: 0}}
	if _, _, err := mergeExtensionSigs(r, "wlock", base, incoming, ""); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("locked-tuple collision: err = %v", err)
	}
}

func TestNewWordExtensionAndTransplant(t *testing.T) {
	// Transplanting onto a registered (builtin) word requires a
	// user-minted argument type per signature — mint one first.
	r := covRegistry(t, nil)
	prefab := r.Types.MintRefinePrefab(TBoolean)
	if err := InstallType(r, "CovFlag", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("InstallType CovFlag: %v", err)
	}
	entry, _ := r.Defs.TopEntry("CovFlag")
	flagT := entry.TypeDef
	if flagT == nil {
		t.Fatal("CovFlag minted no type")
	}

	// A host-authored clone (NewWordExtension) transplants its unlocked
	// sigs onto the importer's base word. The author owner must match
	// the anchor type's owner — CovFlag is minted by this (program)
	// registry, so the clone is program-authored.
	ext := NewWordExtension(OwnerProgram, "cadd", []Signature{{
		Args: []*Type{flagT, TBoolean},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsBoolean(args[0])
			b, _ := AsBoolean(args[1])
			return []Value{NewBoolean(a && b)}, nil
		}),
		Returns: []*Type{TBoolean}, BarrierPos: -1,
	}})
	if ext.Extends != "cadd" {
		t.Fatalf("NewWordExtension.Extends = %q", ext.Extends)
	}

	if err := TransplantExtension(r, ext, "cov:module", ""); err != nil {
		t.Fatalf("TransplantExtension: %v", err)
	}
	flagVal := ReparentValue(NewBoolean(true), flagT)
	out, err := NewTop(r).Run([]Value{NewWord("cadd"), flagVal, NewBoolean(true)})
	if err != nil {
		t.Fatalf("transplanted call: %v", err)
	}
	if got, _ := AsBoolean(out[0]); !got {
		t.Errorf("transplanted overload result = %v", out[0])
	}

	// The ownership rule holds: a tuple with no author-owned nominal
	// anchor declines to transplant (TBoolean is kernel-owned, the
	// author is the program).
	allBuiltin := NewWordExtension(OwnerProgram, "cadd", []Signature{{
		Args: []*Type{TBoolean, TBoolean},
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewBoolean(false)}, nil
		}),
		Returns: []*Type{TBoolean}, BarrierPos: -1,
	}})
	if err := TransplantExtension(r, allBuiltin, "cov:module", ""); err == nil {
		t.Error("all-builtin tuple transplanted onto a core word")
	}

	// Idempotent: re-transplanting the same origin installs nothing new.
	before, _ := r.Defs.TopEntry("cadd")
	if err := TransplantExtension(r, ext, "cov:module", ""); err != nil {
		t.Fatalf("second transplant: %v", err)
	}
	after, _ := r.Defs.TopEntry("cadd")
	fnB, _ := IsWordExtension(before.Body)
	fnA, _ := IsWordExtension(after.Body)
	if len(fnA.Signatures) != len(fnB.Signatures) {
		t.Error("idempotent transplant grew the signature list")
	}

	// Sealed word declines transplant too.
	sealedExt := NewWordExtension(OwnerProgram, "make", nil)
	if err := TransplantExtension(r, sealedExt, "cov:module", ""); err == nil {
		t.Error("transplant onto sealed word accepted")
	}

	// Missing base name: degrade silently (no error, no install).
	orphan := NewWordExtension(OwnerProgram, "never_registered_word", nil)
	if err := TransplantExtension(r, orphan, "cov:module", ""); err != nil {
		t.Errorf("missing-base transplant should be a no-op, got %v", err)
	}
}

func TestNewWordExtensionKernelAuthor(t *testing.T) {
	// The kernel-author path: a kernel-shipped host module may author
	// sigs anchored on KERNEL-owned types (boru:io's Pathon list/remove)
	// by declaring OwnerKernel as the extension's author — which the
	// program-authored path declines (TestNewWordExtensionAndTransplant).
	r := covRegistry(t, nil)

	anchored := NewWordExtension(OwnerKernel, "cadd", []Signature{{
		Args: []*Type{TBoolean, TBoolean},
		Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			a, _ := AsBoolean(args[0])
			b, _ := AsBoolean(args[1])
			return []Value{NewBoolean(a && b)}, nil
		}),
		Returns: []*Type{TBoolean}, BarrierPos: -1,
	}})
	if anchored.ExtOwner != OwnerKernel {
		t.Fatalf("ExtOwner = %q, want OwnerKernel", anchored.ExtOwner)
	}
	if anchored.Extends != "cadd" {
		t.Fatalf("anchored.Extends = %q", anchored.Extends)
	}

	// Transplant succeeds: the [Boolean Boolean] tuple anchors on the
	// kernel-owned Boolean and the author IS the kernel.
	if err := TransplantExtension(r, anchored, "cov:firstparty", ""); err != nil {
		t.Fatalf("anchored transplant declined: %v", err)
	}
	out, err := NewTop(r).Run([]Value{NewWord("cadd"), NewBoolean(true), NewBoolean(true)})
	if err != nil {
		t.Fatalf("anchored overload call: %v", err)
	}
	if got, _ := AsBoolean(out[0]); !got {
		t.Errorf("anchored overload result = %v", out[0])
	}
	// The locked Integer signature still dispatches unchanged.
	out, err = NewTop(r).Run([]Value{NewWord("cadd"), NewInteger(2), NewInteger(3)})
	if err != nil {
		t.Fatalf("locked int call after anchored merge: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 5 {
		t.Errorf("locked int overload result = %v", out[0])
	}
}

// --- fn_capture.go --------------------------------------------------------

func TestWalkBodyWords(t *testing.T) {
	body := []Value{
		NewWord("a"),
		NewList([]Value{NewWord("b"), NewInteger(1)}),
		NewParenExpr([]Value{NewWord("c")}),
		NewInteger(2),
	}
	var seen []string
	WalkBodyWords(body, func(w WordInfo, _ Value) { seen = append(seen, w.Name) })
	got := strings.Join(seen, ",")
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(got, want) {
			t.Errorf("WalkBodyWords missed %q (saw %s)", want, got)
		}
	}
	// Quoted values are opaque.
	q := NewWord("hidden")
	q.Quoted = true
	seen = nil
	WalkBodyWords([]Value{q}, func(w WordInfo, _ Value) { seen = append(seen, w.Name) })
	if len(seen) != 0 {
		t.Errorf("quoted word visited: %v", seen)
	}
}

func TestComputeCaptures(t *testing.T) {
	r := newTestRegistry(t)

	// No baseline (top level): nothing captures.
	sig := &FnSig{
		Params: []FnParam{{Name: "p", Type: TInteger}},
		Impl:   Boru([]Value{NewWord("outer"), NewWord("p")}),
	}
	if caps := ComputeCaptures(r, sig); caps != nil {
		t.Errorf("top-level captures = %v, want nil", caps)
	}

	// Simulate an enclosing fn: baseline snapshot taken BEFORE `outer`
	// was bound, so the binding is fn-local and must capture.
	r.PushFnBaseline(map[string]int{})
	r.Defs.Push("outer", NewInteger(7))
	r.Defs.Push("global", NewInteger(1))
	defer func() {
		r.Defs.Pop("global")
		r.Defs.Pop("outer")
		r.PopFnBaseline()
	}()

	caps := ComputeCaptures(r, sig)
	if len(caps) != 1 || caps[0].Name != "outer" {
		t.Fatalf("captures = %+v, want exactly [outer]", caps)
	}
	if got, _ := AsInteger(caps[0].Value); got != 7 {
		t.Errorf("captured value = %v, want 7", caps[0].Value)
	}

	// A name at or below the baseline depth stays dynamic.
	r.PushFnBaseline(map[string]int{"outer": 1, "global": 1})
	if caps := ComputeCaptures(r, sig); len(caps) != 0 {
		t.Errorf("baseline-deep names captured: %+v", caps)
	}
	r.PopFnBaseline()

	// Param names never capture: body references p, which is bound too.
	r.Defs.Push("p", NewInteger(3))
	r.PushFnBaseline(map[string]int{})
	caps = ComputeCaptures(r, sig)
	for _, c := range caps {
		if c.Name == "p" {
			t.Error("param name captured")
		}
	}
	r.PopFnBaseline()
	r.Defs.Pop("p")
}

func TestMergeCaptures(t *testing.T) {
	a := []CapturedBinding{{Name: "x", Value: NewInteger(1)}, {Name: "y", Value: NewInteger(2)}}
	b := []CapturedBinding{{Name: "y", Value: NewInteger(2)}, {Name: "z", Value: NewInteger(3)}}
	merged := MergeCaptures([][]CapturedBinding{a, b})
	names := map[string]bool{}
	for _, c := range merged {
		if names[c.Name] {
			t.Errorf("duplicate capture %q after merge", c.Name)
		}
		names[c.Name] = true
	}
	for _, want := range []string{"x", "y", "z"} {
		if !names[want] {
			t.Errorf("merge lost %q", want)
		}
	}
	if MergeCaptures(nil) != nil {
		t.Error("MergeCaptures(nil) should be nil")
	}
}
