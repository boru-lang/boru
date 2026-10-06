package core

// Seam-6b cluster B2: unit tests for previously-unreached blocks in
// fn_params.go — the non-concrete input-sig guard, the implicit-map
// optional/`?`-suffix and /q error arms, the whole colon-delimited
// single-Word param form, the type-value branches of ResolveSigType
// (surface, schema, record, typed list/map child errors, TAny tail),
// LookupDefType's def-stack arms, ResolveDefType's tail, and
// ResolveTypeName("Never"). Direct in-package calls throughout.

import (
	"strings"
	"testing"
)

// s6b2ImplicitParamMap builds the `{key: v}` implicit-pair map form the
// production parser produces for a typed param.
func s6b2ImplicitParamMap(key string, v Value) Value {
	om := NewOrderedMap()
	om.Implicit = true
	om.Set(key, v)
	return NewMap(om)
}

func TestS6b2ParseFnParamsNonConcreteInputSig(t *testing.T) {
	r := newTestRegistry(t)
	if _, _, err := ParseFnParams(r, NewCarrier(TList)); err == nil ||
		!strings.Contains(err.Error(), "concrete list") {
		t.Errorf("carrier input sig must error with the concrete-list message, got %v", err)
	}
}

func TestS6b2ParseFnParamsImplicitMapQuestionSuffixKey(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2x?", NewTypeLiteral(TInteger))})
	params, _, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if len(params) != 1 || params[0].Name != "s6b2x" || !params[0].Optional {
		t.Errorf("`s6b2x?` key must yield an optional param named s6b2x, got %+v", params)
	}
}

func TestS6b2ParseFnParamsParenAnnotationMultiValue(t *testing.T) {
	r := newTestRegistry(t)
	paren := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2y", paren)})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "must produce one type") {
		t.Errorf("multi-valued paren annotation must be a def-time error, got %v", err)
	}
}

func TestS6b2ParseFnParamsNamedQuoteAtom(t *testing.T) {
	r := newTestRegistry(t)

	// Named /q atom in the type slot that names NO type is a KEYWORD
	// slot: `s6b2z:S6b2NotAType/q` matches only the literal word
	// `S6b2NotAType`, bound to s6b2z (Quote + an Atom pattern).
	sig := NewList([]Value{s6b2ImplicitParamMap("s6b2z", NewAtom("S6b2NotAType"))})
	params, _, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("named keyword slot must parse, got %v", err)
	}
	if len(params) != 1 || !params[0].Quote || params[0].Pattern == nil || params[0].Name != "s6b2z" {
		t.Fatalf("want a named keyword slot, got %+v", params)
	}
	if name, _ := AsAtom(*params[0].Pattern); name != "S6b2NotAType" {
		t.Errorf("keyword pattern = %q, want S6b2NotAType", name)
	}

	// Atom names a real type but the param name is invalid → error
	// (name validation happens before the type lookup).
	sig = NewList([]Value{s6b2ImplicitParamMap("Bad", NewAtom("Integer"))})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("invalid word name in a /q param must error")
	}

	// A capitalised name on a keyword slot also errors (name validated
	// before the keyword branch).
	sig = NewList([]Value{s6b2ImplicitParamMap("Bad", NewAtom("S6b2NotAType"))})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("invalid word name on a keyword slot must error")
	}

	// Positive twin: valid name + valid atom TYPE is a capture-any /q
	// param (Quote, NO pattern).
	sig = NewList([]Value{s6b2ImplicitParamMap("s6b2q", NewAtom("Integer"))})
	params, _, err = ParseFnParams(r, sig)
	if err != nil || len(params) != 1 || !params[0].Quote || params[0].Pattern != nil {
		t.Errorf("named /q param on a type must be capture-any, got %+v (%v)", params, err)
	}
}

func TestS6b2ParseFnParamsImplicitMapBadName(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{s6b2ImplicitParamMap("Bad", NewTypeLiteral(TInteger))})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "function spec") {
		t.Errorf("capitalised param name must error, got %v", err)
	}
}

func TestS6b2ParseFnParamsInvalidNonImplicitMapParam(t *testing.T) {
	r := newTestRegistry(t)
	// A typed map whose ParenExpr child evaluates to two values: the
	// non-implicit map arm's ResolveSigType call must surface the error.
	badChild := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})
	tm := NewTypedMap(badChild)
	sig := NewList([]Value{tm})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid map param") {
		t.Errorf("typed-map param with a bad child expr must error, got %v", err)
	}
}

func TestS6b2ParseFnParamsColonFormFull(t *testing.T) {
	r := newTestRegistry(t)
	sig := NewList([]Value{
		NewWord("s6b2a:Integer"),
		NewWord("s6b2b?:String"),
		NewWord("s6b2c:Integer/q"),
	})
	params, _, err := ParseFnParams(r, sig)
	if err != nil {
		t.Fatalf("ParseFnParams colon form: %v", err)
	}
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if params[0].Name != "s6b2a" || !params[0].Type.Equal(TInteger) || params[0].Optional || params[0].Quote {
		t.Errorf("plain colon param wrong: %+v", params[0])
	}
	if params[1].Name != "s6b2b" || !params[1].Optional {
		t.Errorf("`?`-suffixed colon param must be optional: %+v", params[1])
	}
	if params[2].Name != "s6b2c" || !params[2].Quote {
		t.Errorf("`/q`-suffixed colon param must be a quote param: %+v", params[2])
	}
}

func TestS6b2ParseFnParamsColonFormErrors(t *testing.T) {
	r := newTestRegistry(t)

	// Unknown type name in the colon form.
	sig := NewList([]Value{NewWord("s6b2d:S6b2Nope")})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid type") {
		t.Errorf("colon form with unknown type must error, got %v", err)
	}

	// Invalid param name in the colon form.
	sig = NewList([]Value{NewWord("Bad:Integer")})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("colon form with capitalised name must error")
	}
}

func TestS6b2ParseFnParamsBareWordUnknownType(t *testing.T) {
	r := newTestRegistry(t)
	// A bare CAPITALISED word that names no type is an error: types are
	// capitalised, and this one does not exist.
	sig := NewList([]Value{NewWord("S6b2Notatype")})
	if _, _, err := ParseFnParams(r, sig); err == nil ||
		!strings.Contains(err.Error(), "invalid type") {
		t.Errorf("bare unknown type-name word must error, got %v", err)
	}
	// A bare lowercase word is an untyped NAMED param of type Any — the
	// lambda binder `[e acc] => […]` (design/IMMUTABLE-DEF.1.md §2.3).
	sig = NewList([]Value{NewWord("e"), NewWord("acc"), NewWord("Integer")})
	params, barrier, err := ParseFnParams(r, sig)
	if err != nil || len(params) != 3 || barrier != -1 {
		t.Fatalf("untyped params: %v %+v %d", err, params, barrier)
	}
	if params[0].Name != "e" || params[0].Type != TAny || params[1].Name != "acc" || params[1].Type != TAny || params[2].Name != "" || params[2].Type != TInteger {
		t.Errorf("untyped params bind by name as Any beside a bare type: %+v", params)
	}
	// A lowercase word that is no valid word name stays an error.
	sig = NewList([]Value{NewWord("3x")})
	if _, _, err := ParseFnParams(r, sig); err == nil {
		t.Error("an invalid word name is no param")
	}
}

// --- ResolveSigType type-value branches ------------------------------------

func TestS6b2ResolveSigTypeSurfaceValue(t *testing.T) {
	r := newTestRegistry(t)
	surf := NewSurfaceType(TSurface, &SurfaceInfo{Name: "S6b2Surf", Required: NewOrderedMap()})
	typ, pat, err := ResolveSigType(r, surf)
	if err != nil || pat != nil {
		t.Fatalf("surface value: err=%v pat=%v", err, pat)
	}
	if typ == nil || typ.ID != TSurface.ID {
		t.Errorf("surface param type = %v, want the surface node", typ)
	}
}

func TestS6b2ResolveSigTypeSchemaValue(t *testing.T) {
	r := newTestRegistry(t)
	node := r.Types.MintType("S6b2Box", TMap)
	schema := NewTypeSchema(node, &TypeSchemaInfo{})
	typ, pat, err := ResolveSigType(r, schema)
	if err != nil || pat != nil {
		t.Fatalf("schema value: err=%v pat=%v", err, pat)
	}
	if typ == nil || typ.ID != node.ID {
		t.Errorf("schema param type = %v, want the minted schema node", typ)
	}
}

func TestS6b2ResolveSigTypeRecordValueDirect(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	rec := NewRecordType(fields)
	typ, pat, err := ResolveSigType(r, rec)
	if err != nil {
		t.Fatalf("record value: %v", err)
	}
	if typ == nil || !typ.Equal(TMap) || pat == nil {
		t.Errorf("record param must be TMap + structural pattern, got %v / %v", typ, pat)
	}
}

func TestS6b2ResolveSigTypeTypedContainerChildErrors(t *testing.T) {
	r := newTestRegistry(t)
	badChild := NewParenExpr([]Value{NewInteger(1), NewInteger(2)})

	if _, _, err := ResolveSigType(r, NewTypedMap(badChild)); err == nil {
		t.Error("typed map with a multi-valued child expr must error")
	}
	if _, _, err := ResolveSigType(r, NewTypedList(badChild)); err == nil {
		t.Error("typed list with a multi-valued child expr must error")
	}
}

func TestS6b2ResolveSigTypeUnclassifiedTail(t *testing.T) {
	r := newTestRegistry(t)
	fn := NewFunction(FnDefInfo{})
	typ, pat, err := ResolveSigType(r, fn)
	if err != nil || pat != nil || typ == nil || !typ.Equal(TAny) {
		t.Errorf("unclassified value must fall to TAny: %v / %v / %v", typ, pat, err)
	}
}

// --- LookupDefType / ResolveDefType / ResolveTypeName -----------------------

func TestS6b2LookupDefTypeArms(t *testing.T) {
	r := newTestRegistry(t)

	// Type-stack binding with a NON-bare body (a record): returned as-is.
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	recBody := NewRecordType(fields)
	r.Defs.PushType("S6b2RecTy", TMap, recBody)
	got := LookupDefType(r, "S6b2RecTy")
	if got == nil || !IsRecordType(*got) {
		t.Errorf("type-stack record body must return the body, got %v", got)
	}

	// Def-stack value binding that is NOT a type body: nil.
	r.Defs.Push("s6b2val", NewInteger(7))
	if got := LookupDefType(r, "s6b2val"); got != nil {
		t.Errorf("plain value binding must not be a def type, got %v", got)
	}

	// Def-stack bare type literal: canonicalized lattice node.
	r.Defs.Push("S6b2Alias", NewTypeLiteral(TInteger))
	got = LookupDefType(r, "S6b2Alias")
	if got == nil || !IsBareTypeNode(*got) || got.ID != TInteger.ID {
		t.Errorf("bare-literal def binding must canonicalize to Integer, got %v", got)
	}
}

func TestS6b2ResolveDefTypeFallthrough(t *testing.T) {
	r := newTestRegistry(t)
	typ, pat, err := ResolveDefType(r, NewInteger(5))
	if err != nil || pat != nil || typ == nil || !typ.Equal(TAny) {
		t.Errorf("non-type body must fall to TAny: %v / %v / %v", typ, pat, err)
	}
}

func TestS6b2ResolveTypeNameNever(t *testing.T) {
	typ, err := ResolveTypeName("Never")
	if err != nil || typ == nil || typ.ID != TNever.ID {
		t.Errorf("ResolveTypeName(Never) = %v (%v), want TNever", typ, err)
	}
}

// --- KEYWORD slots (keywordParam / PatternsFromParams) -----------------------

// s6bKwSig wraps a single unnamed-atom param in the list-of-one-param
// input-sig shape ParseFnParams consumes.
func s6bKwSig(atom string) Value {
	return NewList([]Value{NewAtom(atom)})
}

func TestParseFnParamsUnnamedKeywordSlot(t *testing.T) {
	r := newTestRegistry(t)
	// `in/q` — `in` names no type, so it is a keyword slot: QuoteArgs
	// set AND an Atom pattern of the same name.
	params, _, err := ParseFnParams(r, s6bKwSig("in"))
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if len(params) != 1 {
		t.Fatalf("want 1 param, got %d", len(params))
	}
	p := params[0]
	if !p.Quote || p.Pattern == nil {
		t.Fatalf("keyword slot must set Quote AND a Pattern, got %+v", p)
	}
	if name, _ := AsAtom(*p.Pattern); name != "in" {
		t.Errorf("pattern must be the literal atom `in`, got %q", name)
	}
	if !p.Type.Equal(TAtom) {
		t.Errorf("keyword slot type must be Atom, got %v", p.Type)
	}
}

func TestParseFnParamsTypeNameQSlotStaysCaptureAny(t *testing.T) {
	r := newTestRegistry(t)
	// `Atom/q` — Atom IS a type, so it stays a capture-any /q slot:
	// Quote set, NO pattern.
	params, _, err := ParseFnParams(r, s6bKwSig("Atom"))
	if err != nil {
		t.Fatalf("ParseFnParams: %v", err)
	}
	if !params[0].Quote || params[0].Pattern != nil {
		t.Errorf("a type-named /q slot must capture any word (Quote, no Pattern), got %+v", params[0])
	}
}

func TestKeywordParamShape(t *testing.T) {
	p := keywordParam("k", "in")
	if p.Name != "k" || !p.Quote || !p.Type.Equal(TAtom) || p.Pattern == nil {
		t.Fatalf("keywordParam shape wrong: %+v", p)
	}
	if name, _ := AsAtom(*p.Pattern); name != "in" {
		t.Errorf("pattern atom = %q, want in", name)
	}
}

func TestPatternsFromParams(t *testing.T) {
	if PatternsFromParams(nil) != nil {
		t.Error("no params → nil patterns")
	}
	lit := NewInteger(0)
	kw := NewAtom("in")
	params := []FnParam{
		{Type: TInteger, Pattern: &lit},
		{Type: TString}, // no pattern
		{Type: TAtom, Quote: true, Pattern: &kw},
	}
	pats := PatternsFromParams(params)
	if len(pats) != 2 {
		t.Fatalf("want 2 patterns (positions 0,2), got %v", pats)
	}
	if n, _ := AsInteger(pats[0]); n != 0 {
		t.Errorf("pos 0 pattern = %v, want 0", pats[0])
	}
	if name, _ := AsAtom(pats[2]); name != "in" {
		t.Errorf("pos 2 pattern = %q, want in", name)
	}
}

func TestParseFnParamsColonFormKeywordSlot(t *testing.T) {
	r := newTestRegistry(t)
	// The single-Word colon form (minimal tokenizers): `n:in/q` where
	// `in` names no type is a keyword slot bound to n.
	params, _, err := ParseFnParams(r, NewList([]Value{NewWord("n:in/q")}))
	if err != nil {
		t.Fatalf("colon-form keyword slot must parse, got %v", err)
	}
	if len(params) != 1 || !params[0].Quote || params[0].Pattern == nil || params[0].Name != "n" {
		t.Fatalf("want a named keyword slot, got %+v", params)
	}
	if name, _ := AsAtom(*params[0].Pattern); name != "in" {
		t.Errorf("keyword pattern = %q, want in", name)
	}
	// Colon form with a real type + /q stays capture-any (no pattern).
	params, _, err = ParseFnParams(r, NewList([]Value{NewWord("k:Atom/q")}))
	if err != nil || params[0].Pattern != nil || !params[0].Quote {
		t.Errorf("colon /q on a type must be capture-any, got %+v (%v)", params, err)
	}
	// Colon form with an unknown NON-/q type still errors.
	if _, _, err := ParseFnParams(r, NewList([]Value{NewWord("k:NotAType")})); err == nil {
		t.Error("colon form with an unknown non-/q type must error")
	}
}
