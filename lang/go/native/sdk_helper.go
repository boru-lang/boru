package native

import (
	"fmt"
	"strings"

	udk "github.com/voxgig/udk/go"
)

// getSDK extracts the spec and entity name from an API map ({kind:"api", spec:..., entity:...}),
// looks up or creates the SDK instance, and returns the SDK and entity name.
// The return type is the narrow sdkClient seam (seams.go): production
// instances are always *udk.UniversalSDK, tests plant fakes in r.SDKCache.
// ValidateAPIDescriptor is getSDK's PURE prefix: everything decidable
// from the descriptor MAP alone — a missing "spec", a non-String spec, a
// non-String entity — returning the .json-trimmed spec and the entity
// name. Split out so a check-mode mirror can run THIS and nothing else:
// the rest of getSDK reads host state (the SDK cache, the manager, and
// the manager's Make), which is not decidable from the source and must
// stay with the runtime. Sharing these lines with the handler is what
// keeps the mirrored message byte-identical.
func ValidateAPIDescriptor(apiMap ReadMap, opName string) (string, string, error) {
	specVal, ok := apiMap.Get("spec")
	if !ok {
		return "", "", fmt.Errorf("%s: missing required \"spec\" field", opName)
	}
	spec, err := AsString(specVal)
	if err != nil {
		return "", "", fmt.Errorf("%s: spec: %w", opName, err)
	}
	var entityName string
	if entityVal, ok := apiMap.Get("entity"); ok {
		entityName, err = AsString(entityVal)
		if err != nil {
			return "", "", fmt.Errorf("%s: entity: %w", opName, err)
		}
	}
	// Strip .json extension if present.
	return strings.TrimSuffix(spec, ".json"), entityName, nil
}

func getSDK(apiMap ReadMap, opName string, r *Registry) (sdkClient, string, error) {
	spec, entityName, err := ValidateAPIDescriptor(apiMap, opName)
	if err != nil {
		return nil, "", err
	}

	// Get or create SDK.
	var sdkInst sdkClient
	if cached, ok := r.SDKCache[spec]; ok {
		sdkInst, _ = cached.(sdkClient)
	}
	if sdkInst == nil {
		mgr, ok := r.Manager.(*udk.UniversalManager)
		if !ok || mgr == nil {
			return nil, "", fmt.Errorf("%s: no manager configured", opName)
		}
		sdkInst = mgr.Make(spec)
		r.SDKCache[spec] = sdkInst
	}

	return sdkInst, entityName, nil
}

// entityToAPIMap converts an Object/Resource/Entity instance into the
// OrderedMap that getSDK expects ({kind:..., spec:..., entity:...}).
func entityToAPIMap(v Value) *OrderedMap {
	m := NewOrderedMap()
	if !IsConcrete(&v) {
		return m
	}
	inst, err := AsResourceInstance(v)
	if err != nil {
		return m
	}
	if kind, ok := inst.GetField("kind"); ok {
		m.Set("kind", kind)
	}
	if spec, ok := inst.GetField("spec"); ok {
		m.Set("spec", spec)
	}
	if entity, ok := inst.GetField("entity"); ok {
		m.Set("entity", entity)
	}
	return m
}

// entityToAPIMapWithOpts converts an Entity instance into an API map and
// merges an options map into the given field (query or data).
func entityToAPIMapWithOpts(v Value, opts ReadMap, field string) *OrderedMap {
	m := entityToAPIMap(v)
	return mergeAPIOptions(m, opts, field)
}

// convertResultList converts a []any result from the SDK into a boru list of maps.
func convertResultList(items []any, opName string) ([]Value, error) {
	rows := make([]Value, 0, len(items))
	for _, item := range items {
		if ent, ok := item.(udk.Entity); ok {
			item = ent.Data()
		}
		v, err := anyToValue(item)
		if err != nil {
			return nil, fmt.Errorf("%s: converting result: %w", opName, err)
		}
		rows = append(rows, v)
	}
	return rows, nil
}

// convertResultItem converts a single any result from the SDK into a boru value.
func convertResultItem(item any, opName string) (Value, error) {
	if ent, ok := item.(udk.Entity); ok {
		item = ent.Data()
	}
	v, err := anyToValue(item)
	if err != nil {
		return Value{}, fmt.Errorf("%s: converting result: %w", opName, err)
	}
	return v, nil
}

// extractQuery extracts an optional query map from the API options map.
func extractQuery(apiMap ReadMap) map[string]any {
	if queryVal, ok := apiMap.Get("query"); ok && queryVal.Parent.ConformsTo(TMap) {
		return valueToMap(queryVal)
	}
	return nil
}

// extractData extracts an optional data map from the API options map.
func extractData(apiMap ReadMap) map[string]any {
	if dataVal, ok := apiMap.Get("data"); ok && dataVal.Parent.ConformsTo(TMap) {
		return valueToMap(dataVal)
	}
	return nil
}

// mergeAPIOptions merges the keys of an options map into the base API map.
// For query operations (list, load, remove), options keys go into query.
// For data operations (create, update), options keys go into data.
// Returns a new map; the original is not modified.
func mergeAPIOptions(base ReadMap, opts ReadMap, field string) *OrderedMap {
	merged := NewOrderedMap()
	for _, k := range base.Keys() {
		v, _ := base.Get(k)
		merged.Set(k, v)
	}

	// Get existing field map or create a new one.
	existing := NewOrderedMap()
	if v, ok := merged.Get(field); ok && v.Parent.ConformsTo(TMap) {
		if src, _ := AsMap(v); src != nil {
			for _, k := range src.Keys() {
				val, _ := src.Get(k)
				existing.Set(k, val)
			}
		}
	}

	// Merge opts into the field map.
	for _, k := range opts.Keys() {
		v, _ := opts.Get(k)
		existing.Set(k, v)
	}

	merged.Set(field, NewMap(existing))
	return merged
}
