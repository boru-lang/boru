package check

import (
	core "github.com/boru-lang/boru/core/go"

	"errors"
	"sort"
	"strconv"
	"strings"
)

// Carrier-based static type-checking support.
//
// A "carrier" is a normal Value with Carrier=true and (typically)
// Data=nil: it carries only type information, not a concrete payload.
// The engine is driven in check mode by Registry.Check.Mode. In that
// mode, the same dispatch machinery (matchSignature, forward
// collection, sort order, etc.) runs, but execMatch consults
// Signature.Returns to synthesise carrier results instead of calling
// the handler. This keeps runtime and checker in absolute parity.
//
// What lives HERE is the analysis PASS over carriers: the carrier-result
// builder for a matched signature, the concrete→carrier strip, dispatch
// modelling (disjunct partitioning, dynamic-overload reachability), and
// the fn / loop body models with their memoisation, recursion bailing,
// per-shape quota and Kleene fixed point.
//
// What does NOT live here any more, since ADR-013's 2026-08-08
// amendment, is the carrier VOCABULARY those passes are written in —
// the join lattice, the body runners, guard narrowing, the carrier
// constructors, dead-overload detection. Every one of those was a pure
// function over core types, so they moved down to core/go
// (carrier_join.go, carrier_body.go, guard_narrow.go, carrier_new.go,
// deadsig.go), which is what lets `basic` carry the analysis half of
// its control words without depending on this module. The test when
// adding a helper: if all its operands are core types, it belongs
// below.

// stackHasVariadic reports whether any entry of a residual stack is a
// variadic-spread carrier.
func stackHasVariadic(stk []core.Value) bool {
	for _, v := range stk {
		if _, ok := core.IsVariadicSpread(v); ok {
			return true
		}
	}
	return false
}

// NewCarrierTypedListLen constructs a typed-list carrier with a
// statically-known length, so a downstream index check can reason
// about a computed list (e.g. `iota n`). n MUST be the exact length
// or an upper bound — never an underestimate (see ChildTypeInfo.Len).
func NewCarrierTypedListLen(elem *core.Type, n int) core.Value {
	v := core.NewCarrierTypedList(elem)
	if ct, ok := v.Data.(core.ChildTypeInfo); ok {
		ln := n
		ct.Len = &ln
		v.Data = ct
	}
	return v
}

// ReturnsPreserveListAt builds a ReturnsFunc that returns a typed-
// list carrier whose element type matches the data-list arg at
// index i. Used by list-preserving words like reverse, take, shed,
// unique, at, sortby — they return a list of the same element type
// as their input.
func ReturnsPreserveListAt(i int) core.ReturnsFunc {
	return func(args []core.Value, _ *core.Registry) []core.Value {
		if i < 0 || i >= len(args) {
			return []core.Value{core.NewCarrier(core.TList)}
		}
		elem := DataListElemTypeFromValue(args[i])
		out := core.NewCarrierTypedList(elem)
		// Copy the source's element constraint onto the residual so the check-mode
		// write mirror fires: d2CheckWrite consults ElemConstraint (the elem
		// pointer), which NewCarrierTypedList sets in ChildTypeInfo.Child but not
		// as the pointer — so `(reverse xs) set 0 "bad"` for xs:[:Integer] is
		// diagnosed at check, mirroring the tagged runtime result (#9, round 9).
		if ec, ok := args[i].ElemConstraint(); ok {
			out.SetElemConstraint(ec)
		}
		return []core.Value{out}
	}
}

// ReturnsListElemAt builds a ReturnsFunc that returns the element
// type carrier of the data-list arg at index i. Used by words like
// head/first (if added) that pick a single element out of a list.
func ReturnsListElemAt(i int) core.ReturnsFunc {
	return func(args []core.Value, _ *core.Registry) []core.Value {
		if i < 0 || i >= len(args) {
			return []core.Value{core.NewCarrier(core.TAny)}
		}
		elem := DataListElemTypeFromValue(args[i])
		return []core.Value{core.NewCarrier(elem)}
	}
}

// NewElementCarrier builds the per-invocation element carrier a higher-order
// body (each / fold / scan / filter) sees. When the element type is UNKNOWN —
// TAny from an UNTYPED list (`Test.results end` declares `[List]`, so there is no
// ChildTypeInfo to read the element from) — the carrier is DYNAMIC, so a
// downstream access in the body (`get`, a field read) matches OPTIMISTICALLY
// instead of failing `no_signature` against the bare Any root, exactly as a
// declared-Any return does (CarrierResults). A KNOWN element type stays a strict
// carrier — its real shape is checked normally. Sound under the dynamic-modality
// framework: it only loosens matching, and a guard discharges it back to strict.
func NewElementCarrier(t *core.Type) core.Value {
	c := core.NewCarrier(t)
	if t == nil || t.Equal(core.TAny) {
		c.Dynamic = true
	}
	return c
}

// ElementCarrierFromValue is the check-mode carrier a higher-order body sees
// for one element of data. For a CONCRETE heterogeneous list (or map, whose
// value-bodies see the values) the element is the lattice JOIN of the element
// types — built via core.JoinCarriers, the same join branch merges use: direct
// siblings collapse to the shared parent, DISTANT cousins stay a strict
// Disjunct, so the body dispatch distributes per alternative
// (disjunctPartitionReturns) exactly as the runtime dispatches per element —
// `[1 2 "s"] each [1 add]` matches add's Integer AND String overloads instead
// of failing no_signature against the Scalar ancestor. Every other shape
// (typed lists, empty/single-typed collections, carriers) keeps
// NewElementCarrier(DataListElemTypeFromValue(data)) — an untyped element
// stays a dynamic Any.
func ElementCarrierFromValue(data core.Value) core.Value {
	if core.IsConcrete(data) {
		if joined, ok := joinedElementCarrier(data); ok {
			return joined
		}
	}
	return NewElementCarrier(DataListElemTypeFromValue(data))
}

// joinedElementCarrier joins the element types of a concrete plain list (or
// the value types of a concrete map) via core.JoinCarriers. ok=false when the
// collection is empty, single-typed (the plain-type path is already precise),
// or not a plain list/map payload.
func joinedElementCarrier(data core.Value) (core.Value, bool) {
	var elems []core.Value
	switch p := data.Data.(type) {
	case core.ListPayload:
		elems = p.Elems
	case core.MapPayload:
		if p.M == nil {
			return core.Value{}, false
		}
		for _, k := range p.M.Keys() {
			v, _ := p.M.Get(k)
			elems = append(elems, v)
		}
	default:
		return core.Value{}, false
	}
	if len(elems) < 2 || elems[0].Parent == nil {
		return core.Value{}, false
	}
	mixed := false
	for i := 1; i < len(elems); i++ {
		if elems[i].Parent == nil {
			return core.Value{}, false
		}
		if !elems[i].Parent.Equal(elems[0].Parent) {
			mixed = true
		}
	}
	if !mixed {
		return core.Value{}, false
	}
	out := core.NewCarrier(elems[0].Parent)
	for i := 1; i < len(elems); i++ {
		out = core.JoinCarriers(out, core.NewCarrier(elems[i].Parent))
	}
	return out, true
}

// ParamInputCarrier builds the check-mode carrier for a fn parameter declared of
// type t. An EXPLICITLY-Any (or untyped) parameter binds a DYNAMIC carrier: the
// author wrote "accepts anything", which is the gradual-dispatch intent — a body
// word over it (`get`, `add`, a user helper) poly-matches at runtime instead of
// failing no_signature against the strict Any top. A concrete declared type
// stays a strict carrier so its real shape is checked normally. This is the
// same treatment NewElementCarrier gives an untyped list element, lifted to the
// parameter boundary (the trie/decision "unify Any with concrete params" gap).
func ParamInputCarrier(t *core.Type) core.Value {
	if t == nil || t.Equal(core.TAny) {
		return core.NewDynamicCarrier(core.TAny)
	}
	// A named-union param (`x:T` with `def T (Integer tor String)`) binds
	// the DISTRIBUTING disjunct carrier, not a bare T-tagged one — the same
	// multi-denotation rule as the declared-return side (UnionCarrierForType):
	// install-time body analysis then dispatches `x add x` per alternative.
	// Marked Declared: the param annotation claims every alternative is a
	// valid input, so a body dispatch that fails for one is an ERROR
	// (disjunctPartitionReturns), not the analysis-join partial warning.
	if dv, ok := core.UnionCarrierForType(t); ok {
		if di, isDi := dv.Data.(core.DisjunctInfo); isDi {
			di.Declared = true
			dv.Data = di
		}
		return dv
	}
	return core.NewCarrier(t)
}

// DataListElemTypeFromValue is a package-level duplicate of
// dataListElemType that lives in carrier.go so ReturnsFunc helpers
// don't depend on the native_array_higher.go symbol. It reads the
// ChildTypeInfo first, then joins concrete element VTypes.
func DataListElemTypeFromValue(data core.Value) *core.Type {
	if data.Data == nil {
		return core.TAny
	}
	if ct, ok := data.Data.(core.ChildTypeInfo); ok {
		if ct.Child.Data == nil && !ct.Child.Carrier {
			c := ct.Child // bare type-literal child IS the element type
			return &c
		}
		return ct.Child.Parent
	}
	// A concrete MAP: the higher-order words (each/filter/fold over a map)
	// transform the VALUES (keys are kept), so the element type a value-body
	// closure sees is the common value type, not the list-element type below.
	if mp, ok := data.Data.(core.MapPayload); ok {
		if mp.M == nil || mp.M.Len() == 0 {
			return core.TAny
		}
		var t *core.Type
		for _, k := range mp.M.Keys() {
			v, _ := mp.M.Get(k)
			if t == nil {
				t = v.Parent
			} else {
				t = core.CommonAncestorType(t, v.Parent)
			}
			if t.Equal(core.TAny) {
				break
			}
		}
		if t == nil {
			return core.TAny
		}
		return t
	}
	list, err := core.AsList(data)
	if err != nil || list.IsNil() || list.Len() == 0 {
		return core.TAny
	}
	t := list.Get(0).Parent
	for i := 1; i < list.Len(); i++ {
		t = core.CommonAncestorType(t, list.Get(i).Parent)
		if t.Equal(core.TAny) {
			break
		}
	}
	return t
}

// toCarrier converts a concrete Value to its carrier form. Control /
// structural tokens (words, marks, moves, open-paren, paren-expr,
// interp-string, return-check, def-cleanup, forward) are returned
// unchanged: they drive dispatch and must retain their payloads.
// Lists and maps are returned unchanged for now so that list/map
// signature matching keeps working; carrier-aware list/map handling
// is future work.
func toCarrier(v core.Value) core.Value {
	if core.IsWord(v) || core.IsForward(v) || core.IsMark(v) || core.IsMove(v) ||
		core.IsOpenParen(v) || core.IsParenExpr(v) || core.IsInterpString(v) || core.IsXmlInterp(v) ||
		core.IsReturnCheck(v) || core.IsDefCleanup(v) || core.IsDispatchMod(v) {
		// A `/v`/`/q` dispatch-modifier marker (Word/__DM, parser-emitted right
		// after a paren / dotted-path group) is a control token too: stripping
		// it to a payload-less carrier made check mode UNABLE to consume it
		// (execFnDefLiteral's peek reads the DispatchModInfo) or drop it
		// standalone (stepLiteral's IsDispatchMod drop) — the marker then
		// leaked into the check stack as a phantom value the runtime never
		// has (`([x:Any] => [x])/v is T` bound `is` to the marker instead of
		// the lambda). Kept verbatim, check mode parks/drops it exactly as
		// the interpreter does (Stage M2d).
		return v
	}
	// A dynamic carrier already IS a carrier; its Parent/Data is the
	// gradual bound (which may be a disjunct). Return it unchanged so
	// stripping never nulls the bound or clears the Dynamic flag.
	if v.Dynamic {
		return v
	}
	// Keep lists and maps concrete for now — matchSignature relies
	// on Data presence for a few compound cases.
	if v.Parent.Equal(core.TList) || v.Parent.Equal(core.TMap) {
		return v
	}
	// Keep concrete inert SCALAR literals concrete so the recorder can recover
	// the value. Two drivers: static index checking (an out-of-bounds literal
	// index like `[10 20] 5 getr` needs the integer), and const-baking a DATA
	// list/map whose interior is scalar (`each $.name [{name:"a"}]`, `size
	// people`, a string-bearing table row). Without this the interior strips to
	// a type-only carrier, so the container is no longer an inert const and the
	// operand has no compiled home. Only GENUINE concrete scalars qualify — a
	// DepScalar CONSTRAINT (`Integer gt 10`, `String len 5`) carries a
	// DepScalarInfo payload, not one of these, so it is naturally excluded and
	// stays a carrier for predicate matching. Precision only increases: a
	// literal stays concrete until a word consumes it and produces a computed
	// carrier, exactly as lists/maps already behave.
	// Micron values (MicronPayload; Pathon's PathonPayload) are in the
	// same class: immutable structured scalars whose payload IS the
	// value. They join the list so the `make` model's surfaced
	// construction SURVIVES to its consumers — stripping here rebuilt
	// the bare carrier the model exists to improve on, so the
	// concrete-operand mirrors (micronOpReturns' guaranteed-error
	// replay, the pure-word dry pass) never saw a made micron
	// (PR #346 review).
	switch v.Data.(type) {
	case core.IntPayload, core.StrPayload, core.BoolPayload, core.FloatPayload, core.AtomPayload,
		core.BigIntPayload, core.DecimalPayload, core.TimePayload, core.DurationPayload, core.TimezonePayload,
		core.MicronPayload, core.PathonPayload:
		if core.IsConcrete(v) {
			return v
		}
	}
	// Keep FnDef / Function payloads (FnDefInfo) concrete. Stripping
	// them loses the body, params, and Captured list — which means a
	// factory call producing a closure (`def add5 (make-adder 5)`) in
	// check mode would otherwise bind add5 to an empty carrier rather
	// than to the inner FnDefInfo, breaking subsequent invocation +
	// inference of `add5 3`.
	if _, ok := v.Data.(core.FnDefInfo); ok {
		return v
	}
	// Keep Disjunct / Enum values concrete: their DisjunctInfo (the
	// alternatives) IS the type definition. Stripping to a bare TDisjunct /
	// TEnum carrier loses the alternatives, so IsDisjunct / IsTypeBody go false
	// and `def Maybe (String tor None)` / `def Color enum […]` are wrongly
	// rejected in check mode ("body must be a type value or literal"), and
	// `tcmp` / `is` / `typeof` over the type lose their members (compare.tsv).
	// Same rationale as the FnDef / Module / Reach payload preservations above.
	if _, ok := v.Data.(core.DisjunctInfo); ok {
		return v
	}
	// Keep DEPSCALAR constraints concrete: their DepScalarInfo (the bounds)
	// IS the type definition. Stripping to a bare base-scalar carrier loses
	// the constraint, so a type-algebra meet over named refinements
	// (`A tand B`) degraded to a plain Integer and a typed-def annotation
	// built from a tand/tor expression lost its bounds in check mode. Same
	// rationale as the Disjunct / Enum preservation above.
	if _, ok := v.Data.(core.DepScalarInfo); ok {
		return v
	}
	// Keep generic SCHEMA values concrete: their *TypeSchemaInfo IS the type
	// definition (the parameters + body `of` instantiates). Stripping it to a
	// bare carrier loses the schema, so IsTypeSchema goes false and `of`
	// rejects it — e.g. a schema exported from a module and read back through
	// `Pkg.Box` (whose namespace-map get returns the stored value, carrier-
	// stripped) could no longer be instantiated `Pkg.Box of [Integer]`. Same
	// rationale as the FnDef / Disjunct / Module payload preservations above.
	if _, ok := v.Data.(*core.TypeSchemaInfo); ok {
		return v
	}
	// Keep MODULE values concrete, same rationale as FnDefInfo: stripping
	// nulls the Ideal/Module descriptor's ExtensionPayload, so
	// `MathUtil.$module` would become an opaque carrier the get-resolution
	// elision can no longer follow. They are immutable and import-bound, so
	// a pure read of one (`$name`, `$module.name`, `convert Map …`)
	// const-folds (tryFoldModuleConst). A module NAMESPACE needs no arm of
	// its own — it is a plain facet-carrying Map, already preserved by the
	// TMap guard above (facets ride every Value copy).
	if core.IsModuleFamilyValue(v) {
		return v
	}
	// Keep Reach values (dot-access `m.a`, `Pkg.fn`) concrete. A parsed
	// dot-access is a Reach whose ReachInfo carries the Eval flag and the
	// get/getr segments; the engine expands it in place at step time
	// (isEvalReach → expandReach). Stripping would null the ReachInfo,
	// so isEvalReach goes false and the chain never expands — dot-access
	// would be opaque in check mode, silently dropping module-export type
	// propagation, index checks, and dispatch diagnostics. Same rationale
	// as the FnDefInfo case above.
	if core.IsReach(v) {
		return v
	}
	// Keep an __SP splice marker concrete: it is a compile-time macro binding
	// (`def x word [body]`), expanded inline at each use site by stepLiteral. A
	// carrier-stripped marker would lose its payload and never splice, so the
	// reference would be an opaque carrier in check mode.
	if core.IsSplice(v) {
		return v
	}
	// Keep a sugar marker concrete: the parser's structural desugar
	// output (ADR-012 rule 3, 2026-08-04 amendment), lowered at step
	// time by stepSugar. A carrier-stripped marker loses its SugarInfo
	// and could never lower — the sugar would be opaque in check mode.
	if core.IsSugar(v) {
		return v
	}
	// Type literals (Data already nil) are already in the right
	// shape for sig matching — preserve their Carrier=false marker
	// so sigTypeMatchesAsType can still recognise them as type
	// literals rather than as value-carriers. Without this guard,
	// `Integer gt 10` under check mode loses the Integer
	// type-literal distinction and falls through to the boolean
	// sig instead of the dep-constructor sig. See depscalar.go's
	// MakeDepScalarSig + RunInCheckMode for the matching change.
	if v.Data == nil {
		return v
	}
	// Already a carrier.
	if v.Carrier {
		return v
	}
	v.Carrier = true
	v.Data = nil
	return v
}

// checkModeLiteralWords are words whose adjacent String literal argument
// must survive check-mode carrier-stripping because the handler needs the
// concrete value to do anything useful. `import`/`module` resolve a path
// or module name — without it the checker can't load the target's exports
// and every cross-module reference flags a spurious undefined_word (§4.3).
var checkModeLiteralWords = map[string]bool{
	"import": true,
	"module": true,
	// `unpack 'boru:mod'` / `unpack ExportName 'boru:mod'` resolve a module's
	// (statically-declared) exports and bind them unqualified in check mode;
	// the module-name string must stay concrete for the handler to resolve it.
	// The export-name form puts the string two tokens after `unpack`, so the
	// adjacency window below looks back/forward by two, not one.
	"unpack": true,
}

// StripToCarriers returns a copy of in where every non-structural value
// has been converted to its carrier form. Used at the top-level Run()
// entry to bootstrap check-mode execution.
//
// Exception: a String literal directly adjacent to an `import` / `module`
// word (either order — forward `import "x"` or stack `"x" import`) is kept
// concrete so the import handler can resolve and analyse the target. See
// checkModeLiteralWords and §4.3.
func StripToCarriers(in []core.Value) []core.Value {
	out := make([]core.Value, len(in))
	for i, v := range in {
		if v.Parent.ConformsTo(core.TString) && core.IsConcrete(v) && adjacentToLiteralWord(in, i) {
			out[i] = v
			continue
		}
		out[i] = toCarrier(v)
	}
	return out
}

// adjacentToLiteralWord reports whether the token at index i has an
// immediate neighbour that is a checkModeLiteralWords word.
func adjacentToLiteralWord(in []core.Value, i int) bool {
	// A window of two each way: `import "x"` / `"x" import` are immediate, but
	// `unpack ExportName 'boru:mod'` places the module-name string two tokens
	// after the literal word. Keeping a string concrete is sound regardless
	// (it only adds precision), so the slightly wider window is harmless for
	// the rare non-literal-word case it might also catch.
	for d := 1; d <= 2; d++ {
		if i-d >= 0 && isLiteralWord(in[i-d]) {
			return true
		}
		if i+d < len(in) && isLiteralWord(in[i+d]) {
			return true
		}
	}
	return false
}

func isLiteralWord(v core.Value) bool {
	if !core.IsWord(v) {
		return false
	}
	w, _ := core.AsWord(v)
	return checkModeLiteralWords[w.Name]
}

// CarrierResults returns the carrier Values that a matched signature
// produces in check mode. Resolution order:
//
//  1. If sig.ReturnsFn is set, it is invoked with the carrier-typed
//     args; the results are coerced to carriers (Carrier=true, Data
//     stripped for scalar types) and returned.
//  2. Otherwise, if sig.Returns is non-empty, one fresh carrier is
//     produced per declared Returns type.
//  3. Otherwise a diagnostic is recorded and a single TAny carrier is
//     returned so the checker can keep making progress.
//
// args are the carrier-typed input values in signature order (same
// args that would be passed to the runtime handler). pos carries the
// word's source location so diagnostics can point at it.
//
// tailConsumed reports that this dispatch sits in PAREN-TAIL position —
// the token immediately after the call is a CloseParen, so the result is
// the group's value, consumed by whatever encloses the group (a `def`
// binding, an outer word's arg). It gates the mixed-arity gradual-arity
// model under a real compile (see the `len(out) == 0` branch): only a
// consumed result may optimistically model one dynamic value, because a
// free statement-position residual would be collected by the NEXT word
// and corrupt its arity. The dynamic-recovery callers pass false.
func CarrierResults(r *core.Registry, word string, sig *core.Signature, args []core.Value, pos core.SrcPos, ownerReg *core.Registry, tailConsumed bool) []core.Value {
	if out, ok := specialWordResults(r, word, args, pos); ok {
		return out
	}
	narrowDynamicUses(r, word, sig, args)
	// Per-alternative dispatch for strict disjunct inputs
	// (design/checker-accuracy-review.10.md A1). matchSignature tested
	// the disjunct as a single value, so the matched sig may not be
	// the one runtime dispatch takes for every alternative — e.g.
	// Integer|String reaches add's [Scalar Scalar]→String catch-all
	// although the Integer path takes [Number Number]. Resolve each
	// alternative independently and join the per-alternative returns.
	var partOut []core.Value
	if out, ok := disjunctPartitionReturns(r, word, args, pos); ok {
		// A strict-disjunct straddle is a runtime-dispatch case, not an
		// inherent refusal: if the word is a safe poly candidate (core builtin,
		// no meta/fn-value/code-body sig) and its operands resolve, lower it to
		// OpCallNativePoly so the VM re-matches the one concrete alternative at
		// run time — e.g. `5 is (tnot (Integer gt 0))`. A USER FN under an
		// ARMED recording instead falls THROUGH to the ordinary dispatch
		// below — one recorded CALL_USER with the ORIGINAL (provenance-
		// carrying) args; the runtime value is one concrete alternative the
		// unit's param contract admits — and the partition-joined carriers
		// are re-IDed onto the recorded results at the tail, keeping the
		// per-alternative type precision without orphaning the residual
		// (the L-JOIN recursive-union refusal). Everything else refuses.
		if r.Check.Recorder().Active() && sig != nil && sig.FnFrame() != nil &&
			disjunctCombosTakeSig(r, word, args, sig) {
			partOut = out
		} else {
			if !dispatchTryRecordPoly(r, word, sig, args, out, pos, true, ownerReg, false, nil) {
				r.Check.Recorder().RecordPoly(word)
			}
			return out
		}
	}
	out := declaredReturnCarriers(r, word, sig, args, pos)
	if folded, ok := dispatchTryFoldScalarConst(r, sig, args); ok && len(out) == 1 {
		// Concrete-condition folding (CompileScalarFold): the comparison ran
		// for real over compile-time-known scalars, so the concrete result
		// replaces the declared-type carrier — `(n eq 0)` with a const n
		// reads as a known false downstream (return-membership conformance,
		// future static branch commitment). Unlike tryFoldModuleConst the
		// dispatch still RECORDS below: the VM re-runs the comparison
		// faithfully at run time, and eliding the event would strand every
		// lowering that anchors on a condition EVENT (computed-arm `if`,
		// variadic-else claims — the emit goldens pin this).
		folded.ID = out[0].ID // keep the recorder's result identity
		out[0] = folded
	}
	out = applyGradualContagion(r, word, args, out, pos, tailConsumed)
	dispatchRecordOutcome(r, word, sig, args, out, pos, ownerReg)
	// The partitioned user-fn dispatch (above): hand back the partition-
	// joined carriers under the RECORDED results' identities, so downstream
	// operand resolution reaches the recorded call while the checker keeps
	// the per-alternative precision. A count mismatch keeps the recorded
	// results verbatim — sound, just wider.
	if partOut != nil {
		if len(partOut) != len(out) {
			return out
		}
		for i := range partOut {
			partOut[i].ID = out[i].ID
		}
		return partOut
	}
	return out
}

// scalarFoldOperand reports whether a check-mode dispatch operand carries a
// compile-time-known SCALAR value a CompileScalarFold dispatch may fold
// over: an inert const, or a check-mode literal — which rides as a
// concrete-PAYLOAD carrier (Carrier=true, Data set; the same shape the
// DepScalar predicate evaluation reads for `f 5`), so the isInertConst
// carrier guard alone would reject it. The carrier-tolerant arm admits only
// value-scalar payloads: a compound carrier's payload is structural
// (ChildTypeInfo) and a dynamic carrier's value is unknown — both decline.
func ScalarFoldOperand(v core.Value) bool {
	// A CONTAINER operand never folds, inert const or not — the guard the
	// function's own name and contract always implied, restored after a
	// live false positive was found (PR #346 adversarial review):
	//
	//	def m {a:[1]}
	//	f (if ((m get 'a') eq (m get 'a')) ['s'] [7])   # f wants a String
	//
	// `eq` on a non-scalar is CONTAINER IDENTITY (core/go/compare.go, the
	// sameContainer arms), not structure. At run time both reads return
	// the SAME stored container, so the condition is true and the String
	// arm runs. In check mode each read hands back a CloneValue — a fresh
	// container, minted precisely because the emitter's operand-provenance
	// tracking needs a fresh ID — so the fold computed false, `if` pruned
	// the live branch, and the checker flagged correct code. No corpus row
	// had this shape, so the false-positive pin never saw it.
	//
	// Check mode cannot model runtime container identity at all: its
	// values are copies by construction. So the fold declines on every
	// container, both operands' identity being equally unknowable, rather
	// than declining per-word — `deq` / `cmp` over containers lose a
	// const-fold the checker was never entitled to make from copies.
	// Scalars are unaffected: their equality is by VALUE
	// (scalarFamilyEqual), and the immutable structured scalars (Micron,
	// Pathon, Time) compare structurally too, so both keep folding.
	if core.HasContainerIdentity(v) {
		return false
	}
	if core.IsInertConst(v) {
		return true
	}
	if v.Dynamic || v.Data == nil {
		return false
	}
	switch v.Data.(type) {
	case core.IntPayload, core.FloatPayload, core.StrPayload, core.BoolPayload, core.AtomPayload,
		core.BigIntPayload, core.DecimalPayload:
		return true
	}
	return false
}

// specialWordResults handles the words CarrierResults special-cases before
// ordinary return resolution: the `args` frame projection, the `word` splice
// marker, and compile-time `macroexpand` folding. ok=false falls through to
// the normal path (including macroexpand's error/trap fall-through, which
// must still resolve returns and refuse).
func specialWordResults(r *core.Registry, word string, args []core.Value, pos core.SrcPos) ([]core.Value, bool) {
	// `args` inside a compiled fn body projects the frame's params (pushed by
	// AnalyseFnBody) as a list value with NO recorded event. An `args.N` access
	// then folds to param N — a frame local — via tryFoldStaticIndex; bare
	// `args` has no foldable consumer and refuses at its use site. At top level
	// r.Args is empty so `args` falls through to RecordCall's refusal.
	if word == "args" {
		// Inside a CLOSURE body compile the projection must DECLINE: the
		// analysis args frame there holds the CallableSpec inputs (empty for
		// `do`), while at run time the body executes through InvokeBody in
		// the ENCLOSING call context, so the interpreter's `args` reads the
		// enclosing fn's per-call list. Projecting the closure frame baked
		// that wrong (often empty) list as an inert const — `do [args]`
		// inside a fn compiled to PUSH_CONST [] against the interpreter's
		// [7]. Falling through reaches RecordCall's context-dependent-word
		// refusal, the closure probe declines, and the program takes the
		// interpreter fallback with the correct value. A plain (non-
		// recording) check keeps the projection so diagnostics are unchanged.
		if es := r.Check.Recorder(); es.Active() && es.InClosureUnit() {
			return nil, false
		}
		if top, ok, err := r.Args.Top(); err == nil && ok && core.IsConcrete(top) {
			// In compile mode `args.N` folds to a frame local (PUSH_LOCAL N)
			// for named AND unnamed params alike: an unnamed param is a
			// local re-pushed onto the operand stack at unit entry, and the
			// copy the body never consumes is discarded by the RET's
			// NUnnamed trim — the exact __RC frame discipline — so the fold
			// no longer strands a divergent count (the former "args over a
			// frame with unnamed params" refusal).
			return []core.Value{top}, true
		}
	}
	// `word [body]` is a compile-time macro splice: produce the __SP marker as a
	// non-emitting value (no runtime op). At its use site stepLiteral splices the
	// body inline and re-steps it against the live stack, so the expansion
	// compiles in place — late binding and all. (The `def NAME word …` that binds
	// the marker emits nothing either; the marker has no runtime existence.)
	if word == "word" && len(args) == 1 {
		return []core.Value{core.NewSplice(args[0])}, true
	}
	// `macroexpand (mac args…)` is Lisp-style compile-time expansion: the macro
	// and its operands are static, so run the expansion NOW and bake the
	// resulting token list as a const (code-as-data). Only when the expansion
	// is fully concrete (isInertConst — no carrier from a runtime operand) and
	// succeeds; a too-deep / erroring expansion falls through to refuse, and the
	// interpreter surfaces the same error.
	if word == "macroexpand" && len(args) == 1 {
		toks, err := core.ExpandMacroForm(r, args[0])
		if err == nil {
			if lst := core.NewList(toks); core.IsInertConst(lst) {
				// Determinism guard (mirrors tryFoldModuleConst /
				// constFoldContainerVal): ExpandMacroForm runs the macro template
				// through a sub-engine, which is NOT guaranteed pure — a template
				// reading now/rand/mutable state expands differently each step, and
				// isInertConst accepting the result does not imply determinism (an
				// unevaluated (now) member is inert). Bake the check-time expansion
				// only when a second expansion agrees; on mismatch fall through to
				// refuse so the interpreter expands at its own step. The probe runs
				// under a def-stack snapshot so it leaves no new side effect beyond
				// the first expansion's (which is preserved, as before).
				snap := r.Defs.Snapshot()
				toks2, err2 := core.ExpandMacroForm(r, args[0])
				r.Defs.Restore(snap)
				if err2 == nil && core.ConstFoldAgrees(core.NewList(toks2), lst) {
					return []core.Value{lst}, true
				}
			}
		} else {
			// A runaway recursive macro expands until the depth guard
			// (macro_expand.go) raises `macroexpand_error` — the interpreter
			// raises exactly this error at run time. Compile the byte-identical
			// terminal trap (top-level only — RecordTrap declines a nested
			// occurrence, which keeps the lenient fallback) so the compiled
			// program raises it instead of refusing. Mirrors the sibling
			// mini/parse/emit `*_unknown_lang` expansion-time traps. Any OTHER
			// expansion error (a malformed form, a non-macro head) is not a
			// runtime error to reproduce — it falls through to refuse, and the
			// interpreter surfaces it.
			var ae *core.BoruError
			if errors.As(err, &ae) && ae.Code == "macroexpand_error" {
				r.Check.Recorder().RecordTrap("macroexpand_error", ae.Detail,
					"macroexpand", ae.Hint, pos)
			}
		}
	}
	return nil, false
}

// declaredReturnCarriers is CarrierResults' three-way return resolution:
// ReturnsFn (invoked with the carrier args), declared Returns (one fresh
// carrier per type, declared Any riding dynamic), or the missing_returns
// fallback (a dynamic Any so one unannotated word does not cascade false
// no_signature errors downstream).
func declaredReturnCarriers(r *core.Registry, word string, sig *core.Signature, args []core.Value, pos core.SrcPos) []core.Value {
	var out []core.Value
	switch {
	case sig.ReturnsFn != nil:
		r.Check.CurCallPos = pos // expose call site to ReturnsFn (e.g. make Array identity)
		raw := sig.ReturnsFn(resolveTypeNameArgs(args), r)
		out = make([]core.Value, len(raw))
		for i, v := range raw {
			out[i] = toCarrier(v)
		}
	case sig.Returns == nil:
		// Explicit nil (no annotation) triggers the fallback. An empty but
		// non-nil slice is a valid "returns nothing" declaration.
		r.Check.AddDiagnostic(core.CheckDiagnostic{
			Code:   "missing_returns",
			Detail: "word " + word + " has no declared Returns for matched signature; assuming Any",
			Word:   word,
			Row:    pos.Row,
			Col:    pos.Col,
		})
		// The assumed result is a DYNAMIC Any carrier — "type unknown",
		// not "type is the Any root". A plain Any carrier fails every
		// typed slot (Any conforms only to Any), so one unannotated
		// word used to cascade false no_signature errors through every
		// downstream consumer (`mod (size s) 4` errored on mod). The
		// dynamic bound matches optimistically, the one real diagnostic
		// (this missing_returns) remains, and a guard discharges the
		// modality back to strict.
		c := core.NewCarrier(core.TAny)
		c.Dynamic = true
		out = []core.Value{c}
	default:
		out = make([]core.Value, len(sig.Returns))
		for i, t := range sig.Returns {
			// A declared RECORD return (TMap + field-schema pattern) keeps its
			// schema on the result carrier, mirroring BuildFnBodyReturnsFn's
			// declared loop — see recordReturnCarrier (NUR068).
			if rc, ok := recordReturnCarrier(t, returnPatternAt(sig.ReturnPatterns, i)); ok {
				out[i] = rc
				continue
			}
			c := core.NewCarrier(t)
			// A declared `Any` return means "statically unknown", not
			// "inhabits only the Any root": a STRICT Any carrier
			// conforms to no typed slot, so accessor words declared
			// `Returns: [Any]` (`get`, dotted field reads) poisoned
			// every typed consumer downstream with false no_signature
			// errors (`set 0 1 f.bits` errored because `f.bits` typed
			// as strict Any). Mark it dynamic: optimistic matching,
			// discharged back to strict by a guard.
			if t.Equal(core.TAny) {
				c.Dynamic = true
			}
			out[i] = c
		}
	}
	return out
}

// isConcreteContainerReturn reports whether a declared-return carrier is a
// fully-determined List/Map container (not the Any root, not a dynamic
// carrier). Such a return type is fixed by the word's contract independent of
// the runtime input value, so gradual contagion need not mark it dynamic — a
// downstream `each` / `fold` / accessor can then commit on the known container
// and derive an element carrier. Scoped to containers because that is the only
// consumer that needs the concrete type; scalar returns gain nothing and their
// strict form can spuriously trip the forward/stack split detector.
func isConcreteContainerReturn(v core.Value) bool {
	if v.Dynamic || v.Parent == nil {
		return false
	}
	return v.Parent.ConformsTo(core.TList) || v.Parent.ConformsTo(core.TMap)
}

// applyGradualContagion widens results derived from dynamic args: the
// modality flows downstream (a guard discharges it), a single result widens
// to the union of all reachable overload returns (first-match partition),
// and a 0-return mutator over a dynamic receiver optimistically models one
// value where a value-returning sibling overload is reachable (gated to
// consumed results under a real compile — see CarrierResults' doc).
func applyGradualContagion(r *core.Registry, word string, args []core.Value, out []core.Value, pos core.SrcPos, tailConsumed bool) []core.Value {
	// Gradual contagion (design/dynamic-modality-report.10.md): a result
	// derived from a dynamic carrier is itself dynamic, so the modality
	// flows downstream instead of dying after one dispatch. The bound is
	// the sig's declared return (the first-cut result; the full
	// first-match partition over the bound is a later slice). Sound — it
	// only loosens matching, never tightens — and a guard discharges it
	// back to strict. ReturnsFn results that are already dynamic (e.g.
	// ReturnsIdentity of a dynamic input) stay so via toCarrier.
	if AnyDynamicCarrier(args) {
		// STRICT mode (`boru check --strict`): make the gradual frontier
		// loud — every committed dispatch over a dynamic operand is a
		// point the checker matched optimistically and the runtime will
		// re-verify. Non-gating info; deduped by word+position (the same
		// dispatch can be re-analysed under several call shapes).
		if r.Check.Strict {
			dup := false
			for _, d := range r.Check.Diagnostics {
				if d.Code == "dynamic_dispatch" && d.Word == word && d.Row == pos.Row && d.Col == pos.Col {
					dup = true
					break
				}
			}
			if !dup {
				r.Check.AddDiagnostic(core.CheckDiagnostic{
					Code:   "dynamic_dispatch",
					Detail: "strict: " + word + " dispatched over a dynamic operand — matched optimistically, re-checked at runtime",
					Word:   word,
					Row:    pos.Row,
					Col:    pos.Col,
				})
			}
		}
		// A CONCRETE, fully-determined declared CONTAINER return type does not
		// become statically-unknown just because an input was dynamic: the
		// word's contract fixes the RESULT TYPE regardless of which runtime
		// value flowed in (`StructUtil.items` always returns a `List`). The
		// dynamic input only decides whether the call SUCCEEDS or RAISES — and
		// the call itself is baked as a runtime-faithful guarded dispatch
		// (CALL_NATIVE / poly) that raises exactly as the interpreter on a bad
		// receiver, so a compiled program never observes a wrongly-typed
		// result. Keeping such a List/Map output STRICT lets a downstream
		// `each` / `fold` / accessor commit on the KNOWN container instead of
		// refusing "dynamic input at each" — the cross-module element-typing
		// flip. Scoped to CONTAINER returns on purpose: a downstream
		// higher-order/accessor word needs the concrete container to derive an
		// element carrier, whereas keeping a SCALAR return strict has no such
		// consumer and merely turns a formerly-dynamic if-arm join into a
		// non-dynamic disjunct that trips the conservative forward/stack split
		// detector (the trie/burst `String|Integer` fold-body regression).
		// Guarded to the single-reachable-return case (`keepStrict`): a
		// multi-overload word whose reachable overloads return DIVERGENT types
		// must still ride the dynamic union below (the runtime could take a
		// sibling overload's return), and a genuinely input-dependent return
		// (declared Any / a ReturnsFn that produced a dynamic carrier) stays
		// dynamic — those arrive here already flagged Dynamic and are untouched.
		var reachable []*core.Type
		if len(out) == 1 {
			reachable = dynamicReachableReturns(r, word, args)
		}
		keepStrict := len(out) == 1 && len(reachable) < 2
		for i := range out {
			out[i].Carrier = true
			if keepStrict && !out[i].Dynamic && isConcreteContainerReturn(out[i]) {
				continue
			}
			out[i].Dynamic = true
		}
		// First-match partition (design/dynamic-modality-report.10.md): a
		// dynamic bound can reach MULTIPLE of the word's overloads, whose
		// returns may differ. The single matched-sig return is then too
		// narrow — it would wrongly reject a downstream use of one of the
		// other reachable returns. Widen the (single) result to the union
		// of all reachable returns. No-op for the common case (one
		// reachable return), so unobservable with return-uniform words.
		// The widening MINTS a fresh identity (NewDynamicCarrierValue) — a
		// user-poly ReturnsFn that already recorded the call under the old
		// out[0].ID is re-linked by recordCallElided's poly-alias arm (the
		// §8.2(3) return-join), which rebinds the rebuilt outs onto the
		// recorded event; preserving the old ID here instead reclassified
		// an unrelated compiling flex-set row's residual ("call result
		// above a literal") — the fresh mint is load-bearing for the
		// residual model.
		if len(out) == 1 {
			if len(reachable) >= 2 {
				alts := make([]core.Value, len(reachable))
				for i, t := range reachable {
					alts[i] = core.NewTypeLiteral(t)
				}
				out[0] = core.NewDynamicCarrierValue(core.NewDisjunct(alts))
			}
		} else if len(out) == 0 && (!r.Check.Compiling || tailConsumed) {
			// The matched overload returns NOTHING (an in-place mutator) but the
			// receiver is dynamic and a value-returning sibling is also reachable —
			// `set`'s mixed-arity overloads (Array/Object/Store/Class return 0;
			// Map/List/Flex return the updated node). The runtime receiver could be
			// either, so committing to 0 values poisons every downstream CONSUMER of
			// the result (trie's `(nd "kids" get) set ch child` fed to mk-node: 0
			// values → an unbound arg → a false undefined_word on the consumer's
			// param + no_signature on the consuming call). Model ONE dynamic value —
			// the optimistic gradual arity — so a consuming use type-checks.
			//
			// In PURE check mode this always fires (no runtime, so the model is
			// free). Under a REAL compile it fires ONLY in paren-tail position
			// (tailConsumed): there the single value is immediately consumed by the
			// group close, so it can never be collected as the NEXT word's extra arg
			// — the unsoundness that forced the original !Compiling gate (a
			// statement-position `… set mem true  IO.write …`, where modeling 1
			// value made write bind the phantom as a 3rd arg and underflow at run
			// time — TestSetOverDynamicReceiverPolyCompiles). The VM's
			// OpCallNativePoly is runtime-faithful (callPoly pushes the matched
			// overload's ACTUAL results), so a consumed Map-set pushes the 1 value
			// the recorder wired; a non-value receiver would already error the
			// original program (def of 0 values), erroring alike in both engines. A
			// CONCRETE receiver reaches only its own overload, so vrets is empty and
			// the true 0-arity stands either way.
			if vrets := dynamicReachableValueReturns(r, word, args); len(vrets) > 0 {
				if len(vrets) == 1 {
					c := core.NewCarrier(vrets[0])
					c.Carrier = true
					c.Dynamic = true
					out = []core.Value{c}
				} else {
					alts := make([]core.Value, len(vrets))
					for i, t := range vrets {
						alts[i] = core.NewTypeLiteral(t)
					}
					out = []core.Value{core.NewDynamicCarrierValue(core.NewDisjunct(alts))}
				}
			}
		}
	}
	return out
}

// bodyFreeForFallback reports whether a code body references only words
// the VM's sub-engine can resolve at run time: registered natives /
// fn-defs (r.Lookup non-nil) and the known bare literals. A bare word
// bound by a value `def` resolves via Defs substitution, NOT Lookup, so
// it fails here — correctly, since at VM run time that binding is the
// check pass's CARRIER, not a concrete value.
func BodyFreeForFallback(r *core.Registry, body core.Value) bool {
	free := true
	core.WalkBodyWords([]core.Value{body}, func(w core.WordInfo, _ core.Value) {
		if !free {
			return
		}
		switch w.Name {
		case "true", "false", "none", "null":
			return
		case "break", "continue", "return":
			// A flow-control sentinel inside the body would set the shared
			// registry's FlowCtrl when the island's sub-engine runs it; the
			// VM cannot propagate that to an ENCLOSING compiled loop/frame
			// across the island boundary (the sentinel surfaces as a
			// tape-coupled island result and the VM rejects it). Refuse so
			// the whole program falls back, where the interpreter unwinds
			// it correctly. (Sentinels targeting a loop WITHIN the body are
			// rarer than this conservative rule loses; the gate stays
			// green either way.)
			free = false
			return
		case "args", "__pa":
			// Context-dependent words read the interpreter's per-call args
			// stack, which the VM's CALL_USER frames do not maintain (params
			// are frame locals). An island's sub-engine inside a compiled fn
			// would therefore see an EMPTY args stack where the interpreter
			// sees the enclosing call's list — `do [args]` in a fn islanded
			// to error(args_error) against the interpreter's [7]. Refuse so
			// the whole program falls back (the same divergence RecordCall's
			// context-dependent-word gate refuses for direct dispatches).
			free = false
			return
		}
		if r.Lookup(w.Name) != nil {
			return
		}
		if _, ok := core.TypeNames[w.Name]; ok {
			return
		}
		free = false
	})
	return free
}

// bodyRefsFnLocalFn reports whether any NoEvalArgs code-body arg of a
// body-consuming dispatch (a CompileFallbackBody word, or one declaring a
// CallableSpec) names a FN-LOCAL FN: a bare body Word whose current r.Defs
// binding both lives INSIDE the enclosing fn being compiled (the
// ComputeCaptures scope rule — Depth(name) > TopFnBaseline()[name]) and
// holds a Function value. Returns the first such name.
//
// The shape is the NUR037 leak: the interpreter resolves the body word per
// run through r.Defs, but a compiled program never executes the enclosing
// body's `def step fn […]` (the fn unit is static), so every record-time
// admission that bakes the NAME — the island span (bodyFreeForFallback
// resolves it via r.Lookup against the CHECK-time registry), the plain
// CALL_NATIVE const-bake (the body list is inert data to isInertConst),
// and the closure probe (whose body compile binds the check-time overload)
// — leaves the VM raising undefined_word on a program the interpreter
// runs. recordDispatchOutcome therefore refuses the whole program up front
// ("slow, not wrong" — design/COMPILABLE-SUBSET.md §5).
//
// Scope-matching is deliberately NARROW, mirroring ComputeCaptures:
//   - module-scope callbacks (TopFnBaseline nil, or Depth ≤ baseline) keep
//     compiling — the utils-corpus house-rule shape;
//   - fn-local VALUE defs (a non-Function binding, e.g. the mini-redis
//     KEYS accumulator) are legitimately handled by the closure path's
//     lexical captures and must NOT match;
//   - structured-lowering words (if / for — NoEvalArgs but neither
//     CompileFallbackBody nor Callable) record their bodies as inline
//     events (a fn-local dispatch lowers to CALL_USER by unit ref, no
//     name bake), so the family gate excludes them.
func BodyRefsFnLocalFn(r *core.Registry, sig *core.Signature, args []core.Value) (string, bool) {
	if sig == nil || len(sig.NoEvalArgs) == 0 ||
		(!sig.CompileEffect.Has(core.CompileFallbackBody) && sig.Callable == nil) {
		return "", false
	}
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return "", false // module scope: no enclosing fn, nothing is fn-local
	}
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		name := ""
		core.WalkBodyWords([]core.Value{args[i]}, func(w core.WordInfo, _ core.Value) {
			if name != "" {
				return
			}
			v, bound := r.Defs.Top(w.Name)
			if !bound || r.Defs.Depth(w.Name) <= baseline[w.Name] {
				return
			}
			if _, isFn := v.Data.(core.FnDefInfo); isFn {
				name = w.Name
			}
		})
		if name != "" {
			return name, true
		}
	}
	return "", false
}

// bodyHasSentinel reports whether a code body contains a flow-control
// sentinel (break/continue/return). Such a body cannot compile to a closure
// (or island): the sentinel targets an ENCLOSING loop/frame the VM cannot
// reach across the call boundary, so the whole program must fall back and let
// the interpreter unwind it. Unlike bodyFreeForFallback this does NOT reject
// def references — the closure compile bakes a concrete def as a const or
// threads an enclosing-fn binding as a capture, and the probe compile refuses
// anything it cannot resolve.
func BodyHasSentinel(body core.Value) bool {
	return valueHasSentinel(body)
}

// valueHasSentinel reports whether a code-BODY value contains a live
// break/continue/return that would run when the body executes. It differs
// from WalkBodyWords (capture analysis) in one load-bearing way: it does NOT
// honour the .Quoted flag. A `quote`d list handed to `do`/`each`/… runs ALL
// its tokens as CODE — quote marks the LIST as data, not its tokens as
// non-executable — so a quoted `break` inside is a live sentinel (verified:
// `def b (quote [break]) for 5 [do b i]` breaks the loop in the interpreter).
// WalkBodyWords skipped it, letting the const-folded do-body compile past the
// sentinel gate; the escaped break then reached the loop epilogue as "flow
// signal with no enclosing loop" and was caught as a value — a miscompile.
// Nested fn bodies are still skipped: their sentinel targets their own scope.
func valueHasSentinel(v core.Value) bool {
	if _, ok := v.Data.(core.FnDefInfo); ok {
		return false
	}
	if core.IsWord(v) {
		w, _ := core.AsWord(v)
		return w.Name == "break" || w.Name == "continue" || w.Name == "return"
	}
	if v.Parent != nil && v.Parent.Equal(core.TList) && v.Data != nil {
		lst, _ := core.AsList(v)
		for i := 0; i < lst.Len(); i++ {
			if valueHasSentinel(lst.Get(i)) {
				return true
			}
		}
		return false
	}
	return scanBodyContainers(v, valueHasSentinel)
}

// scanBodyContainers applies scan to every code-position element of v's
// container families and reports the first hit: list elements, paren-expr
// tokens, interpolated-string ${...} expression parts, XML-interpolation
// attribute/child holes (recursing into nested child templates), and map
// values (keys are strings, inert). Every one of these runs as CODE when
// the container materialises in a body — verified: an interp-string
// `${break}` / a `{k: break}` map value in a quoted do-body breaks the
// loop in the interpreter. A non-container value returns false — leaf
// classification (words, fn payloads) belongs to the caller's scan.
func scanBodyContainers(v core.Value, scan func(core.Value) bool) bool {
	if v.Parent != nil && v.Parent.Equal(core.TList) && v.Data != nil {
		lst, _ := core.AsList(v)
		for i := 0; i < lst.Len(); i++ {
			if scan(lst.Get(i)) {
				return true
			}
		}
		return false
	}
	if core.IsParenExpr(v) {
		toks, _ := core.AsParenExpr(v)
		for _, t := range toks {
			if scan(t) {
				return true
			}
		}
		return false
	}
	if core.IsInterpString(v) {
		parts, _ := core.AsInterpString(v)
		for _, p := range parts {
			for _, t := range p.Expr {
				if scan(t) {
					return true
				}
			}
		}
		return false
	}
	if core.IsXmlInterp(v) {
		tmpl, _ := core.AsXmlInterp(v)
		return xmlTmplScan(tmpl, scan)
	}
	if v.Parent != nil && v.Parent.Equal(core.TMap) && v.Data != nil {
		m, _ := core.AsMap(v)
		if m == nil {
			return false
		}
		for _, key := range m.Keys() {
			mv, _ := m.Get(key)
			if scan(mv) {
				return true
			}
		}
	}
	return false
}

// xmlTmplScan applies scan to every ${...} expression token in an XML
// interpolation skeleton — attribute values and child holes, recursing
// into nested child templates (mirrors walkXmlTmplExprs).
func xmlTmplScan(t core.XmlTmpl, scan func(core.Value) bool) bool {
	for _, a := range t.Attr {
		for _, p := range a.Parts {
			for _, tok := range p.Expr {
				if scan(tok) {
					return true
				}
			}
		}
	}
	for _, c := range t.Cren {
		switch c.Kind {
		case core.XmlCrenExpr:
			for _, tok := range c.Expr {
				if scan(tok) {
					return true
				}
			}
		case core.XmlCrenChild:
			if c.Child != nil && xmlTmplScan(*c.Child, scan) {
				return true
			}
		}
	}
	return false
}

// bodyHasSentinelDeep is bodyHasSentinel plus a TRANSITIVE callee scan: a
// body word resolving to a user fn (registry-installed or a def-bound fn
// value) whose body — transitively through further calls — holds a bare
// break/continue leaks that signal through the call into the CALLER's loop
// (probe-verified: the interpreter unwinds `def f fn [.. [break 7]] .. do
// [f 1]` to the enclosing for), while a compiled closure boundary starts a
// fresh loop stack and surfaces "flow signal with no enclosing loop" error
// values — a miscompile. `return` is deliberately NOT counted in callees:
// the fn boundary consumes it. The scan is conservative — a break consumed
// by the callee's OWN loop still declines; the body then falls back with
// parity, so the cost is speed, never correctness.
//
// The closure-compile gates with a Registry in hand ride this deep variant:
// the token-list callable gate (which owns `do <body>` — the proven
// divergence) and the dyn-body bake gate. Where the interpreter does not
// thread a callee's break through the boundary anyway (each/filter callbacks
// raise `break outside loop` — probe-verified), the deep decline is still
// parity-safe: the fallback interpreter run raises identically. The
// const-bake gates (the noEvalBodiesInert twins) and the lambda gates keep
// the syntactic scan — no divergence reproduced there.
func BodyHasSentinelDeep(r *core.Registry, body core.Value) bool {
	if BodyHasSentinel(body) {
		return true
	}
	if r == nil {
		return false
	}
	return calleeValueLeaksFlow(r, body, map[string]bool{})
}

// calleeValueLeaksFlow is the transitive-scan walker: bare break/continue
// count as leaks, other words resolve to callee fn bodies (recursively,
// cycle-guarded by seen), and containers recurse. Constructed fn payloads
// scan too — an APPLIED fn value's break escapes to the caller (only raw
// tokens reach the direct scanner; a constructed FnDefInfo in a body is
// treated as potentially applied).
func calleeValueLeaksFlow(r *core.Registry, v core.Value, seen map[string]bool) bool {
	if fd, ok := v.Data.(core.FnDefInfo); ok {
		return fnDefLeaksFlow(r, &fd, seen)
	}
	if core.IsWord(v) {
		w, _ := core.AsWord(v)
		if w.Name == "break" || w.Name == "continue" {
			return true
		}
		return calleeLeaksFlow(r, w.Name, seen)
	}
	return scanBodyContainers(v, func(e core.Value) bool {
		return calleeValueLeaksFlow(r, e, seen)
	})
}

// calleeLeaksFlow resolves name to its aggregated dispatch table (Lookup
// unions every FnDefInfo binding on the name's def stack — exactly what a
// call would dispatch over) and scans the overload bodies. Native (Go-impl)
// sigs have no boru body and contribute nothing.
func calleeLeaksFlow(r *core.Registry, name string, seen map[string]bool) bool {
	if name == "" || seen[name] {
		return false
	}
	seen[name] = true
	fd := r.Lookup(name)
	return fd != nil && fnDefLeaksFlow(r, fd, seen)
}

// fnDefLeaksFlow scans every overload body of fd for a leaking
// break/continue. A module fn's body words resolve in its OWN registry.
func fnDefLeaksFlow(r *core.Registry, fd *core.FnDefInfo, seen map[string]bool) bool {
	if fd.Registry != nil {
		r = fd.Registry
	}
	for i := range fd.Signatures {
		for _, t := range fd.Signatures[i].Body() {
			if calleeValueLeaksFlow(r, t, seen) {
				return true
			}
		}
	}
	return false
}

// disjunctPartitionCap bounds the alternative cross product a
// partitioned dispatch will enumerate; beyond it the analysis falls
// back to the whole-disjunct match (wide but terminating).
const disjunctPartitionCap = 16

// disjunctPartitionReturns dispatches each alternative of strict
// disjunct args independently and joins the resulting return
// carriers — the abstract domain distributing over first-match
// dispatch. Returns ok=false when the partition does not apply and
// the caller should use the whole-disjunct path: no strict disjunct
// arg, an unknown or single-signature word, a concrete list/map arg
// (body-running ReturnsFns must not be re-entered per alternative),
// mismatched return arity across alternatives, or a cross product
// over the cap.
//
// Alternatives that reach NO signature get a partial_dispatch
// warning (that path would fail dispatch at runtime) and do not
// contribute to the join; if no alternative matches anything the
// partition declines and the engine's no_signature handling stands.
func disjunctPartitionReturns(r *core.Registry, word string, args []core.Value, pos core.SrcPos) ([]core.Value, bool) {
	if r == nil || !r.Check.IsActive() {
		return nil, false
	}
	hasStrictDisjunct := false
	declaredDomain := true // every strict disjunct arg is a declared-union param binding
	for _, a := range args {
		if core.IsDisjunct(a) && a.Carrier && !a.Dynamic {
			hasStrictDisjunct = true
			if di, err := core.AsDisjunct(a); err != nil || !di.Declared {
				declaredDomain = false
			}
		}
		// Body-running ReturnsFns (if, each, fold, do, …) take
		// concrete list/map operands; re-running them per alternative
		// would duplicate branch analysis and its diagnostics.
		if core.IsConcrete(a) && (a.Parent.ConformsTo(core.TList) || a.Parent.ConformsTo(core.TMap)) {
			return nil, false
		}
	}
	if !hasStrictDisjunct {
		return nil, false
	}
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) == 0 {
		return nil, false
	}

	combos, ok := disjunctCombos(args, disjunctPartitionCap)
	if !ok {
		return nil, false
	}

	// Per-combination transfer function: dispatch the concrete
	// alternative tuple and gather its return-carrier row. A combo that
	// reaches no overload is dropped with a partial_dispatch warning; a
	// combo whose overload is unannotated declines the whole partition
	// (the whole-disjunct fallback — missing_returns + dynamic Any —
	// beats a partial join). Surviving rows are joined position-wise.
	//
	// The combo runs are TYPE PROBES over per-alternative carrier COPIES
	// (alternativeCarriers mints fresh IDs), so under an ARMED recording
	// they must not record: a user fn's ReturnsFn would RecordUserCall the
	// fresh-ID copies ("fn call operand of unknown provenance" — the
	// L-JOIN recursive-union refusal) and compile one unit per combo.
	// Suspend for the loop (a no-op on a plain check); the caller records
	// the dispatch ONCE with the original args (CarrierResults' partition
	// arm). Diagnostics (partial_dispatch) are not gated by suspension.
	rows := make([][]core.Value, 0, len(combos))
	resume := r.Check.Recorder().Suspend()
	defer resume()
	for _, combo := range combos {
		comboSig := firstMatchingSig(fn, combo)
		if comboSig == nil {
			// Severity by disjunct PROVENANCE: when every disjunct arg is a
			// DECLARED union param (DisjunctInfo.Declared), the failing
			// alternative is a valid input of the fn's own annotation — the
			// body is broken for part of its declared domain, an error. An
			// analysis-join disjunct (branch join, element join) stays a
			// warning: the runtime materialises one alternative, so the
			// failing path may be dead (the fuzz corpus' dead-branch class).
			d := core.CheckDiagnostic{
				Code: "partial_dispatch",
				Detail: word + " has no overload for alternative (" +
					comboTypeNames(combo) + ") of a disjunct input — that path would fail dispatch at runtime",
				Word: word,
				Row:  pos.Row,
				Col:  pos.Col,
			}
			if declaredDomain {
				d.Severity = core.SeverityError
				d.Detail = word + " has no overload for alternative (" +
					comboTypeNames(combo) + ") of a declared union parameter — a valid argument of the declared type would fail dispatch at runtime"
			}
			r.Check.AddDiagnostic(d)
			continue
		}
		switch {
		case comboSig.ReturnsFn != nil:
			raw := comboSig.ReturnsFn(resolveTypeNameArgs(combo), r)
			rets := make([]core.Value, len(raw))
			for i, v := range raw {
				rets[i] = toCarrier(v)
			}
			rows = append(rows, rets)
		case comboSig.Returns != nil:
			rets := make([]core.Value, len(comboSig.Returns))
			for i, t := range comboSig.Returns {
				rets[i] = core.NewCarrier(t)
			}
			rows = append(rows, rets)
		default:
			return nil, false
		}
	}
	return joinReturnRows(rows)
}

// disjunctCombosTakeSig reports whether EVERY per-alternative combination of
// the disjunct args first-matches exactly the caller's committed sig — the
// condition under which ONE recorded CALL_USER of that sig's unit is faithful
// for every runtime alternative (CarrierResults' partitioned user-fn arm). A
// combo that would first-match a SIBLING overload (a narrow arm ahead of the
// committed wide one) makes the single baked call a miscompile — the
// interpreter dispatches the sibling for that alternative — so the caller
// keeps the refusal. Combo enumeration failure (over the cap) declines.
func disjunctCombosTakeSig(r *core.Registry, word string, args []core.Value, sig *core.Signature) bool {
	fn := r.Lookup(word)
	if fn == nil {
		return false
	}
	combos, ok := disjunctCombos(args, disjunctPartitionCap)
	if !ok {
		return false
	}
	for _, combo := range combos {
		cs := firstMatchingSig(fn, combo)
		// Lookup mints a FRESH aggregate per call in check mode (the
		// pointer-identity contract), so compare by the stable
		// run-implementation identity — Signature.Impl, the same primitive
		// the user-poly drift guard keys on.
		if cs == nil || cs.Impl != sig.Impl {
			return false
		}
	}
	return true
}

// alternativeCarriers expands a strict disjunct carrier into one carrier
// per flattened alternative; any other value yields itself. It is the
// per-argument expansion the disjunct cross product enumerates.
func alternativeCarriers(a core.Value) []core.Value {
	if core.IsDisjunct(a) && a.Carrier && !a.Dynamic {
		lits := core.FlattenAlternatives(a)
		out := make([]core.Value, 0, len(lits))
		for _, lit := range lits {
			out = append(out, core.CarrierOfLiteral(lit))
		}
		return out
	}
	return []core.Value{a}
}

// disjunctCombos enumerates the bounded cross product of the per-argument
// alternative expansions (alternativeCarriers) — the powerset domain a
// union-typed argument list distributes over. Returns ok=false when the
// product would exceed limit, so the caller widens to the whole-disjunct
// path (wide but terminating).
func disjunctCombos(args []core.Value, limit int) ([][]core.Value, bool) {
	combos := [][]core.Value{nil}
	for i, a := range args {
		alts := alternativeCarriers(a)
		if len(combos)*len(alts) > limit {
			return nil, false
		}
		next := make([][]core.Value, 0, len(combos)*len(alts))
		for _, c := range combos {
			for _, alt := range alts {
				row := make([]core.Value, i+1)
				copy(row, c)
				row[i] = alt
				next = append(next, row)
			}
		}
		combos = next
	}
	return combos, true
}

// joinReturnRows position-wise core.JoinCarriers-folds the return-carrier rows
// gathered from each matched alternative — the abstract join at the
// dispatch merge. ok=false when no row survived or the rows disagree on
// return arity (the partition declines; the caller widens). A row set
// whose members are all zero-arity yields an empty slice with ok=true.
func joinReturnRows(rows [][]core.Value) ([]core.Value, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	joined := rows[0]
	for _, rets := range rows[1:] {
		if len(rets) != len(joined) {
			return nil, false
		}
		for i := range joined {
			joined[i] = core.JoinCarriers(joined[i], rets[i])
		}
	}
	return joined, true
}

// firstMatchingSig returns the first signature of fn (registration
// keeps Signatures in SortSignatures match order) whose arity equals
// len(args) and whose every positional type admits the corresponding
// arg, or nil. Mirrors matchSignature's per-arg type test for the
// already-collected case.
func firstMatchingSig(fn *core.FnDefInfo, args []core.Value) *core.Signature {
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		ok := true
		for j := range args {
			if !core.SigTypeMatches(args[j], core.SigArgType(s, j)) {
				ok = false
				break
			}
		}
		if ok {
			return s
		}
	}
	return nil
}

// comboTypeNames renders a combo's types for diagnostics: "Integer, String".
func comboTypeNames(combo []core.Value) string {
	parts := make([]string, len(combo))
	for i, v := range combo {
		parts[i] = v.Parent.Leaf()
	}
	return strings.Join(parts, ", ")
}

// dynamicReachableReturns returns the distinct single-position return
// types of every signature of `word` that the (dynamic) args reach, but
// only when there are TWO OR MORE distinct returns — the case the
// single matched-sig return would get wrong. Returns nil otherwise (the
// common case: contagion's matched-sig return is already correct).
// Restricted to same-arity, single-static-return sigs; any other shape
// falls back to contagion.
// dynamicReachableValueReturns returns the distinct 1-value return types of the
// word's overloads that the (possibly dynamic) args can reach. Unlike
// dynamicReachableReturns it does NOT bail on a sibling that returns 0 values —
// it is for the MIXED-arity case: a mutator like `set` whose in-place overloads
// (Array/Object/Store/Class) return nothing while the value-returning twins
// (Map/List/Flex) return the updated node. A CONCRETE receiver reaches only its
// own overload (sigTypeMatches is exact for it), so this returns nil there and
// the true 0-arity stands; only a genuinely dynamic receiver reaches both.
// dynamicReachableOverloadCount counts how many of word's same-arity overloads
// the (partly-dynamic) arg list could match. ≥2 means the dispatch is AMBIGUOUS:
// a gradual-Any arg matches every overload optimistically, so the checker's single
// committed overload may not be the one the RUNTIME value needs. Used to refuse a
// higher-order word (each/fold/scan) over a gradual collection — its List-vs-Map
// overloads can't be statically chosen and it has no poly re-match (code body).
// fnPredicateOverloadHazard reports whether word's same-arity overload set
// both (a) contains an fn-PREDICATE-typed param slot (*predicateUnifier —
// the boru fn-body membership path whose check-mode match is LENIENT:
// RunPredicate short-circuits true, registry.go) and (b) leaves more than
// one arm reachable for these args — the combination where a static arm
// commit can diverge from the interpreter's runtime predicate fall-through.
// DepScalar and Go-member types match self-contained in check mode (no
// leniency), so they carry no hazard.
func FnPredicateOverloadHazard(r *core.Registry, word string, args []core.Value) bool {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return false
	}
	hasPred, reachable := false, 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			t := core.SigArgType(s, j)
			if t != nil {
				if _, ok := t.Behavior().(*core.PredicateUnifier); ok {
					hasPred = true
				}
			}
			if !core.SigTypeMatches(args[j], t) {
				reach = false
				break
			}
		}
		if reach {
			reachable++
		}
	}
	return hasPred && reachable >= 2
}

func DynamicReachableOverloadCount(r *core.Registry, word string, args []core.Value) int {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return 0
	}
	n := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			// A strict OR gradual Any carrier could hold a value of any type at
			// run time, so it reaches EVERY same-arity arm — the dispatch is
			// genuinely runtime-dynamic and must poly re-match, not commit
			// statically to the Any-slot arm. Without this a strict Any (an Any
			// param's generalised arg — core_helpers.go) reached only the Any
			// overload, so a wrapper forwarding a Map through an Any param baked
			// the Any arm and diverged from the interpreter's Map dispatch (the
			// each/fold-body multi-sig degradation).
			if isAnyCarrier(args[j]) {
				continue
			}
			// A GENERIC placeholder slot ((T extends Comparable)) admits by
			// per-call instantiation over the runtime value, which a gradual
			// arg's bound-disjointness probe cannot see (tand(Integer, T-node)
			// is Never even though a runtime Integer instantiates T) — count
			// the arm reachable so the ambiguous dispatch stays a runtime
			// re-matched user poly instead of a wrong static commit.
			if args[j].Dynamic && core.IsTypeParamNode(core.SigArgType(s, j)) {
				continue
			}
			if !core.SigTypeMatches(args[j], core.SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if reach {
			n++
		}
	}
	return n
}

func dynamicReachableValueReturns(r *core.Registry, word string, args []core.Value) []*core.Type {
	fn := r.Lookup(word)
	if fn == nil {
		return nil
	}
	var rets []*core.Type
	seen := map[string]bool{}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) || len(s.Returns) != 1 || s.Returns[0] == nil {
			continue
		}
		reach := true
		for j := range args {
			if !core.SigTypeMatches(args[j], core.SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if !reach {
			continue
		}
		if t := s.Returns[0]; !seen[t.ID] {
			seen[t.ID] = true
			rets = append(rets, t)
		}
	}
	return rets
}

func dynamicReachableReturns(r *core.Registry, word string, args []core.Value) []*core.Type {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return nil
	}
	// All operands fully unknown (dynamic carriers bound to the Any root): the
	// result is genuinely statically-unknown, so an unknown-return (ReturnsFn)
	// reachable overload may safely fold in as the Any alternative below
	// (widening the union to gradual-permissive). When SOME operand is
	// concrete, the reachable concrete returns are a tighter, trustworthy
	// union and an unknown-return overload must still suppress refinement — a
	// partially-known call should not be blanket-widened to Any (that
	// mis-narrowed trie's `child` through a `get` whose module-namespace overload
	// then looked reachable).
	allUnknown := len(args) > 0
	for _, a := range args {
		if !a.Dynamic || a.Parent == nil || !a.Parent.Equal(core.TAny) {
			allUnknown = false
			break
		}
	}
	var rets []*core.Type
	seen := map[string]bool{}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		// A different-arity overload is not a candidate for THIS call's arg
		// count — skip it rather than bail. The original code bailed here,
		// which disabled refinement for every word with mixed-arity overloads:
		// `slice`'s 1/2/3-arg forms meant a dynamic-receiver slice committed to
		// the single matched overload (`dynamic(List)` for a String-or-List
		// receiver) instead of the `String|List` disjunct, so a String consumer
		// of the result then failed no_signature (radix's edge splitter, where
		// the sliced label is a get-result of unknown type).
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			if !core.SigTypeMatches(args[j], core.SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if !reach {
			continue
		}
		// A REACHABLE overload we cannot reduce to a single return type
		// (ReturnsFn / multi- or zero-return): its result is statically
		// unknown, so it contributes the Any alternative to the union rather
		// than disabling refinement. The old code bailed (`return nil`) here,
		// which left CarrierResults committed to the single matched overload's
		// return — for a word whose overloads return DISJOINT types over
		// dynamic operands (`add` of two dynamic-Any operands matches the
		// temporal `Instant`/`Date` overloads alongside the numeric ReturnsFn
		// and the String concat), that commitment picked `Instant`, and a
		// downstream String consumer then failed no_signature (radix-delete's
		// `(lab (gpair get 0) add)` merged label fed to `set-edge`'s
		// `newlabel:String`). Folding the unknown in as Any makes the union
		// gradual-permissive, matching any later typed use — sound for dynamic
		// inputs, where the modality is optimistic by construction.
		if len(s.Returns) != 1 || s.Returns[0] == nil {
			if !allUnknown {
				return nil
			}
			if !seen[core.TAny.ID] {
				seen[core.TAny.ID] = true
				rets = append(rets, core.TAny)
			}
			continue
		}
		if t := s.Returns[0]; !seen[t.ID] {
			seen[t.ID] = true
			rets = append(rets, t)
		}
	}
	if len(rets) < 2 {
		return nil
	}
	return rets
}

// narrowDynamicUses implements narrowing-through-use
// (design/dynamic-modality-report.10.md): when a dynamic carrier resolved
// from a binding is consumed by a typed slot, the binding tightens to
// dynamic(bound ∩ slot) for downstream uses, so a later provably-disjoint
// use of the same name fails the match rule and is flagged — no explicit
// guard needed. Scoped via the def stack: branch analysis
// (core.RunCarrierBodyWithDefs) truncates these pushes, so a then-branch
// narrowing never leaks to the else-branch. Sound — the bound only
// tightens, never widens.
func narrowDynamicUses(r *core.Registry, word string, sig *core.Signature, args []core.Value) {
	if r == nil || sig == nil || !r.Check.IsActive() {
		return
	}
	for i, a := range args {
		if !a.Dynamic || a.DynFrom() == "" {
			continue
		}
		// Only narrow a binding that is itself still dynamic (consistent
		// with this value) — guards against a since-rebound name.
		cur, ok := r.Defs.Top(a.DynFrom())
		if !ok || !cur.Dynamic {
			continue
		}
		slot := core.SigArgType(sig, i)
		if slot == nil {
			continue
		}
		// Skip a POLYMORPHIC position: when another of the word's overloads
		// is equally reachable for THIS call but constrains position i to a
		// different (disjoint) type, the dynamic value could legitimately
		// dispatch to either overload at run time, so narrowing to the single
		// matched slot is unsound — it would wrongly reject a downstream use of
		// the sibling shape. `slice`'s data arg is String in one 3-arg overload
		// and List in the other; a dynamic-Any label narrowed to List by the
		// first slice then failed every String consumer (radix's edge splitter,
		// where the sliced label is a get-result of unknown type). Leaving the
		// carrier un-narrowed only loosens matching, never tightens — sound.
		if slotIsPolymorphic(r, word, args, i, slot) {
			continue
		}
		bound := cur
		bound.Dynamic = false
		bound.SetDynFrom("")
		narrowed := core.TandValues(bound, core.NewCarrier(slot))
		// A successful match guarantees a non-disjoint intersection; skip
		// when the bound did not actually tighten (no-op / avoids
		// unbounded layer growth on repeated same-type uses).
		if core.IsNeverShape(narrowed) || core.ValuesEqual(bound, narrowed) {
			continue
		}
		// A narrowed intersection that collapses to a bare ROOT node (nil
		// Parent — None / Any / Never) must not be re-promoted to a dynamic
		// carrier: the resulting carrier would carry a nil bound that
		// matchSignature reads as "matches nothing", cascading false
		// no_signature through every downstream use of the name. None in
		// particular is the builder-pattern sentinel (a `none` accumulator that
		// becomes a node — tst/radix's `none entries […tst-insert] fold`);
		// tightening the binding to None breaks the node-rebuild words (get /
		// with-lo / mk-tnode) that follow. Leaving the binding at its prior,
		// broader dynamic bound only loosens matching — sound, never a new
		// false positive.
		if narrowed.Parent == nil {
			continue
		}
		// Identity-preserving rebind: the narrowing refines the STATIC bound
		// of the SAME runtime value — no new value exists at run time.
		// TandValues assembles the intersection as a fresh Value (fresh ID),
		// which would orphan the binding from its producing event: a
		// `def nodes (tree get "nodes")` narrowed to List at its first typed
		// use lost the get event's provenance, so every LATER read of the
		// name refused ("fn call operand of unknown provenance" — the
		// decision eval-tree walkers). Re-stamp the current binding's ID so
		// the recorder's producedBy still resolves the rebound name to its
		// original producer.
		narrowed.ID = cur.ID
		name := a.DynFrom()
		pushed := core.NewDynamicCarrierValue(narrowed)
		r.Defs.Push(name, pushed)
		// The push is an ANALYSIS artifact — the same runtime value under a
		// tighter static bound, not a binding transition (the bind ledger
		// rightly never records it) — so it must not outlive the pass: left
		// in place it SHADOWED the real binding for every later Run on the
		// same instance (a later read of module-io's `def l (IO.lock …)`
		// answered the leaked dynamic carrier instead of the Lock), and the
		// live-depth oracle read it as ledger-vs-live drift. The pop is
		// guarded on the entry still being on top: a narrowing truncated away
		// with its branch, or buried under a later real binding, is left
		// alone — LIFO cleanup ordering tears chained narrowings down
		// top-first. The DEPTH is part of the guard's identity token (Codex
		// P1, PR #418): a later check-mode rebind of the same name can bind a
		// carrier ValuesEqual deems equal to this one — value equality alone
		// would pop that REAL binding and re-expose the narrowing beneath —
		// but such a rebind sits at a DEEPER level, so requiring the depth at
		// which this narrowing was pushed rules the impostor out.
		depthAtPush := r.Defs.Depth(name)
		r.Check.AddPassEndCleanup(func() {
			if top, ok := r.Defs.TopEntry(name); ok && top.TypeDef == nil &&
				r.Defs.Depth(name) == depthAtPush &&
				core.ValuesEqual(top.Body, pushed) {
				r.Defs.Pop(name)
			}
		})
	}
}

// slotIsPolymorphic reports whether the word has a second, equally-reachable
// overload that constrains argument position i to a type other than
// matchedSlot. "Reachable" means same arity, every OTHER position matches the
// actual carrier args, and the dynamic value's bound is not provably disjoint
// from that overload's type at i. When true, position i is polymorphic for
// this call (e.g. slice's data arg: String vs List), so narrowing the dynamic
// carrier to matchedSlot alone would be unsound.
func slotIsPolymorphic(r *core.Registry, word string, args []core.Value, i int, matchedSlot *core.Type) bool {
	if word == "" {
		return false
	}
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return false
	}
	for k := range fn.Signatures {
		s := &fn.Signatures[k]
		if s.Fallback || s.TotalArgs() != len(args) || i >= s.TotalArgs() {
			continue
		}
		st := core.SigArgType(s, i)
		if st == nil || st.Equal(matchedSlot) {
			continue
		}
		reach := true
		for j := range args {
			if j == i {
				// The dynamic value at i: reachable unless provably disjoint
				// from this overload's type there.
				if core.IsNeverShape(core.TandValues(core.NewCarrier(args[j].Parent), core.NewCarrier(st))) {
					reach = false
					break
				}
				continue
			}
			if !core.SigTypeMatches(args[j], core.SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if reach {
			return true
		}
	}
	return false
}

// anyDynamicCarrier reports whether any value is a dynamic carrier — the
// trigger for gradual contagion in CarrierResults.
func AnyDynamicCarrier(vs []core.Value) bool {
	for _, v := range vs {
		if v.Dynamic {
			return true
		}
	}
	return false
}

// anyNonConcreteOperand reports whether any value is not a concrete
// payload-bearing value (a typed carrier or a bare type literal) — the
// operand shape under which a static CoreDefault match is not a dispatch
// proof (recordCallRefusal): the runtime tag may be a strict subtype a
// more-specific unlocked overload claims.
func AnyNonConcreteOperand(vs []core.Value) bool {
	for _, v := range vs {
		if !core.IsConcrete(v) {
			return true
		}
	}
	return false
}

// anyAnyCarrier reports whether any value is an Any-typed carrier — a value
// whose static type is unknown (a List/Map element, an opaque module-fn
// result). Unlike anyDynamicCarrier it does not require the Dynamic flag: a
// strict Any carrier conforms to no concrete container/operand type, so it
// fails matchSignature and reaches the no-signature recovery, where it is the
// signal that the dispatch is genuinely runtime-dynamic (poly), not a concrete
// type error.
func AnyAnyCarrier(vs []core.Value) bool {
	for _, v := range vs {
		if isAnyCarrier(v) {
			return true
		}
	}
	return false
}

// isAnyCarrier reports whether v is an Any-typed carrier (strict or gradual):
// a value whose static type is exactly Any, so at run time it may hold a value
// of any type. Such an arg to a multi-overload user fn makes the dispatch
// genuinely runtime-dynamic — the committed Any-slot arm is not a proof, since
// the runtime value may match a more-specific sibling overload.
func isAnyCarrier(v core.Value) bool {
	return v.Carrier && v.Parent != nil && v.Parent.Equal(core.TAny)
}

// anyDisjunctCarrier reports whether any value is a UNION (Disjunct) carrier — a
// receiver whose static type is a union of alternatives (`Map | Any` from an
// if-branch join, the trie/tst node-vs-none shape). Like an Any carrier it
// conforms to no single concrete overload, so matchSignature cannot commit and
// the dispatch reaches the no-signature recovery — but at run time it holds ONE
// concrete alternative, and the SAME first-match the interpreter takes
// dispatches it. So a poly-safe word (get/getr) over a union receiver records a
// runtime-re-matching OpCallNativePoly instead of refusing; a runtime member
// that matches no overload routes to the sound OpFallback island. (A bare
// disjunct carrier carries no DisjunctInfo payload, so IsDisjunct is false here —
// the carrier's Parent IS the union type, which is what we test.)
func anyDisjunctCarrier(vs []core.Value) bool {
	for _, v := range vs {
		if v.Carrier && v.Parent != nil && v.Parent.ConformsTo(core.TDisjunct) {
			return true
		}
	}
	return false
}

// anyImpreciseCarrier reports whether any value is a CARRIER of a concrete-but-
// imprecise type — a checker stand-in whose static type is a definite tag
// (Integer, String, List, …) that a multi-branch narrowing / carrier-join
// settled on, but which at run time may hold a value of a DIFFERENT type the
// static analysis could not pin (the deep guard-complement chain in the
// mini-redis codec's `join " " reply` — reply lands as a scalar carrier where
// `join`'s List slot cannot commit). Like an Any / Disjunct carrier it is NOT a
// concrete value (Carrier=true), so per the no-signature-recovery contract
// ("Concrete operands are a genuine type error and refuse; carriers recover")
// it is eligible for the runtime-re-matching OpCallNativePoly: tryRecordPoly's
// own poly-safety gates decide whether recovery is faithful, and a runtime
// value that matches no overload routes to the sound OpFallback island / raises
// the interpreter's byte-identical error. Bare type nodes (a type-literal
// operand of is/typeof) are excluded — they are not runtime values to re-match.
func anyImpreciseCarrier(vs []core.Value) bool {
	for _, v := range vs {
		if v.Carrier && v.Parent != nil && !core.IsBareTypeNode(v) {
			return true
		}
	}
	return false
}

// valueTreeHasCarriers reports whether v or any nested map value / list
// element is a carrier or dynamic — i.e. statically unknown. Used to scope
// the static generic-record construction validation to fully-literal data;
// carrier-bearing data defers to the runtime constructor.
func valueTreeHasCarriers(v core.Value) bool {
	if v.Carrier || v.Dynamic {
		return true
	}
	if m, err := core.AsMap(v); err == nil && m.Len() > 0 {
		for _, k := range m.Keys() {
			if mv, ok := m.Get(k); ok && valueTreeHasCarriers(mv) {
				return true
			}
		}
	}
	if l, err := core.AsList(v); err == nil && l.Len() > 0 {
		for _, e := range l.Slice() {
			if valueTreeHasCarriers(e) {
				return true
			}
		}
	}
	return false
}

// ReturnsFreshInstance is a ReturnsFunc for IMPURE constructors (make):
// each result is a fresh VALUE carrier of the TYPE named by args[mapping[i]],
// mirroring the runtime where every construction mints a new value
// (NewValueRaw). ReturnsIdentity returns args[m] verbatim — for `make` that
// is the requested TYPE LITERAL, which is wrong twice over:
//
//   - Identity. A type literal is dual (eng CLAUDE.md "Canonical *Type
//     Pointers"): its Value.ID IS the type's lattice identity, so every
//     `make P {}` returns the SAME P-literal value (one Value.ID). The
//     bytecode lowerer's per-value provenance (emit.go's producedBy) then
//     cannot tell two instances apart — it resolves both operands of a
//     downstream `(make P {}) (make P {}) eq` onto the LAST make, leaving
//     one simulated-stack slot where the binary op needs two ("stack
//     discipline underflow"). A fresh carrier ID keeps each construction a
//     distinct operand.
//   - Faithfulness. The runtime result is a VALUE of type P, not the type
//     literal P; a carrier of P is the same shape CarrierResults gives every
//     other declared return, and conformance sees the instance's type.
//
// (Minting a fresh ID on the type literal itself is NOT viable: it severs
// the literal↔type ID duality, so ValueType can no longer resolve the made
// type and conformance checks fail.)
//
// Only a concrete bare-type-node target is converted; a dynamic/computed
// target (a carrier already) keeps the prior identity behaviour.
func ReturnsFreshInstance(mapping ...int) core.ReturnsFunc {
	return func(args []core.Value, r *core.Registry) []core.Value {
		out := make([]core.Value, len(mapping))
		for i, m := range mapping {
			switch {
			case m < 0 || m >= len(args):
				out[i] = core.NewCarrier(core.TAny)
			case !args[m].Carrier && !args[m].Dynamic:
				// A concrete constructor target — a bare type node
				// (`make Pathon …`, `make Foo …`) or a structural type body
				// (`make P {}` for a class/record, whose literal carries an
				// ClassTypeInfo/RecordTypeInfo payload). ValueType yields the
				// made *Type in both cases (the node itself, or the body's
				// Parent); a fresh carrier of it is the per-call instance.
				//
				// A NAMED target evaluates to its minted node (the Stage 2
				// flip); the structural content the branches below inspect —
				// a generic schema, a record body — is the node's recorded
				// declaration (design/TYPE-REPRESENTATION.1.md §N2).
				target := args[m]
				if content, ok := core.TypeContentOf(target); ok && core.IsBareTypeNode(target) {
					target = content
				}
				//
				// EXCEPTION — a GENERIC RECORD SCHEMA target (`make Rule {…}`
				// where Rule is `gen [R] refine Record […]`): the schema node
				// is minted in the Ideal branch (parent Ideal/Record), but the
				// runtime instance is built by MakeRecordR and carries the
				// TMap tag — a schema-node carrier fails the Map conformance
				// the runtime accepts (the voxgig decision `make-rule … [Map]`
				// return-check false positive). Type the carrier faithfully to
				// the runtime tag: instantiate statically when the data is
				// concrete (the instantiated record body's ValueType sits in
				// the Map branch and keeps the field schema reachable), else
				// fall back to a plain TMap carrier.
				if info, serr := core.AsTypeSchema(target); serr == nil && info.Kind == core.SchemaRecord {
					if r != nil && len(args) == 2 && core.IsConcrete(args[1-m]) && !valueTreeHasCarriers(args[1-m]) {
						// CONCRETE construction data: validate statically, exactly as the
						// runtime will — infer + instantiate, then run the record
						// constructor. On SUCCESS the carrier takes the instantiated
						// body's Map-branch type. On FAILURE (uninferable params —
						// unbound_param — or a construction error like an unknown field)
						// the runtime RAISES, so fall through to the pre-exception
						// schema-node carrier below: its failed Map conformance is what
						// FLAGS the enclosing [Map] return (Codex PR #218 review — a
						// silent TMap carrier here read clean where the runtime raises;
						// pinned by lang/spec/generics.tsv:81 and the stage0 pins).
						validated := false
						if inst, ierr := core.InferAndInstantiateSchema(r, target, args[1-m]); ierr == nil {
							if rt, rerr := core.AsRecordType(inst); rerr == nil {
								if _, mkErr := core.MakeRecordR(rt, args[1-m], false, r); mkErr == nil {
									c := core.NewCarrier(core.CanonicalType(r, core.ValueType(inst)))
									// Ride the instantiated FIELD SCHEMA on the
									// instance carrier (the recordSchemaCarrier
									// shape: dynamic + RecordTypeInfo payload) so a
									// downstream field read narrows via the
									// accessor ReturnsFns instead of ending
									// dynamic(Any) — `(make Test.TestCase {…}) get
									// "out"`. Gradual, guard-discharged.
									if rt.Fields != nil {
										c.Data = rt
										c.Dynamic = true
									}
									out[i] = c
									validated = true
								}
							}
						}
						if validated {
							continue
						}
						// fall through to the schema-node carrier (flags the return)
					} else {
						// Non-concrete data, or a concrete map holding CARRIER members
						// (`{kind:(table.kind) …}` — field values statically unknown):
						// whether construction succeeds is unknowable, so the optimistic
						// Map carrier is the gradual posture — matching the runtime tag
						// on success. The pre-fix schema-node carrier flagged EVERY such
						// make (the voxgig decision make-rule / with-policy FPs).
						c := core.NewCarrier(core.TMap)
						// A PARAMETERLESS record schema's field TYPES are static
						// regardless of the construction data — ride them so a
						// module constructor's `make TestCase {name:name …}`
						// (carrier-membered data) still narrows downstream field
						// reads. Generic schemas with unresolved params keep the
						// plain Map carrier.
						if len(info.Params) == 0 {
							if rt, rerr := core.AsRecordType(info.Body); rerr == nil && rt.Fields != nil {
								c.Data = rt
								c.Dynamic = true
							}
						}
						out[i] = c
						continue
					}
				}
				t := core.ValueType(target)
				if r != nil {
					t = core.CanonicalType(r, t)
				}
				c := core.NewCarrier(t)
				// A plain RECORD BODY target (`def TC refine Record […]` —
				// the binding is the body value, Parent TMap, RecordTypeInfo
				// payload): ride the declared field schema on the instance
				// carrier so downstream field reads narrow (same shape and
				// rationale as the schema-record branches above).
				if rt, ok := target.Data.(core.RecordTypeInfo); ok && rt.Fields != nil {
					c.Data = rt
					c.Dynamic = true
				}
				out[i] = c
			default:
				// Dynamic / computed target (a carrier already): keep the
				// prior identity behaviour.
				out[i] = args[m]
			}
		}
		return out
	}
}

// ReturnsStatic builds a ReturnsFunc that always produces a fixed list
// of carrier types, independent of args. Equivalent to setting Returns
// directly; provided so ReturnsFn call sites can be uniform.
func ReturnsStatic(types ...*core.Type) core.ReturnsFunc {
	return func(_ []core.Value, _ *core.Registry) []core.Value {
		out := make([]core.Value, len(types))
		for i, t := range types {
			out[i] = core.NewCarrier(t)
		}
		return out
	}
}

// ReturnsDynUnion models a declared value-or-sentinel union — env's
// String-or-none, read-line's line-or-EOF, stat's record-or-absent — as a
// DYNAMIC disjunct carrier over the alternatives. Dynamic, deliberately:
// a STRICT union at these words would refuse every gradual call site that
// feeds the result straight into a typed slot without a none-guard (the
// pattern real io code uses everywhere), trading false negatives for
// false positives. The dynamic bound keeps optimistic matching — the
// runtime re-checks the concrete value, the same contract as every other
// dynamic-modality hatch — while surfacing the real alternative set
// instead of dynamic(Any). The precedent shape is the typed-patrun find
// result, dynamic(Function ∪ None).
// The union is minted INSIDE the closure, per invocation — never hoisted
// to construction time. NewDynamicCarrierValue preserves its bound's
// identity, and a hoisted bound gives every call site of every word
// sharing the model ONE value ID (minted outside any check pass, so in
// fact the empty ID). Identity is how the pipeline tracks values; a
// shared ID let the def binder alias three separate read-line results to
// one slot, which the compiled program then read back as three copies of
// the LAST read — the TestReadLineStdinAdvances miscompile. ReturnsStatic
// mints per call for the same reason; this must too.
func ReturnsDynUnion(types ...*core.Type) core.ReturnsFunc {
	return func(_ []core.Value, _ *core.Registry) []core.Value {
		alts := make([]core.Value, len(types))
		for i, t := range types {
			alts[i] = core.NewTypeLiteral(t)
		}
		return []core.Value{core.NewDynamicCarrierValue(core.NewDisjunct(alts))}
	}
}

// ReturnsNumericBinary models the arithmetic-tower result type for
// add/sub/mul/div/mod/pow on [TNumber, TNumber]: the widest leaf wins
// among the exact types (Integer < BigInteger < BigDecimal); an
// Integer⊕Float mix is Float. A Big⊕Float mix errors at runtime (the
// exact types never silently become Float); statically it is modelled as
// the Big leaf so analysis can continue past it.
func ReturnsNumericBinary() core.ReturnsFunc {
	return func(args []core.Value, _ *core.Registry) []core.Value {
		if len(args) != 2 {
			return []core.Value{core.NewCarrier(core.TFloat)}
		}
		a, b := args[0].Parent, args[1].Parent
		switch {
		case a.ConformsTo(core.TBigDecimal) || b.ConformsTo(core.TBigDecimal):
			return []core.Value{core.NewCarrier(core.TBigDecimal)}
		case a.ConformsTo(core.TBigInteger) || b.ConformsTo(core.TBigInteger):
			return []core.Value{core.NewCarrier(core.TBigInteger)}
		case a.ConformsTo(core.TFloat) || b.ConformsTo(core.TFloat):
			return []core.Value{core.NewCarrier(core.TFloat)}
		default:
			return []core.Value{core.NewCarrier(core.TInteger)}
		}
	}
}

// ReturnsAddConcat types the result of add's string-concat overloads
// ([TString TScalar] / [TScalar TString]). Those overloads commit whenever an
// operand fills the String slot — but a GRADUAL (dynamic) operand fills it
// OPTIMISTICALLY: it MIGHT be a String, yet at run time it could be a Number,
// in which case the interpreter takes the NUMERIC overload instead. A definite
// [String] return for that case wrongly rejects a downstream numeric use — the
// sort.boru `msd-go` false positive, where `lo add (Array-get-result)` (Integer
// + a dynamic get) was typed String and then failed the recursive `lo:Integer`
// param. So: return String only when an operand is PROVABLY String (a concrete
// or non-gradual String-typed value); otherwise the result is String-or-Number,
// i.e. a gradual Scalar, which matches a later numeric OR string use and a guard
// discharges back to strict. Runtime concat (addConcatHandler) is unchanged —
// this governs check-mode result typing only.
func ReturnsAddConcat() core.ReturnsFunc {
	return func(args []core.Value, _ *core.Registry) []core.Value {
		for _, a := range args {
			if !a.Dynamic && a.Parent != nil && a.Parent.ConformsTo(core.TString) {
				return []core.Value{core.NewCarrier(core.TString)}
			}
		}
		return []core.Value{core.NewDynamicCarrier(core.TScalar)}
	}
}

// carrierMixedConform reports whether v is a genuinely MIXED gradual
// carrier with respect to type t: a non-concrete Disjunct carrier whose
// flattened alternatives include at least one that conforms to t AND at
// least one that does not. Such a carrier makes a forward/stack dispatch
// split AMBIGUOUS — a concrete value drawn from it could match a more-
// specific overload (and be grabbed from the stack) or not (and be
// skipped, so the word forward-collects instead). The two splits leave
// different stacks, so the bytecode compiler cannot statically reproduce
// the runtime one. A pure carrier (all alternatives conform, or none do)
// and a bare dynamic carrier are NOT mixed and return false: the split is
// consistent, or is the dynamic path handled elsewhere.
func carrierMixedConform(v core.Value, t *core.Type) bool {
	if t == nil || !v.Carrier || core.IsConcrete(v) || v.Dynamic {
		return false
	}
	alts := core.FlattenAlternatives(v)
	if len(alts) < 2 {
		return false
	}
	someConform, someReject := false, false
	for _, alt := range alts {
		// core.FlattenAlternatives yields type LITERALS; the denoted type is
		// typeNodeOf(alt), NOT alt.Parent (a literal's Parent is the
		// denoted node's lattice parent — the `Boolean` literal's Parent
		// is `Scalar`).
		if node := core.TypeNodeOf(alt); node != nil && node.ConformsTo(t) {
			someConform = true
		} else {
			someReject = true
		}
	}
	return someConform && someReject
}

// loopAnalysisRounds bounds the Kleene iteration for loop-body
// analysis: round 1 with the pre-loop bindings, then re-runs with the
// joined bindings until stable. Three rounds suffice for ascent in a
// join-semilattice whose height is bounded by core.CarrierDisjunctCap.
const loopAnalysisRounds = 3

// FnAnalysisQuota caps how many distinct call shapes one fn's body is
// analysed for before the checker answers from the declaration (A9).
const FnAnalysisQuota = 64

// AnalyseLoopBody analyses a loop body to a bounded fixed point
// (design/checker-accuracy-review.10.md A4). Each round binds the
// loop's own names (iterator …) as carriers, runs the body, and
// JOINS the body's net def additions back into the enclosing
// bindings — "the loop may run zero times" is the join with the
// pre-loop binding, exactly core.InstallJoinedDefs' one-branch rule. If
// the joined bindings changed, the body is re-analysed with them (a
// rebinding like `def acc (acc add 0.5)` needs the second round to
// see Integer|Float), up to loopAnalysisRounds.
//
// Only the FINAL round's diagnostics are kept — earlier rounds run
// against not-yet-stable bindings and would both duplicate and
// misreport.
//
// The joined post-loop bindings are left installed (they ARE the
// post-loop environment); the loop-local binds are popped. Returns
// the final round's residual carrier stack.
//
// provenTrips asserts the CALLER's proof that the loop executes at least
// once at run time — a static trip count >= 1 (forCarrierAnalyse's
// staticBounds). Combined with the body carrying no flow-control sentinel
// (bodyHasSentinel — a break/continue/return can bypass a site or discard
// an iteration's spilled values), it gates the LoopBodyDepth stamp the
// S9.2a first-value split consults: the split's soundness argument is
// "every enclosing body runs unconditionally per iteration AND the split
// site is reached with its residual intact", which a computed count (zero
// trips leak the analysis-only binding) or loop control (PR #280 review:
// `continue` bypassed the bind, `break` kept a discarded iteration's
// value) breaks. A non-proven loop body still analyses identically — it
// just declines the split (NestedBodyDepth != LoopBodyDepth).
func AnalyseLoopBody(r *core.Registry, body core.Value, bindNames []string, bindVals []core.Value, provenTrips bool) []core.Value {
	proven := provenTrips && !BodyHasSentinel(body)
	// Loop-lowering hook (`for`): when armed, register the loop
	// bindings as VM locals and capture each round's events as a
	// fragment — the final round's capture (the stable one) is what
	// the caller's RecordLoop consumes via TakeFragment.
	es := r.Check.Recorder()
	loopCapture := es.ConsumeLoopArm()
	if loopCapture {
		for _, v := range bindVals {
			es.RegisterLocal(v.ID)
		}
		// Loop-carried def rebinds: a pre-loop `def` the body REBINDS gets a
		// unit frame slot (NoteLoopCarried per round below), a store at each
		// rebind site (RecordDefRebind, from the def handler), and a slot
		// resolution for every round's joined binding — so a read on a later
		// iteration or after the loop compiles instead of refusing "operand
		// of unknown provenance". EndLoopCarried exposes the slot inits to
		// the RecordLoop that follows this analysis.
		es.BeginLoopCarried()
		defer es.EndLoopCarried()
	}
	var stk []core.Value
	var installed []string
	diagBase := len(r.Check.Diagnostics)
	prev := map[string]core.Value{}
	for round := 0; round < loopAnalysisRounds; round++ {
		r.Check.TruncateDiagnostics(diagBase)
		for i, n := range bindNames {
			r.Defs.Push(n, bindVals[i])
		}
		// Checkpoint the recording pools before an armed round: only the FINAL
		// (stabilised) round's fragment is kept, so a non-final round's interned
		// consts / island spans and its SiteCounts are rolled back rather than
		// orphaned into the Program (bytecode artifact + metric bloat). The
		// def-stack convergence (r.Defs) is independent of the recording, so it
		// proceeds across rounds untouched.
		var cp core.EmitCheckpoint
		if loopCapture {
			cp = es.Checkpoint()
			es.ArmBranchCapture()
		}
		var adds map[string]core.Value
		if proven {
			r.Check.LoopBodyDepth++
		}
		stk, adds = core.RunCarrierBodyWithDefs(r, body)
		if proven {
			r.Check.LoopBodyDepth--
		}
		for i := len(bindNames) - 1; i >= 0; i-- {
			r.Defs.Pop(bindNames[i])
		}
		// Expose the original pre-loop bindings before re-joining.
		for i := len(installed) - 1; i >= 0; i-- {
			r.Defs.Pop(installed[i])
		}
		installed = installed[:0]
		joined := map[string]core.Value{}
		// Deterministic name order: carried-slot allocation and the joined
		// installs must not depend on Go map iteration (slot numbering and
		// the emit goldens would jitter run to run).
		names := make([]string, 0, len(adds))
		for k := range adds {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			v := adds[k]
			if pre, ok := r.Defs.Top(k); ok {
				// An add that is only a NARROWING of the enclosing binding —
				// narrowDynamicUses preserves the value's ID, so same ID as
				// the pre binding means the same runtime value under a
				// tighter static bound — is the pass's own refinement, not a
				// binding the loop leaves: no join, no carried slot, no
				// ledger note (core.InstallJoinedDefs applies the identical
				// rule at the branch join; Codex P2, PR #418 — a body that
				// merely consumed a dynamic name through a typed slot
				// recorded a phantom `def` a twin would replay).
				if v.Dynamic && v.ID != "" && v.ID == pre.ID {
					continue
				}
				j := core.JoinCarriers(v, pre)
				joined[k] = j
				if loopCapture {
					// A rebind of a PRE-EXISTING binding is loop-carried:
					// register (or refresh) its slot and alias this round's
					// joined ID so the next round's / post-loop reads resolve.
					es.NoteLoopCarried(k, j, pre)
				}
			} else {
				joined[k] = v
			}
		}
		for _, k := range names {
			jv, ok := joined[k]
			if !ok {
				// A narrowing-only add was skipped above — nothing to install.
				continue
			}
			r.Defs.Push(k, jv)
			installed = append(installed, k)
		}
		// Stabilised when the body adds no bindings (the common single-round
		// case) or the joined bindings equal the previous round's. The final
		// round is the one that stabilises, or the last permitted round.
		stable := len(joined) == 0
		if !stable {
			stable = len(joined) == len(prev)
			if stable {
				for k, v := range joined {
					pv, ok := prev[k]
					if !ok || !core.ValuesEqual(pv, v) {
						stable = false
						break
					}
				}
			}
			prev = joined
		}
		if loopCapture && !stable && round < loopAnalysisRounds-1 {
			es.Rollback(cp)
		}
		if stable {
			break
		}
	}
	// The FINAL round's joined pushes are the post-loop environment — the
	// bindings the pass actually LEAVES — so the bind ledger records them,
	// exactly as InstallJoinedDefs records a branch join (§6.5: the loop's
	// "may run zero times" join is that function's one-branch rule). Noted
	// here, after the rounds settle, because a non-final round's pushes are
	// popped at the top of the next round and never leave the pass. Recorded
	// in `installed` (push) order so chained depths compose. The position
	// falls to NoteBindTransition's CurWordPos floor — measured, that is a
	// def token INSIDE the body (CurWordPos has moved by join time), a real
	// site but not the construct's own: the same join-position limitation
	// §6.5 already names for the `if`, to revisit with the twin op. The
	// corpus could not see the gap: like NUR110's branch-arm class, every
	// loop-body def it holds sits inside a fn body, where FnBodyDepth
	// suppresses the note; the synthetic while row supplies it.
	for _, k := range installed {
		r.NoteBindTransition(core.BindDef, k, core.SrcPos{})
	}
	return stk
}

// FnAnalysisKey builds the memo key for one fn-body analysis: scope id +
// name + arg type paths + captured-name set + the body's construction
// site. The captures are included so two anonymous lambdas with identical
// bodies but different capture sets don't collide; the construction
// site (the first body token's source position) so two DIFFERENT
// definitions sharing a name and arg types — `def f fn […] … def f
// fn […]` — don't collide either (without it the compile pass bound
// every call to the FIRST definition's code unit). The same
// construction re-analysed (recursion, repeated calls) carries the
// same body tokens, so memoisation and in-flight recursion detection
// are unaffected.
//
// scopeID is the AnalysisScopeID of the registry whose body is being
// analysed. It namespaces the key so a module sub-registry's fn cannot
// alias a same-named, same-positioned parent fn once a check pass is
// shared across registries (design/module-fn-checkstate-ownership.1.md
// §5a) — the position suffix alone does not disambiguate, because parent
// and module are parsed from independent sources whose positions overlap.
// core_helpers' compile hook must build the SAME key (its FnSummaries
// delete relies on the match) — that's why this is a named helper, not
// two inlined loops, and why every caller passes r.AnalysisScopeID() of
// the same registry r that AnalyseFnBody runs the body in.
// carrierTypeName names the lattice type a carrier value denotes, for memo
// keying. A normal carrier carries its type in Parent; a root-node carrier
// (None / Any / Never) has a nil Parent because it IS its own lattice node, so
// its name comes from the value itself. Never deref Parent unguarded here — a
// `none` fold-seed promoted to a dynamic carrier reaches this path with a nil
// Parent and would otherwise panic (tst's `map-from-entries` accumulator).
func carrierTypeName(v core.Value) string {
	if v.Parent != nil {
		return v.Parent.String()
	}
	return v.String()
}

func FnAnalysisKey(scopeID uint64, name string, args []core.Value, captures []core.CapturedBinding, body []core.Value) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(scopeID, 10))
	sb.WriteByte('#')
	sb.WriteString(name)
	sb.WriteByte('#')
	for _, a := range args {
		sb.WriteString(carrierTypeName(a))
		sb.WriteByte(',')
	}
	if len(captures) > 0 {
		sb.WriteByte('|')
		for _, cb := range captures {
			sb.WriteString(cb.Name)
			sb.WriteByte(':')
			sb.WriteString(carrierTypeName(cb.Value))
			sb.WriteByte(',')
		}
	}
	if len(body) > 0 {
		sb.WriteByte('@')
		pos := body[0].Pos()
		if pos.Row == 0 && pos.Col == 0 {
			// A body whose first token carries no position — a lambda body
			// that is one `=>` group — identifies itself by its canonical
			// text instead. Keyed on the (zero) position, two such lambdas
			// of one shape shared one key, one summary and one compiled
			// UNIT: cnot's inner `t:Any => [f:Any => …]` and cif's inner
			// `t:Any => [e:Any => …]` (both a `p:Function` capture over
			// `t:Any`) collided, and cif's returned closure ran cnot's body
			// (`'F' ('T' (cif (cnot ctrue/v)) apply) apply` answered T for
			// the interpreter's F — the thirtieth increment).
			for _, v := range body {
				sb.WriteString(core.CanonValue(v))
				sb.WriteByte(' ')
			}
		} else {
			sb.WriteString(strconv.Itoa(pos.Row))
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(pos.Col))
		}
	}
	return sb.String()
}

// fnQuotaKey identifies a fn DEFINITION for the per-fn analysis quota:
// scope + name + body source position. It deliberately differs from
// FnAnalysisKey in two ways. It OMITS the arg shapes — the quota exists to
// COUNT distinct arg shapes per definition, so the shapes cannot be part of
// the key that groups them. And unlike a bare name it distinguishes the many
// closure bodies that share a synthetic "<word>$body" name (each$body,
// fold$body, scan$body, …) by their source position: a name-only key
// conflated every higher-order closure in the whole program under one budget,
// so a module with 64+ distinct `each` loops exhausted the quota and forced
// later loops to bail to a provenance-less dynamic Any — which the compiler
// then refused ("code-body word each (Stage 2)"). Keyed by definition site,
// each distinct closure gets its own budget while the same body re-analysed
// under many arg shapes still shares one counter.
// The name may be empty (a transparent anonymous body); it is written
// verbatim, since the body position that follows already distinguishes
// distinct anonymous sites. The human-facing diagnostic uses a separate
// displayName, so the key need not spell "<anon>".
func fnQuotaKey(scopeID uint64, name string, body []core.Value) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(scopeID, 10))
	sb.WriteByte('#')
	sb.WriteString(name)
	if len(body) > 0 {
		sb.WriteByte('@')
		pos := body[0].Pos()
		if pos.Row == 0 && pos.Col == 0 {
			// A body whose first token carries no position — a lambda body
			// that is one `=>` group — identifies itself by its canonical
			// text instead. Keyed on the (zero) position, two such lambdas
			// of one shape shared one key, one summary and one compiled
			// UNIT: cnot's inner `t:Any => [f:Any => …]` and cif's inner
			// `t:Any => [e:Any => …]` (both a `p:Function` capture over
			// `t:Any`) collided, and cif's returned closure ran cnot's body
			// (`'F' ('T' (cif (cnot ctrue/v)) apply) apply` answered T for
			// the interpreter's F — the thirtieth increment).
			for _, v := range body {
				sb.WriteString(core.CanonValue(v))
				sb.WriteByte(' ')
			}
		} else {
			sb.WriteString(strconv.Itoa(pos.Row))
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(pos.Col))
		}
	}
	return sb.String()
}

// declaredReturnBail synthesizes the result carriers for an analysis that
// cannot (or need not) run the body: one fresh carrier per declared return
// type, or a single dynamic Any when nothing is declared. Shared by the
// quota and in-flight-recursion short circuits.
func declaredReturnBail(declared []*core.Type) []core.Value {
	if len(declared) > 0 {
		out := make([]core.Value, len(declared))
		for i, t := range declared {
			out[i] = core.NewCarrier(t)
		}
		return out
	}
	return []core.Value{core.NewDynamicCarrier(core.TAny)}
}

// refineRecursiveSummary is AnalyseFnBody's bounded Kleene refinement (A2):
// the first run consumed an Any in-flight bail, so the summary was computed
// under the weakest hypothesis. Seed the memo with it and re-analyse — the
// recursive call now reads the seeded hypothesis from the cache — joining
// each round's result into the hypothesis until stable, up to two extra
// rounds. Only the final round's diagnostics are kept.
func refineRecursiveSummary(r *core.Registry, key string, diagBase int, result []core.Value, runOnce func() []core.Value) []core.Value {
	for round := 0; round < 2; round++ {
		r.Check.FnSummaries[key] = result
		r.Check.TruncateDiagnostics(diagBase)
		next := runOnce()
		joined := core.JoinCarrierStacks(result, next)
		if carrierStacksEqual(joined, result) {
			break
		}
		result = joined
	}
	return result
}

// RunFnBodyOnce performs one full body analysis for AnalyseFnBody: snapshot
// def-stack depths so any defs the body, captures, or parameter bindings
// created unwind afterwards. The same snapshot is pushed as the fn-entry
// baseline so any inner fn/afn construction inside the body sees this scope
// as its enclosing-fn baseline — without it, ComputeCaptures would treat
// outer params as if they lived at module/global scope and miss the capture.
func RunFnBodyOnce(r *core.Registry, name string, paramNames []string, body, args []core.Value, captures []core.CapturedBinding, anonymous bool) []core.Value {
	snapshot := r.Defs.Snapshot()
	r.PushFnBaseline(snapshot)
	defer r.PopFnBaseline()

	// Expose the params as the per-call args list so a body that reads
	// `args` / `args.N` resolves them in check mode. The params ARE the
	// frame's leading locals (0..n-1), so `args.N` folds to that local — at
	// lowering it becomes PUSH_LOCAL N, no runtime args stack needed
	// (CarrierResults' `args` projection + tryFoldStaticIndex). Pushed
	// alongside the FnBaseline; popped together.
	_ = r.Args.Push(core.NewList(append([]core.Value(nil), args...)))
	defer func() { _, _ = r.Args.Pop() }()

	// Install lexical captures first so params (installed below)
	// shadow same-named captures — innermost binding wins,
	// matching runtime dispatch.
	for _, cb := range captures {
		r.Defs.Push(cb.Name, cb.Value)
	}

	// Bind named parameters as simple defs (carrier-typed).
	// Unnamed parameters flow through the stack — push them
	// before the body.
	var input []core.Value
	hasUnnamed := false
	for i, arg := range args {
		if i < len(paramNames) && paramNames[i] != "" {
			r.Defs.Push(paramNames[i], arg)
		} else {
			// An unnamed FN-VALUE param is inert frame DATA under the
			// arguments-are-inert unification (the interpreter no longer
			// auto-fires a frame argument — design/ARG-SEMANTICS-
			// UNIFICATION.0.md), and the unit model now mirrors the frame
			// exactly: the value re-pushes at entry, `args.N` folds to its
			// local, and the RET's NUnnamed trim discards an unconsumed
			// copy — so the former guard refusal is retired.
			input = append(input, arg)
			hasUnnamed = true
		}
	}
	unnamedCount := len(input)
	input = append(input, body...)

	// Record whether this frame has stack-flowing unnamed params so a body
	// `args` / `args.N` read can refuse in compile mode (the unnamed input
	// stays live on the body stack; folding args.N over it strands the input
	// — design/EDGE-SPEC-FINDINGS.0.md §4). Save/restore around the body run
	// so nested analyses see their own frame's answer.
	prevUnnamed := r.Check.ArgsFrameUnnamed
	r.Check.ArgsFrameUnnamed = hasUnnamed
	defer func() { r.Check.ArgsFrameUnnamed = prevUnnamed }()

	sub := core.New(r)
	// The unnamed inputs are the frame's resolved-argument prefix: inert
	// data the collection-hazard scan must never mark (Engine.InertPrefix —
	// an unnamed Function param at the frame bottom collects nothing).
	sub.InertPrefix = unnamedCount
	// When this body is being RECORDED into a compiled CALLBACK unit (a
	// stored-fn / spawn body — see compileStoredFnUnit / compileStoredBody,
	// name "storedfn$body" / "spawnbody$body"), mark the body sub-engine
	// element-eval-recordable so a residual COMPUTED container it returns
	// (`{message: (join …)}` / `[a b]`, a bare map/list result) records its
	// OpMakeMap / OpMakeList assembly instead of leaving an unresolvable
	// residual that refuses "body result of unknown provenance" (the
	// mini-redis catch-all shape). Mirrors the branch arm's treatment
	// (core.RunCarrierBodyWithDefs, peekCaptureArm).
	//
	// Admitted for CALLBACK bodies and MULTI-TOKEN fn bodies. A callback is
	// only ever invoked via InvokeCallback / CallBoru, which evaluate the
	// body residual IN the live frame on both engines. A multi-token body's
	// trailing computed container now ALSO evaluates in-frame on every
	// interpreter dispatch path — CallBoru-class and same-registry spliced,
	// consumed and unconsumed alike (mini-s3's s3-parse-range
	// `{from: from upto: upto}`; the historical spliced-path deferral that
	// blocked this admission is gone) — so the recorded OpMakeMap /
	// OpMakeList (which re-assembles per run) matches the interpreter
	// exactly. A SINGLE-LITERAL container body must NOT enable this: that
	// is the pinned no-closures transparency (`def mk fn [[c1:Integer]
	// [List] [[c1]]] mk 9` → the MODULE binding, def-node-binding.tsv §3 —
	// maps behave identically), where in-frame assembly would bake the
	// param and diverge. Such a body keeps refusing and falls back,
	// byte-identically. The admission is BodyEvalsResidual — the same
	// predicate the frame-tail builders use — so a single
	// paren-expression body (which the interpreter also evaluates
	// in-frame) records too.
	// A NAMED fn (!anonymous) always evaluates its body in-frame, so its
	// residual container is assembled against the live params and records
	// like any in-frame computation. Only an anonymous lambda keeps the
	// single-bare-literal transparency (BodyEvalsResidual), where in-frame
	// assembly would bake the param and diverge — that shape keeps refusing.
	if r.Check.Recorder().Active() && (isCallbackBodyName(name) || !anonymous || core.BodyEvalsResidual(body)) {
		sub.ElemEvalRecordable = true
	}
	result, err := sub.Run(input)
	if err != nil {
		r.Check.AddDiagnostic(core.CheckDiagnostic{
			Code:   "fn_body_error",
			Detail: "fn body analysis error for " + name + ": " + err.Error(),
			Word:   name,
		})
		// A body that ERRORS under an ARMED recording (this analysis is being
		// compiled into an open fn/closure unit) cannot be faithfully lowered:
		// the unit would close EMPTY (the error aborted the body before its
		// residual recorded), and an empty unit SILENTLY DIVERGES from the
		// interpreter (a `var`-let or higher-order body whose check-mode error
		// is mere imprecision — e.g. `get` on an element carrier — runs fine at
		// runtime, so the VM's empty closure raises `body produced no result`
		// where the interpreter succeeds). Refuse so the program falls back to
		// the interpreter instead. Only when active: a SUSPENDED (plain) nested
		// analysis records nothing anyway and must not latch the program.
		if es := r.Check.Recorder(); es.Active() {
			es.MarkUncompilable("fn body analysis error in " + name + ": " + err.Error())
		}
		result = nil
	}
	r.Defs.Restore(snapshot)
	return result
}

// isCallbackBodyName reports whether name is a stored-fn / spawn callback
// body — compileClosureBody builds "storedfn$body" / "spawnbody$body" for the
// words "storedfn" / "spawnbody" (callable_words.go). Such a body is invoked
// only via InvokeCallback / CallBoru, which evaluate a residual COMPUTED
// container (`{message: (join …)}` / `[a b]`) IN the live frame on both
// engines, so recording its OpMakeMap / OpMakeList assembly is safe (it
// re-assembles per run, matching the interpreter). A normal user fn applied
// directly at top level leaves a DEFERRED residual the interpreter evaluates
// after the frame pops — recording there would diverge, so RunFnBodyOnce gates
// elemEvalRecordable on this predicate.
func isCallbackBodyName(name string) bool {
	return name == "storedfn$body" || name == "spawnbody$body"
}

// AnalyseFnBody runs a user-defined fn body through a sub-engine in
// check mode, treating named parameters as deffed values bound to
// their arg carriers and unnamed parameters as pre-pushed stack
// values. Results are cached on the registry keyed by (name,
// arg-types) so recursive functions converge instead of looping.
//
// declared is the signature's declared return types (nil =
// unchecked). It is the induction hypothesis for recursion
// (design/checker-accuracy-review.10.md A2): an in-flight recursive
// call yields carriers of the DECLARED returns — the end-of-body
// return check is the matching proof obligation — instead of the
// everything-matches Any. For unchecked fns the Any bail-out
// remains, but a summary computed under it is refined by bounded
// re-analysis (the bail's result seeds the memo as the hypothesis
// for the next round) rather than cached as final.
//
// Returns the residual carrier stack. An empty or nil return means
// the analyser aborted (recursion detected or body not available) —
// callers should treat that as an Any carrier.
func AnalyseFnBody(r *core.Registry, name string, paramNames []string, body []core.Value, args []core.Value, captures []core.CapturedBinding, declared []*core.Type, anonymous bool) []core.Value {
	// Fn-body analysis runs nested sub-engines — not part of the
	// caller's straight line; pause bytecode recording, UNLESS a fn
	// compilation armed capture (StartFnCompile): the body's events then
	// record into the open fn fragment. Resolved at ENTRY, above every
	// early return below, so an armed analysis that bails early — empty
	// body, a cached summary, the per-fn analysis quota, or in-flight
	// recursion — still CONSUMES the one-shot fnArm rather than stranding
	// it for the next, unrelated AnalyseFnBody (whose guard would then
	// mis-consume it and leak that body's events into the live fn
	// fragment). Mirrors how captureArm/loopArm are consumed at the top
	// of their analysis functions.
	defer r.Check.Recorder().FnBodyGuard()()
	if len(body) == 0 {
		return nil
	}
	// Record the caller→callee edge for the dynamic-scope undefined-word
	// rescue. The current top of FnNameStack is the fn whose body is executing
	// and just triggered this dispatch (empty at the top level). Recorded
	// BEFORE the memo / quota / in-flight early-returns below so a cached or
	// recursively-bailing call still contributes its edge — the self-edge of a
	// recursive fn in particular.
	if name != "" {
		if n := len(r.Check.FnNameStack); n > 0 {
			r.Check.RecordCallEdge(r.Check.FnNameStack[n-1], name)
		}
	}
	// A FORWARD-referenced fn name that isn't defined yet leaks into this
	// per-call-site analysis as a concrete Undefined Atom argument. Gradualize
	// it to a dynamic Any carrier at the analysis boundary — copy-on-write so
	// the caller's slice is untouched — and note the def-read so the
	// dynamic-scope undefined-word rescue still attributes it. Otherwise the
	// phantom concrete value drives dispatch and memo keying as if it were a
	// real Atom, a false no_signature on the forward-ref call. Mirrors the
	// dispatch-arg gradualization in engine.go's match loop.
	sanitized := false
	for i := range args {
		if !args[i].Undefined {
			continue
		}
		if !sanitized {
			args = append([]core.Value(nil), args...)
			sanitized = true
		}
		c := core.NewDynamicCarrier(core.TAny)
		if a, aerr := core.AsAtom(args[i]); aerr == nil && a != "" {
			r.Check.Recorder().NoteDefRead(c.ID, a)
		}
		args[i] = core.WithPos(c, args[i])
	}
	key := FnAnalysisKey(r.AnalysisScopeID(), name, args, captures, body)

	if r.Check.FnSummaries == nil {
		r.Check.FnSummaries = map[string][]core.Value{}
	}
	if r.Check.FnInflight == nil {
		r.Check.FnInflight = map[string]bool{}
	}
	if cached, ok := r.Check.FnSummaries[key]; ok {
		return cached
	}
	// Per-fn analysis quota (A9): a polymorphic helper reached with
	// many distinct arg shapes re-analyses once per shape; past the
	// quota, answer from the declaration (or dynamic Any) and say so
	// once, instead of silently consuming the global step budget. Keyed
	// by DEFINITION SITE (fnQuotaKey), not by bare name, so genuinely-
	// distinct closure bodies that share a synthetic "<word>$body" name
	// do not pool one budget across the whole program.
	if r.Check.FnAnalysisCounts == nil {
		r.Check.FnAnalysisCounts = map[string]int{}
	}
	quotaKey := fnQuotaKey(r.AnalysisScopeID(), name, body)
	displayName := name
	if displayName == "" {
		displayName = "<anon>"
	}
	r.Check.FnAnalysisCounts[quotaKey]++
	// The quota is a CHECK-mode accuracy/step-budget heuristic: past the quota
	// it fabricates a declared-return `declaredReturnBail` carrier that no event
	// produced. That carrier has no producedBy entry, so a real COMPILE pass —
	// which must lower the body's true residual to an operand — cannot resolve
	// it and refuses "fn <name>: body result of unknown provenance" (the trie
	// `match-go` / recursive-collector shape, pushed past the name-keyed count
	// by the re-entrant Test.test compile scopes). The compiler must see the
	// real body events, so the quota does not apply while Compiling; recursion
	// still terminates via the FnInflight cycle-breaker below, and runaway
	// non-recursive shape growth is bounded by the step budget.
	if n := r.Check.FnAnalysisCounts[quotaKey]; n > FnAnalysisQuota && !r.Check.Compiling {
		if n == FnAnalysisQuota+1 {
			r.Check.AddDiagnostic(core.CheckDiagnostic{
				Code: "analysis_truncated",
				Detail: "fn " + displayName + " was analysed for more than " +
					strconv.Itoa(FnAnalysisQuota) + " distinct call shapes; later shapes are typed from the declaration (or dynamic Any) without body re-analysis",
				Word: name,
			})
		}
		return declaredReturnBail(declared)
	}
	if r.Check.FnInflight[key] {
		// Recursion detected. With declared returns, the declaration
		// is the induction hypothesis (assume-guarantee) — precise,
		// and the body's return check is the proof obligation. Without
		// it, break the cycle with an Any carrier and count the bail
		// so the enclosing analysis knows its result needs refinement.
		if len(declared) > 0 {
			return declaredReturnBail(declared)
		}
		r.Check.InflightBails++
		// Plain-check void-recursion (recursion.tsv:53): seed the self-call with
		// a variadic spread (element Never) so the enclosing if-join folds the
		// per-frame leaked lead (`n mul 2` → Integer) into it, yielding a sound
		// 0-or-more residual that covers the runtime's depth-many values. The
		// armed/compiled path keeps its own STAGE A variadic model (Any bail).
		if !r.Check.Recorder().Active() {
			return []core.Value{core.NewVariadicCarrier(core.NewTypeLiteral(core.TNever))}
		}
		return []core.Value{core.NewCarrier(core.TAny)}
	}
	r.Check.FnInflight[key] = true
	defer delete(r.Check.FnInflight, key)

	// Recursive RE-ENTRY by name (a self-call with a different arg shape, which
	// has a different FnInflight key so it did not bail above): suppress the
	// error-level body diagnostics this re-run would emit. The outer, canonical
	// analysis of the same body tokens already reports any real defect; a re-run
	// whose args narrowed a param to a strict Any can spuriously fail dispatch
	// (trie's fuzzy-go recursing on `child = pair get 1` as nd:Map → false
	// kid-items/get/build-row no_signature). Tracked per NAME so only genuine
	// same-fn recursion is suppressed, not nested helper calls.
	if name != "" {
		if r.Check.FnNameInflight == nil {
			r.Check.FnNameInflight = map[string]int{}
		}
		if r.Check.FnNameInflight[name] > 0 {
			r.Check.SuppressBodyErrors++
			defer func() { r.Check.SuppressBodyErrors-- }()
		}
		r.Check.FnNameInflight[name]++
		defer func() { r.Check.FnNameInflight[name]-- }()
	}

	// Diagnostics emitted from here down come from CALL-TIME code —
	// tag them FnBody so an undefined_word that turns out to be a
	// forward reference can be rescued at end of pass
	// (RescueForwardRefDiagnostics).
	r.Check.FnBodyDepth++
	defer func() { r.Check.FnBodyDepth-- }()
	if callShapeSpecialised(args, captures) {
		r.Check.CallShapeDepth++
		defer func() { r.Check.CallShapeDepth-- }()
	}
	// A fn body is a speculative region too (it runs only if called): an
	// `undef` of an enclosing binding inside it must not leak the deletion
	// into the pass model (SpecUndefBlocked — the wrapped-undef FP class;
	// frame teardown pops in-region bindings and stays untouched).
	r.Check.PushSpecBaseline(r.Defs.Snapshot())
	defer r.Check.PopSpecBaseline()

	// Push this fn onto the named-fn stack so its body-local defs and
	// undefined_word diagnostics attribute to it, and record its parameters as
	// binders of their names. Params are frame-lifetime, so a name a callee
	// reads that is one of this fn's params is a sound dynamic-scope reference
	// whenever this fn reaches that callee. Anonymous bodies are transparent
	// (not pushed).
	if name != "" {
		r.Check.FnNameStack = append(r.Check.FnNameStack, name)
		defer func() {
			r.Check.FnNameStack = r.Check.FnNameStack[:len(r.Check.FnNameStack)-1]
		}()
		for _, pn := range paramNames {
			r.Check.RecordFnBinder(pn)
		}
	}

	// runOnce performs one full body analysis: snapshot def-stack
	// depths so any defs the body, captures, or parameter bindings
	// created unwind afterwards. The same snapshot is pushed as the
	// fn-entry baseline so any inner fn/afn construction inside the
	// body sees this scope as its enclosing-fn baseline — without it,
	// ComputeCaptures would treat outer params as if they lived at
	// module/global scope and miss the capture.
	runOnce := func() []core.Value { return RunFnBodyOnce(r, name, paramNames, body, args, captures, anonymous) }

	bailsBefore := r.Check.InflightBails
	diagBase := len(r.Check.Diagnostics)
	result := runOnce()

	// Refinement (A2): the run consumed an Any in-flight bail, so the
	// summary was computed under the weakest hypothesis. Seed the memo
	// with it and re-analyse — the recursive call now reads the seeded
	// hypothesis from the cache — joining each round's result into the
	// hypothesis until stable, up to two extra rounds. Only the final
	// round's diagnostics are kept.
	//
	// Skip refinement when ARMED (this body is being recorded into a compiled fn
	// unit): each extra runOnce re-records the body into the SAME open fragment,
	// leaving multiple residuals the lowerer cannot reconcile. Refinement only
	// sharpens the summary TYPE precision, which a no-contract (`[]`-declared)
	// recursive fn doesn't need — its recursive self-call already reads Any from
	// the in-flight bail, and its return is unconstrained. So a single clean
	// recording is both sound and sufficient. (A non-armed nested analysis is
	// suspended by fnBodyGuard, so active() is true only for the armed compile.)
	armed := r.Check.Recorder().Active()
	// A variadic-spread result already reached its fixpoint in this run (the
	// element came from the else-arm's fixed lead, invariant across rounds), so
	// skip the Kleene refinement — re-running would join variadic-vs-variadic.
	if r.Check.InflightBails > bailsBefore && len(result) > 0 && !armed && !stackHasVariadic(result) {
		result = refineRecursiveSummary(r, key, diagBase, result, runOnce)
	}

	result = stripZeroOutResiduals(r, result)
	r.Check.FnSummaries[key] = result
	return result
}

// isDeferredWordList reports whether v is a parser-evaluated (`Eval`, unquoted)
// plain list that still carries a raw Word element — the unfolded def-node-binding
// deferred residual a transparent fn body like `[[c1]]` returns.
func IsDeferredWordList(v core.Value) bool {
	if !v.Eval || v.Quoted || !v.Parent.Equal(core.TList) || v.Data == nil ||
		core.IsTypedList(v) || core.IsTableType(v) {
		return false
	}
	lst, err := core.AsList(v)
	if err != nil {
		return false
	}
	for _, e := range lst.Slice() {
		if core.IsWord(e) {
			return true
		}
		if e.Parent.Equal(core.TList) && IsDeferredWordList(e) {
			return true
		}
	}
	return false
}

// deferredParamListResidual reports whether a fn body is a single deferred list
// literal that references one of the fn's parameters — the def-node-binding.tsv:54
// shape `[[c1]]` with param `c1`. Such a body returns the list RAW; the param is
// never captured (only `=>` lambdas close over params), so its words auto-evaluate
// in MODULE scope at end-of-run, not the param scope AnalyseFnBody's sub-engine
// used. Returning the raw list lets the residual fold in module scope downstream
// (matching the interpreter) instead of refusing on a param-poisoned carrier.
//
// Deliberately narrow: only a lone, unquoted, parser-evaluated list body that
// names a parameter qualifies. A body that DOESN'T reference a param already
// folds correctly under the normal path; a multi-statement or computed body
// keeps its existing analysis (and its faithful fallback when it can't lower).
func DeferredParamListResidual(body []core.Value, paramNames []string) (core.Value, bool) {
	if len(body) != 1 {
		return core.Value{}, false
	}
	v := body[0]
	if v.Quoted || !v.Eval || !v.Parent.Equal(core.TList) || v.Data == nil ||
		core.IsTypedList(v) || core.IsTableType(v) {
		return core.Value{}, false
	}
	params := make(map[string]bool, len(paramNames))
	for _, p := range paramNames {
		if p != "" {
			params[p] = true
		}
	}
	if len(params) == 0 {
		return core.Value{}, false
	}
	referencesParam := false
	core.WalkBodyWords([]core.Value{v}, func(w core.WordInfo, _ core.Value) {
		if params[w.Name] {
			referencesParam = true
		}
	})
	if !referencesParam {
		return core.Value{}, false
	}
	return v, true
}

// stripZeroOutResiduals drops a body residual's 0-output statement-guard
// phantoms — a trailing `if cond [stmt] []` / `if cond [raise]` registers a
// phantom None carrier but produces 0 RUNTIME values — so a 0-return fn whose
// body is such a guard returns the right count to a multi-value caller (`b i
// i`, where b's call must net 0 so the loop body nets just the trailing i).
// Mirrors the fn-UNIT residual reconciliation (StartFnCompile.finish, which
// skips the same zeroOut events) on the CALL side. Recording-active only: the
// zeroOut flag is set during the compile pass.
func stripZeroOutResiduals(r *core.Registry, stk []core.Value) []core.Value {
	es := r.Check.Recorder()
	if !es.Active() || len(stk) == 0 {
		return stk
	}
	filtered := make([]core.Value, 0, len(stk))
	for _, v := range stk {
		if es.ZeroOutProduced(v.ID) {
			continue
		}
		filtered = append(filtered, v)
	}
	return filtered
}

// carrierStacksEqual reports whether two carrier stacks agree
// position-for-position — the fixed-point stability test.
func carrierStacksEqual(a, b []core.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !core.ValuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// resolveTypeNameArgs is the check-side twin of stepWord's builtin
// type-name fallback (ADR-012 rule 4): the forward plan claims a bare
// type-name token as a raw Word, and at RUNTIME stepWord steps it to
// the canonical literal before it arrives at the handler — so the
// checker's ReturnsFn must see the same literal, or a statically-typed
// dispatch (`convert String x`) degrades to a dynamic result. Only the
// builtin arm applies here: a Defs-bound word was already substituted
// by the claim walk, and /q-captured names arrive as Atoms, never
// Words. Copies lazily — the common all-resolved case allocates nothing.
func resolveTypeNameArgs(args []core.Value) []core.Value {
	var out []core.Value
	for i, a := range args {
		if !core.IsWord(a) {
			continue
		}
		w, err := core.AsWord(a)
		if err != nil {
			continue
		}
		t, ok := core.ResolveBuiltinTypeName(w.Name)
		if !ok {
			continue
		}
		if out == nil {
			out = append([]core.Value{}, args...)
		}
		lit := core.NewTypeLiteral(t)
		lit = core.WithPos(lit, a)
		out[i] = lit
	}
	if out == nil {
		return args
	}
	return out
}

// callShapeSpecialised reports whether this body analysis is a per-CALL-SHAPE
// specialisation rather than a declaration-shaped (generalized) run. A
// generalized run binds every parameter to a CARRIER (ParamBodyCarrier at
// construction time, or the generic-fn carrier args), so nothing in the body
// can constant-fold on an argument; a specialised run binds at least one
// ACTUAL value verbatim (RunFnBodyOnce pushes args unstripped), so folds
// inside the body describe that call and not the code. Captures count for the
// same reason: FnAnalysisKey keys on them, so a closure specialised on a
// concrete capture is re-analysed per capture shape.
func callShapeSpecialised(args []core.Value, captures []core.CapturedBinding) bool {
	for i := range args {
		if core.IsConcrete(args[i]) {
			return true
		}
	}
	for i := range captures {
		if core.IsConcrete(captures[i].Value) {
			return true
		}
	}
	return false
}
