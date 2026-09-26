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
// Compile-pass discipline: the CONTEXT / Store shapes (contextReturns,
// set/get over a Store) are gated to plain (non-Compiling) check passes —
// store programs compile natively through the flat-map typing and that
// stream stays byte-identical. The FLEX shapes are not: `flex` mints on
// both passes, a FlexMap / FlexList write records into its receiver's shape
// on both (2026-09-26 — a member read then carries the adopted bound, so
// `set b 9 f.a` commits the FlexMap overload and its one result instead of
// the dynamic(Any) member's Class overload and none, flex.tsv L228/L230/
// L236), and the compile pass keeps the legacy fresh result carrier so the
// recording's operand identities do not move. Every read is GRADUAL, so a
// compiled consumer re-matches at run time and the VM enforces the result
// count it committed: a shape a hidden writer invalidated defers loudly,
// it never runs the wrong overload.

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

// MintFlexListShapeCarrier is MintFlexShapeCarrier's LIST twin: a CONCRETE
// plain list — the operand of `flex`, or a list written into a shaped flex
// container — becomes an abstract FlexList carrier whose StoreShapeInfo
// records the ELEMENT join in Vals (an index is not a stable key: push /
// unshift / del shift every position, so the shape claims one bound for
// every element). Each element is adopted the way FlexDeepCopy adopts it
// (a nested plain map becomes a FlexMap shape, a nested list a FlexList
// shape), and a dispatch-bearing element poisons the join (RecordVal), so
// reads keep the dynamic(Any) hatch. The writers (`push` / `unshift` /
// `append` / indexed `set`) join into the same Vals; a read surfaces the
// join GRADUAL (ShapeFieldRead), a bound the runtime re-match discharges.
//
// ok=false declines (non-concrete, a typed / table list — the D2 element
// tag owns those — or past the depth cap) and the caller keeps its bare
// FlexList carrier.
func MintFlexListShapeCarrier(src core.Value, depth int) (core.Value, bool) {
	if depth > flexShapeMaxDepth {
		return core.Value{}, false
	}
	if !core.IsConcrete(src) || src.Parent == nil || !src.Parent.ConformsTo(core.TList) ||
		core.IsTypedList(src) || core.IsTableType(src) {
		return core.Value{}, false
	}
	l, err := core.AsList(src)
	if err != nil {
		return core.Value{}, false
	}
	out := core.NewStoreShapeCarrier(core.TFlexList, 0)
	ss, _ := StoreShapeOf(out)
	for _, el := range l.Slice() {
		ss.RecordVal(AdoptShapeValue(el, depth+1))
	}
	return out, true
}

// FlexListShapeOf returns the shape of a store-shaped FlexList CARRIER (the
// element-join shape MintFlexListShapeCarrier mints), or ok=false for
// anything else — a FlexMap / patrun / context shape keys its writes, so an
// element read or write must not reach its Vals.
func FlexListShapeOf(v core.Value) (*core.StoreShapeInfo, bool) {
	if v.Parent == nil || !v.Parent.ConformsTo(core.TFlexList) {
		return nil, false
	}
	return StoreShapeOf(v)
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
	if nested, ok := MintFlexListShapeCarrier(v, depth); ok {
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
