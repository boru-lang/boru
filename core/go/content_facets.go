package core

// The content fields core reads and writes beside the five value_fields.go
// serves: the type metadata, the element constraint and the
// module-namespace facet. A write takes the holder content of its own
// (ownContent), as every content setter does.

func tmetaOf(v *Value) *typeMeta {
	if v.c == nil {
		return nil
	}
	return v.c.tmeta
}

// setTMeta gives v the metadata m. The cached interval labels belong to
// the metadata they mirror, so new metadata drops them and the identity
// and ancestry tests read m's own.
func setTMeta(v *Value, m *typeMeta) {
	c := ownContent(v)
	c.tmeta, c.in, c.out = m, 0, 0
}

func elemOf(v *Value) *Value {
	if v.c == nil {
		return nil
	}
	return v.c.elem
}

func setElemField(v *Value, e *Value) { ownContent(v).elem = e }

func modnsOf(v *Value) *ModuleNSInfo {
	if v.c == nil {
		return nil
	}
	return v.c.modns
}

func setModNSField(v *Value, m *ModuleNSInfo) { ownContent(v).modns = m }
