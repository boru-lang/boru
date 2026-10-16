package core

// Construction from parts (design/VALUE-NO-COPY.0.md §5, exceptions B6):
// a value that differs from an existing one — in its type, its payload, its
// carrier shape or its identity — is MINTED: a new header assembled from the
// parts it shares with the source, never a copy of the source's header with
// a field overwritten. The family replaces the copy-returning retag helpers
// (ReparentValue, FreshenDefault's `out := v`, JoinCarriersInner's
// `out := a`, ReturnsIdentity's re-mint, RetagFlexElem, the cloner's
// withPayload) and the recorder's `x := y; SetID(&x, …)` re-mints.

// mintFrom assembles a header from src's facets with parent and data in
// place of src's own — the one place the family lists the header's parts.
// The occurrence state (position, identity, ascription, source binding,
// recorder key) and the occurrence flags are src's: the new value is the
// same occurrence under the new parts unless the constructor says otherwise.
func mintFrom(src *Value, parent *Type, data Payload) Value {
	s := cont(src)
	return Value{
		c: &content{
			data:       data,
			parent:     parent,
			tmeta:      s.tmeta,
			elem:       s.elem,
			modns:      s.modns,
			isInternal: s.isInternal,
			origin:     s.origin,
			carrier:    s.carrier,
		},
		occ:   src.occ,
		flags: src.flags,
	}
}

// MintLike constructs a value of proto's kind around the payload data: the
// same type and facets — Parent, type-node facet, element constraint,
// module-namespace facet, flags and occurrence state — under an identity of
// its own. A new container instance is built this way (a literal's
// per-evaluation spine, the cloner's containers): its contents are new,
// and it is a different value from proto.
func MintLike(proto *Value, data Payload) Value {
	v := mintFrom(proto, ParentOf(proto), data)
	SetID(&v, GenerateID(IDPrefixForType(ParentOf(&v))))
	return v
}

// MintFresh constructs src under an identity of its own: the same type,
// payload, facets and position. A recording pass tells it apart from src —
// a stack word's duplicated output, a residual handed out once per call, a
// member read at a second site — where a copy would carry src's identity.
func MintFresh(src *Value) Value {
	return MintLike(src, DataOf(src))
}

// MintRetagged constructs src under the type parent: the same payload,
// facets and occurrence (position, identity, ascription), so the exemplar
// the caller still holds keeps its own type (`def p:Pos n` does not retype
// n). It is the typed-def reparent every typed bind routes through
// (predicate, refine-bare and FnUndef binds, a const's interned kind, a
// typed list's unified element type). A recording pass gives the new value
// an identity of its own where it records the retag as an event
// (RecordTypedBind), as any event's result.
func MintRetagged(src *Value, parent *Type) Value {
	return mintFrom(src, parent, DataOf(src))
}

// MintPayload constructs src holding data: the same type, facets and
// occurrence, around a payload of its own — a value whose contents were
// rebuilt (a freshened default's new flex node, a stamped fn's signatures)
// and that is otherwise the one the caller holds.
func MintPayload(src *Value, data Payload) Value {
	return mintFrom(src, ParentOf(src), data)
}

// MintCarrierFrom constructs the check-mode carrier of src's type: no
// payload, src's facets and position, and an identity of its own — a join
// of two arms is a new value (an `if` result), never arm src.
func MintCarrierFrom(src *Value) Value {
	v := mintFrom(src, ParentOf(src), nil)
	v.c.carrier = true // v's content is its own: mintFrom built it
	SetID(&v, GenerateID(IDPrefixForType(ParentOf(&v))))
	return v
}

// MintCarrierKeeping constructs the check-mode carrier of src that is still
// src to a recording pass: src's type, facets and occurrence — its identity
// and recorder key included — around data, nil to drop the payload (a
// literal stripped for the check pass) or src's own to keep it as the bound
// (a disjunct's alternatives, a narrowed refinement). A carrier that is a
// new value (a join, a derivation) is MintCarrierFrom's.
func MintCarrierKeeping(src *Value, data Payload) Value {
	v := mintFrom(src, ParentOf(src), data)
	v.c.carrier = true // v's content is its own: mintFrom built it
	return v
}

// MintElemTyped constructs the container src under the element constraint
// *elem (a flex retag): the same payload — so a flex container keeps its
// shared store and its identity — facets and occurrence, the element type
// elem's. The facet refers to *elem, which is never written after (R2); an
// Any constraint is none.
func MintElemTyped(src *Value, elem *Value) Value {
	v := mintFrom(src, ParentOf(src), DataOf(src))
	v.c.elem = nil // v's content is its own: mintFrom built it
	if ParentOf(elem) != nil && !ParentOf(elem).Equal(TAny) {
		v.c.elem = elem
	}
	return v
}
