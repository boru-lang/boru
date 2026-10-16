package core

// Stage-5 coverage: trace.go. TraceColorize / TraceVisibleLen / TraceWrap
// are pure renderers, driven directly with crafted values; TraceHandler /
// RunTrace are driven through real sub-engine runs whose step notes and
// stack widths are sized to steer every layout branch (inline note,
// next-line note, wrapped stack, clamped gaps) deterministically.

import (
	"strings"
	"testing"
)

// TestTraceColorizeArms pins every value-classification arm of the trace
// renderer, including the accessor-failure fallbacks reached via values
// whose Parent conforms but whose payload is of the wrong shape.
func TestTraceColorizeArms(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"word", NewWord("dup"), cYellow + "dup" + cReset},
		{"word-stack", NewWordModified("dup", -1, true, false), cYellow + "dup/s" + cReset},
		{"word-forward", NewWordModified("dup", -1, false, true), cYellow + "dup/f" + cReset},
		{"forward", NewForward(ForwardInfo{FuncName: "f", CollectedArgs: 1, ExpectedArgs: 2}),
			cMagenta + "→f(1/2)" + cReset},
		{"forward-spec", NewForward(ForwardInfo{FuncName: "g", CollectedArgs: 0, ExpectedArgs: 3,
			Speculative: true, SpeculativeAt: 1}),
			cMagenta + "→g(0/3 spec@1)" + cReset},
		{"open-paren", NewOpenParen(), cDim + "(" + cReset},
		{"type-literal", NewTypeLiteral(TInteger), cCyan + TInteger.String() + cReset},
		{"string", NewString("hi"), cGreen + `"hi"` + cReset},
		{"integer", NewInteger(42), cBlue + "42" + cReset},
		{"bool-true", NewBoolean(true), cCyan + "true" + cReset},
		{"bool-false", NewBoolean(false), cCyan + "false" + cReset},
		{"atom", NewAtom("ok"), cRed + "ok" + cReset},
	}
	for _, c := range cases {
		if got := TraceColorize(c.v); got != c.want {
			t.Errorf("%s: TraceColorize = %q, want %q", c.name, got, c.want)
		}
	}

	// Accessor-failure fallbacks: Parent conforms, payload is wrong.
	badStr := NewValueRaw(TString, IntPayload{N: 5})
	if got := TraceColorize(badStr); !strings.HasPrefix(got, cGreen) {
		t.Errorf("bad string payload renders %q, want green fallback", got)
	}
	badInt := NewValueRaw(TInteger, StrPayload{S: "x"})
	if got := TraceColorize(badInt); !strings.HasPrefix(got, cBlue) {
		t.Errorf("bad integer payload renders %q, want blue fallback", got)
	}
	badAtom := NewValueRaw(TAtom, IntPayload{N: 9})
	if got := TraceColorize(badAtom); !strings.Contains(got, "{9}") {
		t.Errorf("bad atom payload renders %q, want %%v of the payload", got)
	}

	// List: elements colorized recursively inside dim brackets.
	lst := NewList([]Value{NewInteger(1), NewString("a")})
	gotList := TraceColorize(lst)
	if !strings.Contains(gotList, cBlue+"1"+cReset) || !strings.Contains(gotList, cGreen+`"a"`+cReset) {
		t.Errorf("list renders %q, want colorized elements", gotList)
	}

	// Map: key:value pairs; a TMap-conforming value AsMap cannot read
	// falls back to the white String() rendering.
	om := NewOrderedMap()
	om.Set("k", NewInteger(7))
	gotMap := TraceColorize(NewMap(om))
	if !strings.Contains(gotMap, cWhite+"k"+cReset) || !strings.Contains(gotMap, cBlue+"7"+cReset) {
		t.Errorf("map renders %q, want colorized key and value", gotMap)
	}
	badMap := NewValueRaw(TMap, RecordTypeInfo{Fields: NewOrderedMap()})
	if got := TraceColorize(badMap); !strings.HasPrefix(got, cWhite) {
		t.Errorf("unreadable map renders %q, want white fallback", got)
	}

	// Default arm: a float matches none of the earlier cases.
	if got := TraceColorize(NewFloat(1.5)); !strings.HasPrefix(got, cWhite) {
		t.Errorf("float renders %q, want white default", got)
	}
}

// TestTraceVisibleLen pins the ANSI-stripping length: escape sequences
// contribute zero width and terminate at the first letter.
func TestTraceVisibleLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{cRed + "ab" + cReset, 2},
		{cBold + cCyan + "x" + cReset + "y", 2},
		{"\033[90mzz\033[0m!", 3},
	}
	for _, c := range cases {
		if got := TraceVisibleLen(c.in); got != c.want {
			t.Errorf("TraceVisibleLen(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestTraceWrapDirect pins the wrapper's framing arms: the empty-parts
// degenerate "[ ]", the tiny-maxWidth clamp, the one-line close, and the
// multi-line flush with the resolved/pending separator.
func TestTraceWrapDirect(t *testing.T) {
	// Empty parts (with the maxWidth clamp): a single "[ ]" line.
	lines := TraceWrap(nil, 0, 5)
	if len(lines) != 1 || TraceVisibleLen(lines[0]) != 3 {
		t.Fatalf("empty wrap = %q", lines)
	}

	// Everything fits on one line: opened and closed on the same line.
	lines = TraceWrap([]string{"aa", "bb"}, 0, 100)
	if len(lines) != 1 {
		t.Fatalf("fitting wrap = %q, want one line", lines)
	}
	if !strings.Contains(lines[0], "aa bb") || !strings.Contains(lines[0], "]") {
		t.Errorf("fitting wrap line = %q", lines[0])
	}

	// Forced wrapping with a mid-parts pointer: the separator lands
	// before the pointer's part, later lines are continuations, and the
	// final line carries the closing bracket.
	parts := []string{
		strings.Repeat("a", 18),
		strings.Repeat("b", 18),
		strings.Repeat("c", 18),
		strings.Repeat("d", 18),
	}
	lines = TraceWrap(parts, 2, 22)
	if len(lines) < 3 {
		t.Fatalf("wrapped lines = %q, want >= 3", lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "│") {
		t.Error("wrapped output must contain the resolved/pending separator")
	}
	if !strings.HasSuffix(joined, "]"+cReset) {
		t.Errorf("wrapped output must close with ]; got %q", lines[len(lines)-1])
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "  ") {
			t.Errorf("continuation line %q must be indented", l)
		}
	}
}
