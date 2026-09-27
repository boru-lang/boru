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

// specFallbackFn is a specialised unit's fallback fn payload (the fn the unit
// specialises — compiler.CompiledFn.SpecFallback).
func specFallbackFn(fn *compiler.CompiledFn) *core.FnDefInfo {
	fd, _ := fn.SpecFallback.Data.(core.FnDefInfo)
	return &fd
}

// specFallbackSig is the fallback fn's one signature: the checker specialises
// single-signature fns only, and CALL_USER's param contract has already
// passed the args, so this is the signature the interpreter's dispatch picks.
func specFallbackSig(fn *compiler.CompiledFn) *core.Signature {
	sigs := specFallbackFn(fn).OwnSigs()
	return &sigs[0]
}
