package stackform

import (
	"errors"

	core "github.com/boru-lang/boru/core/go"
)

// ErrUnnamedApply reports a form that cannot be replayed because it
// applies a function VALUE rather than calling a word: an inline lambda
// (`([n:Integer] => [n add 1]) 5`), or a fn read out of a container or a
// parameter and applied. The engine records these with an empty Call
// name, which is what this compile failure keys on.
//
// The op vocabulary cannot express them. `Call{Name, Arity}` re-invokes
// BY NAME and does not consume a receiver, while an application consumes
// the fn value the stack already holds — so even when the applied value
// carries a name, replaying it as a Call would strand that value and
// produce it twice. It needs an apply-style Op; see NUR077.
//
// Declining is the point. The alternative — what this replaced — is a
// form that silently replays to a different answer than the program it
// was recorded from, which is how the PBT shrinker came to report
// counterexamples its own generator cannot produce.
var ErrUnnamedApply = errors.New(
	"stackform: cannot replay a function-value application — " +
		"the op vocabulary can call a word by name but cannot apply a value")

// Flatten serialises a StackForm into a token sequence the kernel
// engine can execute. Because the form is already strict-stack, the
// serialisation is direct: each PushLit emits its value, each Call
// emits the word name (which the engine will then dispatch in
// stack-only mode against the previously-pushed args), each Quote
// recursively flattens to a list value.
//
// The resulting token sequence does NOT use any forward-collection
// or paren grouping — it is a pure post-fix program.
func Flatten(form *StackForm) []core.Value {
	tokens, _ := flattenStamped(form)
	return tokens
}

// flattenStamped is Flatten plus the canon of every Function value it had to
// mark Quoted, which Eval needs in order to undo the mark afterwards.
func flattenStamped(form *StackForm) ([]core.Value, map[string]bool) {
	if form == nil {
		return nil, nil
	}
	stamped := map[string]bool{}
	out := make([]core.Value, 0, len(form.Ops))
	for _, op := range form.Ops {
		switch o := op.(type) {
		case PushLit:
			// A Function reaching the engine pointer DISPATCHES
			// (core's stepLiteral); only a Quoted one stays data. In a
			// strict-stack form a function that should be called is a
			// Call op, so a PushLit of one is by construction data —
			// the `/v`-held reference the recorder saw. Marking it
			// Quoted is what keeps it inert through the replay.
			//
			// But Quoted is STICKY where `/v` is positional
			// (design/FUNCTION-VALUE-SCOPE.0.md §12.6), so the mark is
			// not free: left in place it travels out on the result and
			// the replayed value is permanently inert where the
			// recorded one was live. Eval undoes it — that is what the
			// returned set is for.
			v := o.V
			if v.Parent != nil && v.Parent.Equal(core.TFunction) && !v.Quoted {
				v.Quoted = true
				stamped[core.Canon([]core.Value{o.V})] = true
			}
			out = append(out, v)
		case Call:
			// stack-only invocation: the engine reaches the Word
			// with all args already on the stack below it.
			w := core.NewWordModified(o.Name, o.Arity, true, false)
			out = append(out, w)
		case Quote:
			// nested form serialises to a list literal. The list
			// is marked Quoted so the kernel doesn't auto-eval it
			// when consumed.
			inner, innerStamped := flattenStamped(o.Body)
			for k := range innerStamped {
				stamped[k] = true
			}
			lst := core.NewList(inner)
			lst.Quoted = true
			out = append(out, lst)
		case DoEval:
			out = append(out, core.NewWord("do"))
		}
	}
	return out, stamped
}

// Replayable reports whether Eval can faithfully replay this form,
// returning ErrUnnamedApply when it cannot.
//
// The check is deliberately structural rather than a comparison against
// a direct run: a form is replayed to DECIDE things (the PBT reducer
// takes a Fail/Pass verdict from each candidate), so "run it twice and
// compare" would both double every side effect and be meaningless for a
// non-deterministic program.
func Replayable(form *StackForm) error {
	if form == nil {
		return nil
	}
	for _, op := range form.Ops {
		switch o := op.(type) {
		case Call:
			if o.Name == "" {
				return ErrUnnamedApply
			}
		case Quote:
			if err := Replayable(o.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

// Eval runs the StackForm through a fresh engine on the given
// registry and returns the resulting stack. This is the round-trip
// partner of Compile — Eval(Compile(reg, src).form) produces the same
// final stack as running `src` directly (modulo PRNG state for
// non-deterministic programs).
//
// A form the recorder could not capture faithfully is DECLINED here
// rather than replayed to a wrong answer; see Replayable.
//
// A form of PLAIN literal pushes alone — the value-level shrinker's
// candidates (shrinkFailingInput: `[PushLit v]`, v the failing input or one
// of its shrinks, a scalar or a container) — is its literals: the engine's
// step of such a literal is the push of that value, unchanged (identity,
// Quoted, all of it — TestEvalLiteralsOnlyMatchesTheEngine), so Eval answers
// it without an engine run, and a compiled program's shrink stays off the
// interpreter (the interp-entry census's corpus-modules.tsv L164). A form
// with a call, a quote or a do replays on the engine as before, and so does
// one pushing a Function — the push the engine would DISPATCH, which the
// replay stamps Quoted — or any literal the engine steps rather than
// pushes (a paren group, a splice, sugar, a reach).
func Eval(reg *core.Registry, form *StackForm) ([]core.Value, error) {
	if err := Replayable(form); err != nil {
		return nil, err
	}
	if lits, only := plainLiterals(form); only {
		return lits, nil
	}
	tokens, stamped := flattenStamped(form)
	out, err := core.NewTop(reg).Run(tokens)
	if err != nil {
		return out, err
	}
	return unstamp(out, stamped), nil
}

// plainLiterals returns the values of a form made of plain literal pushes
// alone (see Eval), or ok=false for any other form, an empty one included.
func plainLiterals(form *StackForm) ([]core.Value, bool) {
	if form == nil || len(form.Ops) == 0 {
		return nil, false
	}
	out := make([]core.Value, 0, len(form.Ops))
	for _, op := range form.Ops {
		lit, isLit := op.(PushLit)
		if !isLit || !plainLiteral(lit.V) {
			return nil, false
		}
		out = append(out, lit.V)
	}
	return out, true
}

// plainLiteral reports whether the engine's step of v is the push of v
// itself: a scalar, a list or a map value that is not a form the engine
// steps (a paren group, a splice, sugar, a reach). A Function is not plain —
// unquoted it dispatches.
func plainLiteral(v core.Value) bool {
	if v.Parent == nil || core.IsParenExpr(v) || core.IsSplice(v) || core.IsSugar(v) || core.IsReach(v) {
		return false
	}
	return v.Parent.ConformsTo(core.TScalar) || v.Parent.ConformsTo(core.TList) || v.Parent.ConformsTo(core.TMap)
}

// unstamp restores the recorded state of every Function that flattenStamped
// had to mark Quoted to keep it inert during the replay. Without this the
// round trip returns a permanently-inert copy of a value the direct run
// returns live — and `canon` renders a fn without its Quoted flag, so no
// canon-based comparison would ever notice.
//
// Matching is by canon because Flatten stamps a COPY. A stamped entry is by
// construction a value the recorder saw UNQUOTED (the mark is only applied to
// those), so clearing the flag returns it to what was recorded. The one shape
// this cannot separate is a program that produces both a quoted and an
// unquoted copy of the SAME function and leaves the quoted one in the result.
func unstamp(vals []core.Value, stamped map[string]bool) []core.Value {
	if len(stamped) == 0 {
		return vals
	}
	for i, v := range vals {
		if v.Quoted && v.Parent != nil && v.Parent.Equal(core.TFunction) &&
			stamped[core.Canon([]core.Value{unquotedCopy(v)})] {
			vals[i] = unquotedCopy(v)
		}
	}
	return vals
}

func unquotedCopy(v core.Value) core.Value {
	v.Quoted = false
	return v
}
