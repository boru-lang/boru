package termback

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/boru-lang/boru/lang/go/tuikit"
)

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("write declined")
	}
	f.after--
	return len(p), nil
}

// swapSeams installs happy-path fakes and returns restore state via
// t.Cleanup (design/TEST-SEAMS.10.md discipline).
func swapSeams(t *testing.T, in *bytes.Buffer, out *bytes.Buffer) (*int, *int) {
	t.Helper()
	oldIsTerm, oldRaw, oldRestore, oldSize := isTerminal, makeRaw, restore, getSize
	oldIn, oldOut := ttyIn, ttyOut
	t.Cleanup(func() {
		isTerminal, makeRaw, restore, getSize = oldIsTerm, oldRaw, oldRestore, oldSize
		ttyIn, ttyOut = oldIn, oldOut
		ttyBusy.Store(false) // release the single-owner latch between tests
	})
	raws, restores := 0, 0
	isTerminal = func(int) bool { return true }
	makeRaw = func(int) (*term.State, error) { raws++; return &term.State{}, nil }
	restore = func(int, *term.State) error { restores++; return nil }
	getSize = func(int) (int, int, error) { return 10, 4, nil }
	ttyIn = func() (io.Reader, int) { return in, 0 }
	ttyOut = func() (io.Writer, int) { return out, 1 }
	return &raws, &restores
}

func TestSpecShape(t *testing.T) {
	spec := Spec()
	if spec.Name != "tty" || spec.Open == nil {
		t.Fatalf("spec = %+v", spec)
	}
	backend, err := spec.Open()
	if err != nil || backend == nil {
		t.Fatalf("spec.Open = %v, %v", backend, err)
	}
}

// SpecFor / NewFor bind the CALLER's streams and their own fds (a real
// *os.File → its Fd; a non-file → -1, which Open rejects as not_a_tty
// rather than falling back to the process TTY).
func TestSpecForBindsCallerStreams(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })

	if got := streamFD(r); got != int(r.Fd()) {
		t.Errorf("streamFD(*os.File) = %d, want %d", got, r.Fd())
	}
	if got := streamFD(&bytes.Buffer{}); got != -1 {
		t.Errorf("streamFD(non-file) = %d, want -1", got)
	}

	b := NewFor(r, w)
	if b.in != r || b.out != w || b.inFD != int(r.Fd()) || b.outFD != int(w.Fd()) {
		t.Fatalf("NewFor bound the wrong streams/fds: %+v", b)
	}

	spec := SpecFor(r, w)
	if spec.Name != "tty" || spec.Open == nil {
		t.Fatalf("spec = %+v", spec)
	}
	backend, err := spec.Open()
	if err != nil || backend == nil {
		t.Fatalf("spec.Open = %v, %v", backend, err)
	}

	// a non-file stream fails Open cleanly (never the process TTY)
	nb := NewFor(&bytes.Buffer{}, &bytes.Buffer{})
	if _, err := nb.Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "not_a_tty") {
		t.Fatalf("non-file Open = %v, want not_a_tty", err)
	}
}

func TestOpenLifecycleAndRejections(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	raws, restores := swapSeams(t, in, out)

	b := New()
	info, err := b.Open(tuikit.OpenOpts{Mouse: true, Title: "boot"})
	if err != nil || info.Cols != 10 || info.Rows != 4 {
		t.Fatalf("Open = %+v, %v", info, err)
	}
	setup := out.String()
	for _, want := range []string{seqAltScreenOn, seqCursorHide, seqPasteOn, seqClear, seqMouseOn, "\x1b]0;boot\x07"} {
		if !strings.Contains(setup, want) {
			t.Errorf("setup missing %q", want)
		}
	}
	if *raws != 1 {
		t.Errorf("raw calls = %d", *raws)
	}
	if _, err := b.Open(tuikit.OpenOpts{}); err == nil {
		t.Error("second Open accepted")
	}
	if c, r, err := b.Size(); err != nil || c != 10 || r != 4 {
		t.Errorf("Size = %d,%d,%v", c, r, err)
	}
	if b.Events() == nil {
		t.Error("Events nil after open")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	teardown := out.String()
	for _, want := range []string{seqMouseOff, seqPasteOff, seqCursorShow, seqAltScreenOff} {
		if !strings.Contains(teardown, want) {
			t.Errorf("teardown missing %q", want)
		}
	}
	if *restores != 1 {
		t.Errorf("restore calls = %d", *restores)
	}
	if err := b.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
	if _, err := b.Open(tuikit.OpenOpts{}); err == nil {
		t.Error("Open after Close accepted")
	}

	// a never-opened backend closes as a no-op
	b2 := New()
	if err := b2.Close(); err != nil {
		t.Errorf("unopened Close: %v", err)
	}
}

func TestOpenFailureArms(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	_, restores := swapSeams(t, in, out)

	isTerminal = func(int) bool { return false }
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "not_a_tty") {
		t.Fatalf("non-tty = %v", err)
	}
	isTerminal = func(int) bool { return true }

	makeRaw = func(int) (*term.State, error) { return nil, errors.New("no raw") }
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "no raw") {
		t.Fatalf("raw failure = %v", err)
	}
	makeRaw = func(int) (*term.State, error) { return &term.State{}, nil }

	getSize = func(int) (int, int, error) { return 0, 0, errors.New("no size") }
	before := *restores
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil || !strings.Contains(err.Error(), "no size") {
		t.Fatalf("size failure = %v", err)
	}
	if *restores != before+1 {
		t.Error("size failure did not restore raw mode")
	}
	getSize = func(int) (int, int, error) { return 10, 4, nil }

	fw := &failWriter{}
	ttyOut = func() (io.Writer, int) { return fw, 1 }
	before = *restores
	if _, err := New().Open(tuikit.OpenOpts{}); err == nil {
		t.Fatal("setup write failure accepted")
	}
	if *restores != before+1 {
		t.Error("setup write failure did not restore raw mode")
	}
}

func TestPresentDamageAndFailure(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	f := tuikit.NewFrame(4, 1)
	f.Set(0, 0, tuikit.Cell{Content: "h", Width: 1})
	f.Set(1, 0, tuikit.Cell{Content: "i", Width: 1, Style: tuikit.Style{Bold: true}})
	f.Set(2, 0, tuikit.Cell{Content: "漢", Width: 2})
	f.Set(3, 0, tuikit.Cell{Width: -1})
	if err := b.Present(f); err != nil {
		t.Fatal(err)
	}
	first := out.String()
	if !strings.Contains(first, "\x1b[1;1H") {
		t.Errorf("no cursor move: %q", first)
	}
	if !strings.Contains(first, "h") || !strings.Contains(first, "漢") {
		t.Errorf("content missing: %q", first)
	}
	if !strings.Contains(first, "\x1b[1m") {
		t.Errorf("style run missing: %q", first)
	}
	out.Reset()
	if err := b.Present(f); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("unchanged frame emitted %q", out.String())
	}
	// a blank cell paints a space
	f2 := f.Clone()
	f2.Set(0, 0, tuikit.Cell{})
	out.Reset()
	if err := b.Present(f2); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), " ") {
		t.Errorf("blank cell not spaced: %q", out.String())
	}

	fw := &failWriter{}
	b.out = fw
	f3 := f2.Clone()
	f3.Set(1, 0, tuikit.Cell{Content: "z", Width: 1})
	if err := b.Present(f3); err == nil {
		t.Error("present write failure accepted")
	}
}

func TestCursorTitleBell(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := b.SetCursor(2, 1, true); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "\x1b[2;3H") || !strings.Contains(got, seqCursorShow) {
		t.Errorf("cursor show = %q", got)
	}
	out.Reset()
	if err := b.SetCursor(0, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != seqCursorHide {
		t.Errorf("cursor hide = %q", got)
	}
	if err := b.SetTitle("t2"); err != nil {
		t.Fatal(err)
	}
	if err := b.Bell(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "\x1b]0;t2\x07") || !strings.Contains(got, "\x07") {
		t.Errorf("title/bell = %q", got)
	}
	b.out = &failWriter{}
	if err := b.SetCursor(0, 0, true); err == nil {
		t.Error("cursor write failure accepted")
	}
	if err := b.SetCursor(0, 0, false); err == nil {
		t.Error("cursor hide write failure accepted")
	}
	if err := b.SetTitle("x"); err == nil {
		t.Error("title write failure accepted")
	}
	if err := b.Bell(); err == nil {
		t.Error("bell write failure accepted")
	}
	// Close's teardown write failure surfaces
	if err := b.Close(); err == nil {
		t.Error("teardown write failure accepted")
	}
}

// --- the decoder ---

func decodeAll(t *testing.T, input string) []tuikit.Event {
	t.Helper()
	ch := make(chan tuikit.Event, 64)
	var closed atomic.Bool
	decodeInput(bufio.NewReader(strings.NewReader(input)), ch, &closed)
	close(ch) // the backend's decoder wrapper owns this in production
	var out []tuikit.Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestDecodeKeys(t *testing.T) {
	evs := decodeAll(t, "a \r\n\t\x7f\x08\x03\x1c\x00é漢")
	want := []tuikit.Event{
		{Tag: "key", Key: "a", Char: "a"},
		{Tag: "key", Key: "space", Char: " "},
		{Tag: "key", Key: "enter"},
		{Tag: "key", Key: "enter"},
		{Tag: "key", Key: "tab"},
		{Tag: "key", Key: "backspace"},
		{Tag: "key", Key: "backspace"},
		{Tag: "key", Key: "c", Mods: []string{"ctrl"}},
		{Tag: "key", Key: `\`, Mods: []string{"ctrl"}},
		{Tag: "key", Key: "é", Char: "é"},
		{Tag: "key", Key: "漢", Char: "漢"},
	}
	if len(evs) != len(want) {
		t.Fatalf("events = %+v", evs)
	}
	for i := range want {
		if evs[i].Tag != want[i].Tag || evs[i].Key != want[i].Key || evs[i].Char != want[i].Char {
			t.Errorf("ev %d = %+v, want %+v", i, evs[i], want[i])
		}
		if len(want[i].Mods) > 0 && (len(evs[i].Mods) == 0 || evs[i].Mods[0] != want[i].Mods[0]) {
			t.Errorf("ev %d mods = %v", i, evs[i].Mods)
		}
	}
}

func TestDecodeSequences(t *testing.T) {
	cases := []struct {
		in   string
		key  string
		tag  string
		text string
		kind string
	}{
		{"\x1b[A", "up", "key", "", ""},
		{"\x1b[H", "home", "key", "", ""},
		{"\x1b[Z", "backtab", "key", "", ""},
		{"\x1b[1;5C", "right", "key", "", ""},
		{"\x1b[3~", "delete", "key", "", ""},
		{"\x1b[15~", "f5", "key", "", ""},
		{"\x1bOA", "up", "key", "", ""},
		{"\x1bOP", "f1", "key", "", ""},
		{"\x1bOS", "f4", "key", "", ""},
		{"\x1bx", "x", "key", "", ""},
		{"\x1b", "esc", "key", "", ""},
		{"\x1b[200~hi there\x1b[201~", "", "paste", "hi there", ""},
		{"\x1b[<0;3;2M", "", "mouse", "", "press"},
		{"\x1b[<0;3;2m", "", "mouse", "", "release"},
		{"\x1b[<64;1;1M", "", "mouse", "", "wheel-up"},
		{"\x1b[<65;1;1M", "", "mouse", "", "wheel-down"},
		{"\x1b[<32;5;5M", "", "mouse", "", "move"},
	}
	for _, c := range cases {
		evs := decodeAll(t, c.in)
		if len(evs) != 1 {
			t.Fatalf("%q events = %+v", c.in, evs)
		}
		ev := evs[0]
		if ev.Tag != c.tag || ev.Key != c.key || ev.Text != c.text || ev.Kind != c.kind {
			t.Errorf("%q = %+v", c.in, ev)
		}
	}
	if evs := decodeAll(t, "\x1b[<0;3;2M"); evs[0].X != 2 || evs[0].Y != 1 {
		t.Errorf("mouse coords = %+v", evs[0])
	}
}

func TestDecodeDropsAndErrors(t *testing.T) {
	// dropped shapes decode to nothing
	for _, in := range []string{
		"\x1b[99~",    // unknown tilde
		"\x1b[5X",     // unhandled CSI final
		"\x1bOz",      // unknown SS3
		"\x1b[<;1;1M", // malformed mouse params (empty button field)
		"\x1b[<0;1M",  // short mouse params
	} {
		if evs := decodeAll(t, in); len(evs) != 0 {
			t.Errorf("%q delivered %+v", in, evs)
		}
	}
	// truncated sequences end the decoder without panic
	for _, in := range []string{"\x1b[", "\x1bO", "\x1b[200~ab", "\x1b[200~a\x1b"} {
		if evs := decodeAll(t, in); len(evs) != 0 {
			t.Errorf("truncated %q delivered %+v", in, evs)
		}
	}
	// a closed backend drops the event and stops the decoder
	ch := make(chan tuikit.Event, 4)
	var closed atomic.Bool
	closed.Store(true)
	decodeInput(bufio.NewReader(strings.NewReader("abc")), ch, &closed)
	close(ch)
	if _, ok := <-ch; ok {
		t.Error("closed decoder delivered an event")
	}
}

// The single-owner latch: a second Open while the TTY is taken fails
// loudly; Close (and a failed Open) release it (review finding: two
// Tui.open calls stacked raw modes with competing decoders).
func TestSingleOwnerLatch(t *testing.T) {
	// each backend gets its own buffers: a closed backend's decoder may
	// still be draining its input when the next one opens
	fresh := func() *Backend {
		swapSeams(t, &bytes.Buffer{}, &bytes.Buffer{})
		return New()
	}
	a := fresh()
	if _, err := a.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	b := fresh()
	if _, err := b.Open(tuikit.OpenOpts{}); err == nil ||
		!strings.Contains(err.Error(), "already owned") {
		t.Fatalf("second open = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	c := fresh()
	if _, err := c.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatalf("open after close = %v", err)
	}
	_ = c.Close()
	// a failed open never leaves the latch held
	failRaw := fresh()
	oldRaw := makeRaw
	makeRaw = func(int) (*term.State, error) { return nil, errors.New("raw declined") }
	if _, err := failRaw.Open(tuikit.OpenOpts{}); err == nil {
		t.Fatal("raw failure accepted")
	}
	makeRaw = oldRaw
	d := fresh()
	if _, err := d.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatalf("open after failed open = %v", err)
	}
	_ = d.Close()
}

// The resize watcher: seam-fed ticks become resize events; Close ends
// the watcher via the stop channel (review finding: no SIGWINCH path
// existed, so real TTY apps never heard window resizes).
func TestResizeWatcher(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	// park the decoder on a blocking input: an instant EOF would close
	// the events channel and drop the watcher's event under the latch
	blocker := &blockingReader{unblock: make(chan struct{})}
	oldIn := ttyIn
	ttyIn = func() (io.Reader, int) { return blocker, 0 }
	oldInterrupt := interruptRead
	interruptRead = func(r io.Reader) {
		if br, ok := r.(*blockingReader); ok {
			br.Interrupt()
		}
	}
	t.Cleanup(func() { ttyIn = oldIn; interruptRead = oldInterrupt })
	ticks := make(chan struct{}, 2)
	oldSignal := resizeSignal
	var gotStop <-chan struct{}
	resizeSignal = func(stop <-chan struct{}) <-chan struct{} {
		gotStop = stop
		return ticks
	}
	t.Cleanup(func() { resizeSignal = oldSignal })

	// the watcher captures the size seam at Open, so the fake goes in
	// first: call 1 feeds Open itself, call 2 is the transient skip
	// arm, call 3 is the resize the watcher reports
	calls := 0
	oldSize := getSize
	getSize = func(int) (int, int, error) {
		calls++
		switch calls {
		case 1:
			return 80, 24, nil
		case 2:
			return 0, 0, errors.New("transient") // the skip arm
		default:
			return 33, 9, nil
		}
	}
	t.Cleanup(func() { getSize = oldSize })
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	ticks <- struct{}{} // swallowed by the transient size failure
	ticks <- struct{}{}
	select {
	case ev := <-b.Events():
		if ev.Tag != "resize" || ev.Cols != 33 || ev.Rows != 9 {
			t.Fatalf("resize event = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no resize event from the watcher")
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gotStop:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not signal the watcher stop channel")
	}
	close(ticks) // the watcher range ends

	// a tick landing after Close exits through the closed check (run
	// synchronously so the arm is deterministic)
	dead := New()
	dead.closed.Store(true)
	tick2 := make(chan struct{}, 1)
	tick2 <- struct{}{}
	dead.watchResize(tick2)
}

// interruptRead unblocks a decoder stuck in Read at Close, so the next
// keystroke after a session is not consumed by a stale goroutine.
func TestCloseInterruptsBlockedReader(t *testing.T) {
	blocker := &blockingReader{unblock: make(chan struct{})}
	oldIn := ttyIn
	ttyIn = func() (io.Reader, int) { return blocker, 0 }
	inBuf, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, inBuf, out)
	ttyIn = func() (io.Reader, int) { return blocker, 0 } // after swapSeams reset
	oldInterrupt := interruptRead
	interruptRead = func(in io.Reader) {
		if br, ok := in.(*blockingReader); ok {
			br.Interrupt()
		}
	}
	t.Cleanup(func() { ttyIn = oldIn; interruptRead = oldInterrupt })

	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	events := b.Events()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	// the decoder exits promptly (channel closes) without another byte
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("unexpected event from an interrupted decoder")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("decoder stayed blocked after Close")
	}
	// the default interruptRead tolerates non-file readers (no panic)
	oldInterrupt(inBuf)
	// and pokes a real file's read deadline (a pipe stands in for the tty)
	pr, pw, pErr := os.Pipe()
	if pErr != nil {
		t.Fatal(pErr)
	}
	oldInterrupt(pr)
	_ = pr.Close()
	_ = pw.Close()
}

// blockingReader blocks Read until interrupted, then errors.
type blockingReader struct {
	unblock chan struct{}
	once    sync.Once
}

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.unblock
	return 0, errors.New("interrupted")
}

func (r *blockingReader) Interrupt() { r.once.Do(func() { close(r.unblock) }) }

// Control bytes in cell content never reach the TTY raw (review
// finding: ESC/OSC smuggled through widget text desynchronizes the
// diff and can inject terminal commands).
func TestPresentSanitizesControlBytes(t *testing.T) {
	in, out := &bytes.Buffer{}, &bytes.Buffer{}
	swapSeams(t, in, out)
	b := New()
	if _, err := b.Open(tuikit.OpenOpts{}); err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	out.Reset()
	f := tuikit.NewFrame(10, 4)
	f.Set(0, 0, tuikit.Cell{Content: "\x1b", Width: 1})
	f.Set(1, 0, tuikit.Cell{Content: "\x07", Width: 1})
	f.Set(2, 0, tuikit.Cell{Content: "x", Width: 1})
	if err := b.Present(f); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "\x1b]") || strings.Contains(got, "\x07") {
		t.Fatalf("control bytes leaked to the tty: %q", got)
	}
	if !strings.Contains(got, "x") {
		t.Fatalf("printable content lost: %q", got)
	}
	if s := sanitizeCell("clean"); s != "clean" {
		t.Fatalf("clean fast path = %q", s)
	}
	if s := sanitizeCell("a\x1bb\x7f"); s != "a b " {
		t.Fatalf("sanitize = %q", s)
	}
}
