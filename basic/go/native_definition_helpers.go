package basic

// DefName extracts a word name from a Value that is either a word,
// an atom, or a string. Used by def, undef, type, untype handlers.
// /q-marked sig positions deliver Atoms; bare-word slots deliver
// Words; quoted-string slots deliver Strings.
func DefName(v Value) string {
	if IsWord(&v) {
		_as0, _ := AsWord(&v)
		return _as0.Name
	}
	if IsAtom(v) {
		s, _ := AsAtom(v)
		return s
	}
	s, _ := AsString(v)
	return s
}

// DefStackOnly returns true if the name word carries the /s modifier,
// indicating the defined word should be stack-only (not forward
// precedence).
func DefStackOnly(v Value) bool {
	if IsWord(&v) {
		_as2, _ := AsWord(&v)
		return _as2.ForceStack
	}
	return false
}
