package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Stage M1 landing tests — the MERGED-WORD ReturnsFn seam
// (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §2.4a/§5/§6 Stage M1).
//
// An open word — a module-defined `add` merged into the importer's dispatch
// table (TransplantExtension) — dispatches as a BARE word through execMatch on
// the importer's engine. Its ReturnsFn closure captures the MODULE
// sub-registry, and before M1 nothing on that path shared the check state, so
// the closure read the module's own INACTIVE CheckState: StartFnCompile
// declined, RecordUserCall never ran, and every merged dispatch refused
// "user fn call … (Stage 3)" — the entire user-fn-call census bucket
// (open-words.tsv:72,77,78,82,85,86,87,98,99 + micron.tsv:200).
//
// The fix generalises shareCheckState to the ReturnsFn boundary itself
// (shareCheckStateFrom in buildFnBodyReturnsFn): the WHOLE user-fn seam now
// records identically no matter which dispatch path reached it (the §2.2
// one-recording-path rule), while name resolution stays in the module's own
// Defs/Types. These tests pin the compiled/interpreted differential for every
// reproducer shape, the no-island rule (refusal → NATIVE, never → island),
// and the negative twins (non-exported / undef'd / mistyped tuples keep their
// byte-identical error taxonomy; a dynamic operand stays sound).

// mergedWordSound pins RunCompiled == Run on value AND error taxonomy.
func mergedWordSound(t *testing.T, src string) {
	t.Helper()
	a, _ := New()
	want, werr := a.RunInterp(src)
	b, _ := New()
	got, _, gerr := b.RunCompiled(src)
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
}

// mergedWordCompilesNative additionally requires a NATIVE lowering: the
// program compiles (no refusal) and contains no FALLBACK island — the design
// round's codified rule that no stage may convert a refusal into an island.
func mergedWordCompilesNative(t *testing.T, src string) {
	t.Helper()
	mergedWordSound(t, src)
	a, _ := New()
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("did not compile: reason=%q err=%v\n  src: %s", reason, cerr, src)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("expected a native lowering of the merged-word dispatch, got an island\n  src: %s", src)
	}
}

const mergedFlagModule = `import module [` +
	`def Flag (refine Boolean) ` +
	`def add fn [[a:Flag b:Flag] [Boolean] [a and b]] ` +
	`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
	`export "M" {add: add/v mk: mk/v Flag: Flag}]  `

// open-words.tsv:72 — a MIXED tuple (one builtin + one user-minted type)
// anchors the transplanted signature; the module-minted Flag param rides
// SetUnitParamTypes as a *Type pointer (the CanonicalType rule).
func TestMergedWordSeam_MixedTuple(t *testing.T) {
	mergedWordCompilesNative(t,
		`import module [def Flag (refine Boolean) `+
			`def add fn [[a:Integer b:Flag] [Boolean] [true]] `+
			`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] `+
			`export "M" {add: add/v mk: mk/v}]  add 1 (M.mk true)`)
}

// open-words.tsv:77/78 — the transplanted BODY runs (module-closure
// execution), pinned by both boolean outcomes so a stub match cannot pass.
func TestMergedWordSeam_BodyRunsBothOutcomes(t *testing.T) {
	mergedWordCompilesNative(t, mergedFlagModule+`add (M.mk true) (M.mk true)`)
	mergedWordCompilesNative(t, mergedFlagModule+`add (M.mk true) (M.mk false)`)
}

// open-words.tsv:82 — the importer constructs its OWN values via the exported
// type (typed-def alias + OpBindTyped) and the transplanted sig dispatches.
func TestMergedWordSeam_ImporterConstructedValues(t *testing.T) {
	mergedWordCompilesNative(t, mergedFlagModule+
		`def Flag M.Flag  def x:Flag true  def y:Flag true  add x y`)
}

// open-words.tsv:85 — re-export is the opt-in for transitivity: the extension
// crosses TWO module boundaries via the outer module's re-export.
func TestMergedWordSeam_ReExportTransitivity(t *testing.T) {
	mergedWordCompilesNative(t,
		`import module [`+
			`import module [def Flag (refine Boolean) `+
			`def add fn [[a:Flag b:Flag] [Boolean] [a and b]] `+
			`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] `+
			`export "I" {add: add/v mk: mk/v Flag: Flag}] `+
			`def Flag I.Flag def omk fn [[b:Boolean] [Flag] [def v:Flag b v]] `+
			`export "O" {mk: omk/v add: add/v}]  add (O.mk true) (O.mk true)`)
}

// open-words.tsv:86/87 — two modules' same-shaped extensions COEXIST, each
// anchored to its own minted type; BOTH directions pinned so a unit compiled
// for one module's sig cannot silently serve the other's values.
func TestMergedWordSeam_TwoModulesCoexist(t *testing.T) {
	const twoModules = `import module [def Flag (refine Boolean) ` +
		`def add fn [[a:Flag b:Flag] [Boolean] [true]] ` +
		`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
		`export "M" {add: add/v mk: mk/v}]  ` +
		`import module [def Flag (refine Boolean) ` +
		`def add fn [[a:Flag b:Flag] [Boolean] [false]] ` +
		`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
		`export "N" {add: add/v mk: mk/v}]  `
	mergedWordCompilesNative(t, twoModules+`add (M.mk true) (M.mk true)`) // → true
	mergedWordCompilesNative(t, twoModules+`add (N.mk true) (N.mk true)`) // → false
}

// open-words.tsv:98/99 — a module CLASS anchors the extension; the body
// constructs a fresh instance (make + field reads through the record schema).
func TestMergedWordSeam_ClassTuple(t *testing.T) {
	const pointModule = `import module [def Point class {x:Integer y:Integer} ` +
		`def add fn [[a:Point b:Point] [Point] [make Point {x:(a.x add b.x) y:(a.y add b.y)}]] ` +
		`export "Pointer" {Point: Point add: add/v}]  ` +
		`def p0 (make Pointer.Point {x:1 y:2})  def p1 (make Pointer.Point {x:4 y:6})  `
	mergedWordCompilesNative(t, pointModule+`def p2 (p0 add p1)  p2.x`)
	mergedWordCompilesNative(t, pointModule+`def p2 (p0 add p1)  p2 is Pointer.Point`)
}

// micron.tsv:200 — a module-minted Micron (refine-with-body) anchors the
// extension; the body reads the micron field through dot-access.
func TestMergedWordSeam_MicronTuple(t *testing.T) {
	mergedWordCompilesNative(t,
		`import module [def Baron refine Micron {v:Integer} `+
			`def add fn [[a:Baron b:Baron] [Integer] [(a.v) add (b.v)]] `+
			`def mk fn [[n:Integer] [Baron] [make Baron {v:n}]] `+
			`export "M" {add: add/v mk: mk/v}]  (M.mk 2) add (M.mk 3)`)
}

// NEGATIVE TWINS — the error taxonomy of every non-crossing shape stays
// byte-identical (compiled-or-fallback parity; open-words.tsv:32,83,90,100).
func TestMergedWordSeam_NegativeTwins(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// open-words.tsv:83 — a module-body merge WITHOUT an export of the
			// word is module-private; nothing crosses the boundary.
			"non-exported merge stays module-private",
			`import module [def Flag (refine Boolean) ` +
				`def add fn [[a:Flag b:Flag] [Boolean] [a and b]] ` +
				`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
				`export "M" {mk: mk/v Flag: Flag}]  add (M.mk true) (M.mk true)`,
		},
		{
			// open-words.tsv:90 — undef pops the transplant.
			"undef removes the transplanted tuple",
			mergedFlagModule + `undef add  add (M.mk true) (M.mk true)`,
		},
		{
			// open-words.tsv:100 — the overload is anchored to the class on
			// BOTH operands; a Point plus a number stays refused.
			"class tuple rejects a mixed call",
			`import module [def Point class {x:Integer y:Integer} ` +
				`def add fn [[a:Point b:Point] [Point] [make Point {x:(a.x add b.x) y:(a.y add b.y)}]] ` +
				`export "Pointer" {Point: Point add: add/v}]  ` +
				`def p0 (make Pointer.Point {x:1 y:2})  add p0 1`,
		},
		{
			// open-words.tsv:32 — a body-local merge is INVISIBLE after fn exit.
			"body-local merge invisible after fn exit",
			`def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			_, werr := a.RunInterp(c.src)
			if werr == nil {
				t.Fatalf("negative twin unexpectedly succeeded interpreted:\n  src: %s", c.src)
			}
			mergedWordSound(t, c.src)
		})
	}
}

// A DYNAMIC operand reaching a merged word is an AMBIGUOUS dispatch (the
// merged user sig plus the native overloads are all optimistically reachable)
// — it must stay sound: either a faithful compile or a refusal + fallback,
// never a silently-committed wrong overload (the §2.2 poison signature; the
// interpreter re-matches at run time and raises signature_error here because
// a plain Boolean is NOT a Flag — the newtype is nominal).
func TestMergedWordSeam_DynamicOperandStaysSound(t *testing.T) {
	mergedWordSound(t, mergedFlagModule+
		`def fx (flex {v:true})  def x (fx get "v")  add x (M.mk true)`)
}

// Diagnostics propagation — the other half of the seam. Before M1 a merged
// fn's body was ANALYSED CONCRETELY with its diagnostics stranded on the
// module registry's inactive CheckState, so a body defect was INVISIBLE to
// `check` while the sibling dot-access path (M.add) reported it. Now the
// shared state routes body diagnostics to the checking pass: the typo is
// reported, aligning the merged path with every other user-fn dispatch (and
// with the interpreter, which raises undefined_word at run time).
func TestMergedWordSeam_BodyDiagnosticsSurface(t *testing.T) {
	const src = `import module [def Flag (refine Boolean) ` +
		`def add fn [[a:Flag b:Flag] [Boolean] [a zzz-typo b]] ` +
		`def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] ` +
		`export "M" {add: add/v mk: mk/v}]  add (M.mk true) (M.mk true)`
	a, _ := New()
	res, _ := a.Check(src)
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "undefined_word" && strings.Contains(d.Detail, "zzz-typo") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("merged-body typo not reported by check (diagnostics stranded on the module registry?): %+v",
			res.Diagnostics)
	}
	// And the runtime taxonomy stays in agreement (both paths error).
	mergedWordSound(t, src)
}
