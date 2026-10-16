package compiler

import "github.com/boru-lang/boru/core/go"

// LiteralBuild is how a fn unit constructs a compound body literal on each
// evaluation (OpPushConstFresh, OpPushConstFreshLocal, an island's seated
// literal): construction from shared parts, the per-evaluation spine the
// interpreter builds when it evaluates the literal, in place of a copy of
// the pooled value (design/VALUE-NO-COPY.0.md §6 phase 3; exceptions B1).
// A part that is not the literal's own construction — a scalar, a token, a
// type literal, a nested fn value, an enclosing binding's container the
// literal embeds (Program.ConstKeep's members) — is shared as it is.
type LiteralBuild struct {
	Kind LiteralBuildKind
	// Val is the shared part (BuildShared), the fn literal (BuildFn), or the
	// container whose kind and facets the construction takes (BuildList,
	// BuildMap, BuildCopy) — read where it lives: a pooled const, an element
	// of one, never written.
	Val *core.Value
	// Parts are a list's elements, or a map's values in key order.
	Parts []LiteralBuild
	// Keys are a map's keys in order; Implicit and Meta its OrderedMap's own
	// marks, carried by every construction.
	Keys     []string
	Implicit bool
	Meta     map[string]any
}

// LiteralBuildKind is what a LiteralBuild constructs.
type LiteralBuildKind uint8

const (
	// BuildShared is Val itself, shared.
	BuildShared LiteralBuildKind = iota
	// BuildList is a new list over the constructed Parts.
	BuildList
	// BuildMap is a new map of Keys to the constructed Parts.
	BuildMap
	// BuildFn is a new function per evaluation sharing Val's definition
	// (NUR288; exceptions B4): only the literal itself, never a nested fn.
	BuildFn
	// BuildCopy is an independent copy of a mutable container that is not a
	// plain list or map (a flex container, a store, an instance, a table, an
	// error's data, a host body) baked into a literal: the interpreter's
	// evaluation constructs a new one, and so does this, through the cloner.
	BuildCopy
)

// planLiteralBuild is the construction of the fresh literal v whose
// embedded enclosing-binding containers (by ID) are keep.
func planLiteralBuild(v *core.Value, keep map[string]bool) LiteralBuild {
	if _, ok := core.FnDefOf(v.Data()); ok {
		return LiteralBuild{Kind: BuildFn, Val: v}
	}
	return planLiteralPart(v, keep)
}

func planLiteralPart(v *core.Value, keep map[string]bool) LiteralBuild {
	if id := core.IDOf(v); id != "" && keep[id] {
		return LiteralBuild{Kind: BuildShared, Val: v}
	}
	switch d := v.Data().(type) {
	case core.ListPayload:
		parts := make([]LiteralBuild, len(d.Elems))
		for i := range d.Elems {
			parts[i] = planLiteralPart(&d.Elems[i], keep)
		}
		return LiteralBuild{Kind: BuildList, Val: v, Parts: parts}
	case core.MapPayload:
		b := LiteralBuild{Kind: BuildMap, Val: v}
		if d.M == nil {
			return b
		}
		b.Keys = d.M.Keys()
		b.Implicit, b.Meta = d.M.Implicit, d.M.Meta
		b.Parts = make([]LiteralBuild, len(b.Keys))
		for i, k := range b.Keys {
			b.Parts[i] = planLiteralPart(mapValueRef(d.M, k), keep)
		}
		return b
	}
	if core.IsMutablePayload(v.Data()) {
		return LiteralBuild{Kind: BuildCopy, Val: v}
	}
	return LiteralBuild{Kind: BuildShared, Val: v}
}

// mapValueRef is where a plan reads the value m holds under k: an
// OrderedMap keeps its values by value, so the plan takes the one copy a
// read makes and holds it from then on.
func mapValueRef(m *core.OrderedMap, k string) *core.Value {
	mv, _ := m.Get(k)
	return &mv
}

// planConstBuilds gives every const a fresh push constructs its build:
// the OpPushConstFresh sites in every unit and the OpPushConstFreshLocal
// seats (Program.ConstLocals), each from its const and the embedded members
// it keeps (Program.ConstKeep).
func planConstBuilds(p *Program) {
	need := map[int]bool{}
	mark := func(code []Instr) {
		for _, in := range code {
			if in.Op == OpPushConstFresh {
				need[int(in.Arg)] = true
			}
		}
	}
	mark(p.Code)
	for i := range p.Fns {
		mark(p.Fns[i].Code)
	}
	for _, cl := range p.ConstLocals {
		need[cl.ConstIdx] = true
	}
	if len(need) == 0 {
		return
	}
	p.ConstBuilds = make([]*LiteralBuild, len(p.Consts))
	for idx := range need {
		if idx < 0 || idx >= len(p.Consts) {
			continue
		}
		b := planLiteralBuild(&p.Consts[idx], p.ConstKeep[idx])
		p.ConstBuilds[idx] = &b
	}
}
