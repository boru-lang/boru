package modules

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Loopback coverage for the remote tier (tui_serve.go): a real
// ephemeral listener, a raw json-lines wire client, the full
// attach→frames→events→quit choreography, and the §6.3 denial rules —
// negatives paired throughout.

// serveApp quits on the "q" key carrying its state. The title rides in
// the app config (the SAME map shape Tui.run takes — §6.1): serve
// forwards it as a wire line instead of setting it locally.
const serveApp = `def app {
  init: {n: 41}
  title: "served"
  update: ([state:Map ev:Map] => [ case ev.key [
      "up" [ drop state set n (add state.n 1) ]
      "boom" [ drop raise "kaboom" ]
      "q"  [ drop Tui.quit state ]
      state ] ])
  view: ([state:Map] => [ Tui.text (convert String state.n) ])
}`

// startServe launches Tui.serve on its own registry/goroutine and
// reports the bound address plus the final-result channel.
func startServe(t *testing.T) (net.Addr, chan error, chan string) {
	t.Helper()
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }

	errs := make(chan error, 1)
	finals := make(chan string, 1)
	go func() {
		reg, rErr := native.DefaultRegistry()
		if rErr != nil {
			errs <- rErr
			return
		}
		out, sErr := runTuiStepsOn(t, reg, []string{
			`import "boru:tui"`, serveApp,
			`def final (Tui.serve {tcp: 0  token: "s3cret"} app)`,
			`convert String final.n`,
		})
		if sErr != nil {
			errs <- sErr
			return
		}
		finals <- func() string {
			s, _ := out[len(out)-1].AsConcreteString()
			return s
		}()
	}()
	select {
	case addr := <-bound:
		return addr, errs, finals
	case err := <-errs:
		t.Fatalf("serve failed before binding: %v", err)
		return nil, nil, nil
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
		return nil, nil, nil
	}
}

type wireClient struct {
	conn    net.Conn
	scanner *bufio.Scanner
}

func dialWire(t *testing.T, addr net.Addr) *wireClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), tuiWireLimit)
	return &wireClient{conn: conn, scanner: sc}
}

func (w *wireClient) send(t *testing.T, payload map[string]any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.conn.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func (w *wireClient) recv(t *testing.T) map[string]any {
	t.Helper()
	_ = w.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !w.scanner.Scan() {
		t.Fatalf("wire closed: %v", w.scanner.Err())
	}
	var msg map[string]any
	if err := json.Unmarshal(w.scanner.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestTuiServeLoopback(t *testing.T) {
	addr, errs, finals := startServe(t)
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 12, "rows": 3, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	// a second AUTHENTICATED viewer is denied busy at the default cap
	// of one (the busy verdict comes after the handshake, so an
	// unauthenticated probe learns nothing about the session)
	c2 := dialWire(t, addr)
	c2.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c2.recv(t); msg["tag"] != "deny" || msg["why"] != "busy" {
		t.Fatalf("second viewer = %v", msg)
	}
	c2.conn.Close()

	sawTitle, sawFrame := false, false
	// a malformed line and an unknown tag are dropped, not fatal
	if _, err := c.conn.Write([]byte("zoink\n")); err != nil {
		t.Fatal(err)
	}
	c.send(t, map[string]any{"tag": "mystery"})
	c.send(t, map[string]any{"tag": "key", "key": "up", "char": "", "mods": []string{}})
	c.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	for {
		msg := c.recv(t)
		switch msg["tag"] {
		case "title":
			sawTitle = msg["text"] == "served"
		case "frame":
			tree, ok := msg["tree"].(map[string]any)
			if !ok || tree["w"] != "text" {
				t.Fatalf("frame tree = %v", msg["tree"])
			}
			sawFrame = true
		case "quit":
			if !sawTitle || !sawFrame {
				t.Fatalf("missing traffic before quit: title=%v frame=%v", sawTitle, sawFrame)
			}
			select {
			case final := <-finals:
				if final != "42" {
					t.Fatalf("final = %s, want 42", final)
				}
			case err := <-errs:
				t.Fatalf("serve error: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("serve did not return")
			}
			return
		}
	}
}

func TestTuiServeDisconnectQuits(t *testing.T) {
	addr, errs, finals := startServe(t)
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	c.conn.Close() // the viewer vanishes → v1 disconnect-quits
	select {
	case final := <-finals:
		if final != "41" {
			t.Fatalf("final = %s, want the pre-disconnect state 41", final)
		}
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return on disconnect")
	}
}

func TestTuiServeDenials(t *testing.T) {
	addr, errs, _ := startServe(t)
	// wrong token
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "wrong", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["why"] != "bad-token" {
		t.Fatalf("bad token = %v", msg)
	}
	c.conn.Close()
	// wrong proto
	c = dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 9})
	if msg := c.recv(t); msg["why"] != "proto" {
		t.Fatalf("bad proto = %v", msg)
	}
	c.conn.Close()
	// garbage handshake
	c = dialWire(t, addr)
	if _, err := c.conn.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	if msg := c.recv(t); msg["why"] != "transport" {
		t.Fatalf("garbage handshake = %v", msg)
	}
	c.conn.Close()
	// a valid attach then quits the app so the goroutine ends
	c = dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("final handshake = %v", msg)
	}
	c.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	for {
		if msg := c.recv(t); msg["tag"] == "quit" {
			break
		}
	}
	c.conn.Close()
	select {
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	default:
	}
}

func TestTuiServeConfigAndPolicyArms(t *testing.T) {
	reg := tcReg(t)
	check := func(opts, app, want string) {
		t.Helper()
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
			`Tui.serve ` + opts + ` ` + app})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("serve %s = %v, want %q", opts, err, want)
		}
	}
	okApp := `{update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`
	check(`{token: "x"}`, okApp, "tcp: must be an Integer port")
	check(`{tcp: -1  token: "x"}`, okApp, "tcp: must be an Integer port")
	check(`{tcp: 0}`, okApp, "token: is required")
	check(`{tcp: 0  token: 5}`, okApp, "token: must be a String")
	check(`{tcp: 0  token: "x"}`, `{}`, "missing update")
	_, dErr := tuiServeHandler([]native.Value{native.NewTypeLiteral(native.TMap), native.NewMap(native.NewOrderedMap())}, nil, nil, reg)
	if dErr == nil || !strings.Contains(dErr.Error(), "expected an options Map") {
		t.Fatalf("type-literal opts = %v", dErr)
	}

	// sandbox denies the network scope (checked first) — a direct
	// handler call, since sandbox declines the module import itself
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	preg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	sopts := native.NewOrderedMap()
	sopts.Set("tcp", native.NewInteger(0))
	sopts.Set("token", native.NewString("x"))
	_, nErr := tuiServeHandler([]native.Value{native.NewMap(sopts), native.NewMap(native.NewOrderedMap())}, nil, nil, preg)
	// Coded by the gate; the scope rides in the detail rather than a field.
	var nAe *native.BoruError
	if !errors.As(nErr, &nAe) || nAe.Code != "capability_not_installed" ||
		!strings.Contains(nErr.Error(), "network") {
		t.Fatalf("sandbox serve = %v", nErr)
	}

	// a network-allowed, terminal-denied profile reaches the terminal gate
	split, err := policy.LoadInline(`{version: 1  name: "split"  scopes: {
	    network:  { words: { default: "allow" } }
	    terminal: { install: false }
	  }}`)
	if err != nil {
		t.Fatal(err)
	}
	sreg, err := native.DefaultRegistryWithPolicy(split)
	if err != nil {
		t.Fatal(err)
	}
	InstallResolver(sreg)
	_, tErr := runTuiStepsOn(t, sreg, []string{`import "boru:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	var tAe *native.BoruError
	if !errors.As(tErr, &tAe) || tAe.Code != "capability_not_installed" ||
		!strings.Contains(tErr.Error(), "terminal") {
		t.Fatalf("split-profile serve = %v", tErr)
	}

	// a squatted name rejects serve before it listens
	reg2 := tcReg(t)
	if reg2.Procs == nil {
		t.Fatal("no process runtime")
	}
	sq := newSquatter(t, reg2)
	defer sq.Close()
	_, aErr := runTuiStepsOn(t, reg2, []string{`import "boru:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	if aErr == nil || !strings.Contains(aErr.Error(), "already owns the terminal") {
		t.Fatalf("squatted serve = %v", aErr)
	}

	// a failing listener maps to the net error vocabulary
	oldListen := tuiListen
	t.Cleanup(func() { tuiListen = oldListen })
	tuiListen = func(string, string) (net.Listener, error) { return nil, errors.New("no sockets today") }
	_, lErr := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
		`Tui.serve {tcp: 0  token: "x"} ` + okApp})
	if lErr == nil || !strings.Contains(lErr.Error(), "no sockets today") {
		t.Fatalf("listen failure = %v", lErr)
	}
	tuiListen = oldListen

	// a listener that closes before any viewer attaches
	closingListen := func() {
		old := tuiListen
		defer func() { tuiListen = old }()
		oldBound := tuiServeBound
		defer func() { tuiServeBound = oldBound }()
		tuiServeBound = func(a net.Addr) {}
		tuiListen = func(network, addr string) (net.Listener, error) {
			ln, err := net.Listen(network, addr)
			if err != nil {
				return nil, err
			}
			_ = ln.Close() // sabotage: accept fails immediately
			return ln, nil
		}
		_, cErr := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
			`Tui.serve {tcp: 0  token: "x"} ` + okApp})
		if cErr == nil || !strings.Contains(cErr.Error(), "listener closed before a viewer attached") {
			t.Fatalf("closed listener = %v", cErr)
		}
	}
	closingListen()
}

// newSquatter registers a placeholder process under the tui name.
func newSquatter(t *testing.T, reg *native.Registry) interface{ Close() } {
	t.Helper()
	sq := core.NewProcess(reg.Procs, 4, core.OverflowDrop)
	if err := reg.Procs.Insert(sq); err != nil {
		t.Fatal(err)
	}
	if err := reg.Procs.RegisterName(tuiRegisteredName, sq); err != nil {
		t.Fatal(err)
	}
	return sq
}

// The wire paint against the viewer hub: frames carry an increasing
// seq, identical trees are suppressed, a viewer that stops reading is
// dropped on the write deadline, and the last-viewer verdict
// distinguishes reattach sessions.
func TestTuiWirePaintHub(t *testing.T) {
	hub := newTuiViewerHub(2, false)
	client, server := net.Pipe()
	defer client.Close()
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(client)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	if _, ok := hub.admit(server); !ok {
		t.Fatal("first admit declined")
	}
	reg, rErr := native.DefaultRegistry()
	if rErr != nil {
		t.Fatal(rErr)
	}
	paint := tuiWirePaint(reg, hub)
	// a tree the layout core rejects raises bad_widget HERE, at the
	// serving app, before any viewer sees a byte of it
	if err := paint(native.NewString("x"), 4, 2); err == nil || !strings.Contains(err.Error(), "bad_widget") {
		t.Fatalf("invalid tree = %v, want bad_widget", err)
	}
	if err := paint(wireTextTree("x"), 4, 2); err != nil {
		t.Fatal(err)
	}
	if got := <-lines; got != "{\"tag\":\"frame\",\"seq\":1,\"tree\":{\"text\":\"x\",\"w\":\"text\"}}" {
		t.Fatalf("first frame = %s", got)
	}
	// the identical tree is suppressed; the next change is seq 2
	if err := paint(wireTextTree("x"), 4, 2); err != nil {
		t.Fatal(err)
	}
	if err := paint(wireTextTree("y"), 4, 2); err != nil {
		t.Fatal(err)
	}
	if got := <-lines; got != "{\"tag\":\"frame\",\"seq\":2,\"tree\":{\"text\":\"y\",\"w\":\"text\"}}" {
		t.Fatalf("post-suppression frame = %s", got)
	}
}

// wireTextTree builds the minimal valid widget tree the wire paint's
// validation admits.
func wireTextTree(s string) native.Value {
	m := native.NewOrderedMap()
	m.Set("w", native.NewString("text"))
	m.Set("text", native.NewString(s))
	return native.NewMap(m)
}

// Losing viewers mid-write: the deadline drops a stuck viewer; a
// non-reattach session with none left reports viewer-gone, a reattach
// session keeps painting into storage and replays to a late joiner.
func TestTuiWirePaintViewerLoss(t *testing.T) {
	oldTimeout := tuiWriteTimeout
	t.Cleanup(func() { tuiWriteTimeout = oldTimeout })
	tuiWriteTimeout = 50 * time.Millisecond

	// a stuck viewer (nobody reads the pipe) times out and is dropped;
	// it was the last of a non-reattach session
	hub := newTuiViewerHub(1, false)
	c1, s1 := net.Pipe()
	defer c1.Close()
	if _, ok := hub.admit(s1); !ok {
		t.Fatal("admit declined")
	}
	reg, rErr := native.DefaultRegistry()
	if rErr != nil {
		t.Fatal(rErr)
	}
	if err := tuiWirePaint(reg, hub)(wireTextTree("z"), 4, 2); !errors.Is(err, errTuiViewerGone) {
		t.Fatalf("stuck last viewer = %v", err)
	}

	// the same loss under reattach keeps the session alive, headless
	hub2 := newTuiViewerHub(1, true)
	c2, s2 := net.Pipe()
	defer c2.Close()
	if _, ok := hub2.admit(s2); !ok {
		t.Fatal("admit declined")
	}
	paint2 := tuiWirePaint(reg, hub2)
	if err := paint2(wireTextTree("z"), 4, 2); err != nil {
		t.Fatalf("reattach stuck viewer = %v", err)
	}
	// headless paints keep storing; the title sticks too
	if err := paint2(wireTextTree("w"), 4, 2); err != nil {
		t.Fatalf("headless paint = %v", err)
	}
	hub2.setTitle("back")
	// a late joiner replays the title and the CURRENT frame
	c3, s3 := net.Pipe()
	defer c3.Close()
	lines3 := make(chan string, 4)
	go func() {
		sc := bufio.NewScanner(c3)
		for sc.Scan() {
			lines3 <- sc.Text()
		}
		close(lines3)
	}()
	id3, ok := hub2.admit(s3)
	if !ok {
		t.Fatal("late joiner declined")
	}
	hub2.promote(id3)
	if got := <-lines3; !strings.Contains(got, "\"text\":\"back\"") {
		t.Fatalf("replayed title = %s", got)
	}
	hub2.promote(0) // an unknown id replays nothing: the no-op arm
	if got := <-lines3; !strings.Contains(got, "\"tree\":{\"text\":\"w\"") {
		t.Fatalf("replayed frame = %s", got)
	}
	// a stuck viewer is dropped by a title broadcast as well: nobody
	// reads this pipe, so the deadline fires and the viewer is gone
	hub5 := newTuiViewerHub(1, false)
	c5t, s5t := net.Pipe()
	defer c5t.Close()
	if _, ok := hub5.admit(s5t); !ok {
		t.Fatal("admit declined")
	}
	hub5.setTitle("stuck")
	if hub5.broadcastFrame([]byte(`1`)) {
		t.Fatal("title broadcast did not drop the stuck viewer")
	}

	// hub bookkeeping arms: drop's disconnect verdict and idempotence,
	// goodbye idempotence, admissions after the session ends
	hub3 := newTuiViewerHub(1, false)
	c4, s4 := net.Pipe()
	defer c4.Close()
	id4, ok := hub3.admit(s4)
	if !ok {
		t.Fatal("admit declined")
	}
	hub3.drop(id4)
	select {
	case ev := <-hub3.events:
		if ev.Tag != "__disconnect" {
			t.Fatalf("last-drop event = %v", ev)
		}
	default:
		t.Fatal("last drop emitted no disconnect")
	}
	hub3.drop(id4) // already gone: the no-op arm
	hub3.goodbye()
	hub3.goodbye() // idempotent
	if _, ok := hub3.admit(s4); ok {
		t.Fatal("admit after goodbye succeeded")
	}
	// evict semantics: an accepted-then-evicted viewer leaves silently
	hub4 := newTuiViewerHub(1, false)
	c5, s5 := net.Pipe()
	defer c5.Close()
	id5, ok := hub4.admit(s5)
	if !ok {
		t.Fatal("admit declined")
	}
	hub4.evict(id5)
	select {
	case ev := <-hub4.events:
		t.Fatalf("evict emitted %v", ev)
	default:
	}
	hub4.evict(id5) // already gone: the no-op arm
	if !hub3.broadcastFrame([]byte("\"q\"")) {
		t.Fatal("closed-session paint should report alive")
	}
}

// serve on a runtime-less registry mints one before listening (the
// listen seam then fails so the test stays listener-free).
func TestTuiServeLazyRuntime(t *testing.T) {
	reg := tcReg(t)
	if _, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, serveApp}); err != nil {
		t.Fatal(err)
	}
	reg.Procs = nil
	oldListen := tuiListen
	t.Cleanup(func() { tuiListen = oldListen })
	tuiListen = func(string, string) (net.Listener, error) { return nil, errors.New("no sockets today") }
	_, sErr := runTuiStepsOn(t, reg, []string{`Tui.serve {tcp: 0  token: "x"} app`})
	if sErr == nil || !strings.Contains(sErr.Error(), "no sockets today") {
		t.Fatalf("lazy-runtime serve = %v", sErr)
	}
	if reg.Procs == nil {
		t.Fatal("serve did not create the process runtime")
	}
}

// a runtime shut down between the bind and the attach fails the
// process insert and releases the connection and listener.
func TestTuiServeInsertFailure(t *testing.T) {
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }
	reg := tcReg(t)
	rt := reg.Procs
	if rt == nil {
		t.Fatal("no process runtime")
	}
	errs := make(chan error, 1)
	go func() {
		_, sErr := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, serveApp,
			`Tui.serve {tcp: 0  token: "s3cret"} app`})
		errs <- sErr
	}()
	var addr net.Addr
	select {
	case addr = <-bound:
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
	}
	rt.Shutdown()
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	select {
	case sErr := <-errs:
		if sErr == nil || !strings.Contains(sErr.Error(), "shut down") {
			t.Fatalf("insert failure = %v", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not fail")
	}
}

// an error raised inside the app's update propagates out of serve.
func TestTuiServeAppErrorPropagates(t *testing.T) {
	addr, errs, _ := startServe(t)
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	c.send(t, map[string]any{"tag": "key", "key": "boom", "char": "", "mods": []string{}})
	select {
	case sErr := <-errs:
		if sErr == nil || !strings.Contains(sErr.Error(), "kaboom") {
			t.Fatalf("app error = %v", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not propagate the app error")
	}
}

// A view whose tree the layout core rejects fails AT THE SERVING APP:
// serve raises bad_widget exactly like Tui.run would, and the viewer
// gets the graceful quit line — not a mid-frame disconnect (client-side
// layout is about geometry freedom, never about skipping validation).
func TestTuiServeBadViewRaisesServerSide(t *testing.T) {
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }
	errs := make(chan error, 1)
	go func() {
		reg, rErr := native.DefaultRegistry()
		if rErr != nil {
			errs <- rErr
			return
		}
		_, sErr := runTuiStepsOn(t, reg, []string{
			`import "boru:tui"`,
			`def app {init: 0
			  update: ([s:Any e:Map] => [ if (e.tag eq "key") [ Tui.quit s ] [ s ] ])
			  view: ([s:Any] => [ {w: "zeppelin"} ])}`,
			`Tui.serve {tcp: 0 token: "s3cret"} app`,
		})
		errs <- sErr
	}()
	var addr net.Addr
	select {
	case addr = <-bound:
	case err := <-errs:
		t.Fatalf("serve failed before binding: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
	}
	c := dialWire(t, addr)
	defer c.conn.Close()
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	select {
	case sErr := <-errs:
		if sErr == nil || !strings.Contains(sErr.Error(), "bad_widget") {
			t.Fatalf("serve error = %v, want bad_widget", sErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not decline the bad view")
	}
	// the viewer's last line is the graceful goodbye, not a dead socket
	if msg := c.recv(t); msg["tag"] != "quit" {
		t.Fatalf("viewer finale = %v, want quit", msg)
	}
}

// tuiHandshake's transport arms and dimension defaulting, driven
// directly over a net.Pipe (the handshake only VALIDATES — accept is
// the acceptor's post-admission reply).
func TestTuiHandshakeDirect(t *testing.T) {
	// a hello with no dimensions defaults to 80×24
	client, server := net.Pipe()
	go func() {
		data, _ := json.Marshal(map[string]any{"tag": "attach", "token": "t", "proto": 1})
		_, _ = client.Write(append(data, '\n'))
	}()
	cols, rows, why := tuiHandshake(server, "t")
	if why != "" || cols != 80 || rows != 24 {
		t.Fatalf("defaulted handshake = %dx%d %q", cols, rows, why)
	}
	_ = client.Close()
	_ = server.Close()

	// a viewer that hangs up before sending anything
	client, server = net.Pipe()
	_ = client.Close()
	if _, _, why := tuiHandshake(server, "t"); why != "transport" {
		t.Fatalf("silent close = %q", why)
	}
	_ = server.Close()

	// the accept reply failing to land evicts silently — driven through
	// the ENGINE path by a scripted listener whose conn declines writes
	oldListen := tuiListen
	t.Cleanup(func() { tuiListen = oldListen })
	hello, _ := json.Marshal(map[string]any{"tag": "attach", "token": "x", "proto": 1})
	tuiListen = func(string, string) (net.Listener, error) {
		return &scriptedListener{conns: []net.Conn{
			&scriptedConn{in: append(hello, '\n')},
		}}, nil
	}
	_, sErr := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
		`Tui.serve {tcp: 0  token: "x"} {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`})
	if sErr == nil || !strings.Contains(sErr.Error(), "listener closed before a viewer attached") {
		t.Fatalf("accept-write failure = %v", sErr)
	}
}

// scriptedListener serves a fixed set of connections then errors.
type scriptedListener struct {
	conns []net.Conn
	next  int
}

func (l *scriptedListener) Accept() (net.Conn, error) {
	if l.next >= len(l.conns) {
		return nil, errors.New("scripted listener exhausted")
	}
	c := l.conns[l.next]
	l.next++
	return c, nil
}
func (l *scriptedListener) Close() error   { return nil }
func (l *scriptedListener) Addr() net.Addr { return &net.TCPAddr{} }

// scriptedConn delivers fixed input and declines every write.
type scriptedConn struct {
	in  []byte
	off int
}

func (c *scriptedConn) Read(b []byte) (int, error) {
	if c.off >= len(c.in) {
		return 0, errors.New("scripted conn drained")
	}
	n := copy(b, c.in[c.off:])
	c.off += n
	return n, nil
}
func (c *scriptedConn) Write([]byte) (int, error)        { return 0, errors.New("write declined") }
func (c *scriptedConn) Close() error                     { return nil }
func (c *scriptedConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *scriptedConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *scriptedConn) SetDeadline(time.Time) error      { return nil }
func (c *scriptedConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptedConn) SetWriteDeadline(time.Time) error { return nil }

// startServeOpts is startServe with extra transport options spliced in.
func startServeOpts(t *testing.T, extra string) (net.Addr, chan error, chan string) {
	t.Helper()
	oldBound := tuiServeBound
	t.Cleanup(func() { tuiServeBound = oldBound })
	bound := make(chan net.Addr, 1)
	tuiServeBound = func(a net.Addr) { bound <- a }

	errs := make(chan error, 1)
	finals := make(chan string, 1)
	go func() {
		reg, rErr := native.DefaultRegistry()
		if rErr != nil {
			errs <- rErr
			return
		}
		out, sErr := runTuiStepsOn(t, reg, []string{
			`import "boru:tui"`, serveApp,
			`def final (Tui.serve {tcp: 0  token: "s3cret"` + extra + `} app)`,
			`convert String final.n`,
		})
		if sErr != nil {
			errs <- sErr
			return
		}
		finals <- func() string {
			s, _ := out[len(out)-1].AsConcreteString()
			return s
		}()
	}()
	select {
	case addr := <-bound:
		return addr, errs, finals
	case err := <-errs:
		t.Fatalf("serve failed before binding: %v", err)
		return nil, nil, nil
	case <-time.After(5 * time.Second):
		t.Fatal("serve never bound")
		return nil, nil, nil
	}
}

func attachViewer(t *testing.T, addr net.Addr) *wireClient {
	t.Helper()
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 10, "rows": 3, "proto": 1})
	if msg := c.recv(t); msg["tag"] != "accept" {
		t.Fatalf("handshake = %v", msg)
	}
	return c
}

// recvUntil reads until a message with the tag arrives, returning it.
func (w *wireClient) recvUntil(t *testing.T, tag string) map[string]any {
	t.Helper()
	for {
		msg := w.recv(t)
		if msg["tag"] == tag {
			return msg
		}
	}
}

// Two concurrent viewers: frames broadcast to both, input merges from
// both, a third is denied busy, one leaving does NOT end the session,
// and the survivor's quit ends it for everyone.
func TestTuiServeMultiViewer(t *testing.T) {
	addr, errs, finals := startServeOpts(t, `  viewers: 2`)
	a := attachViewer(t, addr)
	defer a.conn.Close()
	// A sees the init frame (41) before B joins
	if msg := a.recvUntil(t, "frame"); msg["seq"] != float64(1) {
		t.Fatalf("A init frame = %v", msg)
	}
	b := attachViewer(t, addr)
	defer b.conn.Close()
	// the late joiner replays the CURRENT frame immediately
	if msg := b.recvUntil(t, "frame"); msg["seq"] != float64(1) {
		t.Fatalf("B replay frame = %v", msg)
	}
	// a third authenticated viewer is over the cap
	c := dialWire(t, addr)
	c.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 8, "rows": 2, "proto": 1})
	if msg := c.recv(t); msg["why"] != "busy" {
		t.Fatalf("third viewer = %v", msg)
	}
	c.conn.Close()
	// B's keystroke reaches the app; the new frame reaches BOTH viewers
	b.send(t, map[string]any{"tag": "key", "key": "up", "char": "", "mods": []string{}})
	am := a.recvUntil(t, "frame")
	bm := b.recvUntil(t, "frame")
	if am["seq"] != float64(2) || bm["seq"] != float64(2) {
		t.Fatalf("broadcast seqs = %v / %v", am["seq"], bm["seq"])
	}
	// A leaves; the session continues for B
	a.conn.Close()
	b.send(t, map[string]any{"tag": "key", "key": "up", "char": "", "mods": []string{}})
	if msg := b.recvUntil(t, "frame"); msg["seq"] != float64(3) {
		t.Fatalf("post-departure frame = %v", msg)
	}
	// B quits the app; the final state carries both keystrokes
	b.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	_ = b.recvUntil(t, "quit")
	select {
	case final := <-finals:
		if final != "43" {
			t.Fatalf("final = %s, want 43", final)
		}
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return")
	}
}

// Losing every viewer of a multi-viewer session (no reattach) quits
// the app with the current state.
func TestTuiServeAllViewersGoneQuits(t *testing.T) {
	addr, errs, finals := startServeOpts(t, `  viewers: 2`)
	a := attachViewer(t, addr)
	b := attachViewer(t, addr)
	a.conn.Close()
	b.conn.Close()
	select {
	case final := <-finals:
		if final != "41" {
			t.Fatalf("final = %s, want 41", final)
		}
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return when the last viewer left")
	}
}

// A reattach session survives losing its only viewer; the next viewer
// resumes with the title and current frame and can quit the app.
func TestTuiServeReattach(t *testing.T) {
	addr, errs, finals := startServeOpts(t, `  reattach: true`)
	a := attachViewer(t, addr)
	if msg := a.recvUntil(t, "frame"); msg["seq"] != float64(1) {
		t.Fatalf("init frame = %v", msg)
	}
	// advance the state so the resumed frame is distinguishable
	a.send(t, map[string]any{"tag": "key", "key": "up", "char": "", "mods": []string{}})
	if msg := a.recvUntil(t, "frame"); msg["seq"] != float64(2) {
		t.Fatalf("pre-detach frame = %v", msg)
	}
	a.conn.Close() // the app stays alive, headless

	// the hub learns of the detach from its reader's EOF, so an instant
	// re-attach can race it into a busy deny — retry like a real client
	var b *wireClient
	for i := 0; ; i++ {
		b = dialWire(t, addr)
		b.send(t, map[string]any{"tag": "attach", "token": "s3cret", "cols": 10, "rows": 3, "proto": 1})
		msg := b.recv(t)
		if msg["tag"] == "accept" {
			break
		}
		b.conn.Close()
		if msg["tag"] != "deny" || msg["why"] != "busy" || i > 40 {
			t.Fatalf("reattach handshake = %v", msg)
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer b.conn.Close()
	// the resumed viewer replays the title and the CURRENT frame
	if msg := b.recvUntil(t, "title"); msg["text"] != "served" {
		t.Fatalf("replayed title = %v", msg)
	}
	msg := b.recvUntil(t, "frame")
	if msg["seq"] != float64(2) || msg["tree"].(map[string]any)["text"] != "42" {
		t.Fatalf("replayed frame = %v", msg)
	}
	b.send(t, map[string]any{"tag": "key", "key": "q", "char": "q", "mods": []string{}})
	_ = b.recvUntil(t, "quit")
	select {
	case final := <-finals:
		if final != "42" {
			t.Fatalf("final = %s, want 42", final)
		}
	case err := <-errs:
		t.Fatalf("serve error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return")
	}
}

// The v2 transport options are validated like the v1 ones.
func TestTuiServeV2ConfigArms(t *testing.T) {
	check := func(opts, want string) {
		t.Helper()
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
			`Tui.serve ` + opts + ` {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("serve %s = %v, want %q", opts, err, want)
		}
	}
	check(`{tcp: 0  token: "x"  viewers: 0}`, "viewers: must be an Integer between 1 and 64")
	check(`{tcp: 0  token: "x"  viewers: 65}`, "viewers: must be an Integer between 1 and 64")
	check(`{tcp: 0  token: "x"  viewers: "two"}`, "viewers: must be an Integer between 1 and 64")
	check(`{tcp: 0  token: "x"  reattach: 5}`, "reattach: must be a Boolean")
}
