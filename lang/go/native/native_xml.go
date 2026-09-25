package native

import "strings"

// xmlNatives installs the Node/Xml query/accessor words (Increment 5 of
// design/XML-LITERAL.0.md §5.5). The stored fields tag / attr / cren are
// reached by dotted access (`x.tag`) via get (native_storage.go); these
// words are the COMPUTED views. All carry the `xml-` prefix so they never
// shadow the generic `text` / `elem` identifiers programs use freely:
//
//	xml-elem <xml>        — the element children only (DOM `children`):
//	                        cren filtered to Node/Xml elements, dropping
//	                        text nodes. Returns a List.
//	xml-text <xml>        — the concatenated text content of the whole
//	                        subtree (DOM `textContent`). Returns a String.
//	xml-attr <name> <xml> — one attribute's value (a String) or none.
//
// They accept both the immutable Node/Xml and the mutable FlexXml (which
// conforms to it). The CSS-selector surface (`cs/…`) sketched in §4 is a
// separate follow-up — it rides the `cs` mini-language kind.
var xmlNatives = []NativeFunc{
	{
		Name: "xml-elem",
		Signatures: []Signature{{
			Args:    []*Type{TXml},
			Impl:    Go(elemHandler),
			Returns: []*Type{TList}, BarrierPos: -1,
		}},
	},
	{
		Name: "xml-text",
		Signatures: []Signature{{
			Args:    []*Type{TXml},
			Impl:    Go(textHandler),
			Returns: []*Type{TString}, BarrierPos: -1,
		}},
	},
	{
		Name: "xml-attr",
		Signatures: []Signature{
			// An attribute value is always a String (the parser's attr map);
			// a missing attribute reads as None — so the result is the
			// gradual String-or-None union, never bare Any.
			{
				Args:      []*Type{TString, TXml},
				Impl:      Go(xmlAttrHandler),
				Returns:   []*Type{TAny},
				ReturnsFn: xmlAttrReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(xmlAttrHandler),
				Returns:   []*Type{TAny},
				ReturnsFn: xmlAttrReturns, BarrierPos: -1,
				// The handler-contract declaration (design/HANDLER-MIGRATION-
				// LINE.0.md, the quoted class, S2a): the quoted atom is the
				// attribute NAME the handler reads off the element's attr map
				// — a key, like `get`'s; a carrier-delivered key lowers as an
				// ordinary operand and the VM reads what the interpreter reads.
				CompileEffect: CompileQuoteKey,
			},
		},
	},
}

func elemHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_, _, cren, ok := XmlParts(args[0])
	if !ok {
		return nil, r.BoruError("xml_elem_error", "xml-elem: expected an Xml element, got "+args[0].Parent.String(), "xml-elem")
	}
	out := make([]Value, 0, len(cren))
	for _, c := range cren {
		if IsXmlValue(c) {
			out = append(out, c)
		}
	}
	return []Value{NewList(out)}, nil
}

func textHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsXmlValue(args[0]) {
		return nil, r.BoruError("xml_text_error", "xml-text: expected an Xml element, got "+args[0].Parent.String(), "xml-text")
	}
	var b strings.Builder
	collectXmlText(&b, args[0])
	return []Value{NewString(b.String())}, nil
}

// collectXmlText appends the subtree's text content: a text-node child
// contributes its raw string; an element child recurses.
func collectXmlText(b *strings.Builder, v Value) {
	_, _, cren, ok := XmlParts(v)
	if !ok {
		if s, err := AsString(v); err == nil {
			b.WriteString(s)
		}
		return
	}
	for _, c := range cren {
		collectXmlText(b, c)
	}
}

// xmlAttrReturns: dynamic(String tor None) — see the sig comment.
func xmlAttrReturns(_ []Value, _ *Registry) []Value {
	return []Value{NewDynamicCarrierValue(NewDisjunct([]Value{
		NewTypeLiteral(TString), NewTypeLiteral(TNone),
	}))}
}

func xmlAttrHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_, attr, _, ok := XmlParts(args[1])
	if !ok {
		return nil, r.BoruError("xml_attr_error", "xml-attr: expected an Xml element, got "+args[1].Parent.String(), "xml-attr")
	}
	if attr != nil {
		if v, has := attr.Get(getKey(args[0])); has {
			return []Value{v}, nil
		}
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}
