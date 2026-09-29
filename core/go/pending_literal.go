package core

import (
	"sort"
	"strings"
)

// A PENDING container literal is a list or map the parser wrote (Eval, not
// quoted) that nothing has consumed yet: the interpreter evaluates it where a
// word takes it, at the end of the run it is left in, or never (a code-body
// slot takes it raw). An `if` arm is spliced onto its enclosing tape as a
// paren group, which does not evaluate what it leaves, so a literal an arm
// ends with stays pending past the arm. The check pass runs the arm in a
// sub-engine of its own, whose end-of-run sweep evaluates it AT the arm —
// the compiled lane's eager assembly (NUR356). These predicates say which
// residuals that sweep evaluates, and what evaluating one early could change.

// isPendingList is a residual list the sweep evaluates as a sub-program.
func isPendingList(v Value) bool {
	return v.Eval && !v.Quoted && v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v)
}

// isPendingMap is a residual map the sweep evaluates value by value.
func isPendingMap(v Value) bool {
	return v.Eval && !v.Quoted && v.Parent.Equal(TMap) && v.Data != nil && !IsTypedMap(v) && !IsRecordType(v) && !IsOptionsType(v)
}

// PendingResidue describes the observable pending literals a spliced arm's
// model run left at its end (RunCarrierArmBody): literals whose evaluation
// is not a copy of what was written — they hold a token the evaluation
// steps, at any depth of their nested literals. A literal of scalar leaves
// evaluates to itself whenever it runs and is no residue.
//
// Reads names every binding such a literal reads by a plain word; Calls
// says one of them steps anything else — a word bound to a fn, a macro, a
// native word or nothing the pass can see, a member path, a paren, an
// interpolation, a computed key. Evaluating a reads-only literal early is
// observable only across a rebinding of a name it reads (or a slot that
// takes it raw); one that calls is observable across anything that runs.
type PendingResidue struct {
	Left  bool
	Calls bool
	Reads []string
}

// Merge is the residue of two literals (or arms) together.
func (p PendingResidue) Merge(q PendingResidue) PendingResidue {
	if !q.Left {
		return p
	}
	if !p.Left {
		return q
	}
	return PendingResidue{Left: true, Calls: p.Calls || q.Calls, Reads: mergeNames(p.Reads, q.Reads)}
}

// ReadsName reports whether the residue's literals read name.
func (p PendingResidue) ReadsName(name string) bool {
	i := sort.SearchStrings(p.Reads, name)
	return i < len(p.Reads) && p.Reads[i] == name
}

// mergeNames is the sorted, deduplicated union of two sorted name lists.
func mergeNames(a, b []string) []string {
	out := append(append([]string(nil), a...), b...)
	sort.Strings(out)
	return compactNames(out)
}

// compactNames drops the adjacent duplicates of a sorted list.
func compactNames(names []string) []string {
	out := names[:0]
	for i, n := range names {
		if i == 0 || n != names[i-1] {
			out = append(out, n)
		}
	}
	return out
}

// PendingLiteralSteps reports whether v is a pending list or map literal
// whose evaluation steps a token — so evaluated, it is no longer what was
// written, and a report rendering the literal as written differs from one
// rendering its value.
func PendingLiteralSteps(v Value) bool {
	if !isPendingList(v) && !isPendingMap(v) {
		return false
	}
	w := pendingWalk{}
	w.literal(v)
	return w.left
}

// pendingResidueOf is the residue a finished run's stack leaves for its
// end-of-run sweep, reading word bindings in r.
func pendingResidueOf(r *Registry, t *Tape) PendingResidue {
	w := pendingWalk{r: r}
	for i := 0; i < t.Len(); i++ {
		if v := t.At(i); isPendingList(v) || isPendingMap(v) {
			w.literal(v)
		}
	}
	if !w.left {
		return PendingResidue{}
	}
	var names []string
	for n := range w.reads {
		names = append(names, n)
	}
	sort.Strings(names)
	return PendingResidue{Left: true, Calls: w.calls, Reads: names}
}

// pendingWalk accumulates a residue over the pending literals it visits.
type pendingWalk struct {
	r     *Registry
	left  bool
	calls bool
	reads map[string]bool
}

// literal visits one pending literal's elements (a list's) or values (a
// map's); a computed key steps.
func (w *pendingWalk) literal(v Value) {
	if isPendingList(v) {
		elems, _ := AsList(v)
		for i := 0; i < elems.Len(); i++ {
			w.elem(elems.Get(i))
		}
		return
	}
	m, _ := AsMutableMap(v)
	if ck, _ := m.Meta["ck"].(map[string]bool); len(ck) > 0 {
		w.left, w.calls = true, true
	}
	for _, k := range m.Keys() {
		val, _ := m.Get(k)
		w.elem(val)
	}
}

// elem visits one element: a scalar leaf is placed as itself, a nested
// pending literal is visited, a word is a read or a call (word), and every
// other token steps (IsSteplessValue's allowlist discipline).
func (w *pendingWalk) elem(v Value) {
	switch {
	case IsSteplessValue(v):
	case isPendingList(v) || isPendingMap(v):
		w.literal(v)
	case IsWord(v):
		w.left = true
		w.word(v)
	default:
		w.left, w.calls = true, true
	}
}

// word classifies one word: a READ of a binding holding data — a `/v`
// spelling, or a name whose top binding is a value neither a fn, a gradual
// value nor anything else the step loop would run — records the name; any
// other word calls. A member path (`m.a`) calls: its member may be a fn the
// read applies.
func (w *pendingWalk) word(v Value) {
	info, _ := AsWord(v)
	if strings.Contains(info.Name, ".") || !(info.ForceVal || readsData(w.r, info.Name)) {
		w.calls = true
		return
	}
	if w.reads == nil {
		w.reads = map[string]bool{}
	}
	w.reads[info.Name] = true
}

// readsData reports whether name's top binding in r is data the step loop
// places when the name is read: a concrete container or inert scalar, a
// carrier of a known type that is no fn (the check pass's model of a
// computed value), or a GRADUAL value — which the compiled lane reads as
// data and guards where it is a fn at run time (NUR123's deopt, or the
// designed defer), never applying it in place. A builtin word, a fn, a
// value that may be one and an unbound name are not.
func readsData(r *Registry, name string) bool {
	if r == nil || r.IsBuiltinWord(name) {
		return false
	}
	v, ok := r.Defs.Top(name)
	if !ok {
		return false
	}
	if v.Dynamic {
		return true
	}
	if IsAppliableFn(v) || IsFnTypedCarrier(v) || UnionMayBeFn(v) {
		return false
	}
	if v.Carrier {
		return v.Parent != nil && !v.Parent.Equal(TAny)
	}
	switch v.Data.(type) {
	case ListPayload, MapPayload:
		return IsConcrete(v)
	}
	return IsInertConst(v)
}
