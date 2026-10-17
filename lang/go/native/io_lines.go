package native

import (
	"bufio"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"

	core "github.com/boru-lang/boru/core/go"
)

// io_lines.go — incremental line reads (IO.read-line) and the buffered
// stdin reader they share with the whole-stream read.
//
// # Why stdin needs a shared reader
//
// `IO.read (IO.stdin)` drains the reader with io.ReadAll (fileio.go). A
// BUFFERED line read pulls 4096 bytes at a time, so a private buffer would
// leave the whole-stream read with nothing: the two words would fight, and
// whichever ran first would eat bytes the other never saw, silently. So
// there is exactly ONE reader over r.Input, used by both words, and
// `IO.read-line` followed by `IO.read` yields the rest of the stream.
//
// To be precise about what forces this, since the comment previously
// overstated it: buffering is a CHOICE. Reading one byte at a time never
// consumes past the newline, and would leave the existing whole-stream read
// untouched — but it costs a syscall per byte, which a `wc`-style filter over
// a large stream cannot afford. The shared reader buys the buffering back
// without the fight.
//
// # Where it lives, and why not somewhere else
//
// The holder sits in a capability slot, lazily created and cached — the
// idiom EffectiveFileOps already uses for the in-memory-FS toggle
// (capabilities.go). Two alternatives were rejected:
//
//   - A field on core.Registry. Input is already an eng field, so this looks
//     natural, but module sub-registries copy Input by assignment in five
//     places (modules/repl.go, modules/test.go, modules/sift.go,
//     modules/vault_tui.go, native_module_module.go). Each would get its own
//     holder over the SAME reader — the fight again, inside module bodies —
//     and a sixth site added later would reintroduce it silently.
//   - A per-handle buffer for streams, mirroring the File case. There is
//     nowhere to put it: a stream handle is a value-type Atom with no
//     payload, and IO.stdin mints a fresh Value on every call.
//
// The slot is named in ModuleInheritedCaps (modules/io.go) so a module body
// shares the importer's reader rather than opening a second one. That only
// works because the holder is installed EAGERLY at registry construction
// (setup.go): RunModuleBody snapshots the inherited slots at IMPORT time, so
// a lazily-created holder would not be there to copy and the module would
// open its own buffer over the importer's reader.
//
// # Correctness details that are not obvious
//
//   - The holder records WHICH reader it wrapped. r.Input is a mutable field
//     that live code swaps and restores (native_help.go does, around the
//     example runner), and a buffer holding bytes from the old reader must
//     not serve them after the swap. A mismatch rebuilds.
//   - The mutex is not decoration: `make test-race` runs lang/go under
//     -race, ForkConcurrent shares the registry pointer, and a bufio.Reader
//     is not safe for concurrent use.
//   - A sub-engine (Vm.run / Vm.run-sandbox) gets a fresh capability
//     registry and the default os.Stdin, so it never shares this holder.
//     That is isolation by construction, not by arrangement.
//   - Reads do NOT call r.NoteEffect(), and that is a deliberate C2
//     non-change rather than an oversight. Consuming stdin is genuinely
//     non-idempotent, so a silent interpreter re-run after a compiled-mode
//     bail would re-read an already-drained reader and see none where the
//     first attempt saw a line. But no read in this package notes an effect
//     (fileio.go's read path has none), so arming it here alone would leave
//     the same hole for `IO.read (IO.stdin)` while changing the effect
//     ledger's meaning for one word. Fixing it belongs to a change that
//     covers the whole read family.

// stdinLines is the single buffered reader over a registry's stdin, plus the
// identity of the reader it wraps so a swapped r.Input is detected.
type stdinLines struct {
	mu  sync.Mutex
	src io.Reader     // the r.Input this buffer was built over
	buf *bufio.Reader // the one reader both read-line and read use
}

// installStdinLines puts the shared-reader holder in its slot. Called once
// per registry from DefaultRegistryWithPolicy, which explains why eagerly.
func installStdinLines(r *Registry) {
	if r == nil {
		return
	}
	_ = r.Capabilities.Set(CapStdinLines, &stdinLines{})
}

// stdinReader returns the shared buffered reader over r.Input, building the
// buffer on first use and rebuilding it if r.Input has since been swapped.
//
// The holder itself is never created here — installStdinLines did that at
// construction, so this function only ever READS the capability map. That is
// deliberate: the map is unsynchronised and shared across concurrency forks,
// so a write from here would be a race. A registry assembled without the
// standard setup has no holder and falls back to an unbuffered read, which
// keeps the whole-stream read correct (it consumes everything anyway) at the
// cost of line reads not sharing — the only shape that cannot happen through
// any production path.
//
// A rebuild after an Input swap DISCARDS whatever the old reader had
// buffered. That is the right call — the host replaced the source, so bytes
// from the previous one are not the program's input any more — but it does
// mean a host must not swap Input mid-line and expect continuity.
func stdinReader(r *Registry) io.Reader {
	holder, _, _ := core.Cap[*stdinLines](r, CapStdinLines)
	if holder == nil {
		return r.Input
	}
	return holder.reader(r.Input)
}

// stdinReadAll drains the rest of standard input through the shared holder's
// LOCK, so a whole-stream read cannot race a concurrent line read over the
// same buffer. Falls back to an unlocked drain only when no holder is
// installed, which no production path produces.
func stdinReadAll(r *Registry) ([]byte, error) {
	if holder, _, _ := core.Cap[*stdinLines](r, CapStdinLines); holder != nil {
		return holder.readAll(r.Input)
	}
	return io.ReadAll(stdinReader(r))
}

// lineReaderForStdin is stdinReader narrowed to what a line read needs: a
// *bufio.Reader. A registry with no holder gets a throwaway buffer, so a line
// read still works (it just cannot share with a later whole-stream read).
func lineReaderForStdin(r *Registry) *bufio.Reader {
	if br, ok := stdinReader(r).(*bufio.Reader); ok {
		return br
	}
	return bufio.NewReader(r.Input)
}

// lineSource is a thing one line can be read FROM, rather than a buffer a
// caller reads. The indirection is what lets the lock cover the read: a
// caller handed a *bufio.Reader can only ever read it unlocked.
type lineSource interface {
	ReadLine() (string, bool, error)
}

// stdinLineSource reads through the registry's shared holder when there is
// one, and falls back to an unshared buffer when there is not (a registry
// assembled outside the standard setup) — the same fallback lineReaderForStdin
// has always made, now with the read inside the holder's lock.
type stdinLineSource struct{ r *Registry }

func (s stdinLineSource) ReadLine() (string, bool, error) {
	if holder, _, _ := core.Cap[*stdinLines](s.r, CapStdinLines); holder != nil {
		return holder.readLine(s.r.Input)
	}
	return readLineFrom(lineReaderForStdin(s.r))
}

// reader returns the buffer over src, rebuilding when src is not the reader
// this holder was built over. The mutex is load-bearing: lang/go runs under
// -race and a bufio.Reader is not safe for concurrent use.
func (h *stdinLines) reader(src io.Reader) *bufio.Reader {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bufLocked(src)
}

// bufLocked is reader's body without the locking; the caller holds h.mu.
func (h *stdinLines) bufLocked(src io.Reader) *bufio.Reader {
	if h.buf == nil || !sameReader(h.src, src) {
		h.src = src
		h.buf = bufio.NewReader(src)
	}
	return h.buf
}

// readLine consumes one line WHILE HOLDING the mutex, and readAll drains the
// rest of the stream the same way.
//
// The mutex used to guard only the buffer's CREATION: `reader` returned the
// *bufio.Reader and the caller read from it after unlocking. Since
// eng.ForkConcurrent leaves Capabilities and Input alone, every await/spawn
// fork shares this one holder, so those reads were unsynchronised against a
// type the standard library documents as unsafe for concurrent use — the
// failure being duplicated, interleaved or lost input rather than a crash,
// which is the kind that reaches a user as corrupt data.
func (h *stdinLines) readLine(src io.Reader) (string, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return readLineFrom(h.bufLocked(src))
}

func (h *stdinLines) readAll(src io.Reader) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return io.ReadAll(h.bufLocked(src))
}

// sameReader reports whether two readers are the same source, WITHOUT ever
// evaluating `==` on an uncomparable dynamic type.
//
// This is not defensive padding. `Registry.Input` is a public io.Reader
// field any host may assign, and `==` on an interface whose dynamic type is
// a func, or a struct holding a slice or map, PANICS at runtime
// ("comparing uncomparable type") — which ADR-005 admits nowhere, and which
// would surface as an internal_error on the second stdin read of a program
// whose host happened to pass such a reader.
//
// Pointer-like kinds compare by address, which covers every reader in the
// tree (*strings.Reader, *os.File, *bytes.Buffer). A comparable non-pointer
// type compares by value. Anything left is uncomparable, and there the
// answer is "same" — keeping the existing buffer rather than rebuilding. An
// undetected swap serves bytes from the previous reader, which is a far
// smaller wrong than a crash, and it takes a host that both installs an
// uncomparable reader AND swaps it mid-stream to reach.
func sameReader(a, b io.Reader) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	switch ta.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func, reflect.Map,
		reflect.Slice, reflect.UnsafePointer:
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
	if ta.Comparable() {
		return a == b
	}
	return true
}

// readLineFrom reads one line from br, returning it without its terminator
// and reporting whether a line was found at all.
//
// The three EOF cases are distinct and all real:
//   - EOF with bytes buffered: a final line with no trailing newline. It IS
//     a line; a filter that dropped it would lose data every time its input
//     came from an editor that does not add a final newline.
//   - EOF with nothing buffered: no more lines. none.
//   - A read error that is not EOF: the caller's problem, propagated.
//
// CRLF is stripped as well as LF so a file written on Windows reads as
// lines rather than lines ending in a stray \r. Only a \r immediately
// before the \n is removed — an interior \r is data.
func readLineFrom(br *bufio.Reader) (string, bool, error) {
	line, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	if line == "" {
		return "", false, nil // EOF at a line boundary: no line left
	}
	// The \r is stripped ONLY as half of a CRLF terminator. Nesting the two
	// TrimSuffix calls unconditionally also ate a bare trailing \r on a final
	// UNTERMINATED line, which is data, not a terminator — the contract three
	// lines above says so, and CLI.md and HOWTO.md repeat it. A file whose
	// last line is "abc\r" with no newline round-tripped as "abc".
	if trimmed, hadLF := strings.CutSuffix(line, "\n"); hadLF {
		return strings.TrimSuffix(trimmed, "\r"), true, nil
	}
	return line, true, nil
}

// readLineHandler implements IO.read-line over a stream handle or a File
// handle, returning the next line without its terminator, or none at EOF.
//
// none-at-EOF rather than "" is what makes the canonical filter loop
// terminate: "" is a legitimate line (a blank one), so a caller must be able
// to tell "an empty line" from "no more lines".
func readLineHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	src, err := lineReaderFor(args[0], r)
	if err != nil {
		return nil, err
	}
	line, ok, rerr := src.ReadLine()
	if rerr != nil {
		return nil, r.BoruErrorAt("read_error",
			"read-line: "+rerr.Error(), "read-line", args[0].Pos())
	}
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{NewString(line)}, nil
}

// lineReaderFor resolves the argument to the buffered reader whose lines it
// names: the shared stdin reader for the stdin stream handle, or the handle's
// own buffer for a File.
//
// An OUTPUT stream is declined rather than answered with none. `IO.read-line
// (IO.stdout)` is a mistake in the program, not a stream that happens to be
// finished, and reporting it as EOF would hide the bug behind a loop that
// exits immediately — the same reasoning as doRead's output-stream compile failure.
func lineReaderFor(v Value, r *Registry) (lineSource, error) {
	if fh, ok := asFileHandle(v); ok {
		return fh, nil
	}
	name, aerr := v.AsConcreteAtom()
	if aerr != nil {
		return nil, r.BoruErrorAt("read_error",
			"read-line: not a stream or File handle", "read-line", v.Pos())
	}
	switch streamSentinels[name] {
	case pathStdin:
		return stdinLineSource{r}, nil
	case pathStdout, pathStderr:
		return nil, r.BoruErrorAt("read_error",
			"read-line: cannot read from an output stream", "read-line", v.Pos())
	}
	return nil, r.BoruErrorAt("read_error",
		"read-line: not a stream or File handle", "read-line", v.Pos())
}

// ttyHandler implements IO.is-tty: is this stream a terminal?
//
// The word is `is-tty`, not the `tty?` of design/CLI-PROGRAMS.0.md §5. `?` is
// rejected in a word name at registration (eng/go/word_name.go's
// ValidateWordName), and its doc states both the rule and the replacement:
// "the Lisp/Scheme/Ruby `?` predicate suffix is NOT allowed; predicates use
// the `is-` prefix convention instead". It is also unlexable inside a word —
// `?` is a fixed token — and in a dot chain that mis-parse is SILENT, so the
// RFC spelling would have produced a word that quietly did nothing.
//
// No probe installed answers FALSE, and so does every profile that
// uninstalls the terminal scope (see SetHostStreamProbe). A program asking
// this question is choosing between two valid behaviours, so "no" is a usable
// answer where an error would not be.
func ttyHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name, err := args[0].AsConcreteAtom()
	if err != nil {
		return nil, r.BoruErrorAt("is_tty_error", "is-tty: not a stream handle", "is-tty", args[0].Pos())
	}
	if _, ok := streamSentinels[name]; !ok {
		return nil, r.BoruErrorAt("is_tty_error", "is-tty: not a stream handle", "is-tty", args[0].Pos())
	}
	probe := HostStreamProbe(r)
	if probe == nil {
		return []Value{NewBoolean(false)}, nil
	}
	return []Value{NewBoolean(probe.IsTerminal(name, streamEndpoint(r, name)))}, nil
}

// streamEndpoint returns the io.Reader / io.Writer the registry currently
// holds for a named stream, so an OS-backed probe inspects what the program
// is ACTUALLY talking to rather than the process's own descriptors. A host
// that redirected a stream therefore gets the honest answer for free.
//
// Engine-side wrappers are unwrapped first. Every concurrency fork rewraps
// the output writers in an core.SyncWriter (eng/go/process.go), which is a
// serialization detail the program never chose — without unwrapping, IO.is-tty
// would answer FALSE for a genuine terminal inside any forked context, and
// answer it silently.
func streamEndpoint(r *Registry, name string) any {
	switch name {
	case "stdin":
		return r.Input
	case "stderr":
		return unwrapWriter(r.ErrOutput)
	default:
		return unwrapWriter(r.Output)
	}
}

// unwrapWriter peels engine-side writer wrappers down to the destination.
func unwrapWriter(w io.Writer) io.Writer {
	for {
		u, ok := w.(interface{ Unwrap() io.Writer })
		if !ok {
			return w
		}
		w = u.Unwrap()
	}
}
