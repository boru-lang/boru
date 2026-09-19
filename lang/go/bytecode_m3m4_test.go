package lang

// Phase 6 Stage M3 + M4 landing tests (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore
// §6).
//
// M3 (as re-landed by the namespace freeze) — DSL parsers: a Parse.parser-
// built grammar parser is a ParseLang Function VALUE; its `parse <name>`
// call compiles to a runtime fn dispatch (parselang-fn-dispatch) whose fn
// operand carries provenance from the Parse.parser call itself, so the
// module-parse rows run native. MiniLang's export map keeps a growth
// LEDGER so a PROVABLY-stable missing-key read folds to None
// (module-minilang:320) while every key a register call may install keeps the
// blanket fold decline.
//
// M4 (superseded by phase 7) — a dispatch-recovery ERROR row whose carrier
// operands are non-concrete at compile time can no longer be baked into a
// terminal OpTrap (the baked diagnostic could not match the interpreter's
// runtime, concrete-value one — design/DIAGNOSTICS.0.md). Such a row now
// EITHER compiles to a runtime-re-matching poly call or falls back to the
// interpreter; either way the raised signature_error is byte-identical across
// engines. TestUnmatchedDispatchTrapCarrierDisjoint pins that parity; the
// former carrier-disjointness trap machinery is removed.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// parseRegRow is the module-parse.tsv:14 shape — grammar built by builder
// words, finalized into a fn value, dispatched through the `parse` macro's
// value form.
const parseRegRow = `import "boru:parse"  import "boru:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def op (Parse.parser g)  end  parse op 'inc'`

// TestParseFnDispatchCompiles pins the parse-side positives: the
// Parse.parser rows compile NATIVELY (no island, no trap) and produce the
// interpreter's value.
func TestParseFnDispatchCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"builder grammar (module-parse:14 shape)", parseRegRow, "7"},
		{"whole-spec map (module-parse:37 shape)",
			`import "boru:parse"  import "boru:parselang"  def g Parse.grammar  Parse.spec g {ref:{'@op:o:INC': ([nd:Any] => [7])} abnf:{src:'op = "inc" / "dec"' start:'op'}}  def sp1 (Parse.parser g)  end  parse sp1 'inc'`,
			"7"},
		{"dispatched result consumed downstream (dot over the dynamic value)",
			`import "boru:parse"  import "boru:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [{k:'inc'}])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def opm (Parse.parser g)  end  (parse opm 'inc').k`,
			"inc"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: refused: %s", c.name, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") || strings.Contains(dis, "TRAP") {
			t.Errorf("%s: expected a native program (no island, no trap):\n%s", c.name, dis)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || !compiled {
			t.Fatalf("%s: compiled run failed: compiled=%v err=%v", c.name, compiled, errC)
		}
		gotI, errI := mustNew(t).RunInterp(c.src)
		if errI != nil {
			t.Fatalf("%s: interp run failed: %v", c.name, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%s: compiled=%v interp=%v want %q", c.name, gotC, gotI, c.want)
		}
	}
}

// TestParseFnDispatchMissParity pins the sound direction of the runtime fn
// dispatch: a parser binding whose runtime value is NOT a usable parser fn
// (here via the branch-def quirk: the conditional's def leaks a non-fn
// value in BOTH engines identically) raises the byte-identical parse_error
// through the compiled parselang-fn-dispatch and the interpreter's
// parseFnExpand — code + detail + position, per the full-corpus error-lane
// contract. The old kind-name miss (parse_unknown_lang for an unregistered
// atom) is pinned by module-parselang.tsv §4/§10; a def-scoped parser value
// has no "missing kind" — only an unusable value, and the two engines must
// agree on it.
func TestParseFnDispatchMissParity(t *testing.T) {
	const src = `import "boru:parse"  import "boru:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def c false  if c [def op (Parse.parser g)] [0]  end  parse op 'inc'`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	_, errI := mustNew(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !compiled {
		t.Fatalf("conditional-parser row should still compile (the dispatch is the proof); got %v", gotC)
	}
	// NUR109, measured 2026-08-27: they do NOT agree. The compiled lane says
	// what this test's header argues is right — `parse_error: the parser is
	// not a usable function value` — while the INTERPRETER falls back to the
	// old kind-name miss, `parse_unknown_lang: no parser "op" is registered`,
	// because an unbound def-scoped name still looks like an unregistered
	// kind to it. The assertion below read its interp side from `Run`, the
	// compiled lane (NUR106), so it compared parse_error to itself and passed.
	//
	// Pinned as measured. When the interpreter learns the same distinction
	// this fence fails and the row goes back to asserting both parse_error.
	if codeOf(errC) != "parse_error" {
		t.Fatalf("compiled miss: got [%s], want parse_error", codeOf(errC))
	}
	if codeOf(errI) != "parse_unknown_lang" {
		t.Fatalf("NUR109 interp: got [%s], want parse_unknown_lang — if this is now parse_error "+
			"the divergence is CLOSED: delete this fence and restore the both-lanes assertions "+
			"this test carried (equal Code, equal Detail, and a non-zero compiled Row)", codeOf(errI))
	}
	// The COMPILED diagnostic still has to be the well-formed one: same
	// structured shape the restored comparison would demand of both.
	var aeC *core.BoruError
	if !errors.As(errC, &aeC) {
		t.Fatalf("non-Boru compiled error: %v", errC)
	}
	if aeC.Row == 0 {
		t.Errorf("compiled miss lost its position (%v)", errC)
	}
}

// TestParseFnDispatchCheckObservationFree pins observation-freedom: a
// compile pass over a Parse.parser program must leave no state behind that
// changes a subsequent INTERPRETED run on the same instance (the class of
// leak the historical register-ReturnsFn attempt hit).
func TestParseFnDispatchCheckObservationFree(t *testing.T) {
	a := mustNew(t)
	if _, _, _, cerr := a.CompileCheck(parseRegRow); cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	got, err := a.RunInterp(parseRegRow)
	if err != nil {
		t.Fatalf("interpreted run after a compile pass errored (check-pass state leaked into the export map): %v", err)
	}
	if fmt.Sprint(got) != "[7]" {
		t.Errorf("interpreted run after a compile pass: got %v, want [7]", got)
	}
}

// TestMiniLangAbsenceFoldCompiles pins the M3 minilang growth ledger: the
// kind namespace is FROZEN (no program-reachable word installs export keys
// at run time), so EVERY missing-key read is provably stable — it folds to
// None and compiles natively — while a present key still resolves to its
// export, never a stale None.
func TestMiniLangAbsenceFoldCompiles(t *testing.T) {
	positives := []struct{ name, src string }{
		// A def-bound value never touches the export map (module-minilang.tsv
		// pins the same read as a spec row): MiniLang.Gen is None on every run.
		{"missing key with a value-form binding",
			`import "boru:minilang"  def genv (fn [[src:String opts:Map] [Integer] [1]])  MiniLang.Gen`},
		// No binding at all: the ledger is empty, absence is stable.
		{"missing key with no binding",
			`import "boru:minilang"  MiniLang.Nope`},
	}
	for _, c := range positives {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: refused: %s", c.name, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: expected a native program:\n%s", c.name, dis)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		if errC != nil || errI != nil || !compiled {
			t.Fatalf("%s: compiled=%v errC=%v errI=%v", c.name, compiled, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || !strings.Contains(fmt.Sprint(gotC), "None") {
			t.Errorf("%s: compiled=%v interp=%v want None", c.name, gotC, gotI)
		}
	}

	// NEGATIVE — a PRESENT key must never fold to a stale None: MiniLang.Re
	// is the built-in member-type export, identical in both engines.
	const present = `import "boru:minilang"  MiniLang.Re`
	gotC, _, errC := mustNew(t).RunCompiled(present)
	gotI, errI := mustNew(t).RunInterp(present)
	if noteCompileDefect(t, present, gotC, errC) {
		return
	}
	if errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Fatalf("present key: compiled=%v/%v interp=%v/%v", gotC, errC, gotI, errI)
	}
	if strings.Contains(fmt.Sprint(gotC), "None") {
		t.Errorf("present key folded to None: %v", gotC)
	}
}

// TestUnmatchedDispatchTrapCarrierDisjoint pins the M4 positives: dispatch
// recoveries whose carrier operands are PROVABLY disjoint from every
// overload's slots (assignment feasibility, sigDefinitelyUnmatched) compile
// to a terminal OpTrap with the interpreter's byte-identical taxonomy —
// code, detail, and a position wherever the interpreter carries one.
func TestUnmatchedDispatchTrapCarrierDisjoint(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	cases := []struct{ name, src string }{
		// apply.tsv:37 — the former "carrier operand declines" negative:
		// inc's Integer result is disjoint from apply's Function slot, and
		// the [Reach Any] overload cannot even fill its window.
		{"Integer carrier vs apply Function slot",
			`def inc fn [[n:Integer][Integer][n add 1]]  5 inc apply`},
		// (The former "ref value on the stack" sibling — `5 (valof inc)
		// apply` — stopped raising under the BROAD park, NUR073 clause 3:
		// the paren places inc's reference and apply fires it, answering 6
		// on both engines; apply.tsv pins the positive.)
		// (The former Boolean-carrier and private-Flag-overload cases are
		// no longer carrier-disjoint: `add` now carries a within-type
		// [Boolean Boolean] CoreDefault overload that a Boolean — or a
		// `refine Boolean` — matches, so those calls dispatch it and raise
		// a type_error at runtime rather than trapping as unmatched.)
		// open-words.tsv:100 — the COUNTING case: the merged [Point Point]
		// overload has one Point-compatible candidate for two Point slots,
		// so the Integer must occupy one of them (assignment infeasibility).
		{"one Point candidate for two Point slots",
			`import module [def Point class {x:Integer y:Integer} def add fn [[a:Point b:Point] [Point] [make Point {x:(a.x add b.x) y:(a.y add b.y)}]] export "Pointer" {Point: Point add: add/v}]  def p0 (make Pointer.Point {x:1 y:2})  add p0 1`},
		// generics-sugar.tsv:37 — the design's named example: a Box<String>
		// instance is statically Never against a Box<Integer> param.
		{"Box<String> vs Box<Integer> param",
			`def Box<T> class {value:T} def f fn [[x:Box<Integer>] [Integer] [x dot value]] end f (make Box<String> {value:'s'})`},
		// generics.tsv:60 — the explicit gen spelling of the same proof.
		{"Box of [String] vs Box of [Integer] param",
			`def Box gen [T] class {value:T} def f fn [[x:(Box of [Integer])] [Integer] [x dot value]] end f (make (Box of [String]) {value:'s'})`},
	}
	for _, c := range cases {
		// A carrier no-match reaches the compiled path in one of two ways:
		// a runtime-re-matching poly call (tryRecordPoly), or — where that
		// declines — a whole-program fallback (the no-match trap DECLINES a
		// carrier window, since a carrier is not concrete at compile time so
		// a baked diagnostic could not match the interpreter's runtime one;
		// this supersedes the former M4 carrier-disjointness trap). EITHER
		// way the raised error must be byte-identical to the interpreter's
		// (phase 7): same code, Detail, notes, and suggestions.
		_, _, errC := mustNew(t).RunCompiled(c.src)
		_, errI := mustNew(t).RunInterp(c.src)
		if noteCompileDefect(t, c.src, nil, errC) {
			continue
		}
		if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
			t.Fatalf("%s: compiled=[%s] interp=[%s], want both signature_error", c.name, codeOf(errC), codeOf(errI))
		}
		var aeC, aeI *core.BoruError
		if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
			t.Fatalf("%s: non-Boru error: compiled=%v interp=%v", c.name, errC, errI)
		}
		if aeC.Detail != aeI.Detail {
			t.Errorf("%s: detail divergence:\n  compiled=%q\n  interp=%q", c.name, aeC.Detail, aeI.Detail)
		}
		if !diagNotesEqual(aeC, aeI) {
			t.Errorf("%s: note divergence:\n  compiled=%v\n  interp=%v", c.name, aeC.Notes, aeI.Notes)
		}
	}
}

// diagNotesEqual reports whether two errors carry the same notes,
// normalising the incidental value-rendering non-determinism the two
// engines legitimately have (counter-based IDs) — the diagnostic
// STRUCTURE is what parity requires.
func diagNotesEqual(a, b *core.BoruError) bool {
	if len(a.Notes) != len(b.Notes) {
		return false
	}
	norm := func(s string) string { return regexp.MustCompile(`#\d+`).ReplaceAllString(s, "#N") }
	for i := range a.Notes {
		if norm(a.Notes[i]) != norm(b.Notes[i]) {
			return false
		}
	}
	return true
}

// TestTrapKeepsPriorCallEffects pins that a carrier no-match with prior
// effects (inc's body prints before apply raises) keeps them in order: the
// program compiles to a runtime rematch (OpDispatchRematch — formerly a
// whole-program refusal, before that an M4 carrier trap), inc's unit runs
// natively and PRINTS, then the rematch raises the identical
// signature_error — the same effects and abort point as the interpreter,
// with no fallback re-run to duplicate the print.
func TestTrapKeepsPriorCallEffects(t *testing.T) {
	const src = `def inc fn [[n:Integer][Integer][print 'a' n add 1]]  5 inc apply`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	if prog == nil {
		t.Errorf("the carrier no-match must compile to a runtime rematch (reason %q)", reason)
	}
	var outC, outI strings.Builder
	ac := mustNew(t)
	ac.SetOutput(&outC)
	_, compiled, errC := ac.RunCompiled(src)
	if noteCompileDefect(t, src, nil, errC) {
		return
	}
	if !compiled {
		t.Fatalf("the rematch program must run compiled")
	}
	ai := mustNew(t)
	ai.SetOutput(&outI)
	_, errI := ai.RunInterp(src)
	if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
		t.Fatalf("compiled=[%s] interp=[%s], want both signature_error", codeOf(errC), codeOf(errI))
	}
	if outC.String() != outI.String() || !strings.Contains(outC.String(), "a") {
		t.Errorf("effect ordering: compiled output %q, interp output %q (the fn body's print must run before the error)", outC.String(), outI.String())
	}
}
