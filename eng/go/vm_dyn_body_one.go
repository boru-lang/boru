package eng

import (
	"fmt"

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
	if len(results) != 1 {
		return vmDefer(r, curDebug, pc, "vm:dyn-body-one", fmt.Sprintf(
			"%s over a computed body left %d value(s) where a single-value seat consumes it; the compiled runtime cannot execute it", word, len(results)))
	}
	if dynBodyValueReSteps(results[0]) {
		return vmDefer(r, curDebug, pc, "vm:dyn-body-one", word+
			" over a computed body left a value the interpreter re-steps (a fn value, class, reach or modifier) where a single-value seat consumes it; the compiled runtime cannot execute it")
	}
	return nil
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
