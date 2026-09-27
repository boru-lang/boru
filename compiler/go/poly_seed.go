package compiler

import core "github.com/boru-lang/boru/core/go"

// The seeded poly pick (PolyRef.Seed). A CALL_NATIVE_POLY site re-matches at
// run time because the checker could not commit one overload for every
// value the site may see. For two common shapes the checker's own pick holds
// under a guard the VM can test without matching:
//
//   - the operands were all strict (only the RESULT was gradual — the
//     census's "dynamic-out-only" reason, e.g. `m.a` over a Map param): the
//     word's overloads are tag-determined (core.TagDeterminedSigs), so a
//     runtime window whose tags EQUAL the checked operands' picks exactly the
//     checked overload — a strict carrier of tag T matches every slot as a
//     concrete value of tag T does;
//   - the word has ONE overload of this arity (`is`, `eq`): the first match is
//     that overload whenever it matches at all.
//
// The guard failing is not an error: the VM takes the ordinary re-match.

// polySeed is the checker's pick for one poly call, and the operand tags it
// was proven for (nil: the pick is the only overload of its arity).
type polySeed struct {
	sig  *core.Signature
	tags []*core.Type
}

// polySeedFor derives the seed for a poly dispatch of sig (a member of sigs,
// the word's aggregate at record time) over the checked args (sig order), or
// nil.
func polySeedFor(sigs []core.Signature, sig *core.Signature, args []core.Value) *polySeed {
	n := len(args)
	if sig == nil || sig.TotalArgs() != n {
		return nil
	}
	// Both guards test tags only (the VM's lean guard runs no pattern Unify),
	// so both need the overloads tag-determined.
	if !core.TagDeterminedSigs(sigs, n) {
		return nil
	}
	same := 0
	for i := range sigs {
		if sigs[i].TotalArgs() == n {
			same++
		}
	}
	if same == 1 {
		return &polySeed{sig: sig}
	}
	tags := make([]*core.Type, n)
	for i, a := range args {
		if a.Parent == nil || a.Dynamic || core.IsBareTypeNode(a) || a.AscribedType() != nil {
			return nil
		}
		// A quoted slot matches a strict carrier and a concrete value alike
		// only for a Word (read as an Atom on both) or an Atom-family tag.
		if sig.QuoteArgs != nil && sig.QuoteArgs[i] && !a.Parent.Equal(core.TWord) && !a.Parent.ConformsTo(core.TAtom) {
			return nil
		}
		tags[i] = a.Parent
	}
	return &polySeed{sig: sig, tags: tags}
}

// takePolySeed hands the pending seed to the poly call being recorded.
func (es *EmitState) takePolySeed() *polySeed {
	s := es.pendingPolySeed
	es.pendingPolySeed = nil
	return s
}
