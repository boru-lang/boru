package native

import (
	"fmt"
)

// The "update" word is registered via the consolidated Natives slice in
// natives.go.
//
// updateEntityHandler handles update with an Entity object instance.
func updateEntityHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap := entityToAPIMap(args[0])
	return updateAPIHandler([]Value{NewMap(apiMap)}, ctx, stack, r)
}

// updateEntityOptsHandler handles update with an Entity object instance and a data map.
// Sig is opts-first: args[0]=data, args[1]=entity.
func updateEntityOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m, _ := AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "data")
	return updateAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// updateAPIOptsHandler handles update with {kind:"api",...} and an extra data map.
// The options map is merged into the data field of the API map.
// Sig is opts-first: args[0]=data, args[1]=apiMap (pattern-matched).
func updateAPIOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m1, _ := AsMap(args[1])
	_m0, _ := AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "data")
	return updateAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// updateAPIHandler handles update with {kind:"api", spec:String, entity:String, data:{...}}.
func updateAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "update", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Update(extractData(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("update: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "update")
	if err != nil {
		return nil, err
	}

	return []Value{v}, nil
}

// updateRecordHandler handles update on a record type (not a table).
// Returns an empty table.
func updateRecordHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewList([]Value{})}, nil
}

// updateHandler finds a record by its "id" field and merges the provided
// fields into it. Returns the updated table. The map must contain an "id" field.
func updateHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	patch, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	rows := _lst.Elems()

	idVal, ok := patch.Get("id")
	if !ok {
		return nil, fmt.Errorf("update: record must contain an \"id\" field")
	}
	id, err := AsString(idVal)
	if err != nil {
		return nil, fmt.Errorf("update: id: %w", err)
	}

	found := false
	result := make([]Value, len(rows))
	for i, row := range rows {
		if !row.Parent().ConformsTo(TMap) {
			result[i] = row
			continue
		}
		rec, _ := AsMap(row)
		existing, ok := rec.Get("id")
		if !ok {
			result[i] = row
			continue
		}
		existingStr, _ := AsString(existing)
		if existingStr != id {
			result[i] = row
			continue
		}

		// Merge: copy existing fields, then overlay patch fields.
		merged := NewOrderedMap()
		for _, key := range rec.Keys() {
			v, _ := rec.Get(key)
			merged.Set(key, v)
		}
		for _, key := range patch.Keys() {
			v, _ := patch.Get(key)
			merged.Set(key, v)
		}
		result[i] = NewMap(merged)
		found = true
	}

	if !found {
		return nil, r.BoruError("update_error", fmt.Sprintf("update: no record found with id %q", id), "update")
	}

	return []Value{NewList(result)}, nil
}
