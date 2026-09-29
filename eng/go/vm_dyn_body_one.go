package eng

import (
	"fmt"
	"slices"

	core "github.com/boru-lang/boru/core/go"
)

// checkDynBodyOne is the runtime count check of a computed `do` body's run
// that a single-value seat consumes (SigRef/PolyRef.DynBodyOne — compiler
// dyn_body_one.go). The recorder modelled the run as ONE value; the fixed
// seat after the call agrees with the interpreter's tape only when the run
// left exactly one value that the tape would not re-step. Anything else — no
// value, several, or a fn value / class / reach / modifier the interpreter
// dispatches when its tape steps it — is a designed defer at the call's own
// position: loud, never a value seated where the interpreter would have
// placed another. (Word tokens, marks, moves and splices never get here:
// screenResults refused them first.)
func checkDynBodyOne(r *core.Registry, word string, results []core.Value, curDebug []core.SrcPos, pc int) error {
	return checkRunOne(r, word+" over a computed body", results, curDebug, pc)
}

// checkRunOne is checkDynBodyOne's rule over any run a single-value seat
// takes — a computed body's, or a strip word's island (FallbackSpan.CheckOne,
// `error` over a literal handler, NUR301) — named by what left it.
func checkRunOne(r *core.Registry, what string, results []core.Value, curDebug []core.SrcPos, pc int) error {
	if len(results) != 1 {
		return vmDefer(r, curDebug, pc, "vm:dyn-body-one", fmt.Sprintf(
			"%s left %d value(s) where a single-value seat consumes it; the compiled runtime cannot execute it", what, len(results)))
	}
	if dynBodyValueReSteps(results[0]) {
		return vmDefer(r, curDebug, pc, "vm:dyn-body-one", what+
			" left a value the interpreter re-steps (a fn value, class, reach or modifier) where a single-value seat consumes it; the compiled runtime cannot execute it")
	}
	return nil
}

// checkDynBodyPlain is the runtime check of a computed `do` body's run seated
// where the prefix island does not re-step it (SigRef/PolyRef.DynBodyPlain —
// compiler lowerCall, NUR213): with values beneath it, entries after it, or
// as a fn's result. A run of plain values is data on both lanes wherever it
// lands; a value the interpreter's tape dispatches when it steps it (a fn
// value, class, reach or modifier) would be applied over the run's
// neighbours, which the seated run cannot do — a designed defer at the call's
// own position, loud.
func checkDynBodyPlain(r *core.Registry, word string, results []core.Value, curDebug []core.SrcPos, pc int) error {
	if dynBodyPlainRefuses(results) {
		return vmDefer(r, curDebug, pc, "vm:dyn-body-plain", word+
			" over a computed body left a value the interpreter re-steps (a fn value, class, reach or modifier) where the run is seated as data; the compiled runtime cannot execute it")
	}
	return nil
}

// dynBodyPlainRefuses reports whether checkDynBodyPlain would refuse
// results: a count island takes such a run instead (SigRef.Count, NUR348).
func dynBodyPlainRefuses(results []core.Value) bool {
	return slices.ContainsFunc(results, dynBodyValueReSteps)
}

// dynBodyOneRefuses reports whether checkDynBodyOne would refuse results,
// without its defer's bail note: a single seat's count island takes such a
// run instead (SigRef.Count, NUR282).
func dynBodyOneRefuses(results []core.Value) bool {
	return len(results) != 1 || dynBodyValueReSteps(results[0])
}

// dynBodyValueReSteps reports whether the interpreter's tape DISPATCHES v
// when it steps it, rather than pushing it as data.
func dynBodyValueReSteps(v core.Value) bool {
	if core.IsAppliableFn(v) || core.IsReach(v) {
		return true
	}
	if _, isClass := v.Data.(*core.ClassTypeInfo); isClass {
		return true
	}
	_, mod := core.AsDispatchMod(v)
	return mod
}

// frameRunReSteps reports whether a unit's computed run holds a value its
// island would step where the interpreter's frame steps on into the caller's
// tape (SigRef.CountFrame, NUR348): a value the tape dispatches, as the run
// left it or as a splice's payload (a list's elements, or the value itself).
// The island ends at the unit's last token; the interpreter's frame does
// not, so a named fn stepped last raises uncalled_function there and parks
// on the island.
func frameRunReSteps(results []core.Value) bool {
	return slices.ContainsFunc(results, func(v core.Value) bool {
		if info, err := core.AsSplice(v); err == nil {
			return slices.ContainsFunc(splicePayload(info.Data), func(e core.Value) bool {
				return core.IsSplice(e) || dynBodyValueReSteps(e)
			})
		}
		return dynBodyValueReSteps(v)
	})
}

// splicePayload is what a splice marker's payload contributes to the tape:
// a list's elements, or any other value itself.
func splicePayload(p core.Value) []core.Value {
	if p.Parent.Equal(core.TList) {
		if l, err := core.AsList(p); err == nil {
			return l.Slice()
		}
	}
	return []core.Value{p}
}
