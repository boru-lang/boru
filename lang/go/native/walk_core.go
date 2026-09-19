package native

import "strconv"

// The core `walk` word — a generic, iterative, visit-only traversal of ANY
// boru data structure.
//
//	walk <options> <data> <descend> [<ascend>]
//
// It descends into lists, maps, XML, and every Ideal value (objects, arrays,
// stores, tables, errors, and user-defined `def` types) via the kernel's
// IdealConverter projection — so user-defined structures traverse with no
// per-type code. The two hooks fire on EVERY node (root, containers, and leaf
// scalars): `descend` in pre-order, `ascend` in post-order. Each hook receives
// one map `{key value path parent depth}`; the root has key=None, path="",
// parent=None, depth=0.
//
// walk is VISIT-ONLY: hooks run for their side effects (e.g. accumulating into
// a captured FlexList), their results are ignored, and walk returns its input
// `data` unchanged so it composes in a pipeline.
//
// The traversal is iterative — an explicit enter/exit stack for depth-first
// (pre + post order) and a FIFO queue for breadth-first (level-order descent,
// ascent replayed in reverse visitation order). There is no recursion, and no
// cycle guard: like StructUtil.walk, walk assumes acyclic data.
//
// This is distinct from `StructUtil.walk` (walk.go), the recursive,
// transform-shaped voxgig-struct word reached only as the qualified module
// export.

// walkFrame is one traversal node plus its hook-payload metadata. The `exit`
// flag drives the two-state depth-first stack (enter = pre-order descend, exit
// = post-order ascend). The key is a string + presence flag (pointer-free, so
// frames copy cleanly and never alias a loop variable).
type walkFrame struct {
	node      Value
	keyStr    string
	hasKey    bool
	path      string
	parent    Value
	hasParent bool
	depth     int64
	exit      bool
}

// walkHook is a classified, ready-to-run hook: a lambda Function, a compiled
// closure, or a quotation token list. `present` is false for an absent
// (optional) ascend hook.
type walkHook struct {
	present bool
	lambda  bool
	closure bool
	fn      Value      // when lambda: the Function value
	body    Value      // when closure: run via InvokeBody
	fnDef   *FnDefInfo // when lambda: its definition (captures + defining registry)
	tokens  []Value    // when quotation: the body tokens
	// sigFn is set for a fn-VALUE closure (ClosureIsFnValue): the bridged
	// FnDefInfo its declared signature is matched under before the hook runs
	// (S1b-2); body then carries the SigMatched mark.
	sigFn Value
}

// walkEntry is one child of a node: its key (map key, or stringified index for
// lists/arrays) and value.
type walkEntry struct {
	key string
	val Value
}

// walkCoreHandler backs both `walk` signatures (3-arg descend-only and 4-arg
// descend+ascend). It reads the mode from the options map, classifies the
// hooks, runs the chosen engine, and returns the input data unchanged.
func walkCoreHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	mode := "depth"
	if IsConcrete(args[0]) {
		if om, _ := AsMap(args[0]); om != nil {
			if mv, ok := om.Get("mode"); ok {
				ms, ok2 := walkModeValue(mv)
				if !ok2 {
					return nil, r.BoruError("walk_error", `walk: options 'mode' must be "depth" or "breadth"`, "walk")
				}
				mode = ms
			}
		}
	}

	descend, err := walkClassifyHook(r, args[2])
	if err != nil {
		return nil, err
	}
	var ascend walkHook
	if len(args) >= 4 {
		ascend, err = walkClassifyHook(r, args[3])
		if err != nil {
			return nil, err
		}
	}

	switch mode {
	case "depth":
		err = walkDFS(r, args[1], descend, ascend)
	case "breadth":
		err = walkBFS(r, args[1], descend, ascend)
	default:
		return nil, r.BoruError("walk_error", `walk: mode must be "depth" or "breadth", got "`+mode+`"`, "walk")
	}
	if err != nil {
		return nil, err
	}
	return []Value{args[1]}, nil
}

// walkModeValue extracts a mode string from the options-map value. Unquoted
// words in a data-context map evaluate (and would error), so the mode is given
// as a string ({mode: "breadth"}) or an atom ({mode: breadth/q}); both are
// accepted here.
func walkModeValue(v Value) (string, bool) {
	if s, err := v.AsConcreteString(); err == nil {
		return s, true
	}
	if a, err := v.AsConcreteAtom(); err == nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return a, true
	}
	return "", false
}

// walkClassifyHook classifies a hook arg once, before the traversal loop —
// mirroring newMapBody (native_map_iter.go): a compiled closure runs via
// InvokeBody, a lambda Function is called via CallBoru, and anything else must
// be a concrete quotation list run as a sub-program.
func walkClassifyHook(r *Registry, body Value) (walkHook, error) {
	h := walkHook{present: true}
	if IsCompiledClosure(body) {
		h.closure = true
		h.body = body
		// A fn-VALUE closure (a capturing `fn` / `=>` literal minted at run
		// time) is matched against its own signature before it runs, as
		// the lambda branch matches a lambda (S1b-2).
		if ClosureIsFnValue(body) {
			if fnv, ok := ClosureAsFnDef(r, body); ok {
				h.sigFn = fnv
				h.body = ClosureSigMatched(body)
			}
		}
		return h, nil
	}
	if body.Parent.ConformsTo(TFunction) {
		h.lambda = true
		h.fn = body
		if fd, ok := body.Data.(FnDefInfo); ok {
			h.fnDef = &fd
		}
		return h, nil
	}
	if !IsConcrete(body) {
		return walkHook{}, r.BoruError("walk_error", "walk: hook must be a quotation list or a lambda", "walk")
	}
	bl, err := AsList(body)
	if err != nil {
		return walkHook{}, r.BoruError("walk_error", "walk: hook must be a quotation list or a lambda", "walk")
	}
	h.tokens = bl.Slice()
	return h, nil
}

// callWalkHook runs one hook for a single node, discarding its result
// (visit-only). A quotation receives the payload map on the stack; a lambda
// receives it as its named parameter.
func callWalkHook(r *Registry, h walkHook, arg Value) error {
	if !h.present {
		return nil
	}
	if h.lambda {
		sig := MatchFnSig(h.fn, []Value{arg})
		if sig == nil {
			return r.BoruError("walk_error", "walk: no matching hook signature", "walk")
		}
		// InvokeCallbackFn, not r.CallBoru: a hook lambda written in another module
		// resolves its free words THERE (design/FUNCTION-VALUE-SCOPE.0.md), and a
		// stamped body runs on the VM rather than the interpreter.
		_, err := InvokeCallbackFn(r, h.fnDef, sig, []Value{arg})
		return err
	}
	if h.closure {
		if h.sigFn.Data != nil && MatchFnSig(h.sigFn, []Value{arg}) == nil {
			return r.BoruError("walk_error", "walk: no matching hook signature", "walk")
		}
		// The fn-VALUE seam: the lambda branch above is InvokeCallbackFn's.
		_, err := InvokeCallbackBody(r, h.body, []Value{arg})
		return err
	}
	_, _, err := runQuotationBody(r, h.tokens, []Value{arg})
	return err
}

// walkHookArgCarrier builds the GENERALISED `{key value path parent depth}`
// payload carrier a compiled hook body is analysed against — the closure-unit
// input mirroring walkBuildHookArg's runtime map (the CallableSpec.Inputs for
// both the quotation and, via LambdaSharesTokenShape, the lambda form). Field
// carriers are types, not one call's values: path/depth are always
// String/Integer; key/value/parent are Any (key and parent are None at the
// root; value is whatever node is visited).
func walkHookArgCarrier() Value {
	om := NewOrderedMap()
	om.Set("key", NewCarrier(TAny))
	om.Set("value", NewCarrier(TAny))
	om.Set("path", NewCarrier(TString))
	om.Set("parent", NewCarrier(TAny))
	om.Set("depth", NewCarrier(TInteger))
	return NewValueRaw(TMap, MapPayload{M: om})
}

// walkBuildHookArg builds the `{key value path parent depth}` payload map for a
// frame. The root has key=None, path="", parent=None (presence is tracked by
// hasKey/hasParent, never by probing Data==nil).
func walkBuildHookArg(f walkFrame) Value {
	om := NewOrderedMap()
	if f.hasKey {
		om.Set("key", NewString(f.keyStr))
	} else {
		om.Set("key", NewNone())
	}
	om.Set("value", f.node)
	om.Set("path", NewString(f.path))
	if f.hasParent {
		om.Set("parent", f.parent)
	} else {
		om.Set("parent", NewNone())
	}
	om.Set("depth", NewInteger(f.depth))
	return NewMap(om)
}

// walkJoinPath builds a child's dot-joined path from its parent's path and key.
func walkJoinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// walkDFS traverses depth-first with an explicit enter/exit stack: an ENTER
// frame fires descend then pushes an EXIT frame (when ascend is present)
// followed by its children in reverse (so the leftmost child is visited
// first); an EXIT frame fires ascend after all descendants. No recursion.
func walkDFS(r *Registry, root Value, descend, ascend walkHook) error {
	stack := []walkFrame{{node: root}}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if f.exit {
			if err := callWalkHook(r, ascend, walkBuildHookArg(f)); err != nil {
				return err
			}
			continue
		}

		if err := callWalkHook(r, descend, walkBuildHookArg(f)); err != nil {
			return err
		}

		entries, _ := childrenOf(f.node)

		if ascend.present {
			ef := f
			ef.exit = true
			stack = append(stack, ef)
		}
		for i := len(entries) - 1; i >= 0; i-- {
			stack = append(stack, walkChildFrame(f, entries[i]))
		}
	}
	return nil
}

// walkBFS traverses breadth-first: a FIFO queue drives level-order descent;
// when ascend is present, every visited frame is recorded and ascent replays
// in reverse visitation order. No recursion.
func walkBFS(r *Registry, root Value, descend, ascend walkHook) error {
	queue := []walkFrame{{node: root}}
	var visited []walkFrame
	for head := 0; head < len(queue); head++ {
		f := queue[head]

		if err := callWalkHook(r, descend, walkBuildHookArg(f)); err != nil {
			return err
		}
		if ascend.present {
			visited = append(visited, f)
		}

		entries, _ := childrenOf(f.node)
		for _, e := range entries {
			queue = append(queue, walkChildFrame(f, e))
		}
	}

	for i := len(visited) - 1; i >= 0; i-- {
		if err := callWalkHook(r, ascend, walkBuildHookArg(visited[i])); err != nil {
			return err
		}
	}
	return nil
}

// walkChildFrame builds a child frame from its parent frame and entry.
func walkChildFrame(parent walkFrame, e walkEntry) walkFrame {
	return walkFrame{
		node:      e.val,
		keyStr:    e.key,
		hasKey:    true,
		path:      walkJoinPath(parent.path, e.key),
		parent:    parent.node,
		hasParent: true,
		depth:     parent.depth + 1,
	}
}

// childrenOf returns a node's ordered child entries and whether it is a
// container. Lists/maps/XML dispatch directly; every other Ideal value (object,
// array, store, table, error, user-defined) projects through the IdealConverter
// walk. Scalars, carriers/type literals, and empty containers are leaves —
// every concrete access is guarded (panic-prevention).
func childrenOf(v Value) ([]walkEntry, bool) {
	if !IsConcrete(v) {
		return nil, false
	}
	if v.Parent.ConformsTo(TList) {
		return listChildren(v)
	}
	if v.Parent.ConformsTo(TMap) {
		// Record/Options/ChildType carriers share Parent=TMap but AsMap
		// returns nil for them — those are leaves.
		return mapChildren(v)
	}
	if IsXmlValue(v) {
		_, _, cren, ok := XmlParts(v)
		if !ok || len(cren) == 0 {
			return nil, false
		}
		out := make([]walkEntry, 0, len(cren))
		for i, c := range cren {
			out = append(out, walkEntry{key: strconv.Itoa(i), val: c})
		}
		return out, true
	}
	if v.Parent.ConformsTo(TIdeal) {
		return idealChildren(v)
	}
	return nil, false
}

// idealChildren enumerates an Ideal value's children via the IdealConverter
// projection: tables project to a List (integer-index keys, so paths
// read list-style); every other Ideal projects to a Map (named keys).
func idealChildren(v Value) ([]walkEntry, bool) {
	if v.Parent.ConformsTo(TTable) {
		lv, err := ConvertIdealToList(v)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, false
		}
		return listChildren(lv)
	}
	mv, err := ConvertIdealToMap(v)
	if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
		return nil, false
	}
	return mapChildren(mv)
}

// listChildren enumerates a concrete List's elements with stringified-index
// keys.
func listChildren(v Value) ([]walkEntry, bool) {
	rl, err := AsList(v)
	if err != nil {
		return nil, false
	}
	n := rl.Len()
	if n == 0 {
		return nil, false
	}
	out := make([]walkEntry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, walkEntry{key: strconv.Itoa(i), val: rl.Get(i)})
	}
	return out, true
}

// mapChildren enumerates a concrete Map's entries in insertion order.
func mapChildren(v Value) ([]walkEntry, bool) {
	rm, err := AsMap(v)
	if err != nil || rm == nil {
		return nil, false
	}
	keys := rm.Keys()
	if len(keys) == 0 {
		return nil, false
	}
	out := make([]walkEntry, 0, len(keys))
	for _, k := range keys {
		cv, _ := rm.Get(k)
		out = append(out, walkEntry{key: k, val: cv})
	}
	return out, true
}
