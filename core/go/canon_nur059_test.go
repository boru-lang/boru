package core

import "testing"

// NUR059: canon gains SOURCE-form renderers for kinds that previously fell
// through to Value.String's debug spelling. The `/`-modifier half is the
// worst of them — `foo/v` and `foo/2` both rendered `word(foo)`, so the
// modifier was not mis-spelled but DROPPED, and re-parsing the canon
// yielded a different program.
//
// The parser-side round-trip is pinned in parser/spec/parse.tsv, which both
// ports run; these cover the renderer arms from core's own suite.

func TestNUR059WordModifierRenders(t *testing.T) {
	cases := []struct {
		w    WordInfo
		want string
	}{
		{WordInfo{Name: "foo", ArgCount: -1}, "foo"},
		{WordInfo{Name: "foo", ArgCount: 2}, "foo/2"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceVal: true}, "foo/v"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceUsurp: true}, "foo/u"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceStack: true}, "foo/s"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceForward: true}, "foo/f"},
		// Combinations render in the canonical order — digits, f|s, u, r —
		// whatever order they were written in, so the render re-parses and
		// canon stays a usable sort key.
		{WordInfo{Name: "foo", ArgCount: -1, ForceUsurp: true, ForceVal: true}, "foo/uv"},
		{WordInfo{Name: "foo", ArgCount: 3, ForceStack: true, ForceVal: true}, "foo/3sv"},
		// f and s are exclusive; f wins the switch, which is the arm order
		// scanWordModifier's own validation makes unreachable from source.
		{WordInfo{Name: "foo", ArgCount: -1, ForceForward: true, ForceStack: true}, "foo/f"},
	}
	for _, c := range cases {
		got := CanonValue(NewValueRaw(TWord, c.w))
		// A plain word renders bare too (NUR072): `word(foo)` re-parsed as
		// the `word` splice over a group, so it was never source.
		if got != c.want {
			t.Errorf("canon %+v = %q, want %q", c.w, got, c.want)
		}
	}
}

func TestNUR059SugarAngleRenders(t *testing.T) {
	angle := NewSugar(SugarInfo{
		Kind:  SugarAngle,
		Name:  "Box",
		Items: []Value{NewWord("Integer")},
	})
	if got := CanonValue(angle); got != "Box<Integer>" {
		t.Errorf("angle sugar canon = %q, want Box<Integer>", got)
	}
	multi := NewSugar(SugarInfo{
		Kind:  SugarAngle,
		Name:  "Pair",
		Items: []Value{NewWord("Integer"), NewWord("String")},
	})
	if got := CanonValue(multi); got != "Pair<Integer String>" {
		t.Errorf("multi-arg angle canon = %q, want Pair<Integer String>", got)
	}
}

// TestNUR072SugarKindsSpellTheirSource pins the three sugar kinds NUR059
// withdrew and NUR072 spelled: the lambda marker as `=>`, a mini literal in
// the canonical `'` delimiter with the lexer's escapes, the type bound as
// `name/t`. (The parser corpus pins that each re-parses to its marker.)
func TestNUR072SugarKindsSpellTheirSource(t *testing.T) {
	for _, c := range []struct {
		info SugarInfo
		want string
	}{
		{SugarInfo{Kind: SugarLambda}, "=>"},
		{SugarInfo{Kind: SugarMini, Name: "m", Src: "src"}, "+m'src'"},
		{SugarInfo{Kind: SugarMini, Name: "re", Src: `\d+`}, `+re'\d+'`},
		{SugarInfo{Kind: SugarMini, Name: "m", Src: `it's a b\`}, `+m'it\'s\ a\ b\\'`},
		{SugarInfo{Kind: SugarMini, Name: "m", Src: "a\tb\\'"}, "+m'a\\\tb\\\\\\''"},
		{SugarInfo{Kind: SugarMini, Name: "hb-2", Src: ""}, "+hb-2''"},
		{SugarInfo{Kind: SugarTypeBound, Items: []Value{NewList([]Value{NewAtom("w")})}}, "w/t"},
	} {
		if got := CanonValue(NewSugar(c.info)); got != c.want {
			t.Errorf("canon %+v = %q, want %q", c.info, got, c.want)
		}
	}
}

// The NEGATIVE half: a marker no source can produce keeps the fallback — a
// force-arity marker (its `/N` rides on the word), a mini source holding a
// newline or a kind the lexer would not read, a bound that is not one
// quoted name.
func TestNUR072UnspellableSugarKeepsTheFallback(t *testing.T) {
	for _, info := range []SugarInfo{
		{Kind: SugarForceArity, N: 2},
		{Kind: SugarMini, Name: "m", Src: "a\nb"},
		{Kind: SugarMini, Name: "M", Src: "x"},
		{Kind: SugarMini, Name: "", Src: "x"},
		{Kind: SugarMini, Name: "m_x", Src: "x"},
		{Kind: SugarTypeBound, Items: []Value{NewList([]Value{NewAtom("w"), NewAtom("v")})}},
		{Kind: SugarTypeBound, Items: []Value{NewAtom("w")}},
		{Kind: SugarTypeBound},
	} {
		if out, handled := canonSugar(info); handled {
			t.Errorf("%+v must NOT claim a source spelling (got %q)", info, out)
		}
	}
}

func TestNUR059ParenGroupRenders(t *testing.T) {
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	if got := CanonValue(pe); got != "(1 add 2)" {
		t.Errorf("paren canon = %q, want (1 add 2)", got)
	}
}
