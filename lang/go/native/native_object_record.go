package native

import "fmt"

// This file holds the record / object type-construction handlers.
// They are not registered as words of their own — the `type`
// constructor (native_type.go) dispatches to them:
//
//	type Record [a:Integer b:String …]   RecordType from a list
//	                                          of single-pair maps.
//	type Object {a:String b:Integer …}    anonymous ObjectType
//	                                          (nominal, inheritance-aware).
//	type <objtype> {b:Integer …}          ObjectType extending the
//	                                          parent — child fields must
//	                                          unify with the parent's
//	                                          same-named fields.
//
// Algorithms (ResolveFieldType, Unify, MintType, NewRecordType,
// NewClassType, …) live in eng.

func recordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	list := args[0]
	if !list.Parent.Equal(TList) {
		return nil, r.BoruError("record_error", "record: argument must be a list", "record")
	}
	if !IsConcrete(list) {
		return nil, r.BoruError("record_error", "record: argument must be a concrete list, got type literal", "record")
	}
	elems, _ := AsList(list)
	if elems.Len() == 0 {
		return nil, r.BoruError("record_error", "record: list must have at least one field", "record")
	}
	fields := NewOrderedMap()
	for _, elem := range elems.Slice() {
		if !elem.Parent.Equal(TMap) {
			return nil, fmt.Errorf("record: each element must be a pair (map), got %s", elem.String())
		}
		m, err := AsMutableMap(elem)
		if err != nil {
			return nil, fmt.Errorf("record: each element must be a concrete pair, got %s", elem.String())
		}
		for _, key := range m.Keys() {
			val, _ := m.Get(key)
			val = ResolveFieldType(r, val)
			fields.Set(key, val)
		}
	}
	return []Value{NewRecordType(fields)}, nil
}

// parseObjectFields converts a map of field definitions into an
// OrderedMap of field name → type-constraint Value, resolving type
// references via r.
func parseObjectFields(fieldsMap *OrderedMap, r *Registry) *OrderedMap {
	fields := NewOrderedMap()
	for _, key := range fieldsMap.Keys() {
		val, _ := fieldsMap.Get(key)
		val = ResolveFieldType(r, val)
		fields.Set(key, val)
	}
	return fields
}

// classHandler implements `class {schema}` — a sealed nominal record
// type minted under Ideal/Class (NOT under Object: classes and the
// open mutable Object container are separate branches — see
// design/legacy/CLASS-OBJECT.10.ignore). The schema map follows the object rules:
// a type value declares a required field, a concrete value declares a
// default. Subclassing reuses `refine <ClassType> {…}`, which routes
// through objectWithParentHandler and propagates the Class flag.
func classHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	if !fieldsVal.Parent.Equal(TMap) {
		if spec := r.TakePendingGen(); spec != nil {
			PopGenBindings(r, spec)
		}
		return nil, r.BoruError("class_error",
			fmt.Sprintf("class: argument must be a map of field definitions, got %s", fieldsVal.String()), "class")
	}
	m, err := AsMutableMap(fieldsVal)
	if err != nil {
		if spec := r.TakePendingGen(); spec != nil {
			PopGenBindings(r, spec)
		}
		return nil, r.BoruError("class_error",
			fmt.Sprintf("class: argument must be a concrete map, got %s", fieldsVal.String()), "class")
	}
	// A pending gen spec (`def Box gen [T] class {value:T}`) turns
	// this into a generic CLASS SCHEMA: the field map is parsed with
	// the placeholder bindings live (so {value:T} declares a required
	// T-typed field), no node is minted here — `of` mints one
	// instantiation node per (schema, args) so instances get nominal
	// identity (deq "same exact class", is-by-ancestry).
	if spec := r.TakePendingGen(); spec != nil {
		fields := parseObjectFields(m, r)
		body := NewValueRaw(TClass, ClassTypeInfo{
			Fields: fields,
			ID:     GenerateObjectTypeID(),
		})
		return genWrapSchema(r, spec, body, SchemaClass)
	}
	fields := parseObjectFields(m, r)
	id := GenerateObjectTypeID()
	info := ClassTypeInfo{
		Fields: fields,
		ID:     id,
	}
	def := r.Types.MintType(id, TClass)
	return []Value{NewClassType(def, info)}, nil
}

func objectWithParentHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fieldsVal := args[0]
	parentVal := args[1]

	if !fieldsVal.Parent.Equal(TMap) {
		return nil, fmt.Errorf("object: first argument must be a map of field definitions, got %s", fieldsVal.String())
	}
	m, err := AsMutableMap(fieldsVal)
	if err != nil {
		return nil, fmt.Errorf("object: first argument must be a concrete map, got %s", fieldsVal.String())
	}

	if !IsClassType(parentVal) {
		return nil, fmt.Errorf("object: parent must be an object type, got %s", parentVal.String())
	}
	parentInfo, _ := AsClassType(parentVal)

	fields := parseObjectFields(m, r)

	parentAllFields := parentInfo.AllFields()
	for _, key := range fields.Keys() {
		childConstraint, _ := fields.Get(key)
		parentConstraint, exists := parentAllFields.Get(key)
		if !exists {
			continue
		}
		_, ok := Unify(parentConstraint, childConstraint)
		if !ok {
			return nil, fmt.Errorf("object: field %q in child type cannot expand parent type %s (child: %s, parent: %s)",
				key, parentInfo.Name, childConstraint.String(), parentConstraint.String())
		}
	}

	id := GenerateObjectTypeID()
	info := ClassTypeInfo{
		Fields: fields,
		Parent: &parentInfo,
		ID:     id,
		Name:   "",
		// Subclassing a class type (refine Foo {…}) yields a class
		// type: sealed, flat instances, minted under the parent's
		// Class-branch node.
	}
	parentDef := parentInfo.Type
	if parentDef == nil {
		parentDef = TClass
	}
	def := r.Types.MintType(id, parentDef)
	return []Value{NewClassType(def, info)}, nil
}
