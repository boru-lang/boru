package modules

import (
	"math"
	"strings"
	"sync"

	"github.com/antchfx/xpath"
	"github.com/boru-lang/boru/lang/go/native"
)

// The `xp` mini-language: query a Node/Xml document with XPath
// (github.com/antchfx/xpath). The document is the stack subject — an
// `<tag>…</tag>` literal value — and the result is returned as a List, the
// same shape as the `jp` / `jq` query kinds:
//
//	<root><a>1</a><a>2</a></root> mini xp '//a'         # → [<a>1</a> <a>2</a>]
//	<r id="7"><a/></r>            mini xp 'string(//@id)' # → ['7']
//	<r><a/><a/></r>              mini xp 'count(//a)'    # → [2]
//
// A node-set result yields the matched nodes — an element node comes back as
// its originating Node/Xml value (so further `xml-elem` / `xml-text` words
// compose), an attribute or text node as its String value. A non-node XPath
// result (count/string/boolean/number functions) returns a one-element List
// carrying the scalar. An integral number coerces to Integer (`count` → 2),
// otherwise Float (`sum` → 5.5).
//
// The immutable boru XML tree carries no parent links and no document root,
// which the antchfx evaluator's cursor model needs, so the handler first
// mirrors the subtree into an xpNode tree (parent/sibling links + a synthetic
// root) and walks it through the xpNav NodeNavigator. Each element node keeps
// a back-reference to its source boru value so a match converts straight back
// without re-rendering.
//
// The mirror is the deliberate adapter, not a stopgap. antchfx clones the
// navigator at essentially every axis step / predicate (Copy()), so the mirror
// trades one O(n) build for O(1) Copy and O(1) parent/sibling hops; a
// path-stack navigator over the boru values in place would make every Copy
// O(depth) (re-deriving siblings from the parent's Cren), i.e. O(nodes·depth)
// overall — slower, not lazier. Adding parent pointers to the kernel
// XmlElementPayload is the wrong direction: Node/Xml is an immutable by-value
// tree with structural sharing, and a child→parent back-pointer would break its
// equality / clone / render contract for one consumer's benefit. What IS worth
// caching is the compiled expression, not the tree — see miniXPathExprs.

// miniXPathExprs memoizes compiled XPath expressions keyed by source, so an
// `xp` call in a loop compiles its expression once per process — the same
// per-src compile memo the `re` kind keeps (miniCompiledPattern). A compiled
// *xpath.Expr is safe to reuse across calls: Evaluate/Select fork their mutable
// iterator state via q.Clone() and never mutate the expression. (The per-call
// xpNode mirror is NOT memoized — the document varies per call, unlike the
// expression.)
var (
	miniXPathMu    sync.Mutex
	miniXPathExprs = map[string]*xpath.Expr{}
)

func miniCompiledXPath(src string) (*xpath.Expr, error) {
	miniXPathMu.Lock()
	defer miniXPathMu.Unlock()
	if e, ok := miniXPathExprs[src]; ok {
		return e, nil
	}
	e, err := xpath.Compile(src)
	if err != nil {
		return nil, err
	}
	miniXPathExprs[src] = e
	return e, nil
}

// xpNode is a navigable mirror of one boru Node/Xml node. Only element and text
// nodes are tree nodes; attributes hang off their element as xpAttr (the
// navigator visits them via MoveToNextAttribute, matching the XPath model).
type xpNode struct {
	typ      xpath.NodeType // RootNode, ElementNode or TextNode
	name     string         // element tag (local name); "" for text/root
	text     string         // text content (TextNode only)
	attrs    []xpAttr       // element attributes, in document order
	children []*xpNode      // element + text children, in document order
	parent   *xpNode
	prev     *xpNode      // previous sibling, nil for the first
	next     *xpNode      // next sibling, nil for the last
	boru     native.Value // the source boru Xml value (ElementNode only)
}

type xpAttr struct {
	name  string
	value string
}

// buildXpTree wraps doc in a synthetic document root (the XPath context node
// `/`) so absolute paths like `/root` resolve.
func buildXpTree(doc native.Value) *xpNode {
	root := &xpNode{typ: xpath.RootNode}
	el := buildXpElement(doc)
	el.parent = root
	root.children = []*xpNode{el}
	return root
}

// buildXpElement mirrors one boru Xml element (and its subtree) into an xpNode.
func buildXpElement(v native.Value) *xpNode {
	tag, attr, cren, _ := native.XmlParts(v)
	n := &xpNode{typ: xpath.ElementNode, name: tag, boru: v}
	if attr != nil {
		for _, k := range attr.Keys() {
			av, _ := attr.Get(k)
			n.attrs = append(n.attrs, xpAttr{name: k, value: xpText(av)})
		}
	}
	var prev *xpNode
	for _, c := range cren {
		var child *xpNode
		if native.IsXmlValue(c) {
			child = buildXpElement(c)
		} else {
			child = &xpNode{typ: xpath.TextNode, text: xpText(c)}
		}
		child.parent = n
		child.prev = prev
		if prev != nil {
			prev.next = child
		}
		prev = child
		n.children = append(n.children, child)
	}
	return n
}

// xpText reads the raw text of a text/attribute child: a String contributes its
// unquoted content, any other scalar its String() rendering (mirroring the
// kernel's xmlChildText).
func xpText(v native.Value) string {
	if s, err := native.AsString(v); err == nil {
		return s
	}
	return v.String()
}

// innerText is an element's XPath string-value: the concatenation of every
// descendant text node, in document order.
func (n *xpNode) innerText() string {
	if n.typ == xpath.TextNode {
		return n.text
	}
	var b strings.Builder
	var walk func(*xpNode)
	walk = func(x *xpNode) {
		if x.typ == xpath.TextNode {
			b.WriteString(x.text)
			return
		}
		for _, c := range x.children {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// xpNav is the antchfx/xpath cursor over an xpNode tree. attr is -1 unless the
// cursor is positioned on one of cur's attributes (index into cur.attrs), the
// standard way the navigator represents attribute nodes without making them
// tree members.
type xpNav struct {
	cur  *xpNode
	root *xpNode
	attr int
}

func newXpNav(root *xpNode) *xpNav { return &xpNav{cur: root, root: root, attr: -1} }

func (x *xpNav) NodeType() xpath.NodeType {
	if x.attr != -1 {
		return xpath.AttributeNode
	}
	return x.cur.typ
}

func (x *xpNav) LocalName() string {
	if x.attr != -1 {
		return x.cur.attrs[x.attr].name
	}
	return x.cur.name
}

func (x *xpNav) Prefix() string { return "" }

func (x *xpNav) Value() string {
	if x.attr != -1 {
		return x.cur.attrs[x.attr].value
	}
	return x.cur.innerText()
}

func (x *xpNav) Copy() xpath.NodeNavigator {
	c := *x
	return &c
}

func (x *xpNav) MoveToRoot() { x.cur, x.attr = x.root, -1 }

func (x *xpNav) MoveToParent() bool {
	if x.attr != -1 {
		x.attr = -1
		return true
	}
	if x.cur.parent == nil {
		return false
	}
	x.cur = x.cur.parent
	return true
}

func (x *xpNav) MoveToNextAttribute() bool {
	if x.attr >= len(x.cur.attrs)-1 {
		return false
	}
	x.attr++
	return true
}

func (x *xpNav) MoveToChild() bool {
	if x.attr != -1 || len(x.cur.children) == 0 {
		return false
	}
	x.cur = x.cur.children[0]
	return true
}

func (x *xpNav) MoveToFirst() bool {
	if x.attr != -1 {
		return false
	}
	for x.cur.prev != nil {
		x.cur = x.cur.prev
	}
	return true
}

func (x *xpNav) MoveToNext() bool {
	if x.attr != -1 || x.cur.next == nil {
		return false
	}
	x.cur = x.cur.next
	return true
}

func (x *xpNav) MoveToPrevious() bool {
	if x.attr != -1 || x.cur.prev == nil {
		return false
	}
	x.cur = x.cur.prev
	return true
}

func (x *xpNav) MoveTo(other xpath.NodeNavigator) bool {
	o, ok := other.(*xpNav)
	if !ok || o.root != x.root {
		return false
	}
	x.cur, x.attr = o.cur, o.attr
	return true
}

// miniXPathHandler implements `xp`: compile the XPath src (args[0]), evaluate it
// over the stack Xml subject (args[2]), and return the result as a List.
func miniXPathHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", "xp: src: "+err.Error(), "lang_xp")
	}
	doc := args[2]
	if !native.IsXmlValue(doc) {
		return nil, r.BoruErrorHint("mini_error",
			"xp: expected an Xml element, got "+doc.Parent().String(), "lang_xp",
			"the xp document must be a Node/Xml value (an <tag>…</tag> literal)")
	}
	expr, perr := miniCompiledXPath(src)
	if perr != nil {
		return nil, r.BoruErrorHint("mini_parse_error", "xp: "+perr.Error(), "lang_xp",
			"write an XPath like //book/title, //@id or count(//item)")
	}
	result := expr.Evaluate(newXpNav(buildXpTree(doc)))
	return []native.Value{native.NewList(xpResultToList(result))}, nil
}

// xpResultToList projects an antchfx XPath result into boru values. A node-set
// yields one value per matched node; a scalar (boolean/number/string) yields a
// single-element list.
func xpResultToList(result any) []native.Value {
	switch v := result.(type) {
	case *xpath.NodeIterator:
		var out []native.Value
		for v.MoveNext() {
			out = append(out, xpNodeToValue(v.Current()))
		}
		return out
	case bool:
		return []native.Value{native.NewBoolean(v)}
	case float64:
		return []native.Value{xpNumberToValue(v)}
	case string:
		return []native.Value{native.NewString(v)}
	default:
		return nil
	}
}

// xpNodeToValue converts one matched node back to a boru value: an element node
// to its source Node/Xml value (so it composes with the xml-* words), an
// attribute or text node to its String value.
func xpNodeToValue(nav xpath.NodeNavigator) native.Value {
	if n, ok := nav.(*xpNav); ok && n.attr == -1 && native.IsXmlValue(n.cur.boru) {
		return n.cur.boru
	}
	return native.NewString(nav.Value())
}

// xpNumberToValue coerces an integral XPath number to Integer (count/position),
// leaving genuine fractions as Float (sum/div).
func xpNumberToValue(f float64) native.Value {
	if !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Trunc(f) {
		return native.NewInteger(int64(f))
	}
	return native.NewFloat(f)
}
