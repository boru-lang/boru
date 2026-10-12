package native

// timer callback helper. The "timeout" word itself lives in
// miscNatives (native_misc.go).

// RunTimerCallback executes a timer callback with do semantics.
// For lists, it runs the list elements as a sub-program.
// For words/atoms, it looks up the word and executes it.
func RunTimerCallback(r *Registry, callback Value, isList bool) {
	sub := New(r)
	var input []Value
	if isList {
		if !IsConcrete(&callback) {
			return
		}
		_lst, _ := AsList(callback)
		input = make([]Value, len(_lst.Slice()))
		copy(input, _lst.Slice())
	} else {
		// Callback is an Atom (from /q) or String (quoted form);
		// either yields a word name to invoke.
		var name string
		if IsAtom(callback) {
			name, _ = AsAtom(callback)
		} else {
			name, _ = AsString(callback)
		}
		input = []Value{NewWord(name)}
	}
	// Execute and discard results — timer callbacks run for side effects.
	_, _ = sub.Run(input)
}
