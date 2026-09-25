package stackform

import core "github.com/boru-lang/boru/core/go"

// Walk visits every Op in the StackForm, descending into Quote
// bodies. `visit` returns false to stop the walk early.
//
// The path argument tracks the visit depth as a slice of indices —
// path[0] is the top-level Op index, path[1] (if present) is the
// index inside the Quote's body, etc. Useful for reducer rewrites
// that need to address a specific nested Op.
func Walk(form *StackForm, visit func(path []int, op Op) bool) {
	if form == nil {
		return
	}
	walk(form, nil, visit)
}

func walk(form *StackForm, prefix []int, visit func([]int, Op) bool) bool {
	for i, op := range form.Ops {
		path := append(prefix, i)
		if !visit(path, op) {
			return false
		}
		if q, ok := op.(Quote); ok && q.Body != nil {
			if !walk(q.Body, path, visit) {
				return false
			}
		}
	}
	return true
}

// Equal reports whether two StackForms are structurally identical:
// same Op kinds in the same order, with equal literal values and
// equal nested bodies.
//
// Literal equality uses core.DeepEqual, which is value-equality
// modulo representation (Integer 1 equals Float 1.0 if the eng
// comparator says so — see eng/go/compare.go).
func Equal(a, b *StackForm) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if len(a.Ops) != len(b.Ops) {
		return false
	}
	for i := range a.Ops {
		if !opEqual(a.Ops[i], b.Ops[i]) {
			return false
		}
	}
	return true
}

func opEqual(a, b Op) bool {
	switch x := a.(type) {
	case PushLit:
		y, ok := b.(PushLit)
		if !ok {
			return false
		}
		return core.DeepEqual(x.V, y.V)
	case Call:
		y, ok := b.(Call)
		return ok && x.Name == y.Name && x.Arity == y.Arity && x.ReStep == y.ReStep
	case Apply:
		y, ok := b.(Apply)
		return ok && x.Arity == y.Arity
	case Quote:
		y, ok := b.(Quote)
		return ok && Equal(x.Body, y.Body)
	case DoEval:
		_, ok := b.(DoEval)
		return ok
	}
	return false
}
