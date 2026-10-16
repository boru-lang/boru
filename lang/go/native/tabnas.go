package native

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	parser "github.com/boru-lang/boru/parser/go"
	tabnascsv "github.com/tabnas/csv/go"
	tabnasfeed "github.com/tabnas/feed/go"
	tabnasini "github.com/tabnas/ini/go"
	tabnasjson "github.com/tabnas/json/go"
	tabnasjson5 "github.com/tabnas/json5/go"
	tabnasjsonc "github.com/tabnas/jsonc/go"
	tabnasjsonic "github.com/tabnas/jsonic/go"
	tabnasmarkdown "github.com/tabnas/markdown/go"
	tabnastoml "github.com/tabnas/toml/go"
	tabnasxml "github.com/tabnas/xml/go"
	tabnasyaml "github.com/tabnas/yaml/go"
	tabnaszon "github.com/tabnas/zon/go"
)

// The tabnas decoder core — the single source of truth for the tabnas
// parser family (github.com/tabnas/*). It is consumed by BOTH the `read`
// word (via DefaultFormats / TabnasFormat in format.go) and the `parse`
// word (the boru:parselang module builds its built-in kinds from
// TabnasKinds). It lives in native — below modules — because the import
// direction is eng < native < modules, so a modules-level definition
// could never be shared down into native's read pipeline.

// TabnasParser is the shape of a tabnas decoder: source text + the
// caller's `parse`/`read` opts → generic Go data (map[string]any / []any
// / scalars), or a parse error.
type TabnasParser func(src string, opts map[string]any) (any, error)

// TabnasKind describes one tabnas decoder: its parse fn, the Value
// converter (generic vs xml), and its declared output type. Both
// surfaces consume this descriptor — read wraps it as a TabnasFormat,
// parse wraps it as a ParseLang (the ParseLangSpec handler).
type TabnasKind struct {
	// Name is the format / parser-kind atom ("ini", "xml", …).
	Name string
	// Returns is the declared output type (TMap/TList/TXml/TAny).
	Returns *Type
	// Parse runs the decoder over the source string with the given opts.
	Parse TabnasParser
	// Convert maps the decoder's generic output to a boru Value —
	// tabnasAnyToValue for most kinds, tabnasXmlToValue for xml.
	Convert func(any) Value
}

// TabnasKinds returns the parser kinds backed by the tabnas parser family.
// The slice order is significant — boru:parselang's ParseLang.kinds is
// pinned to it (lang/spec/module-parselang.tsv §3), so new kinds append at
// the end rather than insert mid-list.
//
// Shapes: the JSON family (json / jsonic / json5 / jsonc) plus yaml / zon
// yield whatever their top level denotes (Map, List or scalar); csv yields
// a List of rows; ini / toml / xml / feed / markdown yield a Map (xml a
// dedicated Node/Xml value, markdown an mdast-adjacent AST document).
//
// Opts: the middle `parse <kind> <opts> <source>` map (and read's
// non-reserved options) is forwarded to the jsonic-plugin kinds (json5 /
// jsonc / csv / xml / markdown / feed), whose option model IS a generic
// map — e.g. `parse csv {field:{separation:';'}}`. The standalone-Parse
// kinds (json / jsonic / yaml / toml / zon / ini) take typed (not map)
// options upstream, so they ignore opts for now.
func TabnasKinds() []TabnasKind {
	// jsonicPlugin adapts a tabnas jsonic plugin (json5, csv, xml, …) — which
	// installs its grammar onto a parser instance from a map of options — into
	// a TabnasParser, merging the caller's opts over the kind's defaults.
	jsonicPlugin := func(defaults map[string]any, install func(*tabnasjsonic.Jsonic, map[string]any) error) TabnasParser {
		return func(src string, opts map[string]any) (any, error) {
			j := parser.SafeMake()
			if err := install(j, mergeOpts(defaults, opts)); err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return nil, err
			}
			return j.Parse(src)
		}
	}
	ignoreOpts := func(parse func(string) (any, error)) TabnasParser {
		return func(src string, _ map[string]any) (any, error) { return parse(src) }
	}
	defs := []struct {
		name    string
		returns *Type
		parse   TabnasParser
	}{
		// INI: [section] headers nest a Map; booleans decode to Booleans.
		{"ini", TMap, ignoreOpts(func(s string) (any, error) { return tabnasini.Parse(s) })},
		// JSON family — top level may be an object, array or scalar.
		{"json", TAny, ignoreOpts(func(s string) (any, error) { return tabnasjson.Parse(s) })},
		{"jsonic", TAny, ignoreOpts(func(s string) (any, error) { return parser.SafeParse(s) })},
		{"json5", TAny, jsonicPlugin(tabnasjson5.Defaults(), tabnasjson5.Json5)},
		{"jsonc", TAny, jsonicPlugin(nil, tabnasjsonc.Jsonc)},
		// CSV: a List of rows, each row a List of field strings.
		{"csv", TList, jsonicPlugin(nil, tabnascsv.Csv)},
		{"toml", TMap, ignoreOpts(func(s string) (any, error) { return tabnastoml.Parse(s) })},
		{"yaml", TAny, ignoreOpts(func(s string) (any, error) { return tabnasyaml.Parse(s) })},
		// XML: a Node/Xml element tree — the SAME value an embedded
		// `<tag>…</tag>` literal produces, so a parsed document and a
		// source literal are interchangeable. See design/XML-LITERAL.0.md §5.6.
		{"xml", TXml, jsonicPlugin(nil, tabnasxml.Xml)},
		{"zon", TAny, ignoreOpts(func(s string) (any, error) { return tabnaszon.Parse(s) })},
		// Markdown: the mdast-adjacent AST the plugin documents — a
		// `{type:'document' children:[…]}` Map whose nodes carry `type` plus
		// their own fields (`depth` on a heading, `value` on text, GFM
		// `table`/`tableRow`/`tableCell` with an `align` list). Walk it by
		// key, not by row index.
		{"markdown", TMap, func(s string, opts map[string]any) (any, error) {
			j := parser.SafeMake()
			if err := j.UseDefaults(tabnasmarkdown.Markdown, tabnasmarkdown.Defaults,
				mergeOpts(nil, opts)); err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return nil, err
			}
			return j.Parse(s)
		}},
		// Feed (RSS/Atom): normalised to a Map. The plugin returns a typed
		// AtomFeed struct, so a JSON round-trip flattens it to generic data.
		{"feed", TMap, func(s string, opts map[string]any) (any, error) {
			j := parser.SafeMake()
			if err := tabnasfeed.Feed(j, mergeOpts(tabnasfeed.Defaults, opts)); err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return nil, err
			}
			v, err := j.Parse(s)
			if err != nil {
				return nil, err
			}
			return jsonRoundTrip(v)
		}},
	}
	kinds := make([]TabnasKind, len(defs))
	for i, d := range defs {
		// XML decodes to a dedicated Node/Xml value (not a generic Map) so
		// `parse xml` and an embedded `<…>` literal yield the same shape;
		// every other kind uses the generic any→Value conversion.
		convert := tabnasAnyToValue
		if d.name == "xml" {
			convert = tabnasXmlToValue
		}
		kinds[i] = TabnasKind{
			Name:    d.name,
			Returns: d.returns,
			Parse:   d.parse,
			Convert: convert,
		}
	}
	return kinds
}

// OptsToMap converts a parser/read options value (the middle `parse <kind>
// <opts> <source>` map, or read's non-reserved options, `{}` when none) to
// a generic map[string]any the tabnas plugins understand. A non-Map /
// empty / non-concrete value yields nil, so a kind's defaults pass through
// mergeOpts untouched.
func OptsToMap(v Value) map[string]any {
	if !ParentOf(&v).ConformsTo(TMap) || !IsConcrete(&v) {
		return nil
	}
	m, ok := ValueToAny(v).(map[string]any)
	if !ok { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil
	}
	return m
}

// mergeOpts returns a fresh map of base overlaid with over — the caller's
// `parse`/`read` opts win over a kind's defaults, leaving both inputs
// untouched. Either may be nil.
func mergeOpts(base, over map[string]any) map[string]any {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// jsonRoundTrip flattens an arbitrary Go value (e.g. a typed feed struct) to
// generic data — map[string]any / []any / scalars — via encoding/json, so the
// shared converter can handle it uniformly.
func jsonRoundTrip(v any) (any, error) {
	b, err := jsonMarshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ansiEscape matches the SGR colour escapes the tabnas parsers embed in their
// diagnostics. The pattern is a compile-time constant, so MustCompile is safe.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// FirstCleanLine returns the first line of a parser diagnostic with colour
// escapes stripped — the tabnas/jsonic parsers emit richly-formatted
// multi-line errors; the boru error keeps just the headline.
func FirstCleanLine(msg string) string {
	msg = ansiEscape.ReplaceAllString(msg, "")
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}

// tabnasXmlToValue maps the tabnas XML decoder's output to a Node/Xml
// element value, the same shape an embedded `<tag>…</tag>` literal
// produces. The decoder yields `{name, localName, attributes, children}`
// where children interleaves text strings and nested element maps; this
// walks that tree into NewXmlElement(tag, attr, cren). A non-element top
// level (defensive) falls back to the generic conversion.
func tabnasXmlToValue(v any) Value {
	m, ok := v.(map[string]any)
	if !ok {
		return tabnasAnyToValue(v)
	}
	tag, _ := m["name"].(string)
	attr := NewOrderedMap()
	if am, ok := m["attributes"].(map[string]any); ok {
		keys := make([]string, 0, len(am))
		for k := range am {
			keys = append(keys, k)
		}
		sort.Strings(keys) // tabnas hands back an unordered map; sort for determinism
		for _, k := range keys {
			attr.Set(k, NewString(fmt.Sprintf("%v", am[k])))
		}
	}
	var cren []Value
	if ch, ok := m["children"].([]any); ok {
		for _, c := range ch {
			switch cc := c.(type) {
			case string:
				cren = append(cren, NewString(cc))
			case map[string]any:
				cren = append(cren, tabnasXmlToValue(cc))
			default:
				cren = append(cren, NewString(fmt.Sprintf("%v", cc)))
			}
		}
	}
	return NewXmlElement(tag, attr, cren)
}

// tabnasMapToValue converts a decoded map to a boru Map, ordering keys
// deterministically (the parsers hand back unordered Go maps).
func tabnasMapToValue(m map[string]any) Value {
	om := NewOrderedMap()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		om.Set(k, tabnasAnyToValue(m[k]))
	}
	return NewMap(om)
}

// AnyToValue maps a single piece of generic decoded Go data (strings,
// booleans, numbers, nested map[string]any / []any) to a boru Value, with
// deterministic key ordering for maps. It is the shared decoder-output
// converter used by the tabnas formats and reused by other modules (e.g.
// boru:query's jsonpath/jq results). An unrecognised shape degrades to its
// string form rather than erroring.
func AnyToValue(v any) Value { return tabnasAnyToValue(v) }

// tabnasAnyToValue maps a single decoded value to a boru Value. The parsers
// emit strings, booleans, numbers, nested maps and lists; an unrecognised
// shape degrades to its string form rather than erroring.
func tabnasAnyToValue(v any) Value {
	switch val := v.(type) {
	case Value:
		// A node a custom-parser semantic action already built as a boru
		// Value (see boru:parse): pass it through untouched so user data types
		// survive to the parse result rather than being flattened.
		return val
	case nil:
		return NewTypeLiteral(TNone)
	case bool:
		return NewBoolean(val)
	case string:
		return NewString(val)
	case float64:
		// A whole-valued number stays an Integer, but ONLY when it fits in
		// int64 — an out-of-range float (e.g. 1e20 from a JSON/YAML source)
		// would convert implementation-dependently, so keep it a Float.
		if val == math.Trunc(val) && val >= math.MinInt64 && val < math.MaxInt64 {
			return NewInteger(int64(val))
		}
		return NewFloat(val)
	case int:
		return NewInteger(int64(val))
	case int64:
		return NewInteger(val)
	case []any:
		elems := make([]Value, len(val))
		for i, item := range val {
			elems[i] = tabnasAnyToValue(item)
		}
		return NewList(elems)
	case *tabnasjsonic.OrderedMap:
		return tabnasOrderedMapToValue(val)
	case map[string]any:
		return tabnasMapToValue(val)
	default:
		return NewString(fmt.Sprintf("%v", val))
	}
}

// tabnasOrderedMapToValue converts the parser's insertion-ordered OrderedMap
// to a boru Map, preserving the source key order (the whole point of the
// ordered parser node) rather than sorting like the legacy plain-map path.
func tabnasOrderedMapToValue(src *tabnasjsonic.OrderedMap) Value {
	om := NewOrderedMap()
	for _, k := range src.Keys {
		om.Set(k, tabnasAnyToValue(src.Vals[k]))
	}
	return NewMap(om)
}
