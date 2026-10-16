package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// buildLiteral constructs a compound body literal from its shared parts, as
// its LiteralBuild says (compiler/go/literal_build.go): a new container per
// evaluation around the constructed or shared parts — the per-evaluation
// spine the interpreter builds — never a copy of the pooled value.
func buildLiteral(b *compiler.LiteralBuild) core.Value {
	switch b.Kind {
	case compiler.BuildList:
		elems := make([]core.Value, len(b.Parts))
		for i := range b.Parts {
			elems[i] = buildLiteral(&b.Parts[i])
		}
		return core.MintLike(b.Val, core.ListPayload{Elems: elems})
	case compiler.BuildMap:
		if b.Keys == nil {
			return core.MintLike(b.Val, core.MapPayload{})
		}
		m := core.NewOrderedMap()
		m.Implicit = b.Implicit
		for i, k := range b.Keys {
			m.Set(k, buildLiteral(&b.Parts[i]))
		}
		if b.Meta != nil {
			m.Meta = make(map[string]any, len(b.Meta))
			for k, mv := range b.Meta {
				m.Meta[k] = mv
			}
		}
		return core.MintLike(b.Val, core.MapPayload{M: m})
	case compiler.BuildFn:
		return core.WithFreshFnIdentity(*b.Val)
	case compiler.BuildCopy:
		return core.CloneValue(*b.Val)
	}
	return *b.Val
}
