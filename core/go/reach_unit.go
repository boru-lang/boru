package core

import "sync"

// The lens half of the Apply kernel (design/FULL-COMPILATION.0.md §Stage 3).
//
// A Reach is a callable value exactly as a fn value is: `p $.name apply`,
// `each $.name people`, `StructUtil.getpath $.a.b m` and `ArrayUtil.sortby
// $.age people` all reach ONE primitive, ApplyReach, which lowers the lens to
// a `[recv dot key dot key2 …]` token chain and runs it. Running it meant a
// pooled sub-engine per application — an interpreter entry inside a native
// handler, the exact shape `TestCompiledCoverage` cannot see (it counts
// OpFallback spans, and these programs disassemble with fallbacks=0).
//
// It was the largest single-mechanism cluster in the interpreter-entry census:
// every dirty row of lang/spec/reach.tsv, through this one function.
//
// WHY NOT A SECOND WALKER IN GO. Walking the segments with direct map/list
// reads would remove the engine entry and introduce a second implementation of
// `dot` — which is why getpathReachHandler routes here in the first place ("so
// per-segment getr strictness and computed keys behave exactly as bare m.a.b —
// the same primitive `apply` uses"). One dispatch path is the rule; this keeps
// it and moves the path onto the VM instead.
//
// WHAT IS COMPILED. The chain is already a one-parameter function body: bind
// the receiver, run the dots. So the unit is compiled from exactly the tokens
// ApplyReach would otherwise have interpreted, with the receiver VALUE replaced
// by a reference to the bound parameter — no hand-lowering, no second model of
// what a segment means. Everything else (dep freshness, the JIT re-stamp, the
// effect fence, the internal-error degrade) is the CompiledRuntime seam's,
// unchanged.
//
// The receiver arrives as a NAMED param rather than a stack push, and that is
// the one place this differs in shape from the interpreted chain. It was
// verified by measurement rather than argument: the whole corpus, the spec
// differential, the variation sweep and the frontier ledgers are unchanged,
// and the census fell 130 -> 114.

// lensUnit caches one lens's compiled unit on the Reach PAYLOAD, so a const
// lens applied per element compiles once rather than per element.
//
// It hangs off ReachInfo as a POINTER, which is what makes the cache shared:
// every copy of the Reach value carries the same *lensUnit, exactly as every
// copy of a signature carries the same *BoruImpl (and its Compiled ref). The
// field is unexported and canon renders a Reach from its SEGMENTS, so nothing
// compares or serialises it.
type lensUnit struct {
	once sync.Once
	sig  *Signature // non-nil once a stamp landed; nil means "declined, stop asking"
}

// lensParam is the receiver parameter's name. It cannot collide with a user
// binding the body reads: the body is a chain of `dot` dispatches over this one
// name and literal keys, with nothing else resolvable in it except a computed
// segment's own expression — and that expression is the user's, evaluated in
// the same scope either way.
const lensParam = "__reach_recv"

// compiledLensSig returns the lens's stamped one-param signature, or nil when
// there is none to have.
//
// Only stamps while runtime stamping is ARMED. Unarmed is the interpreter mode,
// where interpreting the chain is the correct answer and not a debt — and
// gating here (rather than caching a decline taken while unarmed) keeps the
// once-cache from freezing "no" for a value that outlives the unarmed phase.
//
// A decline IS cached: it is a property of the body, not of the moment.
func compiledLensSig(r *Registry, lu *lensUnit, segs []ReachSeg) *Signature {
	if lu == nil || r == nil || !r.RuntimeStampingEnabled() {
		return nil
	}
	lu.once.Do(func() {
		body := expandParenExpr(lowerReach(ReachInfo{
			Receiver: []Value{NewWord(lensParam)},
			Segments: segs,
		}))
		fd := FnDefInfo{Signatures: []FnSig{{
			Params:     []FnParam{{Name: lensParam, Type: TAny}},
			Returns:    []*Type{TAny},
			Impl:       Boru(body),
			BarrierPos: -1,
		}}}
		compiledRuntime.StampDetached(r, fd, SrcPos{})
		// The ref is the compiler piece's opaque handle (S4); core only asks
		// whether one landed, exactly as compiler.CompiledRef reads it.
		if bi, ok := fd.Signatures[0].Impl.(*BoruImpl); ok && bi.Compiled() != nil {
			lu.sig = &fd.Signatures[0]
		}
	})
	return lu.sig
}
