package core

import (
	"fmt"
	"strings"
)

// SurfaceInfo is the payload of a surface type — a pure contract: a
// named set of required operation shapes with no bodies and no state
// (design/legacy/SURFACES.10.ignore). Required maps each operation name to its
// FnUndef shape Value (the `fnsig` form), with TSelf marking the
// positions the conforming type must occupy.
//
// Conform is the post-hoc conformance set `exposes` fills in: the
// canonical lattice IDs of every type that passed the completeness
// check. The payload is held by POINTER (like *StoreInstanceInfo) so
// an `exposes` after minting is visible through every copy of the
// surface's type literal — the same shared-payload discipline the
// canonical-pointer rule covers for Behaviors.
type SurfaceInfo struct {
	Name     string          // full type path (e.g. "Surface/Shape"); set by InstallType
	Required *OrderedMap     // operation name → FnUndef shape Value
	Conform  map[string]bool // canonical *Type IDs that expose this surface
	Type     *Type           // canonical minted node; set by InstallType
}

// NewSurfaceType wraps a *SurfaceInfo as a Value parented at the
// given node (TSurface for an anonymous surface; the minted subtype
// after InstallType).
func NewSurfaceType(t *Type, info *SurfaceInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// IsSurfaceType reports whether v carries a surface payload.
func IsSurfaceType(v Value) bool {
	_, ok := v.Data.(*SurfaceInfo)
	return ok
}

// AsSurfaceType returns the surface payload pointer. The pointer is
// shared across every copy of the surface's type literal — mutating
// Conform through it is how `exposes` registers conformance.
func AsSurfaceType(v Value) (*SurfaceInfo, error) {
	info, ok := v.Data.(*SurfaceInfo)
	if !ok {
		return nil, fmt.Errorf("AsSurfaceType: not a surface type value (got %T)", v.Data)
	}
	return info, nil
}

// SubstituteSelf returns a copy of spec with every Self position
// (param or return) replaced by node — the concrete conformance
// question `exposes` asks of a candidate type.
func SubstituteSelf(spec FnSigSpec, node *Type) FnSigSpec {
	out := FnSigSpec{
		Params:  make([]FnParam, len(spec.Params)),
		Returns: make([]*Type, len(spec.Returns)),
	}
	for i, p := range spec.Params {
		if p.Type != nil && p.Type.ID == TSelf.ID {
			p.Type = node
		}
		out.Params[i] = p
	}
	for i, t := range spec.Returns {
		if t != nil && t.ID == TSelf.ID {
			t = node
		}
		out.Returns[i] = t
	}
	return out
}

// unifySurface is the unifyInner fold for operands involving a surface
// body. Three cases:
//   - surface vs surface: same contract (shared payload) unifies to
//     itself; distinct surfaces have no representable intersection.
//   - surface vs concrete value: membership — the value's declared
//     type must be in the conformance set (via the node's Behavior).
//   - surface vs abstract type: containment — the type unifies (to
//     itself, the narrower side) iff its parent chain reaches a
//     conforming node, e.g. `Circle tand Shape` → Circle when Circle
//     exposes Shape.
func unifySurface(a, b Value) (Value, *UnifyError) {
	surf, other := a, b
	if !IsSurfaceType(surf) {
		surf, other = b, a
	}
	info, _ := AsSurfaceType(surf)
	if info == nil || info.Type == nil {
		return Value{}, unifyFail("surface type has no minted node (declare it with def)", a, b)
	}
	if oinfo, err := AsSurfaceType(other); err == nil {
		if oinfo == info {
			return surf, nil
		}
		return Value{}, unifyFail("distinct surfaces do not unify", a, b)
	}
	if IsConcrete(other) {
		if other.Is(info.Type) {
			return other, nil
		}
		return Value{}, unifyFail("value's type does not expose the surface", a, b)
	}
	var node *Type
	switch {
	case IsClassType(other):
		oi, _ := AsClassType(other)
		node = oi.Type
	case IsBareTypeNode(other):
		node = &other
	case other.Carrier:
		// A check-mode carrier denotes its Parent type — so
		// `def x:Shape (make Circle {…})` unifies statically when
		// Circle exposes Shape.
		node = other.Parent
	}
	for p := node; p != nil; p = p.Parent {
		if info.Conform[p.ID] {
			return other, nil
		}
	}
	return Value{}, unifyFail("type does not expose the surface", a, b)
}

// surfaceUnifier is the kernel-installed Unifier for surface types.
// Membership is EXPLICIT: a value matches a surface iff some type on
// its parent chain is in the surface's conformance set (so subclass
// instances of an exposer conform). No structural duck typing — a
// type that happens to have every operation but never said `exposes`
// is not a member. Same dispatch shape as disjunct/negation/depscalar
// unifiers: installed on the minted lattice node so every Is/Match
// site consults it.
type surfaceUnifier struct {
	behaviorWrapper              // prev Behavior + Equal/Compare delegation
	info            *SurfaceInfo // shared payload — sees post-hoc `exposes` updates
	typeName        string
}

// ContentMembership marks a surface as a CONTENT-based type for the
// ownership-anchored extension rule (design/OPEN-WORDS.1.md §3.1): a
// surface's membership is decided by `exposes` declarations that a type
// can make at ANY time — the conformance set (s.info.Conform) is shared
// and sees post-hoc updates — so a value that predates the surface can
// start matching it. Anchoring a core-word extension on a surface would
// therefore let the merge capture pre-existing calls, breaking the
// reachability theorem exactly as a predicate/union anchor would. So a
// surface can never be a nominal anchor (IsNominalAnchor excludes it),
// even though its membership is nominal-flavoured (explicit `exposes`,
// not structural duck typing).
func (*surfaceUnifier) ContentMembership() {}

// Match walks v's declared-type parent chain against the conformance
// set. Type literals pass through to the default walk — a bare
// Shape-literal is "the type itself", not an inhabitant.
func (s *surfaceUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(s.prev).Match(v, t)
	}
	for p := v.Parent; p != nil; p = p.Parent {
		if s.info.Conform[p.ID] {
			return true
		}
	}
	return false
}

// Unify admits a candidate — concrete value or check-mode carrier —
// against the surface node by the same conformance-set walk Match
// runs, yielding the CANDIDATE. The membership question applies when
// exactly one operand is the bare surface node; two type-level or two
// value-level operands defer to the structural rule. The §N3 Unify
// capability (design/legacy/TYPE-REPRESENTATION.1.ignore): a surface-typed def
// (`def x:Shape (make Circle …)`) unifies against the NODE the name
// now denotes, where it used to unify against the body via the
// surface fold.
func (s *surfaceUnifier) Unify(a, b Value) (Value, *UnifyError) {
	// The conformance rule decides whenever exactly one side IS this
	// surface's node — the candidate may be a concrete value, a
	// carrier, or a TYPE-level operand (a generic bound check:
	// `gen [(T extends Shape)] … of [Circle]` pairs the Shape node
	// with the Circle node), exactly the cases the surface fold
	// decided for the body. A same-node (or node-free) pair settles
	// structurally.
	aSelf := IsBareTypeNode(a) && a.Behavior() == TypeBehavior(s)
	bSelf := IsBareTypeNode(b) && b.Behavior() == TypeBehavior(s)
	if aSelf == bSelf {
		return unifySameOrSubtype(a, b)
	}
	node, candidate := a, b
	if bSelf {
		node, candidate = b, a
	}
	if IsBareTypeNode(candidate) {
		// Type-level containment: the candidate NODE (or an ancestor)
		// must be in the conformance set — the literal-vs-literal walk
		// unifySurface runs for the body.
		cn := &candidate
		for p := cn; p != nil; p = p.Parent {
			if s.info.Conform[p.ID] {
				return candidate, nil
			}
		}
		return Value{}, unifyFail("type does not expose surface "+s.typeName, a, b)
	}
	if s.Match(candidate, &node) {
		return candidate, nil
	}
	return Value{}, unifyFail("value does not expose surface "+s.typeName, a, b)
}

func (s *surfaceUnifier) Format(v Value) string {
	// The surface body renders as its contract: name + operation set.
	if info, err := AsSurfaceType(v); err == nil {
		ops := make([]string, 0, len(info.Required.Keys()))
		ops = append(ops, info.Required.Keys()...)
		return fmt.Sprintf("surface<%s>{%s}", info.Name, strings.Join(ops, " "))
	}
	return baseBehavior(s.prev).Format(v)
}

// SurfaceInfoOf returns the surface payload when t (or an ancestor)
// is a surface-minted node — i.e. carries the surfaceUnifier. Used by
// the check-mode dispatch path (S2) to type a required operation on a
// surface-typed carrier via the contract's shape.
func SurfaceInfoOf(t *Type) (*SurfaceInfo, bool) {
	for p := t; p != nil; p = p.Parent {
		if su, ok := p.Behavior().(*surfaceUnifier); ok {
			return su.info, true
		}
	}
	return nil, false
}

// installSurfaceUnifier attaches a surfaceUnifier to def, wrapping any
// existing Behavior. Called by InstallType when minting a surface type.
func installSurfaceUnifier(def *Type, info *SurfaceInfo, name string) {
	def.ensureTMeta().Behavior = &surfaceUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		info:            info,
		typeName:        name,
	}
}
