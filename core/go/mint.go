package core

// Construction from parts (design/VALUE-NO-COPY.0.md §5, exceptions B6):
// a value that differs from an existing one in its payload is MINTED — a
// new header assembled from the parts it shares — never a copy of the old
// header with a field overwritten.

// MintLike constructs a value of proto's kind around the payload data: the
// same type and facets — Parent, type-node facet, element constraint,
// module-namespace facet, flags and occurrence state — under an identity of
// its own. A new container instance is built this way (a literal's
// per-evaluation spine, the cloner's containers): its contents are new,
// and it is a different value from proto.
func MintLike(proto *Value, data Payload) Value {
	v := Value{
		Data:       data,
		Parent:     proto.Parent,
		tmeta:      proto.tmeta,
		occ:        proto.occ,
		elem:       proto.elem,
		modns:      proto.modns,
		IsInternal: proto.IsInternal,
		Origin:     proto.Origin,
		Carrier:    proto.Carrier,
		flags:      proto.flags,
	}
	SetID(&v, GenerateID(IDPrefixForType(v.Parent)))
	return v
}
