package native

import (
	"fmt"
)

// The "remove" word is registered via the consolidated Natives slice in
// natives.go.
//
// removeEntityHandler handles remove with an Entity object instance.
func removeEntityHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap := entityToAPIMap(args[0])
	return removeAPIHandler([]Value{NewMap(apiMap)}, ctx, stack, r)
}

// removeEntityOptsHandler handles remove with an Entity object instance and an options map.
// Sig is opts-first: args[0]=opts, args[1]=entity.
func removeEntityOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m, _ := AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "query")
	return removeAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// removeAPIOptsHandler handles remove with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func removeAPIOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m1, _ := AsMap(args[1])
	_m0, _ := AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "query")
	return removeAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// removeAPIHandler handles remove with {kind:"api", spec:String, entity:String, query:{...}}.
func removeAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "remove", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Remove(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("remove: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "remove")
	if err != nil {
		return nil, err
	}

	return []Value{v}, nil
}

// removeRecordHandler handles remove on a record type (not a table).
// Returns an empty table.
func removeRecordHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewList([]Value{})}, nil
}

// removeHandler finds a record by its "id" field and removes it from the table.
// Returns the updated table. The map must contain an "id" field.
func removeHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	filter, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	rows := _lst.Elems()

	idVal, ok := filter.Get("id")
	if !ok {
		return nil, fmt.Errorf("remove: filter must contain an \"id\" field")
	}
	id, err := AsString(idVal)
	if err != nil {
		return nil, err
	}

	found := false
	var result []Value
	for _, row := range rows {
		if !ParentOf(&row).ConformsTo(TMap) {
			result = append(result, row)
			continue
		}
		rec, _ := AsMap(row)
		existing, ok := rec.Get("id")
		if ok {
			existingStr, _ := AsString(existing)
			if existingStr == id {
				found = true
				continue // skip this record
			}
		}
		result = append(result, row)
	}

	if !found {
		return nil, r.BoruError("remove_error", fmt.Sprintf("remove: no record found with id %q", id), "remove")
	}

	if result == nil {
		result = []Value{}
	}
	return []Value{NewList(result)}, nil
}
