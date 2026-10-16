package modules

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// This file makes Tensor, Matrix and Vector first-class type-kinds —
// Ideals (see eng/go/ideal.go and design/IDEAL.10.md). Matrix
// and Vector refine Tensor: a Matrix is a rank-2 tensor, a Vector a
// rank-1 tensor. The kinds are registered when `boru:matrix` is
// imported, after which `type` constructs shaped tensor types and
// `make` instantiates them.

// TensorTypeInfo is a constructed (shaped) tensor type — the result of
// `type Matrix {rows:3 cols:3}`. Kind names the type-kind ("Tensor",
// "Matrix", "Vector"); Shape is the fixed shape every instance must
// match. It embeds core.HostTypeBody so the kernel's type machinery
// (IsTypeBody, TypeOf, InstallType) recognises a value carrying it,
// through ExtensionPayload, as a type rather than an instance.
type TensorTypeInfo struct {
	core.HostTypeBody
	Kind  string
	Shape []int
}

// tensorTypeInfo extracts a TensorTypeInfo from a constructed-type value.
func tensorTypeInfo(v native.Value) (TensorTypeInfo, bool) {
	if ep, ok := core.DataOf(&v).(core.ExtensionPayload); ok {
		ti, ok := ep.Body.(TensorTypeInfo)
		return ti, ok
	}
	return TensorTypeInfo{}, false
}

// registerTensorIdeals installs the Tensor, Matrix and Vector
// type-kinds into r.Ideals. Matrix and Vector refine Tensor, so
// disabling the Tensor kind disables the whole family. Called from
// BuildMatrixModule, so the kinds become available exactly when
// `boru:matrix` is imported — a host module dynamically extending the
// type system, the property the Ideal registry exists to provide.
func registerTensorIdeals(r *native.Registry, tt TensorModuleTypes) {
	if r == nil {
		return
	}
	tensor := &core.Ideal{
		Name:        "Tensor",
		Enabled:     true,
		Accepts:     tensorAccepts(tt.Tensor, "Tensor"),
		Construct:   tensorConstruct("Tensor", tt.Tensor),
		Instantiate: tensorInstantiate("Tensor", tt.Tensor),
	}
	matrix := &core.Ideal{
		Name:        "Matrix",
		Enabled:     true,
		Refines:     tensor,
		Accepts:     tensorAccepts(tt.Matrix, "Matrix"),
		Construct:   tensorConstruct("Matrix", tt.Matrix),
		Instantiate: tensorInstantiate("Matrix", tt.Matrix),
	}
	vector := &core.Ideal{
		Name:        "Vector",
		Enabled:     true,
		Refines:     tensor,
		Accepts:     tensorAccepts(tt.Vector, "Vector"),
		Construct:   tensorConstruct("Vector", tt.Vector),
		Instantiate: tensorInstantiate("Vector", tt.Vector),
	}
	r.Ideals.Register(tensor)
	r.Ideals.Register(matrix)
	r.Ideals.Register(vector)
}

// tensorAccepts builds the dispatch predicate for a tensor kind: it
// claims the bare kind literal and any constructed type of that kind.
// It deliberately does not claim instances — Accepts answers "is this
// a type of my kind", the question `type` and `make` ask of a target.
func tensorAccepts(vt *core.Type, kind string) func(native.Value) bool {
	return func(v native.Value) bool {
		if native.IsBareTypeNode(&v) {
			// A bare type literal IS its lattice node (by-value copy
			// post the type/value merge), so test the value's own
			// identity, not v.Parent (which is the supertype).
			return (&v).Equal(vt)
		}
		ti, ok := tensorTypeInfo(v)
		return ok && ti.Kind == kind
	}
}

// tensorConstruct is the Ideal.Construct body for a tensor kind — it
// builds a shaped type from `refine <kind> <shape-spec>`.
func tensorConstruct(kind string, vt *core.Type) func(base, arg native.Value, r *native.Registry) ([]native.Value, error) {
	return func(base, arg native.Value, r *native.Registry) ([]native.Value, error) {
		if core.DataOf(&base) != nil {
			return nil, r.BoruError("type_error",
				fmt.Sprintf("refine %s: a shaped tensor type has no subtyping — construct from the bare %s literal", kind, kind),
				"refine")
		}
		shape, err := parseTensorShapeSpec(kind, arg)
		if err != nil {
			return nil, r.BoruError("type_error", err.Error(), "refine")
		}
		return []native.Value{core.NewExtension(vt, TensorTypeInfo{Kind: kind, Shape: shape})}, nil
	}
}

// tensorInstantiate is the Ideal.Instantiate body for a tensor kind —
// it builds a concrete tensor from `make <type> <data>`, validating
// the data shape against the type's fixed shape when the target is a
// constructed (shaped) type.
func tensorInstantiate(kind string, vt *core.Type) func(typ, data native.Value, r *native.Registry) ([]native.Value, error) {
	return func(typ, data native.Value, _ *native.Registry) ([]native.Value, error) {
		td, err := buildTensor(kind, data)
		if err != nil {
			return nil, err
		}
		if ti, ok := tensorTypeInfo(typ); ok && len(ti.Shape) > 0 {
			if !shapeEqual(ti.Shape, td.Shape) {
				return nil, fmt.Errorf("make %s: data shape %s does not match type shape %s",
					kind, shapeString(td.Shape), shapeString(ti.Shape))
			}
		}
		return []native.Value{tensorValue(vt, td)}, nil
	}
}

// --- shape-spec parsing (for `type`) ---

// parseTensorShapeSpec reads the shape map for `type <kind> <arg>`:
// Matrix {rows:R cols:C}, Vector {len:N}, Tensor {shape:[d0 d1 …]}.
func parseTensorShapeSpec(kind string, arg native.Value) ([]int, error) {
	m, err := native.AsMap(arg)
	if err != nil || m == nil {
		return nil, fmt.Errorf("type %s: expected a shape map %s", kind, shapeSpecHint(kind))
	}
	switch kind {
	case "Matrix":
		rows, err := shapeDim(m, "rows", kind)
		if err != nil {
			return nil, err
		}
		cols, err := shapeDim(m, "cols", kind)
		if err != nil {
			return nil, err
		}
		return []int{rows, cols}, nil
	case "Vector":
		n, err := shapeDim(m, "len", kind)
		if err != nil {
			return nil, err
		}
		return []int{n}, nil
	default: // Tensor
		dimsV, ok := m.Get("shape")
		if !ok {
			return nil, fmt.Errorf("type Tensor: expected a shape map %s", shapeSpecHint(kind))
		}
		return readDimList(dimsV)
	}
}

func shapeSpecHint(kind string) string {
	switch kind {
	case "Matrix":
		return "{rows:R cols:C}"
	case "Vector":
		return "{len:N}"
	default:
		return "{shape:[d0 d1 …]}"
	}
}

// shapeDim reads one positive-integer dimension from a shape map.
func shapeDim(m native.ReadMap, key, kind string) (int, error) {
	v, ok := m.Get(key)
	if !ok {
		return 0, fmt.Errorf("type %s: missing %q in the shape spec %s", kind, key, shapeSpecHint(kind))
	}
	n, err := native.AsInteger(v)
	if err != nil {
		return 0, fmt.Errorf("type %s: %q must be an integer", kind, key)
	}
	if n <= 0 {
		return 0, fmt.Errorf("type %s: %q must be a positive integer, got %d", kind, key, n)
	}
	return int(n), nil
}

// readDimList reads a list of positive integers as a tensor shape.
func readDimList(v native.Value) ([]int, error) {
	lst, _ := native.AsList(v)
	if lst.IsNil() {
		return nil, fmt.Errorf("type Tensor: shape must be a list of positive integers")
	}
	if lst.Len() == 0 {
		return nil, fmt.Errorf("type Tensor: shape must have at least one dimension")
	}
	dims := make([]int, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		n, err := native.AsInteger(lst.Get(i))
		if err != nil {
			return nil, fmt.Errorf("type Tensor: shape dimension %d must be an integer", i)
		}
		if n <= 0 {
			return nil, fmt.Errorf("type Tensor: shape dimension %d must be positive, got %d", i, n)
		}
		dims[i] = int(n)
	}
	return dims, nil
}

// --- tensor construction from data (for `make`) ---

// buildTensor builds a TensorData from the source data of `make`. Each
// kind reads its natural data form; Tensor additionally accepts the
// explicit {shape data} map.
func buildTensor(kind string, data native.Value) (TensorData, error) {
	switch kind {
	case "Vector":
		return vectorFromList(data)
	case "Matrix":
		return matrixFromRows(data)
	default: // Tensor
		return tensorFromData(data)
	}
}

// vectorFromList builds a rank-1 tensor from a flat list of numbers.
func vectorFromList(data native.Value) (TensorData, error) {
	nums, err := readFloatList(data, "make Vector")
	if err != nil {
		return TensorData{}, err
	}
	return TensorData{Shape: []int{len(nums)}, Data: nums}, nil
}

// tensorFromData builds a tensor of any rank — from an explicit
// {shape:[…] data:[…]} map, or by inferring the shape from nested
// lists.
func tensorFromData(data native.Value) (TensorData, error) {
	if m, err := native.AsMap(data); err == nil && m != nil {
		shapeV, hasShape := m.Get("shape")
		dataV, hasData := m.Get("data")
		if hasShape || hasData {
			if !hasShape || !hasData {
				return TensorData{}, fmt.Errorf("make Tensor: the map form needs both a shape and a data key")
			}
			shape, err := readDimList(shapeV)
			if err != nil {
				return TensorData{}, err
			}
			flat, err := readFloatList(dataV, "make Tensor")
			if err != nil {
				return TensorData{}, err
			}
			want := 1
			for _, d := range shape {
				want *= d
			}
			if want != len(flat) {
				return TensorData{}, fmt.Errorf("make Tensor: shape %s needs %d elements, data has %d",
					shapeString(shape), want, len(flat))
			}
			return TensorData{Shape: shape, Data: flat}, nil
		}
	}
	shape, flat, err := flattenNested(data)
	if err != nil {
		return TensorData{}, err
	}
	return TensorData{Shape: shape, Data: flat}, nil
}

// readFloatList reads a list of numbers as a []float64.
func readFloatList(v native.Value, ctx string) ([]float64, error) {
	lst, _ := native.AsList(v)
	if lst.IsNil() {
		return nil, fmt.Errorf("%s: expected a list of numbers", ctx)
	}
	nums := make([]float64, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		n, err := native.AsNumber(lst.Get(i))
		if err != nil {
			return nil, fmt.Errorf("%s: element %d is not a number", ctx, i)
		}
		nums[i] = n
	}
	return nums, nil
}

// flattenNested infers a tensor shape from nested lists and flattens
// the data into row-major order. Sub-tensors must all share a shape —
// ragged data is an error.
func flattenNested(v native.Value) ([]int, []float64, error) {
	lst, _ := native.AsList(v)
	if lst.IsNil() {
		return nil, nil, fmt.Errorf("make Tensor: expected nested lists or a {shape data} map")
	}
	if lst.Len() == 0 {
		return nil, nil, fmt.Errorf("make Tensor: empty tensor")
	}
	// A list of numbers is a rank-1 leaf.
	if _, err := native.AsNumber(lst.Get(0)); err == nil {
		nums, err := readFloatList(v, "make Tensor")
		if err != nil {
			return nil, nil, err
		}
		return []int{len(nums)}, nums, nil
	}
	// Otherwise a list of sub-tensors — recurse; shapes must agree.
	var subShape []int
	var flat []float64
	for i := 0; i < lst.Len(); i++ {
		s, d, err := flattenNested(lst.Get(i))
		if err != nil {
			return nil, nil, err
		}
		if i == 0 {
			subShape = s
		} else if !shapeEqual(s, subShape) {
			return nil, nil, fmt.Errorf("make Tensor: ragged data — sub-tensor %d has shape %s, want %s",
				i, shapeString(s), shapeString(subShape))
		}
		flat = append(flat, d...)
	}
	return append([]int{lst.Len()}, subShape...), flat, nil
}

// shapeEqual reports whether two shapes are element-wise equal.
func shapeEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
