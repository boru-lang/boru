package core

// UninstallType removes a capitalised (TYPE) binding: it pops the entry and
// retires the lattice node the binding MINTED. Reports whether a binding was
// there to remove. It lives in core, beside the other binding operations,
// because it IS one.
func UninstallType(r *Registry, name string) bool {
	if r == nil {
		return false
	}
	entry, ok := r.Defs.PopEntry(name)
	if !ok {
		return false
	}
	// Retire only a node THIS binding minted. An alias binding ADOPTS an
	// existing canonical node (`def Foo Integer` binds the Integer node
	// itself — InstallType's alias arm), so retiring it here would delete a
	// builtin's or another binding's identity from the ID index.
	// …and only when no other live binding still holds the node: the same
	// minted node pushed twice under one name (a twin's replay of one
	// captured entry) is retired by the last pop, not the first (NUR135).
	if entry.TypeDef != nil && entry.Minted && !r.Defs.HoldsType(entry.TypeDef) {
		r.Types.Retire(entry.TypeDef)
	}
	return true
}
