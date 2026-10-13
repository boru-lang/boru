package native

import (
	"bytes"
	"strings"
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// The loop drivers' trace notes and their rarer walks (loop_drivers.go,
// native_map_iter.go): every driver's Describe is what `trace` prints for
// an iteration, so each construct runs once under RunTrace and its notes
// are pinned; the empty-operand walks (inner's 2D form over an empty
// operand, eachrank over an empty sub-list at either end, foldaxis's
// empty body result) and the map forms' lambda error arms run beside them,
// each paired with the passing form it diverges from.

// traceLoop runs src under RunTrace on a registry carrying the
// boru:array-util words bare, returning the canonical result, the trace
// text and the run error.
func traceLoop(t *testing.T, src string) (string, string, error) {
	t.Helper()
	r := arrayTestReg()
	r.SetParseFunc(parser.Parse)
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	res, runErr := RunTrace(r, toks, &buf)
	return Canon(res), buf.String(), runErr
}

func TestLoopDriverTraceNotes(t *testing.T) {
	cases := []struct {
		src, want string
		notes     []string
	}{
		{`fold [add] [1 2 3] 0`, `6`, []string{"loop fold 1/3", "fold 3/3", "loop done fold"}},
		{`scan [add] [1 2 3]`, `[1 3 6]`, []string{"loop scan 2/3", "scan 3/3", "loop done scan"}},
		{`outer [mul] [1 2] [3 4]`, `[[3 4] [6 8]]`, []string{"loop outer (0,0) 1/4", "outer (1,1) 4/4"}},
		{`inner [mul] [add] [1 2] [3 4]`, `11`, []string{"loop inner pair 1/2", "inner fold 2/2"}},
		{`inner [mul] [add] [[1 2] [3 4]] [[5 6] [7 8]]`, `[[19 22] [43 50]]`,
			[]string{"loop inner pair (0,0,0)", "inner fold (1,1,1)"}},
		{`eachrank 0 [mul 2] [[1 2] [3 4]]`, `[[2 4] [6 8]]`, []string{"loop eachrank cell 1", "eachrank cell 4"}},
		{`foldaxis 0 [add] [[1 2] [3 4]]`, `[4 6]`, []string{"loop foldaxis lane 1/2 step 1", "foldaxis lane 2/2 step 1"}},
		{`filter [0 gt] [1 -2 3]`, `[1 3]`, []string{"loop filter 1/3", "filter 3/3"}},
		{`each [mul 2] {a:1 b:2}`, `{a:2 b:4}`, []string{"loop each 1/2", "each 2/2"}},
	}
	for _, c := range cases {
		got, out, err := traceLoop(t, c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got != c.want {
			t.Errorf("%q = %s, want %s", c.src, got, c.want)
		}
		for _, n := range c.notes {
			if !strings.Contains(out, n) {
				t.Errorf("%q: trace lacks %q:\n%s", c.src, n, out)
			}
		}
	}
}

// An empty operand gives inner's 2D form no cell to enter: the walk ends
// before its first step, with the empty product (left empty) or one empty
// row per left row (right empty). The 1D form over vectors is the pair.
func TestInnerEmptyOperandHasNoCell(t *testing.T) {
	for src, want := range map[string]string{
		`inner [mul] [add] [] []`:       `[]`,
		`inner [mul] [add] [[1 2]] []`:  `[[]]`,
		`inner [mul] [add] [1 2] [3 4]`: `11`,
	} {
		got, out, err := traceLoop(t, src)
		if err != nil || got != want {
			t.Errorf("%q = %s / %v, want %s", src, got, err, want)
		}
		if want != `11` && strings.Contains(out, "loop inner") {
			t.Errorf("%q: an empty operand ran an iteration:\n%s", src, out)
		}
	}
}

// An empty sub-list where eachrank expects cells completes at once: before
// the next cell (the walk climbs to the sibling) and as the last frame
// (the climb closes the root and the loop ends).
func TestEachrankEmptySubList(t *testing.T) {
	for src, want := range map[string]string{
		`eachrank 0 [mul 2] [[] [1 2]]`: `[[] [2 4]]`,
		`eachrank 0 [mul 2] [[1] []]`:   `[[2] []]`,
		`eachrank 0 [mul 2] [[1] [2]]`:  `[[2] [4]]`,
	} {
		if got, _, err := traceLoop(t, src); err != nil || got != want {
			t.Errorf("%q = %s / %v, want %s", src, got, err, want)
		}
	}
}

// A foldaxis body that leaves nothing is fold's own error, attributed to
// the lane, at the first step.
func TestFoldaxisEmptyBodyResult(t *testing.T) {
	_, _, err := traceLoop(t, `foldaxis 0 [drop drop] [[1 2] [3 4]]`)
	if err == nil || !strings.Contains(err.Error(), "foldaxis: lane 0:") ||
		!strings.Contains(err.Error(), "fold: step 0: body produced no result") {
		t.Fatalf("foldaxis over an empty body result: got %v", err)
	}
	if got, _, err := traceLoop(t, `foldaxis 0 [drop] [[1 2] [3 4]]`); err != nil || got != `[1 2]` {
		t.Errorf("foldaxis [drop] = %s / %v, want [1 2] (each lane keeps its seed)", got, err)
	}
}

// An empty quotation over a map is the identity on each value: its body
// is the EMPTY list (quotationBody), not a nil-backed one, and the Go
// lane's invocation hands the inputs straight back (quotationInvoke).
func TestEmptyMapQuotationIsIdentity(t *testing.T) {
	if got, _, err := traceLoop(t, `each [] {a:1 b:2}`); err != nil || got != `{a:1 b:2}` {
		t.Errorf("each [] over a map = %s / %v", got, err)
	}
	r := b2Reg(t)
	in := []Value{NewInteger(7)}
	out, err := quotationInvoke(r, nil)(Value{}, in)
	if err != nil || len(out) != 1 || Canon(out) != `7` {
		t.Errorf("quotationInvoke(empty) = %v / %v, want the input", out, err)
	}
	out, err = quotationInvoke(r, []Value{NewWord("dup"), NewWord("add")})(Value{}, in)
	if err != nil || Canon(out) != `14` {
		t.Errorf("quotationInvoke([dup add]) = %v / %v, want 14", out, err)
	}
}

// mapBody.fold's token arm threads (accumulator, value) through the
// quotation as list fold does — value on top.
func TestMapBodyFoldQuotation(t *testing.T) {
	r := b2Reg(t)
	mb, err := newMapBody(r, NewList([]Value{NewWord("sub")}), "fold")
	if err != nil {
		t.Fatal(err)
	}
	res, ok, err := mb.fold(r, NewInteger(10), "k", NewInteger(3), 0, 1)
	if err != nil || !ok || Canon([]Value{res}) != `7` {
		t.Errorf("fold [sub] acc 10 over 3 = %v / %v / %v, want 7", res, ok, err)
	}
}

// The map forms' lambda arms report a body error or an empty result under
// the entry's key, each beside the lambda form that runs.
func TestMapLambdaArmsReportTheKey(t *testing.T) {
	errs := map[string]string{
		`for-each ([kv:Any] => [zzz_undef_word_xyz]) {a:1}`:         `for-each: key "a"`,
		`fold ([acc:Any kv:Any] => [acc drop]) {a:1 b:2}`:           `fold: key "b": body produced no result`,
		`scan ([acc:Any kv:Any] => [zzz_undef_word_xyz]) {a:1 b:2}`: `scan: key "b"`,
	}
	for src, want := range errs {
		if _, _, err := traceLoop(t, src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want an error containing %q, got %v", src, want, err)
		}
	}
	oks := map[string]string{
		`for-each ([kv:Any] => [kv]) {a:1}`:          ``,
		`fold ([acc:Any kv:Any] => [acc]) {a:1 b:2}`: `1`,
		`scan ([acc:Any kv:Any] => [acc]) {a:1 b:2}`: `{a:1 b:1}`,
	}
	for src, want := range oks {
		if got, _, err := traceLoop(t, src); err != nil || got != want {
			t.Errorf("%q = %s / %v, want %s", src, got, err, want)
		}
	}
}
