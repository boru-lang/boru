package modules

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// Wave-3 matrix coverage: DeepClone via the universal clone word, the
// IdealConverter (ToList / ToMap / tensorNested) via `convert`, mat-sub,
// rank edges (Rows/Cols on low-rank tensors), Format/kind-name/shape
// fallbacks, and — paired with every positive case — the loud failures:
// bad dimensions, out-of-bounds access, dimension mismatches, malformed
// row data, and type-literal arguments (errors, never panics).

// runTensorSrcErr parses and runs src, requiring an error containing want.
func runTensorSrcErr(t *testing.T, r *native.Registry, src, want string) {
	t.Helper()
	res, err := runTensorSrc(t, r, src)
	if err == nil {
		t.Fatalf("%s: expected error, got %v", src, res)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: error %q does not contain %q", src, err.Error(), want)
	}
}

// findNativeWave3 returns the named NativeFunc from a module natives slice.
func findNativeWave3(t *testing.T, natives []native.NativeFunc, name string) native.NativeFunc {
	t.Helper()
	for _, n := range natives {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("native %q not found", name)
	return native.NativeFunc{}
}

// callHandlerWave3 invokes a native's first-signature Go handler directly,
// asserting it never panics. Used to pin the per-argument concrete-value
// guards (ADR-005) that signature dispatch shields from type literals.
func callHandlerWave3(t *testing.T, n native.NativeFunc, r *native.Registry, args ...native.Value) ([]native.Value, error) {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("%s panicked: %v", n.Name, rec)
		}
	}()
	h := n.Signatures[0].DispatchHandler()
	if h == nil {
		t.Fatalf("%s has no Go handler", n.Name)
	}
	return h(args, nil, nil, r)
}

// TestMatrixCloneDeepWave3 pins TensorData.DeepClone through the universal
// clone word (StructUtil.clone): the clone is an independent tensor — same
// shape and data, distinct backing arrays.
func TestMatrixCloneDeepWave3(t *testing.T) {
	r := tensorRegistry(t)
	if err := InstallStructExports(r); err != nil {
		t.Fatal(err)
	}
	res, err := runTensorSrc(t, r,
		"def m (make MatrixUtil.Matrix [[1 2][3 4]]) [m (StructUtil.clone m)]")
	if err != nil {
		t.Fatal(err)
	}
	lst, lerr := native.AsList(res[len(res)-1])
	if lerr != nil || lst.Len() != 2 {
		t.Fatalf("expected [orig clone], got %v (%v)", res, lerr)
	}
	orig, cl := AsTensor(lst.Get(0)), AsTensor(lst.Get(1))
	if !shapeEqual(cl.Shape, []int{2, 2}) || cl.Data[3] != 4 {
		t.Fatalf("clone = shape %v data %v", cl.Shape, cl.Data)
	}
	// Mutating the clone's backing arrays must not touch the original.
	cl.Data[0] = 99
	cl.Shape[0] = 7
	if orig.Data[0] != 1 || orig.Shape[0] != 2 {
		t.Errorf("clone shares backing arrays with original: %v %v", orig.Data, orig.Shape)
	}
}

// TestMatrixConvertListMapWave3 pins the IdealConverter: `convert List`
// gives the nested row structure, `convert Map` gives {shape values}.
func TestMatrixConvertListMapWave3(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "convert List (make MatrixUtil.Matrix [[1 2][3 4]])")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "[[1.0 2.0] [3.0 4.0]]" {
		t.Errorf("convert List = %q", got)
	}
	res, err = runTensorSrc(t, r, "convert Map (make MatrixUtil.Matrix [[1 2][3 4]])")
	if err != nil {
		t.Fatal(err)
	}
	m, merr := native.AsMap(res[0])
	if merr != nil {
		t.Fatalf("convert Map result: %v", merr)
	}
	sh, _ := m.Get("shape")
	vals, _ := m.Get("values")
	if sh.String() != "[2 2]" || vals.String() != "[1.0 2.0 3.0 4.0]" {
		t.Errorf("convert Map = shape %s values %s", sh.String(), vals.String())
	}
	// Rank-3 tensor exercises tensorNested recursion.
	res, err = runTensorSrc(t, r, "convert List (make MatrixUtil.Tensor [[[1 2][3 4]][[5 6][7 8]]])")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "[[[1.0 2.0] [3.0 4.0]] [[5.0 6.0] [7.0 8.0]]]" {
		t.Errorf("convert List rank-3 = %q", got)
	}
	// Rank-1 vector: the flat-leaf branch.
	res, err = runTensorSrc(t, r, "convert List (make MatrixUtil.Vector [1 2 3])")
	if err != nil {
		t.Fatal(err)
	}
	if got := res[0].String(); got != "[1.0 2.0 3.0]" {
		t.Errorf("convert List vector = %q", got)
	}
}

// TestMatrixConverterNoPayloadWave3 pins the payload-less fallbacks of the
// converter and formatter: a tensor-typed value with no TensorData yields
// the empty projection and the bare kind name — never a panic.
func TestMatrixConverterNoPayloadWave3(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	// A Matrix-typed extension carrying a non-TensorData body.
	odd := core.NewExtension(tt.Matrix, "not-a-tensor")
	lv, lerr := tensorFormatBehavior{}.ToList(odd)
	if lerr != nil {
		t.Fatalf("ToList no payload: %v", lerr)
	}
	if lst, _ := native.AsList(lv); lst.Len() != 0 {
		t.Errorf("ToList no payload = %s, want []", lv.String())
	}
	mv, merr := tensorFormatBehavior{}.ToMap(odd)
	if merr != nil {
		t.Fatalf("ToMap no payload: %v", merr)
	}
	if m, _ := native.AsMap(mv); m.Len() != 0 {
		t.Errorf("ToMap no payload = %s, want {}", mv.String())
	}
	if got := (tensorFormatBehavior{}).Format(odd); got != "Matrix" {
		t.Errorf("Format no payload = %q, want Matrix", got)
	}
}

// TestMatrixRankEdgesWave3 pins the Go-level shape accessors on low-rank
// tensors, the shapeString scalar rendering, the tensorKindName fallback
// for a non-tensor ancestry, and the AsTensor zero value on a non-tensor.
func TestMatrixRankEdgesWave3(t *testing.T) {
	rank0 := TensorData{Shape: []int{}, Data: []float64{5}}
	if rank0.Rows() != 0 || rank0.Cols() != 0 || rank0.Rank() != 0 {
		t.Errorf("rank-0 rows/cols/rank = %d/%d/%d, want 0/0/0", rank0.Rows(), rank0.Cols(), rank0.Rank())
	}
	rank1 := TensorData{Shape: []int{3}, Data: []float64{1, 2, 3}}
	if rank1.Rows() != 3 || rank1.Cols() != 0 {
		t.Errorf("rank-1 rows/cols = %d/%d, want 3/0", rank1.Rows(), rank1.Cols())
	}
	if got := shapeString(nil); got != "scalar" {
		t.Errorf("shapeString(nil) = %q, want scalar", got)
	}
	if got := tensorKindName(native.TInteger); got != "Tensor" {
		t.Errorf("tensorKindName(Integer) = %q, want Tensor fallback", got)
	}
	if td := AsTensor(native.NewInteger(5)); td.Rank() != 0 || len(td.Data) != 0 {
		t.Errorf("AsTensor(non-tensor) = %+v, want zero", td)
	}
	// A rank-0 tensor value formats as Kind(scalar).
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	v := tensorValue(tt.Tensor, rank0)
	if got := (tensorFormatBehavior{}).Format(v); got != "Tensor(scalar)" {
		t.Errorf("Format rank-0 = %q, want Tensor(scalar)", got)
	}
	// tensorNested with a zero-length leading dimension yields [].
	empty := tensorNested(TensorData{Shape: []int{0, 2}, Data: nil}, 0, 0, 0)
	if lst, _ := native.AsList(empty); lst.Len() != 0 {
		t.Errorf("tensorNested zero dim = %s, want []", empty.String())
	}
}

// TestMatrixMatSubWave3 pins mat-sub (namespaced and via the transplanted
// bare `sub` word extension), paired with its dimension-mismatch failure.
func TestMatrixMatSubWave3(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r,
		"(make MatrixUtil.Matrix [[5 6][7 8]]) MatrixUtil.mat-sub (make MatrixUtil.Matrix [[1 2][3 4]])")
	if err != nil {
		t.Fatal(err)
	}
	got := AsTensor(res[0])
	if got.Data[0] != 4 || got.Data[3] != 4 {
		t.Errorf("mat-sub = %v, want [4 4 4 4]", got.Data)
	}
	// Transplanted bare `sub` on two matrices routes the same handler.
	res, err = runTensorSrc(t, r,
		"(make MatrixUtil.Matrix [[9 9][9 9]]) sub (make MatrixUtil.Matrix [[1 2][3 4]])")
	if err != nil {
		t.Fatal(err)
	}
	got = AsTensor(res[0])
	if got.Data[0] != 8 || got.Data[3] != 5 {
		t.Errorf("matrix sub = %v, want [8 7 6 5]", got.Data)
	}
	// Negative: mismatched shapes are rejected.
	runTensorSrcErr(t, r,
		"(make MatrixUtil.Matrix [[1 2][3 4]]) MatrixUtil.mat-sub (make MatrixUtil.Matrix [[1 2 3][4 5 6]])",
		"mat-sub: dimension mismatch")
}

// TestMatrixArithmeticMismatchWave3 pins the dimension-mismatch failures
// of the remaining arithmetic words.
func TestMatrixArithmeticMismatchWave3(t *testing.T) {
	r := tensorRegistry(t)
	pre := "def a (make MatrixUtil.Matrix [[1 2][3 4]]) def b (make MatrixUtil.Matrix [[1 2 3][4 5 6][7 8 9]]) "
	runTensorSrcErr(t, r, pre+"a MatrixUtil.mat-add b", "mat-add: dimension mismatch")
	runTensorSrcErr(t, r, pre+"a MatrixUtil.mat-emul b", "mat-emul: dimension mismatch")
	runTensorSrcErr(t, r, pre+"a MatrixUtil.mat-mul b", "mat-mul: dimension mismatch")
}

// TestMatrixDimValidationWave3 pins validDims through the constructors:
// negative dimensions and cap-exceeding products are declined.
func TestMatrixDimValidationWave3(t *testing.T) {
	r := tensorRegistry(t)
	runTensorSrcErr(t, r, "MatrixUtil.zeros -1 3", "dimensions must be non-negative")
	runTensorSrcErr(t, r, "MatrixUtil.ones -2 2", "dimensions must be non-negative")
	runTensorSrcErr(t, r, "MatrixUtil.eye -1", "dimensions must be non-negative")
	runTensorSrcErr(t, r, "MatrixUtil.fill -1 3 1.0", "dimensions must be non-negative")
	runTensorSrcErr(t, r, "MatrixUtil.zeros 100000 100000", "exceeds the cap")
	runTensorSrcErr(t, r, "MatrixUtil.eye 100000", "exceeds the cap")
}

// TestMatrixAccessBoundsWave3 pins the out-of-bounds failures of
// elem / row / col, alongside an in-bounds read (forward form binds the
// inner sig top-first: `m MatrixUtil.elem c r` reads column c, row r).
func TestMatrixAccessBoundsWave3(t *testing.T) {
	r := tensorRegistry(t)
	pre := "def m (make MatrixUtil.Matrix [[1 2][3 4]]) "
	res, err := runTensorSrc(t, r, pre+"m MatrixUtil.elem 1 0")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsNumber(res[len(res)-1]); n != 2 {
		t.Errorf("elem col=1 row=0 = %v, want 2", n)
	}
	runTensorSrcErr(t, r, pre+"m MatrixUtil.elem 5 0", "out of bounds")
	runTensorSrcErr(t, r, pre+"m MatrixUtil.elem 0 5", "out of bounds")
	runTensorSrcErr(t, r, pre+"m MatrixUtil.row 5", "out of bounds")
	runTensorSrcErr(t, r, pre+"m MatrixUtil.col 5", "out of bounds")
}

// TestMatrixDetWave3 pins det on a singular matrix (0, not an error) and
// the non-square compile failures of det and trace.
func TestMatrixDetWave3(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "MatrixUtil.det (make MatrixUtil.Matrix [[1 2][2 4]])")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsNumber(res[0]); n != 0 {
		t.Errorf("det singular = %v, want 0", n)
	}
	runTensorSrcErr(t, r, "MatrixUtil.det (make MatrixUtil.Matrix [[1 2 3][4 5 6]])", "not square")
	runTensorSrcErr(t, r, "MatrixUtil.tr (make MatrixUtil.Matrix [[1 2 3][4 5 6]])", "not square")
}

// TestMatrixDotErrorsWave3 pins the vector-dot failures: length mismatch
// and non-number elements on either side, next to one valid product.
func TestMatrixDotErrorsWave3(t *testing.T) {
	r := tensorRegistry(t)
	res, err := runTensorSrc(t, r, "[1 2 3] MatrixUtil.dot [4 5 6]")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := native.AsNumber(res[0]); n != 32 {
		t.Errorf("dot = %v, want 32", n)
	}
	runTensorSrcErr(t, r, "[1 2] MatrixUtil.dot [1 2 3]", "length mismatch")
	runTensorSrcErr(t, r, "['a' 'b'] MatrixUtil.dot [1 2]", "")
	runTensorSrcErr(t, r, "[1 2] MatrixUtil.dot ['a' 'b']", "")
}

// TestMatrixDotNilListWave3 pins the nil-list guard inside the dot
// handler. Signature dispatch declines bare List literals (uncalled
// function), so the guard is exercised by direct handler invocation.
func TestMatrixDotNilListWave3(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	dot := findNativeWave3(t, matrixNatives(tt), "matrix-dot")
	lit := native.NewTypeLiteral(native.TList)
	if _, herr := callHandlerWave3(t, dot, r, lit, lit); herr == nil ||
		!strings.Contains(herr.Error(), "expected two lists") {
		t.Errorf("dot on list literals: got %v, want 'expected two lists'", herr)
	}
}

// TestMatrixFromRowsErrorsWave3 pins matrixFromRows' malformed-data
// failures through MatrixUtil.create, plus the nil-list guard directly.
func TestMatrixFromRowsErrorsWave3(t *testing.T) {
	r := tensorRegistry(t)
	runTensorSrcErr(t, r, "MatrixUtil.create []", "empty list")
	runTensorSrcErr(t, r, "MatrixUtil.create [1 2]", "first element is not a list")
	runTensorSrcErr(t, r, "MatrixUtil.create [[][]]", "first row is empty")
	runTensorSrcErr(t, r, "MatrixUtil.create [[1 2][3]]", "row 1 has 1 elements, expected 2")
	runTensorSrcErr(t, r, "MatrixUtil.create [[1 2][3 'x']]", "")
	if _, err := matrixFromRows(native.NewTypeLiteral(native.TList)); err == nil ||
		!strings.Contains(err.Error(), "expected list of row lists") {
		t.Errorf("matrixFromRows(List literal): got %v", err)
	}
}

// TestTensorShapeSpecWave3 pins parseTensorShapeSpec through `refine`:
// each kind's happy path next to every malformed-spec compile failure (non-map
// spec, missing keys, non-integer and non-positive dimensions) — the
// error text carries the per-kind shapeSpecHint.
func TestTensorShapeSpecWave3(t *testing.T) {
	r := tensorRegistry(t)
	// Tensor {shape:[…]} constructs and instantiates.
	res, err := runTensorSrc(t, r,
		"def T222 (refine MatrixUtil.Tensor {shape:[2 2 2]}) make T222 [[[1 2][3 4]][[5 6][7 8]]]")
	if err != nil {
		t.Fatalf("shaped tensor make: %v", err)
	}
	if td := AsTensor(res[len(res)-1]); !shapeEqual(td.Shape, []int{2, 2, 2}) {
		t.Fatalf("shaped tensor = %v, want [2 2 2]", td.Shape)
	}
	// Mismatched data against the shaped Tensor type is declined.
	runTensorSrcErr(t, r,
		"def T22 (refine MatrixUtil.Tensor {shape:[2 2]}) make T22 [1 2 3 4]",
		"does not match")

	cases := []struct{ src, want string }{
		{"refine MatrixUtil.Matrix [1 2]", "expected a shape map {rows:R cols:C}"},
		{"refine MatrixUtil.Vector [1 2]", "expected a shape map {len:N}"},
		{"refine MatrixUtil.Tensor [1 2]", "expected a shape map {shape:[d0 d1 …]}"},
		{"refine MatrixUtil.Matrix {}", `missing "rows"`},
		{"refine MatrixUtil.Matrix {rows:2}", `missing "cols"`},
		{"refine MatrixUtil.Matrix {rows:'x' cols:2}", `"rows" must be an integer`},
		{"refine MatrixUtil.Matrix {rows:0 cols:2}", `"rows" must be a positive integer`},
		{"refine MatrixUtil.Vector {}", `missing "len"`},
		{"refine MatrixUtil.Vector {len:-3}", `"len" must be a positive integer`},
		{"refine MatrixUtil.Tensor {}", "expected a shape map {shape:[d0 d1 …]}"},
		{"refine MatrixUtil.Tensor {shape:5}", "shape must be a list of positive integers"},
		{"refine MatrixUtil.Tensor {shape:[]}", "at least one dimension"},
		{"refine MatrixUtil.Tensor {shape:['a']}", "dimension 0 must be an integer"},
		{"refine MatrixUtil.Tensor {shape:[0]}", "dimension 0 must be positive"},
	}
	for _, c := range cases {
		runTensorSrcErr(t, r, c.src, c.want)
	}
	// A shaped tensor type has no further subtyping.
	runTensorSrcErr(t, r,
		"refine (refine MatrixUtil.Matrix {rows:2 cols:2}) {rows:3 cols:3}",
		"no subtyping")
}

// TestTensorFromDataWave3 pins tensorFromData / vectorFromList /
// readFloatList / flattenNested edge cases through `make`.
func TestTensorFromDataWave3(t *testing.T) {
	r := tensorRegistry(t)
	cases := []struct{ src, want string }{
		{"make MatrixUtil.Tensor {shape:[2]}", "needs both a shape and a data key"},
		{"make MatrixUtil.Tensor {data:[1 2]}", "needs both a shape and a data key"},
		{"make MatrixUtil.Tensor {shape:'x' data:[1]}", "shape must be a list"},
		{"make MatrixUtil.Tensor {shape:[2] data:'x'}", "expected a list of numbers"},
		{"make MatrixUtil.Tensor {shape:[2 3] data:[1 2 3 4]}", "needs 6 elements, data has 4"},
		{"make MatrixUtil.Tensor {shape:[2] data:[1 'a']}", "element 1 is not a number"},
		{"make MatrixUtil.Tensor 5", "expected nested lists or a {shape data} map"},
		{"make MatrixUtil.Tensor []", "empty tensor"},
		{"make MatrixUtil.Tensor [1 'a']", "element 1 is not a number"},
		{"make MatrixUtil.Tensor [[1 2] 3]", "expected nested lists"},
		{"make MatrixUtil.Vector 5", "expected a list of numbers"},
		{"make MatrixUtil.Vector [1 'a']", "element 1 is not a number"},
	}
	for _, c := range cases {
		runTensorSrcErr(t, r, c.src, c.want)
	}
	// Rank-1 leaf via the Tensor kind (the flat-list branch).
	res, err := runTensorSrc(t, r, "make MatrixUtil.Tensor [3 4 5]")
	if err != nil {
		t.Fatal(err)
	}
	if td := AsTensor(res[0]); !shapeEqual(td.Shape, []int{3}) || td.Data[2] != 5 {
		t.Errorf("rank-1 tensor = %v %v", td.Shape, td.Data)
	}
}

// TestRegisterTensorIdealsNilWave3 pins the nil-registry guard.
func TestRegisterTensorIdealsNilWave3(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("registerTensorIdeals(nil) panicked: %v", rec)
		}
	}()
	registerTensorIdeals(nil, tt)
}

// TestMatrixHandlerTypeLiteralArgsWave3 drives every AsConcrete* guard in
// the matrix constructors and accessors with a type literal in each
// integer/number position — each errors, none panic (ADR-005). Signature
// dispatch shields these guards (a literal never matches a concrete
// Integer slot), so the handlers are invoked directly.
func TestMatrixHandlerTypeLiteralArgsWave3(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := MintTensorTypes(r)
	natives := matrixNatives(tt)
	intLit := native.NewTypeLiteral(native.TInteger)
	numLit := native.NewTypeLiteral(native.TNumber)
	i2 := native.NewInteger(2)
	mat := tensorValue(tt.Matrix, TensorData{Shape: []int{2, 2}, Data: []float64{1, 2, 3, 4}})

	cases := []struct {
		word string
		args []native.Value
	}{
		{"matrix-zeros", []native.Value{intLit, i2}},
		{"matrix-zeros", []native.Value{i2, intLit}},
		{"matrix-ones", []native.Value{intLit, i2}},
		{"matrix-ones", []native.Value{i2, intLit}},
		{"matrix-eye", []native.Value{intLit}},
		{"matrix-fill", []native.Value{intLit, i2, native.NewFloat(1)}},
		{"matrix-fill", []native.Value{i2, intLit, native.NewFloat(1)}},
		{"matrix-fill", []native.Value{i2, i2, numLit}},
		{"matrix-at", []native.Value{intLit, i2, mat}},
		{"matrix-at", []native.Value{i2, intLit, mat}},
		{"matrix-row", []native.Value{intLit, mat}},
		{"matrix-col", []native.Value{intLit, mat}},
		{"matrix-scale", []native.Value{numLit, mat}},
	}
	for _, c := range cases {
		n := findNativeWave3(t, natives, c.word)
		if _, herr := callHandlerWave3(t, n, r, c.args...); herr == nil {
			t.Errorf("%s with a type-literal arg: expected error, got nil", c.word)
		}
	}
}
