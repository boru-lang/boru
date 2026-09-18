package core

import (
	"fmt"
	"strings"
)

// Generic types — schemas, type parameters, instantiation
// (design/legacy/GENERICS.10.ignore; pinned decisions in the approved plan).
//
//	def Box gen [T] class {value:T}
//	def b:(Box of [Integer]) {value:42}
//
// `gen [...]` produces a GenSpec (Type/GenSpec) and pushes one
// PLACEHOLDER type binding per parameter (D1) so the downstream
// constructor body resolves `T` while it builds. A GenSpec-aware
// constructor overload (refine/class/fnsig/fn — D2) consumes the spec,
// builds its body with the placeholders live, pops them, and returns a
// TypeSchema. `def` installs the schema as a type binding (InstallType
// branch). `of` instantiates: constraint-checks the args (D7),
// memoises per (schema, canonical args) via a hidden Defs binding —
// the const-interning pattern — and mints ONE lattice node per
// instantiation as a child of the schema node (D4).

// GenParam is one declared type parameter of a generic schema.
type GenParam struct {
	Name       string
	Bound      Value // `extends` constraint; meaningful iff HasBound
	HasBound   bool
	Default    Value // default type expression; may carry placeholders (D8, lazy)
	HasDefault bool
	Pos        SrcPos
	Node       *Type // the minted placeholder node (Type/TypeParam or the bound's child)
}

// GenSpecInfo is the payload of the value `gen [...]` produces. Bound
// records the names currently pushed as placeholder type bindings in
// the registry (D1): the GenSpec-consuming constructor pops exactly
// these after building its body; the end-of-Run drain pops them for
// an orphan spec.
type GenSpecInfo struct {
	Params []GenParam
	Bound  []string
}

// SchemaKind discriminates what the schema body is, so `of` knows how
// to rebuild the instantiated type value.
type SchemaKind uint8

const (
	SchemaRecord SchemaKind = iota // refine Record [...]
	SchemaClass                    // class {...}
	SchemaFnSig                    // fnsig [[...] [...]]
	SchemaFn                       // fn [[...] [...] [...]] (Phase 4)
)

// TypeSchemaInfo is the payload of an installed generic schema (a
// pointer payload shared between the def-stack body and the minted
// node's Behavior, like *SurfaceInfo). Body holds the constructed
// type value with placeholder references embedded; instantiation
// memoisation is registry-local via hidden Defs bindings (D4), NOT on
// this struct — the same registry-locality discipline surfaces use
// for their conformance sets.
type TypeSchemaInfo struct {
	Name   string // full type path (e.g. "Class/Box"); set by InstallType
	Params []GenParam
	Body   Value
	Kind   SchemaKind
	Type   *Type // canonical minted schema node; set by InstallType
}

// GenInstRef is a DEFERRED instantiation: `Self of [T]` inside a
// schema body (recursion, D5) or any `of` whose args still contain
// placeholders (F-bounds). It is resolved — turned into a real
// instantiation — when the enclosing schema is itself instantiated
// with concrete arguments.
type GenInstRef struct {
	Head Value // the schema literal, or a Self/TypeParam-bearing reference
	Args []Value
}

// ---- constructors / predicates ----

// NewGenSpec wraps a *GenSpecInfo as a Type/GenSpec value.
func NewGenSpec(info *GenSpecInfo) Value {
	return NewValueRaw(TGenSpec, info)
}

// IsGenSpec reports whether v carries a generic-parameter spec.
func IsGenSpec(v Value) bool {
	_, ok := v.Data.(*GenSpecInfo)
	return ok
}

// AsGenSpec returns the shared spec payload.
func AsGenSpec(v Value) (*GenSpecInfo, error) {
	info, ok := v.Data.(*GenSpecInfo)
	if !ok {
		return nil, fmt.Errorf("AsGenSpec: not a gen spec value (got %T)", v.Data)
	}
	return info, nil
}

// NewGenParamValue wraps a GenParam as a Type/GenParam value (the
// result of `extends` / `default` inside a gen list entry).
func NewGenParamValue(p GenParam) Value {
	return NewValueRaw(TGenParam, p)
}

// IsGenParamValue reports whether v carries a single parameter spec.
func IsGenParamValue(v Value) bool {
	_, ok := v.Data.(GenParam)
	return ok
}

// AsGenParamValue returns the parameter spec.
func AsGenParamValue(v Value) (GenParam, error) {
	p, ok := v.Data.(GenParam)
	if !ok {
		return GenParam{}, fmt.Errorf("AsGenParamValue: not a gen param value (got %T)", v.Data)
	}
	return p, nil
}

// NewTypeSchema wraps a *TypeSchemaInfo as a value parented at the
// given node (the minted schema node after InstallType).
func NewTypeSchema(t *Type, info *TypeSchemaInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// IsTypeSchema reports whether v carries a generic schema payload.
func IsTypeSchema(v Value) bool {
	_, ok := v.Data.(*TypeSchemaInfo)
	return ok
}

// AsTypeSchema returns the shared schema payload pointer.
func AsTypeSchema(v Value) (*TypeSchemaInfo, error) {
	info, ok := v.Data.(*TypeSchemaInfo)
	if !ok {
		return nil, fmt.Errorf("AsTypeSchema: not a type schema value (got %T)", v.Data)
	}
	return info, nil
}

// NewGenInstRef wraps a deferred instantiation reference. Parented at
// TType: the ref denotes a type-to-be (resolved when the enclosing
// schema is instantiated with concrete arguments).
func NewGenInstRef(ref GenInstRef) Value {
	return NewValueRaw(TType, ref)
}

// IsGenInstRef reports whether v is a deferred instantiation.
func IsGenInstRef(v Value) bool {
	_, ok := v.Data.(GenInstRef)
	return ok
}

// AsGenInstRef returns the deferred instantiation payload.
func AsGenInstRef(v Value) (GenInstRef, error) {
	ref, ok := v.Data.(GenInstRef)
	if !ok {
		return GenInstRef{}, fmt.Errorf("AsGenInstRef: not a gen instantiation ref (got %T)", v.Data)
	}
	return ref, nil
}

// SetPendingGen parks a spec for the next type constructor. Errors
// when one is already pending (two `gen`s with no constructor between
// them — certainly a bug).
func (r *Registry) SetPendingGen(spec *GenSpecInfo) error {
	if r.pendingGen != nil {
		return &BoruError{
			Code:   "gen_without_constructor",
			Detail: "gen: the previous gen [...] was not consumed by a type constructor",
			Hint:   "follow gen [...] with refine Record [...], class {...}, fnsig [...], or fn [...]",
		}
	}
	r.pendingGen = spec
	return nil
}

// TakePendingGen consumes the pending spec (nil when none) — called
// by the type constructors at handler entry.
func (r *Registry) TakePendingGen() *GenSpecInfo {
	spec := r.pendingGen
	r.pendingGen = nil
	return spec
}

// PendingGen reports the pending spec without consuming it (the
// end-of-Run orphan drain).
func (r *Registry) PendingGen() *GenSpecInfo {
	return r.pendingGen
}

// SuspendPendingGen removes the pending spec for the duration of a
// nested evaluation and returns a restore func. execMatch suspends
// around argument auto-evaluation so a constructor nested INSIDE an
// argument (a record field's `(fnsig […])`) cannot steal the spec
// destined for the word being dispatched (the outer refine/class).
func (r *Registry) SuspendPendingGen() func() {
	spec := r.pendingGen
	r.pendingGen = nil
	restored := false
	// Restore-once: the caller restores explicitly BEFORE its handler
	// runs (the handler is the intended consumer) and also defers the
	// call for early error returns; the second invocation is a no-op
	// so a spec the handler consumed is never resurrected.
	return func() {
		if restored {
			return
		}
		restored = true
		if spec != nil {
			r.pendingGen = spec
		}
	}
}

// ---- placeholder nodes ----

// genParamUnifier is the Behavior installed on a minted type-parameter
// placeholder node. Two questions, two answers (§9.4 strict rule):
//
//   - body-internal identity: a value/carrier whose declared type IS
//     the placeholder (or a descendant) matches the placeholder slot;
//   - call-site admission: any other value matches iff it satisfies
//     the parameter's BOUND (`extends C` = Is-membership in C, D7);
//     an unconstrained parameter admits anything (extends Any).
type genParamUnifier struct {
	behaviorWrapper
	param GenParam
}

func (*genParamUnifier) ContentMembership() {}

func (g *genParamUnifier) Match(v Value, t *Type) bool {
	return g.matchR(v, t, nil)
}

// matchR is Match with the enclosing unify chain's registry threaded
// into the bound check (see isR): a placeholder reached from inside an
// armed unify (`xs:[:T]` element by element) keeps the chain armed for
// a compound bound's own walk.
func (g *genParamUnifier) matchR(v Value, t *Type, r *Registry) bool {
	if v.Parent != nil && v.Parent.ConformsTo(t) {
		return true
	}
	if !g.param.HasBound {
		return true
	}
	// Bound membership routes through the bound's own Behavior (the
	// v.Is doctrine): lattice bounds, predicates, disjuncts,
	// negations, and surfaces all answer uniformly.
	if bnode := boundNode(g.param.Bound); bnode != nil {
		return isR(v, bnode, r)
	}
	_, uerr := unifyWithin(v, g.param.Bound, r)
	return uerr == nil
}

func (g *genParamUnifier) Format(v Value) string {
	if IsBareTypeNode(v) {
		return g.param.Name
	}
	return baseBehavior(g.prev).Format(v)
}

// boundNode extracts the lattice node a bound value denotes, when it
// denotes one directly: a bare type literal IS its node; an object
// type / surface carries its minted node in the payload. Compound
// bounds (disjuncts, depscalars) return nil and route through Unify.
func boundNode(bound Value) *Type {
	switch {
	case IsBareTypeNode(bound):
		return &bound
	case IsClassType(bound):
		oi, _ := AsClassType(bound)
		return oi.Type
	case IsSurfaceType(bound):
		si, _ := AsSurfaceType(bound)
		return si.Type
	}
	return nil
}

// MintTypeParam mints a placeholder node for a parameter (D9): the
// lattice parent is the bound's node when the bound denotes one (so
// e.g. a surface-bounded placeholder's parent chain reaches the
// surface and checkModeSurfaceShape fires), otherwise Type/TypeParam.
func MintTypeParam(r *Registry, p GenParam) *Type {
	parent := TTypeParam
	if p.HasBound {
		if bnode := boundNode(p.Bound); bnode != nil {
			parent = CanonicalType(r, bnode)
		}
	}
	node := r.Types.MintType(p.Name, parent)
	node.ensureTMeta().Behavior = &genParamUnifier{behaviorWrapper: behaviorWrapper{prev: node.Behavior()}, param: p}
	return node
}

// AttachGenBound finalises a provisionally-minted placeholder node
// once its gen-list entry has evaluated: the bound/default land on the
// unifier, and the node reparents at the bound's node when the bound
// denotes one (D9 — so a surface-bounded placeholder's parent chain
// reaches the surface). Safe pre-publication: while the gen list is
// still being walked nothing outside it holds the node.
func AttachGenBound(r *Registry, node *Type, p GenParam) {
	if g, ok := node.Behavior().(*genParamUnifier); ok {
		g.param = p
	}
	if p.HasBound {
		if bn := boundNode(p.Bound); bn != nil {
			node.Parent = CanonicalType(r, bn)
		}
	}
}

// IsTypeParamNode reports whether t is a minted type-parameter
// placeholder (detection via the installed Behavior, like
// SurfaceInfoOf for surfaces).
func IsTypeParamNode(t *Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Behavior().(*genParamUnifier)
	return ok
}

// TypeParamName returns the parameter name of a placeholder node, or
// "" when t is not one.
func TypeParamName(t *Type) string {
	if t == nil {
		return ""
	}
	if g, ok := t.Behavior().(*genParamUnifier); ok {
		return g.param.Name
	}
	return ""
}

// IsUnconstrainedTypeParam reports whether t is a generic type-PARAMETER with
// no `extends` bound (a plain `gen [T]`, not `gen [(T extends C)]`). Such a
// parameter admits a value of ANY type, so — like an explicit `Any` — a value
// of that type is statically unknown and a body word over it must match
// gradually rather than fail against a strict abstract carrier. A bounded
// parameter is analysed at its bound and stays strict.
func IsUnconstrainedTypeParam(t *Type) bool {
	if t == nil {
		return false
	}
	g, ok := t.Behavior().(*genParamUnifier)
	return ok && !g.param.HasBound
}

// schemaUnifier is the Behavior installed on a minted SCHEMA node.
// Matching needs nothing beyond the default lattice walk — an
// instantiation node is a child of the schema node, so instances
// conform to the bare schema by plain ancestry (D3: bare `Box` means
// "any instantiation of Box"). The Behavior exists for rendering and
// schema-node detection.
type schemaUnifier struct {
	behaviorWrapper
	info *TypeSchemaInfo
}

func (*schemaUnifier) ContentMembership() {}

func (s *schemaUnifier) Match(v Value, t *Type) bool { return baseBehavior(s.prev).Match(v, t) }

func (s *schemaUnifier) Format(v Value) string {
	if IsTypeSchema(v) {
		names := make([]string, len(s.info.Params))
		for i, p := range s.info.Params {
			names[i] = p.Name
		}
		return fmt.Sprintf("schema<%s>[%s]", s.info.Name, strings.Join(names, " "))
	}
	return baseBehavior(s.prev).Format(v)
}

// InstallSchemaUnifier attaches a schemaUnifier to a minted schema
// node. Called by InstallType's TypeSchema branch.
func InstallSchemaUnifier(def *Type, info *TypeSchemaInfo) {
	def.ensureTMeta().Behavior = &schemaUnifier{behaviorWrapper: behaviorWrapper{prev: def.Behavior()}, info: info}
}

// SchemaInfoOf returns the schema payload when t is a minted schema
// node (Behavior detection, like SurfaceInfoOf).
func SchemaInfoOf(t *Type) (*TypeSchemaInfo, bool) {
	if t == nil {
		return nil, false
	}
	if s, ok := t.Behavior().(*schemaUnifier); ok {
		return s.info, true
	}
	return nil, false
}
