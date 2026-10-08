package core

// The interpreter compiles a fn VALUE's authored signatures into its
// dispatch form (compileFnDef: normalised, barrier-resolved sigs with the
// body-runner attached) at the point of dispatch — execFnDefLiteral, for a
// Function value stepped on the tape. That is every application of a
// method read off an instance, of a callback per element, of an applied
// value: compiling per dispatch minted a handler closure, its leaf-body
// skeleton, the frame identity and a sorted signature copy on each one,
// a dozen allocations per call that profiled at several percent of the
// call. The form is a pure function of the registry and the authored
// payload, so it is computed once per function and kept on the function's
// identity token, which every copy of the value shares.

// compiledFnDef is one registry's compiled dispatch form of a function,
// with what it was built from: the registry (the body-runner closes over
// it — a fork of the registry compiles its own), the authored signature
// and capture lists by backing array and length, and the dispatch name.
// A token reused over a REBUILT list — a looked-up name's aggregate,
// which aggregateDispatch regenerates — therefore recompiles instead of
// answering a stale form; copies of one value share their arrays and hit.
type compiledFnDef struct {
	reg   *Registry
	name  string
	sigs  *Signature
	nsigs int
	caps  *CapturedBinding
	ncaps int
	fn    *FnDefInfo
}

// compiledFnDefFor returns compileFnDef(reg, fnDef), computed once per
// function identity and registry: the cached form when the token's cache
// was built for this registry from these very lists, a fresh compile
// (which replaces the cache) otherwise. A payload with no identity — a
// synthesized FnDefInfo literal — compiles every time, as before.
func compiledFnDefFor(reg *Registry, fnDef FnDefInfo) *FnDefInfo {
	id := fnDef.ident
	if id == nil {
		return compileFnDef(reg, fnDef)
	}
	var sigs *Signature
	if len(fnDef.Signatures) > 0 {
		sigs = &fnDef.Signatures[0]
	}
	var caps *CapturedBinding
	if len(fnDef.Captured) > 0 {
		caps = &fnDef.Captured[0]
	}
	if c := id.dispatch.Load(); c != nil && c.reg == reg && c.name == fnDef.Name &&
		c.sigs == sigs && c.nsigs == len(fnDef.Signatures) &&
		c.caps == caps && c.ncaps == len(fnDef.Captured) {
		return c.fn
	}
	fn := compileFnDef(reg, fnDef)
	id.dispatch.Store(&compiledFnDef{
		reg:   reg,
		name:  fnDef.Name,
		sigs:  sigs,
		nsigs: len(fnDef.Signatures),
		caps:  caps,
		ncaps: len(fnDef.Captured),
		fn:    fn,
	})
	return fn
}
