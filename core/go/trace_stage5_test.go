package core

// Stage-5 coverage: trace.go. TraceColorize / TraceVisibleLen / TraceWrap
// are pure renderers, driven directly with crafted values; TraceHandler /
// RunTrace are driven through real sub-engine runs whose step notes and
// stack widths are sized to steer every layout branch (inline note,
// next-line note, wrapped stack, clamped gaps) deterministically.

import (
	"bytes"
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

// TestTraceHandlerArms pins the exported handler: a type-literal argument
// is declined; a concrete list runs under the sub-engine and renders the
// trace onto Registry.Output.
func TestTraceHandlerArms(t *testing.T) {
	r := stage5Registry(t)
	var buf bytes.Buffer
	r.Output = &buf

	if _, err := TraceHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r); err == nil {
		t.Fatal("a type-literal argument must be declined")
	} else if !strings.Contains(err.Error(), "concrete") {
		t.Errorf("compile failure message = %q", err)
	}

	out, err := TraceHandler([]Value{NewList([]Value{NewInteger(1), NewInteger(2)})}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Errorf("trace result = %v, want the two integers", out)
	}
	if !strings.Contains(buf.String(), "trace") {
		t.Error("the trace header must reach Registry.Output")
	}
}

// TestRunTraceShortSteps pins the narrow-layout arms: pointer at the
// start (pending-only), pointer mid-stack (resolved │ pending), short
// notes right-aligned on the same line, and the final result banner.
func TestRunTraceShortSteps(t *testing.T) {
	r := stage5Registry(t)
	var buf bytes.Buffer
	tokens := []Value{
		NewMark("m", NewInteger(7)),
		NewMove("m", "loop"),
	}
	res, err := RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("result = %v, want the replayed body [7]", res)
	}
	got := buf.String()
	if !strings.Contains(got, "trace") || !strings.Contains(got, "result") {
		t.Errorf("output must carry header and result banner; got %q", got)
	}
	// The mid-stack pointer renders the resolved/pending separator, and
	// the mark/move notes ride the same line as their short stacks.
	if !strings.Contains(got, "│") {
		t.Error("a mid-stack pointer must render the separator")
	}
	if !strings.Contains(got, "mark m") || !strings.Contains(got, "move→mark m") {
		t.Errorf("step notes must render; got %q", got)
	}
}

// TestRunTraceError pins the failing-run tail: the fault note reaches the
// step list and the error banner is printed before the error surfaces.
func TestRunTraceError(t *testing.T) {
	r := stage5Registry(t)
	var buf bytes.Buffer
	res, err := RunTrace(r, []Value{NewWord("zz-stage5-missing")}, &buf)
	if err == nil {
		t.Fatal("the unknown word must fail the run")
	}
	if res != nil {
		t.Errorf("a failed run returns nil result, got %v", res)
	}
	got := buf.String()
	if !strings.Contains(got, "error") || !strings.Contains(got, "zz-stage5-missing") {
		t.Errorf("the error banner must render; got %q", got)
	}
	if !strings.Contains(got, "fault:") {
		t.Errorf("the fault note must reach the trace; got %q", got)
	}
}

// runTraceMarkMove drives one mark/move replay with the given id and
// returns the rendered output. The id length steers the note and stack
// widths through RunTrace's layout branches.
func runTraceMarkMove(t *testing.T, r *Registry, id string) string {
	t.Helper()
	var buf bytes.Buffer
	tokens := []Value{
		NewMark(id, NewInteger(1)),
		NewMove(id, "r"),
	}
	res, err := RunTrace(r, tokens, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("result = %v, want [1]", res)
	}
	return buf.String()
}

// TestRunTraceLayoutBranches pins the width-sensitive layout arms with
// three mark ids sized around the 120-column budget:
//   - 60 chars:  after the replay the stack is tiny and the note fits
//     inline (right-aligned on the stack's line);
//   - 100 chars: the note misses the inline budget but fits the next
//     line without clamping;
//   - 130 chars: the pre-replay stack wraps across lines (with the
//     separator and a clamped note gap), and the post-replay note is
//     clamped to the minimum gap on its own line.
func TestRunTraceLayoutBranches(t *testing.T) {
	r := stage5Registry(t)

	inline := runTraceMarkMove(t, r, strings.Repeat("c", 60))
	if !strings.Contains(inline, "move→mark "+strings.Repeat("c", 60)) {
		t.Errorf("60-char id note missing:\n%s", inline)
	}

	nextLine := runTraceMarkMove(t, r, strings.Repeat("b", 100))
	if !strings.Contains(nextLine, "move→mark "+strings.Repeat("b", 100)) {
		t.Errorf("100-char id note missing:\n%s", nextLine)
	}

	wrapped := runTraceMarkMove(t, r, strings.Repeat("a", 130))
	if !strings.Contains(wrapped, "mark "+strings.Repeat("a", 130)) {
		t.Errorf("130-char id note missing:\n%s", wrapped)
	}
	// The 130-char stack cannot fit one line: the mark/move display must
	// have wrapped onto continuation lines that still close with ].
	sawContinuation := false
	for _, line := range strings.Split(wrapped, "\n") {
		if strings.HasPrefix(line, cGray+"   "+cReset) {
			sawContinuation = true
		}
	}
	if !sawContinuation {
		t.Errorf("the long stack must wrap onto continuation lines:\n%s", wrapped)
	}
}
