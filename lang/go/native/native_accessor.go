package native

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// accessorNatives covers strict-access words.
//
// `getr` is the strict variant of `get`: same arg order
// ([Key, Container]) but it returns an error when the parent is None
// or the key/index is missing, instead of silently returning None.
//
// Usage:
//
//	{a:1} getr a       → 1
//	getr a {a:1}       → 1
//	{a:1} b getr       → ERROR (key not found)
//	none a getr        → ERROR (parent is none)
//	[10,20] 5 getr     → ERROR (index out of bounds)
var accessorNatives = []NativeFunc{
	{
		Name:          "getr",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// getr EVALUATES its key (the strict sibling of `get`); the
		// bare-word-quoting atom sigs live on `dotr`, which the `!.` sugar
		// lowers to. `m getr "k"` / `m getr <int>` work; a literal field
		// name uses `dotr` (`m dotr a`).
		Signatures: stripQuoteArgs(accessorGetrSignatures()),
	},
	{
		Name:          "dotr",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// dotr is `getr` PLUS the bare-word-quoting atom sigs — the strict
		// (`!.`) dot-sugar target. `m!.a` ≡ `m dotr a` reads the literal
		// field "a" and raises if absent.
		Signatures: accessorGetrSignatures(),
	},
	{
		// `has` is the Boolean presence predicate — the missing third
		// sibling of `get` (None on miss) and `getr` (raise on miss):
		// "is this key/index BOUND, regardless of value", so a
		// present-but-None entry is distinguishable from an absent one
		// (decision DX report finding 3). TOTAL within its container
		// table: a missing key, an out-of-range index, a None parent,
		// or a type-literal container all answer false — it never
		// raises, so it composes inside if/filter/conditions.
		//
		//	{a:None} has 'a'     → true   (present, value is None)
		//	{a:1}    has 'b'     → false  (absent)
		//	[10,20]  has 1       → true
		//	none     has a/q     → false
		Name: "has",
		// CompileModuleFold: a pure presence reader. `has` EVALUATES its key
		// exactly as `get` does (2026-08-21, get-parity ruling): a bare
		// bound word supplies its value, an unbound bare word is an
		// undefined_word error, and a literal field name is spelled `'k'`
		// or `k/q`. The TAtom overloads still match an EVALUATED Atom key.
		CompileEffect: CompileModuleFold,

		Signatures: []Signature{
			// [Key | Node] — Map, List, Options, record-shape
			{Args: []*Type{TAtom, TNode}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			// [Key | Class instance]
			{Args: []*Type{TAtom, TClass}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TAtom, TResource}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			// [Key | Store]
			{Args: []*Type{TAtom, TStore}, BarrierPos: 1, Impl: Go(hasStoreHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TStore}, BarrierPos: 1, Impl: Go(hasStoreHandler), Returns: []*Type{TBoolean}},
			// [Key | Micron] — property presence, including the
			// derived properties (address/href/parts/abs) and the
			// optional Urlon fields (an absent port answers false).
			{Args: []*Type{TAtom, TMicron}, BarrierPos: 1, Impl: Go(hasMicronHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(hasMicronHandler), Returns: []*Type{TBoolean}},
			// [Key | Node/Xml] — the three well-known fields (tag / attr
			// / cren) are BOUND on every Xml element, so `has` agrees
			// with what dot/get can read (NUR021: it used to answer
			// false for a field `dot` successfully returned); any other
			// key answers false. Wins over [Key | Node] by specificity.
			{Args: []*Type{TAtom, TXml}, BarrierPos: 1, Impl: Go(hasXmlHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TXml}, BarrierPos: 1, Impl: Go(hasXmlHandler), Returns: []*Type{TBoolean}},
			// [Key | Module] — presence over the same descriptor lookup
			// get/getr read (NUR021). A module NAMESPACE is a plain Map
			// (facet-carrying), so it takes the [Key | Node] rows above;
			// hasNodeHandler answers the $name/$module synthetics there.
			{Args: []*Type{TAtom, TModuleInst}, BarrierPos: 1, Impl: Go(hasModuleInstHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Impl: Go(hasModuleInstHandler), Returns: []*Type{TBoolean}},
			// [Key | None] — total: an absent parent answers false.
			// The Atom overload takes an EVALUATED atom key (`none has
			// a/q`, `(m get sub) has k/q`) — aligned with get/getr
			// since the 2026-08-21 get-parity ruling.
			{Args: []*Type{TAtom, TNone}, BarrierPos: 1, Impl: Go(hasNoneHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(hasNoneHandler), Returns: []*Type{TBoolean}},
		},
	},
}

// accessorGetrSignatures returns the full strict-read signature set shared by
// `getr` and `dotr`. `dotr` uses it verbatim (bare-word key quoted as a
// literal field — the `!.` sugar); `getr` uses the stripQuoteArgs variant
// (same overloads, QuoteArgs cleared: an evaluated Atom key still matches; a
// bare WORD is evaluated). One source keeps the two words from drifting.
func accessorGetrSignatures() []Signature {
	return []Signature{
		// [Key | Node] — key forward, container from stack
		// Field type narrows exactly as get's twin does (getNodeReturns);
		// the strict-read difference is the miss behaviour (raise vs none),
		// so a statically-provable miss over a concrete container is
		// flagged at check time (getrNodeReturns).
		{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: getrNodeReturns},
		{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: getrNodeReturns},
		{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: returnsGetrIndexChecked},
		// [Key | Class instance] — strict field read (mirrors get's TClass
		// sigs; getrObjectHandler resolves the flat instance via
		// AsClassInstance and raises on a missing field). Field type
		// narrows from the schema via getObjectReturns, as get does; a
		// concrete key provably outside the CLOSED class schema is a
		// guaranteed runtime not_found, flagged by the getr wrapper.
		{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrObjectHandler), ReturnsFn: getrObjectReturns},
		{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(getrObjectHandler), ReturnsFn: getrObjectReturns},
		{Args: []*Type{TAtom, TResource}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrObjectHandler), Returns: []*Type{TAny}},
		{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(getrObjectHandler), Returns: []*Type{TAny}},
		// [Key | Module] — descriptor fields. A module NAMESPACE is a
		// plain Map (facet-carrying) and takes the [Key | Node] rows
		// above; getrMapHandler answers its synthetics and raises the
		// module-flavored miss there.
		{Args: []*Type{TAtom, TModuleInst}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Impl: Go(getrModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		// [Key | Micron] — strict structured-scalar property read
		// (mirrors get's TMicron sigs; a miss raises not_found instead
		// of reading none).
		{Args: []*Type{TAtom, TMicron}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrMicronHandler), ReturnsFn: getrMicronReturns},
		{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(getrMicronHandler), ReturnsFn: getrMicronReturns},
		// [Key | Store] — strict store read (NUR021: mirrors get's TStore
		// sigs; the miss raises the family's not_found).
		{Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrStoreHandler), ReturnsFn: getStoreReturnsFn},
		{Args: []*Type{TString, TStore}, BarrierPos: 1, Impl: Go(getrStoreHandler), ReturnsFn: getStoreReturnsFn},
		// [Key | Node/Xml] — strict well-known-field read (NUR021: mirrors
		// get's TXml sigs; wins over the [Key | Node] rows by specificity
		// exactly as get's do, so `x!.tag` reads and an unknown field
		// raises not_found, mirrored statically by getrXmlReturns).
		// Covers FlexXml via conformance.
		{Args: []*Type{TAtom, TXml}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrXmlHandler), ReturnsFn: getrXmlReturns},
		{Args: []*Type{TString, TXml}, BarrierPos: 1, Impl: Go(getrXmlHandler), ReturnsFn: getrXmlReturns},
		// [Key | None]
		// Always raises not_found — never produces a value; a receiver the
		// checker KNOWS is None (a literal, a strict None carrier from a
		// statically-absent lenient read) flags the same error statically.
		// The atom row quotes a bare-word key as every other receiver's
		// does (NUR198's strict twin: `m.b!.c` raises the family's
		// not_found, not `undefined word: c`).
		{Args: []*Type{TAtom, TNone}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrNoneHandler), ReturnsFn: getrNoneReturns},
		{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(getrNoneHandler), ReturnsFn: getrNoneReturns},
	}
}

// ---- has handlers (the get lookups, returning presence as Boolean) ----

func hasNodeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return []Value{NewBoolean(false)}, nil
	}
	if key.Parent.ConformsTo(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			i := int(idx)
			return []Value{NewBoolean(i >= 0 && i < list.Len())}, nil
		}
		// Fall through to map lookup with stringified key (get parity).
	}
	k := getKey(key)
	// A module namespace additionally answers its facet synthetics —
	// presence agrees with what get/getr can read (NUR021).
	if _, ok := moduleNSGetSynthetic(container, k); ok {
		return []Value{NewBoolean(true)}, nil
	}
	if m, _ := AsMap(container); m != nil {
		_, ok := m.Get(k)
		return []Value{NewBoolean(ok)}, nil
	}
	return []Value{NewBoolean(false)}, nil
}

func hasObjectHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	container := args[1]
	if !IsConcrete(container) {
		return []Value{NewBoolean(false)}, nil
	}
	k := getKey(args[0])
	if m, err := AsMutableMap(container); err == nil {
		_, found := m.Get(k)
		return []Value{NewBoolean(found)}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		_, ok := ri.GetField(k)
		return []Value{NewBoolean(ok)}, nil
	}
	oi, err := AsClassInstance(container)
	if err != nil {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := oi.GetField(k)
	return []Value{NewBoolean(ok)}, nil
}

func hasStoreHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := store.Get(getKey(args[0]))
	return []Value{NewBoolean(ok)}, nil
}

func hasNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(false)}, nil
}

// hasXmlHandler answers presence for the Xml well-known fields: tag /
// attr / cren are bound on every Xml element (they are exactly what
// dot/get/getr can read), anything else is false. Total — a type
// literal or non-Xml payload answers false.
func hasXmlHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if _, _, _, ok := XmlParts(args[1]); !ok {
		return []Value{NewBoolean(false)}, nil
	}
	switch getKey(args[0]) {
	case "tag", "attr", "cren":
		return []Value{NewBoolean(true)}, nil
	}
	return []Value{NewBoolean(false)}, nil
}

// hasModuleInstHandler answers presence over the SAME lookup the
// get/getr handlers read (moduleGet), so the three siblings agree on
// what is bound. Total — a type literal answers false where the getters
// raise. (Module NAMESPACE presence — export fields + synthetics — rides
// the plain-map hasNodeHandler above.)
func hasModuleInstHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[1]) {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := moduleGet(args[1], getKey(args[0]))
	return []Value{NewBoolean(ok)}, nil
}

// returnsGetrIndexChecked is the check-mode ReturnsFn for the
// integer-index `getr` sig. It runs the static bounds check (a no-op
// for map/object containers and unknown-length carriers) and returns
// the container's element-type carrier. `getr` errors at runtime on an
// out-of-bounds list index, so a provably out-of-range literal —
// `[10 20] 5 getr` — is flagged at `boru check`.
func returnsGetrIndexChecked(args []Value, r *Registry) []Value {
	if len(args) >= 2 {
		CheckListIndex(r, args[0], args[1], "getr")
		return ReturnsListElemAt(1)(args, r)
	}
	return []Value{NewCarrier(TAny)}
}

// getrXmlReturns is the Xml twin of getrNodeReturns' strict-read
// static-miss contract: the Xml well-known field set is FIXED (tag /
// attr / cren — getrXmlHandler switches on nothing else), so a concrete
// literal key outside it is a GUARANTEED runtime not_found, flagged
// with the handler's byte-identical text. Unlike the map twin, the
// proof needs no concrete RECEIVER: check mode strips an Xml literal to
// a carrier, and the carrier's Xml-family static type suffices — the
// field set is fixed for the whole family, and the sig is a locked
// native, so no user overload can claim a subtype-tagged runtime value
// first (locked-first ordering; contrast the CoreDefault refinement
// escape booleanArithReturns must guard against). Known keys narrow to
// the handler's fixed result shapes (tag → String, attr → Map, cren →
// List); a computed key keeps the gradual dynamic(Any) the
// declared-Any sig produced before this mirror.
func getrXmlReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 || !IsConcrete(args[0]) {
		return dyn
	}
	switch k := getKey(args[0]); k {
	case "tag":
		return []Value{NewCarrier(TString)}
	case "attr":
		return []Value{NewCarrier(TMap)}
	case "cren":
		return []Value{NewCarrier(TList)}
	default:
		// A bare `Xml` type literal in the value slot would signature-
		// error at runtime, not not_found — exclude it from the claim.
		if r != nil && r.Check.IsActive() && !IsBareTypeNode(args[1]) {
			core.CheckAddUniqueDiagnostic(r, "not_found",
				fmt.Sprintf("getr: Xml has no field %q (tag / attr / cren)", k), "getr", args[0].Pos())
		}
		return dyn
	}
}

// getrNodeReturns is getNodeReturns plus the strict-read static-miss
// contract (the map/list twin of getrMicronReturns): getr means REQUIRED,
// so a miss the checker can prove — a concrete key absent from a concrete
// map, or a concrete list read with a non-integer key — is a guaranteed
// runtime error, flagged with the byte-identical getrMapHandler text. The
// miss is re-proved against the container directly (never inferred from
// the None carrier getNodeReturns returns) because a PRESENT key whose
// stored value is `none` produces the same carrier shape and succeeds at
// runtime. Everything non-concrete keeps getNodeReturns' gradual result.
func getrNodeReturns(args []Value, r *Registry) []Value {
	// A module NAMESPACE receiver (facet map) takes its own strict-read
	// model — raw export resolution, and a not_found TRAP for a provably
	// missing export — instead of the mutable-map miss diagnostic below.
	if out, ok := moduleNSGetrReturns(args, r); ok {
		return out
	}
	out := getNodeReturns(args, r)
	if r == nil || !r.Check.IsActive() || len(args) != 2 ||
		!IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return out
	}
	key, container := args[0], args[1]
	if container.Parent.ConformsTo(TList) && !key.Parent.ConformsTo(TInteger) {
		core.CheckAddUniqueDiagnostic(r, "getr_error",
			fmt.Sprintf("getr: expected a map, got %s", container.Parent.String()), "getr", key.Pos())
		return out
	}
	if m, err := AsMap(container); err == nil && m != nil && container.Parent.ConformsTo(TMap) {
		if _, ok := m.Get(getKey(key)); !ok {
			core.CheckAddUniqueDiagnostic(r, "not_found",
				fmt.Sprintf("getr: key %q not found in map", getKey(key)), "getr", key.Pos())
		}
	}
	return out
}

// getrObjectReturns is getObjectReturns plus the strict-read static-miss
// contract for CLASS instances: a class schema is a closed field set (a
// runtime `set` cannot add fields — it rejects unknown names), so a
// concrete key the resolvable schema lacks is a guaranteed runtime
// not_found, flagged with getrObjectHandler's text. An unresolvable
// schema, a computed key, and every present field keep getObjectReturns'
// result unchanged.
func getrObjectReturns(args []Value, r *Registry) []Value {
	out := getObjectReturns(args, r)
	if r == nil || !r.Check.IsActive() || len(args) != 2 ||
		!IsConcrete(args[0]) || args[1].Parent == nil {
		return out
	}
	body, ok := r.TopTypeBody(args[1].Parent.Leaf())
	if !ok {
		return out
	}
	info, oerr := AsClassType(body)
	if oerr != nil {
		return out
	}
	if _, ok := info.AllFields().Get(getKey(args[0])); !ok {
		core.CheckAddUniqueDiagnostic(r, "not_found",
			fmt.Sprintf("getr: field %q not found in object", getKey(args[0])), "getr", args[0].Pos())
	}
	return out
}

// getrNoneReturns mirrors getrNoneHandler statically: the [Any|None] sig
// ALWAYS raises, so a receiver the checker knows is None — the `none`
// literal, or a strict (non-dynamic) None carrier from a statically-absent
// lenient read — flags the same not_found. A DYNAMIC None (an optimistic
// maybe-miss) stays silent; the residual model stays empty either way
// (the sig never produces a value).
func getrNoneReturns(args []Value, r *Registry) []Value {
	if r != nil && r.Check.IsActive() && len(args) == 2 &&
		core.IsNoneShape(args[1]) && !args[1].Dynamic {
		core.CheckAddUniqueDiagnostic(r, "not_found",
			"getr: parent is None — nothing to read a key from", "getr", args[0].Pos())
	}
	return []Value{}
}

// notFoundKeyError builds the strict-read not_found error with a
// did-you-mean suggestion over the container's actual keys (the
// diagnostics phase-4 hook: a typo'd key suggests its neighbours) and
// the key's source position, so the report points at the offending
// access. The compiled path records the SAME error (built by
// buildNotFoundKeyError) via RecordTrapErr, so the two engines match.
func notFoundKeyError(r *Registry, detail, key string, keyPos core.SrcPos, keys []string) error {
	return buildNotFoundKeyError(r, detail, key, keyPos, keys)
}

// buildNotFoundKeyError is the shared not_found builder both the runtime
// handlers and the compile-time trap use, so the strict-read miss error
// is byte-identical across engines.
func buildNotFoundKeyError(r *Registry, detail, key string, keyPos core.SrcPos, keys []string) *core.BoruError {
	ae := core.MakeBoruErrorAt("not_found", detail, "getr", srcOf(r), "", keyPos)
	if s := core.DidYouMean(key, keys); s != "" {
		ae.Suggestions = append(ae.Suggestions, core.DiagSuggestion{Message: s})
	}
	return ae
}

// srcOf returns the registry's source text (for error excerpts), or "".
func srcOf(r *Registry) string {
	if r == nil {
		return ""
	}
	return r.Source
}

func getrMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.BoruError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	// Integer key on list.
	if key.Parent.ConformsTo(TInteger) {
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			_as3, _ := AsInteger(key)
			idx := int(_as3)
			// index_out_of_range is the code this very condition's
			// CHECK-MODE mirror emits (eng/go/indexcheck.go emitIndexOOB).
			// It was a bare fmt.Errorf, so `[1,2] dotr 9` produced an error
			// with NO code — nothing for `do […] error [dot code case …]` to
			// dispatch on, for the most ordinary indexing mistake there is.
			if idx < 0 || idx >= list.Len() {
				return nil, r.BoruError("index_out_of_range",
					fmt.Sprintf("getr: index %d out of bounds (length %d)", idx, list.Len()), "getr")
			}
			return []Value{list.Get(idx)}, nil
		}
	}
	k := getKey(key)
	// A module namespace answers $name/$module from its facet, and a
	// missing export raises the module-flavored miss (the strict export
	// contract) instead of the generic map wording.
	if val, ok := moduleNSGetSynthetic(container, k); ok {
		return []Value{val}, nil
	}
	m, _ := AsMap(container)
	if m == nil {
		// Same code, same message, as this handler's own check-mode mirror
		// forty lines up (getrNodeReturns) — which was already coded while
		// the runtime path was not.
		return nil, r.BoruError("getr_error",
			fmt.Sprintf("getr: expected a map, got %s", container.Parent.String()), "getr")
	}
	val, ok := m.Get(k)
	if !ok {
		if core.ModuleNSOf(container) != nil {
			return nil, moduleNSGetrMiss(r, container, k, key.Pos())
		}
		return nil, notFoundKeyError(r, fmt.Sprintf("getr: key %q not found in map", k), k, key.Pos(), m.Keys())
	}
	return []Value{val}, nil
}

func getrObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.BoruError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return nil, notFoundKeyError(r, fmt.Sprintf("getr: key %q not found in object", k), k, key.Pos(), m.Keys())
		}
		return []Value{val}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		val, ok := ri.GetField(k)
		if !ok {
			return nil, notFoundKeyError(r, fmt.Sprintf("getr: field %q not found in resource", k), k, key.Pos(), ri.Fields.Keys())
		}
		return []Value{val}, nil
	}
	oi, _ := AsClassInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return nil, notFoundKeyError(r, fmt.Sprintf("getr: field %q not found in object", k), k, key.Pos(), oi.Fields.Keys())
	}
	return []Value{val}, nil
}

func getrNoneHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.BoruError("not_found", "getr: parent is None — nothing to read a key from", "getr")
}
