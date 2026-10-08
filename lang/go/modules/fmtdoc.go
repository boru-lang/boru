package modules

import (
	stdfmt "fmt"

	"github.com/boru-lang/boru/lang/go/formatter"
	"github.com/boru-lang/boru/lang/go/native"
)

// renderDocNative implements Fmt.render: lay out a declarative document
// tree, built as boru data, to a target width. It is the runtime entry to
// the Wadler/Prettier document algebra an XSLT-style rule set emits into
// (design/legacy/fmt-module-and-xslt.0.ignore). The doc-tree vocabulary is
// documented on buildDoc. Call form: `Fmt.render <width> <doc>`.
func renderDocNative() native.NativeFunc {
	return native.NativeFunc{
		Name: "render",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TInteger, native.TAny},
			Impl:       native.Go(renderDocHandler),
			Returns:    []*native.Type{native.TString},
			BarrierPos: -1,
		}},
	}
}

func renderDocHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	width, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, err
	}
	doc, err := buildDoc(args[1])
	if err != nil {
		return nil, err
	}
	return []native.Value{native.NewString(formatter.RenderDoc(int(width), doc))}, nil
}

// buildDoc converts a boru document value into a formatter.Doc. A concrete
// string is literal text; a map keyed by `fmt` selects the node kind:
//
//	"hi"                          literal text (bare-string shorthand)
//	{fmt:'text' s:"hi"}           literal text
//	{fmt:'line'}                  soft break (a space when the group is flat)
//	{fmt:'hardline'}              forced break
//	{fmt:'concat' parts:[d …]}    a sequence of docs
//	{fmt:'group' body:d}          lay d out flat if it fits, else broken
//	{fmt:'indent' by:2 body:d}    indent d by `by` columns
func buildDoc(v native.Value) (formatter.Doc, error) {
	if native.IsConcrete(v) && v.Is(native.TString) {
		s, _ := v.AsConcreteString()
		return formatter.DocText{S: s}, nil
	}
	m, merr := native.AsMap(v)
	if merr != nil {
		return nil, stdfmt.Errorf("fmt render: expected a string or a doc map, got %s", v.String())
	}
	kind, ok := docStr(m, "fmt")
	if !ok {
		return nil, stdfmt.Errorf("fmt render: doc map needs a string 'fmt' kind")
	}
	switch kind {
	case "text":
		s, ok := docStr(m, "s")
		if !ok {
			return nil, stdfmt.Errorf("fmt render: text needs a string 's'")
		}
		return formatter.DocText{S: s}, nil
	case "line":
		return formatter.DocLine{}, nil
	case "hardline":
		return formatter.DocLine{Hard: true}, nil
	case "concat":
		lst, ok := docList(m, "parts")
		if !ok {
			return nil, stdfmt.Errorf("fmt render: concat needs a list 'parts'")
		}
		parts := make([]formatter.Doc, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			d, err := buildDoc(lst.Get(i))
			if err != nil {
				return nil, err
			}
			parts[i] = d
		}
		return formatter.DocConcat{Parts: parts}, nil
	case "group":
		body, err := docChild(m, "body")
		if err != nil {
			return nil, err
		}
		return formatter.DocGroup{Body: body}, nil
	case "indent":
		by, ok := docInt(m, "by")
		if !ok {
			return nil, stdfmt.Errorf("fmt render: indent needs an integer 'by'")
		}
		body, err := docChild(m, "body")
		if err != nil {
			return nil, err
		}
		return formatter.DocIndent{By: int(by), Body: body}, nil
	default:
		return nil, stdfmt.Errorf("fmt render: unknown doc kind %q", kind)
	}
}

// docStr reads a concrete-string value at key, false when absent or not a
// string.
func docStr(m native.ReadMap, key string) (string, bool) {
	v, ok := m.Get(key)
	if !ok {
		return "", false
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", false
	}
	return s, true
}

// docInt reads a concrete-integer value at key, false when absent or not
// an integer.
func docInt(m native.ReadMap, key string) (int64, bool) {
	v, ok := m.Get(key)
	if !ok {
		return 0, false
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, false
	}
	return n, true
}

// docList reads a list value at key, false when absent or not a list.
func docList(m native.ReadMap, key string) (native.ReadList, bool) {
	v, ok := m.Get(key)
	if !ok {
		return native.ReadList{}, false
	}
	lst, err := native.AsList(v)
	if err != nil {
		return native.ReadList{}, false
	}
	return lst, true
}

// docChild parses the child doc at key, erroring when the key is absent.
func docChild(m native.ReadMap, key string) (formatter.Doc, error) {
	v, ok := m.Get(key)
	if !ok {
		return nil, stdfmt.Errorf("fmt render: %s needs a 'body'", key)
	}
	return buildDoc(v)
}
