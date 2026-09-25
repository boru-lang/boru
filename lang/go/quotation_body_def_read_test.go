package lang

import (
	"fmt"
	"strings"
	"testing"
)

// quotation_body_def_read_test.go pins NUR156 (2026-09-22): the two shapes
// module-composition L102–L104 diverged on, silently, since the corpus
// expansion — and the local twins that showed neither was module-specific.
//
//   - a `/v`-MARKED reach group under the `apply` word (`5 M.inc/v apply`):
//     the marker's consumption at the group's collapse leaves the value
//     QUOTED, the runtime handler unquotes it and the re-step dispatches,
//     but the check-mode model kept the quote, so the concrete lead parked
//     on the pass and the re-step recorded nothing — compiled `[5 fn]`. The
//     model now delivers the value unquoted, as the handler does
//     (applyReturns, lang/go/native).
//   - a module-scope def bound to a fn-valued MEMBER read, read BARE inside
//     a code body (`def f tbl.inc end each [f] xs`): the interpreter's
//     stepWord never substitutes a binding holding a fn — the read is a WORD
//     dispatch under the name — where the closure unit captured the VALUE
//     and returned it as data. NoteWordRead now records such a read's NAME
//     on the unit (never the strict count), and the closure-body replay
//     arms on a named def read of a tagged member with the word table: the
//     VM applies natively on a match and re-steps the word through the
//     island on a no-match, which raises `cannot call `f`` as the
//     interpreter does.

const qdModule = `import module [def inc fn n:Integer Integer [n add 1] export "M" {inc: inc/v}] end `
const qdTbl = `import module [def h1 fn n:Integer Integer [n add 1] def tbl {inc: h1/v} export "M" {tbl: tbl}] end `
const qdLocal = `def h1 fn n:Integer Integer [n add 1] end def tbl {inc: h1/v} end def f tbl.inc end `

// TestQuotationBodyDefReadParity: every row compiles and agrees with the
// interpreter.
func TestQuotationBodyDefReadParity(t *testing.T) {
	rows := []struct{ src, want, note string }{
		// the /v-marked export under apply (L102 and its neighbours)
		{qdModule + `5 M.inc/v apply`, "6", "module-composition L102: the /v-marked export under apply"},
		{qdModule + `(5 M.inc/v apply)`, "6", "the same inside a paren"},
		{qdModule + `5 M.inc/v apply add 10`, "16", "the result feeds a later word"},
		{qdModule + `def i 0 end def i (i M.inc/v apply) end i`, "1", "the result def-bound"},
		{`import module [def z fn [[][Integer][7]] export "M" {z: z/v}] end 5 M.z/v apply`, "5 7", "a 0-arg named export fires above the value"},
		{qdModule + `5 (M.inc) apply`, "6", "the unmarked group (as before)"},
		// the def-bound member read (L103 and its neighbours)
		{qdTbl + `def f M.tbl.inc end each [f] [1 2 3]`, "[2 3 4]", "module-composition L103: the def-bound member read bare in an each body"},
		{qdLocal + `each [f] [1 2 3]`, "[2 3 4]", "its local twin"},
		{qdLocal + `each [dup f] [1 2 3]`, "[2 3 4]", "over a duplicated element"},
		{qdLocal + `do [5 f]`, "6", "a do body over a literal"},
		{qdLocal + `each [f add 10] [1 2 3]`, "[12 13 14]", "a later word (the drift window, as before)"},
		{qdLocal + `each [f/v apply] [1 2 3]`, "[2 3 4]", "the /v read applied (as before)"},
		{qdTbl + `def f M.tbl.inc end f 5`, "6", "the word call (as before)"},
		{`def z fn [[][Integer][7]] end def tbl {z: z/v} end def f tbl.z end each [f] [1 2]`, "[7 7]", "a 0-arg member is the landing's at the def (as before)"},
		// the loop-carried root def computed by the /v-marked export under
		// apply (L104 and its local twin): declined "dynamic-scope def `i` of
		// unpromoted computed value" until the user-call write-back promoted
		// the call's result for the carried store and the write-back alike
		{qdModule + `def i 0 end while [i lt 3] [def i (i M.inc/v apply)] end i`, "3", "module-composition L104: the carried root def computed by the export under apply"},
		{`def inc fn n:Integer Integer [n add 1] end def i 0 end while [i lt 3] [def i (i inc/v apply)] end i`, "3", "its local twin (callbacks L89)"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%q: %v, want %s (%s)", c.src, gotC, c.want, c.note)
		}
	}
}

// TestQuotationBodyDefReadNoMatchParity: a lead the values beneath do not
// match raises the interpreter's own error on both lanes — the export's
// `uncalled_function`, the def read's named `cannot call `f“ (the word
// island's dispatch under the binding name).
func TestQuotationBodyDefReadNoMatchParity(t *testing.T) {
	rows := []struct{ src, code, note string }{
		{qdModule + `"s" M.inc/v apply`, "uncalled_function", "the /v-marked export over a value its param rejects"},
		{qdLocal + `each [f] ["s"]`, "signature_error", "the def read over a String element: cannot call `f`"},
		{qdLocal + `each ([e:Integer] => [f]) [1 2 3]`, "signature_error", "a named-param lambda: nothing beneath the read"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: not compiled (%s)", c.src, c.note)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if codeOf(errI) != c.code || codeOf(errC) != c.code {
			t.Errorf("%q: interp [%s] compiled [%s], want %s on both (%s)", c.src, codeOf(errI), codeOf(errC), c.code, c.note)
		}
	}
}

// TestQuotationBodyDefReadSoundCompileFailures: the neighbours that DECLINE,
// each with the interpreter's own answer beside it. L104 was one: it used to
// compile to a runaway loop (`tape_exhausted`) because the apply inside its
// body never fired; with the apply recorded it declined where its local
// twin always did — the dynamic-scope def family's own gate, loud — until
// the user-call write-back (2026-09-25) promoted the call's result for the
// carried root def's write-back; both spellings compile with parity now
// (TestQuotationBodyDefReadParity's last rows).
func TestQuotationBodyDefReadSoundCompileFailures(t *testing.T) {
	rows := []struct{ src, reason, interp string }{
		{qdLocal + `0 fold [add f] [1 2 3]`, "result above a literal", "[9]"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: check: %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — expected a compile failure", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: declined %q, want %q", c.src, reason, c.reason)
		}
		d, err := New()
		if err != nil {
			t.Fatal(err)
		}
		gotI, errI := d.RunInterp(c.src)
		if errI != nil || fmt.Sprint(gotI) != c.interp {
			t.Errorf("%q: interpreter %v err=%v, want %s", c.src, gotI, errI, c.interp)
		}
	}
}

// TestDefReadWordDispatchPending pins what NUR156's fix does NOT reach — the
// same def-bound member read outside a code-body closure, where no replay
// window exists yet: at the MAIN program a bare `5 f` pushes the value
// (`[5 fn]` for the interpreter's 6), and inside a named fn unit the read
// bails as a dynamic-scope read of a dispatching binding. Both are the
// read model's open points (NUR123). The pin fails the day a row agrees:
// move it to TestQuotationBodyDefReadParity.
func TestDefReadWordDispatchPending(t *testing.T) {
	src := qdLocal + `5 f`
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[6]" {
		t.Fatalf("%q: interpreter oracle moved: %v err=%v — re-derive NUR156", src, gotI, errI)
	}
	if !compiled || errC != nil {
		t.Errorf("%q: the top-level read became loud (%v) — record that and retire this pin", src, errC)
	} else if fmt.Sprint(gotC) == fmt.Sprint(gotI) {
		t.Errorf("%q: the top-level read agrees (%v) — move the row to TestQuotationBodyDefReadParity", src, gotC)
	}
	src = qdLocal + `def g fn [[Integer][Any][f]] end g 5`
	gotC, compiled, errC, gotI, errI = runBothEngines(t, src)
	if errI != nil || fmt.Sprint(gotI) != "[6]" {
		t.Fatalf("%q: interpreter oracle moved: %v err=%v — re-derive NUR156", src, gotI, errI)
	}
	if !compiled || !isBailDefect(errC) {
		t.Errorf("%q: the fn-unit read no longer bails (compiled=%v %v err=%v) — retire this pin", src, compiled, gotC, errC)
	}
}
