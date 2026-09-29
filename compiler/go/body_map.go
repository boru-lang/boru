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
// hole) — and whether a body escapes with break/continue (flow).
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
// any value in it, is not concrete (its tokens exist only at run time); else
// the names its values read and whether one escapes (check's deep sentinel
// scan, through the fns a body calls).
func bodyMapShape(r *core.Registry, schema core.Value) bodyMapFacts {
	facts := bodyMapFacts{names: map[string]bool{}}
	m, err := core.RequireConcreteMap(schema, "")
	if err != nil {
		facts.computed = true
		return facts
	}
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		if !core.IsConcrete(v) {
			facts.computed = true
			return facts
		}
		facts.walk(v)
		facts.flow = facts.flow || check.BodyHasSentinelDeep(r, v)
	}
	return facts
}

// walk collects the names v's tokens read: a word's own, and through lists,
// maps, paren groups and a reach's receiver and computed keys. A token that
// reads a name no walk can list — a splice, an interpolated hole — makes the
// body opaque.
func (f *bodyMapFacts) walk(v core.Value) {
	if core.IsSplice(v) || core.IsInterpString(v) || core.IsXmlInterp(v) {
		f.opaque = true
		return
	}
	switch d := v.Data.(type) {
	case core.WordInfo:
		f.names[d.Name] = true
	case core.ListPayload:
		f.walkAll(d.Elems)
	case core.MapPayload:
		if d.M != nil {
			for _, k := range d.M.Keys() {
				mv, _ := d.M.Get(k)
				f.walk(mv)
			}
		}
	case core.ParenExprPayload:
		f.walkAll(d.Toks)
	case core.ReachInfo:
		f.walkAll(d.Receiver)
		for i := range d.Segments {
			f.walkAll(d.Segments[i].KeyExpr)
		}
	}
}

// walkAll walks each of vs.
func (f *bodyMapFacts) walkAll(vs []core.Value) {
	for _, v := range vs {
		f.walk(v)
	}
}

// bodyMapReason is a body-map run's lowering test against the loops open
// around it in this unit (lw.loops), "" when it lowers:
//
//   - a body that reads a counted loop's index — by name, or any name at all
//     (computed, opaque) — reads the frame slot the loop keeps it in on the
//     compiled lane, which no run-time token body resolves: the interpreter's
//     `for` binds the index in the registry, so `for 2 [Rand.map-from
//     {a:[i]}]` answered [{a:0} {a:1}] there and raised undefined_word i
//     compiled (NUR353);
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
	for _, lc := range lw.loops {
		if lc.iterName != "" && (facts.computed || facts.opaque || facts.names[lc.iterName]) {
			return "`" + c.word + "`: a body may read the loop index `" + lc.iterName +
				"`, a frame slot the run-time token body cannot resolve (NUR353)"
		}
	}
	if facts.flow && len(lw.loops) == 0 && !lw.isFnUnit {
		return "`" + c.word + "`: a body's break/continue outside a compiled loop (Stage 2, NUR353)"
	}
	return ""
}
