package core

// Marker cells (design/VALUE-NO-COPY.0.md §9, phase 4). The engine mints
// its control markers — a forward collection's marker and its advance at
// each arrival, a loop iteration's mark and move — per call, per arrival and
// per iteration. A value whose payload is a struct costs two allocations
// once its content is behind a pointer: the content, and the payload boxed
// into the interface. A marker cell holds the content and the payload in
// one allocation, the payload reached by pointer, so a marker costs what it
// cost while the content lived in the holder. A cell is never written once
// a holder refers to it: an advance mints a new cell (advanceForward).

type forwardCell struct {
	c content
	p ForwardInfo
}

type markCell struct {
	c content
	p MarkInfo
}

type moveCell struct {
	c content
	p MoveInfo
}

func newForwardValue(info ForwardInfo) Value {
	fc := &forwardCell{p: info}
	fc.c.parent, fc.c.data = TForward, &fc.p
	return holderOf(TForward, &fc.c)
}

func newMarkValue(info MarkInfo) Value {
	mc := &markCell{p: info}
	mc.c.parent, mc.c.data = TMark, &mc.p
	return holderOf(TMark, &mc.c)
}

func newMoveValue(info MoveInfo) Value {
	mc := &moveCell{p: info}
	mc.c.parent, mc.c.data = TMove, &mc.p
	return holderOf(TMove, &mc.c)
}

// advanceForward repoints slot — a forward marker's own tape cell — at a new
// cell holding info, the marker's other facets kept: a collection advanced
// by one arrival. The cell the slot held is not written, so a copy of the
// slot taken before the arrival (a trace or debugger snapshot) keeps the
// state it had.
func advanceForward(slot *Value, info ForwardInfo) {
	fc := &forwardCell{p: info}
	copyFacets(&fc.c, cont(slot))
	fc.c.data = &fc.p
	slot.c = &fc.c
}
