package native

// Member enumeration for editor tooling (LSP member completion): given the
// static type of a receiver — the token to the left of a `.` — list the
// dot-accessible members that may follow. This is the data behind
// `RECV.<TAB>` completion in the language server.
//
// The module-namespace case (`Rand.` → its exports) lives in the LSP layer
// because lang/go/modules imports this package; here we cover the type-shaped
// members: Micron properties, class/record/table/resource fields, and
// user-defined Micron kinds.

import (
	core "github.com/boru-lang/boru/core/go"
)

// Member is one dot-accessible member of a value or type.
type Member struct {
	Name   string // the identifier to insert after the dot
	Detail string // its type (or a short descriptor), for the completion detail
	Kind   string // "property" | "field"
}

// MembersOfType returns the dot-accessible members of a value whose static
// type is typeName (as reported by lang.CheckResult.Stack), resolving
// user-defined named types through r. It returns nil when the type carries no
// statically-known members — e.g. a bare Map (whose keys are recovered
// separately by the caller) or an unknown name.
func MembersOfType(typeName string, r *Registry) []Member {
	if typeName == "" {
		return nil
	}
	// 1. A builtin Micron leaf — a fixed property table.
	if ms, ok := micronLeafMembers[typeName]; ok {
		return append([]Member(nil), ms...)
	}
	// 2. A user-defined named type (class / record / table / user Micron
	//    kind, including a newtype of one): resolve its bound body and read
	//    its field schema. A non-schema body (alias, enum, bare newtype of a
	//    builtin) yields no members.
	if r != nil {
		if body, ok := r.TopTypeBody(typeName); ok {
			return membersOfTypeBody(body)
		}
	}
	return nil
}

// membersOfTypeBody reads the field schema off a bound type-body value. Only
// the schema-bearing Ideals are covered — Class, Record, Table and user Micron
// kinds; dynamic-key kinds (Store, Resource, Options) carry no static member
// list and fall to the caller's map-key recovery instead.
func membersOfTypeBody(body Value) []Member {
	switch d := DataOf(&body).(type) {
	case core.ClassTypeInfo:
		return fieldsToMembers(d.AllFields(), "field")
	case core.RecordTypeInfo:
		return fieldsToMembers(d.Fields, "field")
	case core.TableTypeInfo:
		return fieldsToMembers(d.Record.Fields, "field")
	case core.MicronTypeInfo:
		return fieldsToMembers(d.Fields, "property")
	}
	return nil
}

// fieldsToMembers converts a field/property schema map to members, rendering
// each entry's value as the member's type name for the completion detail.
func fieldsToMembers(m *core.OrderedMap, kind string) []Member {
	if m == nil {
		return nil
	}
	out := make([]Member, 0, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out = append(out, Member{Name: k, Detail: TypeNameOf(v), Kind: kind})
	}
	return out
}

// prop is a shorthand for a Micron property entry in the table below.
func prop(name, typ string) Member { return Member{Name: name, Detail: typ, Kind: "property"} }

// micronLeafMembers is the property table for the twelve builtin Scalar/Micron
// leaves — stored primary fields plus derived properties — mirroring
// eng.MicronProperty / native.getMicronReturns (a sync test in members_test.go
// keeps them from drifting). Keyed by the leaf's short type name, which is what
// the checker's residual stack reports.
var micronLeafMembers = map[string][]Member{
	"Pathon": {
		prop("parts", "List"), prop("abs", "Boolean"), prop("volume", "String"),
	},
	"Emailon": {
		prop("user", "String"), prop("host", "String"), prop("address", "String"),
	},
	"Urlon": {
		prop("scheme", "String"), prop("host", "String"), prop("port", "Integer"),
		prop("path", "String"), prop("query", "String"), prop("fragment", "String"),
		prop("href", "String"),
	},
	"Ipon": {
		prop("addr", "String"), prop("version", "Integer"),
	},
	"Hoston": {
		prop("host", "String"), prop("port", "Integer"), prop("authority", "String"),
	},
	"Semveron": {
		prop("major", "Integer"), prop("minor", "Integer"), prop("patch", "Integer"),
		prop("prerelease", "String"), prop("build", "String"),
		prop("prereleaseParts", "List"), prop("buildParts", "List"),
		prop("release", "String"), prop("stable", "Boolean"), prop("version", "String"),
	},
	"Cidron": {
		prop("cidr", "String"), prop("addr", "String"), prop("prefix", "Integer"),
		prop("version", "Integer"), prop("hostbits", "Integer"), prop("count", "Integer"),
	},
	"Macon": {
		prop("addr", "String"), prop("bits", "Integer"), prop("eui64", "Boolean"),
		prop("oui", "String"), prop("multicast", "Boolean"), prop("local", "Boolean"),
		prop("broadcast", "Boolean"),
	},
	"Coloron": {
		prop("r", "Integer"), prop("g", "Integer"), prop("b", "Integer"), prop("a", "Integer"),
		prop("hex", "String"), prop("opaque", "Boolean"), prop("alpha", "Float"), prop("css", "String"),
	},
	"Mimon": {
		prop("type", "String"), prop("subtype", "String"), prop("params", "Map"),
		prop("essence", "String"), prop("mime", "String"), prop("suffix", "String"), prop("facet", "String"),
	},
	"Qion": {
		prop("code", "String"), prop("amount", "BigDecimal"), prop("minor", "Integer"),
		prop("units", "Integer"), prop("negative", "Boolean"), prop("display", "String"),
	},
	"Phonon": {
		prop("digits", "String"), prop("e164", "String"), prop("length", "Integer"),
	},
}

// MicronLeafNames returns the builtin Micron leaf type names that have a
// property table (used by the sync test and available to tooling).
func MicronLeafNames() []string {
	out := make([]string, 0, len(micronLeafMembers))
	for k := range micronLeafMembers {
		out = append(out, k)
	}
	return out
}
