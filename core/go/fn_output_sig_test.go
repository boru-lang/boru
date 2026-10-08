package core

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// Unit pins for the output-slot rework (design/legacy/FN-OUTPUT-SIG.0.ignore): the
// `name:Type` return unwrap, the declaration-span fallback that makes the
// two spellings of one declaration diagnose alike, and the value
// renderer. The end-to-end behaviour lives in lang/spec/fn-triple.tsv §7;
// these cover the arms a spec row cannot reach.

// implicitMap builds the value a `name:Type` pair lowers to.
func implicitMap(pairs ...Value) Value {
	om := NewOrderedMap()
	om.Implicit = true
	for i := 0; i+1 < len(pairs); i += 2 {
		k, _ := AsString(pairs[i])
		om.Set(k, pairs[i+1])
	}
	return NewMap(om)
}

func TestUnwrapNamedReturnImplicitPair(t *testing.T) {
	sig := implicitMap(NewString("i"), NewTypeLiteral(TInteger))
	got, err := unwrapNamedReturn(nil, sig)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if !IsBareTypeNode(got) || !ValueType(got).Equal(TInteger) {
		t.Fatalf("unwrapped = %v, want the Integer type literal", got)
	}
}

func TestUnwrapNamedReturnLeavesEverythingElse(t *testing.T) {
	// A non-map value is returned untouched.
	word := NewWord("Integer")
	if got, err := unwrapNamedReturn(nil, word); err != nil || !IsWord(got) {
		t.Errorf("a word must pass through: %v, %v", got, err)
	}
	// An EXPLICIT map declares a Map-typed return — not a named one — so
	// it must survive the unwrap intact.
	om := NewOrderedMap()
	om.Set("i", NewTypeLiteral(TInteger))
	explicit := NewMap(om)
	got, err := unwrapNamedReturn(nil, explicit)
	if err != nil {
		t.Fatalf("explicit map: %v", err)
	}
	if !got.Parent.Equal(TMap) {
		t.Errorf("an explicit map must pass through, got %v", got)
	}
	// A Map-typed value with no payload takes the same early return.
	if got, err := unwrapNamedReturn(nil, NewTypeLiteral(TMap)); err != nil || !IsBareTypeNode(got) {
		t.Errorf("a bare Map type node must pass through: %v, %v", got, err)
	}
}

func TestUnwrapNamedReturnErrors(t *testing.T) {
	two := implicitMap(
		NewString("i"), NewTypeLiteral(TInteger),
		NewString("j"), NewTypeLiteral(TString),
	)
	if _, err := unwrapNamedReturn(nil, two); err == nil ||
		!strings.Contains(err.Error(), "exactly one key") {
		t.Errorf("a two-key return map must error, got %v", err)
	}

	bad := implicitMap(NewString("2bad"), NewTypeLiteral(TInteger))
	if _, err := unwrapNamedReturn(nil, bad); err == nil {
		t.Error("an invalid return name must error")
	}
}

func TestParseFnReturnsNamedPair(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Bare pair, and the same pair wrapped in a list: one declaration,
	// two spellings, one reading.
	pair := implicitMap(NewString("i"), NewWord("Integer"))
	for _, sig := range []Value{pair, NewList([]Value{pair})} {
		types, pats, perr := ParseFnReturns(r, sig)
		if perr != nil {
			t.Fatalf("ParseFnReturns(%v): %v", sig, perr)
		}
		if len(types) != 1 || !types[0].Equal(TInteger) {
			t.Errorf("types = %v, want [Integer]", types)
		}
		if pats != nil {
			t.Errorf("a named type return carries no pattern, got %v", pats)
		}
	}

	// The error arm propagates out of BOTH shapes: the list walk, and
	// the bare single-return sig, which takes its own branch.
	twoKey := implicitMap(
		NewString("i"), NewWord("Integer"),
		NewString("j"), NewWord("String"),
	)
	if _, _, perr := ParseFnReturns(r, NewList([]Value{twoKey})); perr == nil {
		t.Error("a bad named return inside a list must propagate")
	}
	if _, _, perr := ParseFnReturns(r, twoKey); perr == nil {
		t.Error("a bad named return as the bare output sig must propagate")
	}
}

func TestSigDeclPos(t *testing.T) {
	// A slot with its own position answers with it.
	own := NewList([]Value{NewWord("Integer")})
	own.SetPos(SrcPos{Row: 3, Col: 7})
	if p := sigDeclPos(own); p.Row != 3 || p.Col != 7 {
		t.Errorf("own position = %+v, want 3:7", p)
	}

	// A `name:Type` pair has none, so the span falls to the declared type
	// inside it — the fix that stops the pair spelling losing the
	// declaration span the list spelling gets.
	typ := NewWord("Integer")
	typ.SetPos(SrcPos{Row: 1, Col: 22})
	if p := sigDeclPos(implicitMap(NewString("i"), typ)); p.Row != 1 || p.Col != 22 {
		t.Errorf("map descent = %+v, want 1:22", p)
	}

	// A positionless LIST descends to its first positioned element, and
	// skips the unpositioned ones ahead of it.
	nested := NewList([]Value{NewWord("String"), implicitMap(NewString("i"), typ)})
	if p := sigDeclPos(nested); p.Row != 1 || p.Col != 22 {
		t.Errorf("list descent = %+v, want 1:22", p)
	}

	// Nothing positioned anywhere: unknown, and attachDeclSpan drops the
	// span rather than pointing at 0:0.
	if p := sigDeclPos(NewList([]Value{NewWord("Integer")})); p.Row != 0 {
		t.Errorf("positionless sig = %+v, want the zero SrcPos", p)
	}
	if p := sigDeclPos(NewTypeLiteral(TInteger)); p.Row != 0 {
		t.Errorf("bare type node = %+v, want the zero SrcPos", p)
	}
}

func TestReturnCountErrorTextShowsValues(t *testing.T) {
	// The values ride in the detail (design/DIAGNOSTIC-VALUES.0.md).
	got := ReturnCountErrorText("f", 1, 2, []Value{NewInteger(1), NewString("x")})
	if !strings.Contains(got, "got 2 — [1 'x']") {
		t.Errorf("detail = %q, want the values shown", got)
	}
	// No values is a real answer: the bare count, no empty brackets.
	if got := ReturnCountErrorText("f", 1, 0, nil); !strings.HasSuffix(got, "got 0") {
		t.Errorf("valueless detail = %q, want the bare count", got)
	}
}

func TestDiagValueBoundsEveryShape(t *testing.T) {
	// A head limit is per-LEVEL, so a SHORT list of enormous elements
	// slips it entirely: this outer list has length 1. Before the
	// renderer recursed, the whole nested run reached the message.
	inner := make([]Value, 5000)
	for i := range inner {
		inner[i] = NewInteger(int64(i))
	}
	nested := diagValueList([]Value{NewList([]Value{NewList(inner)})})
	if len(nested) > diagMaxRendered {
		t.Errorf("nested list unbounded: %d chars", len(nested))
	}
	if !strings.Contains(nested, "… (4992 more)") {
		t.Errorf("nested list must elide its tail, got %q", nested)
	}

	// A map was not bounded at all.
	om := NewOrderedMap()
	for i := 0; i < 3000; i++ {
		om.Set(strconv.Itoa(i), NewInteger(int64(i)))
	}
	gotMap := diagValueList([]Value{NewMap(om)})
	if len(gotMap) > diagMaxRendered {
		t.Errorf("map unbounded: %d chars", len(gotMap))
	}
	if !strings.Contains(gotMap, "… (2992 more)") {
		t.Errorf("map must elide its tail, got %q", gotMap)
	}

	// Past diagMaxDepth only the shape is kept — for either container.
	deep := NewInteger(1)
	for i := 0; i < 12; i++ {
		deep = NewList([]Value{deep})
	}
	if got := diagValue(deep); !strings.Contains(got, "[…]") {
		t.Errorf("deep list nesting must bottom out in a shape marker, got %q", got)
	}
	deepMap := NewInteger(1)
	for i := 0; i < 12; i++ {
		om := NewOrderedMap()
		om.Set("k", deepMap)
		deepMap = NewMap(om)
	}
	if got := diagValue(deepMap); !strings.Contains(got, "{…}") {
		t.Errorf("deep map nesting must bottom out in a shape marker, got %q", got)
	}

	// A TYPED container conforms to TList/TMap and is concrete, but
	// carries a ChildTypeInfo payload rather than elements — so the
	// container walk cannot read it, and it renders itself. Reachable,
	// which is why neither guard is allowlisted.
	typedList := NewValueRaw(TList, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	if got := diagValue(typedList); got != typedList.String() {
		t.Errorf("typed list = %q, want its own rendering", got)
	}
	typedMap := NewValueRaw(TMap, ChildTypeInfo{Child: NewTypeLiteral(TInteger)})
	if got := diagValue(typedMap); got != typedMap.String() {
		t.Errorf("typed map = %q, want its own rendering", got)
	}

	// A non-concrete value (a bare type node) has no payload to walk and
	// falls straight to its own rendering.
	if got := diagValue(NewTypeLiteral(TList)); got != NewTypeLiteral(TList).String() {
		t.Errorf("a bare type node = %q, want its own rendering", got)
	}

	// A single enormous SCALAR is no container, so no structural limit
	// applies — the character backstop is what bounds it, and it marks
	// the cut so a shortened rendering never reads as a complete one.
	big := strings.Repeat("x", 9000)
	gotBig := diagValue(NewString(big))
	if len(gotBig) > diagMaxRendered+8 {
		t.Errorf("scalar unbounded: %d chars", len(gotBig))
	}
	if !strings.HasSuffix(gotBig, "…") {
		t.Errorf("a clamped value must be marked, got the tail %q", gotBig[len(gotBig)-8:])
	}

	// The clamp cuts on a rune boundary, never mid-character.
	if !utf8.ValidString(diagValue(NewString(strings.Repeat("é", 9000)))) {
		t.Error("clamping must not split a multi-byte rune")
	}

	// And none of this disturbs values small enough to render exactly.
	if got := diagValueList([]Value{NewInteger(1), NewString("x")}); got != "[1 'x']" {
		t.Errorf("small values = %q, want them rendered exactly", got)
	}
	if got := diagValueList([]Value{NewList([]Value{NewInteger(1), NewInteger(2)})}); got != "[[1 2]]" {
		t.Errorf("small nested list = %q, want it rendered exactly", got)
	}
}

func TestDiagValueListAbbreviates(t *testing.T) {
	if got := diagValueList(nil); got != "[]" {
		t.Errorf("empty = %q", got)
	}
	vals := make([]Value, diagMaxListHead+3)
	for i := range vals {
		vals[i] = NewInteger(int64(i))
	}
	got := diagValueList(vals)
	if !strings.Contains(got, "… (3 more)") {
		t.Errorf("over-limit run must elide its tail, got %q", got)
	}
	if strings.Contains(got, strings.TrimSpace(NewInteger(int64(diagMaxListHead)).String())+" ") {
		t.Errorf("elided values must not be rendered, got %q", got)
	}
}

func TestResolveSigTypeStringShapeGate(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A String naming a real type still resolves as a NAME.
	if got, pat, rerr := ResolveSigType(r, NewString("Integer")); rerr != nil ||
		pat != nil || !got.Equal(TInteger) {
		t.Errorf("'Integer' = %v/%v/%v, want the Integer type", got, pat, rerr)
	}
	// A String that could never be a type name is the literal itself.
	got, pat, rerr := ResolveSigType(r, NewString("ok"))
	if rerr != nil || pat == nil || !got.Equal(TString) {
		t.Fatalf("'ok' = %v/%v/%v, want a String literal pattern", got, pat, rerr)
	}
	if s, aerr := AsString(*pat); aerr != nil || s != "ok" {
		t.Errorf("'ok' pattern = %v (%v)", *pat, aerr)
	}
	// A MISSPELLED type name has the shape of one, so it stays a loud
	// error rather than silently becoming a literal that matches nothing.
	if _, _, rerr := ResolveSigType(r, NewString("Integr")); rerr == nil {
		t.Error("'Integr' must stay an unknown-type error, not become a literal")
	}
	// A word is always a name, whatever its shape.
	if _, _, rerr := ResolveSigType(r, NewWord("ok")); rerr == nil {
		t.Error("a bare lowercase word in a type slot must error")
	}
}

func TestLooksLikeTypeName(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"Integer", true},
		{"Scalar/Number/Integer", true},
		{"ok", false},
		{"Scalar/number", false},
		{"42", false},
		{"", false},
	} {
		if got := looksLikeTypeName(c.name); got != c.want {
			t.Errorf("looksLikeTypeName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestUnwrapNamedReturnColonWord(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// The single-Word colon form a minimal tokenizer produces. Without
	// this arm the input/output symmetry held only for consumers of the
	// production parser — ParseFnParams accepts the same token as a param.
	w := NewWord("i:Integer")
	w.SetPos(SrcPos{Row: 4, Col: 9})
	got, uerr := unwrapNamedReturn(r, w)
	if uerr != nil {
		t.Fatalf("unwrap: %v", uerr)
	}
	if !IsWord(got) {
		t.Fatalf("unwrapped = %v, want a Word", got)
	}
	if gw, _ := AsWord(got); gw.Name != "Integer" {
		t.Errorf("unwrapped word = %q, want Integer", gw.Name)
	}
	// The declaration span rides along, so the pair spelling still
	// anchors the "declaration of f expects N return value(s)" note.
	if p := got.Pos(); p.Row != 4 || p.Col != 9 {
		t.Errorf("position = %+v, want 4:9", p)
	}
	// It reaches ParseFnReturns as the declared type.
	types, _, perr := ParseFnReturns(r, NewList([]Value{w}))
	if perr != nil || len(types) != 1 || !types[0].Equal(TInteger) {
		t.Errorf("ParseFnReturns = %v/%v, want [Integer]", types, perr)
	}
	// A word with no colon is a plain type name, untouched.
	if got, uerr := unwrapNamedReturn(r, NewWord("Integer")); uerr != nil || !IsWord(got) {
		t.Errorf("a bare type word must pass through: %v, %v", got, uerr)
	}
	// A leading colon names nothing — idx 0 is not a name/type split.
	if got, uerr := unwrapNamedReturn(r, NewWord(":Integer")); uerr != nil || !IsWord(got) {
		t.Errorf("a leading colon must pass through: %v, %v", got, uerr)
	}
	// An invalid return NAME is rejected, as it is on the input side.
	if _, uerr := unwrapNamedReturn(r, NewWord("2bad:Integer")); uerr == nil {
		t.Error("an invalid name in the colon form must error")
	}
}

func TestEvalSigTypeExprNamedReturn(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A parenthesised annotation must be RUN before it is resolved. Left
	// raw it falls to ResolveSigType's TAny tail — a silent wildcard that
	// makes a declared union return enforce nothing. The union itself
	// needs `tor`, a lang word, so the enforced end-to-end contract is
	// pinned by lang/spec/fn-triple.tsv §7; what is checked here is that
	// the slot reduces its annotation at all, and errors rather than
	// falling through when it cannot.
	paren := NewParenExpr([]Value{NewWord("Integer")})
	sig := implicitMap(NewString("y"), paren)
	types, _, perr := ParseFnReturns(r, NewList([]Value{sig}))
	if perr != nil {
		t.Fatalf("ParseFnReturns: %v", perr)
	}
	if len(types) != 1 || !types[0].Equal(TInteger) {
		t.Errorf("types = %v, want [Integer] — the annotation must be run", types)
	}

	// A non-paren value is returned untouched.
	if got, eerr := EvalSigTypeExpr(r, NewWord("Integer"), "y"); eerr != nil || !IsWord(got) {
		t.Errorf("a plain word must pass through: %v, %v", got, eerr)
	}
	// A nil registry cannot run anything, so the value passes through.
	if got, eerr := EvalSigTypeExpr(nil, paren, "y"); eerr != nil || !IsParenExpr(got) {
		t.Errorf("no registry must pass the ParenExpr through: %v, %v", got, eerr)
	}
	// A FAILING annotation is a def-time error, not a silent wildcard.
	bad := NewParenExpr([]Value{NewWord("nosuchword")})
	if _, eerr := EvalSigTypeExpr(r, bad, "y"); eerr == nil {
		t.Error("an unrunnable annotation must error")
	}
	// So is one that produces more than a single type.
	multi := NewParenExpr([]Value{NewWord("Integer"), NewWord("String")})
	if _, eerr := EvalSigTypeExpr(r, multi, "y"); eerr == nil {
		t.Error("a multi-valued annotation must error")
	}
}

func TestResolveSigTypeAtomNames(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// An Atom carries AtomPayload, so the name has to come from AsAtom:
	// AsString answers "" and every lookup then misses, which the literal
	// fallback would silently turn into an Atom pattern.
	got, pat, rerr := ResolveSigType(r, NewAtom("Integer"))
	if rerr != nil || pat != nil || !got.Equal(TInteger) {
		t.Errorf("Integer/q = %v/%v/%v, want the Integer type", got, pat, rerr)
	}
	// An Atom that could never name a type is still the literal itself.
	got, pat, rerr = ResolveSigType(r, NewAtom("ok"))
	if rerr != nil || pat == nil || !got.Equal(TAtom) {
		t.Fatalf("ok/q = %v/%v/%v, want an Atom literal pattern", got, pat, rerr)
	}
	if a, aerr := AsAtom(*pat); aerr != nil || a != "ok" {
		t.Errorf("ok/q pattern = %v (%v)", *pat, aerr)
	}
	// And a MISSPELLED one keeps the shape of a type name, so it stays a
	// loud error rather than a constraint matching one atom.
	if _, _, rerr := ResolveSigType(r, NewAtom("Integr")); rerr == nil {
		t.Error("Integr/q must stay an unknown-type error")
	}
}
