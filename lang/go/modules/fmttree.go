package modules

import (
	"github.com/boru-lang/boru/lang/go/formatter"
	"github.com/boru-lang/boru/lang/go/native"
)

// fmtTreeNative implements Fmt.tree: parse a boru SOURCE string into its
// layout CST as a boru value tree, so a formatter can be written as
// declarative boru rules over it. Each node is a `$kind`-tagged Map —
// `{$kind:<word|list|map|paren|comment|…> text:<src> children:[…]}` — the
// shape Fmt.kind dispatches on (it reads the `$kind` tag) and a rule set
// recurses over via `node.children`.
//
// This is the bridge for the Phase-3 "layout rules expressed as boru"
// direction (design/legacy/fmt-module-and-xslt.0.ignore): `Fmt.tree` supplies the tree,
// `Fmt.render` the document-algebra output side, and a boru rule table keyed
// by `Fmt.kind` the layout. The tree already carries the emitter's
// semantics-preserving transforms (type capitalisation, fn-bracket elision),
// so a rule set only decides layout — emitting this tree the built-in way
// reproduces `Fmt.format` exactly.
func fmtTreeNative() native.NativeFunc {
	return native.NativeFunc{
		Name: "tree",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString},
			Impl:       native.Go(fmtTreeHandler),
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
		}},
	}
}

// fmtTreeHandler parses the source string argument and returns its layout CST
// as a boru value tree, rejecting a non-concrete argument (a type literal /
// dependent-type constraint) rather than panicking.
func fmtTreeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, err
	}
	return []native.Value{nodeToValue(formatter.ParseTree(src))}, nil
}

// nodeToValue converts one formatter layout node (and its subtree) into the
// `$kind`-tagged Map a boru rule set walks. Every node carries all three
// keys — `$kind`, `text`, `children` — so a rule can read any of them
// without an existence check (a leaf's `children` is the empty list, a
// container's `text` the empty string).
func nodeToValue(n *formatter.Node) native.Value {
	m := native.NewOrderedMap()
	m.Set("$kind", native.NewAtom(formatter.NodeKindName(n.Kind)))
	m.Set("text", native.NewString(n.Text))
	kids := make([]native.Value, len(n.Children))
	for i, c := range n.Children {
		kids[i] = nodeToValue(c)
	}
	m.Set("children", native.NewList(kids))
	return native.NewMap(m)
}
