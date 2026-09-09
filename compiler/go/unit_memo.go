package compiler

import core "github.com/boru-lang/boru/core/go"

// ANALYSIS ORDER IS PROGRAM ORDER — for units (Stage 4b, 2026-09-04).
//
// The compile pass executes the program once, in order, and records each
// dispatch as it happens, so a top-level read bakes exactly the value the
// interpreter would read at that point. A fn/closure UNIT breaks that
// equality in one place: the unit memo. FnAnalysisKey keys a unit on scope,
// name, argument types, captures and body position, and OMITS the bindings
// of the enclosing-scope names the body READS — so a unit analysed at one
// program point is reused at every later call site whatever those bindings
// are there. Six silent default-lane miscompiles were the same fault seen
// from six sides (design/FULL-COMPILATION-HANDOFF.0.md, Stage 4a-4), and the
// freeze discipline answered each by REFUSING the program on a later rebind.
//
// This file makes the memo binding-sensitive instead, and it takes three
// pieces:
//
//   - THE BAKES. NoteFrozenRead records, on the OPEN unit, every enclosing-
//     scope binding the unit baked and the binding's DefTable generation at
//     the read (core passes Gen(name) from the registry the read resolved
//     in). StartFnCompile then treats a finished memo hit as STALE when any
//     bake — the unit's own, or one reachable through the units it calls or
//     pushes as closures — has a generation that has since moved, and
//     compiles a fresh unit for that call site. A stale unit keeps serving
//     the call sites that already reference it: each site's unit is right
//     for that site's program point, which is the top-level equality
//     restored for units.
//
//   - THE ESCAPING LATCH. A unit whose reference ESCAPES into a value — a
//     returned closure (render), a stamped fn value (stampOnly), a fn-value
//     closure (lambdaUnit) — cannot be re-recorded at a later call site,
//     because its later "call" is an apply of the value, invisible to the
//     memo. For those, and only those, NotifyNameRebound keeps the refusal
//     the discipline always made, with the same text.
//
//   - THE BODY RE-RUN ENVIRONMENT. A leaking body (`do`; the each/fold/scan
//     bodies the runtime re-runs per element) is analysed once with
//     recording SUSPENDED and then RE-RUN to compile. Measured, the re-run
//     began from the state the first run LEAKED, so every read before the
//     body's own rebind baked the rebound value: `def k 5  do [ k  def k 9
//     k ]` answered `9 9` compiled against the interpreter's `5 9`, and the
//     each/fold/scan/do-in-fn siblings likewise. The guards clone the
//     binding table when a body run opens and publish it when the run
//     closes; the compile re-run swaps that START table in (a `do` body
//     runs once, so the re-run is the run) or, for a multi-run body, the
//     start table with every name the body rebinds replaced by the JOIN of
//     its start and end bindings — iteration-varying, hence non-concrete,
//     hence a live read and a runtime re-match downstream.
//
// One hazard is deliberately REFUSED rather than fixed here, because the
// fix needs per-read identity the recorder does not have: a RE-PUSHABLE
// residual read — a live OpLookupDynScope or a loop-carried slot — is
// re-pushed at the END of its fragment, after a bind of the same name the
// fragment recorded later than the read. Measured `def k (1 add 4)  def f
// fn [[] [Integer Integer] [k  def k 9  k]]  f` → `9 9` against `5 9`, and
// `def k 5  for 2 [ k  def k 9 ]` → `9 9` against `5 9`, both before this
// file existed. residualReadHazard refuses that shape by name.

// --- the bakes ----------------------------------------------------------

// NoteFrozenRead records on the OPEN unit what it froze about an enclosing-
// scope binding read — the bake KIND, for the escaping latch's refusal text,
// and the binding's GENERATION at the read, the memo's staleness key. No-op
// at top level (analysis order is program order there) and for a stored-ref
// unit, whose rebind handling is NotifyNameRebound's per-ref poisoning; an
// unclassified note (FrozenBakeNone) is dropped rather than recorded as a
// default. FIRST BAKE WINS for both halves: a name frozen two ways is stale
// (or refused) either way, and the text must not depend on analysis order.
func (es *EmitState) NoteFrozenRead(name string, bake core.FrozenBake, gen int64) {
	if !es.Active() || name == "" || bake == core.FrozenBakeNone || len(es.openUnitRecs) == 0 {
		return
	}
	idx := es.openUnitRecs[len(es.openUnitRecs)-1]
	if idx < 0 || idx >= len(es.fnRecs) {
		return
	}
	rec := es.fnRecs[idx]
	if rec == nil || rec.storedRefUnit {
		return
	}
	if rec.frozen == nil {
		rec.frozen = map[string]core.FrozenBake{}
		rec.bakes = map[string]int64{}
	}
	if _, seen := rec.frozen[name]; !seen {
		rec.frozen[name] = bake
		rec.bakes[name] = gen
	}
}

// unitStale reports whether a memoised unit is stale for THIS call site: it
// (or a unit it references) baked a binding whose generation has moved
// since the bake. An UNFINISHED unit is never stale — an in-flight recursion
// must reuse the in-flight unit (its bakes are incomplete, and a second
// in-flight unit for the same key would bail its body empty). The walk is
// over the unit-reference graph with a visited set, so mutual recursion
// terminates.
func (es *EmitState) unitStale(u int) bool {
	seen := map[int]bool{}
	var stale func(int) bool
	stale = func(i int) bool {
		if i < 0 || i >= len(es.fnRecs) || seen[i] {
			return false
		}
		seen[i] = true
		rec := es.fnRecs[i]
		if rec == nil || !rec.finished {
			return false
		}
		reg := rec.reg
		if reg == nil {
			reg = es.reg
		}
		for name, gen := range rec.bakes {
			if reg == nil || reg.Defs == nil || reg.Defs.Gen(name) != gen {
				return true
			}
		}
		found := false
		es.forEachUnitRef(rec, func(j int) {
			if !found && stale(j) {
				found = true
			}
		})
		return found
	}
	return stale(u)
}

// forEachUnitRef calls fn with every Program.Fns index a unit's emission
// references: user calls (single and poly arms) and closure operands
// anywhere in its events, its residual, or its unit-start prefix. It is the
// reachability primitive both unitStale and the escaping latch walk, so a
// new way for a unit to name another unit must join it here or both walks
// go blind to it.
func (es *EmitState) forEachUnitRef(rec *fnUnitRec, fn func(int)) {
	if rec == nil {
		return
	}
	visitOp := func(op EmitOperand) {
		if op.kind == opClosure {
			fn(op.closureUnit)
		}
	}
	for i := range rec.outOps {
		visitOp(rec.outOps[i])
	}
	for i := range rec.retPrefix {
		visitOp(rec.retPrefix[i])
	}
	walkFrag(rec.frag, func(ev *EmitEvent) {
		if ev.kind == evCallUser {
			if ev.uc.unit >= 0 {
				fn(ev.uc.unit)
			}
			if ev.uc.poly != nil {
				for _, u := range ev.uc.poly.units {
					fn(u)
				}
			}
		}
		forEachOperand(ev, visitOp)
	}, visitOp)
}

// walkFrag visits every event of a fragment tree — the fragment's own
// events, then each branch's condition/then/else fragments and each loop's
// body, recursively — and every operand the fragment itself carries outside
// its events (a fragment's captured residual re-push — a loop body's inert
// list or a branch arm's whole one — and the body's per-iteration apply).
func walkFrag(frag *EmitFragment, evFn func(*EmitEvent), opFn func(EmitOperand)) {
	if frag == nil {
		return
	}
	for i := range frag.residualOps {
		opFn(frag.residualOps[i])
	}
	for i := range frag.applyArgs {
		opFn(frag.applyArgs[i])
	}
	if frag.applyFn != nil {
		opFn(*frag.applyFn)
	}
	for i := range frag.events {
		ev := &frag.events[i]
		evFn(ev)
		if ev.br != nil {
			walkFrag(ev.br.condFrag, evFn, opFn)
			walkFrag(ev.br.then, evFn, opFn)
			walkFrag(ev.br.els, evFn, opFn)
		}
		if ev.loop != nil {
			walkFrag(ev.loop.cond, evFn, opFn)
			walkFrag(ev.loop.body, evFn, opFn)
		}
	}
}

// unitEscapes reports whether a unit's reference leaves the program's call
// graph inside a VALUE: a returned closure (render is the fn value's own
// rendering), a stamped fn value (stampOnly), or a fn-value closure body
// (lambdaUnit). Such a unit is applied later through the value, never
// re-recorded at a call site, so the memo cannot refresh it.
func unitEscapes(rec *fnUnitRec) bool {
	return rec != nil && (rec.render != "" || rec.stampOnly || rec.lambdaUnit)
}

// frozenInEscapingUnit reports whether some escaping unit — directly or
// through the units it references — baked `name`, and what it baked.
func (es *EmitState) frozenInEscapingUnit(name string) (core.FrozenBake, bool) {
	seen := map[int]bool{}
	var found core.FrozenBake
	hit := false
	var walk func(int)
	walk = func(i int) {
		if hit || i < 0 || i >= len(es.fnRecs) || seen[i] {
			return
		}
		seen[i] = true
		rec := es.fnRecs[i]
		if rec == nil {
			return
		}
		if b, ok := rec.frozen[name]; ok {
			found, hit = b, true
			return
		}
		es.forEachUnitRef(rec, walk)
	}
	for i, rec := range es.fnRecs {
		if unitEscapes(rec) {
			walk(i)
		}
	}
	return found, hit
}

// --- the body re-run environment --------------------------------------------

// bodyStart is the binding table a leaking body's analysis run started from,
// published by its guard at close, keyed by the body value's ID.
type bodyStart struct {
	defs *core.DefTable
	reg  *core.Registry
}

// bodyRunEnv is the environment a leaking body's compile re-run executes in.
type bodyRunEnv struct {
	start *core.DefTable
	reg   *core.Registry
	// multi marks a body the runtime re-runs per element: names the body
	// rebinds are iteration-varying, so the re-run sees the JOIN of their
	// start and end bindings rather than either.
	multi bool
}

// publishBodyStart records a body run's start table for the dispatch record
// that follows. keepExisting says the run is a later round of a fixed point
// whose first round already published — the first round's start is the
// body's start, and a later round's is the previous round's leak.
func (es *EmitState) publishBodyStart(bodyID string, r *core.Registry, defs *core.DefTable, keepExisting bool) {
	if es == nil || bodyID == "" || defs == nil || !es.Active() {
		return
	}
	if es.bodyStarts == nil {
		es.bodyStarts = map[string]bodyStart{}
	}
	if keepExisting {
		if _, ok := es.bodyStarts[bodyID]; ok {
			return
		}
	}
	es.bodyStarts[bodyID] = bodyStart{defs: defs, reg: r}
}

// takeBodyEnv claims the published start for a leaking body and builds the
// environment its compile re-run runs in. Nil when the body's run published
// nothing (a body that is neither once-run nor multi-run keeps-defs, or one
// whose guard never ran) — the re-run then runs where it always did.
func (es *EmitState) takeBodyEnv(body core.Value, spec core.CallableSpec) *bodyRunEnv {
	if es == nil || body.ID == "" || !(spec.BodyOnceKeepsDefs || spec.BodyMultiRunKeepsDefs) {
		return nil
	}
	bs, ok := es.bodyStarts[body.ID]
	if !ok {
		return nil
	}
	delete(es.bodyStarts, body.ID)
	return &bodyRunEnv{start: bs.defs, reg: bs.reg, multi: spec.BodyMultiRunKeepsDefs}
}

// enter swaps the re-run environment in and returns the table it replaced —
// the LEAKED table, which exit puts back so the enclosing analysis continues
// from the state the interpreter's run leaves. Each compile (probe, real,
// every extra hook) enters afresh from its own clone, because a compile
// mutates the table it runs on.
//
// ok=false declines the closure: a multi-run body that UNBINDS a name present
// at its start (the second iteration's read would be undefined_word where the
// first's was a value — no environment expresses that), or one whose
// rebound binding is a function value (a per-iteration call target the unit
// cannot resolve live). A nil env, or one taken on a different registry,
// enters nothing.
func (env *bodyRunEnv) enter(r *core.Registry) (*core.DefTable, bool) {
	if env == nil || r == nil || env.reg != r || env.start == nil {
		return nil, true
	}
	table := env.start.Clone()
	// TYPE bindings the body installed are carried over from the leaked
	// table rather than restored to absent. A type the body minted leaves
	// its lattice part registered whether or not its binding is restored,
	// and a re-run that re-defines the name over a surviving part without
	// its binding trips the parts conflict instead of the type-shadow path
	// (core RunCarrierBodyKeepDefs records the same hazard for the analysis
	// run); an alias is carried for uniformity, so every type re-def in the
	// re-run takes the shadow path the re-run has always taken. A read of
	// the type BEFORE the body's own def is the interpreter's undefined_word,
	// and the analysis run — which saw the name absent — already reported
	// it, so nothing here can bake a read the interpreter refuses.
	for _, name := range r.Defs.Names() {
		e, ok := r.Defs.TopEntry(name)
		if !ok || e.TypeDef == nil || table.Has(name) {
			continue
		}
		if e.Minted {
			table.PushType(name, e.TypeDef, e.Body)
		} else {
			table.PushTypeAdopted(name, e.TypeDef, e.Body)
		}
	}
	if env.multi {
		for _, name := range table.Names() {
			sv, _ := table.Top(name)
			ev, present := r.Defs.Top(name)
			if !present {
				return nil, false
			}
			// Unchanged: the same value by identity — or, for values minted
			// without one (outside a pass), by equality. An empty id on both
			// sides must not read as "same", or a runtime-minted rebind would
			// pass as its own start binding.
			if sv.ID == ev.ID && (sv.ID != "" || core.ValuesEqual(sv, ev)) {
				continue
			}
			if entry, _ := table.TopEntry(name); entry.TypeDef != nil {
				// A type rebound per element: the arm-resident bridge refuses
				// type installs already, so the start binding stands here.
				continue
			}
			if core.IsFnValueResidual(sv) || core.IsFnValueResidual(ev) {
				return nil, false
			}
			table.Replace(name, core.JoinCarriers(bindCarrier(sv), bindCarrier(ev)))
		}
	}
	return r.SwapDefs(table), true
}

// exit restores the table enter replaced.
func (env *bodyRunEnv) exit(r *core.Registry, prev *core.DefTable) {
	if env == nil || prev == nil || r == nil {
		return
	}
	r.SwapDefs(prev)
}

// bindCarrier is the check-model carrier of a binding's value: a carrier
// already is one; a concrete value contributes its type. A List or Map keeps
// its container shape (the ChildTypeInfo a concrete-container slot needs);
// a nil-Parent root literal is the gradual Any.
func bindCarrier(v core.Value) core.Value {
	if v.Carrier {
		return v
	}
	if v.Parent == nil {
		return core.NewDynamicCarrier(core.TAny)
	}
	switch {
	case v.Parent.ConformsTo(core.TList):
		return core.NewCarrier(core.TList)
	case v.Parent.ConformsTo(core.TMap):
		return core.NewCarrier(core.TMap)
	}
	return core.NewCarrier(v.Parent)
}

// --- the residual-order hazard ----------------------------------------------

// readKey / slotKey key the hazard tables by FRAGMENT: a residual belongs
// to the fragment that produced it, so only that fragment's own reads can
// be the residual's read — a read inside a nested fragment reaches the
// enclosing residual only through the nested construct's result, which
// carries its own provenance.
type readKey struct {
	name string
	frag int
}

type slotKey struct {
	slot int
	frag int
}

// curFrag is the innermost open fragment's id, 0 at the root.
func (es *EmitState) curFrag() int {
	if es == nil || len(es.fragIDs) == 0 {
		return 0
	}
	return es.fragIDs[len(es.fragIDs)-1]
}

// noteFragRead records that the innermost open fragment has read `name`.
func (es *EmitState) noteFragRead(name string) {
	if es == nil || name == "" {
		return
	}
	if es.fragReads == nil {
		es.fragReads = map[readKey]bool{}
	}
	es.fragReads[readKey{name, es.curFrag()}] = true
}

// noteBindHazard is called after a bind of `name` is recorded: every OPEN
// fragment that already read the name — the innermost, and each enclosing
// one — now holds a read that precedes this bind, so a residual re-push of
// that read would execute after the bind. A read in an open fragment
// necessarily precedes a bind recorded now, which is why the check is
// membership, not order.
func (es *EmitState) noteBindHazard(name string) {
	if es == nil || name == "" {
		return
	}
	mark := func(fid int) {
		if !es.fragReads[readKey{name, fid}] {
			return
		}
		if es.bindHazard == nil {
			es.bindHazard = map[readKey]bool{}
		}
		es.bindHazard[readKey{name, fid}] = true
	}
	mark(0)
	for _, fid := range es.fragIDs {
		mark(fid)
	}
}

// noteStoreHazard is noteBindHazard for a loop-carried STORE into a frame
// slot (RecordDefRebind): the slot, not the name, is what the residual's
// operand names, so the slot is marked alongside.
func (es *EmitState) noteStoreHazard(name string, slot int) {
	if es == nil || name == "" {
		return
	}
	es.noteBindHazard(name)
	mark := func(fid int) {
		if !es.fragReads[readKey{name, fid}] {
			return
		}
		if es.storeHazard == nil {
			es.storeHazard = map[slotKey]bool{}
		}
		es.storeHazard[slotKey{slot, fid}] = true
	}
	mark(0)
	for _, fid := range es.fragIDs {
		mark(fid)
	}
}

// residualReadHazard reports the refusal for a residual value of fragment
// frag whose operand is a RE-PUSHABLE read of a binding — a live dyn-scope
// lookup, or a loop-carried slot — that the fragment read BEFORE a bind of
// the same name it (or a fragment nested in it) recorded: the re-push at the
// fragment's end would read the rebound value where the interpreter pushed
// the earlier one. Empty when the residual is safe.
func (es *EmitState) residualReadHazard(v core.Value, op EmitOperand, frag *EmitFragment) string {
	if es == nil {
		return ""
	}
	fid := 0
	if frag != nil {
		fid = frag.id
	}
	switch op.kind {
	case opDynScope:
		name := v.DynFrom()
		if name == "" {
			name = es.defReads[v.ID]
		}
		if name != "" && es.bindHazard[readKey{name, fid}] {
			return "residual read of `" + name + "` precedes its rebind in the same body (Stage 4b)"
		}
	case opLocal:
		if es.storeHazard[slotKey{op.idx, fid}] {
			name := es.defReads[v.ID]
			if name == "" {
				name = "a loop-carried def"
			}
			return "residual read of `" + name + "` precedes its rebind in the same body (Stage 4b)"
		}
	}
	return ""
}

// residualStands settles one residual value of fragment frag for its
// caller — a fn unit's finish, a branch arm, a loop body — and reports
// whether it stands. ok is the caller's own resolution verdict (resolveOperand
// and whatever the site layers on it); a residual the caller could not
// resolve refuses as "<what> of unknown provenance", a resolved one refuses
// under residualReadHazard, and a fn-value lead a later dispatch collected
// past refuses as the collection hazard (hazardLead, NUR121 — `[g x drop]`
// leaves the model's `g` where the interpreter's `g` already ran over `x`).
// All three refusals share this ONE MarkUncompilable site on purpose: the
// refusal-site census (test/go/langspec) is a downward ratchet over call
// sites, and each hazard is another reason at the same four sites, not a
// fifth site.
func (es *EmitState) residualStands(prefix string, v core.Value, op EmitOperand, ok bool, frag *EmitFragment, what string) bool {
	if es == nil {
		return false
	}
	reason := ""
	switch {
	case !ok:
		reason = what + " of unknown provenance"
	case es.hazardLeadIn(v, frag):
		reason = what + " is a fn-value lead a later dispatch collected past (NUR121)"
	default:
		reason = es.residualReadHazard(v, op, frag)
	}
	if reason == "" {
		return true
	}
	es.MarkUncompilable(prefix + reason)
	return false
}
