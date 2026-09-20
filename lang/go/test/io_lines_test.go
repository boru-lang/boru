package test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_lines_test.go — the HOST-DEPENDENT surface of IO.read-line and IO.is-tty.
// The hermetic arms (File handles over the in-memory filesystem, and every
// stream answering false with no probe installed) are spec rows in
// lang/spec/module-io.tsv §28; what needs a host lives here: stdin, the
// interaction between read-line and the whole-stream read, and the "yes, a
// terminal" arm that no redirected test suite can reach without the fake.

// runStdin runs src with stdin bound to in, and returns the residual stack.
func runStdin(t *testing.T, in, src string) []any {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.NativeRegistry().Input = strings.NewReader(in)
	res, err := runReference(t, a, `import "boru:io"  `+src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return res
}

func runStdin1(t *testing.T, in, src string) any {
	t.Helper()
	res := runStdin(t, in, src)
	if len(res) != 1 {
		t.Fatalf("%s: returned %d values, want 1: %v", src, len(res), res)
	}
	return res[0]
}

// The basic stdin line read.
func TestReadLineStdin(t *testing.T) {
	if got := runStdin1(t, "alpha\nbeta\n", `IO.read-line (IO.stdin)`); got != "alpha" {
		t.Errorf("first line = %#v, want alpha", got)
	}
}

// Successive reads advance through the stream.
func TestReadLineStdinAdvances(t *testing.T) {
	for i, want := range []string{"one", "two", "three"} {
		if got := runStdin1(t, "one\ntwo\nthree\n",
			`def a (IO.read-line (IO.stdin))  def b (IO.read-line (IO.stdin))  def c (IO.read-line (IO.stdin))  `+
				[]string{"a", "b", "c"}[i]); got != want {
			t.Errorf("line %d = %#v, want %s", i+1, got, want)
		}
	}
}

// Every terminator shape, and the two EOF shapes. LF and CRLF both strip;
// a final unterminated line is still a line; EOF at a boundary is none.
func TestReadLineStdinTerminators(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		want  any
	}{
		{"LF", "x\n", "x"},
		{"CRLF", "x\r\n", "x"},
		{"bare CR is data, not a terminator", "a\rb\n", "a\rb"},
		{"EOF mid-line: unterminated tail is a line", "tail", "tail"},
		{"EOF at a boundary: no line left", "", "None"},
		{"a blank line is the empty string", "\n", ""},
		{"interior CR survives a CRLF strip", "a\rb\r\n", "a\rb"},
	}
	for _, tc := range cases {
		got := runStdin1(t, tc.stdin, `IO.read-line (IO.stdin)`)
		if got != tc.want {
			t.Errorf("%s: read-line %q = %#v, want %#v", tc.name, tc.stdin, got, tc.want)
		}
	}
}

// THE interaction that makes read-line more than an addition: a line read
// must consume ahead (a reader cannot stop at a byte without consuming up to
// it), so read-line and the whole-stream `IO.read (IO.stdin)` MUST share one
// reader. If they did not, whichever ran first would silently eat bytes the
// other never saw.
func TestReadLineAndReadShareOneReader(t *testing.T) {
	got := runStdin1(t, "one\ntwo\nthree\n",
		`IO.read-line (IO.stdin) drop  IO.read (IO.stdin)`)
	if got != "two\nthree\n" {
		t.Errorf("read after read-line = %#v, want the REST of the stream (\"two\\nthree\\n\")", got)
	}
}

// And with no line read first, the whole-stream read is unchanged — the
// shared buffer is empty, so this is byte-for-byte the old behaviour.
func TestReadStdinUnchangedWithoutReadLine(t *testing.T) {
	got := runStdin1(t, "one\ntwo\n", `IO.read (IO.stdin)`)
	if got != "one\ntwo\n" {
		t.Errorf("read (IO.stdin) = %#v, want the whole stream", got)
	}
}

// A reader that fails part-way propagates its error rather than reporting
// EOF — a truncated stream is not an empty one.
type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n > 0 {
		f.n--
		p[0] = 'x'
		return 1, nil
	}
	return 0, errors.New("reader exploded")
}

func TestReadLineReaderError(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.NativeRegistry().Input = &failingReader{n: 3}
	_, runErr := a.Run(`import "boru:io"  IO.read-line (IO.stdin)`)
	if runErr == nil {
		t.Fatal("a failing reader was reported as EOF")
	}
	if !strings.Contains(runErr.Error(), "reader exploded") {
		t.Errorf("error = %v, want the reader's own failure", runErr)
	}
}

// A swapped r.Input must not be served from the old reader's buffer. Live
// code does swap it (native_help.go, around the example runner), so the
// holder records which reader it wrapped and rebuilds on a mismatch.
func TestReadLineRebuildsOnInputSwap(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	reg.Input = strings.NewReader("first\nbuffered-but-never-read\n")
	if _, err := runReference(t, a, `import "boru:io"  IO.read-line (IO.stdin)`); err != nil {
		t.Fatal(err)
	}
	reg.Input = strings.NewReader("second\n")
	res, err := runReference(t, a, `import "boru:io"  IO.read-line (IO.stdin)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := res[len(res)-1]; got != "second" {
		t.Errorf("after an Input swap, read-line = %#v, want second (a stale buffer would have served the first reader's leftovers)", got)
	}
}

// --- IO.is-tty ---

// The TRUE arm, reachable only through the fake: a test suite's streams are
// redirected, so the OS probe would answer false no matter what.
func TestTTYTrueArm(t *testing.T) {
	a, err := lang.New(lang.Options{Streams: capabilities.FixedStreamProbe{
		Terminal: map[string]bool{"stdout": true, "stderr": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Run renders the residual stack, so a Boolean arrives as "true"/"false".
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`IO.is-tty (IO.stdout)`, "true"},
		{`IO.is-tty (IO.stderr)`, "true"},
		{`IO.is-tty (IO.stdin)`, "false"}, // absent from the map
	} {
		res, err := runReference(t, a, `import "boru:io"  `+tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := res[len(res)-1]; got != tc.want {
			t.Errorf("%s = %#v, want %v", tc.src, got, tc.want)
		}
	}
}

// Per-stream is the point: stdout piped while stderr is still a terminal is
// the normal shape of a program in a pipeline, and one answer for "the
// terminal" would be wrong for one of them.
func TestTTYIsPerStream(t *testing.T) {
	a, err := lang.New(lang.Options{Streams: capabilities.FixedStreamProbe{
		Terminal: map[string]bool{"stderr": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := runReference(t, a, `import "boru:io"  [(IO.is-tty (IO.stdout)) (IO.is-tty (IO.stderr))]`)
	if err != nil {
		t.Fatal(err)
	}
	if got := res[len(res)-1]; got != "[false, true]" && got != "[false true]" {
		t.Errorf("per-stream answers = %#v, want stdout false and stderr true", got)
	}
}

// The OS probe tells the truth about a redirection for free: a non-*os.File
// endpoint is not a terminal, so a host that pointed a stream at a buffer
// gets false without having to say so.
func TestOSStreamProbeRedirectedIsFalse(t *testing.T) {
	a, err := lang.New(lang.Options{Streams: capabilities.OSStreamProbe{}})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	a.SetOutput(&buf)
	res, err := runReference(t, a, `import "boru:io"  IO.is-tty (IO.stdout)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := res[len(res)-1]; got != "false" {
		t.Errorf("a redirected stdout = %#v, want false", got)
	}
}

// A profile that uninstalls the terminal scope clears the slot, so IO.is-tty
// answers false rather than raising. Raising would abort the RFC's own
// colour idiom (`IO.is-tty (IO.stdout)` beside `IO.env "NO_COLOR"`) under every
// profile except trusted — five of the seven shipped ones uninstall it.
func TestTTYUnderUninstalledTerminalScope(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{
		Policy:  pol,
		Streams: capabilities.FixedStreamProbe{Terminal: map[string]bool{"stdout": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe := native.HostStreamProbe(a.NativeRegistry()); probe != nil {
		t.Errorf("sandbox uninstalls the terminal scope: probe = %T, want nil", probe)
	}
	res, err := runReference(t, a, `import "boru:io"  IO.is-tty (IO.stdout)`)
	if err != nil {
		t.Fatalf("IO.is-tty raised under a sandbox instead of answering: %v", err)
	}
	if got := res[len(res)-1]; got != "false" {
		t.Errorf("sandboxed IO.is-tty = %#v, want false", got)
	}
}

// trusted allows the terminal scope, so an installed probe survives.
func TestTTYUnderTrustedProfile(t *testing.T) {
	pol, err := policy.Load("trusted")
	if err != nil {
		t.Fatal(err)
	}
	a, err := lang.New(lang.Options{
		Policy:  pol,
		Streams: capabilities.FixedStreamProbe{Terminal: map[string]bool{"stdout": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := runReference(t, a, `import "boru:io"  IO.is-tty (IO.stdout)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := res[len(res)-1]; got != "true" {
		t.Errorf("trusted IO.is-tty = %#v, want true", got)
	}
}

// A module body reading stdin must share the IMPORTER's reader. This is the
// bug the eager install exists to prevent: RunModuleBody snapshots the
// ModuleInheritedCaps slots at import time, so a lazily-created holder would
// not be there to copy, the module would open a SECOND buffer over the same
// reader, and the two would consume each other's bytes — the importer seeing
// "" where it expected its next line.
func TestReadLineSharedAcrossModuleBody(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.NativeRegistry().Input = strings.NewReader("first\nsecond\nthird\n")
	res, err := a.Run(`import "boru:io"
import module [ import "boru:io"  def take fn [[] [Any] [IO.read-line (IO.stdin)]]  export "M" {take: take/v} ] end
M.take drop
IO.read-line (IO.stdin)`)
	if err != nil {
		t.Fatalf("module body reading stdin: %v", err)
	}
	// The module took "first"; the importer must then see "second", not "" and
	// not "first" again.
	if got := res[len(res)-1]; got != "second" {
		t.Errorf("after a module body read a line, the importer got %#v, want second", got)
	}
}

// IO.seek repositions the handle, so the line buffer's read-ahead is bytes
// from somewhere else and must be dropped. Without this, a read-line after
// `IO.seek f 0` served the NEXT buffered line instead of re-reading from the
// start — the handle's cursor and the buffer disagreed silently.
func TestReadLineSeekDropsBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seek.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Run(`import "boru:io"
def f (IO.open (make Pathon "` + path + `"))
def one (IO.read-line f)
IO.seek f 0 drop
def two (IO.read-line f)
[one two]`)
	if err != nil {
		t.Fatal(err)
	}
	got := res[len(res)-1]
	if got != `["alpha", "alpha"]` && got != "[alpha, alpha]" && got != "['alpha' 'alpha']" {
		t.Errorf("after a seek back to 0, lines = %#v, want both alpha", got)
	}
}

// recordingProbe answers false for everything and records the Go type of the
// endpoint it was handed per stream. What a StreamProbe RECEIVES is the whole
// contract between the runtime and the operating system here: OSStreamProbe
// stats an *os.File, so any wrapper the runtime slips in front of a stream and
// forgets to make peelable silently turns "is this a terminal" into "no".
type recordingProbe struct{ seen map[string]string }

func (p recordingProbe) IsTerminal(stream string, endpoint any) bool {
	p.seen[stream] = fmt.Sprintf("%T", endpoint)
	return false
}

// A MODULE BODY gets the terminal probe too, alongside the environment and the
// argument vector (modules/io.go, ModuleInheritedCaps). boru:cli is the library
// that needs it: its help rendering colours only when stdout is a terminal, and
// without the inheritance that decision read "never a terminal" one import deep
// while the importing program read the truth.
func TestStreamProbeReachesModuleBodies(t *testing.T) {
	a, err := lang.New(lang.Options{Streams: capabilities.FixedStreamProbe{
		Terminal: map[string]bool{"stdout": true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	a.SetOutput(&out)
	if _, err := a.Run(`import module [
  import "boru:io"
  def m-tty fn [[] [Boolean] [ IO.is-tty (IO.stdout) ]]
  export "M" {tty: m-tty/v}
] end
print (M.tty)`); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "true" {
		t.Errorf("module body IO.is-tty printed %q, want true — the host installed a probe saying stdout IS a terminal", out.String())
	}
}

// COMPILED mode must hand the probe the same endpoint the interpreter does.
// It did not: compiled entry points used to arm the C1 effect fence, which
// wrapped Output/ErrOutput so a print counted against the interpreter re-run,
// and the wrapper was not peelable. So
// `IO.is-tty (IO.stdout)` answered FALSE on a real terminal in the DEFAULT
// execution mode while the interpreter answered true — the colour decision
// inverted by an instrument that is supposed to be invisible.
func TestStreamProbeEndpointIsModeIndependent(t *testing.T) {
	const src = `import "boru:io"  IO.is-tty (IO.stdout)  IO.is-tty (IO.stderr)  IO.is-tty (IO.stdin)`
	seen := map[string]map[string]string{}
	for _, mode := range []string{"interp", "compiled"} {
		p := recordingProbe{seen: map[string]string{}}
		a, err := lang.New(lang.Options{Streams: p})
		if err != nil {
			t.Fatal(err)
		}
		if mode == "interp" {
			if _, err := a.RunInterp(src); err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
		} else {
			_, ran, err := a.RunCompiled(src)
			if err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
			if !ran {
				t.Fatalf("%s: the program did not run compiled, so this test proves nothing", mode)
			}
		}
		seen[mode] = p.seen
	}
	for _, stream := range []string{"stdout", "stderr", "stdin"} {
		if seen["interp"][stream] != seen["compiled"][stream] {
			t.Errorf("%s endpoint: interp saw %s, compiled saw %s — the two modes must agree",
				stream, seen["interp"][stream], seen["compiled"][stream])
		}
		if seen["compiled"][stream] != "*os.File" {
			t.Errorf("%s endpoint in compiled mode = %s, want *os.File (the OS probe stats it)",
				stream, seen["compiled"][stream])
		}
	}
}
