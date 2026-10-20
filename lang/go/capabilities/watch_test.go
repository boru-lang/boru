package capabilities

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// recvEvent pulls one event with a timeout (mem events are synchronous
// with the mutation, so the generous timeout only matters for fsnotify).
func recvEvent(t *testing.T, ch <-chan WatchEvent) WatchEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event stream closed early")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a watch event")
		return WatchEvent{}
	}
}

// drainUntil pulls events until one matches (op, pathSuffix) — fsnotify
// hosts may interleave extra events (a create may arrive as create+write).
func drainUntil(t *testing.T, ch <-chan WatchEvent, op, pathSuffix string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("stream closed before %s %s arrived", op, pathSuffix)
			}
			if ev.Op == op && filepath.Base(ev.Path) == pathSuffix {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s %s", op, pathSuffix)
		}
	}
}

func TestMemWatchEvents(t *testing.T) {
	m := NewMem()
	if err := m.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := m.Watch("d", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	// Create / write / chmod / remove on a direct child, in order — mem
	// delivery is synchronous, so the sequence is deterministic.
	if err := m.WriteFile("d/f.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/f.txt") {
		t.Errorf("create event = %+v", ev)
	}
	if err := m.WriteFile("d/f.txt", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "write" || ev.Path != filepath.FromSlash("d/f.txt") {
		t.Errorf("write event = %+v", ev)
	}
	if err := m.Truncate("d/f.txt", 0); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "write" {
		t.Errorf("truncate event = %+v", ev)
	}
	if err := m.Chmod("d/f.txt", 0o600); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "chmod" {
		t.Errorf("chmod event = %+v", ev)
	}
	if err := m.Chtimes("d/f.txt", time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "chmod" {
		t.Errorf("chtimes event = %+v", ev)
	}
	// Rename emits rename(old) + create(new).
	if err := m.Rename("d/f.txt", "d/g.txt"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "rename" || ev.Path != filepath.FromSlash("d/f.txt") {
		t.Errorf("rename event = %+v", ev)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/g.txt") {
		t.Errorf("rename-create event = %+v", ev)
	}
	// Symlink and hard-link creations are events too.
	if err := m.Symlink("g.txt", "d/sl"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/sl") {
		t.Errorf("symlink event = %+v", ev)
	}
	if err := m.Link("d/g.txt", "d/hl"); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/hl") {
		t.Errorf("hard-link event = %+v", ev)
	}
	if err := m.Remove("d/g.txt", false); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "remove" || ev.Path != filepath.FromSlash("d/g.txt") {
		t.Errorf("remove event = %+v", ev)
	}
	// A mutation OUTSIDE the watched dir is not delivered (non-recursive).
	if err := m.WriteFile("elsewhere.txt", []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.MkdirAll("d/sub", 0o755); err != nil { // this one IS a child
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/sub") {
		t.Errorf("expected the child mkdir next (outside write skipped), got %+v", ev)
	}
	if err := m.WriteFile("d/sub/deep.txt", []byte("q"), 0o644); err != nil {
		t.Fatal(err) // grandchild: not delivered
	}
	// Stop closes the stream; a second stop is an idempotent no-op.
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-ch; ok {
		t.Error("stream not closed by stop")
	}
	if err := stop(); err != nil {
		t.Errorf("second stop = %v", err)
	}
	// After stop, mutations emit to nobody (no panic, no delivery).
	if err := m.WriteFile("d/after.txt", []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMemWatchFileTarget(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("f.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := m.Watch("f.txt", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()
	if err := m.WriteFile("f.txt", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "write" || ev.Path != "f.txt" {
		t.Errorf("file-target event = %+v", ev)
	}
	// Watching an ABSENT path errors (fsnotify parity).
	if _, _, err := m.Watch("ghost", WatchOpts{}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("watch of absent path = %v", err)
	}
	// A slow consumer drops events past the buffer rather than blocking.
	for i := 0; i < watchBufferSize+8; i++ {
		if err := m.WriteFile("f.txt", []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOSWatchEvents(t *testing.T) {
	root := t.TempDir()
	o := &OSFileOps{}
	ch, stop, err := o.Watch(root, WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(root, "f.txt")
	if err := o.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "create", "f.txt")
	if err := o.WriteFile(f, []byte("more"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "write", "f.txt")
	if err := o.Remove(f, false); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "remove", "f.txt")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	// The adapter closes the out stream once fsnotify's channels close.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream not closed after stop")
		}
	}
}

func TestOSWatchAbsentErrors(t *testing.T) {
	o := &OSFileOps{}
	if _, _, err := o.Watch(filepath.Join(t.TempDir(), "ghost"), WatchOpts{}); err == nil {
		t.Error("expected watching an absent path to error")
	}
}

func TestOverlayWatchSeesUnionMutations(t *testing.T) {
	lower := NewMem()
	if err := lower.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lower.WriteFile("d/base.txt", []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(NewMem(), lower)
	ch, stop, err := o.Watch("d", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	// A union mutation (upper write) is observed.
	if err := o.WriteFile("d/new.txt", []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "create", "new.txt")
	// A LOWER-side mutation (external change to the real layer) is
	// observed through the merged stream too.
	if err := lower.WriteFile("d/base.txt", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "write", "base.txt")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	// Watching an absent union path errors.
	if _, _, err := o.Watch("nowhere", WatchOpts{}); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("overlay watch of absent path = %v", err)
	}
	// Watching a lower-only FILE materialises it into the upper first.
	o2 := NewOverlay(NewMem(), lower)
	ch2, stop2, err := o2.Watch("d/base.txt", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if err := o2.WriteFile("d/base.txt", []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch2, "write", "base.txt")
	_ = stop2()
}

// recordingAdder is a watchAdder that records every auto-added path; err
// lets a test assert the pump swallows a failing Add.
type recordingAdder struct {
	added []string
	err   error
}

func (r *recordingAdder) Add(name string) error {
	r.added = append(r.added, name)
	return r.err
}

// TestPumpFsnotify drives the fsnotify adapter deterministically with a
// synthetic source: op lowering, the empty-mask skip, and close-on-
// source-close (non-recursive, no overflow).
func TestPumpFsnotify(t *testing.T) {
	src := make(chan fsnotify.Event, 16)
	out := make(chan WatchEvent, watchBufferSize)
	// One of each op, plus an empty mask (skipped).
	src <- fsnotify.Event{Name: "a", Op: fsnotify.Create}
	src <- fsnotify.Event{Name: "a", Op: fsnotify.Write}
	src <- fsnotify.Event{Name: "a", Op: fsnotify.Remove}
	src <- fsnotify.Event{Name: "a", Op: fsnotify.Rename}
	src <- fsnotify.Event{Name: "a", Op: fsnotify.Chmod}
	src <- fsnotify.Event{Name: "a", Op: 0} // empty mask: skipped
	close(src)
	pumpFsnotify(src, &recordingAdder{}, out, "root", false) // synchronous: source is pre-closed

	var got []string
	for ev := range out {
		got = append(got, ev.Op)
	}
	want := []string{"create", "write", "remove", "rename", "chmod"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, op := range want {
		if got[i] != op {
			t.Fatalf("event[%d] = %q, want %q (all: %v)", i, got[i], op, got)
		}
	}
}

// TestPumpFsnotifyRecursiveAutoAdd covers the chokidar-style auto-add: a
// created directory is registered with the adder, a created file and a
// vanished path are not, and a non-create op is ignored — driven through
// real os.Stat on a temp tree so no fake FileInfo is needed.
func TestPumpFsnotifyRecursiveAutoAdd(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "gone")

	src := make(chan fsnotify.Event, 8)
	out := make(chan WatchEvent, watchBufferSize)
	adder := &recordingAdder{err: errors.New("add error is swallowed")}
	src <- fsnotify.Event{Name: dir, Op: fsnotify.Create}  // directory: auto-added
	src <- fsnotify.Event{Name: file, Op: fsnotify.Create} // file: not added
	src <- fsnotify.Event{Name: gone, Op: fsnotify.Create} // stat errors: not added
	src <- fsnotify.Event{Name: dir, Op: fsnotify.Write}   // non-create: ignored
	close(src)
	pumpFsnotify(src, adder, out, root, true)
	for range out { // drain
	}
	if len(adder.added) != 1 || adder.added[0] != dir {
		t.Errorf("auto-added = %v, want [%s]", adder.added, dir)
	}
}

// TestWatchOverflowCoalescing exercises every arm of the sendWatch /
// flushOverflow overflow-coalescing helpers with a tiny buffer.
func TestWatchOverflowCoalescing(t *testing.T) {
	out := make(chan WatchEvent, 2)
	overflow := false
	sendWatch(out, WatchEvent{Op: "write", Path: "a"}, "root", &overflow)
	sendWatch(out, WatchEvent{Op: "write", Path: "b"}, "root", &overflow)
	if overflow {
		t.Fatal("buffer not full yet: overflow should be false")
	}
	sendWatch(out, WatchEvent{Op: "write", Path: "c"}, "root", &overflow) // dropped, sets flag
	if !overflow {
		t.Fatal("a full buffer must set the overflow flag")
	}
	sendWatch(out, WatchEvent{Op: "write", Path: "d"}, "root", &overflow) // marker can't flush; stays set
	if !overflow {
		t.Fatal("still full: overflow must stay set")
	}
	<-out                                                                 // free one slot (removes "a")
	sendWatch(out, WatchEvent{Op: "write", Path: "e"}, "root", &overflow) // marker flushes; "e" then drops
	// Buffer now holds "b" then the coalesced overflow marker, in order.
	if first := <-out; first.Path != "b" {
		t.Errorf("first drained = %+v, want b", first)
	}
	if second := <-out; second.Op != watchOpOverflow || second.Path != "root" {
		t.Errorf("second drained = %+v, want overflow(root)", second)
	}
	// With room for BOTH the marker and the event, overflow clears fully.
	overflow = true
	sendWatch(out, WatchEvent{Op: "write", Path: "f"}, "root", &overflow)
	if overflow {
		t.Error("with room for marker and event, overflow should clear")
	}
	if a := <-out; a.Op != watchOpOverflow {
		t.Errorf("expected the marker first, got %+v", a)
	}
	if b := <-out; b.Path != "f" {
		t.Errorf("expected f after the marker, got %+v", b)
	}
	// flushOverflow delivers a pending marker when there is room...
	overflow = true
	flushOverflow(out, "root", overflow)
	if ev := <-out; ev.Op != watchOpOverflow {
		t.Errorf("flushOverflow emitted %+v, want overflow", ev)
	}
	// ...and is a no-op when nothing overflowed.
	flushOverflow(out, "root", false)
	select {
	case ev := <-out:
		t.Errorf("flushOverflow(false) emitted %+v, want nothing", ev)
	default:
	}
}

func TestFsnotifyOpNameTable(t *testing.T) {
	cases := map[string]fsnotify.Op{
		"create": fsnotify.Create, "write": fsnotify.Write,
		"remove": fsnotify.Remove, "rename": fsnotify.Rename,
		"chmod": fsnotify.Chmod, "": 0,
	}
	for want, op := range cases {
		if got := fsnotifyOpName(op); got != want {
			t.Errorf("fsnotifyOpName(%v) = %q, want %q", op, got, want)
		}
	}
}

func TestForwardWatchEventsDropOnFull(t *testing.T) {
	in := make(chan WatchEvent, 4)
	out := make(chan WatchEvent, 1) // room for exactly one
	done := make(chan struct{}, 1)
	in <- WatchEvent{Op: "write", Path: "a"}
	in <- WatchEvent{Op: "write", Path: "b"} // dropped: out is full
	close(in)
	forwardWatchEvents(in, out, "root", done)
	<-done
	if len(out) != 1 {
		t.Errorf("out holds %d events, want 1 (second dropped)", len(out))
	}
}

// TestMemWatchRecursive covers the {recursive} subtree matching: a
// descendant many levels down is delivered, while a sibling that merely
// shares a name prefix is not.
func TestMemWatchRecursive(t *testing.T) {
	m := NewMem()
	if err := m.MkdirAll("d/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := m.Watch("d", WatchOpts{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stop() }()
	// A grandchild write is delivered under {recursive}.
	if err := m.WriteFile("d/sub/deep.txt", []byte("q"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/sub/deep.txt") {
		t.Errorf("recursive grandchild = %+v", ev)
	}
	// A prefix-sharing sibling ("dd.txt" is not under "d/") is skipped, so
	// the next delivery is the direct child that follows it.
	if err := m.WriteFile("dd.txt", []byte("z"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("d/top.txt", []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ev := recvEvent(t, ch); ev.Op != "create" || ev.Path != filepath.FromSlash("d/top.txt") {
		t.Errorf("expected d/top.txt (sibling dd.txt skipped), got %+v", ev)
	}
}

// TestOSWatchRecursive covers the recursive WalkDir add and chokidar auto-
// add against a real fsnotify watcher: a file created in a subdirectory that
// existed at watch time surfaces, proving the subtree was registered.
func TestOSWatchRecursive(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An existing file exercises the WalkDir callback's non-directory arm.
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := &OSFileOps{}
	ch, stop, err := o.Watch(root, WatchOpts{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "sub", "deep.txt")
	if err := o.WriteFile(deep, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "create", "deep.txt")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

// TestOSWatchRecursiveWalkError covers the WalkDir failure arm: an error
// from the walk (here, a callback invoked with a non-nil err via the seam)
// closes the watcher and propagates.
func TestOSWatchRecursiveWalkError(t *testing.T) {
	orig := watchWalkDir
	watchWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk entry boom"))
	}
	defer func() { watchWalkDir = orig }()
	if _, _, err := (&OSFileOps{}).Watch(t.TempDir(), WatchOpts{Recursive: true}); err == nil {
		t.Error("expected the WalkDir failure to propagate")
	}
}

func TestOSWatchConstructorError(t *testing.T) {
	// The constructor seam: fd-exhaustion (untestable directly) propagates.
	orig := newFsnotifyWatcher
	newFsnotifyWatcher = func() (*fsnotify.Watcher, error) { return nil, errors.New("no fds") }
	defer func() { newFsnotifyWatcher = orig }()
	if _, _, err := (&OSFileOps{}).Watch(t.TempDir(), WatchOpts{}); err == nil {
		t.Error("expected the constructor failure to propagate")
	}
}

func TestDrainWatchErrors(t *testing.T) {
	errs := make(chan error, 2)
	errs <- errors.New("overflow")
	errs <- errors.New("race")
	close(errs)
	drainWatchErrors(errs) // returns once closed; errors observed and dropped
}

func TestOSWatchResolveError(t *testing.T) {
	o := &OSFileOps{getwd: func() (string, error) { return "", errors.New("getwd boom") }}
	if _, _, err := o.Watch("relative.txt", WatchOpts{}); err == nil {
		t.Error("expected the ResolvePath failure to propagate")
	}
}

func TestMemWatchResolveError(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("plain", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Watch("plain/under", WatchOpts{}); !errors.Is(err, errMemNotDir) {
		t.Errorf("watch through a file = %v, want ENOTDIR", err)
	}
}

// watchFailOps decorates a MemFileOps with a failing / error-stopping
// Watch, for the overlay's upper-watch error arms.
type watchFailOps struct {
	*MemFileOps
	failWatch bool
	stopErr   error
}

func (w *watchFailOps) Watch(path string, opts WatchOpts) (<-chan WatchEvent, func() error, error) {
	if w.failWatch {
		return nil, nil, errors.New("watch boom")
	}
	ch, stop, err := w.MemFileOps.Watch(path, opts)
	if err != nil {
		return nil, nil, err
	}
	if w.stopErr != nil {
		return ch, func() error { _ = stop(); return w.stopErr }, nil
	}
	return ch, stop, nil
}

func TestOverlayWatchErrorArms(t *testing.T) {
	// deref cycle.
	o, _, _ := overlayPair(nil)
	if err := o.Symlink("b", "a"); err != nil {
		t.Fatal(err)
	}
	if err := o.Symlink("a", "b"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.Watch("a", WatchOpts{}); !errors.Is(err, errMemTooManyLinks) {
		t.Errorf("watch cycle = %v", err)
	}
	// Materialise failures: dir MkdirAll and file copy-up.
	lower := NewMem()
	if err := lower.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lower.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	o2 := NewOverlay(&overlayFailOps{MemFileOps: NewMem(), failMkdir: true}, lower)
	if _, _, err := o2.Watch("d", WatchOpts{}); err == nil {
		t.Error("expected the upper MkdirAll failure to propagate")
	}
	o3 := NewOverlay(&overlayFailOps{MemFileOps: NewMem(), failWrite: true}, lower)
	if _, _, err := o3.Watch("f", WatchOpts{}); err == nil {
		t.Error("expected the copy-up failure to propagate")
	}
	// The upper's own Watch failure propagates.
	o4 := NewOverlay(&watchFailOps{MemFileOps: NewMem(), failWatch: true}, lower)
	if _, _, err := o4.Watch("d", WatchOpts{}); err == nil {
		t.Error("expected the upper Watch failure to propagate")
	}
	// An upper-only path (absent below): the lower Watch fails and the
	// union stream still works on the upper alone.
	o5, _, _ := overlayPair(nil)
	if err := o5.WriteFile("up/only.txt", []byte("u"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch, stop, err := o5.Watch("up", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if err := o5.WriteFile("up/second.txt", []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	drainUntil(t, ch, "create", "second.txt")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	// A stop error from a layer surfaces from the union stop.
	upper6 := &watchFailOps{MemFileOps: NewMem(), stopErr: errors.New("stop boom")}
	if err := upper6.MemFileOps.MkdirAll("d", 0o755); err != nil {
		t.Fatal(err)
	}
	o6 := NewOverlay(upper6, lower)
	_, stop6, err := o6.Watch("d", WatchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if err := stop6(); err == nil {
		t.Error("expected the layer stop error to surface")
	}
}
