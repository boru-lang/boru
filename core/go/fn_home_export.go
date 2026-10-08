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
	fnDef, ok := v.Data.(FnDefInfo)
	if !ok {
		return v, false
	}
	if !fnDef.HasHome() {
		fnDef.Registry = modReg
	}
	if fnDef.Registry.SameHome(modReg) {
		return NewFunction(fnDef), true
	}
	return v, true
}
