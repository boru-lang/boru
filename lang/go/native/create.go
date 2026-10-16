package native

import (
	"fmt"
)

// The "create" word is registered via the consolidated Natives slice in
// natives.go. Handlers below cover the SDK/table/record overloads.
//
// createEntityHandler handles create with an Entity object instance.
func createEntityHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap := entityToAPIMap(args[0])
	return createAPIHandler([]Value{NewMap(apiMap)}, ctx, stack, r)
}

// createEntityOptsHandler handles create with an Entity object instance and a data map.
// Sig is opts-first: args[0]=data, args[1]=entity.
func createEntityOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m, _ := AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "data")
	return createAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// createAPIOptsHandler handles create with {kind:"api",...} and an extra data map.
// The options map is merged into the data field of the API map.
// Sig is opts-first: args[0]=data, args[1]=apiMap (pattern-matched).
func createAPIOptsHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_m1, _ := AsMap(args[1])
	_m0, _ := AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "data")
	return createAPIHandler([]Value{NewMap(merged)}, ctx, stack, r)
}

// createAPIHandler handles create with {kind:"api", spec:String, entity:String, data:{...}}.
func createAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "create", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Create(extractData(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("create: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "create")
	if err != nil {
		return nil, err
	}

	return []Value{v}, nil
}

// createRecordHandler handles create on a record type (not a table).
// Returns an empty table.
func createRecordHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return []Value{NewList([]Value{})}, nil
}

// createHandler appends a new record to the table.
// The map must contain an "id" field. If a record with the same id already
// exists, an error is returned.
func createHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	rec, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	rows := _lst.Elems()

	idVal, ok := rec.Get("id")
	if !ok {
		return nil, fmt.Errorf("create: record must contain an \"id\" field")
	}
	id, err := AsString(idVal)
	if err != nil {
		return nil, fmt.Errorf("create: id: %w", err)
	}

	// Check for duplicate id.
	for _, row := range rows {
		if !row.Parent().ConformsTo(TMap) {
			continue
		}
		m, _ := AsMap(row)
		if existing, ok := m.Get("id"); ok {
			existingStr, _ := AsString(existing)
			if existingStr == id {
				return nil, r.BoruError("create_error", fmt.Sprintf("create: record with id %q already exists", id), "create")
			}
		}
	}

	result := make([]Value, len(rows)+1)
	copy(result, rows)
	result[len(rows)] = args[0]
	return []Value{NewList(result)}, nil
}
