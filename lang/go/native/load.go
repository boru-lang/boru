package native

import (
	"fmt"
)

// The "load" word is registered via the consolidated Natives slice in
// natives.go.
//
// loadEntityHandler handles load with an Entity object instance.
func loadEntityHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap := entityToAPIMap(args[0])
	return loadAPIHandler([]Value{NewMap(apiMap)}, ctx, stack, r)
}

// loadEntityOptsHandler handles load with an Entity object instance and an options map.
// Sig is opts-first: args[0]=opts, args[1]=entity.
func loadEntityOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m, _ := AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "query")
	return loadAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// loadAPIOptsHandler handles load with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func loadAPIOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m1, _ := AsMap(args[1])
	_m0, _ := AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "query")
	return loadAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// loadAPIHandler handles load with {kind:"api", spec:String, entity:String, query:{...}}.
func loadAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "load", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Load(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("load: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "load")
	if err != nil {
		return nil, err
	}

	return []Value{v}, nil
}

// loadRecordHandler handles load on a record type (not a table).
// Returns an empty map.
func loadRecordHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewMap(NewOrderedMap())}, nil
}

// loadHandler finds and returns a single record matching the filter.
// Returns an error if no matching record is found.
func loadHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	filter, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	rows := _lst.Elems()

	for _, row := range rows {
		if !row.Parent().ConformsTo(TMap) {
			continue
		}
		rec, _ := AsMap(row)
		if recordMatches(rec, filter) {
			return []Value{row}, nil
		}
	}

	return nil, r.BoruError("load_error", "load: no record found matching filter", "load")
}
