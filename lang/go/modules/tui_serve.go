package modules

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// The remote tier of design/TUI.0.md §6 (implementation plan P4, v2
// session rules P6): `Tui.serve {tcp token viewers? reattach?} app`
// runs the SAME driver loop as Tui.run against the remote half of the
// renderer seam — widget trees go down the wire as json-lines (§6.2),
// decoded events come up, the attach client (cmd/go/internal/attach)
// lays out locally at ITS own geometry. Sessions admit up to `viewers`
// concurrent viewers (default 1 — the v1 rule); frames broadcast to
// all, input merges from all, and a frame identical to the previous
// one is not re-sent. Losing the LAST viewer quits the app unless
// `reattach: true`, in which case the app keeps running headless and a
// later viewer resumes with the title and current frame. Token auth is
// a constant-time compare with `token: "none"` as the explicit opt-out.

// Test seams (design/TEST-SEAMS.10.md).
var (
	tuiListen       = net.Listen
	tuiServeBound   = func(net.Addr) {}
	tuiWriteTimeout = 10 * time.Second
)

// tuiHandshakeTimeout bounds how long an attaching connection may take
// to present its handshake line.
const tuiHandshakeTimeout = 10 * time.Second

// tuiWireLimit bounds one json-line (a paste event or a large tree).
const tuiWireLimit = 4 << 20

// tuiMaxViewers caps the viewers: option.
const tuiMaxViewers = 64

// tuiAcceptLine is the handshake's accept reply. A package-level value because
// promote writes it while holding the hub lock (see promote) and there is
// nothing per-connection in it. It replaced a tuiWriteAccept helper that wrote
// with NO deadline; going through hub.writeTo gives it the same broadcast
// deadline every other hub write has, so a stuck client fails its accept
// instead of holding the lock forever.
var tuiAcceptLine = []byte(`{"tag":"accept","proto":1}` + "\n")

type tuiSession struct {
	cols int
	rows int
}

// tuiServeOpts is the parsed transport configuration.
type tuiServeOpts struct {
	port     int
	token    string
	viewers  int
	reattach bool
}

// tuiViewerHub owns a session's viewer set: admission up to max,
// frame/title broadcast with per-write deadlines, unchanged-frame
// suppression, late-joiner replay, and the last-viewer-gone verdict.
type tuiViewerHub struct {
	mu       sync.Mutex
	viewers  map[int]net.Conn
	nextID   int
	max      int
	reattach bool
	closed   bool // goodbye has run; no more admissions or writes
	// pending holds viewers that are ADMITTED but whose accept reply has not
	// landed yet. They occupy a slot and hold a reader token — they are part of
	// the session — but no broadcast may reach them, because the wire contract
	// is hello -> accept -> chrome and a title or frame arriving first is a
	// protocol violation the client rejects. See admitPending.
	pending   map[int]bool
	lastTree  []byte
	lastLine  []byte
	titleLine []byte
	seq       int
	events    chan tuikit.Event
	readers   sync.WaitGroup
}

func newTuiViewerHub(max int, reattach bool) *tuiViewerHub {
	return &tuiViewerHub{
		viewers: map[int]net.Conn{}, pending: map[int]bool{},
		max: max, reattach: reattach,
		events: make(chan tuikit.Event, 64),
	}
}

// admit registers a handshaken connection as immediately LIVE. The false
// return means the session is full (or over) — the caller denies busy. The
// reader's WaitGroup slot is taken here, under the same lock that guards
// closed, so teardown's Wait cannot race a late Add.
//
// admit does not write: the accept reply is the caller's, so the caller owns
// the failure path (evict + close). A server accepting a real connection wants
// admitPending instead — see the race note there.
func (h *tuiViewerHub) admit(conn net.Conn) (id int, ok bool) { return h.admitAs(conn, false) }

// admitPending is admit for a connection whose accept reply has NOT been
// written yet, and it exists because writing that reply outside the hub lock
// is a race:
//
//	id, ok := hub.admit(conn)          // publishes conn into h.viewers, UNLOCKS
//	...
//	if wErr := tuiWriteAccept(conn)    // writes with the lock released
//
// setTitle and broadcastFrame take that same lock and write to every
// connection in h.viewers, so between admit returning and the accept landing a
// concurrent broadcast can reach the freshly published socket first — and the
// client's next line is `title` or `frame` where the protocol requires
// `accept`. replay's own comment states the invariant this breaks: "It runs
// AFTER the accept reply so the wire stays hello -> accept -> chrome."
//
// The window is tiny, which is why it took CI contention to show:
// TestTuiServeAllViewersGoneQuits failed with `handshake = map[tag:title
// text:served]`, while 40 local runs and -race were all green.
//
// Writing the accept INSIDE admit would close the window and change admit's
// contract — three tests call it directly and assert it does not write, one of
// them pinning tuiWriteAccept's error path. So the connection is admitted
// PENDING instead: it holds its slot and its reader token, broadcasts skip it,
// and promote publishes it to the broadcast set once the accept has landed.
func (h *tuiViewerHub) admitPending(conn net.Conn) (id int, ok bool) { return h.admitAs(conn, true) }

func (h *tuiViewerHub) admitAs(conn net.Conn, pending bool) (id int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.viewers) >= h.max {
		return 0, false
	}
	h.nextID++
	id = h.nextID
	h.viewers[id] = conn
	if pending {
		h.pending[id] = true
	}
	h.readers.Add(1)
	return id, true
}

// promote writes a pending viewer's ACCEPT reply, publishes it to the
// broadcast set, and sends it the chrome it missed — the title and the current
// frame — so late joiners resume mid-session. All of it under ONE lock
// acquisition, which is the point: the wire stays hello → accept → chrome and
// no broadcast can interleave. A non-nil error is the accept write failing, and
// the caller evicts. An unknown id (already dropped or evicted) does nothing.
//
// THE ACCEPT IS WRITTEN HERE, NOT BY THE CALLER, and that placement is load
// bearing. Writing it outside the lock leaves a state the hub cannot observe —
// "accepted, not yet promoted" — and goodbye running in that window would treat
// an already-accepted viewer as unaccepted and close it with no `quit`. The
// attach client reads EOF after accept as "server closed the connection"
// (cmd/go/internal/attach/attack.go), so a shutdown racing a late attach would
// surface as a spurious CLI failure. Under the lock the state is binary: a
// viewer is pending iff its accept has not been written.
//
// Promoting a viewer admitted LIVE (plain admit) sends the chrome and no
// accept — it never had a handshake to answer. That keeps admit's contract and
// is why the direct-admit tests read the title as their first line.
func (h *tuiViewerHub) promote(id int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn, ok := h.viewers[id]
	if !ok {
		return nil
	}
	if h.pending[id] {
		if err := h.writeTo(conn, tuiAcceptLine); err != nil {
			return err // still pending, still in viewers: the caller's evict settles it
		}
		delete(h.pending, id)
	}
	if h.titleLine != nil {
		_ = h.writeTo(conn, h.titleLine)
	}
	if h.lastLine != nil {
		_ = h.writeTo(conn, h.lastLine)
	}
	return nil
}

// evict removes a viewer whose accept reply never landed — no reader
// ever started and no disconnect verdict fires (the viewer was never
// part of the session from the app's point of view).
func (h *tuiViewerHub) evict(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.viewers[id]; !ok {
		return
	}
	delete(h.viewers, id)
	delete(h.pending, id)
	h.readers.Done()
}

// writeTo writes one line with the broadcast deadline. Callers hold mu.
func (h *tuiViewerHub) writeTo(conn net.Conn, line []byte) error {
	_ = conn.SetWriteDeadline(time.Now().Add(tuiWriteTimeout))
	_, err := conn.Write(line)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

// drop removes a viewer (read-side EOF). When the LAST viewer leaves a
// non-reattach session, the driver hears the §6.3 disconnect.
func (h *tuiViewerHub) drop(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.viewers[id]; !ok {
		return
	}
	delete(h.viewers, id)
	delete(h.pending, id)
	if len(h.viewers) == 0 && !h.reattach && !h.closed {
		select {
		case h.events <- tuikit.Event{Tag: "__disconnect"}:
		default:
		}
	}
}

// broadcastFrame sends the marshaled tree to every live viewer (a
// failed or timed-out write drops that viewer). A tree identical to
// the previous frame is suppressed — stored but not sent. The returned
// alive is false when a non-reattach session has no viewers left.
func (h *tuiViewerHub) broadcastFrame(treeJSON []byte) (alive bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return true
	}
	if h.lastTree != nil && bytes.Equal(h.lastTree, treeJSON) {
		return h.reattach || len(h.viewers) > 0
	}
	h.seq++
	line := append([]byte(fmt.Sprintf(`{"tag":"frame","seq":%d,"tree":`, h.seq)), treeJSON...)
	line = append(line, '}', '\n')
	h.lastTree = treeJSON
	h.lastLine = line
	for id, conn := range h.viewers {
		if h.pending[id] {
			continue // no frame before its accept; promote replays this line
		}
		if err := h.writeTo(conn, line); err != nil {
			_ = conn.Close()
			delete(h.viewers, id)
			delete(h.pending, id)
		}
	}
	return h.reattach || len(h.viewers) > 0
}

// setTitle records the session title and sends it to current viewers.
func (h *tuiViewerHub) setTitle(text string) {
	data, _ := json.Marshal(map[string]any{"tag": "title", "text": text})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.titleLine = append(data, '\n')
	for id, conn := range h.viewers {
		if h.pending[id] {
			continue // no title before its accept; promote replays it
		}
		if err := h.writeTo(conn, h.titleLine); err != nil {
			_ = conn.Close()
			delete(h.viewers, id)
			delete(h.pending, id)
		}
	}
}

// goodbye ends the session: the quit line goes to every live viewer,
// every connection closes, and no further admissions or writes happen.
func (h *tuiViewerHub) goodbye() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	line := []byte(`{"tag":"quit"}` + "\n")
	for id, conn := range h.viewers {
		if h.pending[id] {
			_ = conn.Close()
			// A pending viewer's accept has provably not been written (promote
			// writes it under this lock), so `quit` would be the same protocol
			// violation a frame would be — closing is the honest end, and the
			// client sees EOF before any accept.
			//
			// Its entry STAYS in both maps. It still holds the reader token
			// admitAs took, and no wire reader was ever started for it, so the
			// only thing that can balance that token is the caller's evict
			// after its promote fails on this now-closed connection. Deleting
			// it here would make that evict a no-op and leave the token
			// outstanding forever — teardown's readers.Wait() would hang, and
			// an app exiting while a viewer attaches would never return.
			continue
		}
		_ = h.writeTo(conn, line)
		_ = conn.Close()
		delete(h.viewers, id)
	}
}

func tuiServeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	opts, err := tuiServeOptsOf(args[0], r)
	if err != nil {
		return nil, err
	}
	if err := checkNetPolicy(r, "serve", "listen", "", opts.port); err != nil {
		return nil, err
	}
	if err := checkTuiPolicy(r, "serve", "serve"); err != nil {
		return nil, err
	}
	app, err := parseTuiApp(args[1], "serve", r)
	if err != nil {
		return nil, err
	}

	rt := r.Procs
	if rt == nil {
		rt = core.NewProcessRuntime()
		r.Procs = rt
	}
	if _, taken := rt.Whereis(tuiRegisteredName); taken {
		return nil, r.BoruError("already_running", "serve: another TUI app already owns the terminal", "serve")
	}

	ln, lErr := tuiListen("tcp", fmt.Sprintf(":%d", opts.port))
	if lErr != nil {
		return nil, mapNetErr(r, "serve", lErr)
	}
	tuiServeBound(ln.Addr())

	hub := newTuiViewerHub(opts.viewers, opts.reattach)
	// the acceptor owns the listener: every authenticated handshake is
	// admitted up to the viewer cap; the rest are denied busy.
	firstCh := make(chan tuiSession, 1)
	go tuiAcceptor(hub, ln, opts.token, firstCh)

	session, ok := <-firstCh
	if !ok {
		return nil, r.BoruError("closed", "serve: listener closed before a viewer attached", "serve")
	}

	teardown := func() {
		_ = ln.Close()
		hub.goodbye()
		hub.readers.Wait()
	}

	proc := core.NewProcess(rt, core.DefaultMailboxBound, core.OverflowBlock)
	if err := rt.Insert(proc); err != nil {
		teardown()
		return nil, mapTuiErr(r, "serve", err)
	}
	if err := rt.RegisterName(tuiRegisteredName, proc); err != nil { //covergate:allow RegisterName fails only when the runtime is down, which the Insert call above already rejected; the guard covers the down-race between the two calls (§modules)
		proc.Close()
		teardown()
		return nil, mapTuiErr(r, "serve", err)
	}

	if app.opts.Title != "" {
		hub.setTitle(app.opts.Title)
	}

	d := &tuiDriver{
		reg: r, app: app, proc: proc,
		events: hub.events,
		paint:  tuiWirePaint(r, hub),
		// finish releases only the LISTENER: the goodbye below still
		// writes the quit line to live viewers on every exit path
		finish: func() { _ = ln.Close() },
		cols:   session.cols, rows: session.rows,
	}
	final, runErr := d.run()
	teardown()
	close(hub.events)
	rt.UnregisterName(tuiRegisteredName)
	proc.Close()
	if runErr != nil {
		return nil, runErr
	}
	return []native.Value{final}, nil
}

// tuiWirePaint is the remote half of the renderer seam: the view tree
// is NOT laid out here — it goes down the wire whole (§6.2) and each
// attach client renders it locally at its own size. The tree is still
// VALIDATED here (a throwaway render at the session's dims): a bad
// view raises bad_widget at the serving app, exactly like Tui.run,
// instead of a layout failure disconnecting every client. Losing the
// last viewer of a non-reattach session is the graceful §6.3
// disconnect-quit.
func tuiWirePaint(r *native.Registry, hub *tuiViewerHub) func(native.Value, int, int) error {
	return func(tree native.Value, cols, rows int) error {
		if _, rErr := tuikit.Render(native.ValueToAny(tree), cols, rows); rErr != nil {
			return r.BoruError("bad_widget", "serve: "+rErr.Error(), "serve")
		}
		data, mErr := json.Marshal(jsonReady(native.ValueToAny(tree)))
		if mErr != nil { //covergate:allow json.Marshal cannot fail on the jsonReady-projected tree shapes the renderer accepts; kept as the io seam's defensive twin (§modules)
			return mErr
		}
		if !hub.broadcastFrame(data) {
			return errTuiViewerGone
		}
		return nil
	}
}

// tuiServeOptsOf validates the transport options map.
func tuiServeOptsOf(v native.Value, r *native.Registry) (tuiServeOpts, error) {
	out := tuiServeOpts{viewers: 1}
	opts, err := native.RequireConcreteMap(v, "serve")
	if err != nil {
		return out, r.BoruError("tui_error", "serve: expected an options Map", "serve")
	}
	port, hasPort, pErr := nodePortOf(opts)
	if pErr != nil || !hasPort {
		return out, r.BoruError("tui_error", "serve: tcp: must be an Integer port", "serve")
	}
	out.port = port
	if out.token, err = tuiOptString(opts, "token"); err != nil {
		return out, r.BoruError("tui_error", "serve: "+err.Error(), "serve")
	}
	if out.token == "" {
		return out, r.BoruError("tui_error", `serve: token: is required (use token: "none" to opt out on a trusted network)`, "serve")
	}
	if vv, ok := opts.Get("viewers"); ok {
		n, nErr := vv.AsConcreteInteger()
		if nErr != nil || n < 1 || n > tuiMaxViewers {
			return out, r.BoruError("tui_error",
				fmt.Sprintf("serve: viewers: must be an Integer between 1 and %d", tuiMaxViewers), "serve")
		}
		out.viewers = int(n)
	}
	if out.reattach, err = tuiOptBoolean(opts, "reattach"); err != nil {
		return out, r.BoruError("tui_error", "serve: "+err.Error(), "serve")
	}
	return out, nil
}

// nodePortOf reads the tcp: port from the serve options.
func nodePortOf(mp native.ReadMap) (int, bool, error) {
	v, ok := mp.Get("tcp")
	if !ok {
		return 0, false, nil
	}
	n, err := v.AsConcreteInteger()
	if err != nil || n < 0 || n > 65535 {
		return 0, false, fmt.Errorf("tcp: must be a port number")
	}
	return int(n), true, nil
}

// tuiAcceptor handshakes connections forever: each authenticated
// attach is admitted up to the hub's cap and only THEN accepted on the
// wire (an unauthenticated probe learns nothing, a full session denies
// busy). It exits when the listener closes; the channel closes if no
// viewer ever completed an attach.
func tuiAcceptor(hub *tuiViewerHub, ln net.Listener, token string, firstCh chan<- tuiSession) {
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow acceptor recover body: the loop calls Accept on a listener this handler minted plus the panic-free handshake/hub helpers (§modules)
			_ = rec
		}
	}()
	sentFirst := false
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !sentFirst {
				close(firstCh)
			}
			return
		}
		cols, rows, why := tuiHandshake(conn, token)
		if why != "" {
			_ = tuiWriteDeny(conn, why)
			_ = conn.Close()
			continue
		}
		id, ok := hub.admitPending(conn)
		if !ok {
			_ = tuiWriteDeny(conn, "busy")
			_ = conn.Close()
			continue
		}
		if wErr := hub.promote(id); wErr != nil {
			hub.evict(id)
			_ = conn.Close()
			continue
		}
		go tuiWireReader(hub, id, conn)
		if !sentFirst {
			sentFirst = true
			firstCh <- tuiSession{cols: cols, rows: rows}
		}
	}
}

func tuiWriteDeny(conn net.Conn, why string) error {
	data, _ := json.Marshal(map[string]any{"tag": "deny", "why": why})
	_, err := conn.Write(append(data, '\n'))
	return err
}

// tuiHandshake validates one attach line WITHOUT replying — the accept
// goes out only after admission; the returned why is "" on success
// ("bad-token" / "proto" / "transport" otherwise).
func tuiHandshake(conn net.Conn, token string) (int, int, string) {
	_ = conn.SetReadDeadline(time.Now().Add(tuiHandshakeTimeout))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	if !scanner.Scan() {
		return 0, 0, "transport"
	}
	_ = conn.SetReadDeadline(time.Time{})
	var hello struct {
		Tag   string `json:"tag"`
		Token string `json:"token"`
		Cols  int    `json:"cols"`
		Rows  int    `json:"rows"`
		Proto int    `json:"proto"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &hello); err != nil || hello.Tag != "attach" {
		return 0, 0, "transport"
	}
	if hello.Proto != 1 {
		return 0, 0, "proto"
	}
	if token != "none" && subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) != 1 {
		return 0, 0, "bad-token"
	}
	if hello.Cols <= 0 {
		hello.Cols = 80
	}
	if hello.Rows <= 0 {
		hello.Rows = 24
	}
	return hello.Cols, hello.Rows, ""
}

// tuiWireReader decodes one viewer's json-lines into the shared event
// stream; on EOF or error the viewer is dropped (which becomes the
// driver's disconnect when it was the last of a non-reattach session).
func tuiWireReader(hub *tuiViewerHub, id int, conn net.Conn) {
	defer hub.readers.Done()
	defer func() {
		if rec := recover(); rec != nil { //covergate:allow wire-reader recover body: the loop only scans a connection and takes the hub mutex, both panic-free (§modules)
			_ = rec
		}
	}()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	for scanner.Scan() {
		var wire struct {
			Tag    string   `json:"tag"`
			Key    string   `json:"key"`
			Char   string   `json:"char"`
			Mods   []string `json:"mods"`
			Cols   int      `json:"cols"`
			Rows   int      `json:"rows"`
			Kind   string   `json:"kind"`
			X      int      `json:"x"`
			Y      int      `json:"y"`
			Text   string   `json:"text"`
			Gained bool     `json:"gained"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &wire); err != nil {
			continue // a malformed line is dropped, not fatal
		}
		switch wire.Tag {
		case "key", "resize", "mouse", "paste", "focus":
			// non-blocking: a storm sheds stale input rather than
			// stranding this goroutine after the driver exits
			select {
			case hub.events <- tuikit.Event{
				Tag: wire.Tag, Key: wire.Key, Char: wire.Char, Mods: wire.Mods,
				Cols: wire.Cols, Rows: wire.Rows, Kind: wire.Kind,
				X: wire.X, Y: wire.Y, Text: wire.Text, Gained: wire.Gained,
			}:
			default:
			}
		default:
			// unknown client messages are dropped
		}
	}
	hub.drop(id)
}

// tuiServeNatives lists the remote-tier words.
func tuiServeNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return native.SideEffecting([]native.NativeFunc{
		{Name: "serve", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TMap), Impl: native.Go(tuiServeHandler), Returns: T(native.TAny),
				ReturnsFn: tuiServeMirror(), BarrierPos: -1, CompileEffect: native.CompileStoresFn},
		}},
	})
}
