package native

import (
	"fmt"
	"sort"

	core "github.com/boru-lang/boru/core/go"
)

// IOModuleNativeFuncs builds the input/output words that were moved out
// of the core registry into the loadable `boru:io` module (namespace
// `IO`). They are registered ONLY into that module's sub-registry by
// modules.BuildIOModule — deliberately absent from the global registry.
// Most handlers live in their feature files (native_print.go's
// core.PrintstrHandler, native_trace.go's core.TraceHandler,
// native_misc.go's read/write in this package).
//
// `streamKind` is the per-import StreamKind type (minted by
// MintStreamKind into the module's sub-registry). It tags the
// stdin/stdout/stderr handles and types the read/write Stream
// signatures, so a stream target is type-distinct from a file path —
// without StreamKind being a global builtin. The stdin/stdout/stderr
// handlers are closures over it so each module instance stamps its own
// StreamKind onto the handles it returns.
//
// `print` is NOT here: it stays a core word so basic output needs no import.
//
// FILE I/O IS PATHON-ONLY. Every filesystem target is a `Scalar/Micron/Pathon`
// — string paths are NOT accepted (write `IO.read (make Pathon "data.csv")`,
// not `IO.read "data.csv"`). This keeps a file target type-distinct from an
// arbitrary string and from a stream handle, so one polymorphic verb dispatches
// unambiguously on the argument's TYPE. read / write additionally accept a
// StreamKind handle (stdin / stdout / stderr).
//
// `list` and `remove` are NOT here: they are exported as POLYMORPHIC WORD
// EXTENSIONS of the core `list` / `remove` words (built in modules/io.go via
// NewWordExtensionAnchored), so after `import "boru:io"` the BARE words gain a
// Pathon overload — `list somePath` / `remove somePath` — rather than a
// separate IO.list / IO.remove. Their handlers (listHandler / doRemoveWord)
// live in io_fs.go and are shared with those extensions.
//
// At dispatch the module FnDef wrapper short-circuits to the inner native's
// handler, which runs against the LIVE engine registry (execMatch passes
// e.registry, not the sub-registry) — so IO.read / IO.write reach the host
// FileOps and IO.stdout / IO.trace / IO.printstr reach the host Output writer
// exactly as the former core words did.
//
//	printstr  write a value's formatted form without a trailing newline
//	read      read a file (Pathon) or stream (StreamKind); optional options map
//	write     write a file (Pathon) or stream (StreamKind); value; optional options map
//	stdin     the standard-input stream handle (a StreamKind atom)
//	stdout    the standard-output stream handle (a StreamKind atom)
//	stderr    the standard-error stream handle (a StreamKind atom)
//	trace     run a list as a sub-program with step-by-step tracing
//	folder    create / list a filesystem folder (Pathon; optional options)
//	stat      describe a Pathon, returning a FileInfo record or `none`
//	move      rename/move a Pathon to a Pathon destination
//	copy      copy a Pathon to a Pathon destination ({recursive} for a tree)
//	link      create a symbolic (or {hard}) link at a Pathon destination
//	touch     create/update a Pathon and apply {mode,mtime,atime,size}
//	watch     run a body per change event on a Pathon (returns a Watcher)
//	unwatch   stop a Watcher, closing its event stream
//	mount     install a map of boru handler fns as the filesystem
//	unmount   restore the filesystem that was active before mount
//
// IOModuleTypes bundles the per-import module-minted types the io words
// close over: StreamKind (stdin/stdout/stderr handle tag), FileType (the
// file/dir/symlink/other stat-record atom enum), Watcher (a live watch
// subscription), and File (a stateful open-file handle). P5 will add Lock
// and Mmap here. Bundling them keeps IOModuleNativeFuncs' signature stable
// as the resource surface grows.
type IOModuleTypes struct {
	StreamKind *Type
	FileType   *Type
	Watcher    *Type
	File       *Type
	Lock       *Type
	Mmap       *Type
}

func IOModuleNativeFuncs(t IOModuleTypes) []NativeFunc {
	streamKind, fileType, watcherType, handleType := t.StreamKind, t.FileType, t.Watcher, t.File
	lockType, mmapType := t.Lock, t.Mmap
	streamHandle := func(name string) NativeFunc {
		return NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Args: []*Type{},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return []Value{newStreamAtom(name, streamKind)}, nil
				}),
				Returns: []*Type{streamKind}, BarrierPos: -1,
			}},
		}
	}
	// statImpl closes over fileType so the stat handler can tag records with
	// the module's FileType atoms.
	statImpl := func(hasOpts bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			return statHandler(a, r, fileType, hasOpts)
		}
	}
	// watchImpl closes over watcherType so each import's watch handles
	// carry its own Watcher identity. watchOptsImpl threads the {recursive
	// match} options map (the third arg) through to the same handler.
	watchImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doWatchWord(a, r, watcherType, Value{})
	}
	watchOptsImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doWatchWord(a, r, watcherType, a[2])
	}
	unwatchImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doUnwatchWord(a, r)
	}
	// openImpl closes over handleType so each import stamps its own File
	// identity; seek/flush/close/read-handle/write-handle reach the handle
	// payload structurally, so they need no type.
	openImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doOpenWord(a, r, handleType)
	}
	seekImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doSeekWord(a, r)
	}
	flushImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doFlushWord(a, r)
	}
	closeImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doCloseWord(a, r)
	}
	readHandleImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return readHandleWord(a, r)
	}
	writeHandleImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return writeHandleWord(a, r)
	}
	lockImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doLockWord(a, r, lockType)
	}
	unlockImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doUnlockWord(a, r)
	}
	mmapImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return doMmapWord(a, r, mmapType)
	}
	readMmapImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return readMmapWord(a, r)
	}
	writeMmapImpl := func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		return writeMmapWord(a, r)
	}
	return []NativeFunc{
		{
			Name: "printstr",
			Signatures: []Signature{{
				Args:    []*Type{TAny},
				Impl:    Go(core.PrintstrHandler),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		},
		{
			Name: "read",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(readOptsHandler), Returns: []*Type{TAny}, ReturnsFn: readReturns(true), BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(readHandler), Returns: []*Type{TAny}, ReturnsFn: readReturns(false), BarrierPos: -1},
				{Args: []*Type{streamKind, TMap}, Impl: Go(readOptsHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{streamKind}, Impl: Go(readHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				// File-handle reads: {offset}/{length}/{enc} slice the handle.
				{Args: []*Type{handleType, TMap}, Impl: Go(readHandleImpl), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{handleType}, Impl: Go(readHandleImpl), Returns: []*Type{TAny}, BarrierPos: -1},
				// Mmap-region reads: {offset}/{length}/{enc} window the map.
				{Args: []*Type{mmapType, TMap}, Impl: Go(readMmapImpl), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{mmapType}, Impl: Go(readMmapImpl), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, TPathon}, Impl: Go(readOptsRevHandler), Returns: []*Type{TAny}, BarrierPos: -1},
				{Args: []*Type{TMap, streamKind}, Impl: Go(readOptsRevHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			},
		},
		{
			// write returns the target it wrote to (the Pathon, or the Stream
			// handle for a standard stream), so the result can be threaded
			// straight into read.
			Name: "write",
			Signatures: []Signature{
				// Binary writes: a Bytes payload is written verbatim (more
				// specific than the TAny/TString sigs, so it wins dispatch).
				{Args: []*Type{TPathon, TBytes, TMap}, Impl: Go(writeBytesOptsHandler), Returns: []*Type{TPathon}, ReturnsFn: writeReturns(), BarrierPos: -1},
				{Args: []*Type{TPathon, TBytes}, Impl: Go(writeBytesHandler), Returns: []*Type{TPathon}, ReturnsFn: writeReturns(), BarrierPos: -1},
				{Args: []*Type{streamKind, TBytes, TMap}, Impl: Go(writeBytesOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TBytes}, Impl: Go(writeBytesHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{TPathon, TString, TMap}, Impl: Go(writeOptsHandler), Returns: []*Type{TPathon}, ReturnsFn: writeEncMirror(2), BarrierPos: -1},
				{Args: []*Type{TPathon, TAny, TMap}, Impl: Go(writeAnyOptsHandler), Returns: []*Type{TPathon}, ReturnsFn: writeReturns(), BarrierPos: -1},
				{Args: []*Type{TPathon, TString}, Impl: Go(writeHandler), Returns: []*Type{TPathon}, ReturnsFn: writeReturns(), BarrierPos: -1},
				{Args: []*Type{TPathon, TAny}, Impl: Go(writeAnyHandler), Returns: []*Type{TPathon}, ReturnsFn: writeReturns(), BarrierPos: -1},
				{Args: []*Type{streamKind, TString, TMap}, Impl: Go(writeOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny, TMap}, Impl: Go(writeAnyOptsHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TString}, Impl: Go(writeHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				{Args: []*Type{streamKind, TAny}, Impl: Go(writeAnyHandler), Returns: []*Type{streamKind}, BarrierPos: -1},
				// File-handle writes return the handle (for threading); a Bytes
				// payload writes verbatim, {offset} positions the write.
				{Args: []*Type{handleType, TAny, TMap}, Impl: Go(writeHandleImpl), Returns: []*Type{handleType}, BarrierPos: -1},
				{Args: []*Type{handleType, TAny}, Impl: Go(writeHandleImpl), Returns: []*Type{handleType}, BarrierPos: -1},
				// Mmap-region writes splice into the map in place ({offset}),
				// returning the region.
				{Args: []*Type{mmapType, TAny, TMap}, Impl: Go(writeMmapImpl), Returns: []*Type{mmapType}, BarrierPos: -1},
				{Args: []*Type{mmapType, TAny}, Impl: Go(writeMmapImpl), Returns: []*Type{mmapType}, BarrierPos: -1},
			},
		},
		streamHandle("stdin"),
		streamHandle("stdout"),
		streamHandle("stderr"),
		{
			// script-args (exported as IO.args) returns the host-installed
			// script positional arguments — everything after the script
			// path on the CLI — as a List of Strings; an empty list when
			// the host set none (REPL, -e, embedded). The inner name is
			// NOT "args": the core fn-arguments word `args` lives in the
			// module's sub-registry too, and sharing its name would merge
			// signature sets; BuildIOModule renames on export.
			Name: "script-args",
			Signatures: []Signature{{
				Args:    []*Type{},
				Impl:    Go(scriptArgsHandler),
				Returns: []*Type{TList}, BarrierPos: -1,
			}},
		},
		{
			// env (exported as IO.env) reads ONE environment variable by
			// name, yielding its value or none. "" is a real value distinct
			// from unset, which is why a missing name is none rather than ""
			// — a program testing `is None` can tell the difference.
			//
			// No host installation reads as "nothing is set", NOT as the
			// real process environment: the runtime never reaches for
			// ambient state the host did not hand it.
			//
			// The whole-environment snapshot is a SEPARATE word (env-all)
			// rather than the 0-arg overload design/CLI-PROGRAMS.0.md §3
			// sketched. The overload was originally impossible — a 0-arg
			// signature won unconditionally on a module export and made the
			// arg-taking ones unreachable (NUR035, since fixed in the engine:
			// a paren expression resolving to a function word no longer
			// induces a call, so a mixed-arity export is dispatchable —
			// lang/spec/fn-value.tsv §4 pins it). The two words stay because
			// the split is better in its own right: each arity is a named,
			// separately describable word rather than one name whose meaning
			// flips with the argument count, and `env-all`'s cost — enumerating
			// the whole environment — is visible at the call site. §3's shape
			// is available again if that judgement is ever revisited.
			Name: "env",
			Signatures: []Signature{{
				Args:    []*Type{TString},
				Impl:    Go(envLookupHandler),
				Returns: []*Type{TAny}, ReturnsFn: ReturnsDynUnion(TString, TNone), BarrierPos: -1,
			}},
		},
		{
			// read-line (exported as IO.read-line) yields the next line of a
			// stream or File handle without its terminator, or none at EOF.
			//
			// none rather than "" at EOF is what makes the filter loop
			// terminate: "" is a legitimate line (a blank one), so a caller
			// must be able to tell an empty line from no more lines. A final
			// line with no trailing newline IS a line — dropping it would lose
			// data from every file an editor left unterminated.
			//
			// Two signatures, no 0-arg convenience form meaning "stdin": the
			// stream is always named, so a filter loop reads the same whether
			// its input is stdin or a file, and switching between them is one
			// token. (Such a form was also unreachable when this shipped —
			// a 0-arg signature won unconditionally on a module export,
			// NUR035, since fixed in the engine.)
			Name: "read-line",
			Signatures: []Signature{
				{
					Args:    []*Type{streamKind},
					Impl:    Go(readLineHandler),
					Returns: []*Type{TAny}, ReturnsFn: ReturnsDynUnion(TString, TNone), BarrierPos: -1,
				},
				{
					Args:    []*Type{handleType},
					Impl:    Go(readLineHandler),
					Returns: []*Type{TAny}, ReturnsFn: ReturnsDynUnion(TString, TNone), BarrierPos: -1,
				},
			},
		},
		{
			// is-tty (exported as IO.is-tty) reports whether a stream is a
			// terminal. Per-stream because the interesting cases are
			// asymmetric: stdout piped while stderr is still a terminal is the
			// normal shape of a program in a pipeline.
			//
			// NOT `tty?`, which design/CLI-PROGRAMS.0.md §5 specifies: `?` is
			// rejected in a word name outright (eng/go/word_name.go), whose doc
			// names the reason and the alternative — "the Lisp/Scheme/Ruby `?`
			// predicate suffix is NOT allowed; predicates use the `is-` prefix
			// convention instead". `is-finite` / `is-inf` / `is-nan` are the
			// shipped precedents.
			//
			// No probe installed answers false, which is what makes this
			// hermetic in the spec suite and honest under a profile that
			// uninstalls the terminal scope.
			Name: "is-tty",
			Signatures: []Signature{{
				Args:    []*Type{streamKind},
				Impl:    Go(ttyHandler),
				Returns: []*Type{TBoolean}, BarrierPos: -1,
			}},
		},
		{
			// exit (exported as IO.exit) asks the DRIVER to terminate with a
			// status. It raises the reserved `boru/exit` control error rather
			// than calling os.Exit: the runtime does not own the process, and
			// an embedded host must be able to decide for itself (it reads the
			// code back with lang.ExitCode). The CLI, a built binary, and
			// `boru test` each recognise it; a sub-engine boundary converts it
			// to an ordinary error so a sandbox cannot terminate its host.
			//
			// The range is 0..125, the shell convention: 126/127 mean
			// "found but not executable" / "not found", and 128+n means
			// "killed by signal n". A program that returns one of those is
			// lying about how it died, so they are declined at the call.
			Name: "exit",
			Signatures: []Signature{{
				Args:      []*Type{TInteger},
				Impl:      Go(exitHandler),
				ReturnsFn: exitCodeMirror(),
				Returns:   []*Type{}, BarrierPos: -1,
			}},
		},
		{
			// env-all (exported as IO.env-all) snapshots every visible
			// variable as a Map — the enumerating half of EnvOps, and the
			// only witness for `printenv` with no arguments. 0-arg only,
			// exactly like script-args.
			Name: "env-all",
			Signatures: []Signature{{
				Args:    []*Type{},
				Impl:    Go(envAllHandler),
				Returns: []*Type{TMap}, BarrierPos: -1,
			}},
		},
		{
			Name: "trace",
			Signatures: []Signature{{
				Args:    []*Type{TList},
				Impl:    Go(core.TraceHandler),
				Returns: []*Type{TAny}, BarrierPos: -1,
			}},
		},
		{
			Name: "folder",
			Signatures: []Signature{
				{Args: []*Type{TOptions, TPathon}, Impl: Go(folderOptsHandler), Returns: []*Type{TList}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(folderHandler), Returns: []*Type{TList}, BarrierPos: -1},
			},
		},
		{
			// stat returns a FileInfo record (or none when absent). The
			// target is a Pathon; an optional map carries {follow, resolve}.
			Name: "stat",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(statImpl(true)), Returns: []*Type{TAny}, ReturnsFn: statShapeReturns(fileType), BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(statImpl(false)), Returns: []*Type{TAny}, ReturnsFn: statShapeReturns(fileType), BarrierPos: -1},
			},
		},
		{
			// move renames/moves src to dst (both Pathon), returning dst.
			// {overwrite:false} declines an existing destination.
			Name: "move",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon, TMap}, Impl: Go(moveOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TPathon}, Impl: Go(moveHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			},
		},
		{
			// copy copies src to dst (both Pathon), returning dst.
			// {recursive:true} copies a directory tree.
			Name: "copy",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon, TMap}, Impl: Go(copyOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TPathon}, Impl: Go(copyHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			},
		},
		{
			// link creates a link at dst referring to src (both Pathon) — a
			// symbolic link by default, a hard link with {hard:true}. Returns dst.
			Name: "link",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TPathon, TMap}, Impl: Go(linkOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon, TPathon}, Impl: Go(linkHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			},
		},
		{
			// watch subscribes to change events on a Pathon (a file, a
			// directory's direct children, or — with {recursive:true} — the
			// whole subtree) and runs the body once per event with the {op,
			// path} record on the stack. {match:"glob"} filters events by
			// path; the coalesced overflow marker always gets through.
			// Returns a Watcher.
			Name: "watch",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TList, TMap}, NoEvalArgs: map[int]bool{1: true}, Impl: Go(watchOptsImpl), Returns: []*Type{watcherType}, BarrierPos: -1},
				{Args: []*Type{TPathon, TList}, NoEvalArgs: map[int]bool{1: true}, Impl: Go(watchImpl), Returns: []*Type{watcherType}, BarrierPos: -1},
			},
		},
		{
			// unwatch stops a Watcher, releasing the subscription.
			Name: "unwatch",
			Signatures: []Signature{
				{Args: []*Type{watcherType}, Impl: Go(unwatchImpl), Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			// open acquires a stateful File handle on a Pathon. The options
			// map carries {mode:read|write|append|rw, create, exclusive,
			// truncate, perm}; the default is a read-only open.
			Name: "open",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(openImpl), Returns: []*Type{handleType}, ReturnsFn: openModeMirror(handleType), BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(openImpl), Returns: []*Type{handleType}, BarrierPos: -1},
			},
		},
		{
			// seek moves a File handle's cursor to n, relative to
			// {from:start|current|end} (default start), and returns the new
			// absolute offset.
			Name: "seek",
			Signatures: []Signature{
				{Args: []*Type{handleType, TInteger, TMap}, Impl: Go(seekImpl), Returns: []*Type{TInteger}, BarrierPos: -1},
				{Args: []*Type{handleType, TInteger}, Impl: Go(seekImpl), Returns: []*Type{TInteger}, BarrierPos: -1},
			},
		},
		{
			// flush syncs a File handle to its backend or flushes a writable
			// Mmap region's changes, returning the handle (polymorphic).
			Name: "flush",
			Signatures: []Signature{
				{Args: []*Type{handleType}, Impl: Go(flushImpl), Returns: []*Type{handleType}, BarrierPos: -1},
				{Args: []*Type{mmapType}, Impl: Go(flushImpl), Returns: []*Type{mmapType}, BarrierPos: -1},
			},
		},
		{
			// close releases a resource handle — a File, Watcher, Lock, or
			// Mmap. TYPED sigs (not one TAny) keep dispatch unambiguous under
			// forward collection and give a static type error for a
			// non-resource; the handler type-switches over all four.
			Name: "close",
			Signatures: []Signature{
				{Args: []*Type{handleType}, Impl: Go(closeImpl), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{watcherType}, Impl: Go(closeImpl), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{lockType}, Impl: Go(closeImpl), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{mmapType}, Impl: Go(closeImpl), Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			// lock takes an advisory lock on a Pathon — {shared:true} for a
			// read lock, {block:false} to return `none` on contention instead
			// of waiting. Returns a Lock (or none).
			Name: "lock",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(lockImpl), Returns: []*Type{TAny}, ReturnsFn: ReturnsDynUnion(lockType, TNone), BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(lockImpl), Returns: []*Type{TAny}, ReturnsFn: ReturnsDynUnion(lockType, TNone), BarrierPos: -1},
			},
		},
		{
			// unlock releases an advisory Lock (the Lock-specific twin of the
			// polymorphic close).
			Name: "unlock",
			Signatures: []Signature{
				{Args: []*Type{lockType}, Impl: Go(unlockImpl), Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			// mmap maps a file's bytes into a Mmap region; {offset, length,
			// writable} shape the mapping. read/write/flush/close drive it.
			Name: "mmap",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(mmapImpl), Returns: []*Type{mmapType}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(mmapImpl), Returns: []*Type{mmapType}, BarrierPos: -1},
			},
		},
		{
			// mount installs a filesystem as the host FileOps; unmount
			// restores the previous one. A handler MAP is the boru→FileOps
			// bridge; a Pathon (a ".zip" path or {zip:true}) mounts a
			// read-only ZIP archive, or a copy-on-write overlay over it with
			// {writable:true}. See io_mount.go for both contracts.
			Name: "mount",
			Signatures: []Signature{
				{Args: []*Type{TMap}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					return doMountWord(a, r)
				}), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TPathon, TMap}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					return doMountZipWord(a, r, a[1])
				}), Returns: []*Type{}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					return doMountZipWord(a, r, Value{})
				}), Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			Name: "unmount",
			Signatures: []Signature{
				{Args: []*Type{}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					return doUnmountWord(a, r)
				}), Returns: []*Type{}, BarrierPos: -1},
			},
		},
		{
			// temp creates a unique temp file (or directory with {dir:true}),
			// optionally inside {in:Pathon} named {prefix}…{suffix}, and
			// returns its Pathon. The options map is REQUIRED ({} for the
			// defaults): a 0-arg sig on a module word would fire eagerly on
			// the dot-access value path before forward collection, stranding
			// the options (the execFnDefLiteral data-vs-call gate).
			Name: "temp",
			Signatures: []Signature{
				{Args: []*Type{TMap}, Impl: Go(tempOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			},
		},
		{
			// space reports the volume holding a Pathon: {total free
			// available bsize type}. Numbers are backend-defined (the mem
			// filesystem reports a synthetic volume).
			Name: "space",
			Signatures: []Signature{
				{Args: []*Type{TPathon}, Impl: Go(spaceHandler), Returns: []*Type{TMap}, ReturnsFn: spaceShapeReturns(), BarrierPos: -1},
			},
		},
		{
			// touch creates a Pathon if absent and applies metadata options
			// {mode, mtime, atime, size} (folding chmod/utimes/truncate).
			// Returns the path.
			Name: "touch",
			Signatures: []Signature{
				{Args: []*Type{TPathon, TMap}, Impl: Go(touchOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
				{Args: []*Type{TPathon}, Impl: Go(touchHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			},
		},
	}
}

// IOWordExtensions builds the WORD-EXTENSION clones the boru:io module exports
// for the core `list` and `remove` words: after `import "boru:io"` the bare
// words gain a Pathon overload so `list somePath` enumerates a directory and
// `remove somePath` deletes a filesystem path — polymorphism over the existing
// verbs rather than a separate IO.list / IO.remove.
//
// These anchor on Pathon, a KERNEL builtin type. The module-scope safety rule
// (requireUserTypedSigs) normally declines a core-word extension whose tuple has
// no user-minted type; NewWordExtensionAnchored waives it because boru:io ships
// and versions WITH the kernel (see eng/go/word_extend.go). The core `list` /
// `remove` sigs match only Map/ResourceEntity/List, disjoint from Pathon, so
// dispatch is unambiguous. listHandler closes over fileType so a
// `list p {detail:true}` record's `type` field is an IO.FileType atom.
func IOWordExtensions(fileType *Type) []FnDefInfo {
	listImpl := func(hasOpts bool) func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
		return func(a []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
			return listHandler(a, r, fileType, hasOpts)
		}
	}
	return []FnDefInfo{
		NewWordExtension(core.OwnerKernel, "list", []Signature{
			{Args: []*Type{TPathon, TMap}, Impl: Go(listImpl(true)), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TPathon}, Impl: Go(listImpl(false)), Returns: []*Type{TList}, BarrierPos: -1},
		}),
		NewWordExtension(core.OwnerKernel, "remove", []Signature{
			{Args: []*Type{TPathon, TMap}, Impl: Go(ioRemoveOptsHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
			{Args: []*Type{TPathon}, Impl: Go(ioRemoveHandler), Returns: []*Type{TPathon}, BarrierPos: -1},
		}),
	}
}

// scriptArgsHandler renders the host-installed script arguments
// (SetHostScriptArgs) as a List of Strings. No host installation reads
// as an empty list — a script can always iterate IO.args.
func scriptArgsHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	hostArgs := HostScriptArgs(r)
	vals := make([]Value, len(hostArgs))
	for i, a := range hostArgs {
		vals[i] = NewString(a)
	}
	return []Value{NewList(vals)}, nil
}

// envLookupHandler reads one environment variable. Unset (or denied by the
// policy, or no host installation) is none — never "", which is a legal
// value a caller must be able to distinguish.
func envLookupHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruErrorAt("env_error", "env: name must be a String", "env", args[0].Pos())
	}
	ops := HostEnvOps(r)
	if ops == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	v, ok := ops.Get(name)
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{NewString(v)}, nil
}

// envAllHandler snapshots every visible variable as a Map, sorted by name.
// Uninstalled or fully denied reads as an empty Map, so a caller can iterate
// unconditionally — the same contract IO.args offers with its empty list.
func envAllHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	om := NewOrderedMap()
	if ops := HostEnvOps(r); ops != nil {
		names := ops.All()
		sort.Strings(names)
		for _, n := range names {
			if v, ok := ops.Get(n); ok {
				om.Set(n, NewString(v))
			}
		}
	}
	return []Value{NewMap(om)}, nil
}

// exitHandler raises the reserved exit control error. It never returns a
// value: the whole point is that execution stops here and the driver
// decides what the status means.
// validateExitCode is exitHandler's PURE prefix: the two compile failures
// decidable from the code VALUE alone. Split out so the check-mode
// mirror can run exactly this and nothing else — the handler's tail
// raises the boru/exit CONTROL error, which is how `exit` works and
// must never be mirrored as a program fault.
func validateExitCode(v Value, r *Registry) error {
	code, err := v.AsConcreteInteger()
	if err != nil {
		return r.BoruErrorAt("exit_error", "exit: code must be an Integer",
			"exit", v.Pos())
	}
	if code < 0 || code > 125 {
		return r.BoruErrorHintAt("exit_error",
			"exit: code must be 0..125", "exit",
			"126 and 127 are reserved for the shell (not executable / not found) "+
				"and 128+n means killed by signal n — a program that returns one "+
				"of those misreports how it died", v.Pos())
	}
	return nil
}

// exitCodeMirror flags an out-of-range literal exit code at check time.
// The gate is the ordinary concrete one (a scalar has no interior, so
// DeepConcrete and IsConcrete agree here), and the validator stops
// before the control-error raise.
func exitCodeMirror() ReturnsFunc {
	return MirrorReturns("exit", DeepConcreteOptions,
		func(args []Value, r *Registry) error { return validateExitCode(args[0], r) },
		ReturnsStatic())
}

// openModeMirror flags an unknown {mode:} at check time by running
// openOptsFromMap — doOpenWord's own first step, and a pure one (it
// reads the options map and returns OpenOpts; it touches no file) —
// wrapped exactly as the handler wraps it.
func openModeMirror(result *Type) ReturnsFunc {
	// Gated at the OPTIONS argument, not the target: open's signature is
	// [Pathon, Map], and doOpenWord rejects an unknown mode from the map
	// alone, before it touches the path. Gating position 0 declined a
	// computed path with a literal {mode:'bogus'} for no reason (PR #348
	// review). The gate still declines any DYNAMIC operand, so an
	// optimistically-matched target cannot pull this model onto a call
	// the runtime dispatches elsewhere.
	return MirrorReturns("open", DeepConcreteOptionsAt(1),
		func(args []Value, r *Registry) error {
			// DeepConcreteOptionsAt(1) has already required an operand at
			// index 1, so the short-args case cannot reach the validator.
			if _, err := openOptsFromMap(args[1]); err != nil {
				return r.BoruError("open_error", fmt.Sprintf("open: %v", err), "open")
			}
			return nil
		},
		ReturnsStatic(result))
}

func exitHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateExitCode(args[0], r); err != nil {
		return nil, err
	}
	code, _ := args[0].AsConcreteInteger()
	return nil, core.NewExitError(code, r.Source, args[0].Pos())
}
