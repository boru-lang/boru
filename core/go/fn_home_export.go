package core

// HomeExportedFn is the module-export half of a fn value's home. A module's
// export map may hold three kinds of fn value, and each needs a different
// hand:
//
//   - a Go-built value (a native the module registered, a wrapper a Go word
//     minted) carries no home yet; it is adopted into modReg and minted
//     afresh;
//   - the module's OWN fn (stamped modReg at construction) is minted afresh
//     too: an export is a NEW value with no source position of its own, not
//     the `name/v` token it was read through — a region claim keyed by that
//     token's position would otherwise land inside the module body;
//   - a fn homed ELSEWHERE — imported and re-exported — passes through
//     untouched, identity and all.
//
// The bool is false when v is not a fn value at all, so a caller can fall
// through to its name / type resolution. resolveModuleExport and the Test
// module's twin both call this; the rule lives here once.
func HomeExportedFn(v Value, modReg *Registry) (Value, bool) {
	fd, ok := FnDefOf(DataOf(&v))
	if !ok {
		return v, false
	}
	if fd.HasHome() && !fd.Registry.SameHome(modReg) {
		return v, true
	}
	return NewFunction(exportedFnDef(fd, modReg)), true
}

// exportedFnDef constructs the definition a module exports for fd: fd's parts
// homed at modReg when fd carries no home of its own (a Go-built value the
// module adopts), its own home otherwise. A new definition — the export is a
// new value — never a write to fd, which the module's bindings still hold.
func exportedFnDef(fd *FnDefInfo, modReg *Registry) FnDefInfo {
	out := *fd
	if !fd.HasHome() {
		out.Registry = modReg
	}
	return out
}
