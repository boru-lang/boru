package parser

import (
	"math"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// valuesEqual compares two core.Value instances for equality.
func valuesEqual(a, b core.Value) bool {
	if !a.Parent().Equal(b.Parent()) {
		return false
	}
	switch {
	case core.IsWord(&a):
		aw, _ := core.AsWord(&a)
		bw, _ := core.AsWord(&b)
		return aw.Name == bw.Name &&
			aw.ArgCount == bw.ArgCount &&
			aw.ForceStack == bw.ForceStack &&
			aw.ForceForward == bw.ForceForward
	case core.IsOpenParen(&a):
		return true
	case a.Parent().ConformsTo(core.TString):
		as, _ := core.AsString(a)
		bs, _ := core.AsString(b)
		return as == bs
	case a.Parent().ConformsTo(core.TInteger):
		an, _ := core.AsInteger(a)
		bn, _ := core.AsInteger(b)
		return an == bn
	default:
		return a.String() == b.String()
	}
}

func assertParse(t *testing.T, input string, want []core.Value) {
	t.Helper()
	got, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", input, err)
	}
	if len(got) != len(want) {
		t.Fatalf("Parse(%q) got %d values, want %d\n  got:  %v\n  want: %v",
			input, len(got), len(want), got, want)
	}
	for i := range got {
		if !valuesEqual(got[i], want[i]) {
			t.Errorf("Parse(%q)[%d] = %s, want %s", input, i, got[i], want[i])
		}
	}
}

func assertParseError(t *testing.T, input string) {
	t.Helper()
	_, err := Parse(input)
	if err == nil {
		t.Fatalf("Parse(%q) expected error, got nil", input)
	}
}

// TestNumberReceiverReachIsSyntaxError pins WAT-audit Exhibit AB: a
// number can never be a `.`-access (reach) receiver — `get`'s receiver
// is always a container — so a numeric literal before a `.` is a parse
// error, not the runtime "no matching signature for get".
func TestNumberReceiverReachIsSyntaxError(t *testing.T) {
	for _, src := range []string{"1.2.3", "1 . 2", "5 . foo", "5.0.foo", "1 . 2 . 3"} {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("Parse(%q): expected a syntax error, got none", src)
			continue
		}
		if !strings.Contains(err.Error(), "no members") {
			t.Errorf("Parse(%q): expected a 'no members' error, got %v", src, err)
		}
	}
	// Valid reaches (container receivers) and plain numeric literals must
	// still parse cleanly.
	for _, src := range []string{"{a:1} . a", "{a:{b:9}} . a . b", "[1 2 3] . 0", "5.0", "1.5", "5"} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", src, err)
		}
	}
}

// --- Basic literal tests ---

func TestParseEmpty(t *testing.T) {
	assertParse(t, "", nil)
}

func TestParseSingleInteger(t *testing.T) {
	assertParse(t, "1", []core.Value{core.NewInteger(1)})
}

func TestParseZero(t *testing.T) {
	assertParse(t, "0", []core.Value{core.NewInteger(0)})
}

func TestParseNegativeInteger(t *testing.T) {
	assertParse(t, "-5", []core.Value{core.NewInteger(-5)})
}

func TestParseLargeInteger(t *testing.T) {
	assertParse(t, "999", []core.Value{core.NewInteger(999)})
}

func TestParseMultipleIntegers(t *testing.T) {
	assertParse(t, "1 2 3", []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
		core.NewInteger(3),
	})
}

func TestParseQuotedStringDouble(t *testing.T) {
	assertParse(t, `"hello"`, []core.Value{core.NewString("hello")})
}

func TestParseQuotedStringSingle(t *testing.T) {
	assertParse(t, `'world'`, []core.Value{core.NewString("world")})
}

func TestParseEmptyQuotedString(t *testing.T) {
	assertParse(t, `""`, []core.Value{core.NewString("")})
}

func TestParseQuotedStringWithSpaces(t *testing.T) {
	assertParse(t, `"hello world"`, []core.Value{core.NewString("hello world")})
}

// --- Word tests ---

func TestParseSingleWord(t *testing.T) {
	assertParse(t, "upper", []core.Value{core.NewWord("upper")})
}

func TestParseUnknownWord(t *testing.T) {
	assertParse(t, "foo", []core.Value{core.NewWord("foo")})
}

func TestParseEndKeyword(t *testing.T) {
	assertParse(t, "end", []core.Value{core.NewEnd()})
}

func TestParseMultipleWords(t *testing.T) {
	assertParse(t, "a b c", []core.Value{
		core.NewWord("a"),
		core.NewWord("b"),
		core.NewWord("c"),
	})
}

// --- Mixed token tests ---

func TestParsePrefixExpression(t *testing.T) {
	// a upper → two words: the engine resolves unknown "a" to a string
	assertParse(t, "a upper", []core.Value{
		core.NewWord("a"),
		core.NewWord("upper"),
	})
}

func TestParseForwardExpression(t *testing.T) {
	// lower B → two words
	assertParse(t, "lower B", []core.Value{
		core.NewWord("lower"),
		core.NewWord("B"),
	})
}

func TestParsePrefixArithmetic(t *testing.T) {
	// 1 2 add → two integers then word
	assertParse(t, "1 2 add", []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
		core.NewWord("add"),
	})
}

func TestParseInfixArithmetic(t *testing.T) {
	// 1 add 2
	assertParse(t, "1 add 2", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseChainedArithmetic(t *testing.T) {
	assertParse(t, "1 add 2 add 3", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
		core.NewWord("add"),
		core.NewInteger(3),
	})
}

func TestParseMixedOperators(t *testing.T) {
	// 2 add 3 mul 4
	assertParse(t, "2 add 3 mul 4", []core.Value{
		core.NewInteger(2),
		core.NewWord("add"),
		core.NewInteger(3),
		core.NewWord("mul"),
		core.NewInteger(4),
	})
}

func TestParseStringThenWord(t *testing.T) {
	// "hello" upper → string then word
	assertParse(t, `"hello" upper`, []core.Value{
		core.NewString("hello"),
		core.NewWord("upper"),
	})
}

func TestParseSetWithEnd(t *testing.T) {
	// set foo 99 end
	assertParse(t, "set foo 99 end", []core.Value{
		core.NewWord("set"),
		core.NewWord("foo"),
		core.NewInteger(99),
		core.NewEnd(),
	})
}

// --- Parentheses ---

func TestParseSimpleParens(t *testing.T) {
	// (1 add 2)
	assertParse(t, "(1 add 2)", []core.Value{
		core.NewParenExpr([]core.Value{
			core.NewInteger(1),
			core.NewWord("add"),
			core.NewInteger(2),
		}),
	})
}

func TestParseNestedParens(t *testing.T) {
	// (1 add (2 mul 3))
	assertParse(t, "(1 add (2 mul 3))", []core.Value{
		core.NewParenExpr([]core.Value{
			core.NewInteger(1),
			core.NewWord("add"),
			core.NewParenExpr([]core.Value{
				core.NewInteger(2),
				core.NewWord("mul"),
				core.NewInteger(3),
			}),
		}),
	})
}

func TestParseAdjacentParens(t *testing.T) {
	// (1)(2) — no space between groups
	assertParse(t, "(1)(2)", []core.Value{
		core.NewParenExpr([]core.Value{core.NewInteger(1)}),
		core.NewParenExpr([]core.Value{core.NewInteger(2)}),
	})
}

func TestParseParenAroundWord(t *testing.T) {
	// (add)
	assertParse(t, "(add)", []core.Value{
		core.NewParenExpr([]core.Value{core.NewWord("add")}),
	})
}

// --- Word modifier tests ---

func TestParseArgCountModifier(t *testing.T) {
	// lower/1
	assertParse(t, "lower/1", []core.Value{
		core.NewWordModified("lower", 1, false, false),
	})
}

func TestParseForceForwardModifier(t *testing.T) {
	// lower/f
	assertParse(t, "lower/f", []core.Value{
		core.NewWordModified("lower", -1, false, true),
	})
}

func TestParseForceStackModifier(t *testing.T) {
	// lower/s
	assertParse(t, "lower/s", []core.Value{
		core.NewWordModified("lower", -1, true, false),
	})
}

func TestParseArgCountAndForwardModifier(t *testing.T) {
	// lower/1f
	assertParse(t, "lower/1f", []core.Value{
		core.NewWordModified("lower", 1, false, true),
	})
}

func TestParseArgCountAndStackModifier(t *testing.T) {
	// lower/1s
	assertParse(t, "lower/1s", []core.Value{
		core.NewWordModified("lower", 1, true, false),
	})
}

func TestParseArgCountZero(t *testing.T) {
	// dup/0
	assertParse(t, "dup/0", []core.Value{
		core.NewWordModified("dup", 0, false, false),
	})
}

func TestParseArgCountTwo(t *testing.T) {
	// set/2
	assertParse(t, "set/2", []core.Value{
		core.NewWordModified("set", 2, false, false),
	})
}

func TestParseModifierInExpression(t *testing.T) {
	// B lower/f → word then modified word
	assertParse(t, "B lower/f", []core.Value{
		core.NewWord("B"),
		core.NewWordModified("lower", -1, false, true),
	})
}

// --- /q quote-suffix modifier ---

func TestParseQuoteSuffix(t *testing.T) {
	// foo/q → Atom(foo), the canonical short form of (quote foo)
	assertParse(t, "foo/q", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseQuoteSuffixOnRegisteredName(t *testing.T) {
	// dup/q → Atom(dup) — /q produces an atom even when the bare name
	// would dispatch as a function. Resolution happens at parse time.
	assertParse(t, "dup/q", []core.Value{
		core.NewAtom("dup"),
	})
}

func TestParseQuoteSuffixOnReservedLikeName(t *testing.T) {
	// true/q → Atom(true) — /q applies to any word, including
	// reserved-looking names; the suffix wins over the literal path.
	assertParse(t, "true/q", []core.Value{
		core.NewAtom("true"),
	})
}

func TestParseQuoteSuffixInExpression(t *testing.T) {
	// foo/q bar/q → two atoms
	assertParse(t, "foo/q bar/q", []core.Value{
		core.NewAtom("foo"),
		core.NewAtom("bar"),
	})
}

// --- Stacked modifiers: order independence ---

func TestParseStackedQuoteAfterStack(t *testing.T) {
	// foo/sq == foo/qs == foo/q — q dominates; result is an Atom.
	assertParse(t, "foo/sq", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseStackedQuoteBeforeStack(t *testing.T) {
	assertParse(t, "foo/qs", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseStackedQuoteAfterForward(t *testing.T) {
	assertParse(t, "foo/fq", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseStackedQuoteBeforeForward(t *testing.T) {
	assertParse(t, "foo/qf", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseStackedQuoteAndDigits(t *testing.T) {
	// digits and q in any order produce an Atom — modifiers other than
	// q are accepted syntactically but ignored for atoms.
	assertParse(t, "foo/1q", []core.Value{
		core.NewAtom("foo"),
	})
	assertParse(t, "foo/q1", []core.Value{
		core.NewAtom("foo"),
	})
}

func TestParseStackedAllModifiers(t *testing.T) {
	// All three modifier flavours (digits, s, q) in arbitrary orders
	// produce the same atom — order independent including the barrier
	// number.
	want := []core.Value{core.NewAtom("foo")}
	for _, src := range []string{"foo/2qs", "foo/qs2", "foo/sq2", "foo/s2q", "foo/q2s", "foo/2sq"} {
		assertParse(t, src, want)
	}
}

func TestParseForwardSuffixBeforeDigits(t *testing.T) {
	// f and digits in either order produce the same word — /f1 == /1f.
	assertParse(t, "lower/f1", []core.Value{
		core.NewWordModified("lower", 1, false, true),
	})
}

func TestParseStackSuffixBeforeDigits(t *testing.T) {
	assertParse(t, "lower/s2", []core.Value{
		core.NewWordModified("lower", 2, true, false),
	})
}

// --- /v ref-suffix modifier ---

func TestParseRefSuffix(t *testing.T) {
	// foo/v → ref-word, short form of (valof foo). The kernel resolves
	// the binding at execution time without invoking it.
	assertParse(t, "foo/v", []core.Value{
		core.NewWordRef("foo"),
	})
}

func TestParseRefSuffixInExpression(t *testing.T) {
	// Multiple ref-words on one line round-trip independently.
	assertParse(t, "add/v mul/v", []core.Value{
		core.NewWordRef("add"),
		core.NewWordRef("mul"),
	})
}

func TestParseRefAndQuoteMutuallyExclusive(t *testing.T) {
	// /q and /v express different intents (data vs. resolved value),
	// so combining them is a LOUD parse error (NUR027 — previously the
	// whole token silently became the word "foo/qv", surfacing later
	// as an obscure undefined_word).
	assertParseError(t, "foo/qv")
	assertParseError(t, "foo/vq")
}

func TestParseRefIgnoresShapeModifiers(t *testing.T) {
	// /v short-circuits dispatch, so /s, /f, and digit modifiers are
	// accepted syntactically but have no effect on the emitted value.
	assertParse(t, "foo/vs", []core.Value{
		core.NewWordRef("foo"),
	})
	assertParse(t, "foo/vf", []core.Value{
		core.NewWordRef("foo"),
	})
	assertParse(t, "foo/2v", []core.Value{
		core.NewWordRef("foo"),
	})
}

func TestParseRefDuplicateRejected(t *testing.T) {
	assertParseError(t, "foo/vv")
}

func TestParseModifiersForwardAndStackMutuallyExclusive(t *testing.T) {
	// f and s in the same suffix are invalid — a loud parse error
	// (NUR027; the all-modifier-alphabet suffix proves a botched
	// modifier, never a name).
	assertParseError(t, "foo/fs")
}

func TestParseModifiersDuplicateRejected(t *testing.T) {
	// Duplicate modifiers are a parse error (NUR027).
	assertParseError(t, "foo/qq")
	assertParseError(t, "foo/ff")
	assertParseError(t, "foo/ss")
	// Digits may not appear in two runs separated by a letter — the
	// argCount is a single contiguous number.
	assertParseError(t, "foo/1q1")
}

func TestParseNonAlphabetSuffixStaysWord(t *testing.T) {
	// The negative boundary of the NUR027 error: a suffix containing
	// any NON-modifier character is a plain slash-bearing name, exactly
	// as before — that is what keeps `add/x`-style names and the
	// builtin type paths (Scalar/Number/Integer, capitalized segments)
	// parsing. Only the all-alphabet botched combos error.
	assertParse(t, "foo/bar", []core.Value{
		core.NewWord("foo/bar"),
	})
	assertParse(t, "foo/x", []core.Value{
		core.NewWord("foo/x"),
	})
	assertParse(t, "foo/qx", []core.Value{
		core.NewWord("foo/qx"),
	})
}

func TestParseMultiDigitArgCount(t *testing.T) {
	// Multi-digit numbers form a single contiguous argCount.
	assertParse(t, "foo/12", []core.Value{
		core.NewWordModified("foo", 12, false, false),
	})
}

// --- String vs word distinction ---

func TestParseQuotedFunctionName(t *testing.T) {
	// "upper" → string, not a word; upper → word
	assertParse(t, `"upper" upper`, []core.Value{
		core.NewString("upper"),
		core.NewWord("upper"),
	})
}

func TestParseQuotedEnd(t *testing.T) {
	// "end" → string (not the end keyword)
	assertParse(t, `"end"`, []core.Value{core.NewString("end")})
}

func TestParseQuotedNumber(t *testing.T) {
	// "1" → string, not an integer
	assertParse(t, `"1"`, []core.Value{core.NewString("1")})
}

// --- Whitespace handling ---

func TestParseExtraSpaces(t *testing.T) {
	assertParse(t, "1  2", []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
	})
}

func TestParseLeadingTrailingSpaces(t *testing.T) {
	assertParse(t, "  1  ", []core.Value{core.NewInteger(1)})
}

func TestParseTabs(t *testing.T) {
	assertParse(t, "1\t2", []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
	})
}

func TestParseWhitespaceOnly(t *testing.T) {
	assertParse(t, "   ", nil)
}

// --- Comment handling ---

func TestParseHashComment(t *testing.T) {
	// 1 # this is a comment
	assertParse(t, "1 # this is a comment", []core.Value{
		core.NewInteger(1),
	})
}

func TestParseSlashComment(t *testing.T) {
	// 1 // inline comment
	assertParse(t, "1 // inline comment", []core.Value{
		core.NewInteger(1),
	})
}

func TestParseCommentOnly(t *testing.T) {
	assertParse(t, "# just a comment", nil)
}

// --- Value keywords disabled (treated as words) ---

func TestParseTrueAsWord(t *testing.T) {
	assertParse(t, "true", []core.Value{core.NewWord("true")})
}

func TestParseFalseAsWord(t *testing.T) {
	assertParse(t, "false", []core.Value{core.NewWord("false")})
}

func TestParseNullAsWord(t *testing.T) {
	assertParse(t, "null", []core.Value{core.NewWord("null")})
}

// --- Full expression tests ---

func TestParseFullPrefixExpression(t *testing.T) {
	// "hello" upper → string then word (engine would call upper on the string)
	assertParse(t, `"hello" upper`, []core.Value{
		core.NewString("hello"),
		core.NewWord("upper"),
	})
}

func TestParseFullInfixWithParens(t *testing.T) {
	// 2 mul (3 add 4)
	assertParse(t, "2 mul (3 add 4)", []core.Value{
		core.NewInteger(2),
		core.NewWord("mul"),
		core.NewParenExpr([]core.Value{
			core.NewInteger(3),
			core.NewWord("add"),
			core.NewInteger(4),
		}),
	})
}

func TestParseForthPrimitives(t *testing.T) {
	// 1 dup swap drop
	assertParse(t, "1 dup swap drop", []core.Value{
		core.NewInteger(1),
		core.NewWord("dup"),
		core.NewWord("swap"),
		core.NewWord("drop"),
	})
}

func TestParseStorageSetGet(t *testing.T) {
	// set x 10 end get x
	assertParse(t, "set x 10 end get x", []core.Value{
		core.NewWord("set"),
		core.NewWord("x"),
		core.NewInteger(10),
		core.NewEnd(),
		core.NewWord("get"),
		core.NewWord("x"),
	})
}

func TestParseMixedLiteralsAndWords(t *testing.T) {
	// 42 "hello" foo 7
	assertParse(t, `42 "hello" foo 7`, []core.Value{
		core.NewInteger(42),
		core.NewString("hello"),
		core.NewWord("foo"),
		core.NewInteger(7),
	})
}

// --- Error cases ---

func TestParseUnterminatedString(t *testing.T) {
	assertParseError(t, `"hello`)
}

func TestParseUnterminatedSingleQuote(t *testing.T) {
	assertParseError(t, `'hello`)
}

// --- Multiline tests ---

func TestParseNewlines(t *testing.T) {
	assertParse(t, "1\nadd\n2", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseCRLF(t *testing.T) {
	assertParse(t, "1\r\nadd\r\n2", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseBlankLines(t *testing.T) {
	assertParse(t, "1\n\n\nadd\n\n2", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseMultilineWithTabs(t *testing.T) {
	assertParse(t, "\t1\n\tadd\n\t2", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseMultilineWithComments(t *testing.T) {
	src := "1 # first value\nadd # operator\n2 # second value"
	assertParse(t, src, []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
	})
}

func TestParseMultilineScript(t *testing.T) {
	src := `
		set x 10 end
		set y 20 end
		get x
		add
		get y
	`
	assertParse(t, src, []core.Value{
		core.NewWord("set"),
		core.NewWord("x"),
		core.NewInteger(10),
		core.NewEnd(),
		core.NewWord("set"),
		core.NewWord("y"),
		core.NewInteger(20),
		core.NewEnd(),
		core.NewWord("get"),
		core.NewWord("x"),
		core.NewWord("add"),
		core.NewWord("get"),
		core.NewWord("y"),
	})
}

// --- Typed list tests (list.child) ---

func TestParseTypedListString(t *testing.T) {
	// [:String] → typed list with child type string
	assertParse(t, "[:String]", []core.Value{
		core.NewTypedList(core.NewWord("String")),
	})
}

func TestParseTypedListNumber(t *testing.T) {
	// [:Number] → typed list with child type number
	assertParse(t, "[:Number]", []core.Value{
		core.NewTypedList(core.NewWord("Number")),
	})
}

func TestParseTypedListBoolean(t *testing.T) {
	assertParse(t, "[:Boolean]", []core.Value{
		core.NewTypedList(core.NewWord("Boolean")),
	})
}

func TestParseTypedListAny(t *testing.T) {
	assertParse(t, "[:Any]", []core.Value{
		core.NewTypedList(core.NewWord("Any")),
	})
}

func TestParseTypedListMap(t *testing.T) {
	// [:{x:Number}] → typed list with child type {x:Number}
	got, err := Parse("[:{x:Number}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !core.IsTypedList(&got[0]) {
		t.Fatalf("expected typed list, got %s", got[0])
	}
	ct0a, _ := core.AsChildType(got[0])
	child := ct0a.Child
	if !child.Parent().Equal(core.TMap) {
		t.Errorf("expected child type map, got %s", child.Parent())
	}
	m, _ := core.AsMap(child)
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if w, err := core.AsWord(&xVal); err != nil || w.Name != "Number" {
		t.Errorf("expected x to be word(Number), got %s (TestParseTypedListMap)", xVal)
	}
}

func TestParseTypedListNested(t *testing.T) {
	// [:[:String]] → typed list of typed lists of strings
	assertParse(t, "[:[:String]]", []core.Value{
		core.NewTypedList(core.NewTypedList(core.NewWord("String"))),
	})
}

func TestParseTypedListDeepNested(t *testing.T) {
	// [:[:[:Number]]] → three levels deep
	assertParse(t, "[:[:[:Number]]]", []core.Value{
		core.NewTypedList(core.NewTypedList(core.NewTypedList(core.NewWord("Number")))),
	})
}

func TestParseTypedListInExpression(t *testing.T) {
	// 1 [:String] → integer then typed list
	assertParse(t, "1 [:String]", []core.Value{
		core.NewInteger(1),
		core.NewTypedList(core.NewWord("String")),
	})
}

func TestParseTypedListMapChild(t *testing.T) {
	// [:{a:String,b:Number}] → typed list with multi-key map child
	got, err := Parse("[:{a:String,b:Number}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsTypedList(&got[0]) {
		t.Fatalf("expected 1 typed list, got %v", got)
	}
	ct0b, _ := core.AsChildType(got[0])
	child0b := ct0b.Child
	m, _ := core.AsMap(child0b)
	if m.Len() != 2 {
		t.Errorf("expected 2 keys, got %d", m.Len())
	}
}

// --- Typed map tests (map.child) ---

func TestParseTypedMapString(t *testing.T) {
	// {:String} → typed map with child type string
	assertParse(t, "{:String}", []core.Value{
		core.NewTypedMap(core.NewWord("String")),
	})
}

func TestParseTypedMapNumber(t *testing.T) {
	// {:Number} → typed map with child type number
	assertParse(t, "{:Number}", []core.Value{
		core.NewTypedMap(core.NewWord("Number")),
	})
}

func TestParseTypedMapBoolean(t *testing.T) {
	assertParse(t, "{:Boolean}", []core.Value{
		core.NewTypedMap(core.NewWord("Boolean")),
	})
}

func TestParseTypedMapAny(t *testing.T) {
	assertParse(t, "{:Any}", []core.Value{
		core.NewTypedMap(core.NewWord("Any")),
	})
}

func TestParseTypedMapList(t *testing.T) {
	// {:[:Number]} → typed map with child type [:Number]
	assertParse(t, "{:[:Number]}", []core.Value{
		core.NewTypedMap(core.NewTypedList(core.NewWord("Number"))),
	})
}

func TestParseTypedMapNested(t *testing.T) {
	// {:{:String}} → typed map of typed maps of strings
	assertParse(t, "{:{:String}}", []core.Value{
		core.NewTypedMap(core.NewTypedMap(core.NewWord("String"))),
	})
}

func TestParseTypedMapDeepNested(t *testing.T) {
	// {:{:{:Number}}} → three levels deep
	assertParse(t, "{:{:{:Number}}}", []core.Value{
		core.NewTypedMap(core.NewTypedMap(core.NewTypedMap(core.NewWord("Number")))),
	})
}

func TestParseTypedMapInExpression(t *testing.T) {
	// 1 {:String} → integer then typed map
	assertParse(t, "1 {:String}", []core.Value{
		core.NewInteger(1),
		core.NewTypedMap(core.NewWord("String")),
	})
}

func TestParseTypedMapConcreteChild(t *testing.T) {
	// {:{x:Number}} → typed map with map child type
	got, err := Parse("{:{x:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsTypedMap(&got[0]) {
		t.Fatalf("expected 1 typed map, got %v", got)
	}
	ct0c, _ := core.AsChildType(got[0])
	child0c := ct0c.Child
	if !child0c.Parent().Equal(core.TMap) {
		t.Errorf("expected child type map, got %s", child0c.Parent())
	}
	m, _ := core.AsMap(child0c)
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if w, err := core.AsWord(&xVal); err != nil || w.Name != "Number" {
		t.Errorf("expected x to be word(Number), got %s (TestParseTypedMapConcreteChild)", xVal)
	}
}

// --- Word list (explicit bracket) tests ---

func TestParseExplicitList(t *testing.T) {
	// [1 add 2] → single list value containing words
	got, err := Parse("[1 add 2]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent().Equal(core.TList) {
		t.Fatalf("expected list, got %s", got[0].Parent())
	}
	_lst, _ := core.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elems))
	}
}

func TestParseListWithStrings(t *testing.T) {
	// ["hello" "world"] → list of strings
	got, err := Parse(`["hello" "world"]`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := core.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Errorf("expected 2 elements, got %d", len(elems))
	}
}

// --- Data list (list inside map) tests ---

func TestParseMapWithList(t *testing.T) {
	// {x:[1,2,3]} → map with list value in data context
	got, err := Parse(`{x:[1,2,3]}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent().Equal(core.TMap) {
		t.Fatalf("expected map, got %s", got[0].Parent())
	}
	m, _ := core.AsMap(got[0])
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x'")
	}
	if !xVal.Parent().Equal(core.TList) {
		t.Errorf("expected list, got %s", xVal.Parent())
	}
	_lst, _ := core.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elems))
	}
}

func TestParseMapWithBooleans(t *testing.T) {
	// {a:true,b:false} → map with word values (word context)
	got, err := Parse(`{a:true,b:false}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	aVal, _ := m.Get("a")
	aw, _ := core.AsWord(&aVal)
	if !core.IsWord(&aVal) || aw.Name != "true" {
		t.Errorf("expected word(true), got %s", aVal)
	}
	bVal, _ := m.Get("b")
	bw, _ := core.AsWord(&bVal)
	if !core.IsWord(&bVal) || bw.Name != "false" {
		t.Errorf("expected word(false), got %s", bVal)
	}
}

func TestParseMapWithNull(t *testing.T) {
	// {x:null} → in jsonic data context, bare "null" is Text
	got, err := Parse(`{x:null}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	_, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x'")
	}
}

func TestParseMapWithTypeName(t *testing.T) {
	// {x:Number} → map with type literal in data context
	got, err := Parse(`{x:Number}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	if w, err := core.AsWord(&xVal); err != nil || w.Name != "Number" {
		t.Errorf("expected word(Number) — the parser is type-name-opaque, got %s", xVal)
	}
}

func TestParseMapWithNestedMap(t *testing.T) {
	// {a:{b:1}} → nested map in data context
	got, err := Parse(`{a:{b:1}}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	aVal, _ := m.Get("a")
	if !aVal.Parent().Equal(core.TMap) {
		t.Errorf("expected nested map, got %s", aVal.Parent())
	}
}

// --- preprocessParens tests ---

func TestParseParensInString(t *testing.T) {
	// "(hello)" → the parens are inside a string, not structural
	assertParse(t, `"(hello)"`, []core.Value{core.NewString("(hello)")})
}

func TestParseNoParens(t *testing.T) {
	// No parens — preprocessParens should be a no-op
	assertParse(t, "1 2 3", []core.Value{
		core.NewInteger(1),
		core.NewInteger(2),
		core.NewInteger(3),
	})
}

// --- Escape in string within parens ---

func TestParseEscapeInParenString(t *testing.T) {
	// Parentheses inside escaped strings in preprocessParens
	got, err := Parse(`"a\"b" 1`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

// --- Semicolon token tests ---

func TestParseSemicolonAsEnd(t *testing.T) {
	// ";" should parse as the word "end"
	assertParse(t, "1 add 2; 99", []core.Value{
		core.NewInteger(1),
		core.NewWord("add"),
		core.NewInteger(2),
		core.NewEnd(),
		core.NewInteger(99),
	})
}

func TestParseSemicolonStandalone(t *testing.T) {
	assertParse(t, ";", []core.Value{
		core.NewEnd(),
	})
}

func TestParseSemicolonAdjacentToWord(t *testing.T) {
	// "foo;bar" — semicolon is a fixed token, so it splits the text
	assertParse(t, "foo;bar", []core.Value{
		core.NewWord("foo"),
		core.NewEnd(),
		core.NewWord("bar"),
	})
}

// (expandDottedWord tests removed — dot notation is now handled by
// simple token conversion: . → get, ! . → getr)

// --- Boolean and nil as top-level values ---

func TestParseBooleanTopLevel(t *testing.T) {
	// Test bool and nil through convertTopLevelValue
	// jsonic with Lex=false means true/false are Text, but let's test
	// nil handling by parsing a map with nil value
	got, err := Parse("{x:null}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: nested map with booleans and nil ---

func TestParseDataMapWithNil(t *testing.T) {
	// {a:null,b:true,c:false,d:1}
	got, err := Parse("{a:null,b:true,c:false,d:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	// b should be word "true" (word context)
	bVal, _ := m.Get("b")
	bw2, _ := core.AsWord(&bVal)
	if !core.IsWord(&bVal) || bw2.Name != "true" {
		t.Errorf("expected word(true), got %s", bVal)
	}
	// c should be word "false" (word context)
	cVal, _ := m.Get("c")
	cw2, _ := core.AsWord(&cVal)
	if !core.IsWord(&cVal) || cw2.Name != "false" {
		t.Errorf("expected word(false), got %s", cVal)
	}
}

// --- Data context: nested structures ---

func TestParseDataMapWithNestedList(t *testing.T) {
	// {x:{y:[1,2,3]}} — nested map with data list inside
	got, err := Parse("{x:{y:[1,2,3]}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Parent().Equal(core.TMap) {
		t.Fatalf("expected nested map, got %s", xVal.Parent())
	}
	inner, _ := core.AsMap(xVal)
	yVal, _ := inner.Get("y")
	if !yVal.Parent().Equal(core.TList) {
		t.Errorf("expected list, got %s", yVal.Parent())
	}
}

// --- Map containing typed list child ---

func TestParseMapWithTypedListValue(t *testing.T) {
	// {x:[:Number]} → map key 'x' has typed list value
	got, err := Parse("{x:[:Number]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- List in data context with bool/nil ---

func TestParseDataListWithBoolAndNil(t *testing.T) {
	// {x:[true,false,null,1]} → data list with bool and nil
	got, err := Parse("{x:[true,false,null,1]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := core.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	// With word context in lists, true/false become words (resolved at runtime),
	// and null becomes a word (resolved to atom at runtime).
	ew0, _ := core.AsWord(&elems[0])
	ew1, _ := core.AsWord(&elems[1])
	ew2, _ := core.AsWord(&elems[2])
	if !core.IsWord(&elems[0]) || ew0.Name != "true" {
		t.Errorf("expected word(true), got %s", elems[0])
	}
	if !core.IsWord(&elems[1]) || ew1.Name != "false" {
		t.Errorf("expected word(false), got %s", elems[1])
	}
	if !core.IsWord(&elems[2]) || ew2.Name != "null" {
		t.Errorf("expected word(null), got %s", elems[2])
	}
}

// --- Float number tests ---

func TestParseFloatNumber(t *testing.T) {
	// 1.5 → decimal (float64 path in convertTopLevelValue and floatToValue)
	got, err := Parse("1.5")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent().ConformsTo(core.TFloat) {
		t.Errorf("expected decimal type, got %s", got[0].Parent())
	}
}

func TestParseFloatInExpression(t *testing.T) {
	// 1.5 add 2.3 → two decimals and a word
	got, err := Parse("1.5 add 2.3")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %d", len(got))
	}
	if !got[0].Parent().ConformsTo(core.TFloat) {
		t.Errorf("expected decimal, got %s", got[0].Parent())
	}
}

func TestParseMapWithFloat(t *testing.T) {
	// {x:1.5} → map with decimal in data context (float64 path in convertDataValue)
	got, err := Parse("{x:1.5}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Parent().ConformsTo(core.TFloat) {
		t.Errorf("expected decimal, got %s", xVal.Parent())
	}
}

// --- Integer literal exactness & overflow (WAT Exhibit K, Phase 0a) ---

// TestParseIntegerLiteralExact pins that decimal integer literals are
// parsed from their exact digits (strconv.ParseInt), not round-tripped
// through float64 — which silently corrupts any integer above 2^53.
func TestParseIntegerLiteralExact(t *testing.T) {
	cases := []struct {
		src  string
		want int64
	}{
		{"5", 5},
		{"-7", -7},
		{"0", 0},
		{"9007199254740992", 9007199254740992},  // 2^53
		{"9007199254740993", 9007199254740993},  // 2^53+1 — was silently → ...992
		{"9007199254740995", 9007199254740995},  // 2^53+3 — was silently → ...996
		{"9223372036854775807", math.MaxInt64},  // int64 max — was a Float
		{"-9223372036854775808", math.MinInt64}, // int64 min
		{"1_000", 1000},                         // underscore separators
		{"010", 10},                             // leading zero is decimal, not octal
		{"08", 8},                               // 8 is not a valid octal digit, but it's decimal here
	}
	for _, c := range cases {
		got, err := Parse(c.src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.src, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("Parse(%q): expected 1 value, got %d", c.src, len(got))
			continue
		}
		if !got[0].Parent().ConformsTo(core.TInteger) {
			t.Errorf("Parse(%q): expected Integer, got %s", c.src, got[0].Parent())
			continue
		}
		n, aerr := core.AsInteger(got[0])
		if aerr != nil || n != c.want {
			t.Errorf("Parse(%q): got value %d (err=%v), want %d", c.src, n, aerr, c.want)
		}
	}
}

// TestParseIntegerLiteralOverflow pins that a decimal integer literal
// outside the int64 range is a hard [boru/integer_overflow] error rather
// than a silent fall-back to Float.
func TestParseIntegerLiteralOverflow(t *testing.T) {
	for _, src := range []string{
		"9223372036854775808",     // int64 max + 1
		"-9223372036854775809",    // int64 min - 1
		"99999999999999999999999", // far out of range
	} {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("Parse(%q): expected an integer_overflow error, got nil", src)
			continue
		}
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "integer_overflow" {
			t.Errorf("Parse(%q): expected [boru/integer_overflow], got %v", src, err)
			continue
		}
		if ae.Row == 0 {
			t.Errorf("Parse(%q): overflow error carries no source position", src)
		}
	}
}

// TestParseBasePrefixedExact pins that hex/octal/binary literals are
// parsed from their exact digits — so magnitudes above 2^53 keep full
// precision instead of round-tripping through float64 — and that a
// leading sign works.
func TestParseBasePrefixedExact(t *testing.T) {
	cases := []struct {
		src  string
		want int64
	}{
		{"0x10", 16},
		{"0xFF_FF", 65535},
		{"0o17", 15},
		{"0b101", 5},
		{"-0x10", -16},
		{"0x20000000000000", 9007199254740992}, // 2^53 (exact before and after)
		{"0x20000000000001", 9007199254740993}, // 2^53+1 — was silently ...992
		{"0x7FFFFFFFFFFFFFFF", math.MaxInt64},  // int64 max via hex — was a lossy Float
		{"0o777777777777777777777", math.MaxInt64},
	}
	for _, c := range cases {
		got, err := Parse(c.src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.src, err)
			continue
		}
		if !got[0].Parent().ConformsTo(core.TInteger) {
			t.Errorf("Parse(%q): expected Integer, got %s", c.src, got[0].Parent())
			continue
		}
		if n, _ := core.AsInteger(got[0]); n != c.want {
			t.Errorf("Parse(%q): got %d, want %d", c.src, n, c.want)
		}
	}
}

// TestParsePlusPrefixInteger pins that a leading '+' on a decimal integer
// is parsed exactly and range-checked like the unsigned/negative forms.
func TestParsePlusPrefixInteger(t *testing.T) {
	got, err := Parse("+9223372036854775807")
	if err != nil {
		t.Fatalf("Parse(+maxint) error: %v", err)
	}
	if n, _ := core.AsInteger(got[0]); n != math.MaxInt64 {
		t.Errorf("+maxint: got %d, want %d", n, int64(math.MaxInt64))
	}
	if _, err := Parse("+9223372036854775808"); err == nil {
		t.Error("+(maxint+1): expected integer_overflow, got nil")
	}
}

// TestParseLeadingDotRejected pins that a receiverless / prefix `.` (or
// `!.`) — e.g. `.5`, `.name`, `!.x` — is a syntax error, not a
// receiverless get/getr. `0.5` and `$.name` remain valid.
func TestParseLeadingDotRejected(t *testing.T) {
	// Bare leading dot (a stray `.` token) and the SIGNED leading-dot
	// forms (which jsonic lexes as one number token) are all rejected.
	for _, src := range []string{".5", ".0", ".name", "!.x", "-.5", "+.5"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "syntax_error" {
			t.Errorf("Parse(%q): expected [boru/syntax_error], got %v", src, err)
		}
	}
	// These must still parse cleanly.
	for _, src := range []string{"0.5", "-0.5", "+0.5", "5.", "$.name", "$!.name"} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", src, err)
		}
	}
}

// TestParseUnderscoreSeparators pins that `_` is a single digit-separator:
// leading/trailing/repeated underscores are a syntax error.
func TestParseUnderscoreSeparators(t *testing.T) {
	for _, src := range []string{"1__0", "1_", "1_000__0", "0xFF__FF", "1__0.5"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "syntax_error" {
			t.Errorf("Parse(%q): expected [boru/syntax_error] for misplaced _, got %v", src, err)
		}
	}
	for _, src := range []string{"1_0", "1_000_000", "0xFF_FF", "1_000.5", "-1_0"} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", src, err)
		}
	}
}

// TestParseBasePrefixOverflowAndMin pins that a base-prefixed integer
// whose magnitude jsonic can't lex (>= 2^63) is handled exactly: the
// signed int64 minimum parses, and a truly out-of-range value is a clean
// integer_overflow (not undefined_word).
func TestParseBasePrefixOverflowAndMin(t *testing.T) {
	got, err := Parse("-0x8000000000000000") // int64 min
	if err != nil {
		t.Fatalf("Parse(-0x8000000000000000) error: %v", err)
	}
	if n, _ := core.AsInteger(got[0]); n != math.MinInt64 {
		t.Errorf("hex int64 min: got %d, want %d", n, int64(math.MinInt64))
	}
	for _, src := range []string{"0x8000000000000000", "0xFFFFFFFFFFFFFFFF"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "integer_overflow" {
			t.Errorf("Parse(%q): expected [boru/integer_overflow], got %v", src, err)
		}
	}
}

// TestParseDigitLedTokens pins that a digit-led token — which can never
// be a word (ValidateWordName forbids a digit first) — is diagnosed as a
// number: an out-of-range value is [boru/float_overflow]; any other
// digit-led junk (including the digit-first `2dup`, now renamed to
// `dup2`) is a malformed-numeric [boru/syntax_error].
func TestParseDigitLedTokens(t *testing.T) {
	for _, src := range []string{"1e309", "1e400", "-1e400"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "float_overflow" {
			t.Errorf("Parse(%q): expected [boru/float_overflow], got %v", src, err)
		}
	}
	for _, src := range []string{"1e", "1foo", "5x", "2dup", "0x1p4"} {
		_, err := Parse(src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != "syntax_error" {
			t.Errorf("Parse(%q): expected malformed-number [boru/syntax_error], got %v", src, err)
		}
	}
	// In-range numbers and letter-led words still parse without error.
	for _, src := range []string{"1e308", "1.5", "42", "dup2", "foo"} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected parse error %v", src, err)
		}
	}
}

// TestParseIntegerLiteralOverflowPos pins that the literal-overflow error
// is located at the offending literal, not at the start of the line.
func TestParseIntegerLiteralOverflowPos(t *testing.T) {
	_, err := Parse("add 1 9223372036854775808")
	ae, ok := err.(*core.BoruError)
	if !ok {
		t.Fatalf("expected *core.BoruError, got %v", err)
	}
	if ae.Row != 1 || ae.Col != 7 {
		t.Errorf("expected position 1:7 (the literal), got %d:%d", ae.Row, ae.Col)
	}
}

// TestParseNonDecimalLiteralsPreserved pins that scientific notation and
// base-prefixed literals keep their existing conversion (they are NOT
// routed through the new exact-decimal path, so jsonic's own base
// interpretation stands).
func TestParseNonDecimalLiteralsPreserved(t *testing.T) {
	intCases := map[string]int64{
		"1e3":   1000, // scientific, whole-valued → Integer (unchanged)
		"0x10":  16,   // hex
		"0o17":  15,   // octal
		"0b101": 5,    // binary
	}
	for src, want := range intCases {
		got, err := Parse(src)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
			continue
		}
		if !got[0].Parent().ConformsTo(core.TInteger) {
			t.Errorf("Parse(%q): expected Integer, got %s", src, got[0].Parent())
			continue
		}
		if n, _ := core.AsInteger(got[0]); n != want {
			t.Errorf("Parse(%q): got %d, want %d", src, n, want)
		}
	}
	// A decimal point still yields a Float, even whole-valued.
	for _, src := range []string{"5.0", "1.5", "1.5e2"} {
		got, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		if !got[0].Parent().ConformsTo(core.TFloat) {
			t.Errorf("Parse(%q): expected Float, got %s", src, got[0].Parent())
		}
	}
}

// TestParseIntegerLiteralExactInMap pins the same exactness in data
// context (map values go through convertDataValue, a separate switch).
func TestParseIntegerLiteralExactInMap(t *testing.T) {
	got, err := Parse("{x: 9007199254740993}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Parent().ConformsTo(core.TInteger) {
		t.Fatalf("expected Integer, got %s", xVal.Parent())
	}
	if n, _ := core.AsInteger(xVal); n != 9007199254740993 {
		t.Errorf("map value: got %d, want 9007199254740993", n)
	}
}

// TestParseSpecialFloatLiterals pins the IEEE special-value literals
// (inf / -inf / nan) — lowercase reserved literals, like true/false/none,
// that parse directly to the corresponding Float (Tier 0).
func TestParseSpecialFloatLiterals(t *testing.T) {
	check := func(src string, pred func(float64) bool) {
		got, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		if !got[0].Parent().ConformsTo(core.TFloat) {
			t.Errorf("Parse(%q): expected Float, got %s", src, got[0].Parent())
			return
		}
		f, _ := core.AsNumber(got[0])
		if !pred(f) {
			t.Errorf("Parse(%q): float value %v failed predicate", src, f)
		}
	}
	check("inf", func(f float64) bool { return math.IsInf(f, 1) })
	check("-inf", func(f float64) bool { return math.IsInf(f, -1) })
	check("nan", func(f float64) bool { return math.IsNaN(f) })
	// Same in data context (map value).
	got, err := Parse("{x: nan}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := core.AsMap(got[0])
	xv, _ := m.Get("x")
	if f, _ := core.AsNumber(xv); !math.IsNaN(f) {
		t.Errorf("map value: expected NaN, got %v", f)
	}
}

// --- Nested list/map in word context ---

func TestParseListWithMap(t *testing.T) {
	// [{x:1}] → list containing a map (convertTopLevelValue MapRef path)
	got, err := Parse("[{x:1}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := core.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if !elems[0].Parent().Equal(core.TMap) {
		t.Errorf("expected map element, got %s", elems[0].Parent())
	}
}

func TestParseNestedList(t *testing.T) {
	// [[1,2],[3,4]] in data context — data list with nested lists
	got, err := Parse("{x:[[1,2],[3,4]]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := core.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

// --- List with dotted word ---

func TestParseListWithDottedWord(t *testing.T) {
	// [foo.bar] → list with the dot chain as a single Reach node
	// (receiver foo, one segment get bar).
	got, err := Parse("[foo.bar]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := core.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element (the dot-chain Reach), got %d", len(elems))
	}
	if !core.IsReach(&elems[0]) {
		t.Fatalf("expected element to be a Reach, got %s", elems[0])
	}
	info, _ := core.AsReach(&elems[0])
	if len(info.Receiver) != 1 || len(info.Segments) != 1 {
		t.Fatalf("expected receiver[foo] + 1 segment, got %+v", info)
	}
}

// --- Receiverless reach ($ sentinel) ---

func TestParseReceiverlessReach(t *testing.T) {
	// `$.a.b` → a RECEIVERLESS reach (a detached lens): empty Receiver,
	// two get segments, and inert (Eval=false) so it evaluates to itself
	// rather than lowering to a `$ get a get b` chain.
	got, err := Parse("$.a.b")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsReach(&got[0]) {
		t.Fatalf("expected a single Reach value, got %d: %v", len(got), got)
	}
	info, _ := core.AsReach(&got[0])
	if len(info.Receiver) != 0 {
		t.Fatalf("receiverless reach must have empty Receiver, got %+v", info.Receiver)
	}
	if info.Eval {
		t.Fatalf("receiverless reach must be inert (Eval=false), got Eval=true")
	}
	if len(info.Segments) != 2 {
		t.Fatalf("expected 2 segments (a, b), got %+v", info.Segments)
	}
	// getr form: `$!.x` → one strict segment, still receiverless/inert.
	got, err = Parse("$!.x")
	if err != nil {
		t.Fatalf("Parse error ($!.x): %v", err)
	}
	info, _ = core.AsReach(&got[0])
	if len(info.Receiver) != 0 || info.Eval || len(info.Segments) != 1 || !info.Segments[0].Getr {
		t.Fatalf("$!.x: want receiverless inert getr reach, got %+v", info)
	}
}

// --- Map with single key (sortedKeys edge case) ---

func TestParseMapSingleKey(t *testing.T) {
	// {a:1} → sortedKeys with 1 key (no sorting needed)
	got, err := Parse("{a:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	if m.Len() != 1 {
		t.Errorf("expected 1 key, got %d", m.Len())
	}
}

// --- Word modifier edge cases ---

func TestParseUnrecognizedModifier(t *testing.T) {
	// foo/x → unrecognized modifier, treated as plain word "foo/x"
	assertParse(t, "foo/x", []core.Value{core.NewWord("foo/x")})
}

func TestParseSlashOnly(t *testing.T) {
	// foo/ → trailing slash with no modifier text (idx == len(name)-1),
	// so it's not matched by the modifier parsing
	assertParse(t, "foo/", []core.Value{core.NewWord("foo/")})
}

func TestParseEmptyDigitsEmptyRest(t *testing.T) {
	// This covers the case where modifier is just "/" at end of string
	// which doesn't trigger the modifier parsing (idx >= len(name)-1 check)
	assertParse(t, "x/", []core.Value{core.NewWord("x/")})
}

// --- Parens within different quote types ---

func TestParseSingleQuoteWithParens(t *testing.T) {
	// '(test)' → parens inside single-quoted string
	assertParse(t, "'(test)'", []core.Value{core.NewString("(test)")})
}

func TestParseBacktickWithParens(t *testing.T) {
	// `(test)` → parens inside backtick-quoted string
	assertParse(t, "`(test)`", []core.Value{core.NewString("(test)")})
}

// --- Data context: typed list inside data value ---

func TestParseDataMapWithTypedList(t *testing.T) {
	// {x:[:String]} → typed list in data context (ListRef.Child path in convertDataValue)
	got, err := Parse("{x:[:String]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: typed map inside data value ---

func TestParseDataMapWithTypedMap(t *testing.T) {
	// {x:{:Number}} → typed map in data context (MapRef with child$ in convertDataValue)
	got, err := Parse("{x:{:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Top-level dotted expansion ---

func TestParseDottedWordTopLevel(t *testing.T) {
	// foo.bar at top level — jsonic may parse this as a single text token
	// with a dot or as a map. Either way, it should not error.
	got, err := Parse("foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

func TestParseDottedWordInExpression(t *testing.T) {
	// 1 foo.bar → 1 <Reach foo.bar> = 2 values. The dot chain is a single
	// Reach node so it binds to foo, not to the result of `1 foo`.
	got, err := Parse("1 foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 values (1 <Reach>), got %d: %v", len(got), got)
	}
	if !core.IsReach(&got[1]) {
		t.Fatalf("expected got[1] to be the dot-chain Reach, got %s", got[1])
	}
}

// --- Dotted access in map values (§3.1) ---

func TestParseDottedMapValue(t *testing.T) {
	// {x: bf.n} must parse (previously errored with "unexpected
	// character(s): ."). The dot chain in the value position is folded
	// into a paren group, the same shape as the explicit {x: (bf.n)}.
	got, err := Parse("{x: bf.n}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !got[0].Parent().Equal(core.TMap) {
		t.Fatalf("expected a single map value, got %d: %v", len(got), got)
	}
}

func TestParseDottedMapValueMatchesParenWorkaround(t *testing.T) {
	// The bare-dot form and the explicit-paren workaround must produce
	// the same parse — that is the whole point of the fix.
	bare, err := Parse("{x: bf.n}")
	if err != nil {
		t.Fatalf("bare parse error: %v", err)
	}
	paren, err := Parse("{x: (bf.n)}")
	if err != nil {
		t.Fatalf("paren parse error: %v", err)
	}
	if bare[0].String() != paren[0].String() {
		t.Errorf("bare %q != paren %q", bare[0].String(), paren[0].String())
	}
}

func TestParseDottedMapValueMultiPair(t *testing.T) {
	// A dotted value followed by another pair must keep both pairs (an
	// earlier version of the fix dropped the second pair).
	got, err := Parse("{a: bf.n b: 9}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, mErr := core.AsMap(got[0])
	if mErr != nil || m == nil {
		t.Fatalf("expected a concrete map, got %v (%v)", got[0], mErr)
	}
	if m.Len() != 2 {
		t.Errorf("expected 2 pairs, got %d: %v", m.Len(), got[0])
	}
}

func TestParseDottedWordStillWorksInWordContext(t *testing.T) {
	// The map-value fold must NOT change word context: `1 foo.bar` is
	// still `1 <Reach foo.bar>` = 2 values (no double-wrap).
	got, err := Parse("1 foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(got), got)
	}
	if !core.IsReach(&got[1]) {
		t.Fatalf("expected got[1] to be the dot-chain Reach, got %s", got[1])
	}
}

// --- Map as top-level value ---

func TestParseTopLevelMap(t *testing.T) {
	// {a:1,b:2} as the only input → hits the MapRef branch in Parse
	got, err := Parse("{a:1,b:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent().Equal(core.TMap) {
		t.Errorf("expected map, got %s", got[0].Parent())
	}
}

// --- List pair syntax: maps inside lists ---

func TestParseListPairSyntax(t *testing.T) {
	// [x:1] → list with pair syntax creates map inside list
	// This hits map[string]any case in convertTopLevelValue
	got, err := Parse("[x:1]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

func TestParseListPairTypedMap(t *testing.T) {
	// [x:Number] → list with pair syntax, type name in value
	got, err := Parse("[x:Number]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: map with string values ---

func TestParseMapWithStringValues(t *testing.T) {
	// {x:"hello"} → quoted string in data context (Text with quote)
	got, err := Parse(`{x:"hello"}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	xValS, _ := core.AsString(xVal)
	if !xVal.Parent().ConformsTo(core.TString) || xValS != "hello" {
		t.Errorf("expected string hello, got %s", xVal)
	}
}

// --- Nested typed structures in data context ---

func TestParseDataNestedTypedMap(t *testing.T) {
	// {x:{:String},y:{:Number}} → multiple typed maps in data context
	got, err := Parse("{x:{:String},y:{:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data list with typed list child ---

func TestParseDataListWithTypedList(t *testing.T) {
	// {x:[[:String]]} → list containing a typed list in data context
	got, err := Parse("{x:[[:String]]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- convertTopLevelValue: bool path (only reachable from list-pair context) ---

func TestParseListWithBoolean(t *testing.T) {
	// [1, true, false] with booleans in list context
	got, err := Parse("[1, true, false]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Nil/null at top level ---

func TestParseNullValue(t *testing.T) {
	// null at top level → word "null" (with Lex=false)
	assertParse(t, "null", []core.Value{core.NewWord("null")})
}

// --- Data context: map with nil value via jsonic ---

func TestParseDataMapNilValue(t *testing.T) {
	// Testing data paths more thoroughly
	got, err := Parse("{a:1,b:hello,c:true,d:false,e:Number,f:Any}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	// Type names stay opaque Words in data context (ADR-012 rule 4).
	eVal, _ := m.Get("e")
	if w, err := core.AsWord(&eVal); err != nil || w.Name != "Number" {
		t.Errorf("expected word(Number), got %s", eVal)
	}
	fVal, _ := m.Get("f")
	if w, err := core.AsWord(&fVal); err != nil || w.Name != "Any" {
		t.Errorf("expected word(Any), got %s", fVal)
	}
}

// --- List inside list (word context) ---

func TestParseListInsideList(t *testing.T) {
	// [[1,2]] → list containing a nested list in word context
	got, err := Parse("[[1,2]]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Empty map ---

func TestParseEmptyMap(t *testing.T) {
	got, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Map with many keys (sortedKeys with >1 key) ---

func TestParseMapManyKeys(t *testing.T) {
	got, err := Parse("{c:3,a:1,b:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	if m.Len() != 3 {
		t.Errorf("expected 3 keys, got %d", m.Len())
	}
}

// --- Float inside data list ---

func TestParseDataListWithFloat(t *testing.T) {
	got, err := Parse("{x:[1.5,2.7]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := core.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := core.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

// --- preprocessParens: escape inside string with parens ---

func TestParseEscapeInStringWithParens(t *testing.T) {
	// Escape char inside a string when parens are also present
	// This covers the escape-in-string path in preprocessParens (lines 124-127)
	got, err := Parse(`"a\"b" (1)`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

// --- parseWord edge cases ---

func TestParseEmptyNameAfterModifier(t *testing.T) {
	// /1f → modifier on empty name, should error
	assertParseError(t, "/1f")
}

func TestParseSlashFModifier(t *testing.T) {
	// /f → empty name with forward modifier, should error
	assertParseError(t, "/f")
}

// --- convertTopLevelValue / convertDataValue unreachable cases ---
// These are defensive code paths for jsonic types not produced by
// the current configuration. We test the reachable paths thoroughly
// and accept the defensive branches as uncovered.

// --- Direct function tests for better coverage ---

func TestFloatToValueWholeNumber(t *testing.T) {
	// Whole number float → integer
	v := floatToValue(42.0)
	if !v.Parent().ConformsTo(core.TInteger) {
		t.Errorf("expected integer, got %s", v.Parent())
	}
}

func TestFloatToValueFractional(t *testing.T) {
	// Fractional float → decimal
	v := floatToValue(3.14)
	if !v.Parent().ConformsTo(core.TFloat) {
		t.Errorf("expected decimal, got %s", v.Parent())
	}
}

func eqKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOrderedKeysEmpty(t *testing.T) {
	keys := orderedKeys(map[string]any{}, nil)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestOrderedKeysSourceOrder(t *testing.T) {
	// ko covers every key → pure source order, no sorting.
	union := map[string]any{"c": 3, "a": 1, "b": 2}
	keys := orderedKeys(union, []string{"c", "a", "b"})
	if !eqKeys(keys, []string{"c", "a", "b"}) {
		t.Errorf("expected [c a b], got %v", keys)
	}
}

func TestOrderedKeysFallbackSorted(t *testing.T) {
	// Absent ko → sorted determinism fallback (pre-D1 behaviour).
	keys := orderedKeys(map[string]any{"c": 3, "a": 1, "b": 2}, nil)
	if !eqKeys(keys, []string{"a", "b", "c"}) {
		t.Errorf("expected [a b c], got %v", keys)
	}
}

func TestOrderedKeysPartialCoverage(t *testing.T) {
	// ko covers some keys (kept in ko order); the rest append sorted.
	union := map[string]any{"z": 1, "m": 2, "a": 3, "q": 4}
	keys := orderedKeys(union, []string{"z", "m"})
	if !eqKeys(keys, []string{"z", "m", "a", "q"}) {
		t.Errorf("expected [z m a q], got %v", keys)
	}
}

func TestOrderedKeysDedupAndStale(t *testing.T) {
	// A repeated ko entry keeps its first slot; a ko key absent from the
	// union (a stale order entry) is skipped.
	union := map[string]any{"a": 1, "b": 2}
	keys := orderedKeys(union, []string{"a", "gone", "a", "b"})
	if !eqKeys(keys, []string{"a", "b"}) {
		t.Errorf("expected [a b], got %v", keys)
	}
}

func TestResolveTextValueTypes(t *testing.T) {
	tests := []struct {
		input string
		check func(core.Value) bool
	}{
		{"true", func(v core.Value) bool { b, _ := core.AsBoolean(v); return v.Parent().ConformsTo(core.TBoolean) && b }},
		{"false", func(v core.Value) bool { b, _ := core.AsBoolean(v); return v.Parent().ConformsTo(core.TBoolean) && !b }},
		// Type names are NOT resolved (ADR-012 rule 4) — they are data
		// here, Atoms, like any other bare name.
		{"Number", func(v core.Value) bool {
			s, _ := core.AsAtom(v)
			return v.Parent().ConformsTo(core.TAtom) && s == "Number"
		}},
		{"String", func(v core.Value) bool {
			s, _ := core.AsAtom(v)
			return v.Parent().ConformsTo(core.TAtom) && s == "String"
		}},
		{"hello", func(v core.Value) bool {
			s, _ := core.AsAtom(v)
			return v.Parent().ConformsTo(core.TAtom) && s == "hello"
		}},
	}
	for _, tt := range tests {
		v := resolveTextValue(tt.input)
		if !tt.check(v) {
			t.Errorf("resolveTextValue(%q) = %s, unexpected", tt.input, v)
		}
	}
}

// --- Direct unexported function tests for defensive code paths ---

func TestConvertTopLevelValueBool(t *testing.T) {
	v, err := convertTopLevelValue(true, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := core.AsBoolean(v)
	if !v.Parent().ConformsTo(core.TBoolean) || !b1 {
		t.Errorf("expected true, got %s", v)
	}
	v, err = convertTopLevelValue(false, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := core.AsBoolean(v)
	if !v.Parent().ConformsTo(core.TBoolean) || b2 {
		t.Errorf("expected false, got %s", v)
	}
}

func TestConvertTopLevelValueNil(t *testing.T) {
	// A nil element is an empty list slot (`[1,,2]`, `[,1]`) — a syntax
	// error, not a fabricated `null`. (Explicit `null` arrives as the
	// jsonic.Text "null" token, not as nil.)
	_, err := convertTopLevelValue(nil, &parseDepth{})
	if err == nil {
		t.Fatal("expected an empty-element error for a nil element, got none")
	}
	if !strings.Contains(err.Error(), "empty list element") {
		t.Errorf("expected empty-element error, got %v", err)
	}
}

func TestConvertTopLevelValueUnsupported(t *testing.T) {
	_, err := convertTopLevelValue(struct{}{}, &parseDepth{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestConvertDataValueBool(t *testing.T) {
	v, err := convertDataValue(true, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	b3, _ := core.AsBoolean(v)
	if !v.Parent().ConformsTo(core.TBoolean) || !b3 {
		t.Errorf("expected true, got %s", v)
	}
}

func TestConvertDataValueNil(t *testing.T) {
	// As TestConvertTopLevelValueNil, in data (map-value) context.
	_, err := convertDataValue(nil, &parseDepth{})
	if err == nil {
		t.Fatal("expected an empty-element error for a nil element, got none")
	}
	if !strings.Contains(err.Error(), "empty list element") {
		t.Errorf("expected empty-element error, got %v", err)
	}
}

func TestConvertDataValueUnsupported(t *testing.T) {
	_, err := convertDataValue(struct{}{}, &parseDepth{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestConvertDataValueRawMap(t *testing.T) {
	// map[string]any without child$ key
	v, err := convertDataValue(map[string]any{"x": float64(1)}, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().Equal(core.TMap) {
		t.Errorf("expected map, got %s", v.Parent())
	}
}

func TestConvertDataValueRawMapWithChild(t *testing.T) {
	// map[string]any with child$ key → typed map
	v, err := convertDataValue(map[string]any{"child$": float64(1)}, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	if !core.IsTypedMap(&v) {
		t.Errorf("expected typed map, got %s", v)
	}
}

func TestConvertTopLevelValueRawMap(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"x": float64(1)}, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent().Equal(core.TMap) {
		t.Errorf("expected map, got %s", v.Parent())
	}
}

func TestConvertTopLevelValueRawMapWithChild(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"child$": float64(1)}, &parseDepth{})
	if err != nil {
		t.Fatal(err)
	}
	if !core.IsTypedMap(&v) {
		t.Errorf("expected typed map, got %s", v)
	}
}

func TestParseWordDirectCoverage(t *testing.T) {
	// Test parseWord directly for various modifiers
	tests := []struct {
		input string
		name  string
		ok    bool
	}{
		{"hello", "hello", true},
		{"foo/2s", "foo", true},
		{"foo/0", "foo", true},
		{"foo/bad", "foo/bad", true}, // unrecognized modifier
		{"foo/", "foo/", true},       // slash at end not processed
	}
	for _, tt := range tests {
		v, err := parseWord(tt.input)
		if tt.ok && err != nil {
			t.Errorf("parseWord(%q) error: %v", tt.input, err)
		} else if !tt.ok && err == nil {
			t.Errorf("parseWord(%q) expected error", tt.input)
		}
		if tt.ok {
			vw, _ := core.AsWord(&v)
			if vw.Name != tt.name {
				t.Errorf("parseWord(%q) name = %q, want %q", tt.input, vw.Name, tt.name)
			}
		}
	}

	// The decimal matcher normally classifies these before parseWord sees
	// them. Keep the defensive text fallback itself pinned as well: it must
	// preserve exact base integers and structured overflow diagnostics.
	min, err := parseWord("-0x8000000000000000")
	if err != nil {
		t.Fatalf("parseWord(int64 min): %v", err)
	}
	if n, err := core.AsInteger(min); err != nil || n != math.MinInt64 {
		t.Errorf("parseWord(int64 min) = %v (err %v), want %d", min, err, int64(math.MinInt64))
	}
	for _, tc := range []struct {
		src  string
		code string
	}{
		{"0x8000000000000000", "integer_overflow"},
		{"1e400", "float_overflow"},
	} {
		_, err := parseWord(tc.src)
		ae, ok := err.(*core.BoruError)
		if !ok || ae.Code != tc.code {
			t.Errorf("parseWord(%q) = %v, want [boru/%s]", tc.src, err, tc.code)
		}
	}
}

// =============================================================================
// MapRef.Implicit — implicit pair syntax vs explicit maps
// =============================================================================

func TestParseImplicitMapInList(t *testing.T) {
	// [x:Integer] — pair syntax inside list produces an implicit map
	vals, err := Parse("[x:Integer]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	if !list.Parent().Equal(core.TList) {
		t.Fatalf("expected list, got %s", list.Parent())
	}
	_lst, _ := core.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].Parent().Equal(core.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].Parent())
	}
	m, _ := core.AsMutableMap(elems[0])
	if !m.Implicit {
		t.Error("expected Implicit=true for pair syntax [x:Integer]")
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
}

func TestParseExplicitMapInList(t *testing.T) {
	// [{x:Integer}] — explicit map inside list is NOT implicit
	vals, err := Parse("[{x:Integer}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	_lst, _ := core.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].Parent().Equal(core.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].Parent())
	}
	m, _ := core.AsMutableMap(elems[0])
	if m.Implicit {
		t.Error("expected Implicit=false for explicit map [{x:Integer}]")
	}
}

func TestParseExplicitMapTopLevel(t *testing.T) {
	// {a:1} — explicit map at top level is NOT implicit
	vals, err := Parse("{a:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if !vals[0].Parent().Equal(core.TMap) {
		t.Fatalf("expected map, got %s", vals[0].Parent())
	}
	m, _ := core.AsMutableMap(vals[0])
	if m.Implicit {
		t.Error("expected Implicit=false for explicit map {a:1}")
	}
}

func TestParseOptionalFieldDisjunct(t *testing.T) {
	// {a?:Integer} → key "a" with value (Integer or None)
	vals, err := Parse("{a?:Integer}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, _ := core.AsMap(vals[0])
	if m == nil {
		t.Fatalf("core.AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data())
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "a" {
		t.Errorf("expected key 'a', got %v", keys)
	}
	val, _ := m.Get("a")
	if !core.IsDisjunct(&val) {
		t.Fatalf("expected disjunct for optional field, got %s", val.String())
	}
	dj, _ := core.AsDisjunct(val)
	alts := dj.Alternatives
	// `?:T` desugars to `disjunct(T, None, Absent)` — Absent is the
	// kernel type denoting "key not present", and the third
	// alternative is what makes the missing-key half of the rule
	// work via the type system rather than out-of-band metadata. The
	// alternatives are stored in canonical tcmp order (disjuncts sort at
	// construction), so assert the SET of members, not their positions.
	if len(alts) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(alts))
	}
	// The type-name alternative stays an OPAQUE Word (ADR-012 rule 4);
	// the None/Absent literals are synthesized by the `?` sugar itself.
	// The engine's resolve prepass resolves the Word and re-simplifies
	// at consumption.
	seenWord := false
	want := map[*core.Type]bool{core.TNone: false, core.TAbsent: false}
	for _, alt := range alts {
		if w, err := core.AsWord(&alt); err == nil && w.Name == "Integer" {
			seenWord = true
		}
		for ty := range want {
			if alt.Equal(ty) {
				want[ty] = true
			}
		}
	}
	if !seenWord {
		t.Errorf("expected alternative word(Integer) among %v", alts)
	}
	for ty, seen := range want {
		if !seen {
			t.Errorf("expected alternative %s among %v", ty.Leaf(), alts)
		}
	}
}

func TestParseMapShorthand(t *testing.T) {
	// Each shorthand form must parse to the identical value tree as its
	// explicit `key:value` equivalent.
	cases := []struct{ shorthand, explicit string }{
		{"{foo}", "{foo:foo}"},                     // plain shorthand
		{"{foo/v}", "{foo:foo/v}"},                 // word modifier stays on the value
		{"{foo/q}", "{foo:foo/q}"},                 // /q → atom value
		{"{foo?}", "{foo?:foo}"},                   // optional shorthand
		{"{foo/v?}", "{foo?:foo/v}"},               // optional shorthand + modifier: key is base, modifier stays on value
		{"{foo/q?}", "{foo?:foo/q}"},               // optional shorthand + /q
		{"{foo a:1 bar}", "{foo:foo a:1 bar:bar}"}, // mixed, keys sorted
		{"{a:{foo}}", "{a:{foo:foo}}"},             // nested
	}
	for _, c := range cases {
		sh, err := Parse(c.shorthand)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.shorthand, err)
		}
		ex, err := Parse(c.explicit)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.explicit, err)
		}
		if len(sh) != 1 || len(ex) != 1 {
			t.Fatalf("expected 1 value each for %q / %q, got %d / %d",
				c.shorthand, c.explicit, len(sh), len(ex))
		}
		if sh[0].String() != ex[0].String() {
			t.Errorf("shorthand %q = %s, want same as %q = %s",
				c.shorthand, sh[0].String(), c.explicit, ex[0].String())
		}
	}
}

func TestParseMapShorthandRejects(t *testing.T) {
	// Only unquoted identifiers trigger the shorthand; quoted keys and
	// non-identifier tokens remain parse errors.
	assertParseError(t, "{'foo'}")
	assertParseError(t, `{"foo"}`)
	assertParseError(t, "{123}")
}

func TestParseMapKeyModifierRejected(t *testing.T) {
	// A word modifier (/v /q /f /s /N) on a bare explicit key is illegal:
	// the modifier could only qualify the key, which is meaningless. (On a
	// shorthand entry it qualifies the value — see TestParseMapShorthand.)
	for _, src := range []string{
		"{f/v: 99}",    // /v on bare key
		"{f/v?: 99}",   // /v on bare optional key
		"{f/q: 1}",     // /q
		"{m/2: 1}",     // arg-count modifier
		"{a:1 b/v: 2}", // mixed with a clean pair
	} {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("Parse(%q) expected illegal_key error, got nil", src)
			continue
		}
		if !strings.Contains(err.Error(), "boru/illegal_key") {
			t.Errorf("Parse(%q) error = %q, want it to contain boru/illegal_key", src, err.Error())
		}
	}
}

func TestParseMapLiteralSlashKeyAllowed(t *testing.T) {
	// A slash in a key is fine when the key is an explicit literal: quoted
	// (`{'a/b': v}`) or computed (`{[a/b]: v}`). Only a BARE key with a
	// real word-modifier suffix is rejected. Each form parses to key a/b.
	for _, src := range []string{"{'a/b': 1}", "{[a/b]: 1}"} {
		vals, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		m, _ := core.AsMap(vals[0])
		if m == nil {
			t.Fatalf("Parse(%q): not a map", src)
		}
		if _, ok := m.Get("a/b"); !ok {
			t.Errorf("Parse(%q): expected key %q, got %s", src, "a/b", vals[0].String())
		}
	}
}

func TestParseOptionalFieldMixed(t *testing.T) {
	// {a:Integer, b?:String} → "a" is plain, "b" is disjunct
	vals, err := Parse("{a:Integer, b?:String}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := core.AsMap(vals[0])
	if m == nil {
		t.Fatalf("core.AsMap() returned nil")
	}
	aVal, _ := m.Get("a")
	if core.IsDisjunct(&aVal) {
		t.Errorf("expected 'a' to NOT be a disjunct, got %s", aVal.String())
	}
	bVal, _ := m.Get("b")
	if !core.IsDisjunct(&bVal) {
		t.Errorf("expected 'b' to be a disjunct, got %s", bVal.String())
	}
}

func TestParseOptionalFieldInList(t *testing.T) {
	// [x?:Integer] → implicit map with key "x" and disjunct value
	vals, err := Parse("[x?:Integer]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	_lst, _ := core.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d: %s", len(elems), list.String())
	}
	m, _ := core.AsMutableMap(elems[0])
	if m == nil {
		t.Fatalf("expected map element, got %s (data: %T)", elems[0].String(), elems[0].Data())
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
	val, _ := m.Get("x")
	if !core.IsDisjunct(&val) {
		t.Errorf("expected disjunct for x, got %s", val.String())
	}
}

func TestParseComputedKey(t *testing.T) {
	// {[x]:1} → key "x" with value 1
	vals, err := Parse("{[x]:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, _ := core.AsMap(vals[0])
	if m == nil {
		t.Fatalf("core.AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data())
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
}

func TestParseComputedKeyMultiple(t *testing.T) {
	// {[a]:1, [b]:2} → keys "a" and "b"
	vals, err := Parse("{[a]:1, [b]:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := core.AsMap(vals[0])
	if m == nil {
		t.Fatalf("core.AsMap() returned nil")
	}
	keys := m.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %v", keys)
	}
}

// --- String interpolation tests ---

func TestParseBacktickNoInterpolation(t *testing.T) {
	// Backtick string without ${} is just a plain string.
	assertParse(t, "`hello`", []core.Value{core.NewString("hello")})
}

func TestParseBacktickSimpleInterpolation(t *testing.T) {
	// `hello ${name}` produces an InterpString with 2 parts.
	got, err := Parse("`hello ${name}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !core.IsInterpString(&got[0]) {
		t.Fatalf("expected InterpString, got %s", got[0].Parent())
	}
	parts, _ := core.AsInterpString(got[0])
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	// Part 0: literal "hello "
	if parts[0].Expr != nil || parts[0].Lit != "hello " {
		t.Errorf("part 0: expected literal 'hello ', got %+v", parts[0])
	}
	// Part 1: expression [Word(name)]
	if parts[1].Expr == nil || len(parts[1].Expr) != 1 || !core.IsWord(&parts[1].Expr[0]) {
		t.Errorf("part 1: expected expression with Word(name), got %+v", parts[1])
	}
}

func TestParseBacktickMultipleInterpolations(t *testing.T) {
	// `${a} and ${b}` → [empty-lit, expr(a), " and ", expr(b), empty-lit]
	got, err := Parse("`${a} and ${b}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsInterpString(&got[0]) {
		t.Fatalf("expected 1 InterpString value, got %d values", len(got))
	}
	parts, _ := core.AsInterpString(got[0])
	// Parts: expr(a), " and ", expr(b)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
}

func TestParseBacktickExpressionInterpolation(t *testing.T) {
	// `result: ${1 add 2}` → InterpString with expression [1, Word(add), 2]
	got, err := Parse("`result: ${1 add 2}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsInterpString(&got[0]) {
		t.Fatalf("expected 1 InterpString value")
	}
	parts, _ := core.AsInterpString(got[0])
	// Parts: "result: ", expr(1 add 2), empty trailing
	if parts[0].Lit != "result: " {
		t.Errorf("expected literal 'result: ', got %q", parts[0].Lit)
	}
	if parts[1].Expr == nil || len(parts[1].Expr) != 3 {
		t.Errorf("expected expression with 3 values, got %+v", parts[1])
	}
}

func TestParseBacktickNestedBraces(t *testing.T) {
	// `${{a:1}}` → expression containing a map
	got, err := Parse("`${{a:1}}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !core.IsInterpString(&got[0]) {
		t.Fatalf("expected 1 InterpString value")
	}
}

func TestParseBacktickUnclosedInterpolation(t *testing.T) {
	// `${name` → error: unclosed interpolation
	_, err := Parse("`${name`")
	if err == nil {
		t.Fatal("expected error for unclosed interpolation")
	}
}

func TestParseBacktickOnlyLiteral(t *testing.T) {
	// Backtick string with $ but no ${ is just a plain string.
	assertParse(t, "`price: $100`", []core.Value{core.NewString("price: $100")})
}

// --- Words starting with '-' (CLI flag style) ---
//
// `-h`, `--help`, `--limit` need to parse as Words so a CLI built
// on the engine (e.g. sdkgen go-cli) can register them as native boru
// words and dispatch the same way as any other word. The grammar
// preserves the existing number-literal precedence — `-3.14`,
// `-42`, `+5` still tokenise as numbers because matchNumber only
// returns nil when the sign isn't followed by digits/`.`.

func TestParseSingleDashWord(t *testing.T) {
	assertParse(t, "-h", []core.Value{core.NewWord("-h")})
}

func TestParseDoubleDashWord(t *testing.T) {
	assertParse(t, "--help", []core.Value{core.NewWord("--help")})
}

func TestParseDashWordWithBareword(t *testing.T) {
	assertParse(t, "-h book", []core.Value{
		core.NewWord("-h"),
		core.NewWord("book"),
	})
}

func TestParseDashWordWithIntegerArg(t *testing.T) {
	assertParse(t, "--limit 10", []core.Value{
		core.NewWord("--limit"),
		core.NewInteger(10),
	})
}

func TestParseDashWordPreservesNegativeFloat(t *testing.T) {
	// `-3.14` must still tokenise as Float, not Word("-3.14").
	assertParse(t, "-3.14", []core.Value{core.NewFloat(-3.14)})
}

func TestParseDashWordPreservesNegativeInteger(t *testing.T) {
	assertParse(t, "-42", []core.Value{core.NewInteger(-42)})
}

func TestParseDashLetterDigitMix(t *testing.T) {
	// `-x5` is a Word (the leading `-` is followed by a non-digit).
	assertParse(t, "-x5", []core.Value{core.NewWord("-x5")})
}

// =====================================================================
// Source position capture (aontu-style Site)
// =====================================================================

// TestSourcePositionWordRowCol verifies the parser stamps Value.Pos with
// the 1-based row/col of each top-level word's opening token.
func TestSourcePositionWordRowCol(t *testing.T) {
	vals, err := Parse("foo\n  bar baz")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %d", len(vals))
	}
	// foo @ 1:1, bar @ 2:3, baz @ 2:7
	want := []core.SrcPos{{Row: 1, Col: 1}, {Row: 2, Col: 3}, {Row: 2, Col: 7}}
	for i, w := range want {
		if vals[i].Pos().Row != w.Row || vals[i].Pos().Col != w.Col {
			t.Errorf("value %d: expected %d:%d, got %d:%d",
				i, w.Row, w.Col, vals[i].Pos().Row, vals[i].Pos().Col)
		}
	}
}

// TestNoSitedLeak parses a spread of surface forms and asserts none leaks a
// parser-internal sited wrapper past a converter type-switch. A leak surfaces
// as an "unsupported value type parser.sited" error from a convert*Inner
// default arm (the sited wrapper is not a sealed core.Payload, so it can only
// manifest as such a parse error, never as a stored Value.Data).
func TestNoSitedLeak(t *testing.T) {
	forms := []string{
		`a b c`,
		`a.b.c`,
		`m!.k`,
		`[1 2 3]`,
		`{a:1 b:2}`,
		`f (g x)`,
		"`hello ${1 add 2} world`",
		`[x:Integer] => [x]`,
		`a ; b`,
		`{foo/v}`,
		`[:String]`,
		`{:Integer}`,
		`def f fn [[n:Integer] Integer [n add 1]]`,
	}
	for _, src := range forms {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) errored (possible sited leak): %v", src, err)
		}
	}
}

// TestParseArrowPairDive pins the lambda-arrow convenience form
// `x:Integer => body`: an implicit pair before the arrow (a paren
// pair-dive, the top-level implicit map, or a list-pair elem) closes on
// the #AR token, so the one-entry map becomes the lambda's input sig —
// equivalent to the bracketed `[x:Integer] => body`. Explicit-map pairs
// (pk = 0, set at the `{` open) keep erroring exactly as before.
func TestParseArrowPairDive(t *testing.T) {
	ok := []string{
		`(x:Integer => [x mul 2])`,
		`(p:Any => [p.value gt 3])`,
		`def double (x:Integer => [x mul 2])`,
		`def double x:Integer => [x mul 2]`,
		`filter (p:Any => [p.value gt 3]) [1 2 3 4 5]`,
		`filter p:Any => [p.value gt 3] [1 2 3 4 5]`,
		`(x:Integer => [x mul 2]) 5`,
		`x:Integer => [x mul 2]`,
	}
	for _, src := range ok {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", src, err)
		}
	}
	// The arrow must NOT close explicit-map pairs — that parse state
	// stays fatal.
	for _, src := range []string{
		`{a:1 => 2}`,
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q): expected a syntax error, got none", src)
		}
	}
}

// TestParseArrowFold pins the arrow's GROUPING: `SIG => BODY` parses as
// the single paren group `(SIG afn BODY)` — the arrow binds tighter
// than any enclosing word's forward collection, so
// `def double x:Integer => [x mul 2]` is the same program as the
// explicitly-parenthesised form (def's value operand is the group,
// which evaluates to the Function — NOT the bare sig map followed by a
// flat afn call that def's Any slot would race for and win).
func TestParseArrowFold(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int // expected top-level item count after the fold
	}{
		{`def double x:Integer => [x mul 2]`, 3}, // def double (group)
		{`def d [x:Integer] => [x mul 2]`, 3},    // def d (group)
		{`f p:Any => [p] [1 2]`, 3},              // f (group) [1 2]
		// Right-assoc currying: the chained arrow nests INSIDE the value
		// group — def mk ({x…} afn ({y…} afn [body])) — so the count
		// stays 3. (As the program's FIRST tokens a chained arrow stays
		// flat-outer — no enclosing collector exists to race — with the
		// inner arrow folded; semantics are identical.)
		{`def mk x:Integer => y:Integer => [x add y]`, 3},
	} {
		vals, err := Parse(tc.src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.src, err)
		}
		if len(vals) != tc.want {
			t.Errorf("Parse(%q): got %d top-level items, want %d (arrow did not fold)",
				tc.src, len(vals), tc.want)
		}
	}
}

// TestParseNestedEmptyParen pins the fix for a grammar bug where an empty
// paren `()` as the LAST element of an enclosing paren made the enclosing
// paren appear unclosed (`(())`, `(1 ())`, `( () )` → bogus
// "unmatched opening parenthesis" with no source position). The empty-paren
// Open alternative used to CONSUME the ")", so the Close phase then ate a
// SECOND ")", stealing the enclosing paren's closer. It now backtracks and
// lets the Close phase consume the single ")", like a non-empty paren.
func TestParseNestedEmptyParen(t *testing.T) {
	ok := []string{
		"()", "(())", "((()))", "( () )", "(() 1)", "(1 ())",
		"(()())", "((1)())", "(()(()))", "(add 1 ())", "[() 1]", "[1 ()]",
	}
	for _, src := range ok {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", src, err)
		}
	}
	// Genuinely-unclosed parens must STILL be a syntax error.
	for _, src := range []string{"(", "(1", "((1)", "(()"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q): expected a syntax error, got none", src)
		}
	}
}
