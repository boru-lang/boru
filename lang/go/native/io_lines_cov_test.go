package native

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_lines_cov_test.go — sameReader's arms, and the crash it exists to
// prevent.
//
// Registry.Input is a public io.Reader field any host may assign, and `==` on
// an interface whose dynamic type is uncomparable PANICS. The holder's
// staleness check compares the reader it wrapped against the current one on
// every stdin read, so a host that installed such a reader would have crashed
// on its SECOND read — reported as an internal_error where the old code
// returned a value.

// uncomparableReader's slice field makes the struct uncomparable, so `==` on
// an io.Reader holding one panics. Passed BY VALUE so the interface's dynamic
// type is the struct itself, not a pointer to it.
type uncomparableReader struct {
	data []byte // <- what makes it uncomparable
	pos  int
}

func (r uncomparableReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	return n, nil
}

// comparableReader is a value type with only comparable fields — the arm
// where a plain `==` is both safe and meaningful.
type comparableReader struct{ ch byte }

func (comparableReader) Read(p []byte) (int, error) { return 0, io.EOF }

// The crash, directly: two reads over an uncomparable reader must not panic.
func TestStdinReaderUncomparableInputDoesNotPanic(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Input = uncomparableReader{data: []byte("a\nb\n")}
	// The second call is the one that compares, and the one that panicked.
	_ = stdinReader(r)
	_ = stdinReader(r)
}

// Every arm of the comparison, so the panic-free path is not merely lucky.
func TestSameReaderArms(t *testing.T) {
	p1 := strings.NewReader("x")
	p2 := strings.NewReader("x")
	unc := uncomparableReader{data: []byte("x")}
	cmp1 := comparableReader{ch: 'a'}
	cmp2 := comparableReader{ch: 'b'}

	cases := []struct {
		name string
		a, b io.Reader
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", nil, p1, false},
		{"other nil", p1, nil, false},
		{"different dynamic types", p1, unc, false},
		{"same pointer", p1, p1, true},
		{"different pointers, same type", p1, p2, false},
		{"comparable values, equal", cmp1, cmp1, true},
		{"comparable values, different", cmp1, cmp2, false},
		// Uncomparable: reported same, so the buffer is KEPT rather than
		// rebuilt. Rebuilding every call would discard read-ahead bytes on
		// every read; crashing is not an option at all.
		{"uncomparable reports same", unc, uncomparableReader{data: []byte("y")}, true},
	}
	for _, tc := range cases {
		if got := sameReader(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: sameReader = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A registry with no holder in its slot still reads: stdinReader falls back
// to r.Input, and a line read to a throwaway buffer. Only reachable by
// deleting the slot, which no production path does — the holder is installed
// at construction (setup.go).
func TestStdinReaderWithoutHolder(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Capabilities.Delete(CapStdinLines); err != nil {
		t.Fatal(err)
	}
	r.Input = strings.NewReader("solo\n")
	if got := stdinReader(r); got != r.Input {
		t.Errorf("with no holder, stdinReader should hand back r.Input itself, got %T", got)
	}
	line, ok, rerr := readLineFrom(lineReaderForStdin(r))
	if rerr != nil || !ok || line != "solo" {
		t.Errorf("holder-less line read = (%q, %v, %v), want (solo, true, nil)", line, ok, rerr)
	}
}

// installStdinLines tolerates a nil registry, like every other installer.
func TestInstallStdinLinesNilRegistry(t *testing.T) {
	installStdinLines(nil)
}

// unwrapWriter peels engine-side wrappers so a terminal probe inspects the
// destination rather than a concurrency wrapper. Without this IO.is-tty answers
// false for a real terminal inside any forked context.
func TestUnwrapWriterPeelsSyncWriter(t *testing.T) {
	var sink strings.Builder
	wrapped := core.NewSyncWriter(core.NewSyncWriter(&sink))
	if got := unwrapWriter(wrapped); got != io.Writer(&sink) {
		t.Errorf("unwrapWriter did not reach the destination: got %T", got)
	}
	if got := unwrapWriter(&sink); got != io.Writer(&sink) {
		t.Errorf("an unwrapped writer should pass through, got %T", got)
	}
}

// readLineFrom propagates a non-EOF read error rather than reporting EOF: a
// truncated stream is not an empty one.
type explodingReader struct{}

func (explodingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestReadLineFromPropagatesError(t *testing.T) {
	_, ok, err := readLineFrom(bufio.NewReader(explodingReader{}))
	if err == nil {
		t.Fatal("a read error was reported as EOF")
	}
	if ok {
		t.Error("a failed read should not report a line")
	}
}

// --- OSStreamProbe against real files ---

// The production probe's own arms.
//
// This test used to hand the probe /dev/null to reach its TRUE arm, on the
// reasoning that /dev/null is a character device on every platform. It is —
// and that was the bug. The probe tested os.ModeCharDevice, which is true for
// /dev/null, /dev/zero and /dev/urandom alike, so `prog > /dev/null` answered
// "yes, a terminal" and took the colour branch: the single most common
// redirection there is, and exactly the case --color=auto exists to avoid.
// The test passed because it asserted the defect.
//
// The probe now asks the kernel (term.IsTerminal). /dev/null is the NEGATIVE
// case, and the true arm is reached with a real pty via /dev/ptmx — skipped
// where the platform has none rather than quietly dropped.
func TestOSStreamProbeAgainstRealFiles(t *testing.T) {
	probe := capabilities.OSStreamProbe{}

	dev, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("no %s on this platform: %v", os.DevNull, err)
	}
	defer func() { _ = dev.Close() }()
	if probe.IsTerminal("stdout", dev) {
		t.Errorf("%s is a character device but NOT a terminal — answering true "+
			"here is what made `prog > /dev/null` emit colour", os.DevNull)
	}

	// Exercise a real terminal on every supported desktop OS.
	terminal := openTestTerminal(t)
	if !probe.IsTerminal("stdout", terminal) {
		t.Error("an initialized terminal should read as a terminal")
	}

	reg, err := os.CreateTemp(t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()
	if probe.IsTerminal("stdout", reg) {
		t.Error("a regular file is not a terminal")
	}

	// Stat on a closed descriptor fails, and a probe that cannot tell must
	// answer false rather than guess.
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if probe.IsTerminal("stdout", closed) {
		t.Error("a probe that cannot Stat its endpoint must answer false")
	}
}

// --- SetHostStreamProbe's arms ---

func TestSetHostStreamProbeArms(t *testing.T) {
	// A nil registry is a no-op, like every other installer (ADR-005).
	SetHostStreamProbe(nil, capabilities.OSStreamProbe{})

	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostStreamProbe(r, capabilities.OSStreamProbe{})
	if HostStreamProbe(r) == nil {
		t.Fatal("expected an installed probe")
	}
	// nil clears the slot: a host revoking the capability must be able to.
	SetHostStreamProbe(r, nil)
	if HostStreamProbe(r) != nil {
		t.Error("a nil probe should clear the slot")
	}
}

// --- the handler arms boru cannot reach ---

// The signatures admit only StreamKind and File, so these guards are
// unreachable from boru source — but they are the difference between a clear
// error and a wrong answer if a future signature admits something looser.
func TestLineReaderForRejectsNonStreams(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A non-atom: fails AsConcreteAtom.
	if _, herr := lineReaderFor(NewInteger(42), r); herr == nil {
		t.Error("an Integer was accepted as a stream")
	}
	// An atom that is not one of the three stream names.
	if _, herr := lineReaderFor(NewAtom("banana"), r); herr == nil {
		t.Error("an unrelated atom was accepted as a stream")
	}
	// A DepScalar constraint in the slot must be refused, not read as "".
	if _, herr := lineReaderFor(NewDepScalar(DepLT, NewString("z")), r); herr == nil {
		t.Error("a String constraint was accepted as a stream")
	}
}

func TestTTYHandlerRejectsNonStreams(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]Value{
		"an Integer":          NewInteger(42),
		"an unrelated atom":   NewAtom("banana"),
		"a String constraint": NewDepScalar(DepLT, NewString("z")),
	} {
		if _, herr := ttyHandler([]Value{v}, nil, nil, r); herr == nil {
			t.Errorf("%s was accepted as a stream handle", name)
		}
	}
}
