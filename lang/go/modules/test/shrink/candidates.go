package shrink

import (
	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/stackform"
)

// generateCandidates produces every candidate mutation of `form`
// the reducer should consider. Each candidate is a NEW StackForm —
// `form` is not mutated.
//
// Rewrite families implemented today (per PBT-PLAN.10.md Stage 5):
//
//  1. Drop-one-op: N candidates each missing one Op.
//  2. Literal shrinking: per PushLit, try cheaper alternatives:
//     - Integer → 0, then halve (binary search toward 0).
//     - String  → "", then first char only.
//     - Boolean → false (if true).
//  3. Quote-body recursion: for each Quote op, generate candidates
//     by reducing its body and substituting back.
//
// Deferred (Phase-4 stretch in PBT-PLAN): list-element dropping,
// stack-op simplification (dup/drop→identity), generator-semantic
// rewrites (Rand.int min max → Rand.int 0 1).
func generateCandidates(form *stackform.StackForm, policy *Policy) []*stackform.StackForm {
	var out []*stackform.StackForm
	out = append(out, dropOpCandidates(form)...)
	out = append(out, literalShrinkCandidates(form)...)
	out = append(out, quoteBodyCandidates(form, policy)...)
	return out
}

// dropOpCandidates yields N variants each with one op removed.
// The reducer will reject any that fail to evaluate (Outcome=Invalid)
// or fail to preserve the failure (Outcome=Pass); no need to do
// validity checks here.
func dropOpCandidates(form *stackform.StackForm) []*stackform.StackForm {
	if form == nil {
		return nil
	}
	out := make([]*stackform.StackForm, 0, len(form.Ops))
	for i := range form.Ops {
		newOps := make([]stackform.Op, 0, len(form.Ops)-1)
		newOps = append(newOps, form.Ops[:i]...)
		newOps = append(newOps, form.Ops[i+1:]...)
		out = append(out, &stackform.StackForm{Ops: newOps})
	}
	return out
}

// literalShrinkCandidates yields variants where one PushLit's value
// has been replaced with a simpler alternative.
func literalShrinkCandidates(form *stackform.StackForm) []*stackform.StackForm {
	if form == nil {
		return nil
	}
	var out []*stackform.StackForm
	for i, op := range form.Ops {
		lit, ok := op.(stackform.PushLit)
		if !ok {
			continue
		}
		for _, smaller := range shrinkLiteral(lit.V) {
			newOps := make([]stackform.Op, len(form.Ops))
			copy(newOps, form.Ops)
			newOps[i] = stackform.PushLit{V: smaller}
			out = append(out, &stackform.StackForm{Ops: newOps})
		}
	}
	return out
}

// shrinkLiteral returns simpler-shaped alternatives for a single
// value. The reducer tries each in cost order.
//
// Per type:
//   - Integer: → 0; → N/2 (binary halving toward 0).
//   - Float: → 0.0; → N/2.
//   - String : → "" (if non-empty); → first char only (if len > 1).
//   - Boolean: true → false.
//   - List   : → [] (if non-empty); → first half; → first element.
func shrinkLiteral(v core.Value) []core.Value {
	if core.ParentOf(&v) == nil {
		return nil
	}
	switch {
	case core.ParentOf(&v).ConformsTo(core.TInteger):
		return shrinkInteger(v)
	case core.ParentOf(&v).ConformsTo(core.TFloat):
		return shrinkFloat(v)
	case core.ParentOf(&v).ConformsTo(core.TString):
		return shrinkString(v)
	case core.ParentOf(&v).ConformsTo(core.TBoolean):
		return shrinkBoolean(v)
	case core.ParentOf(&v).ConformsTo(core.TList):
		return shrinkList(v)
	}
	return nil
}

func shrinkInteger(v core.Value) []core.Value {
	n, err := core.AsInteger(v)
	if err != nil {
		return nil
	}
	var out []core.Value
	if n != 0 {
		out = append(out, core.NewInteger(0))
	}
	// Binary halving (toward 0). Drops a high bit per step.
	if n > 1 || n < -1 {
		out = append(out, core.NewInteger(n/2))
	}
	// Unit step (toward 0). After halving stalls (e.g. 12→6 makes
	// the property pass, but 11/10 still fails), this finds the
	// exact minimum violator one step at a time.
	if n > 0 {
		out = append(out, core.NewInteger(n-1))
	} else if n < 0 {
		out = append(out, core.NewInteger(n+1))
	}
	return out
}

func shrinkFloat(v core.Value) []core.Value {
	f, err := core.AsFloat(v)
	if err != nil {
		return nil
	}
	var out []core.Value
	if f != 0 {
		out = append(out, core.NewFloat(0))
	}
	if f > 1 || f < -1 {
		out = append(out, core.NewFloat(f/2))
	}
	return out
}

func shrinkString(v core.Value) []core.Value {
	s, err := core.AsString(v)
	if err != nil {
		return nil
	}
	var out []core.Value
	if s != "" {
		out = append(out, core.NewString(""))
	}
	if len(s) > 1 {
		out = append(out, core.NewString(s[:1]))
	}
	if len(s) > 2 {
		out = append(out, core.NewString(s[:len(s)/2]))
	}
	return out
}

func shrinkBoolean(v core.Value) []core.Value {
	b, err := core.AsBoolean(v)
	if err != nil {
		return nil
	}
	if b {
		return []core.Value{core.NewBoolean(false)}
	}
	return nil
}

func shrinkList(v core.Value) []core.Value {
	lst, err := core.RequireConcreteList(v, "shrinkList")
	if err != nil {
		return nil
	}
	n := lst.Len()
	if n == 0 {
		return nil
	}
	var out []core.Value
	// Empty list — biggest single shrink.
	out = append(out, core.NewList(nil))
	// First half.
	if n > 1 {
		half := n / 2
		els := make([]core.Value, half)
		for i := 0; i < half; i++ {
			els[i] = lst.Get(i)
		}
		out = append(out, core.NewList(els))
	}
	// First element only.
	if n > 1 {
		out = append(out, core.NewList([]core.Value{lst.Get(0)}))
	}
	return out
}

// quoteBodyCandidates recurses into Quote ops, generating candidates
// where the inner body has been shrunk via the same machinery.
func quoteBodyCandidates(form *stackform.StackForm, policy *Policy) []*stackform.StackForm {
	if form == nil {
		return nil
	}
	var out []*stackform.StackForm
	for i, op := range form.Ops {
		q, ok := op.(stackform.Quote)
		if !ok {
			continue
		}
		bodyCands := generateCandidates(q.Body, policy)
		for _, bc := range bodyCands {
			newOps := make([]stackform.Op, len(form.Ops))
			copy(newOps, form.Ops)
			newOps[i] = stackform.Quote{Body: bc}
			out = append(out, &stackform.StackForm{Ops: newOps})
		}
	}
	return out
}
