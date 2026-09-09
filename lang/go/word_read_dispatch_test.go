package lang

// NUR123 (measured 2026-09-05). A bare read of a frame binding holding a fn
// is a WORD dispatch on the interpreter — stepWord routes a bound FnDefInfo
// through Registry.Lookup under the binding name: a 0-arg fn fires, an n-arg
// fn collects from the tokens after it and the frame's stack below it, a
// no-match raises `cannot call `g`` — while the check model bound a CARRIER
// the emitter lowered as a slot push: `def f fn [[g:Function][Any][g]]  f
// ([] => [42])` answered `fn` for the interpreter's 42, and the identity fn
// over a 0-arg lambda the same, both on the DEFAULT lane with exit 0. The
// engine now notes each such read (NoteWordRead), the unit's finish arms the
// whole-frame replay over the residual with the binding names, and the VM
// re-steps a fn-valued read as the WORD through the interpreter's own
// dispatch (CompiledFn.DynFrameWords). A fn-typed read the replay cannot
// seat refuses; a gradual one keeps the slot push it always had.

import (
	"fmt"
	"strings"
	"testing"
)

// requireParityHead is requireParity on the error's first line: the
// interpreter's real frame carries the body's DefCleanup marker, which its
// no-match NOTES list as a stray argument (`… and __dc (a __DC)`), while the
// island region re-stepped by the replay holds only the residual — the
// code and message agree, the notes name a marker only one side has.
func requireParityHead(t *testing.T, src string, gotC []any, errC error, gotI []any, errI error) {
	t.Helper()
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || firstErrLine(errC) != firstErrLine(errI) {
		t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}

const wrF = `def f fn [[g:Function][Any]`

// TestWordReadDispatchParity pins every measured shape that now agrees on
// both lanes, value and error text alike (positions are NUR118's).
func TestWordReadDispatchParity(t *testing.T) {
	rows := []struct{ src, want string }{
		{wrF + `[g]]  f ([] => [42])`, "42 — was fn"},
		{wrF + `[g]]  f (z:Integer => [z])`, "cannot call `g` — was fn (Integer)"},
		{wrF + `[g]]  f add/v`, "cannot call `g` — was fn add(…)"},
		{wrF + `[(g)]]  f ([] => [42])`, "42 — the paren's word dispatches"},
		{wrF + `[(g)]]  f (z:Integer => [z])`, "cannot call `g`"},
		{`def f fn [[g:Function][Any][g] [g:Function][Any][g]]  f ([] => [42])`, "42 — two overloads"},
		{wrF + `[def y 1  g]]  f ([] => [42])`, "42 — a def before the read is not after it"},
		{`def f fn [[g:Function][Integer][g]]  f ([] => [42])`, "42 — the dispatch's result meets the declared type"},
		{wrF + `[g]]  f ([] => [42]) print`, "42"},
		{`def id fn [[x:Any][Any][x]]  id ([] => [42])`, "42 — the gradual identity over a fn"},
		{`def id fn [[x:Any][Any][x]]  id 5`, "5 — and over plain data, no island"},
		{`def mk fn [[] [Function] [([] => [42])]]  def f fn [[g:Function][Any][g]]  f (mk)`, "42 — a returned closure"},
		{wrF + `[g]]  def g0 ([] => [42])  f g0/v`, "42 — a named 0-arg lambda"},
		{wrF + `[g]]  f (quote ([] => [42]))`, "42 — a quoted fn still dispatches as the binding"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:Integer => [mul 3 z]) 5`, "15 — the word collects the frame's stack"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:String => [z]) 5`, "cannot call `g`"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f (z:String => [z]) 5`, "cannot call `g` — was the count error over [fn 5] (NUR122's park)"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f ([] => [42]) 5`, "f: expected 1, got 2 — [42 5], the 0-arg fn fires (was [fn 5])"},
		{wrF + `[g 3]]  f (z:Integer => [mul 3 z])`, "9"},
		{wrF + `[g 3]]  f ([] => [42])`, "f: expected 1, got 2 — [42 3]"},
		{wrF + `[g 3]]  f (z:String => [z])`, "cannot call `g`"},
		{wrF + `[do [g]]]  f ([] => [42])`, "42 — the do body islands"},
		// a compiled CLOSURE reaching the binding: the bridge dispatches it by
		// name under the lambda's OWN declared params (lamParamContract), so a
		// typed lambda no-matches exactly as the interpreter's frame binding
		{`def mk fn [[k:Integer][Function][([] => [k])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "[3] — was [fn]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g 2]]  [(mk 3)] each [f]`, "[6]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g "s"]]  [(mk 3)] each [f]`, "cannot call `g` — was [0] (the closure ran over the String)"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "cannot call `g`"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 2]]  [(mk 3)] each [f]`, "cannot call `g` — was [3]"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 "b"]]  [(mk 3)] each [f]`, "[3]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function x:Integer][Any][x g]]  [(mk 3)] each [f 5]`, "[15] — the closure bridged, collecting the frame's x"},
		// value deliveries stay values: a Function-expecting forward, `/v`
		{`def h fn [[k:Function][Any][typeof k/v]]  def f fn [[g:Function][Any][h g]]  f ([] => [42])`, "Function — delivered, not dispatched"},
		{wrF + `[g/v]]  f ([] => [42]) typeof`, "Function"},
		{`[1 2] each ([g:Function] => [g])`, "[fn (Function) fn (Function)] — an unmatched lambda stays data"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParityHead(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestWordReadDispatchRefuses pins the reads the replay cannot seat: a
// container member, a branch residual, a stack-collected argument, a read
// mixed with a `/v` read of the same binding — each a sound fallback that
// answers the interpreter's value.
func TestWordReadDispatchRefuses(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{wrF + `[[g]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[{a: g}]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[if true [g] [0]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Function][Any][k/v]]  def f fn [[g:Function][Any][g h]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[g drop  g/v]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g/v drop  g]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g typeof]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		// a GRADUAL param bound to a fn at the call: the pass re-runs the
		// body under the argument's RUNTIME type (a Function carrier), so
		// the strict accounting applies — these refuse, they never diverged
		// (the map-literal spelling's CHECK pass is the NUR125 pin below).
		{`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][{a: (k)}]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][[k]]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][k typeof]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the read has no faithful lowering here", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestGradualFnParamMapLiteralCheckIsClean pins NUR125 (resolved 2026-09-05):
// the check pass over a map literal whose value is a bare read of a gradual
// param bound to a fn PANICKED (recovered as internal_error, both lanes,
// exit 1). The check run binds a concrete lambda argument by pushing it as
// a plain def — a FnDefInfo no installDef gave a runner — and the literal's
// const-fold sub-run dispatched the bare read by name into a signature
// whose DispatchHandler is nil. execMatch now raises internal_error on a
// nil runner (ADR-005: an error, never a panic), the fold declines on it,
// and the literal records normally: the check pass is clean, and both lanes
// answer the interpreter's value (the compiled lane through the refusal
// pinned above). Boru.Check is the entry point that reached the fold;
// CompileCheck never did, so the refusal rows alone could not pin it.
func TestGradualFnParamMapLiteralCheckIsClean(t *testing.T) {
	for _, src := range []string{
		`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: (k)}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: k}]]  h 5  h ([] => [42])`,
		`def h fn [[k:Any n:Integer][Any][{a: k, b: n}]]  h ([] => [42]) 1`,
	} {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		res, cerr := a.Check(src)
		if cerr != nil || len(res.Diagnostics) != 0 {
			t.Errorf("%q: check must be clean: err=%v diags=%v", src, cerr, res.Diagnostics)
		}
	}
	// the negative twin: a gradual param whose fn does not match its read
	// stays the interpreter's no_signature diagnostic, not a clean pass
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := `def h fn [[k:Any][Any][{a: k}]]  h (z:Integer => [z])`
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("%q: check error: %v", src, cerr)
	}
	if len(res.Diagnostics) != 1 || !strings.Contains(fmt.Sprint(res.Diagnostics[0]), "cannot call `k`") {
		t.Errorf("%q: want one no_signature diagnostic naming k, got %v", src, res.Diagnostics)
	}
}

// TestBodyLocalWordReadParity pins NUR123's leftover as far as the residual
// reaches (2026-09-05): a body-local `def` bound to a CONTAINER ELEMENT the
// pass types dynamic(Any) on every run — `def j (m get "f")  j` answered
// `fn` for the interpreter's 42, exit 0 — resolves through its producing
// event, not a slot, so NoteWordRead never counted it. A body-local
// producer of the unit is now the unit's word read (gradual: named, never
// strict), the tail test anchors a word read at its READ rather than at
// the value's producer (an event between the two ran before the dispatch
// on both lanes), and the VM's no-match reads the INSTALLED binding — the
// interpreter's "group the call in parens" help line included. Full-text
// parity (positions, notes and help) on every row.
//
// Still open on the record: the same read consumed OUTSIDE the residual
// (`j typeof`, `{a: j}`, an if/each/do body, the stack-collected `j j add`
// — which the VM's no-match deferral hands back to the interpreter, slow
// and right) or followed by an event (`j  def y 1`, `j  5 drop`), which
// keep the slot push — a gradual read is best effort — and the `/v` render
// (NUR119).
func TestBodyLocalWordReadParity(t *testing.T) {
	const hM = `def h fn [[m:Map][Any]`
	rows := []struct{ src, want string }{
		{hM + `[def j (m get "f")  j]]  h {f: ([] => [42])}`, "42 — was fn"},
		{hM + `[def j (m get "f")  j]]  h {f: 5}`, "5 — plain data, no island"},
		{hM + `[def j (m get "f")  j]]  h {f: (z:Integer => [z])}`, "cannot call `j` — with the interpreter's forward-args help line"},
		{hM + `[def j (m get "f")  j]]  h {f: (fn [[a:Integer] [Integer] [a add 1]])}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  j]]  h {f: add/v}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  j 3]]  h {f: (z:Integer => [mul 3 z])}`, "9"},
		{hM + `[def j (m get "f")  def y 1  j]]  h {f: ([] => [42])}`, "42 — a def between the producer and the read"},
		{hM + `[def j (m get "f")  def y 1  j y]]  h {f: (z:Integer => [mul 3 z])}`, "3"},
		{hM + `[def j (m get "f")  j drop  j]]  h {f: ([] => [42])}`, "42 — the first read consumed, the second seated"},
		{hM + `[def j m.f  j]]  h {f: ([] => [42])}`, "42 — the member spelling"},
		{`def h fn [[m:Map x:Integer][Any][def j (m get "f")  x j]]  h {f: (z:Integer => [mul 3 z])} 5`, "15 — the word collects the frame's stack"},
		{`def h fn [[xs:List][Any][def j (xs get 0)  j]]  h [([] => [42])]`, "42 — a list element"},
		{hM + `[def j (m get "f")  (j)]]  h {f: ([] => [42])}`, "42"},
		{`def mk fn [[] [Function] [([] => [42])]]  def f fn [[] [Any] [def r (mk)  r]]  f`, "42 — a returned closure bound to a body-local"},
		{`def f fn [[x:Integer] [Any] [def r (x add 1)  r]]  f 4`, "5 — a plain computed local"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
	// the top-level spelling has no frame to seat in: the Stage-3 refusal
	// it always had, and the interpreter's answer
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := `def m {f: ([] => [42])}  def j (m get "f")  j`
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog != nil || !strings.Contains(reason, "fn value read from a container auto-dispatches") {
		t.Errorf("%q: want the Stage-3 container refusal, got prog=%v reason=%q err=%v", src, prog != nil, reason, cerr)
	}
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	requireParity(t, src, gotC, errC, gotI, errI)
}

// TestBodyLocalDeoptParity pins the per-read DEOPT (NUR123, the ninth
// increment): a bare read of a body-local the pass types dynamic(Any) —
// `def j (m get "f")` over a Map param — consumed where the whole-frame
// replay cannot re-step it (a container member, an argument, a branch arm,
// a read an event follows, a def sourcing it) tests the binding's runtime
// value where the statement holding the read begins and, on a fn, hands
// the rest of the body to the interpreter (OpDeoptIfFn). Every row was a
// `fn` for the interpreter's value, or a wrong answer for its error, on
// the default lane with exit 0. Full-text parity; plain data pays the
// test alone (the `{f: 5}` twins).
func TestBodyLocalDeoptParity(t *testing.T) {
	const hM = `def h fn [[m:Map][Any]`
	rows := []struct{ src, want string }{
		{hM + `[def j (m get "f")  j typeof]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  j typeof]]  h {f: 5}`, "Integer — plain data, the test alone"},
		{hM + `[def j (m get "f")  {a: j}]]  h {f: ([] => [42])}`, "{a:42} — was {a:fn}"},
		{hM + `[def j (m get "f")  {a: j}]]  h {f: "s"}`, "{a:'s'}"},
		{hM + `[def j (m get "f")  {a: j, b: (print "x")}]]  h {f: ([] => [42])}`, "x once, {a:42} — the island runs the literal whole"},
		{hM + `[def j (m get "f")  {a: j}]]  h {f: (z:Integer => [z])}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  [j 1] size]]  h {f: ([] => [42])}`, "2"},
		{hM + `[def j (m get "f")  {a: j} get "a"]]  h {f: ([] => [42])}`, "42"},
		{hM + `[def j (m get "f")  if true [j] [0]]]  h {f: ([] => [42])}`, "42 — was fn"},
		{hM + `[def j (m get "f")  if false [0] [j]]]  h {f: ([] => [42])}`, "42"},
		{hM + `[def j (m get "f")  if true [j] [0] typeof]]  h {f: ([] => [42])}`, "Integer"},
		{`def h fn [[m:Map x:Integer][Any][def j (m get "f")  if (x gt 3) [j] [0]]]  h {f: ([] => [42])} 7`, "42"},
		{`def h fn [[m:Map x:Integer][Any][def j (m get "f")  if (x gt 3) [j] [0]]]  h {f: ([] => [42])} 1`, "0"},
		{hM + `[def j (m get "f")  j  def y 1]]  h {f: ([] => [42])}`, "42 — a read an event follows"},
		{hM + `[def j (m get "f")  j  5 drop]]  h {f: ([] => [42])}`, "42"},
		{hM + `[def j (m get "f")  j j add]]  h {f: ([] => [42])}`, "84 — both reads dispatch"},
		{hM + `[def j (m get "f")  5 j add]]  h {f: ([] => [42])}`, "47 — the literal below the read is on the stack at the test"},
		{hM + `[def a (m get "g")  def j (m get "f")  a j add]]  h {f: ([] => [42]) g: 1}`, "43"},
		{hM + `[def j (m get "f")  1 2 j drop add]]  h {f: ([] => [42])}`, "3"},
		{hM + `[def j (m get "f")  def k j  k]]  h {f: ([] => [42])}`, "def is still waiting — the interpreter's error, was 42"},
		{hM + `[def j (m get "f")  def k j  k]]  h {f: 5}`, "5"},
		{hM + `[def j (m get "f")  j typeof]]  h {f: ([] => [print "fired"  42])}`, "fired Integer — the fn's effects run once"},
		{hM + `[def j (m get "f")  print "before"  j typeof]]  h {f: ([] => [print "fired"  42])}`, "before fired — effects before the statement never repeat"},
		{hM + `[def j (m get "f")  j drop  j]]  h {f: ([] => [print "hi"  42])}`, "hi hi"},
		// the paren-nested read: the paren is the statement
		{hM + `[def j (m get "f")  (j typeof)]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  (j typeof)]]  h {f: 5}`, "Integer"},
		{hM + `[def j (m get "f")  (j typeof)]]  h {f: (z:Integer => [z])}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  (j typeof)  5  drop]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  def y 5  (j typeof)  y add 1  drop]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  ((m get "g") j add)]]  h {f: ([] => [42]) g: 1}`, "43"},
		{hM + `[def j (m get "f")  (1 j add)]]  h {f: ([] => [42])}`, "43"},
		{hM + `[def j (m get "f")  (j 1 add)]]  h {f: ([] => [42])}`, "43"},
		{hM + `[def j (m get "f")  (j) typeof]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  [(j typeof)]]]  h {f: ([] => [42])}`, "[Integer]"},
		{hM + `[def j (m get "f")  {a: (j typeof)}]]  h {f: ([] => [42])}`, "{a:Integer}"},
		{hM + `[def j (m get "f")  (j typeof) typeof]]  h {f: ([] => [42])}`, "Number"},
		{hM + `[def j (m get "f")  (j typeof) (m get "g") drop]]  h {f: ([] => [42]) g: 1}`, "Integer"},
		{hM + `[def j (m get "f")  print "before"  (j typeof)]]  h {f: ([] => [print "fired"  42])}`, "before fired"},
		{hM + `[def j (m get "f")  ((print "in") j typeof)]]  h {f: ([] => [print "fired"  42])}`, "in fired — the paren runs whole in the island"},
		{hM + `[def j (m get "f")  5  (j typeof)  drop]]  h {f: ([] => [42])}`, "5"},
		{hM + `[def j (m get "f")  [1 2]  (j typeof)  drop]]  h {f: ([] => [42])}`, "[1 2]"},
		{hM + `[def j (m get "f")  ([1 2] size)  (j typeof)  drop]]  h {f: ([] => [42])}`, "2"},
		{hM + `[def j (m get "f")  def k (j)  k]]  h {f: ([] => [42])}`, "42 — the def's source holds the read"},
		{hM + `[def j (m get "f")  def k (j)  k]]  h {f: 5}`, "5"},
		// an event between the read and its stack consumer
		{hM + `[def j (m get "f")  j (m get "g") add]]  h {f: ([] => [42]) g: 1}`, "43 — the get runs before the push: tested before it"},
		{hM + `[def j (m get "f")  j (m get "g") add]]  h {f: 5 g: 1}`, "6"},
		{hM + `[def j (m get "f")  (m get "g") j add]]  h {f: ([] => [42]) g: 1}`, "43"},
		{hM + `[def j (m get "f")  j print "x" typeof]]  h {f: ([] => [42])}`, "x Integer"},
		// a loop body: the for word is the statement
		{hM + `[def j (m get "f")  for 1 [j typeof]]]  h {f: ([] => [42])}`, "Integer — was Function"},
		{hM + `[def j (m get "f")  for 1 [(j typeof)]]]  h {f: ([] => [42])}`, "Integer"},
		{hM + `[def j (m get "f")  def y 1  for 1 [j typeof]]]  h {f: ([] => [42])}`, "Integer — the def consumed its literal"},
		// a def-bound result before the read: the island reads it by name
		{hM + `[def n (m size)  def j (m get "f")  j  n drop]]  h {f: ([] => [42])}`, "42 — was fn"},
		{hM + `[def n (m size)  def j (m get "f")  j  n add]]  h {f: ([] => [42])}`, "43"},
		{hM + `[def n (m size)  def j (m get "f")  (j typeof)  n drop]]  h {f: ([] => [42])}`, "Integer"},
		{hM + `[def j (m get "f")  j  def n (m size)  n drop]]  h {f: ([] => [42])}`, "42"},
		// an event after a residual read
		{hM + `[def j (m get "f")  j  (m get "g") drop]]  h {f: ([] => [42]) g: 1}`, "42"},
		{hM + `[def j (m get "f")  j  print "x"]]  h {f: ([] => [42])}`, "x 42"},
		{hM + `[def j (m get "f")  j  5 add 1  drop]]  h {f: ([] => [42])}`, "42"},
		// a def the island never reads keeps its plain lowering
		{hM + `[def y (m get "g")  def j (m get "f")  j typeof]]  h {f: ([] => [42]) g: 1}`, "Integer"},
		{hM + `[def y (m get "g")  def j (m get "f")  j typeof]]  h {f: 5 g: 1}`, "Integer"},
		{hM + `[def j (m get "f")  def y 5  {a: j}  drop  y add 5]]  h {f: ([] => [42])}`, "10"},
		// a CODE BODY's read of the captured local (the tenth increment):
		// the closure unit deopts on its capture slot, the parent binds j
		{hM + `[def j (m get "f")  [1] each [j]]]  h {f: ([] => [42])}`, "[42] — was [fn]"},
		{hM + `[def j (m get "f")  [1 2] each [j]]]  h {f: (z:Integer => [mul 3 z])}`, "[3 6] — was [fn (Integer) fn (Integer)]"},
		{hM + `[def j (m get "f")  [1] each [j]]]  h {f: 5}`, "[5]"},
		{hM + `[def j (m get "f")  [1 2] each [j add]]]  h {f: 5}`, "[6 7]"},
		{hM + `[def j (m get "f")  do [j]]]  h {f: ([] => [42])}`, "42 — was fn"},
		{hM + `[def j (m get "f")  do [j typeof]]]  h {f: ([] => [42])}`, "Integer"},
		{hM + `[def j (m get "f")  do [(j typeof)]]]  h {f: ([] => [42])}`, "Integer"},
		{hM + `[def j (m get "f")  do [def y 1  j]]]  h {f: ([] => [42])}`, "42"},
		{hM + `[def j (m get "f")  if true [do [j]] [0]]]  h {f: ([] => [42])}`, "42"},
		{hM + `[def j (m get "f")  [1] each [j] size]]  h {f: ([] => [42])}`, "1"},
		{hM + `[def j (m get "f")  5  do [j]  add]]  h {f: ([] => [42])}`, "47"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestLambdaValueBodyDeoptParity pins the fourteenth increment: NUR123's
// word-dispatch rule reaching a lambda VALUE's own body. The tenth increment
// closed the rule for a code body a native runs IN the enclosing frame
// (each, do, fold, for) and deliberately withheld the body tokens from an
// escaping unit — "no frame to resume in". Half of that was right: the
// FACTORY's frame is gone by the time the returned lambda is applied, so
// seedParentDeopt's promise does not hold. But the lambda's OWN frame is
// live for the whole apply, with its captures in slots, so it plans from
// there instead (lambdaNamesSelfBound + emitDynParamBinds over the capture
// slots). Before it, every row below answered `fn` / `Function` for the
// interpreter's value, on the default lane with exit 0.
func TestLambdaValueBodyDeoptParity(t *testing.T) {
	const hf = `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any]`
	rows := []struct{ src, want string }{
		{hf + `[j]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — was fn"},
		{hf + `[j]] )]]  def q (h {f: 5})  (q 7)`, "5 — plain data, the test alone"},
		{hf + `[j]] )]]  def q (h {f: "s"})  (q 7)`, "s"},
		{hf + `[j typeof]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "Integer — was Function"},
		{hf + `[j typeof]] )]]  def q (h {f: 5})  (q 7)`, "Integer"},
		{hf + `[[j 1] size]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "2 — the read inside a list literal"},
		{hf + `[x j add]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "49 — the capture and the lambda's own param together"},
		{hf + `[j j add]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "84 — both reads dispatch"},
		{hf + `[j typeof]] )]]  def q (h {f: ([] => [print "fired"  42])})  (q 7)`, "fired Integer — the fn's effects run once"},
		// the fifteenth increment: a read inside a BRANCH ARM of that body,
		// and inside a nested code body. The `if true` rows are the ones the
		// registry-membership form of lambdaNamesSelfBound declined, over the
		// bare word `true`; the each row raised a WRONG ERROR before it.
		{hf + `[if true [j] [0]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — was fn"},
		{hf + `[if true [j] [0]]] )]]  def q (h {f: 5})  (q 7)`, "5 — plain data"},
		{hf + `[if false [0] [j]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — the else arm"},
		{hf + `[if (x gt 3) [j] [0]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — a dynamic condition"},
		{hf + `[if true [j] [0] typeof]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "Integer — was Function"},
		{hf + `[[1] each [j]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "[42] — was the error `did you mean h or q?`"},
		// the sixteenth increment: the do body's value IS j's value, so its
		// result carrier is j's carrier and resolveOperand's capture override
		// sent the RESIDUAL back to the slot — the lowering emitted
		// `CALL_NATIVE do; DROP; PUSH_LOCAL`, discarding the island's dispatch.
		{hf + `[do [j]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — was fn"},
		{hf + `[do [j]]] )]]  def q (h {f: 5})  (q 7)`, "5 — plain data"},
		{hf + `[(do [j])]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "42 — the paren twin"},
		{hf + `[do [j typeof]]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "Integer — the point fires INSIDE the do body"},
		// The OPERAND form: `typeof` consumes the do's result while the body is
		// still recording, so it resolves through the fragFloors arm of
		// producedInCurrentUnit rather than the finish-time rec.frag arm. It was
		// recorded as still diverging when this increment first landed, on the
		// reading that the guard did not reach that path — wrong: the guard's
		// own floor index was off by one (es.frames opens with a root frame
		// carrying no floor), so the recording-time arm never fired at all.
		{hf + `[do [j] typeof]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "Integer — was Function"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// The same `do [j]` in a PLAIN fn body always agreed — that contrast is what
// located the fault in resolveOperand's capture override rather than in `do`
// or in the deopt, so it is pinned as the control it served as.
func TestDoBodyCaptureResidualPlainFn(t *testing.T) {
	src := `def h fn [[m:Map][Any][def j (m get "f")  do [j]]]  h {f: ([] => [42])}`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Fatalf("%q: must compile natively; err=%v", src, errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
	if fmt.Sprint(gotC) != "[42]" {
		t.Errorf("%q = %v, want [42]", src, gotC)
	}
}

// The lambda-body deopt's own boundaries, each measured. A read the island
// cannot seat, and a factory whose result the residual model cannot place,
// keep the whole-program refusal — a sound interpreter fallback, never a
// wrong answer. The `if` arm row is the one shape still DIVERGING (pinned
// as the interpreter's answer via the fallback lane, not as parity on a
// compiled run): a read inside a branch arm of a lambda body plans no point
// and keeps its slot push, exactly as it did before this increment.
func TestLambdaValueBodyDeoptRefusals(t *testing.T) {
	const hf = `def h fn [[m:Map][Function][def j (m get "f")  ( fn [[x:Integer][Any]`
	refuse := []struct{ src, reason string }{
		{hf + `[{a: j}]] )]]  def q (h {f: ([] => [42])})  (q 7)`, "body result of unknown provenance"},
	}
	for _, c := range refuse {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: want a refusal, got a compiled program", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal = %q, want %q", c.src, reason, c.reason)
		}
	}
	// The two-factory row refused "fn value precedes residual args" until the
	// thirty-sixth increment: the def of a produced closure claims its
	// shape, so each read models its own dispatch and the row compiles.
	src := hf + `[j]] )]]  def q (h {f: ([] => [42])})  def r (h {f: 5})  (q 7) (r 1)`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Fatalf("%q: must compile now; err=%v", src, errC)
	}
	requireParity(t, src, gotC, errC, gotI, errI)
}
