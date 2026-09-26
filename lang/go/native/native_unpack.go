package native

import (
	"fmt"
	"strings"

	check "github.com/boru-lang/boru/check/go"
	core "github.com/boru-lang/boru/core/go"
)

// unpackNatives covers the destructuring word `unpack`, which extracts
// selected entries from a Map (or Record) value and binds each to a
// bare word in the current scope — boru's analogue of JavaScript object
// destructuring (`const {from, where, select} = query`).
//
// Surface (forward form): `unpack [names] map`.
//
//	def m {x:1}
//	unpack [x] m            # binds x → 1 in the current scope
//	x                       # → 1
//
// The motivating use case is improving the SQL DX of boru:query: after
// `import "boru:query"`, the words live under a dot namespace
// (Query.from, Query.where, …). Destructuring lifts the chosen ones to
// bare names:
//
//	import "boru:query"
//	unpack [select from where] query
//	select [name age] from people where [age gt 18]
//
// Because module export values are FnDefInfo Values that already carry
// their sub-registry, re-binding the extracted value under a bare name
// preserves module-scope dispatch — no copying or re-wrapping.
//
// Three selector forms (sig[0]), all over the same source map/record
// (sig[1]):
//
//   - `unpack [names] map`     — explicit names: bind each listed key.
//   - `unpack all map`         — bind every key of the source.
//   - `unpack {renames} map`   — rename: each entry `srcKey: localName`
//     binds source key `srcKey` to the bare
//     word `localName`.
//
// Examples:
//
//	def m {a:1 b:2}
//	unpack [a] m          # binds a → 1
//	unpack all m          # binds a → 1 and b → 2
//	unpack {a: x b: y} m  # binds x → 1 and y → 2
var unpackNatives = []NativeFunc{
	{
		Name: "unpack",
		Signatures: []Signature{
			// `unpack [names] map`: explicit names list. NoEvalArgs[0]
			// keeps the bare words un-evaluated so they survive as names.
			{
				Args:       []*Type{TList, TMap},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(unpackHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
				// S2b's declaration: the list is the NAMES of the bindings —
				// keys the handler reads, as def's quoted name is
				// (CompileQuoteKey); the binds are lowered by the run-time
				// bind / binder hooks, never as a body.
				CompileEffect: CompileQuoteKey,
			},
			// `unpack {renames} map`: the first map's entries drive the
			// bindings (srcKey → localName). NoEvalMapArgs[0] keeps the
			// target names un-evaluated so they survive as bare words.
			{
				Args:          []*Type{TMap, TMap},
				NoEvalMapArgs: map[int]bool{0: true},
				Impl:          Go(unpackRenameHandler, RunInCheck()),
				Returns:       []*Type{},
				BarrierPos:    -1,
			},
			// `unpack all map`: the `all` keyword (captured as an atom via
			// /q even though it is a registered word) binds every key.
			{
				Args:       []*Type{TAtom, TMap},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(unpackAllHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
				// The handler-contract declaration (design/HANDLER-MIGRATION-
				// LINE.0.md, the quoted class, S2a): the quoted `all` is a
				// literal keyword the handler consumes verbatim — inert data.
				// The word runs in check mode (the bindings must exist for the
				// checker), so the recorder never reaches it through the
				// quoted-operand gates; the flag answers the census.
				CompileEffect: CompileQuoteInert,
			},
			// `unpack 'boru:time-util'`: import a module and bind every word
			// of every export namespace as a bare local — `now`, `sleep`, …
			// usable without the `TimeUtil.` prefix.
			//
			// RunInCheckMode: a module's exports are statically declared, so the
			// checker resolves them exactly as it does for `import` — binding
			// them unqualified here lets a later bare `sqrt` type-check instead
			// of flagging undefined_word (module unpacking is NOT a runtime-only
			// effect; only the module-NAME string must survive carrier-stripping,
			// see checkModeLiteralWords).
			{
				Args:       []*Type{TString},
				Impl:       Go(unpackModuleHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
			},
			// `unpack ExportName 'boru:mod'`: import the module and unpack only
			// the named export namespace (for multi-export modules). The name
			// is captured as an atom via /q.
			{
				Args:       []*Type{TAtom, TString},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       Go(unpackModuleExportHandler, RunInCheck()),
				Returns:    []*Type{},
				BarrierPos: -1,
				// The quoted export name is the KEY the handler reads to select
				// one export namespace of the module (see `unpack all`'s note on
				// why the flag answers the census rather than lowering).
				CompileEffect: CompileQuoteKey,
			},
		},
	},
}

// resolveModuleDescForUnpack resolves a module name (with or without the
// "boru:" prefix) to its ModuleDesc via the registry's native-module resolver.
func resolveModuleDescForUnpack(r *Registry, modName string) (ModuleDesc, error) {
	name := strings.TrimPrefix(modName, "boru:")
	if name == "" {
		return ModuleDesc{}, r.BoruError("unpack_error", "unpack: empty module name", "unpack")
	}
	if r.Modules.Resolver == nil {
		return ModuleDesc{}, r.BoruError("unpack_error", "unpack: module resolver not configured (cannot unpack "+modName+")", "unpack")
	}
	desc, err := r.Modules.Resolver(name, r)
	if err != nil {
		return ModuleDesc{}, r.BoruError("unpack_error", "unpack: "+err.Error(), "unpack")
	}
	desc.Ref = "boru:" + name
	desc.Kind = "native"
	// NUR045: the export's policy identity is stamped onto its
	// dispatchable signatures once, here, where the resolved id is
	// first known — every dispatch gate downstream is a pointer test.
	StampModuleCallGates(desc.Exports, desc.Ref)
	return desc, nil
}

// unpackExportMap binds each word in an export namespace's map as a bare def
// in the current scope. Capitalised (type) names are skipped — unpack binds
// values only.
func unpackExportMap(r *Registry, exportMap *OrderedMap) {
	if exportMap == nil {
		return
	}
	for _, k := range exportMap.Keys() {
		if IsCapitalisedName(k) || strings.HasPrefix(k, "$") {
			continue
		}
		if v, ok := exportMap.Get(k); ok {
			InstallDef(r, k, v)
		}
	}
}

// unpackModuleHandler implements `unpack 'boru:mod'` — resolve the module and
// bind every word of every export namespace as a bare local.
func unpackModuleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	modName, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("unpack_error", "unpack: module name must be a string", "unpack")
	}
	desc, err := resolveModuleDescForUnpack(r, modName)
	if err != nil {
		return nil, err
	}
	for _, exportMap := range desc.Exports {
		unpackExportMap(r, exportMap)
	}
	return nil, nil
}

// unpackModuleExportHandler implements `unpack ExportName 'boru:mod'` — resolve
// the module and unpack only the named export namespace.
func unpackModuleExportHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	exportName, err := AsAtom(args[0])
	if err != nil {
		return nil, r.BoruError("unpack_error", "unpack: export name must be a word/atom", "unpack")
	}
	modName, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("unpack_error", "unpack: module name must be a string", "unpack")
	}
	desc, err := resolveModuleDescForUnpack(r, modName)
	if err != nil {
		return nil, err
	}
	exportMap, ok := desc.Exports[exportName]
	if !ok {
		return nil, r.BoruError("unpack_error", fmt.Sprintf("unpack: export %q not found in module %q", exportName, modName), "unpack")
	}
	unpackExportMap(r, exportMap)
	return nil, nil
}

// unpackSource resolves the source (sig[1]) to a keyed lookup plus its
// ordered key list. Accepts a concrete map or a record's field map. In
// check mode a non-concrete source (e.g. an imported export not yet
// materialised) yields an empty getter so analysis can continue with
// Any carriers rather than erroring.
// The `proven` result reports whether the getter reads a CONCRETE source —
// a miss through a proven getter is a guaranteed runtime unpack_error the
// checker may flag, where the check-mode stub's misses (an abstract source)
// prove nothing.
func unpackSource(src Value, r *Registry) (get func(string) (Value, bool), keys []string, proven bool, err error) {
	switch {
	case IsConcrete(src):
		if m, mErr := AsMap(src); mErr == nil && m != nil {
			return m.Get, m.Keys(), true, nil
		}
		if IsRecordType(src) {
			rec, rErr := AsRecordType(src)
			if rErr != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
				return nil, nil, false, r.BoruError("unpack_error", "unpack: source record is malformed", "unpack")
			}
			return rec.Fields.Get, rec.Fields.Keys(), true, nil
		}
		return nil, nil, false, r.BoruError("unpack_error", "unpack: source must be a map or record", "unpack")
	case r.Check.IsActive():
		return func(string) (Value, bool) { return Value{}, false }, nil, false, nil
	default:
		return nil, nil, false, r.BoruError("unpack_error", "unpack: source is not a concrete map", "unpack")
	}
}

// bindUnpackEntry binds localName to the source entry under srcKey.
// Validates the target name (rejects capitalised/type-clashing names),
// and is strict on missing keys except in check mode (where it binds an
// Any carrier so later references still resolve). pos locates the name
// token for unused-def analysis.
func bindUnpackEntry(r *Registry, localName, srcKey string, get func(string) (Value, bool), proven bool, pos SrcPos) error {
	if localName == "" {
		return r.BoruError("unpack_error", "unpack: names must be words, atoms, or strings", "unpack")
	}
	if IsCapitalisedName(localName) {
		return r.BoruError("unpack_error", "unpack: cannot bind capitalised (type) name "+localName+" — unpack binds values only", "unpack")
	}
	if err := ValidateWordName(localName); err != nil {
		return err
	}
	if r.Defs.IsType(localName) {
		return r.BoruError("unpack_error", "unpack: name clash — "+localName+" is already a type", "unpack")
	}

	val, ok := get(srcKey)
	if !ok {
		if r.Check.IsActive() {
			val = NewCarrier(TAny)
			// An UNPROVEN source binds a value the pass knows nothing
			// about: a GRADUAL Any, so a downstream dispatch over it
			// poly re-matches at run time (as a `x:Any` param's does)
			// instead of recovering to a strict no-match (2026-09-25).
			if !proven {
				val.Dynamic = true
			}
			// A miss against a PROVEN (concrete) source is a guaranteed
			// runtime unpack_error — flag it (a RuntimeMirror: the trap
			// below compiles the identical error, and the compile failure loop
			// skips mirrors — TestEmitTrap pins the trap still compiling).
			// An abstract source's stub miss proves nothing; a nested /
			// fn-body unpack is conditionally reached and stays lenient
			// (the top-level gate).
			if proven && check.CheckAtUncaughtTopLevel(r) {
				core.CheckAddUniqueDiagnostic(r, "unpack_error",
					"unpack: key "+srcKey+" not found in source", "unpack", pos)
			}
			// Lenient binding either way, but the interpreter errors at
			// runtime — ONLY against a PROVEN source. Record a TERMINAL trap
			// so a bytecode compile raises the byte-identical unpack_error
			// here (the same detail as the non-check branch below) instead
			// of declining; if the trap can't be recorded (a nested unpack),
			// keep the blanket-compile failure flag so the program falls
			// back. An UNPROVEN source (a carrier Map — a param, a fn's
			// result) misses through the stub getter whatever it holds, and
			// the run-time unpack over the real value succeeds or raises on
			// its own: a trap here compiled `unpack [a b] (f)` over a
			// computed `{a:1 b:2}` to an unpack_error the interpreter never
			// raised (a live miscompile until 2026-09-25), and the latch
			// declined the same shape inside a fn. Neither is recorded for
			// an unproven source; the dispatch lowers as the plain
			// CALL_NATIVE it is, binding the names at run time.
			if proven && !r.Check.Recorder().RecordTrap("unpack_error",
				"unpack: key "+srcKey+" not found in source", "unpack", "", pos) {
				r.Check.SuppressedRuntimeError = true
			}
			if !proven {
				r.Check.Recorder().NoteRuntimeBind(localName)
			}
		} else {
			return r.BoruError("unpack_error", "unpack: key "+srcKey+" not found in source", "unpack")
		}
	}
	_, err := installAndRecordDef(r, localName, val, pos)
	return err
}

// unpackHandler binds each name in args[0] to the matching entry of the
// map/record args[1] — `unpack [names] map`.
func unpackHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	names, err := RequireConcreteList(args[0], "unpack")
	if err != nil {
		return nil, err
	}
	get, _, proven, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, el := range names.Slice() {
		name := defName(el)
		if err := bindUnpackEntry(r, name, name, get, proven, el.Pos()); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// unpackAllHandler binds every key of the source — `unpack all map`.
// args[0] is the `all` keyword atom (only the literal `all` is accepted);
// args[1] is the source map/record.
func unpackAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	kw, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	if kw != "all" {
		return nil, r.BoruError("unpack_error", "unpack: expected a name list, the keyword `all`, or a rename map, got "+kw, "unpack")
	}
	get, keys, proven, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if err := bindUnpackEntry(r, k, k, get, proven, args[0].Pos()); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// unpackRenameHandler binds each source key to a chosen local name —
// `unpack {srcKey: localName …} map`. args[0] is the rename map (values
// kept un-evaluated via NoEvalMapArgs so the target names survive as
// bare words); args[1] is the source map/record.
func unpackRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	renames, err := RequireConcreteMap(args[0], "unpack")
	if err != nil {
		return nil, err
	}
	get, _, proven, err := unpackSource(args[1], r)
	if err != nil {
		return nil, err
	}
	for _, srcKey := range renames.Keys() {
		target, _ := renames.Get(srcKey)
		localName := defName(target)
		if err := bindUnpackEntry(r, localName, srcKey, get, proven, target.Pos()); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
