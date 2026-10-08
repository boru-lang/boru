package compiler

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// A BODY-MAP word — `Rand.map-from` — is a CompileDynBody signature with no
// CallableSpec whose one NoEvalMapArgs operand is a Map of code bodies: the
// handler runs each value, in key order, as a run-time token body on the
// shared registry (InvokeBody; eng vm_token_body.go hosts it as a
// run-time-stamped unit). Its dispatch records as the plain call it is; what
// the recorder owes beyond that is `do`'s over a computed body
// (tryRecordDynBody), stated per fact below.

// bodyMapFacts is what a body-map run's lowering asks of the loops open
// around it (lowerer.bodyMapReason): the names its bodies read (names), and
// whether they may read ANY name — a body the pass never saw (computed), or
// a token that reads a name no walk lists (opaque: a splice, an interpolated
// hole, a sugar marker) — and whether a body escapes with break/continue
// (flow).
type bodyMapFacts struct {
	names    map[string]bool
	opaque   bool
	computed bool
	flow     bool
}

// noteBodyMapRun is the recorded run of a body-map word, right after its
// dispatch recorded:
//
//   - the bodies resolve names against the registry, so a body naming
//     anything needs the program's DYNAMIC-ENVIRONMENT mirror (es.dynEnv),
//     where every def and named param is registry-visible;
//   - a body the pass never saw — the schema is a carrier (a Map param), or
//     a value in it is (`{a: b}` over a List param) — may bind any name for
//     the rest of the program, as the interpreter keeps a generator body's
//     defs: the kept-defs latch arms (runKeptDefs) and the names the unit
//     defined before the call are taken as leaked (noteDynKeepDefsLeak), so
//     a later read seats live on the registry or the compile declines
//     (NUR210's discipline), never reading the model's stale binding
//     (NUR330: `def k 5 def f fn [[m:Map][Any][Rand.map-from m]] end f {a:
//     (quote [def k 1 k])} k` answered [{a:1} 5] for the interpreter's
//     [{a:1} 1]);
//   - the facts the lowering tests against the open loops ride the call
//     (emitCall.bodyMap): a loop's index is a frame slot no run-time token
//     body resolves, and a break/continue with no compiled loop to take it
//     has no compiled answer (NUR353).
//
// A literal body the pass modelled (the word's ReturnsFn runs it,
// native.AnalyseMultiRunBody) keeps its own record.
func noteBodyMapRun(r *core.Registry, word string, sig *core.Signature, args []core.Value, pos core.SrcPos) {
	es, _ := r.Check.Recorder().(*EmitState)
	if es == nil || !es.Active() || sig == nil || sig.Callable != nil || !sig.CompileEffect.Has(core.CompileDynBody) {
		return
	}
	mp, ok := soleNoEvalMapSlot(sig, len(args))
	if !ok {
		return
	}
	facts := bodyMapShape(r, args[mp])
	if facts.computed || facts.opaque || len(facts.names) > 0 {
		es.dynEnv = true
	}
	if facts.computed {
		es.noteDynKeepDefsLeak(pos)
		es.runKeptDefs(word)
	}
	frame := es.frames[len(es.frames)-1]
	if n := len(frame); n > 0 && frame[n-1].kind == evCall && frame[n-1].call.word == word {
		frame[n-1].call.bodyMap = &facts
	}
}

// soleNoEvalMapSlot is the one NoEvalMapArgs position of a signature taking n
// operands, or ok=false when it has none or several.
func soleNoEvalMapSlot(sig *core.Signature, n int) (int, bool) {
	mp := -1
	for i := 0; i < n; i++ {
		if sig.NoEvalMapArgs[i] {
			if mp >= 0 {
				return -1, false
			}
			mp = i
		}
	}
	return mp, mp >= 0
}

// bodyMapShape reads a body map operand's facts: computed when the map, or
// any value in it, is not concrete (its tokens exist only at run time); the
// scan still reads every concrete value, so a literal body's escape is seen
// whatever the key order. A value is a body in whatever form the handler
// takes one (RequireConcreteList: a list, a flex list); one it refuses runs
// nothing. The names are read through the fns a body calls (bodyMapWalk),
// and the escape by check's deep sentinel scan.
func bodyMapShape(r *core.Registry, schema core.Value) bodyMapFacts {
	facts := bodyMapFacts{names: map[string]bool{}}
	m, err := core.RequireConcreteMap(schema, "")
	if err != nil {
		facts.computed = true
		return facts
	}
	quoted := bodyMapFacts{names: map[string]bool{}}
	w := &bodyMapWalk{facts: &facts, reg: r, seen: map[string]bool{},
		data: &bodyMapWalk{facts: &quoted, reg: r, seen: map[string]bool{}}}
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		if !core.IsConcrete(v) {
			facts.computed = true
			continue
		}
		body, err := core.RequireConcreteList(v, "")
		if err != nil {
			continue
		}
		w.seq(body.Slice())
		facts.flow = facts.flow || check.BodyHasSentinelDeep(r, core.NewList(body.Slice()))
	}
	// Quoted data is read as code only by a word that runs a value (`do`,
	// `each`, `apply`, a code-body word): where the bodies — or a fn they
	// call — name one, what the quoted data reads counts too.
	if w.runsData {
		for n := range quoted.names {
			facts.names[n] = true
		}
		facts.opaque = facts.opaque || quoted.opaque
	}
	return facts
}

// bodyMapWalk collects what a body map's bodies read into facts: the words
// the bodies read and — resolved once each (seen) — the words of the boru
// fns they call, whose free words resolve in the registry at the call just
// as the body's own do. QUOTED data (a quoted list, the operand of a
// `quote` word) goes to the data walker instead, whose words are read only
// where a word that runs a value takes them (runsData). The data walker
// (data nil) resolves no callee.
type bodyMapWalk struct {
	facts    *bodyMapFacts
	reg      *core.Registry
	seen     map[string]bool
	data     *bodyMapWalk
	runsData bool
}

// seq walks a token sequence. The token after a `quote` word is its
// operand, data — a paren group excepted: it evaluates before quote takes
// its result.
func (w *bodyMapWalk) seq(toks []core.Value) {
	for i := 0; i < len(toks); i++ {
		w.walk(toks[i])
		if wi, err := core.AsWord(toks[i]); err == nil && wi.Name == "quote" && i+1 < len(toks) && !core.IsParenExpr(toks[i+1]) {
			i++
			w.quotedData(toks[i])
		}
	}
}

// quotedData hands v, quoted data, to the data walker (the data walker
// reads it where it stands).
func (w *bodyMapWalk) quotedData(v core.Value) {
	v.Quoted = false
	if w.data != nil {
		w.data.walk(v)
		return
	}
	w.walk(v)
}

// walk collects the names v reads: a word's own (and its callee's), and
// through lists, maps, paren groups, a reach's receiver and computed keys,
// and a fn value's bodies. A quoted list is data. A token that reads a name
// no walk can list — a splice, an interpolated hole, a sugar marker — makes
// the body opaque.
func (w *bodyMapWalk) walk(v core.Value) {
	if v.Quoted {
		w.quotedData(v)
		return
	}
	if core.IsSplice(v) || core.IsInterpString(v) || core.IsXmlInterp(v) || core.IsSugar(v) {
		w.facts.opaque = true
		return
	}
	switch d := v.Data.(type) {
	case core.WordInfo:
		w.facts.names[d.Name] = true
		w.callee(d.Name)
	case core.FnDefInfo:
		w.fnBodies(&d)
	case core.ListPayload:
		w.seq(d.Elems)
	case core.MapPayload:
		if d.M != nil {
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				w.walk(mv)
			}
		}
	case core.ParenExprPayload:
		w.seq(d.Toks)
	case core.ReachInfo:
		w.seq(d.Receiver)
		for i := range d.Segments {
			w.seq(d.Segments[i].KeyExpr)
		}
	}
}

// callee resolves a word the bodies read to what a call of it dispatches
// over (Lookup) and reads that, once per name. The data walker resolves
// nothing: data calls nothing.
func (w *bodyMapWalk) callee(name string) {
	if w.data == nil || w.seen[name] {
		return
	}
	w.seen[name] = true
	if fd := w.reg.Lookup(name); fd != nil {
		w.fnBodies(fd)
	}
}

// fnBodies reads every overload of fd: a boru body in the fn's own home
// registry, a native's signature for whether it runs a value.
func (w *bodyMapWalk) fnBodies(fd *core.FnDefInfo) {
	home, _ := core.FnHome(w.reg, fd)
	prev := w.reg
	w.reg = home
	for i := range fd.Signatures {
		sig := &fd.Signatures[i]
		if _, boru := sig.Impl.(*core.BoruImpl); boru {
			w.seq(sig.Body())
			continue
		}
		w.runsData = w.runsData || sigRunsValue(sig)
	}
	w.reg = prev
}

// sigRunsValue reports whether a native signature may run a value handed to
// it as code: a code-body slot, a callable body, a body-running, storing or
// re-stepping compile effect, or a Function operand.
func sigRunsValue(sig *core.Signature) bool {
	const runs = core.CompileDynBody | core.CompileFallbackBody | core.CompileResteps | core.CompileRunsBodyIsolated |
		core.CompileRunsBodyOnRegistry | core.CompileStoresBody | core.CompileStoresBodyList | core.CompileStoresFn |
		core.CompileOwnLowering
	if sig.Callable != nil || sig.CompileEffect&runs != 0 {
		return true
	}
	for _, t := range sig.Args {
		if t != nil && t.ConformsTo(core.TFunction) {
			return true
		}
	}
	// An unevaluated slot is a code body unless the word declares it inert
	// data or a key it reads (`quote`, `enum`, `unpack`'s names).
	return (len(sig.NoEvalArgs) > 0 || len(sig.NoEvalMapArgs) > 0) &&
		!sig.CompileEffect.Has(core.CompileQuoteInert|core.CompileQuoteKey)
}

// bodyMapReason is a body-map run's lowering test against the loops open
// around it in this unit (lw.loops), "" when it lowers:
//
//   - a body that escapes with break/continue outside any compiled loop of
//     the program (the root, no loop open — lowerBreak's own rule): the
//     interpreter raises its flow_error where its tape stands, which no
//     compiled op reproduces, and the VM's escaped signal found no loop to
//     take it (an internal_error, NUR353). Inside a loop the escape takes
//     the loop on both lanes.
func (lw *lowerer) bodyMapReason(c *emitCall) string {
	facts := c.bodyMap
	if facts == nil {
		return ""
	}
	// A body reading a counted loop's index — by name, or any name at all
	// (computed, opaque) — armed the program's dynamic-environment mirror
	// (noteBodyMapRun), under which every counted loop publishes its index
	// on the registry (lowerer.publishesIndex): `for 2 [Rand.map-from
	// {a:[i]}]` reads the index as the interpreter's does, in this unit or
	// through a fn the loop calls (NUR353, NUR354).
	if facts.flow && len(lw.loops) == 0 && !lw.isFnUnit {
		return "`" + c.word + "`: a body's break/continue outside a compiled loop (Stage 2, NUR353)"
	}
	return ""
}
