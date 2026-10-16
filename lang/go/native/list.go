package native

import (
	"fmt"
)

// The "list" word is registered via the consolidated Natives slice in
// natives.go. This file keeps the SDK/table/record-type handlers along
// with shared helpers (recordMatches, valuesEqual) used by load/update/
// remove.
//
// listEntityHandler handles list with an Entity object instance.
func listEntityHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap := entityToAPIMap(args[0])
	return listAPIHandler([]Value{NewMap(apiMap)}, ctx, stack, r)
}

// listEntityOptsHandler handles list with an Entity object instance and an options map.
// Sig is opts-first per the data-last principle: args[0]=opts, args[1]=entity.
func listEntityOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m, _ := AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "query")
	return listAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// listAPIOptsHandler handles list with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func listAPIOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m1, _ := AsMap(args[1])
	_m0, _ := AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "query")
	return listAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// listAPIHandler handles list with {kind:"api", spec:String, entity:String}.
// It uses the UniversalManager to create/cache an SDK, then calls entity.List().
// An optional query field ({query:{...}}) is passed as the reqmatch filter.
func listAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "list", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.List(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("list: entity %q: %w", entityName, err)
	}

	items, _ := result.([]any)
	rows, err := convertResultList(items, "list")
	if err != nil {
		return nil, err
	}

	return []Value{NewList(rows)}, nil
}

// listAllHandler returns all records from a table as a list.
func listAllHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	rows := _lst.Elems()
	result := make([]Value, len(rows))
	copy(result, rows)
	return []Value{NewList(result)}, nil
}

// listFilterHandler returns records from a table that match the given map.
// A record matches when every key-value pair in the filter map has an equal
// value in the corresponding record field. Sig is opts-first: args[0]=filter,
// args[1]=list.
func listFilterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[1])
	rows := _lst.Elems()
	filter, _ := AsMap(args[0])

	var matched []Value
	for _, row := range rows {
		if !row.Parent().ConformsTo(TMap) {
			continue
		}
		rec, _ := AsMap(row)
		if recordMatches(rec, filter) {
			matched = append(matched, row)
		}
	}

	if matched == nil {
		matched = []Value{}
	}
	return []Value{NewList(matched)}, nil
}

// listRecordAllHandler handles list on a record type (not a table).
// Returns an empty table.
func listRecordAllHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewList([]Value{})}, nil
}

// listRecordFilterHandler handles list on a record type with a filter.
// Returns an empty table.
func listRecordFilterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewList([]Value{})}, nil
}

// recordMatches reports whether all key-value pairs in filter are present
// in rec with equal values. Equality is checked by comparing Value.String()
// representations for scalar types and structural equality for others.
func recordMatches(rec ReadMap, filter ReadMap) bool {
	for _, key := range filter.Keys() {
		filterVal, _ := filter.Get(key)
		recVal, ok := rec.Get(key)
		if !ok {
			return false
		}
		if !valuesEqual(filterVal, recVal) {
			return false
		}
	}
	return true
}

// valuesEqual checks equality between two values using type-aware comparison.
func valuesEqual(a, b Value) bool {
	switch {
	case a.Parent().ConformsTo(TInteger) && b.Parent().ConformsTo(TInteger):
		ai, _ := AsInteger(a)
		bi, _ := AsInteger(b)
		return ai == bi
	case a.Parent().ConformsTo(TString) && b.Parent().ConformsTo(TString):
		as, _ := AsString(a)
		bs, _ := AsString(b)
		return as == bs
	case a.Parent().ConformsTo(TBoolean) && b.Parent().ConformsTo(TBoolean):
		ab, _ := AsBoolean(a)
		bb, _ := AsBoolean(b)
		return ab == bb
	case a.Parent().Equal(TAtom) && b.Parent().Equal(TAtom):
		aa, _ := AsAtom(a)
		ba, _ := AsAtom(b)
		return aa == ba
	// Cross-type: atom and string are interchangeable for equality.
	case a.Parent().Equal(TAtom) && b.Parent().ConformsTo(TString):
		aa, _ := AsAtom(a)
		bs, _ := AsString(b)
		return aa == bs
	case a.Parent().ConformsTo(TString) && b.Parent().Equal(TAtom):
		as, _ := AsString(a)
		ba, _ := AsAtom(b)
		return as == ba
	default:
		return a.String() == b.String()
	}
}
