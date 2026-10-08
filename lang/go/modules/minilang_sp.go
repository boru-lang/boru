package modules

import (
	"github.com/antchfx/xpath"
	"github.com/boru-lang/boru/lang/go/native"
)

// The `sp` (structure-path) mini-language: query boru native structure (Maps
// and Lists) with XPath-style paths (github.com/antchfx/xpath). Where `xp`
// runs XPath over a Node/Xml
// document, `sp` runs the SAME XPath engine over ordinary boru data, so the
// full axis / predicate / function vocabulary (//, [pred], count(), sum(),
// string(), contains(), position(), …) reaches into maps and lists — the
// path-shaped query layer the XSLT-style rule engine wants (see
// design/legacy/fmt-module-and-xslt.0.ignore). The document is the stack subject and
// the result is a List, the same shape as `xp` / `jp` / `jq`:
//
//	{a:1 b:2} mini sp '/a'                       # → [1]
//	{books:[{title:'A'} {title:'B'}]} mini sp '//title'   # → ['A' 'B']
//	{items:[{p:10} {p:30}]} mini sp '//item[p>20]/p'      # → [30]
//	{a:1 b:2 c:3} mini sp 'count(/*)'            # → [3]
//
// Structure → tree model: a Map is an element whose children are its
// entries (each named by its key); a List is elements named `item`, one per
// element, so positional predicates work (`/item[2]`); a scalar is the text
// of its key element. The subject's entries hang directly off the document
// root, so `/key` addresses a top-level entry and `//key` any descendant.
//
// A matched element comes back as its SOURCE boru value (a nested map/list or
// a scalar), so results compose with the rest of boru; a text() or scalar
// XPath result comes back as a String / Number / Boolean, exactly like `xp`.
// The compiled-expression memo (miniCompiledXPath) is shared with `xp`.

// buildSpTree mirrors a boru Map/List/scalar subject into the navigable
// xpNode tree the antchfx cursor walks. The subject's own entries become
// the document root's children (no wrapper element), so an absolute `/key`
// addresses a top-level entry.
func buildSpTree(v native.Value) *xpNode {
	root := &xpNode{typ: xpath.RootNode}
	container := buildSpElement("", v)
	root.children = container.children
	for _, c := range root.children {
		c.parent = root
	}
	return root
}

// buildSpElement mirrors one boru value into an element node named name. A
// Map recurses into key-named element children; a List into repeated `item`
// element children; any other value becomes a single text child (its scalar
// rendering). Each element keeps a back-reference to its source boru value so
// a match converts straight back.
func buildSpElement(name string, v native.Value) *xpNode {
	n := &xpNode{typ: xpath.ElementNode, name: name, boru: v}
	if m, err := native.AsMap(v); err == nil {
		for _, k := range m.Keys() {
			cv, _ := m.Get(k)
			spAppend(n, buildSpElement(k, cv))
		}
		return n
	}
	if lst, err := native.AsList(v); err == nil {
		for i := 0; i < lst.Len(); i++ {
			spAppend(n, buildSpElement("item", lst.Get(i)))
		}
		return n
	}
	spAppend(n, &xpNode{typ: xpath.TextNode, text: xpText(v)})
	return n
}

// spAppend links child as the last child of parent, wiring the parent and
// prev/next sibling pointers the navigator relies on.
func spAppend(parent, child *xpNode) {
	child.parent = parent
	if k := len(parent.children); k > 0 {
		prev := parent.children[k-1]
		prev.next = child
		child.prev = prev
	}
	parent.children = append(parent.children, child)
}

// miniSpHandler implements `sp`: compile the XPath src (args[0]), evaluate it
// over the stack Map/List subject (args[2]), and return the result as a List.
func miniSpHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", "sp: src: "+err.Error(), "lang_sp")
	}
	doc := args[2]
	_, mapErr := native.AsMap(doc)
	_, listErr := native.AsList(doc)
	if mapErr != nil && listErr != nil {
		return nil, r.BoruErrorHint("mini_error",
			"sp: expected a Map or List, got "+doc.Parent.String(), "lang_sp",
			"the sp document is a boru structure — a Map or a List")
	}
	expr, perr := miniCompiledXPath(src)
	if perr != nil {
		return nil, r.BoruErrorHint("mini_parse_error", "sp: "+perr.Error(), "lang_sp",
			"write an XPath like /name, //title or count(//item)")
	}
	result := expr.Evaluate(newXpNav(buildSpTree(doc)))
	return []native.Value{native.NewList(spResultToList(result))}, nil
}

// spResultToList projects an antchfx XPath result into boru values: a node-set
// yields one value per matched node (an element as its source boru value, a
// text node as its String), a scalar (boolean/number/string) a single-element
// list.
func spResultToList(result any) []native.Value {
	switch v := result.(type) {
	case *xpath.NodeIterator:
		var out []native.Value
		for v.MoveNext() {
			out = append(out, spNodeToValue(v.Current()))
		}
		return out
	case bool:
		return []native.Value{native.NewBoolean(v)}
	case float64:
		return []native.Value{xpNumberToValue(v)}
	case string:
		return []native.Value{native.NewString(v)}
	default: //covergate:allow module provably-invariant / grammar-defensive guard (§modules)
		return nil
	}
}

// spNodeToValue converts one matched node back to a boru value: an element to
// its source value (map/list/scalar), a text node to its String value.
func spNodeToValue(nav xpath.NodeNavigator) native.Value {
	if n, ok := nav.(*xpNav); ok && n.attr == -1 && n.cur.typ == xpath.ElementNode {
		return n.cur.boru
	}
	return native.NewString(nav.Value())
}
