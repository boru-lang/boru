package lang

import (
	"fmt"
	"strings"
	"testing"
)

// named_fn_candidates_test.go pins the named fn value's candidates
// (2026-09-23): NUR186 (recorded and pinned pending the same day) and the
// three records found closing it — NUR187's fn-unit and word-after halves,
// NUR188 and NUR189. The interpreter's re-step of a NAMED fn value at the
// pointer (execFnDefLiteral) matches over the values beneath it and the
// tokens after it; with a CANDIDATE — a value beneath, a word after, or a
// fn frame's tail markers after the body's last token — and no match it
// RAISES `uncalled_function` (a name always calls, ADR-011); with none it
// leaves the value as data. The compiled lane's landing stood aside as data
// wherever no zero-argument overload existed: a module fn at a factory's
// tail returned the value the caller then applied (`5 (mk) apply` was 6 for
// the raise). The landing note now carries what follows the value and
// whether values sit beneath it, the landing op raises the interpreter's
// error for a named fn over a candidate and an empty frame (a dynamic member
// read), and a CONCRETE named fn the check pass saw as data at a fn or
// lambda frame's tail — its body tape carries no markers — declines the
// unit; dispatch wreckage (a named fn the check pass matched nothing for
// inside a body) has no provenance an operand may carry. NUR188: a poly that
// collected a container member's fn value written BEFORE the word (`m.f
// typeof`, `7 m.f typeof`) handed the word the fn where the interpreter
// re-steps it first; it declines, a read written after the word (`typeof
// m.f`) stays. NUR189: the trailing arm applied a paren-PLACED member (`7
// (m.f)` was 8 for `[7 fn]`); it asks the park now. NUR187's word-after
// half: a word right after the value ends its collection (`7 m.f three` is
// `[8 3]`, islanded to `[7 4]`), and its fn-unit half: the whole-frame
// replay re-stepped `[M.inc ; 5]` across the `;`. A word after the value is
// a candidate only when the re-step's forward phase STOPS at it — a
// registered word, a binding that dispatches; a word bound to a VALUE is
// collected (`m.f k` with `def k 2` is 3, `7 m.f k` is `[7 3]`), which the
// residual arms model as they always did (the unit-suite ledger caught a
// draft that read every word as a candidate: `m.f k` raised where the
// interpreter answers, and a capturing lambda's `kv.v k` declined).

const nfK = `def k 2 def ks "s" `
const nfM = `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end `
const nfMM = nfM + `def m {f: M.inc} `
const nfT = `def three fn [[][Integer][3]] end `
const nfZ = `def zero fn [[][Integer][0]] end def m {z: zero/v} `

// TestNamedFnCandidatesAgree: every row compiles and agrees with the
// interpreter.
func TestNamedFnCandidatesAgree(t *testing.T) {
	rows := []struct{ src, want, note string }{
		{nfM + `M.inc`, "[fn inc(Integer)]", "nothing beneath, nothing after: data"},
		{nfM + `M.inc ; 5`, "[fn inc(Integer) 5]", "a boundary after: data"},
		{nfM + `(M.inc) 5`, "[fn inc(Integer) 5]", "a paren places it"},
		{nfM + `M.inc 5`, "[6]", "a value after: collected"},
		{nfM + `7 M.inc typeof`, "[Integer]", "a value beneath matches, then the word"},
		{nfM + `[1] each [drop M.inc]`, "[[fn inc(Integer)]]", "a code body's tail has no markers"},
		{nfM + `do [M.inc]`, "[fn inc(Integer)]", "a do body's tail has none either"},
		{nfM + `def mk fn [[Integer][Any][M.inc]] end (mk 1)`, "[2]", "an unnamed param beneath: the frame replay applies it"},
		{nfM + `def mk fn [[Any][Any][M.inc]] end (mk 1)`, "[2]", "over a gradual param the same"},
		{nfM + `def mk fn [[][Function][M.inc/v]] end 5 (mk) apply`, "[6]", "the /v twin is inert at the tail (as before)"},
		{nfM + `def mk fn [[][Function][(M.inc)]] end (mk)`, "[fn inc(Integer)]", "a paren places it at the tail"},
		{nfM + `def mk fn [[][List][[M.inc]]] end (mk)`, "[[fn inc(Integer)]]", "a list literal's element"},
		{nfM + `def mk fn [[][Any][5 M.inc typeof]] end (mk)`, "[Integer]", "a value beneath inside the body"},
		{`import module [def z fn [[][Integer][0]] export "Z" {z: z/v}] end def mk fn [[][Any][Z.z]] end (mk)`, "[0]", "a 0-arg export fires at the tail"},
		// The dynamic member read: the landing decides at run time.
		{nfMM + `m.f`, "[fn inc(Integer)]", "nothing beneath, nothing after: data"},
		{nfMM + `m.f ; 5`, "[fn inc(Integer) 5]", "a boundary after"},
		{nfMM + `5 m.f`, "[6]", "the trailing apply"},
		{nfMM + `1 5 m.f`, "[1 6]", "the trailing window"},
		{nfMM + `3 m.f 2`, "[3 3]", "the mixed island"},
		{nfMM + `m.f 5`, "[6]", "the lead apply"},
		{nfMM + `7 m.f 2 3`, "[7 3 3]", "the token after wins"},
		{nfMM + `typeof m.f`, "[Function]", "a read written AFTER the word is its operand (NUR188's other half)"},
		{nfMM + `(m.f) typeof`, "[Function]", "a paren-PLACED read is data the word collects (compare-restrict.tsv's `(m dot a) eq (m dot a)`)"},
		{nfMM + `7 (m.f) typeof`, "[7 Function]", "placed under a value too"},
		{nfMM + `(m.f) eq (m.f)`, "[true]", "the corpus row's shape"},
		{nfMM + `do [m.f]`, "[fn inc(Integer)]", "a do body's tail: no candidate"},
		{nfMM + `[1] each [drop m.f]`, "[[fn inc(Integer)]]", "an each body's tail: no candidate"},
		{nfMM + `7 do [m.f]`, "[8]", "the do result re-stepped over the caller's 7"},
		{nfMM + `[1 2] each [m.f]`, "[[2 3]]", "the element beneath"},
		{nfMM + `(7 (m.f))`, "[8]", "an enclosing paren re-steps the placed member"},
		// NUR189: a paren PLACES the member; the trailing arms no longer apply it.
		{nfMM + `1 7 (m.f)`, "[1 7 fn inc(Integer)]", "the trailing window (compiled [1 8] on main; the 2-entry twin `7 (m.f)`, 8 on main, declines at the residual's layout)"},
		{nfMM + `7 (m.f) 3`, "[7 fn inc(Integer) 3]", "the mixed window"},
		{nfMM + `3 (m.f) 2`, "[3 fn inc(Integer) 2]", "the mixed window, two literals"},
		{nfMM + `def g fn [[Integer][Any][m.f]] end (g 5)`, "[6]", "an unnamed param beneath inside a fn body (the frame replay)"},
		{nfMM + `def g fn [[][Any][7 m.f]] end (g)`, "[8]", "a value beneath inside a fn body"},
		{nfMM + `def g fn [[][Any][m.f 5]] end (g)`, "[6]", "a token after inside a fn body"},
		{nfMM + `def g fn [[][Any][m.f/v]] end (g)`, "[fn inc(Integer)]", "the /v read at a fn tail is data"},
		{nfZ + `def g fn [[][Any][m.z]] end (g)`, "[0]", "a 0-arg member fires at a fn tail"},
		{nfZ + `m.z ; 5`, "[0 5]", "and before a boundary"},
		{nfZ + `7 m.z typeof`, "[7 Integer]", "and under a value with a word after"},
		{nfZ + `do [m.z]`, "[0]", "and at a do body's tail"},
		{nfT + nfZ + `7 m.z three`, "[7 0 3]", "and before a word that produces a value"},
		// A word bound to a VALUE after the member: the forward phase collects
		// it (LandingNextValue), as on main before the landing note existed.
		{nfK + nfMM + `m.f k`, "[3]", "a value-bound word after: collected"},
		{nfK + nfMM + `7 m.f k`, "[7 3]", "collected over the 7 beneath"},
		{nfK + nfMM + `m.f k 9`, "[3 9]", "collected, then the literal"},
		{nfK + nfMM + `7 m.f ks`, "[8 s]", "a mismatching value-bound word: the 7 beneath matches, the word's value stays"},
		{nfK + nfMM + `def g fn [[][Any][m.f k]] end (g)`, "[3]", "collected inside a fn body"},
		{nfK + nfZ + `m.z k`, "[0 2]", "a 0-arg member fires, then the word's value"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s): %v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestNamedFnCandidatesRaiseAlike: rows that raise `uncalled_function` on
// both lanes — the landing's raise at a fn or lambda frame's tail and inside
// a do body returned to one, a value beneath that matches nothing, a word
// after the value, and a user call that collected the member.
func TestNamedFnCandidatesRaiseAlike(t *testing.T) {
	for _, src := range []string{
		nfMM + `def g fn [[][Any][m.f]] end (g)`,
		nfMM + `def g fn [[Any][Any][m.f]] end (g "s")`,
		nfMM + `def f ([] => [m.f]) (f)`,
		nfMM + `def g fn [[][Any][do [m.f]]] end (g)`,
		nfM + `def mk fn [[][Any][do [M.inc]]] end (mk)`,
		nfM + `"s" M.inc`,
		nfM + `def mk fn [[s:String][Any][s M.inc]] end (mk "x")`,
		nfM + `def mk fn [[][Any][M.inc "s"]] end (mk)`,
		nfT + nfMM + `m.f three`,
		nfMM + `def g fn [[f:Function][Integer][7]] end m.f g`,
		// A mismatching value-bound word with nothing beneath: the phase
		// collects nothing, the word's value is the candidate.
		nfK + nfMM + `m.f ks`,
		nfK + nfMM + `def g fn [[][Any][m.f ks]] end (g)`,
	} {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled {
			t.Errorf("%q: not compiled: %v", src, errC)
			continue
		}
		if codeOf(errI) != "uncalled_function" {
			t.Errorf("%q: the interpreter must raise uncalled_function, got %v err=%v", src, gotI, errI)
		}
		// The code and the residual, not the rendered report: the compiled
		// lane stamps the landing op's token where the interpreter borrows a
		// candidate's, and a body-local raise carries no position at all.
		if codeOf(errC) != codeOf(errI) || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled %v err=[%s], interp %v err=[%s]", src, gotC, codeOf(errC), gotI, codeOf(errI))
		}
	}
}

// TestNamedFnCandidatesWalk pins the landing's overload WALK (NUR190's
// closed half, 2026-09-23): a DYNAMIC fn value under a FUNCTION word is
// re-stepped by the interpreter over that word, so the landing runs the
// interpreter's own plan over the value and the word — a mixed overload's
// zero-argument fallback fires (`m.f z` is `[42 0]`; the wordless landing
// stood aside and the residual apply answered 1), an anonymous fn parks as
// data for the rest of the statement (`m.l z` is `[fn lam(Integer) 0]`, was
// 1), a named fn with no overload taking the word raises, and an Any-typed
// slot's speculative claim raises the strict barrier's stranded-forward
// error on both lanes.
func TestNamedFnCandidatesWalk(t *testing.T) {
	const nfW = `def h fn [[] [Integer] [42]] end def h fn [[n:Integer] [Integer] [n add 1]] end ` +
		`def a fn [[x:Any] [Any] [x]] end def lam ([n:Integer] => [n add 1]) ` +
		`def mk fn [[] [Map] [{f: h/v a: a/v l: lam/v}]] end def m (mk) end ` +
		`def z fn [[] [Integer] [0]] end def k 5 `
	rows := []struct{ src, want, note string }{
		{nfW + `m.f z`, "[42 0]", "the zero-argument fallback fires under a user fn word (was 1)"},
		{nfW + `m.f typeof`, "[Integer]", "and under a native word (was Function)"},
		{nfW + `m.f z 9`, "[42 0 9]", "the rest of the statement follows"},
		{nfW + `m.f k`, "[6]", "a value-bound word is collected by the unary (the Value note)"},
		{nfW + `5 m.f`, "[6]", "a value beneath selects the unary (as before)"},
		{nfW + `m.l z`, "[fn lam(Integer) 0]", "an anonymous fn parks at the word (was 1)"},
		{nfW + `m.l z 5`, "[fn lam(Integer) 0 5]", "and stays data under the later entries"},
		{nfW + `[1] each [drop m.f z]`, "[[0]]", "inside a code body"},
		// The generated sweep's do-catch cell: a factory's 0-arg LAMBDA value
		// under the `error` word parks (ADR-016), where the walk's fallback
		// fired it in a draft.
		{`do [def mk fn [[][Function][([] => [1])]] end if true (mk) [2]] error [dot code]`, "[fn]", "an anonymous 0-arg value under a word parks"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s): %v", c.src, c.note, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
	// A speculative claim (an Any-typed slot) strands at the word on both
	// lanes: the same code, the same detail.
	src := nfW + `m.a z`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Fatalf("%q: not compiled: %v", src, errC)
	}
	if codeOf(errI) != "signature_error" || codeOf(errC) != codeOf(errI) || detailOf(errC) != detailOf(errI) || len(gotC) != 0 || len(gotI) != 0 {
		t.Errorf("%q: compiled %v [%s] %s, interp %v [%s] %s", src, gotC, codeOf(errC), detailOf(errC), gotI, codeOf(errI), detailOf(errI))
	}
}

// TestNamedFnCandidatesOpenShapes pins, as MEASURED, the neighbour this
// increment leaves open (NUR190, recorded and pinned pending 2026-09-23): a
// DYNAMIC fn value under a FUNCTION word whose arg-taking overload can claim
// the word. The interpreter's `/q` slot captures the word (`y` never runs);
// the landing stands aside for a mixed overload and the lead arm applies the
// fn over the word's result. fn-value.tsv's `m.f z` passes by coincidence
// (z's result is its own atom); the mixed twin declines soundly since the
// islands read the word as a crossing (NUR187).
func TestNamedFnCandidatesOpenShapes(t *testing.T) {
	const nfQ = `def z fn [[] [Atom] [(quote z)]] end def y fn [[] [Integer] [42]] end ` +
		`def h fn [[] [Integer] [42]] end def h fn [[x:Atom/q] [Atom] [x]] end ` +
		`def mk fn [[] [Map] [{f: h/v}]] end def m (mk) end `
	// The landing's overload walk (the same day) settles the typed slot, the
	// Any-typed claim and the anonymous park (TestNamedFnCandidatesWalk); the
	// `/q` capture still stands aside (the residual apply answers by the
	// word's result: `m.q z` is `[42 0]` for `[z]`), and a Function-typed
	// slot's reference BAILS loudly where the wordless landing raised a
	// false uncalled_function (`m.g z` is 7 interpreted) — the word's call
	// is compiled after the landing and cannot be skipped.
	const nfR = `def g fn [[f:Function] [Integer] [7]] end def q fn [[] [Integer] [42]] end def q fn [[x:Atom/q] [Atom] [x]] end ` +
		`def mk fn [[] [Map] [{g: g/v q: q/v}]] end def m (mk) end def z fn [[] [Integer] [0]] end `
	rows := []struct {
		src, interp, compiled, reason string
		bail                          bool
	}{
		{nfQ + `m.f y`, "[y]", "[42 42]", "", false},
		{nfQ + `m.f z`, "[z]", "[z]", "", false},
		{nfQ + `7 m.f y`, "[7 y]", "", "dynamic value precedes residual args", false},
		{nfR + `m.q z`, "[z]", "[42 0]", "", false},
		{nfR + `m.g z`, "[7]", "", "takes the word `z` as its argument", true},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
		if c.reason != "" {
			if !strings.Contains(fmt.Sprint(errC), c.reason) || len(gotC) != 0 {
				t.Errorf("%q: want a sound decline or bail %q, got compiled=%v %v err=%v", c.src, c.reason, compiled, gotC, errC)
			}
			// A BAIL — the program compiled and the runtime abandoned it — is
			// the ledger's worse half and is booked (compile_defect_test.go).
			if c.bail {
				requireCompileDefect(t, c.src, gotC, errC)
			}
			continue
		}
		if !compiled || errC != nil || fmt.Sprint(gotC) != c.compiled {
			t.Errorf("%q: compiled (measured, open): want %s, got %v err=%v", c.src, c.compiled, gotC, errC)
		}
	}
}

// TestNamedFnCandidatesSoundCompileFailures: what is left DECLINES, each row
// with the interpreter's own answer beside it — a concrete named fn at a fn
// or lambda frame's tail (the unit has no trap for the raise yet), dispatch
// wreckage reaching a word inside a body, a member read a word collected
// (NUR188), a placed member the trailing arm no longer applies (NUR189), a
// value's collection ended by a word or a boundary (NUR187).
func TestNamedFnCandidatesSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp, interpErr string }{
		{nfM + `def mk fn [[][Function][M.inc]] end (mk)`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def mk fn [[][Function][M.inc]] end 5 (mk) apply`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def mk fn [[][Function][M.inc]] end 7 5 (mk) apply`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def mk fn [[][Any][def x 1 M.inc]] end (mk)`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def mk fn [[a:Integer][Any][M.inc]] end (mk 1)`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def f ([] => [M.inc]) (f)`, "takes no zero-argument call", "", "uncalled_function"},
		{nfM + `def mk fn [[][Any][M.inc typeof]] end (mk)`, "reaches typeof", "", "uncalled_function"},
		{nfM + `M.inc typeof`, "check diagnostics", "", "uncalled_function"},
		{nfM + `def mk fn [[][Any][M.inc ; 5]] end (mk)`, "unapplied fn-value in body residual", "", "type_error"},
		{nfMM + `def g fn [[][Any][m.f ; 5]] end (g)`, "unapplied fn-value in body residual", "", "type_error"},
		{nfMM + `m.f typeof`, "(NUR188)", "", "uncalled_function"},
		{nfMM + `7 m.f typeof`, "(NUR188)", "[Integer]", ""},
		{nfMM + `"s" m.f typeof`, "(NUR188)", "", "uncalled_function"},
		{nfMM + `m.f print`, "(NUR188)", "", "uncalled_function"},
		{nfT + nfMM + `7 m.f three`, "dynamic value precedes residual args", "[8 3]", ""},
		{nfT + nfMM + `"s" m.f three`, "dynamic value precedes residual args", "", "uncalled_function"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			if c.interpErr == "" || !strings.Contains(cerr.Error(), c.reason) {
				t.Errorf("%q: check: %v", c.src, cerr)
			}
		} else {
			if prog != nil {
				t.Errorf("%q: compiled — expected a compile failure", c.src)
				continue
			}
			if !strings.Contains(reason, c.reason) {
				t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
			}
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if c.interpErr != "" {
			if codeOf(errI) != c.interpErr {
				t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interpErr)
			}
		} else if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}
