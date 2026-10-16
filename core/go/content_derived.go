package core

import "sync/atomic"

// Contents derived from a word's content (design/VALUE-NO-COPY.0.md §9,
// phase 4). Two engine rewrites replace a word token with a value built from
// it each time the word is evaluated: a completed forward collection
// re-dispatches its word in force-stack form (forceStackWord), and a /q slot
// takes an arriving word as the atom of its name (CollectArrival). What
// either builds depends on the word's content alone, which never changes
// once published, so it is built once and kept with the content: a loop
// body's or a fn body's word is rewritten without an allocation from its
// second evaluation on. Engines running concurrently share program tokens,
// so the memo is filled atomically by whichever engine needs it first; it is
// no part of what the content means, and a copy of the content
// (copyFacets) does not carry it.
type derivedContents struct {
	forceStack atomic.Pointer[content]
	atom       atomic.Pointer[content]
}

// derivedOf is c's memo, installed on first use.
func derivedOf(c *content) *derivedContents {
	if d := c.derived.Load(); d != nil {
		return d
	}
	c.derived.CompareAndSwap(nil, &derivedContents{})
	return c.derived.Load()
}

// wordContentIs reports whether c is the content of the word w — the case
// the memo serves; a caller handed some other word mints its own content.
func wordContentIs(c *content, w WordInfo) bool {
	cw, ok := c.data.(WordInfo)
	return ok && cw == w
}

// forceStackContent is the content of the word w in force-stack form, for
// the holder cell that holds w: memoized on the cell's content.
func forceStackContent(cell *Value, w WordInfo) *content {
	fs := WordInfo{Name: w.Name, ArgCount: w.ArgCount, ForceStack: true}
	if cell.c == nil || !wordContentIs(cell.c, w) {
		return &content{parent: TWord, data: fs}
	}
	d := derivedOf(cell.c)
	if m := d.forceStack.Load(); m != nil {
		return m
	}
	d.forceStack.CompareAndSwap(nil, &content{parent: TWord, data: fs})
	return d.forceStack.Load()
}

// atomContent is the content of the atom named by the word w, for the
// holder cell that holds w: memoized on the cell's content.
func atomContent(cell *Value, w WordInfo) *content {
	if cell.c == nil || !wordContentIs(cell.c, w) {
		return &content{parent: TAtom, data: AtomPayload{Name: w.Name}}
	}
	d := derivedOf(cell.c)
	if m := d.atom.Load(); m != nil {
		return m
	}
	d.atom.CompareAndSwap(nil, &content{parent: TAtom, data: AtomPayload{Name: w.Name}})
	return d.atom.Load()
}
