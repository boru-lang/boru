package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// The XSLT-style rule-dispatch vocabulary for boru:fmt. Together with the
// document algebra (Fmt.render, see fmtdoc.go) these two PURE words let a
// formatter be written as a declarative boru rule table keyed by node kind —
// the "template rules dispatched by node kind, apply recursion, document
// output" model the design concludes is the natural boru expression of
// formatting (design/legacy/fmt-module-and-xslt.0.ignore, "The XSLT investigation").
//
// The two words are deliberately pure value→value transforms: they classify
// a node and expose its children, leaving DISPATCH and RECURSION to ordinary
// boru (fetch the rule fn from the table by kind, apply it, recurse on
// children). So the rule engine itself is boru — no fn-invocation or
// registry-threading in Go — and the whole loop reads like an XSLT stylesheet:
//
//	import "boru:fmt"
//	def rules {
//	  map:    (n => {fmt:'group' body:{fmt:'concat' parts:(Fmt.children n each apply)}})
//	  entry:  (n => {fmt:'concat' parts:[n.key ': ' (apply n.value)]})
//	  list:   (n => {fmt:'group' body:{fmt:'concat' parts:(Fmt.children n each apply)}})
//	  scalar: (n => (canon n))
//	}
//	def apply fn [node:Any Any [(rules get (Fmt.kind node)) node]]
//	Fmt.render 72 (apply {a:1 b:[2 3]})
//
// A rule table is just a Map from a kind atom to a rule fn; `apply` is the
// four-line driver (fetch-by-kind then apply). Fmt.kind supplies the dispatch
// key (an XSLT match pattern), Fmt.children the child sequence
// (apply-templates' node list).

// fmtRuleNatives are the two rule-vocabulary words. Both are pure and
// all-forward eligible (BarrierPos -1) so the infix form `node Fmt.kind`
// dispatches like the other module wrappers (lang/go/CLAUDE.md, "Module
// FnDef Wrappers").
var fmtRuleNatives = []native.NativeFunc{
	{
		Name: "kind",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Impl:       native.Go(fmtKindHandler),
			Returns:    []*native.Type{native.TAtom},
			BarrierPos: -1,
		}},
	},
	{
		Name: "children",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Impl:       native.Go(fmtChildrenHandler),
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
		}},
	},
}

// fmtKindHandler implements Fmt.kind: classify a node into the atom a rule
// table dispatches on. A tagged Map (one carrying a `$kind` String/Atom
// field) reports that tag — the analogue of an XSLT element name — so a rule
// set can name its own node kinds (`entry`, `header`, …); an untagged Map is
// `map`, a List is `list`, and everything else (scalars, and non-structural
// values like a bare type literal) is `scalar`.
func fmtKindHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	node := args[0]
	if m, err := native.AsMap(node); err == nil {
		if tag, ok := kindTag(m); ok {
			return []native.Value{native.NewAtom(tag)}, nil
		}
		return []native.Value{native.NewAtom("map")}, nil
	}
	if _, err := native.AsList(node); err == nil {
		return []native.Value{native.NewAtom("list")}, nil
	}
	return []native.Value{native.NewAtom("scalar")}, nil
}

// kindTag reads a Map's `$kind` dispatch tag; false when the key is absent or
// not textual. AsConcreteString accepts both a String and an Atom tag (it
// returns an atom's text), so a rule set may name kinds either way — `entry`
// and 'entry' tag alike — while a non-textual value (a number, a list) is no
// tag and the Map falls back to the structural `map` kind.
func kindTag(m native.ReadMap) (string, bool) {
	v, ok := m.Get("$kind")
	if !ok {
		return "", false
	}
	if s, err := v.AsConcreteString(); err == nil {
		return s, true
	}
	return "", false
}

// fmtChildrenHandler implements Fmt.children: the child sequence a rule
// recurses over (XSLT's apply-templates node list). A Map yields one `entry`
// node per key ({$kind:'entry' key:<k> value:<v>} — so an `entry` rule can
// lay out `key: value` and recurse on the value), skipping the reserved
// `$kind` tag; a List yields its elements verbatim; any other value (a
// scalar leaf) yields the empty list.
func fmtChildrenHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	node := args[0]
	if m, err := native.AsMap(node); err == nil {
		var out []native.Value
		for _, k := range m.Keys() {
			if k == "$kind" {
				continue
			}
			cv, _ := m.Get(k)
			entry := native.NewOrderedMap()
			entry.Set("$kind", native.NewAtom("entry"))
			entry.Set("key", native.NewString(k))
			entry.Set("value", cv)
			out = append(out, native.NewMap(entry))
		}
		return []native.Value{native.NewList(out)}, nil
	}
	if lst, err := native.AsList(node); err == nil {
		out := make([]native.Value, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			out[i] = lst.Get(i)
		}
		return []native.Value{native.NewList(out)}, nil
	}
	return []native.Value{native.NewList(nil)}, nil
}
