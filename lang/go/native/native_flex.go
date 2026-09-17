package native

import (
	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// flexNatives installs the flex-node words:
//
//	flex <node>        — deep mutable copy: Map→FlexMap, List→FlexList,
//	                     nested Nodes converted recursively. Sugar for
//	                     `make FlexMap m` / `make FlexList l`.
//	node <node>        — the inverse: deep immutable conversion.
//	                     Identity on a node that contains no flex parts.
//	append v  fl       — append one element to a FlexList in place.
//	append [..] fl     — concatenate a list's elements in place (the
//	                     List sig outranks the Any sig, so lists concat
//	                     by default; wrap to append a list AS an
//	                     element: `append [[1 2]] fl`).
//
// Mutating words return the flex node itself so calls chain:
// `append 1 fl` leaves fl on the stack for the next word. Values are
// stored AS GIVEN (by reference, like map values everywhere else) —
// containment is the job of the conversion words, not storage: flex
// an element first if it must be mutable inside the container.
//
// The algorithms (FlexDeepCopy, NodeDeepCopy, MakeNodeHandler) live
// in eng/go/core_flex.go; this file owns the word names and dispatch
// wiring. The in-place `set` sigs live in native_storage.go and the
// flex push/pop/unshift/shift sigs in natives.go (handlers in
// listops.go).
var flexNatives = []NativeFunc{
	{
		Name: "flex",

		Signatures: []Signature{{
			Args:      []*Type{TNode},
			Impl:      Go(flexHandler),
			Returns:   []*Type{TNode},
			ReturnsFn: flexReturns, BarrierPos: -1,
		}},
	},
	{
		Name: "node",

		Signatures: []Signature{{
			Args:      []*Type{TNode},
			Impl:      Go(nodeHandler),
			Returns:   []*Type{TNode},
			ReturnsFn: nodeReturns, BarrierPos: -1,
		}},
	},
	{
		Name: "append",

		Signatures: []Signature{
			// List source: concatenate its elements. More specific
			// than the Any sig, so it wins whenever the argument is a
			// list (including another FlexList, which conforms to List).
			{
				Args:      []*Type{TList, TFlexList},
				Impl:      Go(appendListHandler),
				Returns:   []*Type{TFlexList},
				ReturnsFn: appendListReturns, BarrierPos: -1,
			},
			// Any other value: append as a single element.
			{
				Args:      []*Type{TAny, TFlexList},
				Impl:      Go(appendElemHandler),
				Returns:   []*Type{TFlexList},
				ReturnsFn: flexGrowReturns("append"), BarrierPos: -1,
			},
			// WeakFlexList: append ONE element, classified per the weak
			// value domain (scalar → strong, handle → weak, immutable
			// Node → refused with weak_value_error). No list-splice
			// form: an immutable List argument is exactly the refused
			// kind, and the refusal is the teachable path. The TList
			// twin exists so a list-valued argument (immutable → the
			// refusal; a flex handle → one weak element) reaches THIS
			// classify path instead of the FlexList concatenate sig,
			// which sorts first on position 0 otherwise.
			{
				Args:      []*Type{TList, TWeakFlexList},
				Impl:      Go(appendWeakElemHandler),
				Returns:   []*Type{TWeakFlexList},
				ReturnsFn: weakAppendListReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAny, TWeakFlexList},
				Impl:      Go(appendWeakElemHandler),
				Returns:   []*Type{TWeakFlexList},
				ReturnsFn: weakAppendListReturns, BarrierPos: -1,
			},
			// WeakFlexXml: append one child, same classification.
			{
				Args:      []*Type{TAny, TWeakFlexXml},
				Impl:      Go(appendWeakXmlChildHandler),
				Returns:   []*Type{TWeakFlexXml},
				ReturnsFn: weakAppendXmlReturns, BarrierPos: -1,
			},
			// FlexXml: append child nodes (elements or text) in place.
			// A List splices its elements; any other value is one child.
			{
				Args:    []*Type{TList, TFlexXml},
				Impl:    Go(appendXmlListHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:    []*Type{TAny, TFlexXml},
				Impl:    Go(appendXmlChildHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
		},
	},
}

// flexReturns models the precise FLEX subtype `flex` produces from the input's
// node family (mirroring FlexDeepCopy: Map→FlexMap, List→FlexList, Xml→FlexXml).
// The static `Returns: [TNode]` was the supertype, so a FlexMap/FlexList
// consumer (`set`/`append`/`push`/`sort`/`each` over the result) never matched
// its `Flex*` sig and failed no_signature (flex.tsv). An unknown node shape
// (a dynamic Any / bare Node receiver) stays a DYNAMIC Node so a Flex* slot
// still matches optimistically rather than failing on a strict supertype.
//
// In PLAIN check mode a concrete map source additionally mints the
// container's abstract StoreShapeInfo (design/legacy/checker-precision-fronts.0.ignore
// §2 stage 1 — `flex` is a store-creating word: one shape per creation
// site), so downstream `set`/`get`/`dot` over the result read/write ITS
// key types instead of degrading to dynamic(Any). A flex-of-flex source
// CLONES the source's shape (runtime FlexDeepCopy disconnects aliasing).
// The COMPILE pass narrows through the same shape so downstream dispatch
// commits the right result arity (see getNodeReturns' flex-shape rule);
// the dispatches stay runtime-re-matched polys with an enforced NOut claim.
func flexReturns(args []Value, r *Registry) []Value {
	if len(args) != 1 || args[0].Parent == nil {
		return []Value{NewDynamicCarrier(TNode)}
	}
	shapes := r != nil && r.Check.IsActive()
	// flex only toggles mutability — a {:T}/[:T] source stays typed, so carry the
	// element tag onto the residual (FlexDeepCopy preserves it at runtime). The
	// tag drives the check-mode write mirror (d2CheckWrite reads ElemConstraint),
	// so `append "bad" (flex xs)` for xs:[:Integer] is diagnosed (#6, Codex round 6).
	switch p := args[0].Parent; {
	case p.ConformsTo(TMap):
		if shapes {
			if ss, ok := check.StoreShapeOf(args[0]); ok {
				v := NewCarrier(TFlexMap)
				v.Data = ss.CloneShape()
				return []Value{d2RetainElem(v, args[0])}
			}
			if v, ok := check.MintFlexShapeCarrier(args[0], 0); ok {
				return []Value{d2RetainElem(v, args[0])}
			}
		}
		return []Value{d2RetainElem(NewCarrier(TFlexMap), args[0])}
	case p.ConformsTo(TList):
		return []Value{d2RetainElem(NewCarrier(TFlexList), args[0])}
	case p.ConformsTo(TXml):
		return []Value{NewCarrier(TFlexXml)}
	}
	return []Value{NewDynamicCarrier(TNode)}
}

// nodeReturns is the inverse: `node` deep-copies a flex tree back to a PLAIN
// Node, so the result's family is the input's (FlexMap→Map, FlexList→List,
// FlexXml→Xml). Same dynamic-Node fallback for an unknown shape.
func nodeReturns(args []Value, _ *Registry) []Value {
	if len(args) != 1 || args[0].Parent == nil {
		return []Value{NewDynamicCarrier(TNode)}
	}
	// node just toggles mutability back — a {:T}/[:T] flex stays typed as a plain
	// container, so retain the element tag (NodeDeepCopy preserves it at runtime).
	switch p := args[0].Parent; {
	case p.ConformsTo(TMap):
		return []Value{d2RetainElem(NewCarrier(TMap), args[0])}
	case p.ConformsTo(TList):
		return []Value{d2RetainElem(NewCarrier(TList), args[0])}
	case p.ConformsTo(TXml):
		return []Value{NewCarrier(TXml)}
	}
	return []Value{NewDynamicCarrier(TNode)}
}

func flexHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("flex_error", "flex: expected a concrete map or list, got "+args[0].String(), "flex")
	}
	out, err := core.FlexDeepCopy(args[0])
	if err != nil {
		return nil, r.BoruError("flex_error", err.Error(), "flex")
	}
	return []Value{out}, nil
}

func nodeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("node_error", "node: expected a concrete map or list, got "+args[0].String(), "node")
	}
	out, err := core.NodeDeepCopy(args[0])
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil, r.BoruError("node_error", err.Error(), "node")
	}
	return []Value{out}, nil
}

func appendElemHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a FlexList, got "+args[1].Parent.String(), "append")
	}
	// A typed flex list ([:T]) enforces + recursively re-tags on a grow.
	tagged, werr := d2AdoptTyped(r, args[1], args[0], "append")
	if werr != nil {
		return nil, werr
	}
	// A flex tree stays ENTIRELY mutable: a plain Node element is deep-
	// flexed on the way in (core.AdoptIntoFlex; flex handles share).
	elem, aerr := core.AdoptIntoFlex(tagged)
	if aerr != nil {
		return nil, r.BoruError("append_error", aerr.Error(), "append")
	}
	fd.Elems = append(fd.Elems, elem)
	return []Value{args[1]}, nil
}

// appendWeakElemHandler appends one classified element to a
// WeakFlexList (design/FLEX-ATTRS.1.md §4.4).
func appendWeakElemHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	wd, err := AsWeakFlexList(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a WeakFlexList, got "+args[1].Parent.String(), "append")
	}
	// Typed weak list ([:T]) enforcement on grow, mirroring the flex
	// column — weakness never drops the element contract.
	tagged, werr := d2AdoptTyped(r, args[1], args[0], "append")
	if werr != nil {
		return nil, werr
	}
	if refusal := wd.Append(tagged); refusal != nil {
		return nil, WeakRefusalError(r, "append", "WeakFlexList", refusal)
	}
	return []Value{args[1]}, nil
}

// appendWeakXmlChildHandler appends one classified child to a
// WeakFlexXml element.
func appendWeakXmlChildHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	wd, err := AsWeakFlexXml(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a WeakFlexXml, got "+args[1].Parent.String(), "append")
	}
	if refusal := wd.AppendChild(args[0]); refusal != nil {
		return nil, WeakRefusalError(r, "append", "WeakFlexXml", refusal)
	}
	return []Value{args[1]}, nil
}

func appendXmlChildHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexXml(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a FlexXml, got "+args[1].Parent.String(), "append")
	}
	// A flex tree stays entirely mutable: a plain Node/Xml child is
	// deep-flexed on the way in; flex handles share.
	child, aerr := core.AdoptIntoFlex(args[0])
	if aerr != nil {
		return nil, r.BoruError("append_error", aerr.Error(), "append")
	}
	fd.Cren = append(fd.Cren, child)
	return []Value{args[1]}, nil
}

func appendXmlListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexXml(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a FlexXml, got "+args[1].Parent.String(), "append")
	}
	src, err := RequireConcreteList(args[0], "append")
	if err != nil {
		return nil, r.BoruError("append_error", err.Error(), "append")
	}
	elems := src.Slice()
	for _, el := range elems {
		child, aerr := core.AdoptIntoFlex(el)
		if aerr != nil {
			return nil, r.BoruError("append_error", aerr.Error(), "append")
		}
		fd.Cren = append(fd.Cren, child)
	}
	return []Value{args[1]}, nil
}

func appendListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fd, err := AsFlexList(args[1])
	if err != nil {
		return nil, r.BoruError("append_error", "append: expected a FlexList, got "+args[1].Parent.String(), "append")
	}
	src, err := RequireConcreteList(args[0], "append")
	if err != nil {
		return nil, r.BoruError("append_error", err.Error(), "append")
	}
	// Slice() snapshots before the grow, so `append f f` (self-concat)
	// is well-defined. Each appended element is enforced + re-tagged against
	// the [:T] element type (the concat spread must not bypass what the
	// single-value append enforces) then adopted so the tree stays mutable.
	elems := src.Slice()
	adopted := make([]Value, len(elems))
	for i, el := range elems {
		tagged, terr := d2AdoptTyped(r, args[1], el, "append")
		if terr != nil {
			return nil, terr
		}
		a, aerr := core.AdoptIntoFlex(tagged)
		if aerr != nil {
			return nil, r.BoruError("append_error", aerr.Error(), "append")
		}
		adopted[i] = a
	}
	fd.Elems = append(fd.Elems, adopted...)
	return []Value{args[1]}, nil
}

// appendListReturns is the check-mode mirror for the list-concat append
// (`[TList, TFlexList]`): dispatch picks this more-specific overload for a list
// source, which otherwise has no ReturnsFn, so check mode never preflighted the
// per-element enforcement appendListHandler runs at runtime. Each provably-known
// source element is validated against the destination's [:T] tag (mirroring the
// runtime raise), and the flex list's tag is retained on the residual (#5, Codex
// round 5). args are [sourceList, FlexList].
func appendListReturns(args []Value, r *Registry) []Value {
	res := NewCarrier(TFlexList)
	if len(args) == 2 {
		if src, err := RequireConcreteList(args[0], "append"); err == nil {
			for i := 0; i < src.Len(); i++ {
				d2CheckWrite(r, args[1], src.Get(i), "append", args[0].Pos())
			}
		}
		res = d2RetainElem(res, args[1])
	}
	return []Value{res}
}
