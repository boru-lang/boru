package core

import (
	"strings"
	"testing"
)

// Tests for the `make` construction primitives (core_make.go), type
// installation (core_type.go), and fn-param parsing (fn_params.go).
// Positive and negative cases are paired per the repo's test discipline.

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.InitRootContext()
	return r
}

func intStrRecord() RecordTypeInfo {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	fields.Set("b", NewTypeLiteral(TString))
	return RecordTypeInfo{Fields: fields}
}

func mapOf(pairs ...any) Value {
	m := NewOrderedMap()
	for i := 0; i < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(m)
}

// --- MakeRecord ------------------------------------------------------------

func TestMakeRecordFromMap(t *testing.T) {
	rec := intStrRecord()
	out, err := MakeRecord(rec, mapOf("a", NewInteger(1), "b", NewString("x")), false)
	if err != nil {
		t.Fatalf("MakeRecord: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	m, err := AsMap(out[0])
	if err != nil {
		t.Fatalf("result not a map: %v", err)
	}
	a, _ := m.Get("a")
	if got, _ := AsInteger(a); got != 1 {
		t.Errorf("field a = %v, want 1", a)
	}
}

func TestMakeRecordFromPositionalList(t *testing.T) {
	rec := intStrRecord()
	out, err := MakeRecord(rec, NewList([]Value{NewInteger(2), NewString("y")}), false)
	if err != nil {
		t.Fatalf("MakeRecord positional: %v", err)
	}
	m, _ := AsMap(out[0])
	b, _ := m.Get("b")
	if got, _ := AsString(b); got != "y" {
		t.Errorf("field b = %v, want y", b)
	}
}

func TestMakeRecordFromNamedPairList(t *testing.T) {
	rec := intStrRecord()
	out, err := MakeRecord(rec, NewList([]Value{
		mapOf("a", NewInteger(3)),
		mapOf("b", NewString("z")),
	}), false)
	if err != nil {
		t.Fatalf("MakeRecord named pairs: %v", err)
	}
	m, _ := AsMap(out[0])
	a, _ := m.Get("a")
	if got, _ := AsInteger(a); got != 3 {
		t.Errorf("field a = %v, want 3", a)
	}
}

func TestMakeRecordRejectsUnknownField(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, mapOf("nope", NewInteger(1)), false)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field: err = %v, want unknown-field error", err)
	}
}

func TestMakeRecordRejectsMissingField(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, mapOf("a", NewInteger(1)), false)
	if err == nil || !strings.Contains(err.Error(), "missing field") {
		t.Fatalf("missing field: err = %v, want missing-field error", err)
	}
}

func TestMakeRecordUseBaseFillsDefaults(t *testing.T) {
	rec := intStrRecord()
	out, err := MakeRecord(rec, mapOf(), true)
	if err != nil {
		t.Fatalf("MakeRecord base: %v", err)
	}
	m, _ := AsMap(out[0])
	a, _ := m.Get("a")
	if got, _ := AsInteger(a); got != 0 {
		t.Errorf("base a = %v, want 0", a)
	}
	b, _ := m.Get("b")
	if got, _ := AsString(b); got != "" {
		t.Errorf("base b = %v, want empty string", b)
	}
}

func TestMakeRecordRejectsWrongPositionalCount(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, NewList([]Value{NewInteger(1)}), false)
	if err == nil || !strings.Contains(err.Error(), "expected 2 values") {
		t.Fatalf("wrong count: err = %v", err)
	}
}

func TestMakeRecordRejectsScalarSource(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, NewInteger(42), false)
	if err == nil {
		t.Fatal("scalar source accepted for record make")
	}
}

func TestMakeRecordRejectsListTypeLiteral(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, NewTypeLiteral(TList), false)
	if err == nil {
		t.Fatal("bare List type literal accepted as record source")
	}
}

func TestMakeRecordRejectsMixedNamedPositional(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, NewList([]Value{
		mapOf("a", NewInteger(1)),
		NewString("z"),
	}), false)
	if err == nil || !strings.Contains(err.Error(), "mixed") {
		t.Fatalf("mixed named/positional: err = %v", err)
	}
}

func TestMakeRecordRejectsFieldTypeMismatch(t *testing.T) {
	rec := intStrRecord()
	_, err := MakeRecord(rec, mapOf("a", NewString("not-an-int"), "b", NewString("x")), false)
	if err == nil {
		t.Fatal("string accepted for Integer-constrained field")
	}
}

// --- MakeObject / MakeClassInstance -----------------------------------------

func testClassType() ClassTypeInfo {
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	fields.Set("y", NewTypeLiteral(TString))
	return ClassTypeInfo{Fields: fields, Name: "Class/Cov", ID: "T_cov000000001"}
}

func TestMakeObjectFromMap(t *testing.T) {
	r := newTestRegistry(t)
	out, err := MakeObject(testClassType(), mapOf("x", NewInteger(5), "y", NewString("s")), r)
	if err != nil {
		t.Fatalf("MakeObject: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	oi, err := AsClassInstance(out[0])
	if err != nil {
		t.Fatalf("result not a class instance: %v", err)
	}
	v, ok := oi.GetField("x")
	if !ok {
		t.Fatal("field x missing on instance")
	}
	if got, _ := AsInteger(v); got != 5 {
		t.Errorf("x = %v, want 5", v)
	}
	// AllFields includes both.
	if oi.AllFields().Len() != 2 {
		t.Errorf("AllFields = %d entries, want 2", oi.AllFields().Len())
	}
}

func TestMakeObjectRejectsNonMap(t *testing.T) {
	r := newTestRegistry(t)
	_, err := MakeObject(testClassType(), NewInteger(1), r)
	if err == nil {
		t.Fatal("non-map source accepted for object make")
	}
}

func TestMakeObjectRejectsUnknownField(t *testing.T) {
	r := newTestRegistry(t)
	_, err := MakeObject(testClassType(), mapOf("zz", NewInteger(1)), r)
	if err == nil {
		t.Fatal("unknown field accepted for object make")
	}
}

func TestMakeObjectRejectsMissingField(t *testing.T) {
	r := newTestRegistry(t)
	_, err := MakeObject(testClassType(), mapOf("x", NewInteger(1)), r)
	if err == nil {
		t.Fatal("missing field accepted for object make")
	}
}

func TestMakeClassInstanceDirect(t *testing.T) {
	r := newTestRegistry(t)
	provided := NewOrderedMap()
	provided.Set("x", NewInteger(9))
	provided.Set("y", NewString("q"))
	v, err := MakeClassInstance(testClassType(), provided, r)
	if err != nil {
		t.Fatalf("MakeClassInstance: %v", err)
	}
	oi, err := AsClassInstance(v)
	if err != nil {
		t.Fatalf("AsClassInstance: %v", err)
	}
	if got, _ := oi.GetField("y"); got.String() == "" {
		t.Error("field y empty")
	}
	// Negative: a plain integer is not a class instance.
	if _, err := AsClassInstance(NewInteger(1)); err == nil {
		t.Error("AsClassInstance(1) should error")
	}
}

// --- MakeResource -----------------------------------------------------------

func TestMakeResource(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	rt := ResourceTypeInfo{Fields: fields, Name: "Ideal/Resource/CovRes", ID: "T_covres0000001"}

	provided := NewOrderedMap()
	provided.Set("id", NewInteger(7))
	out, err := MakeResource(rt, provided, r)
	if err != nil {
		t.Fatalf("MakeResource: %v", err)
	}
	ri, err := AsResourceInstance(out[0])
	if err != nil {
		t.Fatalf("AsResourceInstance: %v", err)
	}
	if ri.Fields == nil {
		t.Fatal("resource instance has no fields")
	}

	// Negative: unknown field declined.
	bad := NewOrderedMap()
	bad.Set("nope", NewInteger(1))
	if _, err := MakeResource(rt, bad, r); err == nil {
		t.Error("unknown resource field accepted")
	}
	// Negative: type mismatch declined.
	wrong := NewOrderedMap()
	wrong.Set("id", NewString("x"))
	if _, err := MakeResource(rt, wrong, r); err == nil {
		t.Error("string accepted for Integer resource field")
	}
	// Negatives for the accessor pair.
	if _, err := AsResourceInstance(NewInteger(1)); err == nil {
		t.Error("AsResourceInstance(1) should error")
	}
	if IsResourceType(NewInteger(1)) {
		t.Error("IsResourceType(1) = true")
	}
	if _, err := AsResourceType(NewInteger(1)); err == nil {
		t.Error("AsResourceType(1) should error")
	}
}

// --- MakeTable ---------------------------------------------------------------

func TestMakeTable(t *testing.T) {
	tt := TableTypeInfo{Record: intStrRecord()}
	rows := NewList([]Value{
		NewList([]Value{NewInteger(1), NewString("x")}),
		NewList([]Value{NewInteger(2), NewString("y")}),
	})
	out, err := MakeTable(tt, rows)
	if err != nil {
		t.Fatalf("MakeTable: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}

	// Negative: non-list source.
	if _, err := MakeTable(tt, NewInteger(1)); err == nil {
		t.Error("scalar accepted as table source")
	}
	// Negative: row with wrong arity.
	badRows := NewList([]Value{NewList([]Value{NewInteger(1)})})
	if _, err := MakeTable(tt, badRows); err == nil {
		t.Error("short row accepted for table make")
	}
	// Negative: row with wrong field type.
	wrongRows := NewList([]Value{NewList([]Value{NewString("no"), NewString("x")})})
	if _, err := MakeTable(tt, wrongRows); err == nil {
		t.Error("mistyped row accepted for table make")
	}
}

// --- MakeFieldValue / MakeConvert / FreshenDefault ---------------------------

func TestMakeFieldValue(t *testing.T) {
	// Positive: value satisfying constraint passes through.
	v, err := MakeFieldValue(NewInteger(4), NewTypeLiteral(TInteger))
	if err != nil {
		t.Fatalf("MakeFieldValue: %v", err)
	}
	if got, _ := AsInteger(v); got != 4 {
		t.Errorf("got %v, want 4", v)
	}
	// Negative: mismatch declined.
	if _, err := MakeFieldValue(NewString("s"), NewTypeLiteral(TInteger)); err == nil {
		t.Error("string accepted against Integer constraint")
	}
}

func TestMakeConvert(t *testing.T) {
	// Same-type convert is identity-ish.
	v, err := MakeConvert(NewInteger(42), TInteger)
	if err != nil {
		t.Fatalf("MakeConvert int→Integer: %v", err)
	}
	if got, _ := AsInteger(v); got != 42 {
		t.Errorf("got %v, want 42", v)
	}
	// Int → String.
	s, err := MakeConvert(NewInteger(42), TString)
	if err == nil {
		if str, _ := AsString(s); !strings.Contains(str, "42") {
			t.Errorf("convert to string: got %q", str)
		}
	}
	// String → Integer parse.
	n, err := MakeConvert(NewString("17"), TInteger)
	if err == nil {
		if got, _ := AsInteger(n); got != 17 {
			t.Errorf("parse: got %v, want 17", n)
		}
	}
	// Negative: unparseable string.
	if _, err := MakeConvert(NewString("not-a-number"), TInteger); err == nil {
		t.Error("unparseable string converted to Integer without error")
	}
}

func TestFreshenDefaultCopiesSharedMutables(t *testing.T) {
	// A value WITHOUT shared mutables passes through untouched.
	plain := NewList([]Value{NewInteger(1)})
	if !containsSharedMutable(NewStore(TStore)) {
		t.Fatal("a Store must read as a shared mutable")
	}
	if containsSharedMutable(plain) {
		t.Fatal("a plain int list must NOT read as a shared mutable")
	}
	same := FreshenDefault(plain)
	if got := same.String(); got != plain.String() {
		t.Errorf("plain list changed by FreshenDefault: %v", got)
	}

	// A list holding a Store is deep-copied: the fresh store is a
	// DIFFERENT instance, so mutating the copy leaves the original alone.
	store := NewStore(TStore)
	si, err := AsStore(store)
	if err != nil {
		t.Fatalf("AsStore: %v", err)
	}
	si.Data["k"] = NewInteger(1)
	orig := NewList([]Value{store})
	fresh := FreshenDefault(orig)
	freshElems, _ := AsMutableList(fresh)
	fsi, err := AsStore(freshElems[0])
	if err != nil {
		t.Fatalf("fresh store unreadable: %v", err)
	}
	if fsi == si {
		t.Fatal("FreshenDefault kept the SAME store instance")
	}
	fsi.Data["k"] = NewInteger(99)
	if got, _ := AsInteger(si.Data["k"]); got != 1 {
		t.Error("mutating the freshened store leaked into the original")
	}
	// A class instance holding fields is freshened by the same path.
	fields := NewOrderedMap()
	fields.Set("s", store)
	inst := NewClassInstance(TClass, ClassInstanceInfo{Fields: fields})
	fi := FreshenDefault(inst)
	oi, err := AsClassInstance(fi)
	if err != nil {
		t.Fatalf("freshened instance unreadable: %v", err)
	}
	if _, ok := oi.GetField("s"); !ok {
		t.Error("freshened instance lost its field")
	}
}

// --- makePathon ---------------------------------------------------------------

func TestMakePathon(t *testing.T) {
	out, err := MakePathon(NewString("usr/local/bin"), false)
	if err != nil {
		t.Fatalf("makePathon: %v", err)
	}
	p, err := AsPathon(out[0])
	if err != nil {
		t.Fatalf("AsPathon: %v", err)
	}
	if p.Abs {
		t.Error("relative pathon reads as absolute")
	}
	if len(p.Parts) != 3 {
		t.Errorf("parts = %v, want 3 segments", p.Parts)
	}

	abs, err := MakePathon(NewString("/etc/hosts"), true)
	if err != nil {
		t.Fatalf("makePathon abs: %v", err)
	}
	pa, _ := AsPathon(abs[0])
	if !pa.Abs {
		t.Error("absolute pathon lost its Abs flag")
	}

	// Negative: non-string source.
	if _, err := MakePathon(NewInteger(1), false); err == nil {
		t.Error("integer accepted as pathon source")
	}
	// Negative accessor.
	if _, err := AsPathon(NewInteger(1)); err == nil {
		t.Error("AsPathon(1) should error")
	}
}

func TestMakePathonWindows(t *testing.T) {
	mkVal := func(s string) Value {
		out, err := MakePathon(NewString(s), false)
		if err != nil {
			t.Fatalf("MakePathon(%q): %v", s, err)
		}
		return out[0]
	}
	mk := func(s string) PathonInfo { p, _ := AsPathon(mkVal(s)); return p }

	// Drive-absolute — both slash spellings canonicalize to backslash form.
	for _, s := range []string{`C:\Windows\System32`, "C:/Windows/System32"} {
		p := mk(s)
		if p.Volume != "C:" || !p.Abs || len(p.Parts) != 2 ||
			p.Parts[0] != "Windows" || p.Parts[1] != "System32" {
			t.Errorf("%q → %+v", s, p)
		}
		if got := p.String(); got != `C:\Windows\System32` {
			t.Errorf("%q render = %q", s, got)
		}
	}
	// Drive-relative: no separator after the colon.
	if p := mk("C:docs/x"); p.Volume != "C:" || p.Abs || p.String() != `C:docs\x` {
		t.Errorf("drive-relative = %+v (%q)", p, p.String())
	}
	// Bare drive and drive root.
	if p := mk("C:"); p.Volume != "C:" || p.Abs || len(p.Parts) != 0 || p.String() != "C:" {
		t.Errorf("bare drive = %+v (%q)", p, p.String())
	}
	if p := mk("C:/"); p.Volume != "C:" || !p.Abs || len(p.Parts) != 0 || p.String() != `C:\` {
		t.Errorf("drive root = %+v (%q)", p, p.String())
	}
	// Driveless paths stay POSIX (only "/" splits; "\" is an ordinary
	// character), so nothing that isn't drive-prefixed changes behaviour —
	// in particular a forward-slash root is NOT parsed as a Windows volume.
	if p := mk("/usr/bin"); p.Volume != "" || !p.Abs || p.String() != "/usr/bin" {
		t.Errorf("posix = %+v (%q)", p, p.String())
	}
	if p := mk("//a//b//"); p.Volume != "" || !p.Abs || len(p.Parts) != 2 || p.String() != "/a/b" {
		t.Errorf("redundant-slash root must stay POSIX, got %+v (%q)", p, p.String())
	}
	if p := mk(`a\b\c`); p.Volume != "" || p.Abs || len(p.Parts) != 1 || p.String() != `a\b\c` {
		t.Errorf("driveless backslash must be one literal segment, got %+v (%q)", p, p.String())
	}

	// Content equality: spelling-insensitive within a volume; drive-sensitive.
	if !valuesEqualDefault(mkVal("C:/x"), mkVal(`C:\x`)) {
		t.Error(`C:/x and C:\x must be equal`)
	}
	if valuesEqualDefault(mkVal("C:/x"), mkVal("D:/x")) {
		t.Error("different drives must not be equal")
	}
	if valuesEqualDefault(mkVal("C:/x"), mkVal("/x")) {
		t.Error("a drive path must not equal the driveless path")
	}

	// List form is POSIX: a leading "/" on the first element marks absolute,
	// each element splits on "/" (a backslash is an ordinary character), and
	// the result never carries a volume.
	labs, err := MakePathon(NewList([]Value{NewString("/a"), NewString("b/c")}), false)
	if err != nil {
		t.Fatalf("list makePathon: %v", err)
	}
	if p, _ := AsPathon(labs[0]); !p.Abs || p.Volume != "" || len(p.Parts) != 3 {
		t.Errorf("list form = %+v", p)
	}
}

// --- core_type.go ---------------------------------------------------------------

func TestPathOfAndTypeOf(t *testing.T) {
	// PathOf renders the ancestry as a list of segments.
	p := PathOf(NewTypeLiteral(TInteger))
	if !strings.Contains(p.String(), "Integer") {
		t.Errorf("PathOf(Integer) = %v", p)
	}
	tv := TypeOf(NewInteger(3))
	if !IsTypeLiteral(tv) {
		t.Errorf("TypeOf(3) = %v, want a type literal", tv)
	}
	if !tv.Equal(TInteger) {
		t.Errorf("TypeOf(3) = %v, want the Integer literal", tv)
	}
}

func TestIsRecordShapeAndIsValueOfType(t *testing.T) {
	if !IsValueOfType(NewInteger(1), NewTypeLiteral(TInteger)) {
		t.Error("1 should be a value of type Integer")
	}
	if IsValueOfType(NewString("s"), NewTypeLiteral(TInteger)) {
		t.Error("\"s\" should NOT be a value of type Integer")
	}
	// Record shape: a map whose values are type literals.
	shape := mapOf("a", NewTypeLiteral(TInteger))
	if !IsRecordShape(shape) {
		t.Error("map of type literals should be a record shape")
	}
	if IsRecordShape(mapOf("a", NewInteger(1))) {
		t.Error("map of concrete values should NOT be a record shape")
	}
	if IsRecordShape(NewInteger(1)) {
		t.Error("integer should NOT be a record shape")
	}
}

func TestInstallTypeAlias(t *testing.T) {
	r := newTestRegistry(t)
	if err := InstallType(r, "CovAlias", NewTypeLiteral(TInteger)); err != nil {
		t.Fatalf("InstallType alias: %v", err)
	}
	if !r.Defs.IsType("CovAlias") {
		t.Error("CovAlias not installed as a type binding")
	}
	// The bound body is the Integer literal (alias semantics).
	body, ok := r.Defs.Top("CovAlias")
	if !ok {
		t.Fatal("CovAlias has no bound body")
	}
	if !IsTypeLiteral(body) {
		t.Errorf("alias body = %v, want a type literal", body)
	}
}

func TestInstallTypeRejectsLowercaseName(t *testing.T) {
	r := newTestRegistry(t)
	err := InstallType(r, "lowercase", NewTypeLiteral(TInteger))
	if err == nil {
		t.Fatal("lowercase type name accepted")
	}
	if !strings.Contains(err.Error(), "capital") {
		t.Errorf("err = %v, want capitalisation complaint", err)
	}
}

func TestInstallTypeRejectsConcreteBody(t *testing.T) {
	r := newTestRegistry(t)
	// A word is not a type body.
	err := InstallType(r, "CovBad", NewWord("oops"))
	if err == nil {
		t.Fatal("word body accepted as type body")
	}
}

func TestInstallTypeRefinePrefab(t *testing.T) {
	r := newTestRegistry(t)
	prefab := r.Types.MintRefinePrefab(TInteger)
	if err := InstallType(r, "CovPos", NewTypeLiteral(prefab)); err != nil {
		t.Fatalf("InstallType refine prefab: %v", err)
	}
	if !r.Defs.IsType("CovPos") {
		t.Fatal("CovPos not installed")
	}
	// Nominal newtype: a plain Integer is NOT a CovPos.
	entry, _ := r.Defs.TopEntry("CovPos")
	if entry.TypeDef == nil {
		t.Fatal("CovPos binding carries no minted type")
	}
	if NewInteger(1).Is(entry.TypeDef) {
		t.Error("plain Integer must not match the bare-refine newtype")
	}
}

// --- fn_params.go ---------------------------------------------------------------

func implicitParam(name string, typeVal Value) Value {
	m := NewOrderedMap()
	m.Implicit = true
	m.Set(name, typeVal)
	return NewMap(m)
}

func TestParseFnParamsNamed(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{
		implicitParam("a", NewTypeLiteral(TInteger)),
		implicitParam("b", NewTypeLiteral(TString)),
	})
	params, barrier, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if len(params) != 2 {
		t.Fatalf("got %d params, want 2", len(params))
	}
	if params[0].Name != "a" || !params[0].Type.Equal(TInteger) {
		t.Errorf("param 0 = %+v", params[0])
	}
	if barrier != -1 {
		t.Errorf("barrier = %d, want -1 sentinel (no | in sig)", barrier)
	}
}

func TestParseFnParamsBarrierAndOptional(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{
		implicitParam("a", NewTypeLiteral(TInteger)),
		NewWord("|"),
		implicitParam("b", NewTypeLiteral(TString)),
		NewWord("?"),
	})
	params, barrier, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if barrier != 1 {
		t.Errorf("barrier = %d, want 1", barrier)
	}
	if !params[1].Optional {
		t.Error("trailing ? did not mark param optional")
	}
}

func TestParseFnParamsRejectsNonList(t *testing.T) {
	r := newTestRegistry(t)
	if _, _, err := ParseFnParams(r, NewInteger(1)); err == nil {
		t.Error("non-list input sig accepted")
	}
	if _, _, err := ParseFnParams(r, NewTypeLiteral(TList)); err == nil {
		t.Error("bare List literal accepted as input sig")
	}
}

func TestParseFnParamsRejectsMultiKeyMap(t *testing.T) {
	r := newTestRegistry(t)
	m := NewOrderedMap()
	m.Implicit = true
	m.Set("a", NewTypeLiteral(TInteger))
	m.Set("b", NewTypeLiteral(TString))
	sig := NewList([]Value{NewMap(m)})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("two-key param map accepted")
	}
}

func TestParseFnReturns(t *testing.T) {
	r := newTestRegistry(t)
	rets, pats, err := ParseFnReturns(r, NewList([]Value{NewTypeLiteral(TInteger)}))
	if err != nil {
		t.Fatalf("ParseFnReturns: %v", err)
	}
	if len(rets) != 1 || !rets[0].Equal(TInteger) {
		t.Errorf("returns = %v", rets)
	}
	// A plain type literal carries no structural constraint, so no
	// pattern slice is allocated — the common case stays allocation-free.
	if pats != nil {
		t.Errorf("a bare type-literal return needs no pattern, got %v", pats)
	}
	// A non-list return spec routes through ResolveSigType; an unknown
	// type word must be declined rather than silently degrading.
	if _, _, err := ParseFnReturns(r, NewList([]Value{NewWord("noSuchTypeZz")})); err == nil {
		t.Error("unknown return type word accepted")
	}
}

func TestResolveTypeNameAndLookups(t *testing.T) {
	tt, err := ResolveTypeName("Integer")
	if err != nil || !tt.Equal(TInteger) {
		t.Errorf("ResolveTypeName(Integer) = %v, %v", tt, err)
	}
	if _, err := ResolveTypeName("NoSuchTypeXyz"); err == nil {
		t.Error("unknown type name resolved")
	}

	r := newTestRegistry(t)
	if err := InstallType(r, "CovLocal", NewTypeLiteral(TString)); err != nil {
		t.Fatalf("InstallType: %v", err)
	}
	body := LookupDefType(r, "CovLocal")
	if body == nil {
		t.Fatal("LookupDefType(CovLocal) = nil")
	}
	if LookupDefType(r, "NotDefined") != nil {
		t.Error("LookupDefType invented a body for an undefined name")
	}

	// ResolveDefType maps the bare literal to its *Type.
	tt2, _, err := ResolveDefType(r, *body)
	if err != nil {
		t.Fatalf("ResolveDefType: %v", err)
	}
	if tt2 == nil {
		t.Fatal("ResolveDefType returned nil type")
	}
}

func TestQuoteArgsFromParams(t *testing.T) {
	qa := QuoteArgsFromParams([]FnParam{{Name: "a", Type: TAtom, Quote: true}, {Name: "b", Type: TInteger}})
	if !qa[0] || qa[1] {
		t.Errorf("QuoteArgsFromParams = %v, want {0:true}", qa)
	}
	if QuoteArgsFromParams([]FnParam{{Name: "n", Type: TInteger}}) != nil {
		t.Error("no /q params should yield nil map")
	}
}

// --- MakeHandler (the `make` word's Go entry) -------------------------------

func TestMakeHandlerScalarAndErrors(t *testing.T) {
	r := newTestRegistry(t)
	// make Integer "42" — arg order: sig position 0 is the target type.
	out, err := MakeHandler([]Value{NewTypeLiteral(TInteger), NewString("42")}, nil, nil, r)
	if err != nil {
		t.Fatalf("MakeHandler Integer \"42\": %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 42 {
		t.Errorf("got %v, want 42", out[0])
	}
	// Negative: non-type target.
	if _, err := MakeHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r); err == nil {
		t.Error("non-type make target accepted")
	}
}
