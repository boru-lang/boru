package native

import (
	"fmt"
	"strings"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// storageNatives covers `set` / `get` / `context`. The unified
// dispatch table mixes Node / Object / Array (kernel-territory
// containers) and Store (context-aware, copy-on-write) sigs in one
// place, keeping `set` and `get` polymorphic from the caller's
// perspective.
//
// `set` and `get` carry ReturnsFn closures on the Store sigs that
// thread the static type tracker (r.RecordContextSet /
// r.LookupContextType) so check-mode can recover a typed carrier
// from a previous set on the same key.
//
// Algorithms (GetKey, AsStore, AsArray, CowSet, AsClassInstance,
// AsMutableMap, …) live in eng; this file owns the word names and
// dispatch wiring.
var storageNatives = []NativeFunc{
	{
		Name: "set",

		Signatures: []Signature{
			// Store (copy-on-write)

			{
				Args:      []*Type{TString, TAny, TStore},
				Impl:      Go(setStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},

			// Map (immutable — copy-returning). Unlike the three
			// mutable containers above, a Map is a value: set returns
			// a NEW map with the key bound and leaves the receiver
			// untouched — the same contract as push / StructUtil.setpath.
			{
				Args:      []*Type{TString, TAny, TMap},
				Impl:      Go(setMapHandler),
				Returns:   []*Type{TMap},
				ReturnsFn: setMapTypedReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setMapHandler),
				Returns:   []*Type{TMap},
				ReturnsFn: setMapTypedReturns, BarrierPos: -1,
			},

			// List (immutable — copy-returning, completing the column
			// rule: Map and List both return the updated copy). set
			// range-checks its index at runtime (never grows), so a
			// provably out-of-range index over a known length is flagged
			// at check time (setListIndexReturns → CheckListIndex).
			{
				Args:    []*Type{TInteger, TAny, TList},
				Impl:    Go(setListHandler),
				Returns: []*Type{TList}, ReturnsFn: setListIndexReturns, BarrierPos: -1,
			},

			// Class instance (in-place, SEALED): a declared field
			// writes in place and returns nothing; an undeclared
			// field is a loud sealed_field error — see
			// design/legacy/CLASS-OBJECT.10.ignore §3.3. A statically-decidable
			// violation (unknown field, or a concrete value failing the
			// same MakeClassFieldValue check the write runs) is flagged
			// at check time (setClassInstanceReturns).
			{
				Args:    []*Type{TString, TAny, TClass},
				Impl:    Go(setClassInstanceHandler),
				Returns: []*Type{}, ReturnsFn: setClassInstanceReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TClass},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setClassInstanceHandler),
				Returns:   []*Type{}, ReturnsFn: setClassInstanceReturns, BarrierPos: -1,
			},

			// FlexMap (in-place key set; returns the node for chaining)
			{
				Args:      []*Type{TString, TAny, TFlexMap},
				Impl:      Go(setFlexMapHandler),
				Returns:   []*Type{TFlexMap},
				ReturnsFn: setFlexMapReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TFlexMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setFlexMapHandler),
				Returns:   []*Type{TFlexMap},
				ReturnsFn: setFlexMapReturns, BarrierPos: -1,
			},

			// FlexList (in-place index set; 0..len-1 only — sparse is
			// an error, growth is append's job)
			{
				Args:      []*Type{TInteger, TAny, TFlexList},
				Impl:      Go(setFlexListHandler),
				Returns:   []*Type{TFlexList},
				ReturnsFn: setFlexListReturns, BarrierPos: -1,
			},

			// FlexXml (in-place attribute set; name → value, like the DOM
			// setAttribute. Children grow via `append`.)
			{
				Args:    []*Type{TString, TAny, TFlexXml},
				Impl:    Go(setFlexXmlHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TFlexXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setFlexXmlHandler),
				Returns:   []*Type{TFlexXml}, BarrierPos: -1,
			},

			// WeakFlexMap (in-place key set; scalars store STRONGLY,
			// mutable handles store WEAKLY, immutable Nodes and other
			// value-like data are refused with a weak_value_error —
			// design/FLEX-ATTRS.1.md §4.4. The dedicated sig is forced:
			// the inherited FlexMap handler's AsMutableMap refuses the
			// weak payload, by design.)
			{
				Args:      []*Type{TString, TAny, TWeakFlexMap},
				Impl:      Go(setWeakFlexMapHandler),
				Returns:   []*Type{TWeakFlexMap},
				ReturnsFn: weakSetMapReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TWeakFlexMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setWeakFlexMapHandler),
				Returns:   []*Type{TWeakFlexMap},
				ReturnsFn: weakSetMapReturns, BarrierPos: -1,
			},

			// WeakFlexList (in-place index set over the post-sweep
			// view; same value domain as WeakFlexMap).
			{
				Args:      []*Type{TInteger, TAny, TWeakFlexList},
				Impl:      Go(setWeakFlexListHandler),
				Returns:   []*Type{TWeakFlexList},
				ReturnsFn: weakSetListReturns, BarrierPos: -1,
			},

			// WeakFlexXml (in-place attribute set; attributes are part
			// of the element and always store strongly).
			{
				Args:    []*Type{TString, TAny, TWeakFlexXml},
				Impl:    Go(setWeakFlexXmlHandler),
				Returns: []*Type{TWeakFlexXml}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TWeakFlexXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setWeakFlexXmlHandler),
				Returns:   []*Type{TWeakFlexXml}, BarrierPos: -1,
			},

			// Micron (IMMUTABLE — always errors): the explicit erroring
			// sig pins "set: <Kind> values are immutable" where
			// sig-absence would raise an opaque dispatch failure.
			// isInertConst's MicronPayload arm relies on this staying
			// an error — see eng/go/emit.go.
			{
				Args:      []*Type{TString, TAny, TMicron},
				Impl:      Go(setMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: setMicronReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TMicron},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: setMicronReturns, BarrierPos: -1,
			},
		},
	},
	{
		// del removes a key from a container, completing the storage
		// column rule `set` established: it dispatches over EXACTLY the
		// containers `set` does (NUR022), and each one either removes the
		// slot or says why it will not.
		//
		//   - Map: immutable — returns a NEW map without the key and
		//     leaves the receiver untouched.
		//   - FlexMap / WeakFlexMap: in-place; returns the node for
		//     chaining.
		//   - FlexXml / WeakFlexXml: removes an ATTRIBUTE, the slot set
		//     writes. Children are grown by `append` and removed by the
		//     node words, exactly as on the set side.
		//   - Store: copy-on-write, via a tombstone layer (CowDel).
		//   - Class: a declared field cannot be removed — the shape is
		//     SEALED. Registered refusal.
		//   - Micron: immutable. Registered refusal, mirroring set's.
		//   - List / FlexList / WeakFlexList: index removal SHIFTS the
		//     tail, which is not the inverse of set's in-place replace.
		//     Registered refusal naming the words that do it.
		//
		// The refusals are registered signatures rather than sig-absence
		// for the reason set's Micron form is: an absent sig raises an
		// opaque dispatch failure, a present one raises the specific
		// message, and negative spec rows can pin it.
		//
		// A missing key is a no-op wherever removal is supported
		// (deletion is idempotent — the same totality posture as `has`).
		// Keys are strings or atoms, computed keys via parens:
		// `m del (k)`.
		Name: "del",
		Signatures: []Signature{
			// Store (copy-on-write, mirroring set's Store form: writes
			// nothing to the stack, the context layer is the effect).
			{
				Args:      []*Type{TString, TStore},
				Impl:      Go(delStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: delStoreReturnsFn, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TStore},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: delStoreReturnsFn, BarrierPos: -1,
			},

			// Map (immutable — copy-returning, mirroring set's Map form).
			{
				Args:      []*Type{TString, TMap},
				Impl:      Go(delMapHandler),
				Returns:   []*Type{TMap},
				ReturnsFn: delMapTypedReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delMapHandler),
				Returns:   []*Type{TMap},
				ReturnsFn: delMapTypedReturns, BarrierPos: -1,
			},

			// FlexMap (in-place key delete; returns the node for chaining).
			{
				Args:      []*Type{TString, TFlexMap},
				Impl:      Go(delFlexMapHandler),
				Returns:   []*Type{TFlexMap},
				ReturnsFn: delFlexMapReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TFlexMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delFlexMapHandler),
				Returns:   []*Type{TFlexMap},
				ReturnsFn: delFlexMapReturns, BarrierPos: -1,
			},

			// WeakFlexMap. The dedicated sig is forced for the same
			// reason set's is: the inherited FlexMap handler's
			// AsMutableMap refuses the weak payload by design.
			{
				Args:      []*Type{TString, TWeakFlexMap},
				Impl:      Go(delWeakFlexMapHandler),
				Returns:   []*Type{TWeakFlexMap},
				ReturnsFn: delWeakFlexMapReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TWeakFlexMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delWeakFlexMapHandler),
				Returns:   []*Type{TWeakFlexMap},
				ReturnsFn: delWeakFlexMapReturns, BarrierPos: -1,
			},

			// FlexXml / WeakFlexXml (in-place attribute delete).
			{
				Args:    []*Type{TString, TFlexXml},
				Impl:    Go(delFlexXmlHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TFlexXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delFlexXmlHandler),
				Returns:   []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TString, TWeakFlexXml},
				Impl:    Go(delWeakFlexXmlHandler),
				Returns: []*Type{TWeakFlexXml}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TWeakFlexXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delWeakFlexXmlHandler),
				Returns:   []*Type{TWeakFlexXml}, BarrierPos: -1,
			},

			// Class (SEALED — always errors): a declared field is part
			// of the class shape, so removing one would leave an
			// instance that no longer satisfies its own type.
			{
				Args:      []*Type{TString, TClass},
				Impl:      Go(delClassInstanceHandler),
				Returns:   []*Type{},
				ReturnsFn: delClassInstanceReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TClass},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delClassInstanceHandler),
				Returns:   []*Type{},
				ReturnsFn: delClassInstanceReturns, BarrierPos: -1,
			},

			// Micron (IMMUTABLE — always errors), mirroring set's pair.
			{
				Args:      []*Type{TString, TMicron},
				Impl:      Go(delMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: delMicronReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TMicron},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(delMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: delMicronReturns, BarrierPos: -1,
			},

			// List / FlexList / WeakFlexList (always error): removal at
			// an index shifts the tail, so it is not set's inverse. The
			// sigs exist to name the words that do it.
			{
				Args:      []*Type{TInteger, TList},
				Impl:      Go(delListHandler),
				Returns:   []*Type{},
				ReturnsFn: delListReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TInteger, TFlexList},
				Impl:      Go(delListHandler),
				Returns:   []*Type{},
				ReturnsFn: delListReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TInteger, TWeakFlexList},
				Impl:      Go(delListHandler),
				Returns:   []*Type{},
				ReturnsFn: delListReturns, BarrierPos: -1,
			},
		},
	},
	{
		Name:          "get",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// get EVALUATES its key: the bare-word-quoting atom sigs live on
		// `dot` (the word the `.`/`!.` sugar lowers to), so `lst get i`
		// reads the VALUE of i, not the literal field "i". A literal field
		// name uses `dot` (`m dot a`) or a string key (`m get "a"`).
		Signatures: stripQuoteArgs(accessorGetSignatures()),
	},
	{
		Name:          "dot",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// dot is `get` PLUS the bare-word-quoting atom sigs. The `.` / `!.`
		// dot-sugar lowers to dot/dotr (lowerReach), so `m.a` ≡ `m dot a`
		// reads the literal field "a" regardless of any binding of a.
		Signatures: accessorGetSignatures(),
	},
	{
		Name: "context",

		Signatures: []Signature{{
			Args:      []*Type{},
			Impl:      Go(contextHandler),
			Returns:   []*Type{TStore},
			ReturnsFn: contextReturns, BarrierPos: -1,
		}},
	},
}

// accessorGetSignatures returns the full read-accessor signature set shared
// by `get` and `dot`. `dot` uses it verbatim (so a bare-word key is quoted
// as a literal field, the `.`-sugar behaviour); `get` uses the
// stripQuoteArgs variant (the same overloads with QuoteArgs cleared, so a
// bare WORD is evaluated but an already-evaluated Atom key still matches).
// Keeping ONE
// source for both words prevents the two from drifting apart.
func accessorGetSignatures() []Signature {
	return []Signature{
		// [Key | Node] — covers Map, List, Options, record-shape. The
		// atom / string key sigs narrow a concrete map FIELD read via
		// getNodeReturns; the integer-key sig stays Any because an
		// integer key is a LIST index (handled precisely downstream by
		// tryFoldStaticIndex) or a stringified map key — getNodeReturns
		// must not stringify-and-miss an integer over a list.
		// A module NAMESPACE (a plain Map carrying the module-namespace
		// facet) dispatches these same rows: getNodeHandler answers the
		// $name/$module synthetics from the facet, and getNodeReturns
		// resolves exports to their raw values in check mode.
		{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getNodeReturns},
		{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getNodeReturns},
		{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getIntKeyReturns},
		// [Key | Module] — descriptor fields (id/kind/file/folder/exports)
		{Args: []*Type{TAtom, TModuleInst}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Impl: Go(getModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		// [Key | Class instance] — flat field read (no prototype
		// chain; class instances resolve every field at make). Field
		// type resolved from the schema (getObjectReturns).
		{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getObjectReturns},
		{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getObjectReturns},
		// Resource/Entity instances (SDK object hierarchy) — flat field
		// read. Field type resolved from the Resource schema (getResourceReturns).
		{Args: []*Type{TAtom, TResource}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getResourceReturns},
		{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getResourceReturns},
		// [Key | None] — chained-read propagation
		{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(getNoneHandler), Returns: []*Type{TNone}},
		// [Key | Store] — check-mode-aware ReturnsFn picks up a
		// typed carrier from a previously-set key.
		{
			Args: []*Type{TString, TStore}, BarrierPos: 1, Impl: Go(getStoreHandler),
			ReturnsFn: getStoreReturnsFn,
		},
		{
			Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getStoreHandler),
			ReturnsFn: getStoreReturnsFn,
		},
		// [Key | Micron] — structured-scalar property read: primary
		// fields plus the derived properties (Pathon parts/abs,
		// Emailon address, Urlon href). A miss reads none; the strict
		// twin is in accessorGetrSignatures. Microns sit under Scalar,
		// so these don't overlap the [Key | Node] rows above.
		{Args: []*Type{TAtom, TMicron}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getMicronHandler), ReturnsFn: getMicronReturns},
		{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(getMicronHandler), ReturnsFn: getMicronReturns},
		// [Key | Node/Xml] — well-known field read: 'tag' / 'attr' /
		// 'cren' (any other key → none). More specific than the
		// [Key | Node] sigs above, so it wins for an XML receiver and
		// keeps getNodeHandler off the non-map XML payload. Covers
		// FlexXml too (it conforms to Node/Xml).
		{Args: []*Type{TAtom, TXml}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getXmlHandler), Returns: []*Type{TAny}},
		{Args: []*Type{TString, TXml}, BarrierPos: 1, Impl: Go(getXmlHandler), Returns: []*Type{TAny}},
	}
}

// stripQuoteArgs returns sigs with the bare-word-quoting QuoteArgs flag
// cleared — the `get`/`getr` subset that EVALUATES its key. It does NOT drop
// the atom overloads: an already-evaluated Atom value (a variable bound to
// `a/q`, a computed atom) still matches `{TAtom, …}` and the handler coerces
// it, so `def k a/q  {a:1} get k` reads field "a". Only the implicit
// quote-on-bare-word goes away — an unbound bare word stays a Word, matches
// no overload, and errors, which is the split's intent. `dot`/`dotr` keep the
// QuoteArgs versions (literal bare-word keys). Sigs are copied so the shared
// accessorGetSignatures slice (used by `dot`) keeps its QuoteArgs.
func stripQuoteArgs(sigs []Signature) []Signature {
	out := make([]Signature, len(sigs))
	for i, s := range sigs {
		s.QuoteArgs = nil
		out[i] = s
	}
	return out
}

// ---- kernel-container handlers (Node / Store / Class / None) ----

// d2WriteConforms reports whether writing `v` into a typed container whose
// element constraint is `elem` is permitted — the SAME per-element unify the
// construction check runs (unifyMapValues: unifyInner(childType, val)). The
// element order (elem first) mirrors construction exactly. A carrier/dynamic
// written value can't be statically judged, so it conforms (gradual — the
// runtime handler re-checks the concrete value). See
// design/TYPED-CONTAINER-TAG-RETENTION.0.md.
func d2WriteConforms(elem, v Value) bool {
	if !IsConcrete(v) {
		return true
	}
	_, ok := Unify(elem, v)
	return ok
}

// d2AdoptTyped is the runtime write-adoption for a typed container: it both
// ENFORCES the element tag AND recursively RE-TAGS the stored value, mirroring
// construction (unifyTyped{Map,List}WithConcrete). Returns the value to store
// (unchanged for an untyped container / a scalar element / a non-concrete
// value) or a type_error. The re-tag is the fix for the nested-container hole:
// writing an UNTYPED `{y:2}` into a `{:{:Integer}}` must leave `{y:2}` tagged
// `{:Integer}` so a later write into IT is enforced — construction tags nested
// containers recursively, and writes must too. Scoped to CONTAINER element
// types: a scalar element (`{:Integer}`) needs no tag, so the value is returned
// byte-identical (no stored-representation churn).
func d2AdoptTyped(r *Registry, container, v Value, word string) (Value, error) {
	elem, ok := container.ElemConstraint()
	if !ok {
		return v, nil
	}
	unified, uok := Unify(elem, v)
	if !uok {
		return Value{}, r.BoruError("type_error",
			fmt.Sprintf("%s: value %s does not conform to element type %s", word, v.String(), elem.String()),
			word)
	}
	if IsTypedMap(elem) || IsTypedList(elem) {
		// Nested typed container — store the recursively-tagged value. A FLEX child
		// is by-reference: Unify above validated it but rebuilt a DETACHED copy
		// (unifyTyped*WithConcrete's flex branch allocates a fresh store), so a
		// later mutation through the original child would not be visible through
		// the container. Keep the caller's flex value and retag it IN PLACE
		// instead (#9, Codex round 9) — validation already passed via Unify.
		if IsFlexMap(v) || IsFlexList(v) {
			if ci, cerr := AsChildType(elem); cerr == nil {
				return core.RetagFlexElem(v, ci.Child), nil
			}
		}
		return unified, nil
	}
	return v, nil
}

// setFlexListReturns mirrors setFlexListHandler's [:T] write enforcement in
// check mode (the FlexList set sig otherwise has no ReturnsFn). args are
// [index, value, FlexList].
func setFlexListReturns(args []Value, r *Registry) []Value {
	res := NewCarrier(TFlexList)
	if len(args) == 3 {
		d2CheckWrite(r, args[2], args[1], "set", args[0].Pos())
		res = d2RetainElem(res, args[2])
	}
	return []Value{res}
}

// flexGrowReturns builds the check-mode mirror for a flex GROW word
// (append/push/unshift over a FlexList): args are [value, FlexList], so a
// non-conforming top-level grow into a typed flex list is flagged at check time
// the same way the runtime raises. word rides in the diagnostic.
func flexGrowReturns(word string) func([]Value, *Registry) []Value {
	return func(args []Value, r *Registry) []Value {
		res := NewCarrier(TFlexList)
		if len(args) == 2 {
			d2CheckWrite(r, args[1], args[0], word, args[0].Pos())
			res = d2RetainElem(res, args[1])
		}
		return []Value{res}
	}
}

// d2ReTagContainer enforces + re-tags a whole rebuilt container (a merge result
// that lost its tag through valueToAny/structConvert) against a typed operand's
// element constraint: every entry must conform, and the result carries the tag.
// Returns the result unchanged when neither operand is a typed container. word
// rides in the diagnostic.
func d2ReTagContainer(r *Registry, typedSrc, result Value, word string) (Value, error) {
	elem, ok := typedSrc.ElemConstraint()
	if !ok {
		return result, nil
	}
	var constraint Value
	switch {
	case result.Parent.ConformsTo(TMap):
		constraint = core.NewTypedMap(elem)
	case result.Parent.ConformsTo(TList):
		constraint = core.NewCarrierTypedListValue(elem)
	default:
		return result, nil
	}
	unified, uok := Unify(constraint, result)
	if !uok {
		return Value{}, r.BoruError("type_error",
			fmt.Sprintf("%s: a merged value does not conform to element type %s", word, elem.String()), word)
	}
	return unified, nil
}

// d2typedMergeOperand returns whichever of a/b carries an element tag (a first),
// or a when neither does (d2ReTagContainer then no-ops).
func d2typedMergeOperand(a, b Value) Value {
	if _, ok := a.ElemConstraint(); ok {
		return a
	}
	return b
}

// d2RetainElem copies src's element tag (if any) onto res and returns res — a
// rebuilt map/list copy (the immutable list mutators, and the set/setpath
// check-mode residuals) must carry the {:T}/[:T] tag so downstream reads narrow
// and downstream writes stay enforced (the checker mirrors the runtime).
func d2RetainElem(res, src Value) Value {
	if elem, ok := src.ElemConstraint(); ok {
		res.SetElemConstraint(elem)
	}
	return res
}

// d2TypedListResidual / d2TypedMapResidual build a set-copy check residual as a
// PROPER typed carrier when the receiver is tagged: the element type rides in
// BOTH the carrier's ChildTypeInfo.Child (so a READ from `(xs set i v)` narrows
// to the element bound via getIntKeyReturns instead of degrading to dynamic(Any)
// — Codex round 4) AND the `elem` pointer (so a CHAINED write into the residual
// stays enforced — d2CheckWrite reads ElemConstraint, round-3 #3). An untyped
// receiver keeps the bare carrier.
func d2TypedListResidual(src Value) Value {
	elem, ok := src.ElemConstraint()
	if !ok {
		return NewCarrier(TList)
	}
	v := core.NewCarrierTypedListValue(elem)
	v.SetElemConstraint(elem)
	return v
}

func d2TypedMapResidual(src Value) Value {
	elem, ok := src.ElemConstraint()
	if !ok {
		return NewCarrier(TMap)
	}
	v := core.NewTypedMap(elem)
	v.Carrier = true
	v.SetElemConstraint(elem)
	return v
}

// d2CheckWrite is the check-mode mirror of the write enforcement: at the
// top-level straight line, a provably-non-conforming CONCRETE write into a
// tagged container is flagged as the type_error the runtime raises identically
// (a RuntimeMirror — the program compiles and raises). Inside a fn body the
// runtime still enforces (both compiled and interpreted raise), but the
// checker stays conservative here, matching setClassInstanceReturns.
func d2CheckWrite(r *Registry, recv, v Value, word string, pos SrcPos) {
	if !atUncaughtTopLevel(r) {
		return
	}
	elem, ok := recv.ElemConstraint()
	if !ok || d2WriteConforms(elem, v) {
		return
	}
	core.CheckAddUniqueDiagnostic(r, "type_error",
		fmt.Sprintf("%s: value %s does not conform to element type %s", word, v.String(), elem.String()),
		word, pos)
}

// setMapHandler is the Map form of set. A Map stays immutable: the
// handler returns a NEW map with the key bound (overwriting an existing
// entry), leaving the receiver untouched. This is the language's rule
// of thumb made concrete — mutable containers (Store / Object / Array)
// mutate in place and return nothing; immutable values return the
// updated copy. Keys are strings or atoms, computed keys via parens:
// `m set (k) v`.
func setMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[2], "set")
	if err != nil {
		return nil, err
	}
	key := StoreKey(args[0])
	// The copy-returning column stays ENTIRELY immutable: a value with
	// flex inside is snapshot to its plain shape (core.AdoptIntoNode),
	// so the "immutable" result can never change underneath through a
	// live flex handle. Flex-free values pass through untouched.
	// R2: a write into a typed container ({:T}) must conform to the element tag
	// and store the recursively re-tagged value (nested {:{:T}} stays enforced).
	tagged, werr := d2AdoptTyped(r, args[2], args[1], "set")
	if werr != nil {
		return nil, werr
	}
	val, aerr := core.AdoptIntoNode(tagged)
	if aerr != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil, aerr
	}
	out := NewOrderedMap()
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out.Set(k, v)
	}
	out.Set(key, val)
	res := NewMap(out)
	if elem, ok := args[2].ElemConstraint(); ok {
		res.SetElemConstraint(elem)
	}
	return []Value{res}, nil
}

// delMapHandler is the Map form of del. A Map stays immutable: the
// handler returns a NEW map without the key, leaving the receiver
// untouched — the same copy-returning contract as set's Map form. A
// missing key returns an unchanged copy (no-op, never an error).
func delMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[1], "del")
	if err != nil {
		return nil, err
	}
	key := StoreKey(args[0])
	out := NewOrderedMap()
	for _, k := range m.Keys() {
		if k == key {
			continue
		}
		v, _ := m.Get(k)
		out.Set(k, v)
	}
	res := NewMap(out)
	if elem, ok := args[1].ElemConstraint(); ok {
		res.SetElemConstraint(elem)
	}
	return []Value{res}, nil
}

// delFlexMapHandler is the FlexMap form of del: removes the key in
// place and returns the node for chaining, mirroring set's FlexMap
// contract. A missing key is a no-op.
func delFlexMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[1]
	m, err := AsMutableMap(container)
	if err != nil {
		return nil, r.BoruError("del_error", "del: expected a FlexMap, got "+container.Parent.String(), "del")
	}
	m.Delete(StoreKey(args[0]))
	return []Value{container}, nil
}

// delWeakFlexMapHandler is the WeakFlexMap form of del. It mirrors
// delFlexMapHandler; the separate handler exists because the weak
// payload has its own accessor (AsMutableMap refuses it by design).
// Unlike set's weak form there is no value to classify, so this cannot
// raise weak_value_error.
func delWeakFlexMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[1]
	wd, err := AsWeakFlexMap(container)
	if err != nil {
		return nil, r.BoruError("del_error",
			"del: expected a WeakFlexMap, got "+container.Parent.String(), "del")
	}
	wd.DeleteKey(StoreKey(args[0]))
	return []Value{container}, nil
}

// delWeakFlexMapReturns is the check-mode twin of weakSetMapReturns
// minus both mirrors it runs: del stores nothing, so neither the
// weak-domain refusal nor the typed-write enforcement can fire. The
// receiver passes through, matching the in-place runtime contract.
func delWeakFlexMapReturns(args []Value, _ *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TWeakFlexMap)}
	}
	return []Value{args[1]}
}

// delFlexXmlHandler removes one ATTRIBUTE of a FlexXml element — the
// slot set writes, so the pair is symmetric. Children are grown by
// `append` and are not addressed by name; an absent attribute is a
// no-op. The name is NOT validity-checked the way set's is: set refuses
// an invalid name to keep one from being created, while removing a name
// that could never have been created is already a no-op.
func delFlexXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[1]
	fd, err := AsFlexXml(container)
	if err != nil {
		return nil, r.BoruError("del_error",
			"del: expected a FlexXml, got "+container.Parent.String(), "del")
	}
	if fd.Attr != nil {
		fd.Attr.Delete(StoreKey(args[0]))
	}
	return []Value{container}, nil
}

// delWeakFlexXmlHandler is delFlexXmlHandler over the weak payload.
// Attributes are part of the element and always strong, so weakness
// changes nothing about deletion.
func delWeakFlexXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[1]
	wd, err := AsWeakFlexXml(container)
	if err != nil {
		return nil, r.BoruError("del_error",
			"del: expected a WeakFlexXml, got "+container.Parent.String(), "del")
	}
	wd.DeleteAttr(StoreKey(args[0]))
	return []Value{container}, nil
}

// delStoreHandler is the Store form of del: a copy-on-write tombstone
// layer, mirroring set's CowSet. Like set's Store form it returns
// nothing — the context layer is the effect. Removing a key that was
// never bound is a no-op.
func delStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return nil, reg.BoruError("del_error",
			"del: expected a Store, got "+args[1].Parent.String(), "del")
	}
	CowDel(store, StoreKey(args[0]), reg)
	return nil, nil
}

// delStoreReturnsFn is the check-mode twin of setStoreReturnsFn, and
// the inverse of what it records: a deleted key must STOP narrowing.
// The shape model is join-only monotone, so forgetting is expressed by
// widening the key to dynamic Any — exactly as delFlexMapReturns does
// for the flex column. The flat context tracker is widened too, since
// it is the fallback unshaped readers consult.
func delStoreReturnsFn(args []Value, r *Registry) []Value {
	if r == nil || len(args) < 2 {
		return nil
	}
	key := StoreKey(args[0])
	// The forget marker is DYNAMIC Any — the widening this comment
	// describes — not strict. Strict Any is the one carrier that conforms
	// to NO typed slot, so recording it turns "this key is no longer
	// narrowed" into "every later use of it refuses". getStoreReturnsFn
	// hands the recorded carrier straight back to the consumer, so the
	// marker's modality is the consumer's modality:
	//
	//   context set 'k' 1 end context del 'k' end
	//   context set 'k' 2 end (context get 'k') add 1
	//
	// ran to 3 while check reported no_signature on `add` — a false
	// positive on a working program.
	//
	// The flex column reaches the right answer by a different road: its
	// reader wraps every shape hit through check.ShapeFieldRead, which is
	// always gradual, so a strict marker never escapes there. Widening the
	// marker here rather than de-stricting the store READER keeps every
	// genuinely-recorded carrier strict, so ordinary set/get precision —
	// and the refusals that depend on it — is untouched.
	if !r.Check.Compiling {
		if ss, ok := check.StoreShapeOf(args[1]); ok {
			ss.RecordKey(key, NewDynamicCarrier(TAny))
		}
	}
	r.Check.RecordContextSet(key, NewDynamicCarrier(TAny))
	return nil
}

// delClassInstanceHandler is the SEALED contract stated as a refusal.
// A class declares its fields; an instance missing one would no longer
// satisfy its own type, so there is no coherent removal — the field is
// cleared by writing to it, not by deleting it. Registered rather than
// absent for the same reason set's Micron form is: a specific message
// beats an opaque dispatch failure.
func delClassInstanceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.BoruErrorHint("type_error", delClassDetail(args), "del",
		"a class declares its fields — set the field to a new value instead")
}

func delClassDetail(args []Value) string {
	name := "Class"
	if len(args) == 2 && args[1].Parent != nil {
		name = args[1].Parent.Leaf()
	}
	return fmt.Sprintf("del: %s fields are sealed — a declared field cannot be removed", name)
}

// delClassInstanceReturns is the guaranteed-error mirror of the
// always-erroring handler.
func delClassInstanceReturns(args []Value, r *Registry) []Value {
	if r != nil && r.Check.IsActive() && len(args) == 2 {
		core.CheckAddUniqueDiagnostic(r, "type_error", delClassDetail(args), "del", args[0].Pos())
	}
	return []Value{}
}

// delListHandler refuses index removal on every list flavour. set
// REPLACES at an index and leaves length alone; removing at an index
// shifts the tail, so it is a different operation with different words
// — which the hint names. The sig exists to say that rather than to
// fail opaquely.
func delListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.BoruErrorHint("type_error", delListDetail(args), "del", delListHint)
}

const delListHint = "removing an element shifts the tail, which `set` does not do — " +
	"use pop / shift, or ArrayUtil.remove-at for an arbitrary index"

func delListDetail(args []Value) string {
	name := "List"
	if len(args) == 2 && args[1].Parent != nil {
		name = args[1].Parent.Leaf()
	}
	return fmt.Sprintf("del: cannot remove an element of a %s by index", name)
}

// delListReturns is the guaranteed-error mirror of delListHandler.
func delListReturns(args []Value, r *Registry) []Value {
	if r != nil && r.Check.IsActive() && len(args) == 2 {
		core.CheckAddUniqueDiagnostic(r, "type_error", delListDetail(args), "del", args[0].Pos())
	}
	return []Value{}
}

// delMapTypedReturns is the check-mode mirror of delMapHandler's
// constraint preservation: the updated-copy residual keeps the
// receiver's {:T} tag so chained writes stay enforced and reads keep
// narrowing (the same residual set's Map form uses). del writes no
// value, so there is no d2CheckWrite write-enforcement mirror.
func delMapTypedReturns(args []Value, _ *Registry) []Value {
	res := NewCarrier(TMap)
	if len(args) == 2 {
		res = d2TypedMapResidual(args[1])
	}
	return []Value{res}
}

// delFlexMapReturns is the FlexMap `del` check-mode twin of
// setFlexMapReturns: after a delete the key must STOP narrowing, and
// the shape model is join-only monotone ("widen, never replace"), so
// forgetting is expressed as widening the key's entry to dynamic Any.
// The receiver carrier itself is returned, mirroring the in-place
// runtime contract (`del … f` leaves f, so the shape flows on for
// chaining). An unshaped receiver — and every COMPILE pass — keeps the
// legacy fresh FlexMap carrier, exactly as set's twin does.
func delFlexMapReturns(args []Value, r *Registry) []Value {
	if r != nil && !r.Check.Compiling && len(args) >= 2 {
		if ss, ok := check.StoreShapeOf(args[1]); ok {
			ss.RecordKey(StoreKey(args[0]), NewCarrier(TAny))
			return []Value{args[1]}
		}
	}
	return []Value{NewCarrier(TFlexMap)}
}

// setClassInstanceHandler is the sealed in-place write for class
// instances: a field declared in the class schema (own or inherited)
// writes into the flat field map and returns nothing; an undeclared
// field raises sealed_field loudly — the open-bag use case belongs to
// plain maps / FlexMaps, not class instances (design/legacy/CLASS-OBJECT.10.ignore).
// classSchemaOf resolves the CLASS schema governing a check-mode receiver
// via the type-binding body (TopTypeBody). An unresolvable schema — the
// class was `undef`'d after construction, or the receiver is an
// instantiated generic whose minted node carries no ClassTypeInfo payload
// — reports false and the caller stays silent (the runtime write-time
// check still guards those instances through their retained TypeRef).
func classSchemaOf(r *Registry, recv Value) (ClassTypeInfo, bool) {
	if recv.Parent == nil {
		return ClassTypeInfo{}, false
	}
	if body, ok := r.TopTypeBody(recv.Parent.Leaf()); ok {
		if info, err := AsClassType(body); err == nil {
			return info, true
		}
	}
	return ClassTypeInfo{}, false
}

// setClassInstanceReturns is the check-mode mirror of the sealed-write
// contract below: an unknown field is a guaranteed sealed_field, and a
// CONCRETE value failing MakeClassFieldValue — the byte-identical check the
// runtime write runs — is a guaranteed type_error. Both flag on the
// top-level straight line; a carrier value, unresolvable schema, or
// computed key keeps the declared no-residual model silently.
func setClassInstanceReturns(args []Value, r *Registry) []Value {
	if !atUncaughtTopLevel(r) || len(args) != 3 || !IsConcrete(args[0]) {
		return []Value{}
	}
	info, ok := classSchemaOf(r, args[2])
	if !ok {
		return []Value{}
	}
	key := StoreKey(args[0])
	all := info.AllFields()
	constraint, declared := all.Get(key)
	if !declared {
		name := info.Name
		if name == "" {
			name = args[2].Parent.Name()
		}
		core.CheckAddUniqueDiagnostic(r, "sealed_field",
			fmt.Sprintf("set: %q is not a field of %s (fields: %s)", key, name, strings.Join(all.Keys(), " ")),
			"set", args[0].Pos())
		return []Value{}
	}
	if IsConcrete(args[1]) {
		if _, err := MakeClassFieldValue(args[1], constraint, r); err != nil {
			core.CheckAddUniqueDiagnostic(r, "type_error",
				fmt.Sprintf("set: field %q: %s", key, err.Error()), "set", args[0].Pos())
		}
	}
	return []Value{}
}

func setClassInstanceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	if !IsConcrete(container) {
		return nil, r.BoruError("set_error", "set: cannot set field on type literal", "set")
	}
	oi, ok := container.Data.(ClassInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("set: expected a class instance, got %s", container.Parent.String())
	}
	key := StoreKey(args[0])
	val := args[1]
	if oi.TypeRef != nil {
		all := oi.TypeRef.AllFields()
		constraint, declared := all.Get(key)
		if !declared {
			name := oi.TypeRef.Name
			if name == "" {
				name = container.Parent.Name()
			}
			return nil, r.BoruErrorHint("sealed_field",
				fmt.Sprintf("set: %q is not a field of %s (fields: %s)", key, name, strings.Join(all.Keys(), " ")),
				"set",
				"class instances are sealed — declare the field in the class schema, or use a plain map for open data")
		}
		// Write-time enforcement matches construction: the same strict
		// field check make runs (typed fields conform, predicates run,
		// defaulted fields constrain to the default's own type).
		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, r.BoruError("type_error",
				fmt.Sprintf("set: field %q: %s", key, err.Error()), "set")
		}
		val = checked
	}
	oi.Fields.Set(key, val)
	return nil, nil
}

func setFlexMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	m, err := AsMutableMap(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a FlexMap, got "+container.Parent.String(), "set")
	}
	// A typed flex map ({:T}) enforces its element tag on an IN-PLACE write and
	// stores the recursively re-tagged value — flex only toggles mutability, it
	// never drops the element contract (incl. nested {:{:T}}).
	tagged, werr := d2AdoptTyped(r, container, args[1], "set")
	if werr != nil {
		return nil, werr
	}
	// A flex tree stays ENTIRELY mutable: a plain Node value is deep-
	// flexed on the way in — otherwise a later write into the immutable
	// inner is copy-returning and silently lost. Flex handles share.
	val, aerr := core.AdoptIntoFlex(tagged)
	if aerr != nil {
		return nil, r.BoruError("set_error", aerr.Error(), "set")
	}
	m.Set(StoreKey(args[0]), val)
	return []Value{container}, nil
}

// setWeakFlexMapHandler stores one entry in a WeakFlexMap. The value
// domain is the decided Python-style rule (design/FLEX-ATTRS.1.md
// §4.4): scalars strong, mutable handles weak, everything else refused
// with the rich weak_value_error diagnostic.
func setWeakFlexMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	wd, err := AsWeakFlexMap(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a WeakFlexMap, got "+container.Parent.String(), "set")
	}
	// A typed weak map ({:T}) enforces its element tag on a write,
	// exactly like the flex column — weakness never drops the element
	// contract.
	tagged, werr := d2AdoptTyped(r, container, args[1], "set")
	if werr != nil {
		return nil, werr
	}
	if refusal := wd.SetValue(StoreKey(args[0]), tagged); refusal != nil {
		return nil, WeakRefusalError(r, "set", "WeakFlexMap", refusal)
	}
	return []Value{container}, nil
}

// weakValueMirror is the check-time twin of the runtime weak-domain
// refusal (the guaranteed-error mirror discipline, eng/go/CLAUDE.md):
// a statically-known stored value outside the weak domain — a concrete
// value or a bare type literal — deterministically raises
// weak_value_error at runtime, so the checker reports the same
// diagnostic. Dynamic values stay runtime-only.
func weakValueMirror(r *Registry, v Value, word, container string) {
	if !atUncaughtTopLevel(r) {
		return
	}
	// Statically-known stored values: a concrete value, a bare type
	// literal, or ANY None-shaped check value — None has a single
	// inhabitant, so even a None carrier is provably the none the
	// runtime refuses.
	if !IsConcrete(v) && !core.IsBareTypeNode(v) && !core.IsNoneShape(v) {
		return
	}
	if refusal := core.ClassifyWeakRefusal(v); refusal != nil {
		core.CheckAddUniqueDiagnostic(r, "weak_value_error",
			fmt.Sprintf("%s: cannot store %s in a %s", word, refusal.Kind, container),
			word, v.Pos())
	}
}

// weakSetMapReturns / weakSetListReturns / weakAppendListReturns /
// weakAppendXmlReturns run the weak-domain mirror plus the typed-write
// mirror (d2CheckWrite — a weak {:T}/[:T] enforces on writes exactly
// like the flex column) and pass the receiver through (the runtime
// returns the node for chaining, tag intact).
func weakSetMapReturns(args []Value, r *Registry) []Value {
	if len(args) != 3 {
		return []Value{NewCarrier(TWeakFlexMap)}
	}
	weakValueMirror(r, args[1], "set", "WeakFlexMap")
	d2CheckWrite(r, args[2], args[1], "set", args[0].Pos())
	return []Value{args[2]}
}

func weakSetListReturns(args []Value, r *Registry) []Value {
	if len(args) != 3 {
		return []Value{NewCarrier(TWeakFlexList)}
	}
	weakValueMirror(r, args[1], "set", "WeakFlexList")
	d2CheckWrite(r, args[2], args[1], "set", args[0].Pos())
	return []Value{args[2]}
}

func weakAppendListReturns(args []Value, r *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TWeakFlexList)}
	}
	weakValueMirror(r, args[0], "append", "WeakFlexList")
	d2CheckWrite(r, args[1], args[0], "append", args[0].Pos())
	return []Value{args[1]}
}

// weakAppendXmlReturns has no typed-write mirror: Xml carries no
// element constraint (there is no typed-Xml literal).
func weakAppendXmlReturns(args []Value, r *Registry) []Value {
	if len(args) != 2 {
		return []Value{NewCarrier(TWeakFlexXml)}
	}
	weakValueMirror(r, args[0], "append", "WeakFlexXml")
	return []Value{args[1]}
}

// setWeakFlexListHandler replaces one element of a WeakFlexList. The
// index addresses the post-sweep view; out-of-range is the same error
// as FlexList (sparse weak lists are equally illegal).
func setWeakFlexListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	wd, err := AsWeakFlexList(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a WeakFlexList, got "+container.Parent.String(), "set")
	}
	asInt, ierr := args[0].AsConcreteInteger()
	if ierr != nil {
		return nil, r.BoruError("set_error", "set: WeakFlexList index must be a concrete integer", "set")
	}
	idx := int(asInt)
	if n := wd.Len(); idx < 0 || idx >= n {
		return nil, r.BoruErrorHint("set_error",
			fmt.Sprintf("set: index %d out of bounds for WeakFlexList (length %d)", idx, n),
			"set", "use append to grow; note entries may have been collected — length reflects surviving elements")
	}
	// Typed weak list ([:T]) enforcement, mirroring the flex column.
	tagged, werr := d2AdoptTyped(r, container, args[1], "set")
	if werr != nil {
		return nil, werr
	}
	if refusal := wd.SetIndex(idx, tagged); refusal != nil {
		return nil, WeakRefusalError(r, "set", "WeakFlexList", refusal)
	}
	return []Value{container}, nil
}

// setWeakFlexXmlHandler writes one attribute of a WeakFlexXml element.
// Attributes are part of the element (not weak entries) and always
// store strongly, mirroring FlexXml.
func setWeakFlexXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	wd, err := AsWeakFlexXml(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a WeakFlexXml, got "+container.Parent.String(), "set")
	}
	wd.SetAttr(StoreKey(args[0]), args[1])
	return []Value{container}, nil
}

func setFlexListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	fd, err := AsFlexList(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a FlexList, got "+container.Parent.String(), "set")
	}
	asInt, ierr := args[0].AsConcreteInteger()
	if ierr != nil {
		return nil, r.BoruError("set_error", "set: FlexList index must be a concrete integer", "set")
	}
	idx := int(asInt)
	if idx < 0 || idx >= len(fd.Elems) {
		return nil, r.BoruErrorHint("set_error",
			fmt.Sprintf("set: index %d out of bounds for FlexList (length %d)", idx, len(fd.Elems)),
			"set", "use append to grow a FlexList; sparse FlexLists are an error")
	}
	// A typed flex list ([:T]) enforces + recursively re-tags on an in-place write.
	tagged, werr := d2AdoptTyped(r, container, args[1], "set")
	if werr != nil {
		return nil, werr
	}
	// Entirely-mutable invariant: adopt a plain Node element into flex.
	val, aerr := core.AdoptIntoFlex(tagged)
	if aerr != nil {
		return nil, r.BoruError("set_error", aerr.Error(), "set")
	}
	fd.Elems[idx] = val
	return []Value{container}, nil
}

func setFlexXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	fd, err := AsFlexXml(container)
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a FlexXml, got "+container.Parent.String(), "set")
	}
	name := StoreKey(args[0])
	if !core.IsValidXmlName(name) {
		return nil, r.BoruErrorHint("set_error",
			"set: "+name+" is not a valid XML attribute name", "set",
			"attribute names start with a letter or '_' and contain letters, digits, '-', '.', or ':'")
	}
	// Attributes are String-valued; store a String view of the value so a
	// flex attribute round-trips through `node` and renders correctly.
	val := args[1]
	if !val.Is(TString) {
		val = NewString(ValToString(val))
	}
	fd.Attr.Set(StoreKey(args[0]), val)
	return []Value{container}, nil
}

// getXmlHandler reads a well-known field of a Node/Xml (or FlexXml)
// element: 'tag' → String, 'attr' → Map, 'cren' → List of all children.
// Any other key reads as None (lenient, like a missing map key), so
// `x.tag` / `x.attr` / `x.cren` dotted access works. The element-only
// view and subtree text are the computed words `elem` / `text`.
func getXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	tag, attr, cren, ok := XmlParts(args[1])
	if !ok {
		return nil, r.BoruError("get_error", "get: cannot access field on a non-Xml value", "get")
	}
	switch getKey(args[0]) {
	case "tag":
		return []Value{NewString(tag)}, nil
	case "attr":
		return []Value{NewMap(attr)}, nil
	case "cren":
		return []Value{NewList(cren)}, nil
	default:
		return []Value{NewTypeLiteral(TNone)}, nil
	}
}

// getrXmlHandler is the strict twin of getXmlHandler (NUR021 — the
// accessor family covers one container table): the same three
// well-known fields, but any other key raises not_found instead of
// reading None.
func getrXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	tag, attr, cren, ok := XmlParts(args[1])
	if !ok {
		return nil, r.BoruError("getr_error", "getr: cannot access field on a non-Xml value", "getr")
	}
	switch k := getKey(args[0]); k {
	case "tag":
		return []Value{NewString(tag)}, nil
	case "attr":
		return []Value{NewMap(attr)}, nil
	case "cren":
		return []Value{NewList(cren)}, nil
	default:
		return nil, r.BoruError("not_found", fmt.Sprintf("getr: Xml has no field %q (tag / attr / cren)", k), "getr")
	}
}

// isClosureBearingWrapper reports whether v is a module-method wrapper FnDef
// whose inner native declares a CallableSpec (a code-body higher-order word
// like Rand.list-of). Such a wrapper, surfaced concrete from a field read,
// dispatches through execFnDefLiteral's trivial-delegation -> execMatch ->
// carrierResults -> tryRecordClosure, lowering its NoEvalArgs body to a
// closure unit. The shape is seed-agnostic (see getNodeReturns), so it is
// sound to resolve from an abstract instance carrier. A plain RNG-bound
// wrapper (rand-int) has no CallableSpec and stays dynamic.
func isClosureBearingWrapper(v Value) bool {
	fd, ok := v.Data.(FnDefInfo)
	if !ok || fd.Registry == nil || fd.Name == "" {
		return false
	}
	inner := fd.Registry.Lookup(fd.Name)
	if inner == nil {
		return false
	}
	for i := range inner.Signatures {
		if inner.Signatures[i].Callable != nil {
			return true
		}
	}
	return false
}

// getNodeReturns narrows a field read over a CONCRETE map / record-shape
// to the stored value's type when the key is statically known, instead of
// the poison Any the [Key|Node] sigs otherwise declare — `{a:1 b:2}.a`
// checks as Integer, `{x:'hi'}.x` as ProperString, a statically-absent key
// as None. A computed receiver, a non-literal key, a non-OrderedMap
// container (AsMap nil), or a list falls back to dynamic(Any) — the exact
// shape carrierResults already produced for the declared Any, so those
// paths (e.g. tryFoldStaticIndex's list-element fold) are unchanged.
// Object / Class instances are abstract type carriers in check mode (no
// field payload), so they keep their own Any sigs until schema resolution
// lands. Mirrors getNodeHandler's map branch so check and run agree.
// recordSchemaFieldReturns is getNodeReturns' record-schema field rule,
// shared by the direct RecordTypeInfo-payload branch (a record-typed
// param's schema carrier, a shaped ReturnsFn result) and the minted-node
// branch (a carrier whose Parent record type carries its body). The result
// is GRADUAL, never strict: a field absent at run time (an open map) or a
// value the schema did not pin reads optimistically and is discharged by a
// guard. A field outside the schema, or a dispatch-bearing
// (Function/FnDef) field, keeps dynamic Any.
func recordSchemaFieldReturns(rt RecordTypeInfo, key Value) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	fv, ok := rt.Fields.Get(getKey(key))
	if !ok {
		return dyn
	}
	// A field whose declared value itself bears a record schema (a
	// nested RecordTypeInfo — e.g. the fst/lst match sub-shape of a
	// `mini re` result carrier) propagates the nested schema so a
	// chained read (`r.fst.m`) narrows too. Same gradual modality as
	// the flat field case: dynamic, discharged by a guard at run time.
	if nested, ok := fv.Data.(RecordTypeInfo); ok && nested.Fields != nil {
		return []Value{NewDynamicCarrierValue(NewRecordType(nested.Fields))}
	}
	ft := ValueType(fv)
	if ft == nil || ft.ConformsTo(TFunction) {
		return dyn
	}
	return []Value{NewDynamicCarrier(ft)}
}

// d2TypedContainerBound is the D2 (read-type precision) narrowing: a read over a
// TYPED-container CARRIER ({:T} map / [:T] list) narrows to a DYNAMIC carrier of
// the declared element type instead of dynamic(Any). Returns (_, false) when the
// container carries no narrower-than-Any element type (an untyped Map/List keeps
// dynamic(Any)). See design/legacy/TYPED-CONTAINER-ELEMENT-PRECISION.0.ignore, Part B.
//
// The bound is DYNAMIC (gradual) — a read is only a claim the write-enforcement
// (Part C) backs. What the narrower bound buys: a provably-DISJOINT dispatch (a
// {:Boolean} read reaching an Integer|String word) refuses at compile time, while
// a COVERED read commits or polys exactly as the element type warrants, and an
// UNTYPED read is unchanged.
func d2TypedContainerBound(container Value) (Value, bool) {
	ci, err := AsChildType(container)
	if err != nil {
		return Value{}, false
	}
	child := ci.Child
	if child.Parent == nil || child.Parent.Equal(TAny) {
		return Value{}, false
	}
	// A disjunct element ({:(String tor Integer)}) keeps its DisjunctInfo bound;
	// clone for a fresh identity (the shared child ID would collide in operand-
	// provenance tracking). A single-type element takes a fresh dynamic carrier
	// of child.Parent — the element's lattice SUPERTYPE (Integer→Number). This
	// is imprecise (R4 attempted the exact type via DenotedTypeNode) BUT
	// load-bearing: narrowing to the exact element type diverged the compiled
	// vs interpreted runs (edge-containers-1.tsv:L114 — a `(r get 0) eq
	// (r get 1)` over an each-produced typed list folded differently). The
	// exact narrowing needs the downstream dispatch fixed first; kept at the
	// supertype until then. See design/TYPED-CONTAINER-TAG-RETENTION.0.md (R4).
	if IsDisjunct(child) {
		return NewDynamicCarrierValue(CloneValue(child)), true
	}
	return NewDynamicCarrier(child.Parent), true
}

func getNodeReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 {
		return dyn
	}
	// A module NAMESPACE receiver (facet map) takes its own read model:
	// raw export values (a fn export stays dispatchable) and strict-Any
	// misses (the keyspace can grow) — see moduleNSGetReturns.
	if out, ok := moduleNSGetReturns(args); ok {
		return out
	}
	key, container := args[0], args[1]
	if !IsConcrete(key) {
		return dyn
	}
	// A record-schema carrier (a record-typed param reparented at the call
	// boundary by narrowArgsToParams) carries a RecordTypeInfo schema rather
	// than a concrete map. Recover the field's declared type from the schema so
	// a body's `c get "field"` types instead of degrading to Any. The result is
	// GRADUAL, never strict: a field absent at run time (an open map) or a value
	// the param guard did not pin reads optimistically and is discharged by a
	// guard — never a committed strict op on a possibly-absent field (the
	// Array<T> OOB→None lesson). A field outside the schema, or a dispatch-
	// bearing (Function/FnDef) field, keeps dynamic Any.
	if rt, ok := container.Data.(RecordTypeInfo); ok && rt.Fields != nil {
		return recordSchemaFieldReturns(rt, key)
	}
	// A DYNAMIC DISJUNCT receiver with a SHAPED alternative — stat's
	// dynamic(Record∪None) union — resolves the field through the record
	// alternative's schema, so `(IO.stat p).size` types dynamic(Integer)
	// instead of dynamic(Any). Same gradual modality as the direct
	// schema branch above: the None alternative (a stat miss) and any
	// field absent at run time read optimistically and are discharged by
	// a guard — recordSchemaFieldReturns never claims strictly. A union
	// with no record alternative falls through unchanged.
	if container.Dynamic {
		if di, err := AsDisjunct(container); err == nil {
			for _, alt := range di.Alternatives {
				if rt, ok := alt.Data.(RecordTypeInfo); ok && rt.Fields != nil {
					return recordSchemaFieldReturns(rt, key)
				}
			}
		}
	}
	// A store-shaped FLEX carrier (`flex {…}` and the set-writes threaded
	// through it — design/legacy/checker-precision-fronts.0.ignore §2 stage 1): a key
	// this container saw written reads back its recorded bound, surfaced
	// GRADUAL like the record-schema rule above (a flex tree has runtime
	// writers the shape cannot see, so the claim is a bound a guard
	// discharges, never strict/concrete). A key the shape missed keeps
	// dynamic(Any) — untracked writers exist, so absent-key None would be
	// unsound. The COMPILE pass narrows through the same bound: overload
	// commitment then models the right RESULT ARITY for a downstream
	// dispatch (`set b/q 9 f.a drop …` — a dynamic(Any) receiver committed
	// set's 0-return Store overload and starved the drop, flex.tsv:88/95),
	// while the dispatch itself still records a runtime-re-matching poly
	// whose NOut claim the VM enforces (a hidden-writer bound miss defers
	// to the interpreter instead of shifting the stack).
	if ss, ok := check.StoreShapeOf(container); ok {
		if r != nil {
			if v, hit := ss.LookupKey(getKey(key)); hit {
				return []Value{check.ShapeFieldRead(v)}
			}
		}
		return dyn
	}
	// A CARRIER of a minted record type (`make Test.TestCase {…}`, a module
	// fn declared `[TestCase]`): `type Type = Value`, and the record-refine
	// lattice node carries its RecordTypeInfo body — recover the field
	// schema from the canonical node so instance field reads narrow too.
	if container.Carrier && container.Parent != nil {
		if rt, ok := container.Parent.Data.(RecordTypeInfo); ok && rt.Fields != nil {
			return recordSchemaFieldReturns(rt, key)
		}
	}
	// D2 Part B: a TYPED-map carrier ({:T}) narrows the read to its declared
	// element type. A concrete map falls through to the exact per-key narrowing
	// below; an untyped carrier keeps dynamic(Any).
	if !IsConcrete(container) && IsTypedMap(container) {
		if b, ok := d2TypedContainerBound(container); ok {
			return []Value{b}
		}
	}
	if !IsConcrete(container) || !container.Parent.ConformsTo(TMap) {
		return dyn
	}
	m, err := AsMap(container)
	if err != nil || m == nil {
		return dyn
	}
	val, ok := m.Get(getKey(key))
	if !ok {
		return []Value{NewCarrier(TNone)} // statically-absent key reads as None
	}
	// Only narrow plain DATA fields. A field bearing dispatch — a stored
	// Function / FnDef, a /v ref (Reach), or a word-splice — keeps the
	// dynamic Any the poly / island path already handles: returning its
	// concrete value would push the compiler to lower a fn-value call or a
	// modifier re-dispatch and refuse to compile (fn-value.tsv `m.f 2 3`,
	// path-modifier.tsv `m.a/u`). Return a FRESH carrier of the field's
	// TYPE, not the stored value — the stored value's Value ID is shared
	// with the map field and collides in the emitter's operand-provenance
	// tracking (`def a {b:5} [a.b]`). Deep nesting (`.a.b`) narrows the
	// first hop to a Map carrier and the rest falls to dynamic(Any), which
	// is still strictly better than the all-Any baseline.
	// A closure-bearing module-method wrapper FnDef (`r.list-of`, where `r` is
	// a `Rand.with-seed` instance) folds to the concrete wrapper so its dispatch
	// records the SAME closure-word path the module-export form takes
	// (`Rand.list-of [body] n` -> PUSH_CLOSURE). The wrapper delegates to an
	// inner native carrying a CallableSpec; surfacing it lets execFnDefLiteral's
	// trivial-delegation reach execMatch -> carrierResults -> tryRecordClosure, so
	// the NoEvalArgs body lowers to a closure unit (re-run per iteration) rather
	// than being auto-evaluated and frozen. The wrapper SHAPE (which methods
	// exist, their sigs, NoEvalArgs, CallableSpec) is static for any instance --
	// only the captured RNG state differs, and a closure-driver handler like
	// rand-list-of does NOT itself draw from the RNG (the body does, against the
	// module-export generator it names), so the resolved shape is seed-agnostic.
	// Cloned for a fresh ID (the stored value's ID is shared with the map field
	// and would collide in operand-provenance tracking). A NON-closure wrapper
	// (`r.int`, RNG-bound) stays dynamic so it does not bake a seed-specific
	// handler -- it takes the runtime CALL_DYNAMIC path instead.
	if val.Parent.ConformsTo(TFunction) &&
		isClosureBearingWrapper(val) {
		return []Value{CloneValue(val)}
	}
	if val.Parent.ConformsTo(TFunction) ||
		IsReach(val) || IsSplice(val) {
		// Shaped-instance-method annotation (Stage M2c, eng/method_shape.go):
		// a NON-closure delegation wrapper member (`l.info`, `c.add`, `r.int`)
		// stays dynamic — the runtime instance carries per-call state, so the
		// member must never bake (freeze-gate) — but the fresh carrier is
		// NOTED with the resolved member so the compile pass can model the
		// interpreter's auto-dispatch mid-stream as a guarded OpCallDynMethod.
		// NoteMethodShape vets the member (delegation wrapper only, never a
		// genuine 0-arg overload — the miscompile-E auto-dispatch class stays
		// refused); everything it declines keeps the bare dynamic Any.
		if r != nil && val.Parent.ConformsTo(TFunction) {
			out := NewDynamicCarrier(TAny)
			r.Check.NoteMethodShape(out, val)
			return []Value{out}
		}
		return dyn
	}
	// A concrete LIST / MAP field folds to the stored CONTAINER value, not
	// merely its type carrier: cloned so it carries a FRESH ID (the stored
	// value's ID is shared with the map field and would collide in the
	// emitter's operand-provenance tracking — `def a {b:5} [a.b]`). Folding
	// the concrete container lets a downstream `(m get "k") size` / `for`
	// over the field see a concrete count instead of a carrier Integer, so
	// a statically-empty collection's loop body is correctly pruned rather
	// than analysed over Any (module-test:38). Scalars keep the type
	// carrier — there is nothing to fold, and the ID-collision guard above
	// is the reason we never returned the bare stored scalar.
	if IsConcrete(val) && (val.Parent.ConformsTo(TList) || val.Parent.ConformsTo(TMap)) {
		return []Value{CloneValue(val)}
	}
	return []Value{elementReadCarrier(val)}
}

// elementReadCarrier builds the carrier for a container element / field
// read. The stored value's MODALITY rides along: a gradual bound put
// into a container is still a gradual bound when it comes back out.
//
// Rebuilding it strict invents certainty the value never had, and the
// two ways that goes wrong are both live:
//
//   - dynamic(Any) becomes strict Any — the one carrier that conforms to
//     no typed slot — so a value that merely had an unknown type starts
//     REFUSING every typed use. `def l [(context get 'k')] (l get 0) add 1`
//     ran to 3 while check reported no_signature on `add`.
//   - dynamic(T) becomes strict T, which promotes a wrong-but-gradual
//     bound into a wrong-and-committed one. `typeCovered` waves through
//     any Dynamic carrier, so laundering is also what makes a stale flex
//     shape VISIBLE to the soundness gate rather than merely wrong.
//
// Fixing the modality does not fix a stale bound — a wrong dynamic(T) is
// still wrong. It stops the read from upgrading the claim.
func elementReadCarrier(el Value) Value {
	c := NewCarrier(el.Parent)
	c.Dynamic = el.Dynamic
	return c
}

// getIntKeyReturns narrows an INTEGER-key read over a CONCRETE list to the
// indexed element's type — the plain-check-mode analogue of the emit-only
// tryFoldStaticIndex fold, so `[10 20 30] get 0` checks as Integer instead of
// dynamic(Any). An out-of-range or negative index reads as None (mirroring
// getNodeHandler). A computed index, a non-list receiver (a Map keyed by a
// stringified integer, an abstract object carrier), or a dispatch-bearing
// element keeps dynamic(Any). Restricting to LISTS is deliberate: an integer
// key over a list is an INDEX, never a stringified map key (the bug that
// mistyped a parselang `get 1` over a list as None).
func getIntKeyReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	// D2 Part B: an index read over a TYPED-list carrier ([:T]) narrows to its
	// declared element type. A concrete list falls through to the exact per-index
	// narrowing below; an untyped carrier keeps dynamic(Any).
	if len(args) == 2 && IsConcrete(args[0]) && !IsConcrete(args[1]) && IsTypedList(args[1]) {
		if b, ok := d2TypedContainerBound(args[1]); ok {
			return []Value{b}
		}
	}
	if len(args) != 2 || !IsConcrete(args[0]) || !IsConcrete(args[1]) ||
		!args[1].Parent.ConformsTo(TList) {
		return dyn
	}
	idx, err := AsInteger(args[0])
	if err != nil {
		return dyn
	}
	list, lerr := AsList(args[1])
	if lerr != nil || list.IsNil() {
		return dyn
	}
	i := int(idx)
	if i < 0 || i >= list.Len() {
		return []Value{NewCarrier(TNone)} // out-of-range index reads as None
	}
	el := list.Get(i)
	if el.Parent.ConformsTo(TFunction) ||
		IsReach(el) || IsSplice(el) {
		return dyn
	}
	// A QUOTED CODE element (a dispatch-table entry — `def ops [quote [1
	// add 2] …] (ops get 0)`) carries its analysed stack effect on the
	// carrier, so a downstream `do` types its result instead of the
	// dynamic(Any) hatch (design/legacy/checker-precision-fronts.0.ignore §1 stage 1).
	// The helper self-gates to plain (non-Compiling) check mode and
	// declines anything it cannot analyse cleanly, so this only ever
	// NARROWS the plain-carrier fallback below.
	if c, ok := AnalyseCodeEffect(r, el); ok {
		return []Value{c}
	}
	return []Value{elementReadCarrier(el)}
}

func getNodeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.BoruError("get_error", "get: cannot access property on type literal", "get")
	}
	// Integer key: list index access.
	if key.Parent.ConformsTo(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			i := int(idx)
			if i < 0 || i >= list.Len() {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{list.Get(i)}, nil
		}
		// Fall through to map lookup with stringified key.
	}
	// String/atom/word key: map property access.
	k := getKey(key)
	// A module namespace answers $name/$module from its facet; every
	// other key falls through to the plain map read below.
	if val, ok := moduleNSGetSynthetic(container, k); ok {
		return []Value{val}, nil
	}
	if m, _ := AsMap(container); m != nil {
		val, ok := m.Get(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

// getObjectReturns narrows a field read over an OBJECT / CLASS instance
// carrier to the field's DECLARED type, resolved from the type SCHEMA. In
// check mode the instance is an abstract type carrier (no field payload),
// so the field type comes from the bound type's ClassTypeInfo.AllFields()
// (own + inherited). A method field (function-typed), an absent/sealed
// field (→ None), or a type whose schema can't be resolved keeps the
// dynamic(Any) the poly path handles — the same dispatch-bearing exclusion
// as the concrete-map case (a returned fn value would push the compiler to
// lower a fn-value call and refuse).
func getObjectReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if r == nil || len(args) != 2 || !IsConcrete(args[0]) || args[1].Parent == nil {
		return dyn
	}
	body, ok := r.TopTypeBody(args[1].Parent.Leaf())
	if !ok {
		return dyn
	}
	info, oerr := AsClassType(body)
	if oerr != nil {
		return dyn
	}
	fv, ok := info.AllFields().Get(getKey(args[0]))
	if !ok {
		return []Value{NewCarrier(TNone)} // sealed / absent field reads as None
	}
	ft := ValueType(fv)
	if ft == nil || ft.ConformsTo(TFunction) {
		return dyn
	}
	return []Value{NewCarrier(ft)}
}

// getResourceReturns is getObjectReturns for the Resource/Entity
// hierarchy: it reads the field type from the ResourceType schema so a
// Resource-instance field read (`e.spec`) carries its declared type in
// check/bytecode contexts instead of degrading to Any. Resource/Entity
// are installed as type bindings (installResourceTypes), so their
// schema is reachable via TopTypeBody like a class type's.
func getResourceReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if r == nil || len(args) != 2 || !IsConcrete(args[0]) || args[1].Parent == nil {
		return dyn
	}
	// Resource/Entity are def bindings, not type bindings, so resolve
	// the schema through the def store by the instance's type name.
	info, ok := lookupResourceTypeByName(r, args[1].Parent.Leaf())
	if !ok {
		return dyn
	}
	fv, ok := info.AllFields().Get(getKey(args[0]))
	if !ok {
		return []Value{NewCarrier(TNone)} // absent field reads as None
	}
	ft := ValueType(fv)
	if ft == nil || ft.ConformsTo(TFunction) {
		return dyn
	}
	return []Value{NewCarrier(ft)}
}

func getObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.BoruError("get_error", "get: cannot access property on type literal", "get")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		val, ok := ri.GetField(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	oi, _ := AsClassInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- set Store handler ----

func setStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store, err := AsStore(args[2])
	if err != nil {
		return nil, fmt.Errorf("set: expected a Store, got %s", args[2].Parent.String())
	}
	key := StoreKey(args[0])
	CowSet(store, key, args[1], reg)
	return nil, nil
}

func setStoreReturnsFn(args []Value, r *Registry) []Value {
	// Store-identity typing (design/legacy/checker-precision-fronts.0.ignore §2
	// stage 1): a SHAPED store carrier records the write in ITS OWN
	// KeyTypes, so two stores' same-named keys no longer join. The flat
	// map is ALSO written — it remains the compatibility fallback for
	// unshaped readers, so precision only increases. Plain check mode
	// only: the compiled stream must stay byte-identical (store rows
	// compile natively through the flat-map typing today).
	if r != nil && !r.Check.Compiling && len(args) >= 3 {
		if ss, ok := check.StoreShapeOf(args[2]); ok {
			ss.RecordKey(StoreKey(args[0]), args[1])
		}
	}
	if len(args) >= 2 {
		r.Check.RecordContextSet(StoreKey(args[0]), args[1])
	}
	return nil
}

// ---- get Store handler ----

func getStoreHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return nil, fmt.Errorf("get: expected a Store, got %s", args[1].Parent.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		// `key_error` — the historical code the getr twin's comment names. It
		// was spelled "unknown key_error" (the message's leading word glued to
		// the code) with "unknown key" passed as the WORD, and both halves of
		// that typo were user-visible: a code containing a SPACE cannot be
		// matched by any `case` arm, and the compiled path renders the caret
		// span at len(Word) — 11 carets for "unknown key" against the
		// interpreter's 3 for the real token.
		return nil, r.BoruError("key_error", fmt.Sprintf("unknown key: %s", key), "get")
	}
	return []Value{val}, nil
}

// getrStoreHandler is the strict twin of getStoreHandler (NUR021): the
// same lookup, with the miss raised as the accessor family's not_found
// (getStoreHandler predates the family codes and keeps its historical
// key_error).
func getrStoreHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return nil, fmt.Errorf("getr: expected a Store, got %s", args[1].Parent.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		return nil, r.BoruError("not_found", fmt.Sprintf("getr: key %q not found in store", key), "getr")
	}
	return []Value{val}, nil
}

func getStoreReturnsFn(args []Value, r *Registry) []Value {
	// Store-identity typing: a SHAPED store answers a key it saw
	// written through ITSELF (no cross-store join). A key the shape
	// missed falls back to the flat map — today's optimism, unchanged
	// (the prototype chain / another scope's layer may have written
	// it). Plain check mode only (compile parity).
	if len(args) < 2 {
		return []Value{NewDynamicCarrier(TAny)}
	}
	if r != nil && !r.Check.Compiling {
		if ss, ok := check.StoreShapeOf(args[1]); ok {
			if v, hit := ss.LookupKey(StoreKey(args[0])); hit {
				return []Value{v}
			}
		}
	}
	v, ok := r.Check.LookupContextType(StoreKey(args[0]))
	if !ok {
		// Escape hatch: the checker has no proven type for this key.
		// Emit a bounded gradual carrier dynamic(Any) — optimistically
		// compatible with any slot — rather than strict Carry<Any>, which
		// would fail every typed slot downstream and force a no_signature
		// or Any catch-all. (design/legacy/dynamic-modality-report.10.ignore, escape
		// hatch 1.) A key recorded by a prior `set` keeps its real, strict
		// carrier.
		return []Value{NewDynamicCarrier(TAny)}
	}
	return []Value{v}
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.Contexts.Top()
	if store == nil {
		return nil, reg.BoruError("context_error", "context: no active context", "context")
	}
	return []Value{NewStoreValue(TStore, store)}, nil
}

// contextReturns is `context`'s check-mode twin: it returns the ABSTRACT
// shape carrier for the live top context layer (minted once per layer —
// CheckState.ContextShape), never the real store. The real
// *StoreInstanceInfo must not flow through check mode (toCarrier strips
// it deliberately; the check pass is observation-free), and per-call-
// site minting would be wrong — every `context` call in one scope
// returns the SAME runtime store, so they must share ONE shape or a
// set-then-get across two `context` calls would miss. During a COMPILE
// pass the legacy bare Store carrier stands (byte-identical lowering —
// context/set/get rows compile natively through the flat map today).
func contextReturns(_ []Value, r *Registry) []Value {
	if r == nil || r.Check.Compiling {
		return []Value{NewCarrier(TStore)}
	}
	top := r.Contexts.Top()
	if top == nil {
		return []Value{NewCarrier(TStore)}
	}
	return []Value{r.Check.ContextShape(top, r.Contexts.Depth())}
}

// setFlexMapReturns is the FlexMap `set` check-mode twin: it records the
// write in the receiver's StoreShapeInfo (the value adopted the way the
// runtime AdoptIntoFlex would — a concrete map becomes a nested FlexMap
// shape) and returns the RECEIVER carrier itself, mirroring the in-place
// runtime contract (`set … f` leaves f, so the same shape flows on for
// chaining). An unshaped receiver — and every COMPILE pass — keeps the
// legacy fresh FlexMap carrier, so nothing changes where the shape
// machinery is not in play.
func setFlexMapReturns(args []Value, r *Registry) []Value {
	if len(args) == 3 {
		d2CheckWrite(r, args[2], args[1], "set", args[0].Pos()) // flex {:T} write mirror
	}
	// len guard: the no-signature recovery can assume this sig with a
	// short arg window (defensive — panic prevention).
	if r != nil && !r.Check.Compiling && len(args) >= 3 {
		if ss, ok := check.StoreShapeOf(args[2]); ok {
			ss.RecordKey(StoreKey(args[0]), check.AdoptShapeValue(args[1], 1))
			return []Value{args[2]}
		}
	}
	res := NewCarrier(TFlexMap)
	if len(args) == 3 {
		res = d2RetainElem(res, args[2]) // chained/bound flex writes stay checked
	}
	return []Value{res}
}

// setListHandler is the List form of set: copy-returning, like Map —
// a NEW list with the element at the index replaced; the receiver is
// untouched. Out-of-range indices are a loud error (edits, not
// lookups). Completes the immutable column of the container 2x2.
// setListIndexReturns is the check-mode mirror of setListHandler's range
// check: set never grows a list, so a provably out-of-range index over a
// statically-known length is a guaranteed runtime index_out_of_range —
// CheckListIndex flags it. The result model is the declared updated-copy
// List either way (soundness: an unknown length or index stays silent).
func setListIndexReturns(args []Value, r *Registry) []Value {
	res := NewCarrier(TList)
	if len(args) == 3 {
		CheckListIndex(r, args[0], args[2], "set")
		d2CheckWrite(r, args[2], args[1], "set", args[0].Pos()) // R2: [:T] write enforcement
		res = d2TypedListResidual(args[2])
	}
	return []Value{res}
}

// setMapTypedReturns is the check-mode mirror of setMapHandler's typed-container
// write enforcement (R2): a non-conforming concrete write into a {:T} map is the
// type_error the runtime raises. The residual is the declared updated-copy Map
// (unchanged) — the receiver's tag drives the check, not the residual.
func setMapTypedReturns(args []Value, r *Registry) []Value {
	res := NewCarrier(TMap)
	if len(args) == 3 {
		d2CheckWrite(r, args[2], args[1], "set", args[0].Pos())
		res = d2TypedMapResidual(args[2]) // typed carrier: chained writes + read narrowing
	}
	return []Value{res}
}

func setListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_idx, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.BoruError("set_error", "set: expected a concrete Integer index", "set")
	}
	lst, err2 := RequireConcreteList(args[2], "set")
	if err2 != nil {
		return nil, err2
	}
	idx := int(_idx)
	n := lst.Len()
	if idx < 0 || idx >= n {
		return nil, r.BoruError("index_out_of_range",
			fmt.Sprintf("set: index %d out of range for list of length %d", idx, n), "set")
	}
	// R2: a typed list ([:T]) enforces + recursively re-tags on write (see
	// setMapHandler) and keeps the tag on the returned copy.
	tagged, werr := d2AdoptTyped(r, args[2], args[1], "set")
	if werr != nil {
		return nil, werr
	}
	// Entirely-immutable invariant for the copy-returning column: see
	// setMapHandler.
	val, aerr := core.AdoptIntoNode(tagged)
	if aerr != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil, aerr
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		out[i] = lst.Get(i)
	}
	out[idx] = val
	res := NewList(out)
	if elem, ok := args[2].ElemConstraint(); ok {
		res.SetElemConstraint(elem)
	}
	return []Value{res}, nil
}
