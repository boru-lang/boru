package eng

import (
	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// specGuardsHold reports whether a CALL_USER may enter a call-site
// specialised unit: every guarded param's arg (args in sig order — the
// frame's param slots) must be the very fn the unit was compiled for
// (core.ExactEqual, fn identity: what `eq` answers).
func specGuardsHold(guards []compiler.SpecGuard, args []core.Value) bool {
	for _, g := range guards {
		if !core.ExactEqual(args[g.Param], g.Fn) {
			return false
		}
	}
	return true
}

// specFallbackInputs lays a failed guard's signature args (sig order) out as
// the fallback island's resolved stack: the value stack fills a signature's
// positions top first, so param 0 goes on top. The island steps only the fn
// itself (compiler.CompiledFn.SpecFallback) over them — never an arg, so a fn
// VALUE among them stays the value it was, as in the frame's param slot.
func specFallbackInputs(args []core.Value) []core.Value {
	out := make([]core.Value, len(args))
	for i, v := range args {
		out[len(args)-1-i] = v
	}
	return out
}
