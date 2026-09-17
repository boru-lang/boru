package check

import core "github.com/boru-lang/boru/core/go"

// Store-identity context typing — stage 1 of
// design/legacy/checker-precision-fronts.0.ignore §2. CheckState.ContextTypes is ONE
// flat string-keyed namespace for the whole check pass, so two stores'
// same-named keys JOIN and every store read is answered from a global
// map. The StoreShapeInfo payload keys the same best-effort typing by
// STORE IDENTITY instead: each store-class container the checker can
// identify (a context Store layer, a `flex` map) carries ONE abstract
// shape minted in check mode, and `set`/`get` over that carrier
// read/write ITS KeyTypes.
//
// The carrier is a NEW ABSTRACT payload, never a preserved concrete
// *StoreInstanceInfo: stores are mutable runtime state, and the check
// pass must be observation-free — toCarrier deliberately strips real
// store payloads, and the check-mode set/get handlers never run the
// runtime Impl (execMatch's check intercept routes them through
// ReturnsFn). The shape is what those check-mode twins mutate instead
// of the real store.
//
// Gradual/compat contract (precision only increases):
//   - a store the minting misses (an unshaped carrier from a fn param,
//     a join, an older pass) keeps the flat-ContextTypes path entirely;
//   - a SHAPED store whose key misses ALSO falls back to the flat map
//     (today's optimism, unchanged) — the shape only answers keys it
//     saw written through this store;
//   - writes go to BOTH the shape and the flat map, so unshaped readers
//     elsewhere in the pass lose nothing.
//
// Compile-pass discipline: minting, reads, and writes are all gated to
// plain (non-Compiling) check passes. Store and flex programs COMPILE
// natively today through the flat-map typing, and the compiled stream
// must stay byte-identical — the shape machinery is checker precision,
// not compile coverage (the CodeEffectInfo discipline).

// payloadMarker — see payload.go's catalogue; registered there.

// StoreShapeOf returns the shape payload of a store-shaped CARRIER, or
// ok=false for anything else (a real store value, a bare carrier, a
// concrete container).
func StoreShapeOf(v core.Value) (*core.StoreShapeInfo, bool) {
	if !v.Carrier {
		return nil, false
	}
	ss, ok := v.Data.(*core.StoreShapeInfo)
	return ss, ok
}

// flexShapeMaxDepth caps MintFlexShapeCarrier's recursion over nested
// concrete maps. A concrete map graph is finite but may be deep or (via
// shared flex handles adopted into a literal) cyclic; past the cap a
// nested map field decays to a bare FlexMap carrier.
const flexShapeMaxDepth = 8

// MintFlexShapeCarrier converts a CONCRETE plain map — the operand of
// `flex` in check mode — into an abstract FlexMap carrier bearing its
// StoreShapeInfo, mirroring FlexDeepCopy's shape statically:
//
//   - a nested concrete plain-map field mints a nested FlexMap shape
//     (recursively, depth-capped) — runtime FlexDeepCopy converts it to
//     a FlexMap;
//   - a concrete list / xml field records a bare FlexList / FlexXml
//     carrier (element typing is a later stage);
//   - an already-shaped field (a flex handle stored in the literal)
//     shares its shape pointer — runtime adoption shares the handle;
//   - a dispatch-bearing field (Function/FnDef/Reach/Splice) is OMITTED
//     so reads keep the dynamic(Any) hatch (the getNodeReturns
//     exclusion — surfacing it would push fn-value dispatch);
//   - any other field (concrete scalar, carrier) records as-is.
//
// ok=false declines (non-concrete / non-plain-map / structural-type
// input) and the caller keeps its legacy conversion, so precision only
// increases.
func MintFlexShapeCarrier(src core.Value, depth int) (core.Value, bool) {
	if depth > flexShapeMaxDepth {
		return core.Value{}, false
	}
	if !core.IsConcrete(src) || src.Parent == nil || !src.Parent.ConformsTo(core.TMap) ||
		core.IsRecordType(src) || core.IsOptionsType(src) || core.IsTypedMap(src) {
		return core.Value{}, false
	}
	m, err := core.AsMap(src)
	if err != nil || m == nil {
		return core.Value{}, false
	}
	out := core.NewStoreShapeCarrier(core.TFlexMap, 0)
	ss, _ := StoreShapeOf(out)
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if fv.Parent == nil || fv.Parent.ConformsTo(core.TFunction) ||
			core.IsReach(fv) || core.IsSplice(fv) {
			continue // dispatch-bearing: absent key reads dynamic(Any)
		}
		ss.RecordKey(k, AdoptShapeValue(fv, depth+1))
	}
	return out, true
}

// AdoptShapeValue is the static twin of AdoptIntoFlex for a value
// WRITTEN into a shaped flex container (a `set` value, a minted field):
// a concrete plain map becomes a nested FlexMap shape, a concrete
// list/xml becomes the corresponding bare flex carrier, an
// already-shaped carrier shares its pointer, anything else records
// as-is.
func AdoptShapeValue(v core.Value, depth int) core.Value {
	if _, ok := StoreShapeOf(v); ok {
		return v // flex handles share
	}
	if nested, ok := MintFlexShapeCarrier(v, depth); ok {
		return nested
	}
	if core.IsConcrete(v) && v.Parent != nil {
		switch {
		case v.Parent.ConformsTo(core.TXml):
			return core.NewCarrier(core.TFlexXml)
		case v.Parent.ConformsTo(core.TList):
			return core.NewCarrier(core.TFlexList)
		}
	}
	return v
}

// ShapeFieldRead surfaces a shape-recorded value at a READ site
// (`get`/`dot` over a shaped flex container). Always GRADUAL, never a
// strict or concrete commitment — a flex tree has runtime writers the
// shape cannot see (StructUtil.setpath, walk mutation), so the claim is
// a bound a guard discharges, exactly the record-schema field rule
// (recordSchemaFieldReturns). A nested shape / disjunct join keeps its
// payload so chained reads narrow too; a dispatch-bearing or
// unrepresentable value keeps dynamic(Any).
func ShapeFieldRead(v core.Value) core.Value {
	if _, ok := StoreShapeOf(v); ok {
		return core.NewDynamicCarrierValue(v)
	}
	if core.IsDisjunct(v) {
		return core.NewDynamicCarrierValue(v)
	}
	ft := core.ValueType(v)
	if ft == nil || ft.ConformsTo(core.TFunction) {
		return core.NewDynamicCarrier(core.TAny)
	}
	return core.NewDynamicCarrier(ft)
}
