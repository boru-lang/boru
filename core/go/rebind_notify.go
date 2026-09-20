package core

// The REBIND NOTIFICATION funnel.
//
// A compiled fn/closure unit can BAKE a module-scope binding three ways — the
// read's value as a PUSH_CONST, its type identity as a PUSH_TYPE, or a call
// target as a CALL_USER (core.FrozenBake). Each bake is frozen for the life of
// the unit while the interpreter re-resolves the name per call, so any later
// change to that binding must reach the recorder: `NotifyNameRebound` declines
// the program, and the interpreter answers it correctly instead.
//
// WHY THIS FILE EXISTS. Until 2026-09-04 the notification was called from the
// individual `def` / `undef` WORD HANDLERS in basic, and every binder that
// returned before reaching one skipped it silently. Four did, and each was a
// live miscompile on the default lane — `boru run`, no flags, exit 0, wrong
// answer:
//
//   - `DefHandler`'s capitalised arm returned `InstallType` directly;
//   - `UndefHandler`'s capitalised arm returned after its own ledger note;
//   - `UndefFnHandler` returned after `UninstallFnSigs` with no notification
//     of any kind;
//   - and on the READ side the same shape, three times over.
//
// The lesson those four cost is that a notification attached to HANDLERS is a
// notification each new handler can forget. So it is seated HERE instead, at
// core's binding OPERATIONS — the layer every binder must go through to bind
// anything at all. A word library cannot bind a name without calling one of
// them, so it cannot skip the notification by construction.
//
// WHY NOT LOWER. `DefTable.Push` and friends were the obvious floor and are
// the wrong one: they carry frame bindings, guard narrowings, generic
// parameter installs and carrier joins as well as user rebinds. A narrowing
// push at module scope (`if (k is Integer) […]`) would then decline every
// program with a frozen `k`. The semantic binding operation — install a def,
// uninstall a def, install a type, uninstall a type, uninstall signatures —
// is the level at which "the user rebound this name" is actually true.
//
// WHY NOT ON THE TWIN NOTE. `NoteBindTransition` is already seated at exactly
// these sites and looks like a free ride, but its population is NARROWER than
// this one's in two directions that both lose compile failures. It suppresses on
// `RolledBackBodyDepth > 0`, which would drop the `if`-arm rebind the latch
// declines today; and `UninstallFnSigs` notes only when a removal actually
// COMMITS, so a sig-undef whose every match is locked would stop notifying.
// The two answer different questions — "what must the VM replay" versus "what
// might have gone stale in the bytecode" — and the second is deliberately the
// wider, more conservative set.
//
// The scope decision is NOT made here. Every call is unconditional, and the
// recorder's own `rebindReachesModuleScope` decides whether the rebind reaches
// the module-scope binding set (a fn-body-local def does not, and its
// notification is a no-op). Keeping the funnel dumb is what lets a new binding
// operation join it without re-deriving that reasoning.
//
// `TestBindingOpsNotifyRebind` drives every operation below and fails if one
// stops notifying — the gate that makes the original defect class impossible
// to reintroduce quietly.
func noteRebind(r *Registry, name string) {
	if r == nil || r.Check == nil || name == "" {
		return
	}
	r.Check.Recorder().NotifyNameRebound(name)
}

// UninstallType removes a capitalised (TYPE) binding: it pops the entry,
// retires the lattice node the binding MINTED, notes the runtime-visible
// removal for the bind twins, and notifies the rebind. Reports whether a
// binding was there to remove.
//
// It lives in core, beside the other binding operations, because it IS one —
// it was inline in basic's `undef` handler until 2026-09-04, which is how it
// came to be the one unbinder with no rebind notification at all
// (`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  undef T  f` answered
// `true true` compiled against the interpreter's `undefined_word`).
func UninstallType(r *Registry, name string) bool {
	if r == nil {
		return false
	}
	entry, ok := r.Defs.PopEntry(name)
	if !ok {
		return false
	}
	// Retire only a node THIS binding minted. An alias binding ADOPTS an
	// existing canonical node (`def Foo Integer` binds the Integer node
	// itself — InstallType's alias arm), so retiring it here would delete a
	// builtin's or another binding's identity from the ID index.
	if entry.TypeDef != nil && entry.Minted {
		r.Types.Retire(entry.TypeDef)
	}
	// A capitalised undef pops through PopEntry, not UninstallDef, so it needs
	// its own ledger note (§6.5). The RETIREMENT above is already covered by
	// BindingSandbox's partition — this records the BINDING removal, which is
	// the half a twin has to replay.
	r.NoteBindTransition(BindUndef, name, SrcPos{})
	noteRebind(r, name)
	return true
}

// noteBindingRead is the READ half's funnel, and the twin of noteRebind above:
// one place that decides whether a resolved binding read freezes into the unit
// being analysed, and as WHAT.
//
// It exists for the reason the rebind funnel does. The three resolution paths
// in stepWord / stepWordVal each carried their own copy of this decision, and
// the copies disagreed: the type arm noted unconditionally, the value arm
// required IsConcrete, and the `/v` arm — which reaches NEITHER of the other
// two, because it resolves through ResolveRef before they run — noted nothing
// at all. That last gap was two live miscompiles, one of them older than the
// discipline's type arm:
//
//	def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  def T String  f
//	  interpreted -> true false      compiled -> true true
//	def k 5  def f fn [[] [Integer] [k/v add 2]]  f  def k 9  f
//	  interpreted -> 7 11            compiled -> 7 7
//
// So the classification lives here once and every path hands it the RESOLVED
// value. A new resolution path gets the same answer as the old ones by
// construction rather than by its author remembering the rule.
//
// The two arms mirror what resolveOperand does with the value downstream: a
// bare type node lowers to OpPushType, which bakes the node's compile-time
// IDENTITY, and it does so unconditionally — there is exactly one arm, so
// there is nothing to be concrete about. A value bakes only when it is
// concrete; a carrier (a module-scope `flex`, a computed def) routes to a live
// OpLookupDynScope instead and freezes nothing.
//
// NoteFrozenRead self-guards on an open unit, so a TOP-LEVEL read records
// nothing: there analysis order is program order and the bake IS the read the
// interpreter makes.
//
// The note carries the binding's GENERATION (DefTable.Gen) as of this read,
// from the registry the read resolved in — the key the compiler's
// binding-sensitive unit memo compares at a later call site. A type read also
// registers the read's value ID against the name (NoteDefRead), exactly as
// stepWord's value arm already does for a value read: the two arms must be
// nameable the same way downstream, or a resolution path that only one of
// them takes answers the freeze question differently.
func (e *Engine) noteBindingRead(name string, v Value) {
	if e == nil || e.Registry == nil || name == "" || !e.Registry.analysisActive() {
		return
	}
	if !ModuleScopeBinding(e.Registry, name) {
		return
	}
	gen := e.Registry.Defs.Gen(name)
	switch {
	case IsBareTypeNode(v):
		e.Registry.analysisRecorder().NoteDefRead(v.ID, name)
		e.Registry.analysisRecorder().NoteFrozenRead(name, FrozenBakeType, gen)
	case IsConcrete(v):
		e.Registry.analysisRecorder().NoteFrozenRead(name, FrozenBakeValue, gen)
	}
}
