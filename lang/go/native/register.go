package native

// Register installs every built-in word on the given registry. Called
// from DefaultRegistry. Word definitions live in the various
// native_*.go and feature files alongside their handlers.
//
// Lang owns every word name. The eng kernel exposes algorithm
// primitives (CoerceBoolean, TandValues, TorHandler, ...) that the
// registrations below wire into the dispatch; eng does not register
// any word of its own.
//
// Post-consolidation (engine → native), this is the single entry
// point. The Natives slice (in natives.go) covers the
// data-manipulation words formerly registered by native.Register;
// the per-category slices below cover the language-layer primitives
// formerly registered by engine.Register.
func Register(r *Registry) {
	for _, n := range makeNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range inspectNatives {
		r.RegisterNativeFunc(n)
	}
	// break / continue are owned by lang (see native_control.go); the
	// kernel only provides the FlowCtrl type and the Run-loop dispatch.

	// String words moved to the boru:string-util module.

	// Stack ops
	for _, n := range stackNatives {
		r.RegisterNativeFunc(n)
	}

	// Math: basic arithmetic.
	for _, n := range mathNatives {
		r.RegisterNativeFunc(n)
	}

	// Boolean.
	for _, n := range booleanNatives {
		r.RegisterNativeFunc(n)
	}

	// Bitwise operators (band, bor, …) moved to the boru:bin module.

	// Comparison
	for _, n := range comparisonNatives {
		r.RegisterNativeFunc(n)
	}

	// Storage
	for _, n := range storageNatives {
		r.RegisterNativeFunc(n)
	}

	// Definition
	for _, n := range definitionNatives {
		r.RegisterNativeFunc(n)
	}

	// Unpack — map/record destructuring into local bindings.
	for _, n := range unpackNatives {
		r.RegisterNativeFunc(n)
	}

	// Ref / apply — first-class function value pipeline.
	for _, n := range valofNatives {
		r.RegisterNativeFunc(n)
	}

	// __folder / __file — source-location words.
	for _, n := range fileInfoNatives {
		r.RegisterNativeFunc(n)
	}

	// *Type
	for _, n := range genNatives {
		r.RegisterNativeFunc(n)
	}

	for _, n := range typeNatives {
		r.RegisterNativeFunc(n)
	}
	r.RegisterNativeFunc(behaveNative)
	r.RegisterNativeFunc(constNative)
	// nodify moved to the boru:struct module (see struct_module.go).
	r.RegisterNativeFunc(sortNative)
	installResourceTypes(r)
	installIdeals(r)
	installMicronIdeals(r)
	bindSugarWords(r)

	// Control flow
	for _, n := range controlNatives {
		r.RegisterNativeFunc(n)
	}

	// Error raising (raise) + Error-value field access
	for _, n := range errorNatives {
		r.RegisterNativeFunc(n)
	}

	// Accessors
	for _, n := range accessorNatives {
		r.RegisterNativeFunc(n)
	}

	// Bytes type — core construction / conversion / slicing words.
	for _, n := range bytesNatives {
		r.RegisterNativeFunc(n)
	}

	// Within-type scalar & Micron operations (String/Atom/Bytes occurrence
	// package, Boolean's defined arithmetic error, the Micron field-wise
	// default) — installed as CoreDefault overloads AFTER mathNatives and
	// bytesNatives so they append to the existing arithmetic words. Unlocked
	// (CoreDefault) so a user's more-specific override wins; see
	// installScalarOpDefaults.
	installScalarOpDefaults(r)

	// I/O, help, module, temporal (consolidated)
	for _, n := range miscNatives {
		r.RegisterNativeFunc(n)
	}
	for _, n := range printNatives {
		r.RegisterNativeFunc(n)
	}
	// printstr / read / write / stdin / stdout / stderr / trace moved to the
	// boru:io module (see io_module.go); only `print` stays in core.

	// Unify
	for _, n := range unifyNatives {
		r.RegisterNativeFunc(n)
	}

	// Array (core + higher-order)
	for _, n := range arrayNatives {
		r.RegisterNativeFunc(n)
	}

	// Map projections (keys / vals); the each/for-each/fold/filter Map
	// overloads live on those words' own signatures.
	for _, n := range mapNatives {
		r.RegisterNativeFunc(n)
	}

	// Flex nodes (flex / node / append)
	for _, n := range flexNatives {
		r.RegisterNativeFunc(n)
	}

	// XML accessor / query words (elem / text / xml-attr)
	for _, n := range xmlNatives {
		r.RegisterNativeFunc(n)
	}

	// Macro system (gensym; macro/unquote/splice/macroexpand in later phases)
	for _, n := range macroNatives {
		r.RegisterNativeFunc(n)
	}

	// Data-manipulation words (former native.Register body).
	for _, n := range Natives {
		r.RegisterNativeFunc(n)
	}
	// Patrun pattern-matching dispatch table (patrun / find / patterns, plus
	// the add / remove Patrun overloads). Registered AFTER mathNatives and
	// Natives so the add / remove overloads append to the existing words
	// (upsertFnDef) rather than starting fresh.
	for _, n := range patrunNatives {
		r.RegisterNativeFunc(n)
	}
	// BEAM-style processes (spawn / self / send / receive / register /
	// whereis / unregister). Registered after patrunNatives — `receive`
	// routes clauses through the same trie.
	for _, n := range processNatives {
		r.RegisterNativeFunc(n)
	}
	// In-process services (service / add / call / send / state / wrap).
	// After processNatives so the `send` Service overload appends.
	for _, n := range serviceNatives {
		r.RegisterNativeFunc(n)
	}
	// def's keyword-form overloads (`def name fn […]`, `def name gen
	// [T] class {…}`, …) are synthesized from the constructors' live
	// signature tables, so this must run AFTER every constructor slice
	// above has registered — and again for module sub-registries, whose
	// preambles use the same idioms.
	registerDefKeywordForms(r)

	r.Modules.InitFunc = func(child *Registry) {
		for _, n := range Natives {
			child.RegisterNativeFunc(n)
		}
		registerDefKeywordForms(child)
		// Module sub-registries construct microns in preambles the
		// same way top-level source does — the family Ideal moved to
		// basic with the content (the kernel no longer registers it),
		// so install it here too.
		installMicronIdeals(child)
		// Module sub-registries run preamble source with the same
		// surface sugars — bind the roles there too.
		bindSugarWords(child)
	}
}

// bindSugarWords binds the kernel's sugar roles to boru's word names
// (ADR-012 rule 3, 2026-08-04 amendment). The parser emits structural
// sugar markers; the engine lowers each marker through these bindings
// at step time (core/go/sugar.go). This table is the ONLY place the
// surface sugars' word names are decided.
func bindSugarWords(r *Registry) {
	r.BindSugarWord(SugarUsurp, "usurp")
	r.BindSugarWord(SugarStackArgs, "stack-args")
	r.BindSugarWord(SugarForwardArgs, "forward-args")
	r.BindSugarWord(SugarForceArity, "force-arity")
	r.BindSugarWord(SugarMini, "mini")
	r.BindSugarWord(SugarLambda, "afn")
	r.BindSugarWord(SugarGenHead, "gen")
	r.BindSugarWord(SugarGenApply, "of")
	r.BindSugarWord(SugarGenDefault, "default")
}
