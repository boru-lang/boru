package core

import "strconv"

// An ANONYMOUS fn value's reader identity (NUR257). The dynamic-scope rescue
// keeps an undefined-word finding unless some fn that binds the name reaches
// the READER — the fn whose body read it — through the recorded call graph.
// A named fn is its own reader. An anonymous fn value had none: its body is
// analysed at the end of the pass (NUR105's drain) outside every frame, or
// compiled as a stored unit at the site that wrote it, and a name only a fn
// binds was a finding even when the value runs inside that fn's frame (a
// behaviour a fn exercises, a callback it runs), or it was not one at all.
//
// The identity is the body's first token's source position — the key the
// construction-time check already uses (FnBodyChecked) — registered when the
// value is queued for its body check (NoteAnonFnBody), with the span its body
// covers, so a finding inside the body names its reader by position whatever
// analysis made it. The frames that dispatch the value reach it:
//
//   - one that runs its body — a callback, an application — records the edge
//     when the analysis enters the body (NoteAnonDispatch, AnalyseFnBody);
//   - one that handles a value of a type `behave` installed it on may run it
//     as that type's capability (NoteBehaveReader, NoteBehaveDispatch).

// AnonFnReader is the call-graph name of the anonymous fn value whose body
// starts at start. No program can write it: it holds spaces.
func AnonFnReader(start SrcPos) string {
	return "fn value at " + strconv.Itoa(start.Row) + ":" + strconv.Itoa(start.Col)
}

// BehaveReader is one fn value `behave` installed in this pass: the target
// type whose capability it became, and the value's reader name — its own
// name, or its anonymous identity.
type BehaveReader struct {
	Target *Type
	Reader string
}

// posAfter reports whether a comes after b in the source.
func posAfter(a, b SrcPos) bool {
	return a.Row > b.Row || (a.Row == b.Row && a.Col > b.Col)
}

// bodyEnd is the last source position the tokens cover, nested lists,
// groups and map values included.
func bodyEnd(toks []Value) SrcPos {
	var end SrcPos
	for _, t := range toks {
		if p := t.Pos(); posAfter(p, end) {
			end = p
		}
		if e := bodyEnd(innerTokens(t)); posAfter(e, end) {
			end = e
		}
	}
	return end
}

// innerTokens are the tokens a body token holds: a list's elements, a
// group's tokens, a map's values.
func innerTokens(t Value) []Value {
	switch d := t.Data.(type) {
	case ListPayload:
		return d.Elems
	case ParenExprPayload:
		return d.Toks
	case MapPayload:
		if d.M == nil {
			return nil
		}
		out := make([]Value, 0, d.M.Len())
		for _, k := range d.M.Keys() {
			v, _ := d.M.Get(k)
			out = append(out, v)
		}
		return out
	}
	return nil
}

// NoteAnonFnBody registers an anonymous fn value's bodies — each signature's
// first token and the span it covers — as reader identities. A named value
// is its own reader, and a body with no position has no identity.
func (c *CheckState) NoteAnonFnBody(fd FnDefInfo) {
	if fd.Name != "" {
		return
	}
	for i := range fd.Signatures {
		body := fd.Signatures[i].Body()
		if len(body) == 0 || body[0].Pos().Row == 0 {
			continue
		}
		if c.AnonFnBodies == nil {
			c.AnonFnBodies = map[SrcPos]SrcPos{}
		}
		c.AnonFnBodies[body[0].Pos()] = bodyEnd(body)
	}
}

// NoteAnonDispatch records that the named fn under analysis reaches the
// anonymous fn value whose body starts at start: the analysis is entering
// that body in the fn's frame. A body that is no anonymous value's, or an
// analysis outside every named fn, records nothing.
func (c *CheckState) NoteAnonDispatch(start SrcPos) {
	if _, ok := c.AnonFnBodies[start]; !ok || len(c.FnNameStack) == 0 {
		return
	}
	c.RecordCallEdge(c.FnNameStack[len(c.FnNameStack)-1], AnonFnReader(start))
}

// NoteBehaveReader records fd as a capability `behave` installed on target
// in this pass (BehaveReaders): under its name, or each positioned body's
// anonymous identity.
func (c *CheckState) NoteBehaveReader(target *Type, fd FnDefInfo) {
	if c == nil || !c.Mode || target == nil {
		return
	}
	if fd.Name != "" {
		c.BehaveReaders = append(c.BehaveReaders, BehaveReader{Target: target, Reader: fd.Name})
		return
	}
	for i := range fd.Signatures {
		if body := fd.Signatures[i].Body(); len(body) > 0 && body[0].Pos().Row > 0 {
			c.BehaveReaders = append(c.BehaveReaders, BehaveReader{Target: target, Reader: AnonFnReader(body[0].Pos())})
		}
	}
}

// NoteBehaveDispatch records, for a dispatch in the named fn under analysis,
// an edge to every installed capability whose target one of the operands is
// — a value of the type, or the type itself (a constructor) — since the
// word may run it in this fn's frame.
func (c *CheckState) NoteBehaveDispatch(args []Value) {
	if len(c.BehaveReaders) == 0 || len(c.FnNameStack) == 0 {
		return
	}
	fn := c.FnNameStack[len(c.FnNameStack)-1]
	for i := range args {
		a := &args[i]
		for _, br := range c.BehaveReaders {
			if (a.Parent != nil && a.Parent.ConformsTo(br.Target)) || (IsBareTypeNode(*a) && a.ConformsTo(br.Target)) {
				c.RecordCallEdge(fn, br.Reader)
			}
		}
	}
}

// anonReaderAt is the reader identity of the innermost registered anonymous
// body whose span holds at.
func (c *CheckState) anonReaderAt(at SrcPos) (string, bool) {
	var best SrcPos
	found := false
	for start, end := range c.AnonFnBodies {
		if posAfter(start, at) || posAfter(at, end) {
			continue
		}
		if !found || posAfter(start, best) {
			best, found = start, true
		}
	}
	if !found {
		return "", false
	}
	return AnonFnReader(best), true
}

// AnonScopeReachable reports whether a read of name at `at` — inside an
// anonymous fn value's body — is a dynamic-scope reference: some fn that
// binds the name reaches the value (DynamicScopeReachable under its
// identity). The rescue's question, asked of the reader a nameless analysis
// could not name (NUR257).
func (c *CheckState) AnonScopeReachable(name string, at SrcPos) bool {
	reader, ok := c.anonReaderAt(at)
	return ok && c.DynamicScopeReachable(name, reader)
}

// NoteFnMemberRead tags a get-family read's one result with the fn value it
// resolved from a concrete container at a concrete key (FnMemberReads): the
// read's result is a gradual carrier, and a `behave` over it installs that
// value, which its check-mode half then sees through (FnMemberRead, NUR257).
func (c *CheckState) NoteFnMemberRead(word string, args, results []Value) {
	if len(results) != 1 || results[0].ID == "" || (!IsGetWord(word) && !IsGetrWord(word)) {
		return
	}
	member, ok := fnMemberAt(args)
	if !ok {
		return
	}
	if c.FnMemberReads == nil {
		c.FnMemberReads = map[string]Value{}
	}
	c.FnMemberReads[results[0].ID] = member
}

// FnMemberRead is the fn value a tagged member read resolved (NoteFnMemberRead).
func (c *CheckState) FnMemberRead(id string) (Value, bool) {
	v, ok := c.FnMemberReads[id]
	return v, ok
}

// fnMemberAt resolves a get-family read's operands to the fn value they
// name: a concrete list at a concrete Integer index, or a concrete map at a
// concrete key.
func fnMemberAt(args []Value) (Value, bool) {
	for _, a := range args {
		if a.Carrier {
			continue
		}
		var member Value
		found := false
		switch d := a.Data.(type) {
		case ListPayload:
			for _, k := range args {
				if n, ok := k.Data.(IntPayload); ok && !k.Carrier && n.N >= 0 && n.N < int64(len(d.Elems)) {
					member, found = d.Elems[n.N], true
				}
			}
		case MapPayload:
			for _, k := range args {
				if key, ok := memberKey(k); ok && d.M != nil {
					member, found = d.M.Get(key)
				}
			}
		}
		if _, isFn := member.Data.(FnDefInfo); found && isFn {
			return member, true
		}
	}
	return Value{}, false
}

// memberKey is a concrete read key's text: an Atom or a String.
func memberKey(k Value) (string, bool) {
	if k.Carrier {
		return "", false
	}
	switch d := k.Data.(type) {
	case AtomPayload:
		return d.Name, true
	case StrPayload:
		return d.S, true
	}
	return "", false
}
