package modules

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// The accept/broadcast race (CI failure on f2e95c4, PR #410):
// tuiWriteAccept was called after admit had released the hub lock, and
// setTitle/broadcastFrame take that same lock and write to every connection in
// h.viewers. So a concurrent broadcast could reach a freshly published socket
// before the accept landed, and the client's next line was `title` or `frame`
// where the protocol requires `accept`.
//
// The race itself is timing-dependent — 40 local runs and -race were green,
// and only CI contention surfaced it — so these tests pin the MECHANISM that
// closes it rather than trying to lose the race on demand. A timing test that
// passes 40 times and fails on the 41st is not a gate.

// readLines drains conn into a channel, one line per send.
func readLines(t *testing.T, conn net.Conn, n int) chan string {
	t.Helper()
	lines := make(chan string, n)
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	return lines
}

// A PENDING viewer receives nothing at all: not a frame, not a title. It holds
// its slot and its reader token — it is in the session — but the wire stays
// silent until its accept has landed.
func TestPendingViewerReceivesNoBroadcast(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	client, server := net.Pipe()
	defer client.Close()
	lines := readLines(t, client, 8)

	id, ok := hub.admitPending(server)
	if !ok {
		t.Fatal("admitPending declined")
	}
	// Both broadcast paths, while the viewer is pending. Neither may write:
	// if either did, net.Pipe is unbuffered and the write would block until
	// the deadline, so a regression shows up as a dropped viewer too.
	hub.setTitle("served")
	if !hub.broadcastFrame([]byte(`{"w":"text","text":"x"}`)) {
		t.Fatal("a pending viewer still holds the session open")
	}
	select {
	case got := <-lines:
		t.Fatalf("a pending viewer received %q before its accept", got)
	default:
	}

	// Promotion writes the accept and THEN the chrome it missed, in one lock
	// acquisition — the hello -> accept -> chrome order the protocol requires.
	if err := hub.promote(id); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := <-lines; !strings.Contains(got, `"tag":"accept"`) {
		t.Fatalf("first line after promote = %s, want the accept", got)
	}
	second := <-lines
	if !strings.Contains(second, `"tag":"title"`) || !strings.Contains(second, `"text":"served"`) {
		t.Fatalf("second line after promote = %s, want the title", second)
	}
	third := <-lines
	if !strings.Contains(third, `"tag":"frame"`) || !strings.Contains(third, `"text":"x"`) {
		t.Fatalf("third line after promote = %s, want the frame", third)
	}

	// And once promoted it is an ordinary viewer.
	if !hub.broadcastFrame([]byte(`{"w":"text","text":"y"}`)) {
		t.Fatal("session closed after promote")
	}
	if got := <-lines; !strings.Contains(got, `"text":"y"`) {
		t.Fatalf("promoted viewer missed a live frame: %s", got)
	}
	if len(hub.pending) != 0 {
		t.Errorf("pending not cleared: %v", hub.pending)
	}
}

// A pending viewer occupies a slot — it is admitted, so it counts against max
// exactly as a live viewer does. Otherwise a burst of connections could admit
// past `viewers:` while each waits for its accept.
func TestPendingViewerHoldsItsSlot(t *testing.T) {
	hub := newTuiViewerHub(1, false)
	c1, s1 := net.Pipe()
	defer c1.Close()
	if _, ok := hub.admitPending(s1); !ok {
		t.Fatal("first admitPending declined")
	}
	c2, s2 := net.Pipe()
	defer c2.Close()
	if _, ok := hub.admit(s2); ok {
		t.Error("a pending viewer must still fill the session")
	}
}

// goodbye closes a pending viewer's connection but does NOT send it the quit
// line: `quit` before `accept` is the same protocol violation a frame would be.
// The client sees EOF, which is the honest end of a connection that never
// completed its handshake.
func TestGoodbyeSkipsQuitForPendingViewer(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	cp, sp := net.Pipe()
	defer cp.Close()
	pendingLines := readLines(t, cp, 4)
	if _, ok := hub.admitPending(sp); !ok {
		t.Fatal("admitPending declined")
	}

	cl, sl := net.Pipe()
	defer cl.Close()
	liveLines := readLines(t, cl, 4)
	if _, ok := hub.admit(sl); !ok {
		t.Fatal("admit declined")
	}

	hub.goodbye()

	// The live viewer is told; the pending one just sees the socket close.
	if got := <-liveLines; !strings.Contains(got, `"tag":"quit"`) {
		t.Fatalf("live viewer got %s, want quit", got)
	}
	if got, open := <-pendingLines; open {
		t.Fatalf("pending viewer received %q instead of a close", got)
	}
	// The pending entry SURVIVES goodbye on purpose — see the next test.
	if len(hub.pending) != 1 {
		t.Errorf("goodbye must leave the pending entry for its caller's evict, got %v", hub.pending)
	}
}

// goodbye leaves a pending viewer in the maps so the caller's evict can still
// balance the reader token admitAs took. Deleting it there would make that
// evict a no-op, and teardown's readers.Wait() would block forever: an app
// exiting while a viewer is mid-attach would hang instead of returning.
//
// Codex flagged this on PR #411 and it was real. It is also older than the
// pending set — plain admit had the same shape — but nothing could FIX it
// before, because goodbye had no way to tell a viewer whose reader had started
// from one whose reader never would.
func TestGoodbyeLeavesPendingForEvictToBalance(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	c, s := net.Pipe()
	defer c.Close()
	id, ok := hub.admitPending(s)
	if !ok {
		t.Fatal("admitPending declined")
	}

	hub.goodbye()
	// The accept now fails on the closed connection — the server path's cue to
	// evict, exactly as it would for any failed accept.
	if err := hub.promote(id); err == nil {
		t.Error("promote must report the accept write failing on a closed conn")
	}
	hub.evict(id)

	done := make(chan struct{})
	go func() { hub.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readers.Wait() blocked — the pending viewer's reader token leaked")
	}
}

// evict — the accept-write-failed path — clears the pending mark along with
// the viewer, so a later id reuse cannot inherit a stale "do not broadcast".
func TestEvictClearsPending(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	c, s := net.Pipe()
	defer c.Close()
	id, ok := hub.admitPending(s)
	if !ok {
		t.Fatal("admitPending declined")
	}
	hub.evict(id)
	if len(hub.pending) != 0 {
		t.Errorf("evict left pending entries: %v", hub.pending)
	}
	hub.promote(id) // an evicted id promotes nothing: the no-op arm
}

// drop — the read-side EOF path — does the same.
func TestDropClearsPending(t *testing.T) {
	hub := newTuiViewerHub(2, true)
	c, s := net.Pipe()
	defer c.Close()
	id, ok := hub.admitPending(s)
	if !ok {
		t.Fatal("admitPending declined")
	}
	hub.drop(id)
	if len(hub.pending) != 0 {
		t.Errorf("drop left pending entries: %v", hub.pending)
	}
}
