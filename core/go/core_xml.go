package core

import "strings"

// xmlBehavior is the TypeBehavior for Node/Xml. It renders an element
// back to well-formed XML (Format), and compares two elements
// structurally (Equal). Match stays nominal — DefaultBehavior delegates
// to ConformsTo, so a Node/Xml value matches the Xml / Node / Any slots.
//
// Node/Xml is parser-emitted (embedded `<tag>…</tag>` literals), so the
// type itself is kernel-declared in builtinDecls; this file only
// attaches the Behavior, mirroring coretype_list_map_behaviors.go.
// See design/XML-LITERAL.0.md.
type xmlBehavior struct{}

func (xmlBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (xmlBehavior) matchesAsDefault()           {}

func (xmlBehavior) Format(v Value) string {
	tag, attr, cren, ok := xmlParts(v)
	if !ok {
		// Bare type literal (Data == nil) or unexpected payload.
		return "Xml"
	}
	var b strings.Builder
	formatXmlInto(&b, tag, attr, cren)
	return b.String()
}

func (xmlBehavior) Equal(a, b Value) bool {
	_, _, _, okA := xmlParts(a)
	_, _, _, okB := xmlParts(b)
	if okA != okB {
		return false
	}
	if !okA {
		// Both bare type literals: equal iff same lattice node.
		return DefaultBehavior.Equal(a, b)
	}
	return xmlElementsEqual(a, b)
}

// xmlParts extracts the (tag, attr, cren) view of an XML element value,
// from either the immutable XmlElementPayload or the mutable
// *FlexXmlData. ok is false for a bare type literal or non-XML value, so
// Format / Equal / the conversion walks treat both element shapes
// uniformly. See design/XML-LITERAL.0.md §5.
func xmlParts(v Value) (tag string, attr *OrderedMap, cren []Value, ok bool) {
	switch x := DataOf(&v).(type) {
	case XmlElementPayload:
		return x.Tag, x.Attr, x.Cren, true
	case *FlexXmlData:
		return x.Tag, x.Attr, x.Cren, true
	case *WeakFlexXmlData:
		// Swept strong snapshot of the children (see AsMap's weak arm).
		return x.Tag, x.Attr, x.snapshotCren(), true
	}
	return "", nil, nil, false
}

// IsXmlValue reports whether v is a concrete XML element of either kind
// (immutable Node/Xml or mutable Node/Xml/FlexXml).
func IsXmlValue(v Value) bool {
	_, _, _, ok := xmlParts(v)
	return ok
}

// XmlParts is the exported (tag, attr, cren) view of an XML element, for
// the lang-layer accessor words (elem / text / xml-attr / get). ok is
// false for a bare type literal or non-XML value.
func XmlParts(v Value) (tag string, attr *OrderedMap, cren []Value, ok bool) {
	return xmlParts(v)
}

// formatXmlInto serialises one element. Empty-cren elements self-close
// (`<tag/>`); otherwise `<tag attrs>children</tag>`. Attribute values
// and text are entity-escaped. Child elements of either kind recurse.
func formatXmlInto(b *strings.Builder, tag string, attr *OrderedMap, cren []Value) {
	b.WriteByte('<')
	b.WriteString(tag)
	if attr != nil {
		for _, k := range attr.Keys() {
			val, _ := attr.Get(k)
			b.WriteByte(' ')
			b.WriteString(k)
			b.WriteString(`="`)
			b.WriteString(escapeXmlAttr(xmlChildText(val)))
			b.WriteByte('"')
		}
	}
	if len(cren) == 0 {
		b.WriteString("/>")
		return
	}
	b.WriteByte('>')
	for _, c := range cren {
		if ctag, cattr, ccren, ok := xmlParts(c); ok {
			formatXmlInto(b, ctag, cattr, ccren)
			continue
		}
		b.WriteString(escapeXmlText(xmlChildText(c)))
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
}

// xmlChildText extracts the raw string of a text/scalar child without
// the quoting that Value.String adds to a String value.
func xmlChildText(v Value) string {
	if sp, ok := DataOf(&v).(StrPayload); ok {
		return sp.S
	}
	return v.String()
}

// xmlElementsEqual compares two XML element values structurally, reading
// each through xmlParts so an immutable Node/Xml and a content-equal
// FlexXml compare equal (flexness is a mutability mode, not identity).
func xmlElementsEqual(a, b Value) bool {
	at, aattr, acren, _ := xmlParts(a)
	bt, battr, bcren, _ := xmlParts(b)
	if at != bt || len(acren) != len(bcren) {
		return false
	}
	if orderedMapLen(aattr) != orderedMapLen(battr) {
		return false
	}
	if aattr != nil {
		for _, k := range aattr.Keys() {
			av, _ := aattr.Get(k)
			bv, ok := battr.Get(k)
			if !ok || xmlChildText(av) != xmlChildText(bv) {
				return false
			}
		}
	}
	for i := range acren {
		ca, cb := acren[i], bcren[i]
		aIsEl, bIsEl := IsXmlValue(ca), IsXmlValue(cb)
		if aIsEl != bIsEl {
			return false
		}
		if aIsEl {
			if !xmlElementsEqual(ca, cb) {
				return false
			}
			continue
		}
		if xmlChildText(ca) != xmlChildText(cb) {
			return false
		}
	}
	return true
}

func orderedMapLen(m *OrderedMap) int {
	if m == nil {
		return 0
	}
	return m.Len()
}

var xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var xmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func escapeXmlText(s string) string { return xmlTextEscaper.Replace(s) }
func escapeXmlAttr(s string) string { return xmlAttrEscaper.Replace(s) }

// IsValidXmlName reports whether s is a usable XML tag / attribute name
// (a name-start char then name chars). Mirrors the literal parser's
// isXmlNameStart / isXmlNameChar so a FlexXml mutation cannot store a key
// that would render as malformed XML (e.g. `set "bad name" …`).
func IsValidXmlName(s string) bool {
	if s == "" {
		return false
	}
	isStart := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
	}
	isChar := func(c byte) bool {
		return isStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == ':'
	}
	if !isStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isChar(s[i]) {
			return false
		}
	}
	return true
}

func init() {
	TXml.ensureTMeta().Behavior = xmlBehavior{}
}
