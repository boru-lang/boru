package langspec

import (
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestIOSurfaceCompilesNoRefusal is the focused companion to the corpus-wide
// TestRefusalsAreFailures gate: it pins that the boru:io Pathon-only redesign —
// the bare `list`/`remove` word extensions (a Pathon overload grafted onto the
// core words at import) and every namespaced IO word — never regresses into a
// WHOLE-PROGRAM compile refusal (a nil Program with no check error), which
// would silently strand the row on the interpreter (design/COMPILABLE-SUBSET.md).
//
// The corpus census only sees the exact module-io.tsv rows; this test exercises
// io shapes that are NOT verbatim spec rows — the {recursive}/{force}/{detail}
// option forms, the bare extension after the in-memory-FS context toggle (the
// dynamic-value-then-extension shape whose static dispatch does not resolve and
// so leans on the runtime rematch), and each IO word in isolation. If a future
// dispatch change made any of these refuse, the runtime rematch that currently
// keeps them native would have silently broken.
func TestIOSurfaceCompilesNoRefusal(t *testing.T) {
	t.Parallel()
	// Each program is valid and must lower to a non-nil Program. The mem-FS
	// toggle rows exercise the bare extension sitting after a dynamic value on
	// the stack — the case the checker cannot statically resolve, which must
	// still COMPILE (to a runtime rematch), not refuse.
	compileOK := []string{
		// bare `list` extension (core word gains a Pathon overload on import)
		`import "boru:io"  list (make Pathon "d")`,
		`import "boru:io"  list (make Pathon "d") {detail:true}`,
		`import "boru:io"  list (make Pathon "d") {recursive:true}`,
		`import "boru:io"  context dot __sys dot fs set mem true  list (make Pathon "mem://X")`,
		// the core `list` meaning still compiles alongside the overload
		`import "boru:io"  list [1 2 3]`,
		// bare `remove` extension
		`import "boru:io"  remove (make Pathon "f")`,
		`import "boru:io"  remove (make Pathon "f") {recursive:true}`,
		`import "boru:io"  remove (make Pathon "f") {force:true}`,
		`import "boru:io"  context dot __sys dot fs set mem true  remove (make Pathon "mem://g") {force:true}`,
		// P1 option forms: glob filter, overwrite refusal, text encodings
		`import "boru:io"  list (make Pathon "d") {match:"*.txt"}`,
		`import "boru:io"  list (make Pathon "d") {recursive:true match:"**.txt"}`,
		`import "boru:io"  IO.copy (make Pathon "a") (make Pathon "b") {overwrite:false}`,
		`import "boru:io"  IO.move (make Pathon "a") (make Pathon "b") {overwrite:false}`,
		`import "boru:io"  IO.read (make Pathon "d") {enc:'utf16le'}`,
		`import "boru:io"  IO.write (make Pathon "d") "x" {enc:'latin1'}`,
		// P2 option forms: ownership + extended attributes
		`import "boru:io"  IO.stat (make Pathon "d") {xattr:true}`,
		`import "boru:io"  IO.touch (make Pathon "d") {owner:1 group:1}`,
		`import "boru:io"  IO.touch (make Pathon "d") {xattr:{note:'x'}}`,
		// P3: temp files/dirs, disk info, atomic write
		`import "boru:io"  IO.temp {}`,
		`import "boru:io"  IO.temp {dir:true prefix:"log-" suffix:".txt"}`,
		`import "boru:io"  IO.temp {in:(make Pathon "d")}`,
		`import "boru:io"  IO.space (make Pathon "d")`,
		`import "boru:io"  IO.write (make Pathon "d") "x" {atomic:true}`,
		// P4: stateful File handles + {exclusive}
		`import "boru:io"  IO.open (make Pathon "d")`,
		`import "boru:io"  IO.open (make Pathon "d") {mode:'write' create:true exclusive:true truncate:true perm:0o600}`,
		`import "boru:io"  IO.seek (IO.open (make Pathon "d")) 4 {from:'start'}`,
		`import "boru:io"  IO.flush (IO.open (make Pathon "d") {mode:'write'})`,
		`import "boru:io"  IO.close (IO.open (make Pathon "d"))`,
		`import "boru:io"  IO.read (IO.open (make Pathon "d")) {length:3 offset:0 enc:'bytes'}`,
		`import "boru:io"  IO.write (IO.open (make Pathon "d") {mode:'write'}) "x" {offset:0}`,
		`import "boru:io"  IO.write (make Pathon "d") "x" {exclusive:true}`,
		// P5: advisory locks + memory-mapped files
		`import "boru:io"  IO.lock (make Pathon "d")`,
		`import "boru:io"  IO.lock (make Pathon "d") {shared:true block:false}`,
		`import "boru:io"  IO.unlock (IO.lock (make Pathon "d"))`,
		`import "boru:io"  IO.mmap (make Pathon "d")`,
		`import "boru:io"  IO.mmap (make Pathon "d") {offset:0 length:8 writable:true}`,
		`import "boru:io"  IO.read (IO.mmap (make Pathon "d")) {offset:0 length:3 enc:'bytes'}`,
		`import "boru:io"  IO.write (IO.mmap (make Pathon "d") {writable:true}) (convert Bytes "x") {offset:0}`,
		`import "boru:io"  IO.flush (IO.mmap (make Pathon "d") {writable:true})`,
		`import "boru:io"  IO.close (IO.mmap (make Pathon "d"))`,
		// P6: watch {recursive match} + coalesced overflow event
		`import "boru:io"  IO.watch (make Pathon "d") [drop]`,
		`import "boru:io"  IO.watch (make Pathon "d") [drop] {recursive:true match:"*.txt"}`,
		// P7: zip-archive-as-filesystem (the Pathon form of mount)
		`import "boru:io"  IO.mount (make Pathon "a.zip")`,
		`import "boru:io"  IO.mount (make Pathon "a.zip") {zip:true writable:true}`,
		// namespaced IO words, all Pathon-only
		`import "boru:io"  IO.read (make Pathon "d")`,
		`import "boru:io"  IO.read (make Pathon "d") {enc:'bytes'}`,
		`import "boru:io"  IO.write (make Pathon "d") "x"`,
		`import "boru:io"  IO.write (make Pathon "d") (convert Bytes "x")`,
		`import "boru:io"  IO.stat (make Pathon "d")`,
		`import "boru:io"  IO.move (make Pathon "a") (make Pathon "b")`,
		`import "boru:io"  IO.copy (make Pathon "a") (make Pathon "b") {recursive:true}`,
		`import "boru:io"  IO.link (make Pathon "a") (make Pathon "b") {hard:true}`,
		`import "boru:io"  IO.touch (make Pathon "a") {size:2}`,
		`import "boru:io"  IO.folder (make Pathon "a")`,
		// stream handles route through the same read/write signatures
		`import "boru:io"  IO.write IO.stdout "hi"`,
	}
	for _, src := range compileOK {
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Errorf("%q: unexpected check error, want a clean compile: %v", src, cerr)
			continue
		}
		if prog == nil {
			t.Errorf("%q: bytecode compiler REFUSED (reason %q) — the io redesign must lower this to a native Program, not strand it on the interpreter", src, reason)
		}
	}

	// Negative pairing (test discipline): a REJECTED io call — a String file
	// target (Pathon-only refuses it) or the bare Pathon overload used without
	// `import "boru:io"` — must still never become a SILENT whole-program
	// refusal. Rejection is allowed to land either statically (a check
	// diagnostic) or as a native compile-to-runtime-rematch that raises when
	// run; both keep the program off the refusal list. The one forbidden state
	// is a nil Program with no diagnostic and an empty reason — the exact
	// silent refusal design/COMPILABLE-SUBSET.md rules out.
	notRefused := []string{
		`import "boru:io"  IO.read "data.csv"`,      // String path, not a Pathon
		`import "boru:io"  IO.write "out.txt" "hi"`, // String path, not a Pathon
		`list (make Pathon "d")`,                    // bare Pathon overload absent without import
		`remove (make Pathon "f")`,                  // same for remove
	}
	for _, src := range notRefused {
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		silentRefusal := prog == nil && cerr == nil && reason != "check diagnostics"
		if silentRefusal {
			t.Errorf("%q: bytecode compiler SILENTLY REFUSED (reason %q) — a rejected io call must check-error or compile to a runtime rematch, never strand the whole program on the interpreter", src, reason)
		}
	}
}
