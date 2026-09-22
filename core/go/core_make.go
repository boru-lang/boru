package core

import (
	"fmt"
	"strconv"
	"strings"
)

// registerCoreMake installs `make TARGET data` — the universal
// constructor for typed values. Multiple overloads cover the major
// type-construction shapes:
//
//	make ScalarType data            cast / parse a scalar
//	make ScalarType {opts} data     scalar with options (Pathon abs flag)
//	make ClassType data             instantiate a class (flat, sealed)
//	make *Type *Type {opts}           three-arg shape with arbitrary options
//	make *Type Any                   two-arg fallback
//
// Mirrors the production lang `make` (formerly in
// lang/go/engine/native_type_make_helpers.go); the handlers are
// ported verbatim. Lang re-exports the helpers via aliases so any
// callers that reach into the package-private surface keep working.
// The `make` word registration has moved to
// lang/go/engine/native_make.go. The Make* handlers below are exported
// algorithm primitives — lang's registration wires the dispatch
// table without forking the algorithm.

// isTypeLike returns true if v looks like a type target for make
// (type literal with nil Data, record type, options type, table
// type, or object type).
func isTypeLike(v Value) bool {
	if IsBareTypeNode(v) {
		return true
	}
	return IsRecordType(v) || IsOptionsType(v) || IsTableType(v) ||
		IsClassType(v) || IsMicronType(v) || IsHostTypeBody(v)
}

// MakeRecord creates a record instance from a source value and
// options. Used by `make` (via the Record Ideal's Instantiate) and
// by the 3-arg make-with-options path.
//
// Backward-compat wrapper — delegates to MakeRecordR with nil
// Registry. Use MakeRecordR when fields may carry predicate-type
// constraints that need RunPredicate to evaluate.
func MakeRecord(recType RecordTypeInfo, srcVal Value, useBase bool) ([]Value, error) {
	return MakeRecordR(recType, srcVal, useBase, nil)
}

// MakeRecordR is MakeRecord with Registry threading for
// predicate-typed field constraints. See MakeFieldValueR.
func MakeRecordR(recType RecordTypeInfo, srcVal Value, useBase bool, r *Registry) ([]Value, error) {
	fieldKeys := recType.Fields.Keys()
	result := NewOrderedMap()

	fillFromMap := func(provided *OrderedMap) error {
		for _, key := range provided.Keys() {
			if _, ok := recType.Fields.Get(key); !ok {
				return fmt.Errorf("make: unknown field %q", key)
			}
		}
		for _, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			val, ok := provided.Get(key)
			if !ok {
				if useBase {
					bv, err := BaseValueForConstraint(constraint)
					if err != nil {
						return fmt.Errorf("make: field %q: %w", key, err)
					}
					result.Set(key, bv)
					continue
				}
				noneVal := NewTypeLiteral(TNone)
				if _, unifOK := Unify(constraint, noneVal); unifOK {
					result.Set(key, noneVal)
					continue
				}
				return fmt.Errorf("make: missing field %q", key)
			}
			converted, err := MakeFieldValueR(val, constraint, r)
			if err != nil {
				return fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
		return nil
	}

	if srcVal.Parent.ConformsTo(TMap) {
		provided, err := AsMutableMap(srcVal)
		if err != nil {
			return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
		return []Value{NewMap(result)}, nil
	}

	if !srcVal.Parent.ConformsTo(TList) {
		return nil, fmt.Errorf("make: record values must be a list or map, got %s", srcVal.String())
	}
	if !IsConcrete(srcVal) {
		return nil, fmt.Errorf("make: record values must be a concrete list, got type literal")
	}
	elems, _ := AsList(srcVal)

	isNamed := elems.Len() > 0 && elems.Get(0).Parent.ConformsTo(TMap)
	if isNamed {
		if _, err := AsMutableMap(elems.Get(0)); err != nil {
			isNamed = false
		}
	}

	if isNamed {
		provided := NewOrderedMap()
		for _, elem := range elems.Slice() {
			if !elem.Parent.ConformsTo(TMap) {
				return nil, fmt.Errorf("make: mixed named and positional fields")
			}
			m, err := AsMutableMap(elem)
			if err != nil {
				return nil, fmt.Errorf("make: expected concrete map pair, got %s", elem.String())
			}
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				provided.Set(key, val)
			}
		}
		if err := fillFromMap(provided); err != nil {
			return nil, err
		}
	} else {
		if elems.Len() != len(fieldKeys) {
			return nil, fmt.Errorf("make: expected %d values, got %d",
				len(fieldKeys), elems.Len())
		}
		for i, key := range fieldKeys {
			constraint, _ := recType.Fields.Get(key)
			converted, err := MakeFieldValueR(elems.Get(i), constraint, r)
			if err != nil {
				return nil, fmt.Errorf("make: field %q: %w", key, err)
			}
			result.Set(key, converted)
		}
	}

	return []Value{NewMap(result)}, nil
}

// parseMakeOptions extracts make options from an options map.
func parseMakeOptions(opts Value) (useBase bool, err error) {
	if !opts.Parent.ConformsTo(TMap) {
		return false, fmt.Errorf("make: options must be a map, got %s", opts.String())
	}
	m, err := AsMutableMap(opts)
	if err != nil {
		return false, fmt.Errorf("make: expected concrete options map")
	}
	if v, ok := m.Get("base"); ok {
		v = ResolveWordValue(v)
		if b, bErr := AsBoolean(v); bErr == nil && b {
			useBase = true
		}
	}
	return useBase, nil
}

// MakeObject is the exported wrapper around the class-instance
// construction path. Used by lang-side `def x:T body` to build a
// typed instance from a raw Map body when the typed binding's
// constraint is an object (class) type.
func MakeObject(objType ClassTypeInfo, srcVal Value, r *Registry) ([]Value, error) {
	return makeObject(objType, srcVal, r)
}

func makeObject(objType ClassTypeInfo, srcVal Value, r *Registry) ([]Value, error) {
	if !srcVal.Parent.ConformsTo(TMap) {
		return nil, fmt.Errorf("make: object values must be a map, got %s", srcVal.String())
	}
	provided, err := AsMutableMap(srcVal)
	if err != nil {
		return nil, fmt.Errorf("make: expected concrete map, got %s", srcVal.String())
	}
	// Every object type is now a class — flat, sealed instances (open
	// objects and their prototype chain were removed). See
	// design/legacy/CLASS-OBJECT.10.ignore §3.
	return makeClassInstance(objType, provided, r)
}

// makeClassInstance constructs a flat, sealed class instance: the
// full field set (inherited first, then own — AllFields order) is
// resolved eagerly into a single field map. A provided key the schema
// doesn't declare is an error; a missing field takes its schema
// default when one exists (constraint with a concrete payload) and is
// otherwise a loud missing-field error. The instance carries no
// Prototype — reads are a single map lookup.
//
// Field validation is STRICT (no silent conversion — unlike the
// legacy object path): a typed field rejects non-conforming values
// loudly, predicate-typed fields run their predicate via Unify, and
// a defaulted field rejects values outside the default's own type.
// See design/legacy/CLASS-OBJECT.10.ignore §3c.
func makeClassInstance(objType ClassTypeInfo, provided *OrderedMap, r *Registry) ([]Value, error) {
	allFields := objType.AllFields()

	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			return nil, fmt.Errorf("make: unknown field %q for class %s", key, objType.Name)
		}
	}

	result := NewOrderedMap()
	for _, key := range allFields.Keys() {
		constraint, _ := allFields.Get(key)
		val, hasVal := provided.Get(key)

		if !hasVal {
			if constraint.Data != nil {
				// A concrete default fills the omitted field — as a
				// FRESH copy when it is (or contains) shared-mutable
				// state, so every instance gets its own FlexList /
				// FlexMap / Array / Store / instance rather than all
				// instances aliasing the single schema value (the
				// Python mutable-default trap, silent until two
				// instances cross-talk).
				result.Set(key, FreshenDefault(constraint))
				continue
			}
			return nil, fmt.Errorf("make: missing field %q for class %s", key, objType.Name)
		}

		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, fmt.Errorf("make: field %q: %w", key, err)
		}
		result.Set(key, checked)
	}

	instanceType := objType.Type
	if instanceType == nil {
		instanceType = TClass
	}
	return []Value{NewClassInstance(instanceType, ClassInstanceInfo{
		TypeRef: &objType,
		Fields:  result,
	})}, nil
}

// makeResource constructs a flat instance of a ResourceType (the SDK
// Resource/Entity hierarchy). Like a class instance it resolves every
// field (own + inherited) eagerly into a single map with no prototype;
// unlike a class it uses the object-type diagnostics ("missing field",
// "unknown field") the SDK spec pins, and its own ResourceInstanceInfo
// payload keeps it off the class-instance representation.
// MakeResource is the exported entry to construct a Resource/Entity
// instance from a resolved ResourceType and a provided field map,
// mirroring MakeObject for the class path. Used by the lang layer's
// typed-def and setpath rebuild paths so a Resource instance is built
// the same way `make Entity {…}` builds it.
func MakeResource(resType ResourceTypeInfo, provided *OrderedMap, r *Registry) ([]Value, error) {
	return makeResource(resType, provided, r)
}

func makeResource(resType ResourceTypeInfo, provided *OrderedMap, r *Registry) ([]Value, error) {
	allFields := resType.AllFields()

	for _, key := range provided.Keys() {
		if _, ok := allFields.Get(key); !ok {
			return nil, fmt.Errorf("make: unknown field %q for %s", key, resType.Name)
		}
	}

	result := NewOrderedMap()
	for _, key := range allFields.Keys() {
		constraint, _ := allFields.Get(key)
		val, hasVal := provided.Get(key)
		if !hasVal {
			if constraint.Data != nil {
				result.Set(key, FreshenDefault(constraint))
				continue
			}
			return nil, fmt.Errorf("make: missing field %q for %s", key, resType.Name)
		}
		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, fmt.Errorf("make: field %q: %w", key, err)
		}
		result.Set(key, checked)
	}

	instanceType := resType.Type
	return []Value{NewResourceInstance(instanceType, ResourceInstanceInfo{
		TypeRef: &resType,
		Fields:  result,
	})}, nil
}

// MakeClassInstance constructs a class instance from a field map,
// running the same strict validation as `make` — exported for the
// struct-utility writers (StructUtil.setpath, clone) whose instance
// edits round-trip through construction so schema checks run.
func MakeClassInstance(objType ClassTypeInfo, provided *OrderedMap, r *Registry) (Value, error) {
	vals, err := makeClassInstance(objType, provided, r)
	if err != nil {
		return Value{}, err
	}
	return vals[0], nil
}

// containsSharedMutable reports whether v is — or transitively
// contains — a payload whose mutations are visible through shared
// Value copies: a flex node (FlexList's *FlexListData, FlexMap's
// pointer-backed *OrderedMap), a Store (*StoreInstanceInfo), or an
// object/class instance (whose Fields *OrderedMap `set` writes in
// place). Drives FreshenDefault's identity fast path: scalars and
// purely-immutable nodes share safely and are returned unchanged.
// containsCapturingFn reports whether v is, or holds at any depth, a fn
// value WITH captures — a closure, whose captured state no const can carry.
func containsCapturingFn(v Value) bool {
	if fd, ok := v.Data.(FnDefInfo); ok {
		return len(fd.Captured) > 0
	}
	if !IsConcrete(v) {
		return false
	}
	if v.Parent.ConformsTo(TMap) {
		if m, err := AsMap(v); err == nil && m != nil {
			for _, k := range m.Keys() {
				val, _ := m.Get(k)
				if containsCapturingFn(val) {
					return true
				}
			}
		}
		return false
	}
	if v.Parent.ConformsTo(TList) {
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			for i := 0; i < lst.Len(); i++ {
				if containsCapturingFn(lst.Get(i)) {
					return true
				}
			}
		}
	}
	return false
}

func containsSharedMutable(v Value) bool {
	if IsFlexNode(v) || IsWeakFlexNode(v) || IsStore(v) || IsClassInstance(v) {
		return true
	}
	if !IsConcrete(v) {
		return false
	}
	if v.Parent.ConformsTo(TMap) {
		if m, err := AsMap(v); err == nil && m != nil {
			for _, k := range m.Keys() {
				val, _ := m.Get(k)
				if containsSharedMutable(val) {
					return true
				}
			}
		}
		return false
	}
	if v.Parent.ConformsTo(TList) {
		if lst, err := AsList(v); err == nil && !lst.IsNil() {
			for i := 0; i < lst.Len(); i++ {
				if containsSharedMutable(lst.Get(i)) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// FreshenDefault returns v with every shared-mutable container payload
// it transitively contains replaced by a fresh, independent copy —
// same kind, same type tag, new identity. Flex nodes copy to flex
// nodes, Stores to a fresh own-data layer, and instances to a fresh
// Fields map; immutable containers are rebuilt
// only on the path down to a mutable payload, and a value with no
// shared-mutable state anywhere inside is returned unchanged.
//
// This is what makes a concrete mutable default in a class schema
// per-instance: `def Foo class {items:(flex [])}` hands each `make`
// its own FlexList instead of aliasing the schema's single one.
// (Pointer payloads the kernel cannot meaningfully duplicate — a
// running Timer/Interval, module/extension payloads — pass through
// unchanged.)
func FreshenDefault(v Value) Value {
	if !containsSharedMutable(v) {
		return v
	}
	out := v
	switch {
	case IsWeakFlexNode(v):
		// Freshen re-establishes the weak representation: strong slots
		// freshen recursively; weak slots carry over as-is — the
		// referent is a handle the caller shares, and a freshened
		// referent nobody holds would be dead on arrival.
		switch wd := v.Data.(type) {
		case *WeakFlexMapData:
			out.Data = CloneWeakFlexMapData(wd, FreshenDefault)
		case *WeakFlexListData:
			out.Data = CloneWeakFlexListData(wd, FreshenDefault)
		case *WeakFlexXmlData:
			out.Data = CloneWeakFlexXmlData(wd, FreshenDefault, freshenAttrMap)
		}
	case IsClassInstance(v):
		info, err := AsClassInstance(v)
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return v
		}
		fields := NewOrderedMap()
		for _, k := range info.Fields.Keys() {
			fv, _ := info.Fields.Get(k)
			fields.Set(k, FreshenDefault(fv))
		}
		ninfo := info // struct copy: TypeRef stays shared (class instances are flat, no prototype)
		ninfo.Fields = fields
		out.Data = ninfo
	case IsStore(v):
		si, err := AsStore(v)
		if err != nil || si == nil {
			return v
		}
		nsi := *si // Prototype chain stays shared (read-only fallback)
		nsi.Data = make(map[string]Value, len(si.Data))
		for k, val := range si.Data {
			nsi.Data[k] = FreshenDefault(val)
		}
		out.Data = &nsi
	case v.Parent.ConformsTo(TMap):
		// Plain Map and FlexMap share the MapPayload shape; the Parent
		// tag (kept by the struct copy) preserves the flavor.
		m, err := AsMap(v)
		if err != nil || m == nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return v
		}
		om := NewOrderedMap()
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			om.Set(k, FreshenDefault(val))
		}
		out.Data = MapPayload{M: om}
	case v.Parent.ConformsTo(TList):
		lst, err := AsList(v)
		if err != nil || lst.IsNil() {
			return v
		}
		elems := make([]Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			elems[i] = FreshenDefault(lst.Get(i))
		}
		if IsFlexList(v) {
			out.Data = &FlexListData{Elems: elems}
		} else {
			out.Data = ListPayload{Elems: elems}
		}
	}
	return out
}

// freshenAttrMap freshens an XML attribute map for FreshenDefault's
// weak-xml arm.
func freshenAttrMap(m *OrderedMap) *OrderedMap {
	om := NewOrderedMap()
	if m != nil {
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			om.Set(k, FreshenDefault(v))
		}
	}
	return om
}

// MakeClassFieldValue validates one value against a class-schema
// field constraint, strictly: a bare type-node constraint requires a
// conforming value (no conversion fallback — a Float is not an
// Integer; say so); a predicate / disjunction constraint runs through
// Unify so the predicate body executes; a concrete default constrains
// the field to the default's own (declared) type, which makes
// refined-typed defaults enforce their refinement. Shared by `make`
// and the class-instance `set` handler so write-time enforcement
// matches construction.
func MakeClassFieldValue(val Value, constraint Value, r *Registry) (Value, error) {
	val = ResolveWordValue(val)

	// A NAMED field constraint ({i:Inner}) was evaluated to its NODE at
	// class-declaration time (the Stage 2 flip); the nested-class probe
	// below needs the schema, which the node records (§N2). Constraint
	// kinds whose membership runs through the node's Behavior (dep
	// scalars, predicates, disjuncts) keep the NODE — UnifyExplainR
	// consults the Behavior — so only class-content nodes resolve.
	if IsBareTypeNode(constraint) {
		if body, ok := constraint.TypeBody(); ok && IsClassType(body) {
			constraint = body
		}
	}

	// Concrete default — the field's type is the default value's own
	// type (its Parent), so {x:1} accepts Integers and rejects the
	// rest, a default carrying a refined type enforces it, and a
	// const-singleton default admits exactly its inhabitant. The
	// membership question routes through v.Is(t) — the one-predicate
	// rule — so the type's Behavior decides: DefaultBehavior is plain
	// lattice conformance, bareRefineUnifier stays nominal, and a
	// const singleton's Behavior matches by value equality.
	if constraint.Data != nil && !IsTypeBody(constraint) {
		if val.Is(constraint.Parent) {
			return val, nil
		}
		return Value{}, fmt.Errorf("expected %s (the default's type), got %s (%s)",
			constraint.Parent.Name(), val.Parent.Name(), val.String())
	}

	// Class-typed field ({i:Inner}) — nominal check: the value must be
	// an instance of that class (or a subclass, via the lattice).
	if IsClassType(constraint) {
		info, _ := AsClassType(constraint)
		if IsClassInstance(val) && info.Type != nil && val.Parent.ConformsTo(info.Type) {
			return val, nil
		}
		return Value{}, fmt.Errorf("expected a %s instance, got %s (%s)",
			info.Name, val.Parent.Name(), val.String())
	}

	// Type constraint — bare node, predicate type, disjunction.
	// UnifyExplainR runs predicates and reports the reason on failure.
	unified, uerr := UnifyExplainR(constraint, val, r)
	if uerr != nil {
		return Value{}, fmt.Errorf("expected %s, got %s (%s): %s",
			constraint.String(), val.Parent.Name(), val.String(), uerr.Error())
	}
	return unified, nil
}

// makePathon creates a Pathon value from a source: a string ("a/b") or a
// list of segments (["a" "b"]). Slashes are normalised — every "/"
// separates segments and empty segments (from a "//" run, or a
// leading/trailing "/") are dropped, so "a//b" and ["a/" "b"] both
// yield ["a" "b"]. A leading "/" on the source — the string, or the
// first list element — marks the path absolute; the abs argument
// (from a `{ abs:… }` option map) forces it absolute regardless.
func MakePathon(srcVal Value, abs bool) ([]Value, error) {
	switch {
	case srcVal.Parent.ConformsTo(TString) && srcVal.Data != nil:
		// The string form recognises a Windows drive ("C:\..." / "C:/...")
		// in addition to POSIX; a driveless path parses POSIX-style.
		s, _ := AsString(srcVal)
		info := parsePathonString(s)
		if abs { // an { abs:true } option forces it absolute regardless
			info.Abs = true
		}
		return []Value{NewValueRaw(TPathon, PathonPayload{Info: info})}, nil
	case srcVal.Parent.ConformsTo(TList) && srcVal.Data != nil:
		// The list form is explicit driveless (POSIX) segments; each element
		// is split on "/", and a leading "/" on the first marks it absolute.
		elems, _ := AsList(srcVal)
		var parts []string
		for i := 0; i < elems.Len(); i++ {
			r := ValToString(elems.Get(i))
			if i == 0 && strings.HasPrefix(r, "/") {
				abs = true
			}
			parts = append(parts, splitPosixSegs(r)...)
		}
		return []Value{NewPathon(parts, abs)}, nil
	default:
		return nil, &BoruError{Code: "type_error", Detail: fmt.Sprintf("make: Pathon source must be a list or string, got %s", srcVal.String())}
	}
}

// NewPathonFromString parses a path string into a Pathon value with the
// same rules as `make Pathon "<s>"` (a Windows drive prefix switches on
// drive parsing; anything else is POSIX). Host code that surfaces OS
// paths as values uses this — the boru:io watch events in particular,
// whose records must carry Pathon microns, never bare strings.
func NewPathonFromString(s string) Value {
	return NewValueRaw(TPathon, PathonPayload{Info: parsePathonString(s)})
}

// isPathonSep reports whether c is a Pathon path separator. Both the POSIX
// "/" and the Windows "\" separate segments, so a Windows path spelled
// with either slash parses the same way.
func isPathonSep(c byte) bool { return c == '/' || c == '\\' }

// splitPathonSegs splits s on either separator, dropping empty segments
// (so "a//b", "a\\b", and "a/b\\" all yield ["a", "b"]). Used for the
// segments of a Windows DRIVE path, where "\" is a separator.
func splitPathonSegs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '\\' })
}

// splitPosixSegs splits s on "/" only, dropping empty segments. Used for a
// driveless path, where "\" is an ordinary character — so POSIX paths (the
// only kind the cross-engine differential exercises) parse exactly as they
// did before Windows-drive support: "//a//b//" → ["a","b"], not a UNC root.
func splitPosixSegs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '/' })
}

// isDriveLetter reports whether c is an ASCII drive letter (a-z / A-Z).
func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// parsePathonString parses a path string into a PathonInfo. A Windows DRIVE
// prefix ("C:", optionally followed by a separator) switches on Windows
// parsing: either slash separates segments and the drive is recorded as the
// volume (rendered "C:\a\b", or "C:a\b" when drive-relative). Any other
// string stays POSIX: "/" separates segments, "\" is an ordinary character,
// and the path is absolute iff it begins with "/". (UNC "\\server\share" is
// not special-cased — forward-slash "//x" is a POSIX path, not a volume.)
func parsePathonString(s string) PathonInfo {
	if len(s) >= 2 && isDriveLetter(s[0]) && s[1] == ':' {
		rest := s[2:]
		return PathonInfo{
			Volume: s[:2],
			Abs:    len(rest) > 0 && isPathonSep(rest[0]),
			Parts:  splitPathonSegs(rest),
		}
	}
	return PathonInfo{
		Abs:   strings.HasPrefix(s, "/"),
		Parts: splitPosixSegs(s),
	}
}

// MakeHandler is the position-agnostic 2-arg make dispatcher.
func MakeHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	if !isTypeLike(targetVal) && isTypeLike(srcVal) {
		targetVal, srcVal = srcVal, targetVal
	}

	targetVal = ResolveTypeLiteralDef(targetVal, reg)

	// A generic SCHEMA as the make target — `make Box {value:42}` —
	// infers its type arguments from the construction body and
	// instantiates first (design/legacy/GENERICS.10.ignore Phase 7 / D12); the
	// instantiation then takes the ordinary path below. Uninferable,
	// undefaulted parameters error (unbound_param) — never silent Any.
	if IsTypeSchema(targetVal) {
		inst, err := InferAndInstantiateSchema(reg, targetVal, srcVal)
		if err != nil {
			return nil, err
		}
		return MakeHandler([]Value{inst, srcVal}, nil, nil, reg)
	}

	// The target type's own constructor, ahead of the Ideal registry
	// (NUR056) — the same precedence MakeObjHandler applies, on the
	// generic path a `make` reaches when no arity-specific sig matched.
	if out, ok, err := TryMake(reg, targetVal, srcVal); ok {
		return out, err
	}

	// Structural kinds (object / record / table) instantiate through
	// the Ideal registry — see ideal.go and design/IDEAL.10.md.
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
		if m := reg.Ideals.Match(targetVal); m != nil && !m.available() {
			return nil, fmt.Errorf("make: the %s type-kind is not available in this registry", m.Name)
		}
	}

	// (The former 2-arg Pathon special case is gone — the Micron
	// Ideal's Instantiate, dispatched above, owns the whole family.)

	if targetVal.Equal(TOptions) && IsBareTypeNode(targetVal) {
		if !srcVal.Parent.Equal(TMap) || !IsConcrete(srcVal) {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		src, err := AsMutableMap(srcVal)
		if err != nil {
			return nil, fmt.Errorf("make: Options requires a concrete map")
		}
		// Resolve field constraints through the canonical cascade —
		// bare type names and `?:`-desugared disjuncts arrive as
		// Words post-opacity (ADR-012 rule 4).
		resolved := NewOrderedMap()
		for _, key := range src.Keys() {
			fv, _ := src.Get(key)
			resolved.Set(key, ResolveFieldType(reg, fv))
		}
		return []Value{NewOptionsType(resolved)}, nil
	}

	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: first argument must be a type literal or record type, got %s", targetVal.String())
	}

	targetType := &targetVal
	if srcVal.Parent.ConformsTo(targetType) {
		return []Value{srcVal}, nil
	}

	// A user-minted scalar refinement (def Foo refine Integer) is a
	// nominal newtype, and `make Foo v` is its constructor: cast v to
	// the BASE type if needed, then tag the result with the refinement
	// — the same reparent the typed-def path (`def x:Foo v`) performs.
	// Without this, make silently returned a base-tagged value
	// (design/legacy/CLASS-OBJECT.10.ignore §3c typed-defaults gap 1), so
	// `(make Foo 1) is Foo` was false and a Foo-typed schema default
	// could not be expressed.
	if canon := CanonicalType(reg, targetType); reg != nil && canon != nil && canon.Origin == OriginUserDef {
		targetType = canon
		base := builtinBaseOf(targetType)
		if base != nil && base.ConformsTo(TScalar) {
			conv := srcVal
			if !srcVal.Parent.ConformsTo(base) {
				c, cerr := MakeConvert(srcVal, base)
				if cerr != nil {
					return nil, cerr
				}
				conv = c
			}
			return []Value{ReparentValue(conv, targetType)}, nil
		}
	}

	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// builtinBaseOf walks a user-minted type's parent chain to the first
// builtin ancestor — the conversion base for `make <Refinement> v`.
func builtinBaseOf(t *Type) *Type {
	for p := t.Parent; p != nil; p = p.Parent {
		if p.Origin == OriginBuiltin {
			return p
		}
	}
	return nil
}

// MakeTable instantiates a table value — a list of record-conforming
// rows — from a table type and a list of row data. Each row may be
// positional or named. Backs the Table Ideal's Instantiate.
func MakeTable(tt TableTypeInfo, srcVal Value) ([]Value, error) {
	return MakeTableR(tt, srcVal, nil)
}

// MakeTableR is MakeTable with Registry threading for predicate-typed
// field constraints in table rows. See MakeFieldValueR.
func MakeTableR(tt TableTypeInfo, srcVal Value, r *Registry) ([]Value, error) {
	recType := tt.Record
	if !srcVal.Parent.ConformsTo(TList) {
		return nil, fmt.Errorf("make: table values must be a list of row lists, got %s", srcVal.String())
	}
	if !IsConcrete(srcVal) {
		return nil, fmt.Errorf("make: table values must be a concrete list, got type literal")
	}
	rows, _ := AsList(srcVal)
	fieldKeys := recType.Fields.Keys()
	resultRows := make([]Value, 0, rows.Len())

	for rowIdx, rowVal := range rows.Slice() {
		if !rowVal.Parent.ConformsTo(TList) {
			return nil, fmt.Errorf("make: table row %d must be a list, got %s", rowIdx, rowVal.String())
		}
		if !IsConcrete(rowVal) {
			return nil, fmt.Errorf("make: table row %d must be a concrete list, got type literal", rowIdx)
		}
		rowElems, _ := AsList(rowVal)

		isNamed := rowElems.Len() > 0 && rowElems.Get(0).Parent.ConformsTo(TMap)
		if isNamed {
			if _, err := AsMutableMap(rowElems.Get(0)); err != nil {
				isNamed = false
			}
		}

		result := NewOrderedMap()
		if isNamed {
			provided := NewOrderedMap()
			for _, elem := range rowElems.Slice() {
				if !elem.Parent.ConformsTo(TMap) {
					return nil, fmt.Errorf("make: table row %d: mixed named and positional fields", rowIdx)
				}
				m, err := AsMutableMap(elem)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: expected concrete map pair, got %s", rowIdx, elem.String())
				}
				for _, key := range m.Keys() {
					val, _ := m.Get(key)
					provided.Set(key, val)
				}
			}
			for _, key := range fieldKeys {
				val, ok := provided.Get(key)
				if !ok {
					return nil, fmt.Errorf("make: table row %d: missing field %q", rowIdx, key)
				}
				constraint, _ := recType.Fields.Get(key)
				converted, err := MakeFieldValueR(val, constraint, r)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
				}
				result.Set(key, converted)
			}
			for _, key := range provided.Keys() {
				if _, ok := recType.Fields.Get(key); !ok {
					return nil, fmt.Errorf("make: table row %d: unknown field %q", rowIdx, key)
				}
			}
		} else {
			if rowElems.Len() != len(fieldKeys) {
				return nil, fmt.Errorf("make: table row %d: expected %d values, got %d",
					rowIdx, len(fieldKeys), rowElems.Len())
			}
			for i, key := range fieldKeys {
				constraint, _ := recType.Fields.Get(key)
				converted, err := MakeFieldValueR(rowElems.Get(i), constraint, r)
				if err != nil {
					return nil, fmt.Errorf("make: table row %d: field %q: %w", rowIdx, key, err)
				}
				result.Set(key, converted)
			}
		}

		resultRows = append(resultRows, NewMap(result))
	}

	return []Value{NewList(resultRows)}, nil
}

// registerKernelIdeals installs the kernel type-kind descriptors with
// their dispatch predicate (Accepts) and value constructor
// (Instantiate). The type-level constructor (Ideal.Construct) is
// filled in by the language layer's installIdeals — type construction
// reuses the surface-registered object/record handlers. Called from
// NewRegistry so every Registry, including the bare eng spec runner,
// can `make` the structural kinds.
func registerKernelIdeals(r *Registry) {
	// The "Object" Ideal instantiates class types (ObjectType payloads,
	// minted under Ideal/Class). The bare Object lattice type was removed;
	// this kind is keyed purely on the ObjectType payload now.
	r.Ideals.Register(&Ideal{
		Name:    "Object",
		Enabled: true,
		Accepts: func(v Value) bool {
			return IsClassType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			objType, err := AsClassType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a class type, got %s", typ.String())
			}
			return makeObject(objType, data, r)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Resource",
		Enabled: true,
		Accepts: func(v Value) bool {
			return IsResourceType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			resType, err := AsResourceType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a Resource/Entity type, got %s", typ.String())
			}
			if !data.Parent.ConformsTo(TMap) {
				return nil, fmt.Errorf("make: resource values must be a map, got %s", data.String())
			}
			provided, err := AsMutableMap(data)
			if err != nil {
				return nil, fmt.Errorf("make: expected concrete map, got %s", data.String())
			}
			return makeResource(resType, provided, r)
		},
	})
	r.Ideals.Register(&Ideal{
		Name:    "Record",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (IsBareTypeNode(v) && v.Equal(TRecord)) || IsRecordType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			recType, err := AsRecordType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed record type, got %s", typ.String())
			}
			return MakeRecordR(recType, data, false, r)
		},
	})
	// The "Micron" Ideal moved to the family's content layer
	// (basic/go/micron.go — InstallMicronIdeals registers the full
	// descriptor per registry), so the kernel no longer names the
	// structured-scalar family here; MakeScalarHandler reaches it
	// through the generic Ideal dispatch below.
	r.Ideals.Register(&Ideal{
		Name:    "Table",
		Enabled: true,
		Accepts: func(v Value) bool {
			return (IsBareTypeNode(v) && v.Equal(TTable)) || IsTableType(v)
		},
		Instantiate: func(typ, data Value, r *Registry) ([]Value, error) {
			tt, err := AsTableType(typ)
			if err != nil {
				return nil, fmt.Errorf("make: expected a constructed table type, got %s", typ.String())
			}
			return MakeTableR(tt, data, r)
		},
	})
}

// MakeWithOpts is the 3-arg make-with-options dispatcher.
func MakeWithOpts(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	var targetVal, srcVal, optsVal Value
	for _, a := range args {
		resolved := ResolveTypeLiteralDef(a, reg)
		switch {
		case isTypeLike(resolved) && targetVal.Parent.Equal(nil):
			targetVal = resolved
		default:
			if srcVal.Parent.Equal(nil) {
				srcVal = a
			} else {
				optsVal = a
			}
		}
	}
	if optsVal.Parent.ConformsTo(TList) && srcVal.Parent.ConformsTo(TMap) && srcVal.Data != nil {
		srcVal, optsVal = optsVal, srcVal
	}

	useBase, err := parseMakeOptions(optsVal)
	if err != nil {
		return nil, err
	}

	// The target type's own constructor, here too (NUR056). The options
	// form reaches the structural branches below DIRECTLY — it does not
	// route through MakeHandler — so without this arm `make T data` used
	// the custom constructor while `make T data opts` silently used the
	// kernel's, which is precisely the kind of same-word-two-answers split
	// the record was opened against.
	//
	// The options themselves are not forwarded: they configure the KERNEL's
	// construction (`base`, Pathon's `abs`), and a type that supplies its
	// own constructor is not running that code. A Maker that needs options
	// takes them from its source.
	if out, ok, err := TryMake(reg, targetVal, srcVal); ok {
		return out, err
	}

	if IsClassType(targetVal) {
		objType, _ := AsClassType(targetVal)
		return makeObject(objType, srcVal, reg)
	}

	if IsRecordType(targetVal) {
		recType, _ := AsRecordType(targetVal)
		return MakeRecordR(recType, srcVal, useBase, reg)
	}

	if IsBareTypeNode(targetVal) && targetVal.Equal(TPathon) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.Parent.ConformsTo(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return MakePathon(srcVal, abs)
	}

	// Pass reg through so the Ideal-registry dispatch in MakeHandler
	// can reach the structural kinds (e.g. a table target with opts).
	return MakeHandler([]Value{srcVal, targetVal}, nil, nil, reg)
}

// MakeScalarHandler converts a scalar value to a target scalar type.
func MakeScalarHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	// A NAMED structural target evaluates to its node (the Stage 2
	// flip); the Ideal dispatch and conversions below operate on the
	// recorded content, exactly as MakeHandler resolves through
	// ResolveTypeLiteralDef. Nominal nodes (builtins, refines) have no
	// content and pass through for the newtype arm below.
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	// The target type's own constructor outranks every kernel path
	// (NUR056) — including the user-refinement newtype arm below, which
	// rewrites the target to its builtin BASE before reaching
	// MakeConvert, and so would hide a Maker installed on the refinement.
	if out, ok, err := TryMake(reg, targetVal, srcVal); ok {
		return out, err
	}
	// A scalar target claimed by a registered Ideal kind (the Micron
	// family — whose resolved def body is a MicronTypeInfo, not a bare
	// literal — and its newtypes) constructs through that Ideal's
	// Instantiate: the same routine the generic make path runs, so the
	// scalar sig path and the generic make path agree. The content
	// layer registers the family descriptor (InstallMicronIdeals).
	if reg != nil {
		if id := reg.Ideals.For(targetVal); id != nil && id.Instantiate != nil {
			return id.Instantiate(targetVal, srcVal, reg)
		}
	}
	if targetVal.Data != nil {
		return nil, fmt.Errorf("make: expected a type literal, got %s", targetVal.String())
	}
	targetType := &targetVal
	if srcVal.Parent.ConformsTo(targetType) {
		return []Value{srcVal}, nil
	}
	// A user-minted scalar refinement (def Foo refine Integer) is a
	// nominal newtype, and `make Foo v` is its constructor: cast v to
	// the BASE type if needed, then tag the result with the refinement
	// — the same reparent the typed-def path (`def x:Foo v`) performs.
	// Without this, make silently returned a base-tagged value, so
	// `(make Foo 1) is Foo` was false and a Foo-typed schema default
	// could not be expressed (design/legacy/CLASS-OBJECT.10.ignore §3c gap 1).
	if canon := CanonicalType(reg, targetType); reg != nil && canon != nil && canon.Origin == OriginUserDef {
		if base := builtinBaseOf(canon); base != nil && base.ConformsTo(TScalar) {
			conv := srcVal
			if !srcVal.Parent.ConformsTo(base) {
				c, cerr := MakeConvert(srcVal, base)
				if cerr != nil {
					return nil, cerr
				}
				conv = c
			}
			return []Value{ReparentValue(conv, canon)}, nil
		}
	}
	result, err := MakeConvert(srcVal, targetType)
	if err != nil {
		return nil, err
	}
	return []Value{result}, nil
}

// MakeObjHandler is the 2-arg [IdealType, Map] make handler. It
// instantiates object types and Ideal-kind types; a non-object
// IdealType target (e.g. Options) defers to the generic make
// dispatcher.
func MakeObjHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, srcVal := args[0], args[1]
	targetVal = ResolveTypeLiteralDef(targetVal, reg)
	// Ahead of the Ideals REGISTRY (NUR056): the registry is the Go-side
	// construction hook, and the capability is its boru-side twin — a type
	// that says how it is built outranks the kind-level default.
	if out, ok, err := TryMake(reg, targetVal, srcVal); ok {
		return out, err
	}
	if reg != nil {
		if ideal := reg.Ideals.For(targetVal); ideal != nil && ideal.Instantiate != nil {
			return ideal.Instantiate(targetVal, srcVal, reg)
		}
		if m := reg.Ideals.Match(targetVal); m != nil && !m.available() {
			return nil, fmt.Errorf("make: the %s type-kind is not available in this registry", m.Name)
		}
	}
	if IsClassType(targetVal) {
		objType, _ := AsClassType(targetVal)
		return makeObject(objType, srcVal, reg)
	}
	// Not an object type and unclaimed by an Ideal kind (e.g.
	// Options) — defer to the generic make dispatcher.
	return MakeHandler([]Value{targetVal, srcVal}, nil, nil, reg)
}

// MakeScalarOptsHandler is the 3-arg [ScalarType, Map, Any] make handler.
func MakeScalarOptsHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	targetVal, optsVal, srcVal := args[0], args[1], args[2]
	if IsBareTypeNode(targetVal) && targetVal.Equal(TPathon) {
		abs := false
		if optsMap, _ := AsMap(optsVal); optsMap != nil {
			if v, ok := optsMap.Get("abs"); ok && v.Parent.ConformsTo(TBoolean) {
				abs, _ = AsBoolean(v)
			}
		}
		return MakePathon(srcVal, abs)
	}
	// Thread the registry through: the scalar handler's Ideal dispatch
	// (the Micron family) needs it — a nil registry would silently drop
	// the family for the 3-arg opts form.
	return MakeScalarHandler([]Value{targetVal, srcVal}, nil, nil, reg)
}

// MakeConvert converts a source value to a target scalar type.
// Exported so production lang and downstream tooling can reuse the
// same scalar-coercion logic that backs `make`.
func MakeConvert(src Value, targetType *Type) (Value, error) {
	// The target type's own constructor first (NUR056). This is the arm
	// that serves the callers reaching the shared scalar coercion
	// directly — record-field coercion via MakeFieldValue — since the
	// `make` handlers consult TryMake before they get here.
	if out, ok, err := makerCapability(targetType, src); ok {
		return out, err
	}
	switch {
	case targetType.ConformsTo(TString):
		return NewString(ValToString(src)), nil

	case targetType.ConformsTo(TFloat):
		text := ValToString(src)
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return Value{}, &BoruError{Code: "type_error", Detail: fmt.Sprintf("make: cannot convert %q to float", text)}
		}
		return NewFloat(f), nil

	case targetType.ConformsTo(TNumber) || targetType.ConformsTo(TInteger):
		text := ValToString(src)
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(text, 64)
			if ferr != nil {
				return Value{}, &BoruError{Code: "type_error", Detail: fmt.Sprintf("make: cannot convert %q to number", text)}
			}
			return NewInteger(int64(f)), nil
		}
		return NewInteger(n), nil

	case targetType.ConformsTo(TBoolean):
		// Boolean is a COERCION, not a parse — identical to the truthiness
		// rule `if` and friends apply (CoerceBoolean), and identical to the
		// `convert` word: a value is false iff it is absent/empty (empty
		// String, 0, none, empty collection) and true otherwise. String
		// CONTENT is never inspected, so "false"/"true" are ordinary
		// non-empty strings that both coerce to true. Parsing the literal
		// words "true"/"false" into booleans is a separate operation that
		// lives elsewhere, not in make/field coercion.
		return NewBoolean(CoerceBoolean(src)), nil

	case targetType.Equal(TAtom):
		return NewAtom(ValToString(src)), nil

	default:
		return Value{}, &BoruError{Code: "unsupported", Detail: fmt.Sprintf("make: unsupported target type %s", targetType)}
	}
}

// MakeFieldValue converts a value to match a record field's type
// constraint. Exported for the same reason as MakeConvert — keeps
// the production lang's record-make path on the engine's canonical
// implementation.
//
// Backward-compat wrapper that delegates to MakeFieldValueR with a
// nil Registry — sufficient for non-predicate constraints (type
// literals, structural records, scalars). For predicate-type field
// constraints use MakeFieldValueR to thread a Registry so the
// predicate body can run.
func MakeFieldValue(val Value, constraint Value) (Value, error) {
	return MakeFieldValueR(val, constraint, nil)
}

// MakeFieldValueR is MakeFieldValue with Registry threading so
// predicate-type field constraints (`def Rec refine Record [x:Pos]`
// where Pos is a predicate fn type) can run the predicate body via
// RunPredicate. When the constraint is an FnDef/Function value (a
// predicate), the candidate is gated by the predicate's input type
// and admitted only if the predicate accepts.
func MakeFieldValueR(val Value, constraint Value, r *Registry) (Value, error) {
	val = ResolveWordValue(val)

	if IsBareTypeNode(constraint) {
		constraintType := ValueType(constraint)
		// A constraint node whose kind enforces membership through a
		// Unify capability (predicate, dependent scalar, binding-body —
		// the evaluated NAME of such a type, post the Stage 2 flip)
		// runs its rule: nominal conformance below would admit a base
		// value without ever running the constraint, and MakeConvert
		// would launder it into the newtype.
		if HasConstraintUnify(constraintType) {
			out, uerr := UnifyExplainR(constraint, val, r)
			if uerr != nil {
				return Value{}, fmt.Errorf("%s", uerr.Error())
			}
			return out, nil
		}
		if val.Parent.ConformsTo(constraintType) {
			return val, nil
		}
		// A numeric field must not launder a non-numeric source into a
		// number by stringify-then-reparse: `make Point ["1" "2"]` for a
		// [x:Number y:Number] record used to yield {x:1 y:2}, and a Boolean
		// source produced the confusing `cannot convert "true" to number`
		// (WAT-AUDIT.5.md §V). A numeric field now admits only a numeric
		// source — Integer↔Float widening still coerces via MakeConvert, but
		// a String/Boolean/Atom is rejected. (Explicit scalar `make Number
		// "99"` is a different, deliberately lenient path and is unaffected.)
		if constraintType.ConformsTo(TNumber) && !val.Parent.ConformsTo(TNumber) {
			return Value{}, fmt.Errorf(
				"field expects %s but %s is not a number",
				constraintType.Leaf(), val.String())
		}
		return MakeConvert(val, constraintType)
	}

	// Predicate-type constraint (and disjunct-with-predicate) — route
	// through UnifyExplainR so the predicate body runs via RunPredicate
	// instead of failing with "incompatible types" when Unify tries to
	// match an FnDef against a scalar.
	unified, uerr := UnifyExplainR(constraint, val, r)
	if uerr != nil {
		return Value{}, fmt.Errorf("value %s does not match constraint %s: %s",
			val.String(), constraint.String(), uerr.Error())
	}
	return unified, nil
}

// ResolveFieldType resolves a record field's type constraint value.
//
// Three resolution strategies:
//  1. String matching a user-defined type name in DefStacks → replaced
//     with the defined type value (e.g., disjunctions by name).
//  2. Concrete list → evaluated as code in a sub-engine so that
//     expressions like [string or none] produce a disjunction.
//  3. Everything else passes through unchanged.
func ResolveFieldType(r *Registry, v Value) Value {
	if v.Data != nil && (v.Parent.ConformsTo(TString) || v.Parent.ConformsTo(TAtom) || IsWord(v)) {
		var name string
		if IsWord(v) {
			_as2, _ := AsWord(v)
			name = _as2.Name
		} else {
			name, _ = AsString(v)
		}
		// Only CAPITALISED names are type references (the type-name
		// convention). Without this gate, a lowercase string default
		// that happens to spell a registered word name resolved to
		// that word's FnDef — `class {op:"add"}` seeded the field
		// with add's function value instead of the string "add"
		// (IsTypeBody admits FnDef values because predicate types are
		// fn-bodied).
		if !IsCapitalisedName(name) {
			return v
		}
		if tv, ok := r.TopTypeBody(name); ok {
			if IsTypeBody(tv) {
				return tv
			}
		}
		if top, ok := r.Defs.Top(name); ok {
			if IsTypeBody(top) {
				return top
			}
		}
		// Builtin arm of the canonical cascade — WORDS only: a bare
		// `{a:Integer}` field arrives as a Word post-opacity (ADR-012
		// rule 4), while a QUOTED capitalised string default
		// (`{unit:"Integer"}`) must stay the string it always was.
		if IsWord(v) {
			if t, ok := ResolveBuiltinTypeName(name); ok {
				return NewTypeLiteral(t)
			}
		}
		return v
	}

	// A disjunct or typed-container constraint (`{y?:String}`'s
	// `(String tor None tor Absent)`, `{items:[:Integer]}`) carries its
	// type names one level down — resolve them through the registry-
	// armed deep resolver, which also re-simplifies resolved disjuncts
	// to `tor`'s canonical order.
	if IsDisjunct(v) || IsTypedList(v) || IsTypedMap(v) {
		return ResolveWordsDeepR(v, r)
	}

	if v.Parent.Equal(TList) && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		input := make([]Value, elems.Len())
		for i, e := range elems.Slice() {
			if (e.Parent.ConformsTo(TString) || e.Parent.ConformsTo(TAtom)) && e.Data != nil {
				name, _ := AsString(e)
				if r.Lookup(name) != nil {
					input[i] = NewWord(name)
					continue
				}
			}
			input[i] = e
		}
		sub := New(r)
		results, err := sub.Run(input)
		if err == nil && len(results) == 1 {
			return results[0]
		}
		return v
	}

	return v
}
