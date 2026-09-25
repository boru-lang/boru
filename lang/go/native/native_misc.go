package native

import (
	"fmt"
	"strings"
	"time"

	"github.com/boru-lang/boru/lang/go/native/help"
)

// miscNatives covers the smaller engine word groupings: file I/O
// (read, write, stdin, stdout, stderr), help, module/import,
// temporal (timeout, await).
//
// The supporting helpers (formatFromExt, parseFileOpts, valueToJsonic,
// doRead, doWrite, RunModuleBody, install*Exports, runParallelBranch,
// awaitAll/Full/First/Any, makeDynamicEval, etc.) live in their
// original feature files (fileio.go, native_help.go,
// native_module_module.go, native_temporal_await.go,
// native_temporal_timeout.go).
//
// Initialised in init() rather than as a direct var literal because
// the module/import handlers transitively call DefaultRegistry ->
// Register -> miscNatives, and Go's package-init cycle detector
// flags that as a forbidden cycle when the slice literal is at file
// scope. init-time assignment defers the function-value capture
// past the cycle check.
var miscNatives []NativeFunc

func init() {
	miscNatives = []NativeFunc{
		// file I/O (read/write) and stream words (stdin/stdout/stderr)
		// moved to the boru:io module — see io_module.go.

		// ---- help (language overview) ----
		{
			Name: "help",

			Signatures: []Signature{
				// Prints the overview to r.Output; produces no value.
				{Args: []*Type{}, Impl: Go(helpOverviewHandler), Returns: []*Type{}, BarrierPos: -1},
			},
		},

		// ---- describe (per-word documentation) ----
		{
			Name: "describe",

			Signatures: []Signature{
				// Prints the documentation to r.Output; produces no value.
				{Args: []*Type{TString}, Impl: Go(describeWordHandler), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TAtom}, Impl: Go(describeWordHandler), Returns: []*Type{}, BarrierPos: -1},
				{
					Args:      []*Type{TAtom},
					QuoteArgs: map[int]bool{0: true},
					Impl:      Go(describeWordHandler),
					Returns:   []*Type{}, BarrierPos: -1,
					// The handler-contract declaration (design/HANDLER-
					// MIGRATION-LINE.0.md, the quoted class, S2a): the quoted
					// word is the datum the handler documents, consumed
					// verbatim — so `describe foo` bakes as a CALL_NATIVE over
					// the inert atom and the VM prints what the interpreter
					// prints (the same handler, the same registry).
					CompileEffect: CompileQuoteInert,
				},
				{Args: []*Type{}, Impl: Go(describeSelfHandler), Returns: []*Type{}, BarrierPos: -1},
			},
		},

		// ---- referent (what a quoted atom refers to) ----
		{
			Name: "referent",

			Signatures: []Signature{
				{Args: []*Type{TAtom}, Impl: Go(referentHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},

		// ---- module / import ----
		{
			Name: "module",

			Signatures: []Signature{{
				Args:       []*Type{TList},
				NoEvalArgs: map[int]bool{0: true},
				Impl:       Go(moduleHandler, RunInCheck()),
				Returns:    []*Type{TModuleInst},
				BarrierPos: -1,
			}},
		},
		{
			Name: "import",

			Signatures: []Signature{
				{
					Args:       []*Type{TModuleInst},
					Impl:       Go(importAllHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					// The leading list holds export *names* to rename
					// (`import [Orig Renamed] mod`) — import name syntax,
					// not evaluable expressions. NoEvalArgs keeps them raw
					// (bare words never degrade to data, so without this
					// the unbound names would raise undefined_word).
					Args:       []*Type{TList, TModuleInst},
					NoEvalArgs: map[int]bool{0: true},
					Impl:       Go(importRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TAtom, TModuleInst},
					Impl:       Go(importSingleRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TString},
					Impl:       Go(importFileHandler, RunInCheck()),
					Returns:    []*Type{TModuleInst},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TList, TString},
					NoEvalArgs: map[int]bool{0: true},
					Impl:       Go(importFileRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				// Inline module forms: use /q to capture "module" as a quoted word
				// instead of executing it as a function.
				{
					Args:       []*Type{TAtom, TList},
					QuoteArgs:  map[int]bool{0: true},
					NoEvalArgs: map[int]bool{1: true},
					Impl:       Go(importInlineHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TList, TAtom, TList},
					QuoteArgs:  map[int]bool{1: true},
					NoEvalArgs: map[int]bool{0: true, 2: true},
					Impl:       Go(importInlineRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TAtom, TAtom, TList},
					QuoteArgs:  map[int]bool{1: true},
					NoEvalArgs: map[int]bool{2: true},
					Impl:       Go(importInlineSingleRenameHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
			},
		},
		// ---- export (top-level no-op) ----
		//
		// `export` does its real work only inside an import context, where
		// RunModuleBody registers a collecting handler on the module
		// sub-registry that shadows this one. At the top level — running a
		// file directly (`boru foo.boru`) or in the REPL — there is nowhere
		// to export to, so `export` is a no-op that simply consumes its
		// (name, map) arguments. This lets a single file both run
		// standalone and export a namespace when imported. See §8.3 in the
		// DX report. The branch arity/types mirror the real handler in
		// native_module_module.go so dispatch behaves identically.
		{
			Name: "export",

			Signatures: []Signature{
				{
					Args:       []*Type{TAtom, TMap},
					Impl:       Go(exportNoopHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
				{
					Args:       []*Type{TString, TMap},
					Impl:       Go(exportNoopHandler, RunInCheck()),
					Returns:    []*Type{},
					BarrierPos: -1,
				},
			},
		},

		// timeout / await moved to the boru:time-util module — see
		// native/time_async_module.go.
	}
}

// ---- file I/O handlers ----

// extractPath returns the routing path from a Stream handle or a Pathon. A
// Stream atom (stdin/stdout/stderr) resolves to its internal stream sentinel
// so doRead/doWrite route to the host stream; a Pathon renders to its path
// string. File I/O is Pathon-only — string paths are not accepted — so these
// are the only two shapes reaching here.
func extractPath(v Value) string {
	if sentinel, ok := streamSentinel(v); ok {
		return sentinel
	}
	_as5, _ := AsPathon(v)
	return _as5.String()
}

// returnPath is the value a filesystem word hands back: the Pathon or Stream
// handle the caller passed. Every io target is now a value-carrying micron or
// stream handle, so the input is returned verbatim — the resolved path string
// is used only for the operation itself, never as the return.
func returnPath(v Value) Value {
	return v
}

func readHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	format := formatFromExt(r, path)
	if format == "" {
		format = "text"
	}
	return doRead(r, path, "utf8", format, "lf", nil, args[0].Pos())
}

// readReturns is the check-mode result model for read's Pathon sigs.
// The FORMAT decision — String vs Bytes vs the structured decoders — is
// the handler's own routing over statically-known data (the target's
// extension, the enc/offset/length opts), so with a CONCRETE Pathon
// (and concrete opts, on the opts sig) the model resolves the same
// routing and the residual carries the format's decode type; only the
// TYPE is decided, the content stays a carrier. Anything
// value-dependent — a carrier target (a Stream or handle can never be
// concrete at check time, so those sigs need no model), a computed
// opts map, a format whose decode shape depends on the payload —
// declines to the declared dynamic Any, byte-identical to the bare
// `Returns: [Any]` residual.
func readReturns(withOpts bool) ReturnsFunc {
	dynAny := func() []Value {
		c := NewCarrier(TAny)
		c.Dynamic = true
		return []Value{c}
	}
	return func(args []Value, r *Registry) []Value {
		if r == nil || len(args) < 1 || !IsConcrete(args[0]) {
			return dynAny()
		}
		pi, err := AsPathon(args[0])
		if err != nil {
			return dynAny()
		}
		path := pi.String()
		if withOpts {
			// DEEP-concrete, not merely concrete: this model ROUTES on the
			// options' interior (enc picks Bytes vs String, offset/length
			// pick the positioned read, fmt overrides the extension), and a
			// carrier FIELD inside a concrete map reads as absent — so a
			// computed `{enc: e}` would silently take the utf8 default and
			// claim String where the run produces Bytes. That was a real
			// soundness violation, caught before merge.
			if len(args) < 2 || !DeepConcrete(args[1]) {
				return dynAny()
			}
			// Mirror readOptsHandler: the binary / positioned bypass
			// first ({enc:'bytes'} reads Bytes, {offset}/{length} slices
			// the decoded text), then the explicit-format / extension
			// resolution doRead routes on.
			enc, format, _, _, fmtExplicit, _ := parseFileOpts(args[1])
			if enc == "bytes" || enc == "binary" {
				return []Value{NewCarrier(TBytes)}
			}
			_, hasOffset := mapIntOpt(args[1], "offset")
			_, hasLength := mapIntOpt(args[1], "length")
			if hasOffset || hasLength {
				return []Value{NewCarrier(TString)}
			}
			if !fmtExplicit {
				if extFmt := formatFromExt(r, path); extFmt != "" {
					format = extFmt
				}
			}
			return readFormatResult(format, dynAny)
		}
		format := formatFromExt(r, path)
		if format == "" {
			format = "text"
		}
		return readFormatResult(format, dynAny)
	}
}

// writeEncMirror flags a write whose ENCODING provably fails at run
// time: an unknown {enc:} name, or content carrying a character the
// requested encoding cannot represent (`"€" {enc:'latin1'}`). Both come
// from encodeEnc — doWrite's own encoder, a pure function of
// (content, enc) — so the mirror runs exactly that and wraps it the way
// doWrite does, keeping code and detail byte-identical.
//
// Gated on deep concreteness of the OPTIONS map (a computed {enc:} must
// not be read as a missing one) and on a concrete String content. A
// Bytes payload is written verbatim with no encoding step, so those
// sigs carry no mirror at all.
func writeEncMirror(optsAt int) ReturnsFunc {
	base := writeReturns()
	gate := DeepConcreteOptionsAt(optsAt)
	return MirrorReturns("write",
		func(args []Value) bool {
			if !gate(args) || len(args) <= optsAt || !IsConcrete(args[1]) {
				return false
			}
			// A RESERVED STREAM path never reaches the encoder: doWrite
			// branches on the sentinel and prints the content verbatim
			// (fileio.go, the pathStdout / pathStderr arm), returning
			// before encodeEnc. So `IO.write (make Pathon "<stdout>") "€"
			// {enc:'latin1'}` runs fine, and a mirror that encoded anyway
			// rejected a working program — a false positive caught in
			// review before merge. Reuse the handler's own predicate so
			// the two cannot drift.
			return !isStreamPath(extractPath(args[0]))
		},
		func(args []Value, r *Registry) error {
			// writeOptsHandler's order, top to bottom. The anchor is the
			// HANDLER, not doWrite: writeOptsHandler rejects a read-only
			// {fmt:} before it ever calls doWrite, so `{fmt:"xml"
			// exclusive:true atomic:true}` raises the FORMAT error at run
			// time. Modelling doWrite's first step and calling that the
			// beginning named the option combo instead — the same mistake
			// this file's audit was fixing, one frame up.
			enc, format, mode, _, fmtExplicit, _ := parseFileOpts(args[optsAt])
			if fErr := checkWritableFormat(r, format, fmtExplicit); fErr != nil {
				return fErr
			}
			if cErr := validateWriteOptionCombo(r,
				mapBoolOpt(args[optsAt], "atomic", false),
				mapBoolOpt(args[optsAt], "exclusive", false),
				mode, args[0].Pos()); cErr != nil {
				return cErr
			}
			// A TString slot can hold a DepScalar CONSTRAINT (`String len
			// 5`), which AsConcreteString rejects rather than reading as a
			// zero value — there is no literal text to encode, so the
			// mirror declines and the runtime owns it.
			content, err := args[1].AsConcreteString()
			if err != nil {
				return nil
			}
			if _, encErr := encodeEnc(content, enc); encErr != nil {
				return r.BoruError("write_error", fmt.Sprintf("write: %v", encErr), "write")
			}
			return nil
		}, base)
}

// writeReturns is the check-mode result model for write's Pathon sigs:
// the runtime hands back the target it wrote to VERBATIM (returnPath),
// so a concrete Pathon argument IS the result — returning it keeps the
// construction concrete through a write-then-read chain
// (`IO.read (IO.write p "x")`), where the declared [Pathon] return
// minted a fresh carrier and stranded read's format routing. Identity
// is faithful by the same token: the runtime returns the caller's
// value, not a fresh mint, so the model returning args[0] aliases
// exactly where the interpreter does. A non-concrete target keeps the
// declared fresh carrier.
func writeReturns() ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		if len(args) >= 1 && IsConcrete(args[0]) {
			if _, err := AsPathon(args[0]); err == nil {
				return []Value{args[0]}
			}
		}
		return []Value{NewCarrier(TPathon)}
	}
}

// readFormatResult maps a resolved format name to its decode's result
// type: text yields one String; lines a List of line strings; csv/tsv
// a table-shaped List (TableData rides Parent=TList through both the
// plain and the sqlite-stored wrapping). Every other format's shape
// depends on the payload (a json root may be any node), so those
// decline to the dynamic Any the sig declares.
func readFormatResult(format string, dynAny func() []Value) []Value {
	switch format {
	case "text":
		return []Value{NewCarrier(TString)}
	case "lines":
		return []Value{NewCarrier(TList)}
	case "csv", "tsv":
		return []Value{NewCarrier(TList)}
	}
	return dynAny()
}

func readOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	enc, format, _, nl, fmtExplicit, parserOpts := parseFileOpts(args[1])
	// Binary ({enc:'bytes'}) and positioned ({offset}/{length}) reads bypass
	// format decoding; anything else falls through to the normal decode path.
	if res, handled, err := tryBinaryRead(r, path, enc, args[1]); handled {
		if err != nil {
			return nil, r.BoruError("read_error", fmt.Sprintf("read: %v", err), "read")
		}
		return res, nil
	}
	if !fmtExplicit {
		if extFmt := formatFromExt(r, path); extFmt != "" {
			format = extFmt
		}
	}
	return doRead(r, path, enc, format, nl, parserOpts, args[0].Pos())
}

// Reversed handler for stack-first usage: "path" {opts} read
// In nearest-first stack matching, opts (top) maps to sig[0], path to sig[1].
func readOptsRevHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	return readOptsHandler([]Value{args[1], args[0]}, ctx, stack, r)
}

func writeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	result, err := doWrite(r, path, content, "utf8", "text", "write", "lf", false, false, args[0].Pos())
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

func writeOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content, _ := args[1].AsConcreteString()
	enc, format, mode, nl, fmtExplicit, _ := parseFileOpts(args[2])
	if err := checkWritableFormat(r, format, fmtExplicit); err != nil {
		return nil, err
	}
	result, err := doWrite(r, path, content, enc, format, mode, nl, mapBoolOpt(args[2], "atomic", false), mapBoolOpt(args[2], "exclusive", false), args[0].Pos())
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// checkWritableFormat rejects a write whose explicit {fmt:…} resolves to a
// read-only (parser-backed) format. Write is otherwise extension-blind —
// only an explicit fmt triggers the check — so every existing write call
// (no fmt, or an encodable fmt) is unaffected.
func checkWritableFormat(r *Registry, format string, fmtExplicit bool) error {
	if !fmtExplicit {
		return nil
	}
	f, ok := HostFormats(r)[format]
	if !ok {
		return nil // unknown format is doWrite's concern; nothing read-only to reject
	}
	if ro, ok := f.(ReadOnlyFormat); ok && ro.ReadOnly() {
		return r.BoruErrorHint("write_error",
			fmt.Sprintf("write: format %q is read-only", format), "write",
			"write supports text/json/jsonic/lines/csv/tsv")
	}
	return nil
}

// writeAnyHandler: [path/string/stream, any] -> [path/string/stream].
// Serializes non-string data with no options map required — the value is
// encoded as jsonic (the same default the options form upgrades to).
func writeAnyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	content := valueToJsonic(args[1])
	result, err := doWrite(r, path, content, "utf8", "jsonic", "write", "lf", false, false, args[0].Pos())
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// write: [path/string, any, map] -> [path/string] (for non-string data with fmt)
func writeAnyOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path := extractPath(args[0])
	_, format, mode, nl, fmtExplicit, _ := parseFileOpts(args[2])
	if err := checkWritableFormat(r, format, fmtExplicit); err != nil {
		return nil, err
	}
	if format == "text" {
		format = "jsonic"
	}
	content := valueToJsonic(args[1])
	result, err := doWrite(r, path, content, "utf8", format, mode, nl, mapBoolOpt(args[2], "atomic", false), mapBoolOpt(args[2], "exclusive", false), args[0].Pos())
	if err != nil {
		return result, err
	}
	return []Value{returnPath(args[0])}, nil
}

// ---- help / describe handlers ----

// helpOverviewHandler implements the 0-arg `help` word: a language
// overview plus a pointer at `describe` for per-word docs.
func helpOverviewHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fmt.Fprint(r.Output, help.Overview())
	return nil, nil
}

// describeSelfHandler implements the 0-arg `describe` word: a categorised
// guide to every built-in word and loadable module (the same guide the CLI
// `boru describe` prints).
func describeSelfHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	DescribeIndex(r.Output)
	return nil, nil
}

// describeWordHandler implements `describe <name>` for a word, category,
// module, or module word. The full dispatch — including class/surface schema
// views, module resolution, and load-if-unknown — lives in DescribeName so the
// `/describe` meta-command shares one implementation. See describe.go.
func describeWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	DescribeName(r, r.Output, ValToString(args[0]))
	return nil, nil
}

// formatTypeSchema renders the `describe <Class>` schema view for a
// name def'd to a class or object type. Field rule mirrors make: a
// type-valued constraint declares a required field, a concrete value
// declares a default (whose own type then constrains the field).
// Inherited fields from the refine chain are listed and marked.
func formatTypeSchema(name string, v Value) string {
	info, _ := AsClassType(v)
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s", name, "class")
	if info.Name != "" {
		fmt.Fprintf(&b, " (%s)", info.Name)
	}
	b.WriteString("\n")
	if info.Parent != nil {
		fmt.Fprintf(&b, "\nRefines: %s\n", info.Parent.Name)
	}
	b.WriteString("\nFields:\n")
	all := info.AllFields()
	w := 0
	for _, k := range all.Keys() {
		if len(k) > w {
			w = len(k)
		}
	}
	for _, k := range all.Keys() {
		c, _ := all.Get(k)
		_, own := info.Fields.Get(k)
		inherited := ""
		if !own {
			inherited = "  (inherited)"
		}
		if IsConcrete(c) {
			fmt.Fprintf(&b, "  %-*s : %s = %s  (default)%s\n", w, k, c.Parent.Name(), c.String(), inherited)
		} else {
			fmt.Fprintf(&b, "  %-*s : %s  (required)%s\n", w, k, c.String(), inherited)
		}
	}
	fmt.Fprintf(&b, "\nInstances: make %s {…} — sealed (typed set of existing fields only).\n", name)
	return b.String()
}

// formatSurfaceSchema renders the `describe <Surface>` contract view:
// the required operation shapes and how to declare conformance.
func formatSurfaceSchema(name string, v Value) string {
	info, err := AsSurfaceType(v)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — surface", name)
	if info.Name != "" {
		fmt.Fprintf(&b, " (%s)", info.Name)
	}
	b.WriteString("\n\nRequired operations:\n")
	w := 0
	for _, k := range info.Required.Keys() {
		if len(k) > w {
			w = len(k)
		}
	}
	for _, k := range info.Required.Keys() {
		shape, _ := info.Required.Get(k)
		shapeStr := shape.String()
		if undef, ok := shape.Data.(FnUndefInfo); ok {
			var parts []string
			for _, spec := range undef.Sigs {
				parts = append(parts, renderSpec(spec))
			}
			shapeStr = strings.Join(parts, ", ")
		}
		fmt.Fprintf(&b, "  %-*s : %s\n", w, k, shapeStr)
	}
	fmt.Fprintf(&b, "\nConformance: <Type> exposes %s — explicit, checked loudly at declaration.\n", name)
	return b.String()
}

// referentHandler implements `referent`: given an atom, return what its name
// refers to. A captured snapshot (from `quote` or the load-time resolution
// pass) is returned as-is; otherwise the atom's name is resolved against the
// current bindings (the lazy fallback, which covers a bare `name/q` whose
// binding was made at runtime). An atom whose name is unbound is an error.
func referentHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	v := args[0]
	if ref, ok := AtomReferent(v); ok {
		return []Value{ref}, nil
	}
	name, err := AsAtom(v)
	if err != nil {
		return nil, r.BoruError("referent_error", "referent: expected an atom", "referent")
	}
	if bound, ok := r.Defs.Top(name); ok {
		return []Value{bound}, nil
	}
	return nil, r.BoruError("referent_error",
		fmt.Sprintf("referent: atom %q has no referent (name is unbound)", name), "referent")
}

// ---- module / import handlers ----

// exportNoopHandler is the top-level `export` handler: it discards the
// export name and map, producing no value. The real collecting handler
// is registered per-module in RunModuleBody and shadows this one. See
// the "export (top-level no-op)" entry above and §8.3 in the DX report.
//
// In CHECK mode it does one thing before discarding: it records every export
// value as a use of its def. A standalone module file (`boru check trie.boru`)
// reaches this no-op, not the collecting handler — so without this, every
// reference-exported public word (`export "X" { make: impl/v }`) is falsely
// flagged unused_def precisely because it is public. The map arrives already
// auto-evaluated, so `name/v` values are fn data whose name is read off the
// FnDefInfo; a bare name resolves through ResolveRef (which also records it).
func exportNoopHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if r == nil || !r.Check.IsActive() || len(args) < 2 || !IsConcrete(args[1]) {
		return nil, nil
	}
	m, err := RequireConcreteMap(args[1], "export")
	if err != nil {
		return nil, nil
	}
	for _, key := range m.Keys() {
		v, _ := m.Get(key)
		if fnDef, ok := v.Data.(FnDefInfo); ok && fnDef.Name != "" {
			r.Check.RecordUse(fnDef.Name)
			continue
		}
		switch {
		case IsWord(v):
			w, _ := AsWord(v)
			r.Check.RecordUse(w.Name)
		case IsAtom(v):
			a, _ := AsAtom(v)
			r.Check.RecordUse(a)
		case v.Parent.ConformsTo(TString):
			// A bare name in an export map can survive as a String after
			// auto-eval; treat its text as the referenced name.
			if s, sok := AsString(v); sok == nil {
				r.Check.RecordUse(s)
			}
		}
	}
	return nil, nil
}

func moduleHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("module_error", "module: argument must be a concrete list, got type literal", "module")
	}
	_lst, _ := AsList(args[0])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("module: %w", err)
	}
	return []Value{NewModuleInstance(desc)}, nil
}

func importAllHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[0])
	return nil, installExports(r, desc, nil)
}

func importRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[1])
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("import_error", "import: rename list must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

func importSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	desc, _ := asModuleDesc(args[1])
	newName, _ := args[0].AsConcreteAtom()
	return nil, installSingleRename(r, desc, newName)
}

func importFileHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[0].AsConcreteString()
	// In check mode the path string literal has been stripped to a
	// carrier (StripToCarriers), so `path` is empty. We can't read,
	// parse, or analyse the target file without it — but the importing
	// file itself is valid, so a hard error here would block `boru check`
	// on every file that imports a sibling. Treat the import as opaque:
	// return a Module carrier and let analysis continue (imported names
	// resolve to Any, which is the conservative check-mode default).
	if path == "" && r.Check.IsActive() {
		return []Value{NewCarrier(TModuleInst)}, nil
	}
	// In check mode the path literal is preserved (see StripToCarriers /
	// §4.3) so the importing file's cross-module references can be
	// resolved. Loading the target binds its export namespaces, so
	// `Pkg.word` no longer flags undefined_word. But the target may be
	// missing, unparseable, or itself fail to load — none of which should
	// block `boru check` on the importing file. On any such error, fall
	// back to an opaque Module carrier and let analysis continue.
	if r.Check.IsActive() {
		if err := loadImportForCheck(r, path); err != nil {
			return []Value{NewCarrier(TModuleInst)}, nil
		}
		return nil, nil
	}
	if isNativeModImport(path) {
		return nil, resolveNativeMod(r, path)
	}
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, err
		}
		return nil, installExports(r, desc, nil)
	}
	if isDataFile(r, path) {
		return loadDataFile(r, path)
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	return nil, installExports(r, desc, nil)
}

// loadImportForCheck resolves an import in check mode for its export
// symbols only. It mirrors the runtime import paths (native module,
// bare module, file module) but is the single guarded entry the
// check-mode branch of importFileHandler uses so a failure to load the
// target degrades to an opaque module rather than a hard check error.
// Data-file imports are skipped (no exports to bind). See §4.3.
func loadImportForCheck(r *Registry, path string) error {
	if isNativeModImport(path) {
		return resolveNativeMod(r, path)
	}
	if isDataFile(r, path) {
		return nil
	}
	resolved := path
	if !isFilePath(path) {
		var err error
		resolved, err = resolveBareModule(r, path)
		if err != nil {
			return err
		}
	}
	desc, err := loadFileModule(r, resolved)
	if err != nil {
		return err
	}
	return installExports(r, desc, nil)
}

func importFileRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	path, _ := args[1].AsConcreteString()
	// Check mode: the path literal was stripped to a carrier. Skip the
	// import gracefully so `boru check` doesn't hard-fail (see
	// importFileHandler for the full rationale).
	if path == "" && r.Check.IsActive() {
		return nil, nil
	}
	if !isFilePath(path) {
		resolved, err := resolveBareModule(r, path)
		if err != nil {
			return nil, err
		}
		desc, err := loadFileModule(r, resolved)
		if err != nil { //covergate:allow native handler defensive error-propagation / same-assertion guard (§native)
			return nil, err
		}
		_lst, _ := AsList(args[0])
		return nil, installRenamedExports(r, desc, _lst.Slice())
	}
	if isDataFile(r, path) {
		return nil, r.BoruError("import_error", fmt.Sprintf("import: rename not supported for data files (%s)", path), "import")
	}
	desc, err := loadFileModule(r, path)
	if err != nil {
		return nil, err
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

// import: [atom/q list] -> [] — inline module: import module [body]
// The /q captures "module" as a quoted word; the handler runs the body
// to produce a module descriptor, then imports all exports.
func importInlineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[0])
	if name != "module" {
		return nil, r.BoruError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", name), "import")
	}
	if !IsConcrete(args[1]) {
		return nil, r.BoruError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[1])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	return nil, installExports(r, desc, nil)
}

func importInlineRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := defName(args[1])
	if name != "module" {
		return nil, r.BoruError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", name), "import")
	}
	if !IsConcrete(args[0]) {
		return nil, r.BoruError("import_error", "import: rename list must be a concrete list, got type literal", "import")
	}
	if !IsConcrete(args[2]) {
		return nil, r.BoruError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst2, _ := AsList(args[2])
	desc, err := RunModuleBody(r, _lst2.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	_lst, _ := AsList(args[0])
	return nil, installRenamedExports(r, desc, _lst.Slice())
}

func importInlineSingleRenameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	modName := defName(args[1])
	if modName != "module" {
		return nil, r.BoruError("import_error", fmt.Sprintf("import: unknown inline form %q (expected 'module')", modName), "import")
	}
	if !IsConcrete(args[2]) {
		return nil, r.BoruError("import_error", "import: module body must be a concrete list, got type literal", "import")
	}
	_lst, _ := AsList(args[2])
	desc, err := RunModuleBody(r, _lst.Slice())
	if err != nil {
		return nil, fmt.Errorf("import module: %w", err)
	}
	return nil, installSingleRename(r, desc, defName(args[0]))
}

// ---- temporal handlers ----

func timeoutListHandler(tt TemporalModuleTypes) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doTimeout(tt, r, args, true)
	}
}

func timeoutWordHandler(tt TemporalModuleTypes) Handler {
	return func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doTimeout(tt, r, args, false)
	}
}

func doTimeout(tt TemporalModuleTypes, r *Registry, args []Value, isList bool) ([]Value, error) {
	ms, _ := args[0].AsConcreteInteger()
	if ms < 0 {
		return nil, r.BoruError("timeout_error", fmt.Sprintf("timeout: milliseconds must be non-negative, got %d", ms), "timeout")
	}
	callback := args[1]

	id := GenerateID("T_")
	// Fork now, on the scheduling goroutine, so the callback runs on an
	// isolated registry and cannot race the main interpreter when it
	// fires later on the timer goroutine.
	fork := r.ForkConcurrent()
	timer := time.AfterFunc(time.Duration(ms)*time.Millisecond, func() {
		RunTimerCallback(fork, callback, isList)
	})

	info := &TimeoutInfo{
		ID:    id,
		Ms:    ms,
		Timer: timer,
	}
	return []Value{tt.NewTimeout(info)}, nil
}

func awaitWithOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	// args[0] = Options, args[1] = List (parallels)
	return doAwait(r, awaitOptsMode(args[0]), args[1])
}

func awaitDefaultHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return doAwait(r, "all", args[0])
}

func doAwait(r *Registry, mode string, parallels Value) ([]Value, error) {
	if !IsConcrete(parallels) {
		return nil, r.BoruError("await_error", "await: parallels must be a concrete list, got type literal", "await")
	}
	_lst, _ := AsList(parallels)
	elems := _lst.Slice()
	if len(elems) == 0 {
		return []Value{NewList([]Value{})}, nil
	}

	run := awaitRunner(mode)
	if run == nil {
		return nil, awaitUnknownMode(r, mode)
	}
	return run(r, elems)
}
