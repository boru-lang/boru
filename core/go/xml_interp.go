package core

// rebuildXmlFromTmpl is BuildXmlFromTmpl's HOLE-CONSUMING twin (OpInterpXml,
// COMPILE FAILURE-CLOSURE §9.2c): instead of evaluating the ${...} expressions it
// consumes their pre-evaluated single values from holes, in the SAME
// traversal order the build evaluates (attributes first, then children
// left-to-right, depth-first through nested elements). Every assembly rule
// mirrors the build exactly: attribute holes stringify via ValToString into
// the attribute text; a child hole applies the List one-level splice rule;
// adjacent text coalesces. Returns the element and the number of holes
// consumed (the VM verifies it equals the spec's count).
func RebuildXmlFromTmpl(t XmlTmpl, holes []Value) (Value, int) {
	used := 0
	attr := NewOrderedMap()
	for _, a := range t.Attr {
		var sb []byte
		for _, p := range a.Parts {
			if p.Expr == nil {
				sb = append(sb, p.Lit...)
				continue
			}
			if used < len(holes) {
				sb = append(sb, ValToString(holes[used])...)
				used++
			}
		}
		attr.Set(a.Name, NewString(string(sb)))
	}

	var cren []Value
	addText := func(s string) {
		if s == "" {
			return
		}
		if n := len(cren); n > 0 {
			if sp, ok := cren[n-1].Data.(StrPayload); ok {
				cren[n-1] = NewString(sp.S + s)
				return
			}
		}
		cren = append(cren, NewString(s))
	}
	addOne := func(r Value) {
		if IsXmlValue(r) {
			cren = append(cren, r)
			return
		}
		addText(ValToString(r))
	}
	addChild := func(r Value) {
		if IsConcrete(r) && r.Parent.ConformsTo(TList) {
			if rl, err := AsList(r); err == nil {
				for i := 0; i < rl.Len(); i++ {
					addOne(rl.Get(i))
				}
				return
			}
		}
		addOne(r)
	}

	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenLit:
			addText(c.Lit)
		case XmlCrenChild:
			if c.Child == nil {
				continue
			}
			child, cUsed := RebuildXmlFromTmpl(*c.Child, holes[used:])
			used += cUsed
			cren = append(cren, child)
		case XmlCrenExpr:
			if used < len(holes) {
				addChild(holes[used])
				used++
			}
		}
	}
	return NewXmlElement(t.Tag, attr, cren), used
}
