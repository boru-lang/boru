package native

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// watchReg builds a registry with the io words, a mem FS, and a `snoop`
// native that captures each callback's event record (the fork shares
// native registrations, so the callback body can reach it — the same
// observation pattern the interval tests use).
func watchReg(t *testing.T) (*Registry, func() []Value) {
	t.Helper()
	r, mem := ioFSReg(t)
	_ = mem
	var mu sync.Mutex
	var seen []Value
	r.Register("snoop", Signature{
		Args: []*Type{TAny},
		Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			mu.Lock()
			seen = append(seen, a[0])
			mu.Unlock()
			return nil, nil
		}),
		BarrierPos: -1,
	})
	return r, func() []Value {
		mu.Lock()
		defer mu.Unlock()
		out := make([]Value, len(seen))
		copy(out, seen)
		return out
	}
}

// awaitSnoop polls until at least n events were captured (the callback
// pump is asynchronous) or the deadline passes.
func awaitSnoop(t *testing.T, snapshot func() []Value, n int) []Value {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if evs := snapshot(); len(evs) >= n {
			return evs
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d callback event(s); got %d", n, len(snapshot()))
	return nil
}

func TestWatchWordDeliversEvents(t *testing.T) {
	r, snapshot := watchReg(t)
	runBoru(t, r, []Value{NewWord("folder"), pathV("d")})

	// watch d [snoop] — each event record is passed to the snoop native.
	body := NewList([]Value{NewWord("snoop")})
	body.Quoted = true
	res := runBoru(t, r, []Value{NewWord("watch"), pathV("d"), body})
	if len(res) != 1 {
		t.Fatalf("watch returned %d values", len(res))
	}
	wi, ok := asWatcherInfo(res[0])
	if !ok {
		t.Fatalf("watch returned %v, want a Watcher handle", res[0])
	}
	if res[0].Parent.Name() != "Watcher" {
		t.Errorf("watcher type = %s", res[0].Parent)
	}
	if !strings.Contains(res[0].String(), "Watcher(") {
		t.Errorf("watcher renders as %s", res[0])
	}

	// A write inside the watched dir triggers the callback with the
	// {op, path} record.
	runBoru(t, r, []Value{NewWord("write"), pathV("d/a.txt"), NewString("x")})
	evs := awaitSnoop(t, snapshot, 1)
	m, err := AsMap(evs[0])
	if err != nil || m == nil {
		t.Fatalf("event record = %v (%v)", evs[0], err)
	}
	if op, _ := m.Get("op"); func() string { a, _ := op.AsConcreteAtom(); return a }() != "create" {
		t.Errorf("event op = %v, want create atom", func() Value { v, _ := m.Get("op"); return v }())
	}
	p, _ := m.Get("path")
	if !IsPathon(p) || p.String() != "d/a.txt" {
		t.Errorf("event path = %v, want the Pathon d/a.txt", p)
	}

	// A remove triggers again; unwatch stops the stream and the pump.
	runBoru(t, r, []Value{NewWord("remove"), pathV("d/a.txt")})
	awaitSnoop(t, snapshot, 2)
	runBoru(t, r, []Value{NewWord("unwatch"), res[0]})
	select {
	case <-wi.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("callback pump did not exit after unwatch")
	}
	// Post-unwatch mutations deliver nothing.
	before := len(snapshot())
	runBoru(t, r, []Value{NewWord("write"), pathV("d/late.txt"), NewString("z")})
	time.Sleep(10 * time.Millisecond)
	if got := len(snapshot()); got != before {
		t.Errorf("events after unwatch: %d -> %d", before, got)
	}
	// unwatch is idempotent through the word too.
	runBoru(t, r, []Value{NewWord("unwatch"), res[0]})
}

// TestWatchWordOptsFilter covers the option-bearing watch: {recursive:true}
// is threaded to the backend and {match:"*.txt"} filters events word-side —
// a non-matching write is dropped, so only .txt records reach the callback.
func TestWatchWordOptsFilter(t *testing.T) {
	r, snapshot := watchReg(t)
	runBoru(t, r, []Value{NewWord("folder"), pathV("d")})
	body := NewList([]Value{NewWord("snoop")})
	body.Quoted = true
	opts := NewOrderedMap()
	opts.Set("recursive", NewBoolean(true))
	opts.Set("match", NewString("*.txt"))
	res := runBoru(t, r, []Value{NewWord("watch"), pathV("d"), body, NewMap(opts)})
	if len(res) != 1 {
		t.Fatalf("watch returned %d values", len(res))
	}
	// A matching write fires; a non-matching one is filtered; the following
	// match fires — awaitSnoop(2) proves the .log did not count.
	runBoru(t, r, []Value{NewWord("write"), pathV("d/keep.txt"), NewString("x")})
	awaitSnoop(t, snapshot, 1)
	runBoru(t, r, []Value{NewWord("write"), pathV("d/skip.log"), NewString("y")})
	runBoru(t, r, []Value{NewWord("write"), pathV("d/again.txt"), NewString("z")})
	evs := awaitSnoop(t, snapshot, 2)
	for _, ev := range evs {
		m, _ := AsMap(ev)
		p, _ := m.Get("path")
		if !strings.HasSuffix(p.String(), ".txt") {
			t.Errorf("a non-.txt event slipped through {match}: %v", p)
		}
	}
	runBoru(t, r, []Value{NewWord("unwatch"), res[0]})
}

// TestWatchRel covers watchRel's relative-path computation and every
// fallback: the root itself (rel "."), a path outside the root (rel ".."),
// and an unrelatable mixed absolute/relative pair (Rel error).
func TestWatchRel(t *testing.T) {
	cases := []struct{ root, ev, want string }{
		{"d", "d/sub/x.txt", "sub/x.txt"},
		{"d", "d", "d"},       // rel "." → base name
		{"d", "other/x", "x"}, // rel "../other/x" → base name
		{"d", "/abs/x", "x"},  // Rel error (mixed abs/rel) → base name
	}
	for _, c := range cases {
		if got := watchRel(c.root, c.ev); got != c.want {
			t.Errorf("watchRel(%q,%q) = %q, want %q", c.root, c.ev, got, c.want)
		}
	}
}

func TestWatchWordErrors(t *testing.T) {
	r, _ := watchReg(t)
	body := NewList([]Value{NewWord("snoop")})
	body.Quoted = true
	// Watching an absent path errors.
	if err := runBoruError(t, r, []Value{NewWord("watch"), pathV("ghost"), body}); err == nil {
		t.Error("expected watch of an absent path to error")
	}
	// A non-concrete body list is declined.
	if _, err := doWatchWord([]Value{pathV("d"), NewTypeLiteral(TList)}, r, MintWatcherType(r), Value{}); err == nil {
		t.Error("expected a type-literal body to be declined")
	}
	// unwatch of a non-Watcher is a clean error.
	if _, err := doUnwatchWord([]Value{NewInteger(1)}, r); err == nil {
		t.Error("expected unwatch of a non-Watcher to error")
	}
}

func TestWatcherFormatAndStop(t *testing.T) {
	// The nil-payload render arm and the direct Stop error path.
	if got := (watcherFormatBehavior{}).Format(NewInteger(1)); got != "Watcher(nil)" {
		t.Errorf("format of non-watcher = %q", got)
	}
	calls := 0
	wi := &WatcherInfo{ID: "W_x", Path: "p", stop: func() error { calls++; return nil }, done: make(chan struct{})}
	if err := wi.Stop(); err != nil || calls != 1 {
		t.Errorf("first stop: err=%v calls=%d", err, calls)
	}
	if err := wi.Stop(); err != nil || calls != 1 {
		t.Errorf("second stop must be a no-op: err=%v calls=%d", err, calls)
	}
	// watchEventValue builds the {op, path} record.
	ev := watchEventValue("write", "/a/b.txt")
	m, err := AsMap(ev)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := m.Get("path"); !IsPathon(p) {
		t.Errorf("event path not a Pathon: %v", p)
	}
}

// TestWatchCoverageArms covers the remaining branches: the behavior's
// Equal delegate, the standalone NewWatcherType constructor, the
// stop-error surface through unwatch, and the not-installed / policy
// gates on Watch.
func TestWatchCoverageArms(t *testing.T) {
	// Behavior Equal delegates to the kernel default.
	wt := NewWatcherType()
	if wt == nil || wt.Name() != "Watcher" {
		t.Fatalf("NewWatcherType = %v", wt)
	}
	a := NewInteger(1)
	if !(watcherFormatBehavior{}).Equal(a, a) {
		t.Error("behavior Equal should delegate to the default")
	}
	// A failing stop surfaces as an unwatch_error.
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	wi := &WatcherInfo{ID: "W_e", Path: "p", stop: func() error { return fmt.Errorf("stop boom") }, done: make(chan struct{})}
	h := NewValueRaw(wt, ExtensionPayload{Body: wi})
	if _, err := doUnwatchWord([]Value{h}, r); err == nil || !strings.Contains(err.Error(), "unwatch") {
		t.Errorf("failing stop through unwatch = %v", err)
	}
	// The not-installed stub declines Watch.
	if _, _, err := (notInstalledFileOps{}).Watch("p", capabilities.WatchOpts{}); err == nil {
		t.Error("not-installed Watch must decline")
	}
}
