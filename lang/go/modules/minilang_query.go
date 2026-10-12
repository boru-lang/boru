package modules

import (
	"math/big"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/itchyny/gojq"
	"github.com/ohler55/ojg/jp"
)

// The `jp` and `jq` mini-languages: query a document with JSONPath
// (github.com/ohler55/ojg) or a jq filter (github.com/itchyny/gojq). The
// document is the stack subject and may be any boru data shape — a Node (Map /
// List tree), an Object, an Array, a Table, or a Record — converted to generic
// data for the query engine and the matches converted back to boru values.
//
//	{store:{book:[{title:'A'} {title:'B'}]}} mini jp '$.store.book[*].title'  # → ['A' 'B']
//	{a:1 b:2} mini jq '.a + .b'                                              # → [3]
//
// Both return a LIST of results: JSONPath yields the matched nodes, jq yields
// its output stream (a single-output filter therefore returns a one-element
// list). Grab the first with `.0` / `get 0` when a scalar is wanted.

// docToAny converts a boru subject to the generic Go data (map[string]any /
// []any / scalars) the query engines operate on. Nodes (Map/List), Objects and
// Records go through the standard value→any path; Arrays, Tables and other
// Ideals are projected through their IdealConverter.
func docToAny(v native.Value) any {
	if !native.IsConcrete(&v) {
		return nil
	}
	switch {
	case v.Parent.ConformsTo(native.TMap):
		m, err := native.AsMap(v)
		if err != nil || m == nil { //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
			return native.ValueToAny(v)
		}
		out := make(map[string]any, m.Len())
		for _, k := range m.Keys() {
			vv, _ := m.Get(k)
			out[k] = docToAny(vv)
		}
		return out
	case v.Parent.ConformsTo(native.TList):
		// Plain List, or a Table (a TList value carrying TableData that AsList
		// can't read — project its rows via the Ideal converter).
		if lst, err := native.AsList(v); err == nil {
			return sliceToAny(lst.Slice())
		}
		return idealListToAny(v)
	case native.IsClassInstance(v):
		return native.ValueToAny(v) // already a recursive field map
	default:
		// Other Ideals (Store, …) project to a map; everything else (scalars)
		// falls through to the standard conversion.
		if mv, err := core.ConvertIdealToMap(v); err == nil {
			if m, e2 := native.AsMap(mv); e2 == nil && m != nil && m.Len() > 0 {
				out := make(map[string]any, m.Len())
				for _, k := range m.Keys() {
					vv, _ := m.Get(k)
					out[k] = docToAny(vv)
				}
				return out
			}
		}
		return native.ValueToAny(v)
	}
}

// idealListToAny projects an Ideal (Array, Table, …) to a []any via its
// IdealConverter list projection.
func idealListToAny(v native.Value) any {
	lv, err := core.ConvertIdealToList(v)
	if err != nil {
		return native.ValueToAny(v)
	}
	lst, err := native.AsList(lv)
	if err != nil {
		return native.ValueToAny(v)
	}
	return sliceToAny(lst.Slice())
}

func sliceToAny(elems []native.Value) []any {
	out := make([]any, len(elems))
	for i, e := range elems {
		out[i] = docToAny(e)
	}
	return out
}

// queryResultToValue converts a jsonpath / jq output value back to a boru
// Value, reusing the shared any→Value conversion and handling gojq's big.Int.
func queryResultToValue(v any) native.Value {
	if bi, ok := v.(*big.Int); ok {
		if bi.IsInt64() {
			return native.NewInteger(bi.Int64())
		}
		f, _ := new(big.Float).SetInt(bi).Float64()
		return native.NewFloat(f)
	}
	return native.AnyToValue(v)
}

// miniJsonPathHandler implements `jp`: run a JSONPath query (args[0]) over the
// stack subject (args[2]) and return the matched nodes as a List.
func miniJsonPathHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", "jp: src: "+err.Error(), "lang_jp")
	}
	expr, perr := jp.ParseString(src)
	if perr != nil {
		return nil, r.BoruErrorHint("mini_parse_error", "jp: "+perr.Error(), "lang_jp",
			"write a JSONPath like $.store.book[*].title")
	}
	matches := expr.Get(docToAny(args[2]))
	out := make([]native.Value, len(matches))
	for i, m := range matches {
		out[i] = queryResultToValue(m)
	}
	return []native.Value{native.NewList(out)}, nil
}

// miniJqHandler implements `jq`: run a jq filter (args[0]) over the stack
// subject (args[2]) and return its output stream as a List.
func miniJqHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", "jq: src: "+err.Error(), "lang_jq")
	}
	query, perr := gojq.Parse(src)
	if perr != nil {
		return nil, r.BoruErrorHint("mini_parse_error", "jq: "+perr.Error(), "lang_jq",
			"write a jq filter like .store.book[].title")
	}
	var out []native.Value
	iter := query.Run(docToAny(args[2]))
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, ok := v.(error); ok {
			return nil, r.BoruErrorHint("mini_eval_error", "jq: "+e.Error(), "lang_jq",
				"check the filter and that it matches the document shape")
		}
		out = append(out, queryResultToValue(v))
	}
	return []native.Value{native.NewList(out)}, nil
}
