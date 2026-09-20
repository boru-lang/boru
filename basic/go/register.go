package basic

// Register installs the basic layer on the given registry: the
// forth-style stack words, the definition family (def / undef / var /
// fn / afn / fnsig / args), the type-generics words (gen / extends /
// default / of), `const`, the predefined Resource / Entity type
// definitions, and the control-flow words (do / if / case / for /
// break / continue / error). The relative order mirrors the order
// lang's Register applies the same groups.
//
// lang does NOT call this: its Register interleaves these groups with
// the rest of the word library at the historical registration points
// (overload appends are order-sensitive there). Register exists for
// eng-only embedders that want the fundamental words without the full
// language layer — the calc/go pattern, one layer up.
//
// It returns the first registration failure, or nil. Both channels a
// failure can arrive on are folded in, because an eng-only embedder has
// no DefaultRegistryWithPolicy above it to check them: the init-time
// type-registration accumulator (typeinit.go) and the registry's own
// error list, which RegisterNativeFunc appends to rather than raising.
func Register(r *Registry) error {
	// An init-time external-type failure means the type table these words
	// close over is already wrong, so decline before installing anything —
	// the early return lang's DefaultRegistryWithPolicy makes for its
	// own layer.
	if err := TypeInitError(); err != nil {
		return err
	}
	for _, n := range StackNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range DefinitionNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range GenNatives {
		r.RegisterNativeFunc(n)
	}
	r.RegisterNativeFunc(ConstNative)
	InstallResourceTypes(r)
	InstallMicronIdeals(r)
	for _, n := range ControlNatives {
		r.RegisterNativeFunc(n)
	}
	// After every constructor slice above: the def keyword forms are
	// synthesized from the constructors' live signature tables.
	RegisterDefKeywordForms(r)
	return r.Err()
}
