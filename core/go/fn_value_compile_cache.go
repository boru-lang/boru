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
// and capture lists by identity (sameList), and the dispatch name. A token
// reused over a REBUILT list — a looked-up name's aggregate, which
// aggregateDispatch regenerates — therefore recompiles instead of
// answering a stale form; copies of one value share their arrays and hit.
type compiledFnDef struct {
	reg  *Registry
	name string
	sigs []Signature       // the authored list the form was built from
	caps []CapturedBinding // the capture list it was built from
	fn   *FnDefInfo
}

// sameList reports whether a and b are the same list: the same backing
// array at the same length. A list rebuilt by aggregateDispatch is a
// different array; a copy of a value shares its arrays. Two empty lists
// are the same empty list.
func sameList[T any](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

// compiledFnDefFor returns compileFnDef(reg, fnDef), computed once per
// function identity and registry: the cached form when the token's cache
// was built for this registry from these very lists, a fresh compile
// (which replaces the cache) otherwise. A payload with no identity — a
// synthesized FnDefInfo literal — compiles every time, as before.
func compiledFnDefFor(reg *Registry, fnDef *FnDefInfo) *FnDefInfo {
	id := fnDef.ident
	if id == nil {
		return compileFnDef(reg, *fnDef)
	}
	if c := id.dispatch.Load(); c != nil && c.reg == reg && c.name == fnDef.Name &&
		sameList(c.sigs, fnDef.Signatures) && sameList(c.caps, fnDef.Captured) {
		return c.fn
	}
	fn := compileFnDef(reg, *fnDef)
	id.dispatch.Store(&compiledFnDef{
		reg:  reg,
		name: fnDef.Name,
		sigs: fnDef.Signatures,
		caps: fnDef.Captured,
		fn:   fn,
	})
	return fn
}

// installedFnSigs is the install form of a function under one name in one
// registry — compileFnSigs' signatures in dispatch order — with what it was
// built from: the installing registry and the one the body runs in (FnHome:
// the body-runner closes over the registry the fn lives in), the name and
// barrier mode the signatures carry, and the authored signature and
// capture lists by identity (sameList).
type installedFnSigs struct {
	reg       *Registry
	runsIn    *Registry
	name      string
	stackOnly bool
	sigs      []Signature
	caps      []CapturedBinding
	out       []Signature
}

// installedFnSigsFor returns compileFnSigs(r, name, *fnDef, stackOnly) in
// dispatch order, computed once per function identity, registry, name and
// barrier mode. Binding a fn value under a name — a Function parameter, on
// every call of the fn that takes it — compiled it afresh each time: a body
// runner, its leaf-frame skeleton, the frame identity and a sorted
// signature copy per call, which profiled at 13% of a user-defined fold. The
// form is the same one the function's first install built, as a `def`'d fn
// is compiled once at its definition and a fn value once per identity
// (compiledFnDefFor) — the body's frame-state judgement included. The
// signatures are shared by every binding made from them and never written.
//
// An analysis pass compiles every install afresh, as Lookup gives it a
// fresh aggregate: its carrier joins tell two constructions apart by their
// signature arrays (sameFnConstruction). A payload with no identity — a
// synthesized FnDefInfo literal — compiles every time.
func installedFnSigsFor(r *Registry, name string, fnDef *FnDefInfo, stackOnly bool) []Signature {
	id := fnDef.ident
	if id == nil || r.analysisActive() {
		return compileSortedFnSigs(r, name, fnDef, stackOnly)
	}
	runsIn, _ := FnHome(r, fnDef)
	if c := id.install.Load(); c != nil && c.reg == r && c.runsIn == runsIn &&
		c.name == name && c.stackOnly == stackOnly &&
		sameList(c.sigs, fnDef.Signatures) && sameList(c.caps, fnDef.Captured) {
		return c.out
	}
	out := compileSortedFnSigs(r, name, fnDef, stackOnly)
	id.install.Store(&installedFnSigs{
		reg:       r,
		runsIn:    runsIn,
		name:      name,
		stackOnly: stackOnly,
		sigs:      fnDef.Signatures,
		caps:      fnDef.Captured,
		out:       out,
	})
	return out
}

// compileSortedFnSigs is compileFnSigs in dispatch order.
func compileSortedFnSigs(r *Registry, name string, fnDef *FnDefInfo, stackOnly bool) []Signature {
	sigs := compileFnSigs(r, name, *fnDef, stackOnly)
	SortSignatures(sigs)
	return sigs
}

// singleAggregate is the run-time dispatch aggregate built for a name whose
// only fn binding is the function, with what it was built from: the registry
// (the synthetic fallback closes over it), the name, the binding's signature
// and capture lists by identity, and the binding's other fields the
// aggregate carries. Only a function with no home is kept — a fn defined in
// the running scope, the Function parameter's common case — so the
// aggregate's Registry is the binding's by construction.
type singleAggregate struct {
	reg      *Registry
	name     string
	sigs     []Signature
	caps     []CapturedBinding
	anon     bool
	macro    bool
	miniKind string
	extends  string
	agg      *FnDefInfo
}

// singleAggregateFor returns the aggregate rememberSingleAggregate kept for
// a binding like top under name, or nil. Run time only: an analysis pass's
// Lookup builds a fresh aggregate per call by contract (Registry.Lookup).
func (r *Registry) singleAggregateFor(name string, top *FnDefInfo) *FnDefInfo {
	id := top.ident
	if id == nil || top.HasHome() || r.analysisActive() {
		return nil
	}
	if c := id.aggregate.Load(); c != nil && c.matches(r, name, top) {
		return c.agg
	}
	return nil
}

// rememberSingleAggregate keeps agg, the aggregate of top under name, on the
// function's identity for singleAggregateFor.
func (r *Registry) rememberSingleAggregate(name string, top *FnDefInfo, agg *FnDefInfo) {
	id := top.ident
	if id == nil || top.HasHome() || r.analysisActive() {
		return
	}
	id.aggregate.Store(&singleAggregate{
		reg:      r,
		name:     name,
		sigs:     top.Signatures,
		caps:     top.Captured,
		anon:     top.Anonymous,
		macro:    top.Macro,
		miniKind: top.MiniKind,
		extends:  top.Extends,
		agg:      agg,
	})
}

// matches reports whether c was built for a binding like top under name in r.
func (c *singleAggregate) matches(r *Registry, name string, top *FnDefInfo) bool {
	return c.reg == r && c.name == name && sameList(c.sigs, top.Signatures) &&
		sameList(c.caps, top.Captured) && c.anon == top.Anonymous &&
		c.macro == top.Macro && c.miniKind == top.MiniKind && c.extends == top.Extends
}

// sigView is a function's dispatch-ordered signature list (FnDispatchSigs)
// with the authored list it was built from, by identity (sameList).
type sigView struct {
	src  []Signature
	sigs []Signature
}

// FnDispatchSigs returns fd's signatures as the interpreter's step matches a
// fn value under them (compileFnDef's view, without the body runners a match
// needs): each with its all-forward barrier resolved and normalised, sorted
// into dispatch order. It is built once per function identity and authored
// list and shared by every caller after: never write into it. A payload with
// no identity builds it on every call.
func FnDispatchSigs(fd *FnDefInfo) []Signature {
	id := fd.ident
	if id == nil {
		return dispatchSigs(fd.Signatures)
	}
	if c := id.view.Load(); c != nil && sameList(c.src, fd.Signatures) {
		return c.sigs
	}
	sigs := dispatchSigs(fd.Signatures)
	id.view.Store(&sigView{src: fd.Signatures, sigs: sigs})
	return sigs
}

func dispatchSigs(src []Signature) []Signature {
	sigs := make([]Signature, len(src))
	for i := range src {
		s := src[i]
		s.Params = append([]FnParam(nil), s.Params...)
		if s.BarrierPos == BarrierAllForward {
			s.BarrierPos = len(s.Params)
		}
		NormalizeSig(&s)
		sigs[i] = s
	}
	SortSignatures(sigs)
	return sigs
}
