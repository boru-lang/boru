package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// runMatrixSrc parses and runs src on a fresh resolver-equipped registry,
// returning the top result stack (or the run error).
func runMatrixSrc(t *testing.T, src string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	modules.InstallResolver(reg)
	vals, pErr := parser.Parse(src)
	if pErr != nil {
		t.Fatal(pErr)
	}
	return native.NewTop(reg).Run(vals)
}

func wantOne(t *testing.T, src, want string) {
	t.Helper()
	out, err := runMatrixSrc(t, src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if len(out) != 1 || out[len(out)-1].String() != want {
		t.Fatalf("%q = %v, want [%s]", src, out, want)
	}
}

// TestMatrixCrossImportDispatch pins the iota…each import-boundary fix
// (the stats-library blocker): a tensor VALUE built by one
// `import "boru:matrix-util"` dispatches against ANOTHER import's words.
// Each import mints its own Tensor/Matrix/Vector nodes, so the nominal
// rule alone cannot match across the boundary; tensorFormatBehavior now
// matches STRUCTURALLY by payload rank (and by family kind for
// check-mode carriers). Before the fix, `MatrixUtil.cols mat` inside the
// imported fn found no signature, the wrapper parked as data, `def k`
// bound the FUNCTION, and `(iota k) each […]` collapsed into
// "cannot call iota — got (__FN)".
func TestMatrixCrossImportDispatch(t *testing.T) {
	// The reported repro, inline-module form: the module body has its
	// own matrix-util import (its own mints); the caller passes a matrix
	// from ITS import through the Any param.
	wantOne(t, `
import module [
  import "boru:matrix-util"
  def f fn [
    [mat:Any] [List] [
      def k (MatrixUtil.cols mat)
      (iota k) each ([j] => [j])
    ]
  ]
  export "M" { f: f/v }
]
import "boru:matrix-util"
(M.f (MatrixUtil.create [[1 2] [3 4]]) end)`, "[0 1]")

	// The wider surface the stats library needs: element reads inside
	// the each body, row access, scaling — all cross-import.
	wantOne(t, `
import module [
  import "boru:matrix-util"
  def diag fn [
    [mat:Any] [List] [
      (iota (MatrixUtil.rows mat)) each ([j] => [MatrixUtil.elem mat j j])
    ]
  ]
  export "M" { diag: diag/v }
]
import "boru:matrix-util"
(M.diag (MatrixUtil.create [[1 2] [3 4]]) end)`, "[1.0 4.0]")

	wantOne(t, `
import module [
  import "boru:matrix-util"
  def r0 fn [ [mat:Any] [List] [ MatrixUtil.row mat 0 ] ]
  export "M" { r0: r0/v }
]
import "boru:matrix-util"
(M.r0 (MatrixUtil.create [[1 2] [3 4]]) end)`, "[1.0 2.0]")

	// `is` agrees with dispatch across the boundary (the one-predicate
	// doctrine shortcut in isHandler) — and rank keeps Vector distinct.
	wantOne(t, `
import module [
  import "boru:matrix-util"
  def t1 fn [ [mat:Any] [Boolean] [ mat is MatrixUtil.Matrix ] ]
  def t2 fn [ [mat:Any] [Boolean] [ mat is MatrixUtil.Vector ] ]
  export "M" { t1: t1/v t2: t2/v }
]
import "boru:matrix-util"
def m (MatrixUtil.create [[1 2] [3 4]])
[(M.t1 m end) (M.t2 m end)]`, "[true false]")
}

// TestMatrixForwardFormAccessors pins the DUAL-signature accessors: the
// natural matrix-first forward form (`MatrixUtil.row mat 0` — the order
// the wrapper advertises) now dispatches, and the legacy stack form
// pinned in lang/spec/module-matrix.tsv §3 keeps working (same import,
// both orders — Matrix and Integer/Number are disjoint so the matched
// sig disambiguates).
func TestMatrixForwardFormAccessors(t *testing.T) {
	const pre = `import "boru:matrix-util" def m (MatrixUtil.create [[1 2] [3 4]]) `
	wantOne(t, pre+`(MatrixUtil.row m 1)`, "[3.0 4.0]")
	wantOne(t, pre+`(MatrixUtil.col m 0)`, "[1.0 3.0]")
	wantOne(t, pre+`(MatrixUtil.elem m 0 1)`, "2.0")
	wantOne(t, pre+`(MatrixUtil.scale m 2)`, "Matrix(2x2)")
	// Legacy stack forms (the spec's pinned spellings).
	wantOne(t, pre+`(m MatrixUtil.row 1)`, "[3.0 4.0]")
	wantOne(t, pre+`(m MatrixUtil.elem 1 0)`, "2.0")
	wantOne(t, pre+`(MatrixUtil.scale 2 m)`, "Matrix(2x2)")
}

// TestDottedParamAnnotation pins the dotted type annotation in fn param
// specs: `mat:MatrixUtil.Matrix` resolves through the bound module
// export to the tensor node at fn-construction time — so matrix params
// no longer have to be typed Any — and matches values from any import.
func TestDottedParamAnnotation(t *testing.T) {
	wantOne(t, `
import "boru:matrix-util"
def f fn [ [mat:MatrixUtil.Matrix] [Integer] [ MatrixUtil.cols mat ] ]
(f (MatrixUtil.create [[1 2] [3 4]]) end)`, "2")

	// Cross-import: the annotation is the lib's mint, the value the
	// caller's.
	wantOne(t, `
import module [
  import "boru:matrix-util"
  def f fn [ [mat:MatrixUtil.Matrix] [Integer] [ MatrixUtil.cols mat ] ]
  export "MA" { f: f/v }
]
import "boru:matrix-util"
(MA.f (MatrixUtil.create [[1 2] [3 4]]) end)`, "2")

	// Unnamed positional form.
	wantOne(t, `
import "boru:matrix-util"
def g fn [ [MatrixUtil.Matrix] [Integer] [ def mat args.0 MatrixUtil.rows mat ] ]
(g (MatrixUtil.create [[1 2] [3 4]]) end)`, "2")

	// NEGATIVE: a dotted path that reaches a non-type (a word export) is
	// an invalid parameter, not a silent Any.
	_, err := runMatrixSrc(t, `
import "boru:matrix-util"
def f fn [ [mat:MatrixUtil.cols] [Integer] [ 0 ] ]
0`)
	if err == nil || !strings.Contains(err.Error(), "invalid parameter") {
		t.Fatalf("non-type dotted annotation: err = %v, want invalid parameter", err)
	}

	// NEGATIVE: the annotation enforces — a non-matrix arg raises, it is
	// not admitted.
	_, err = runMatrixSrc(t, `
import "boru:matrix-util"
def f fn [ [mat:MatrixUtil.Matrix] [Integer] [ MatrixUtil.cols mat ] ]
(f 5 end)`)
	if err == nil || !strings.Contains(err.Error(), "no signature") {
		t.Fatalf("integer against Matrix param: err = %v, want no_signature", err)
	}
}

// TestMatrixDottedReturnTypeNoPhantom pins the fix for a DOTTED return type.
// A fn declaring `[MatrixUtil.Matrix]` as its RETURN type (a Reach reaching a
// module-exported type) was mis-classified by OutputSigIsConcreteReturns as a
// concrete return-BY-VALUE — the resolved Matrix type literal got SPLICED
// onto the body, so the fn left two values (its result plus the phantom
// literal) and every call raised "expected 1 return value(s), got 2". The
// plain-word type-name case was already handled; IsSigTypeValue now also
// recognises the dotted (Reach) form. Regression: the fn returns its matrix.
func TestMatrixDottedReturnTypeNoPhantom(t *testing.T) {
	// A fn whose return type is the dotted MatrixUtil.Matrix, returning the
	// param unchanged — no wrapper involved, so this isolates the return-type
	// splice (the stats cov-matrix false positive's root).
	wantOne(t, `
import "boru:matrix-util"
def id fn [ [m:MatrixUtil.Matrix] [MatrixUtil.Matrix] [ m ] ]
(id (make MatrixUtil.Matrix [[1.0 2.0][3.0 4.0]]))`, "Matrix(2x2)")

	// A wrapper-produced Matrix carrier through the dotted return type.
	wantOne(t, `
import "boru:matrix-util"
def tr fn [ [m:MatrixUtil.Matrix] [MatrixUtil.Matrix] [ MatrixUtil.transpose m ] ]
(tr (make MatrixUtil.Matrix [[1.0 2.0][3.0 4.0]]))`, "Matrix(2x2)")
}
