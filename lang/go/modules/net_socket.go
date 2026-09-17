package modules

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Tier-1 low-level sockets for boru:net — the executable slice of
// design/NETWORK-SERVERS.0.md §4 and design/NETWORK-CLIENTS.0.md §4:
//
//	listen {tcp: <port> host: "…"}       -> Listener     (network.listen gated)
//	accept <Listener>                    -> Socket        (blocks)
//	connect-raw {tcp: "host:port"}       -> Socket        (network.connect gated)
//	serve-raw {tcp: <port>} [handler]    -> Listener      (accept-loop + per-conn actor)
//	recv <Socket> <n> ({within: ms})     -> Bytes         (up to n; n=0 → whatever arrives)
//	recv-bytes <Socket> <n> ({within})   -> Bytes         (exactly n)
//	recv-until <Socket> <delim> ({within}) -> Bytes       (through delim, delim stripped)
//	send-bytes <Bytes> <Socket>                            (write all)
//	shutdown <Socket> <half>                               ("read"/"write"/"both")
//	close <Socket|Listener|Endpoint>
//	peer <Socket>                        -> {host port}
//	peer-cert <Socket>                   -> the verified peer certificate, or None
//
// Passive (pull) reads only — active mode (`set-active`) is a deferred
// follow-on (design/legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore §1). Every recv*
// honours an optional `{within: <ms>}` deadline, raising `timeout`, so a
// slow peer cannot pin an actor forever. Peer disconnect raises `closed`
// — catch it with `do […] error [ case [ [get code eq "closed"] […] … ] ]`.

// TSocket / TListener are the opaque connection handles. FixedIDs
// 5009/5010 — next free in the 5000-band after Pid 5007 / Service 5008
// (5003/5005/5006 are retired). Pinned in fixedid_stability_test.go.
var (
	TSocket   = registerNetType("Ideal/Socket", 5009, socketBehavior{})
	TListener = registerNetType("Ideal/Listener", 5010, listenerBehavior{})
)

func registerNetType(path string, id int, b core.TypeBehavior) *core.Type {
	t, err := core.Builtin.RegisterType(path, id, "boru:net", b)
	if err != nil {
		native.RecordTypeInitError(fmt.Errorf("net: register %s: %w", path, err))
	}
	return t
}

// socketConn wraps one OS connection: a buffered reader (recv-until /
// recv-bytes need lookahead), separate read/write mutexes (full-duplex:
// one actor may stream out while another reads), and a closed latch.
type socketConn struct {
	id     string
	conn   net.Conn
	br     *bufio.Reader
	rmu    sync.Mutex
	wmu    sync.Mutex
	closed atomic.Bool
}

func newSocketValue(conn net.Conn) native.Value {
	sc := &socketConn{
		id:   native.GenerateID("SK_"),
		conn: conn,
		br:   bufio.NewReader(conn),
	}
	return core.NewExtension(TSocket, sc)
}

func asSocket(v native.Value) (*socketConn, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	sc, ok := ep.Body.(*socketConn)
	return sc, ok
}

func (sc *socketConn) close() error {
	if sc.closed.Swap(true) {
		return nil
	}
	return sc.conn.Close()
}

// netListener wraps a bound listen socket plus the id its Format shows.
type netListener struct {
	id string
	ln net.Listener
	// deadlineOn is what `accept {within: ms}` sets its deadline on. It
	// is normally ln itself, but a tls.NewListener wrapper does NOT
	// forward SetDeadline — without keeping the bound listener here,
	// turning on TLS would silently take {within:} away from accept.
	deadlineOn net.Listener
	// closed latches so double-close is a no-op and the serve-raw accept
	// loop can distinguish deliberate shutdown from accept errors.
	closed atomic.Bool
}

func newListenerValue(ln net.Listener) native.Value {
	return newListenerValueOn(ln, ln)
}

// newListenerValueOn is newListenerValue for a wrapped listener:
// `accept` reads from ln and sets deadlines on deadlineOn.
func newListenerValueOn(ln, deadlineOn net.Listener) native.Value {
	nl := &netListener{id: native.GenerateID("LS_"), ln: ln, deadlineOn: deadlineOn}
	return core.NewExtension(TListener, nl)
}

func asListener(v native.Value) (*netListener, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	nl, ok := ep.Body.(*netListener)
	return nl, ok
}

func (nl *netListener) close() error {
	if nl.closed.Swap(true) {
		return nil
	}
	return nl.ln.Close()
}

type socketBehavior struct{}

func (socketBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}
func (socketBehavior) Equal(a, b native.Value) bool {
	sa, oka := asSocket(a)
	sb, okb := asSocket(b)
	if !oka || !okb {
		return native.DefaultBehavior.Equal(a, b)
	}
	return sa == sb
}
func (socketBehavior) Format(v native.Value) string {
	if sc, ok := asSocket(v); ok {
		state := ""
		if sc.closed.Load() {
			state = " closed"
		}
		return "Socket<" + sc.id + state + ">"
	}
	return "Socket"
}

type listenerBehavior struct{}

func (listenerBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}
func (listenerBehavior) Equal(a, b native.Value) bool {
	la, oka := asListener(a)
	lb, okb := asListener(b)
	if !oka || !okb {
		return native.DefaultBehavior.Equal(a, b)
	}
	return la == lb
}
func (listenerBehavior) Format(v native.Value) string {
	if nl, ok := asListener(v); ok {
		return "Listener<" + nl.ln.Addr().String() + ">"
	}
	return "Listener"
}

// checkNetPolicy gates a socket word behind the network capability scope
// (NETWORK-SERVERS.0.md §9): op is "listen" (bind) or "connect" (dial).
// Mirrors fetch.go::checkFetchPolicy.
func checkNetPolicy(r *native.Registry, word, op, host string, port int) error {
	if r == nil {
		return nil
	}
	pol := native.HostPolicy(r)
	if pol == nil {
		return nil
	}
	args := policy.Args{"host": host, "port": port}
	if !pol.Installed("network") {
		return native.PolicyRefusal(r, word, &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "network",
			Op:      op,
			Profile: pol.Name(),
			Blame:   "network.install=false",
			Args:    args,
		})
	}
	return native.PolicyRefusal(r, word, pol.Check("network", op, args))
}

// ---- option parsing ----

// netAddrOpts reads a {tcp: … host: …} / {unix: …} options map. tcp: is
// an Integer port (listen) or a "host:port" String (dial); unix: a path.
type netAddrOpts struct {
	network string // "tcp" or "unix"
	addr    string // dial/bind address
	host    string // policy host (dial: remote host; listen: bind host)
	port    int
	// tlsProfile is the parsed `tls: {…}` sub-map on a DIAL; serverTLS
	// is its listen-side counterpart (only one is ever populated, chosen
	// by which side parseNetAddr was called for). hasTLS distinguishes
	// "no tls: key" from "tls: {}" (the latter means TLS with defaults,
	// which is the documented spelling in NETWORK-CLIENTS.0.md §5).
	tlsProfile capabilities.TLSProfile
	serverTLS  capabilities.ServerTLSProfile
	hasTLS     bool
}

// parseTLSSubMap reads the optional `tls:` key shared by the dial and
// bind paths, using the SAME parsers fetch uses so the surfaces cannot
// disagree about what an option means. `listening` picks the side: a
// listener presents a certificate and may demand one, a dial verifies
// one and may present one, and each rejects the other's keys by name.
func parseTLSSubMap(r *native.Registry, mp native.ReadMap, word string, listening bool, out *netAddrOpts) error {
	tv, ok := mp.Get("tls")
	if !ok {
		return nil
	}
	if listening {
		prof, err := native.ParseServerTLSOpts(r, tv, word)
		if err != nil {
			return err
		}
		out.serverTLS = prof
		out.hasTLS = true
		return nil
	}
	prof, err := native.ParseTLSOpts(r, tv, word)
	if err != nil {
		return err
	}
	out.tlsProfile = prof
	out.hasTLS = true
	return nil
}

func parseNetAddr(r *native.Registry, v native.Value, word string, listening bool) (netAddrOpts, error) {
	mp, err := native.RequireConcreteMap(v, word)
	if err != nil {
		return netAddrOpts{}, err
	}
	if uv, ok := mp.Get("unix"); ok {
		path, sErr := uv.AsConcreteString()
		if sErr != nil || path == "" {
			return netAddrOpts{}, r.BoruError("net_error", word+": unix: must be a socket path", word)
		}
		out := netAddrOpts{network: "unix", addr: path, host: "unix:" + path}
		// A unix socket can carry TLS too, and — more to the point —
		// silently DROPPING a tls: option here would be the same
		// mislead ParseTLSOpts rejects unknown keys to prevent.
		if err := parseTLSSubMap(r, mp, word, listening, &out); err != nil {
			return netAddrOpts{}, err
		}
		return out, nil
	}
	tv, ok := mp.Get("tcp")
	if !ok {
		return netAddrOpts{}, r.BoruError("net_error", word+": options need tcp: (or unix:)", word)
	}
	if listening {
		port, iErr := tv.AsConcreteInteger()
		if iErr != nil || port < 0 || port > 65535 {
			return netAddrOpts{}, r.BoruError("net_error", word+": tcp: must be a port Integer (0–65535)", word)
		}
		host := ""
		if hv, ok := mp.Get("host"); ok {
			host, _ = hv.AsConcreteString()
		}
		out := netAddrOpts{
			network: "tcp",
			addr:    net.JoinHostPort(host, strconv.FormatInt(port, 10)),
			host:    host,
			port:    int(port),
		}
		if err := parseTLSSubMap(r, mp, word, listening, &out); err != nil {
			return netAddrOpts{}, err
		}
		return out, nil
	}
	addr, sErr := tv.AsConcreteString()
	if sErr != nil || addr == "" {
		return netAddrOpts{}, r.BoruError("net_error", word+`: tcp: must be a "host:port" String`, word)
	}
	host, portStr, splitErr := net.SplitHostPort(addr)
	port := 0
	if splitErr == nil {
		port, _ = strconv.Atoi(portStr)
	} else {
		host = addr
	}
	out := netAddrOpts{network: "tcp", addr: addr, host: host, port: port}
	if err := parseTLSSubMap(r, mp, word, listening, &out); err != nil {
		return netAddrOpts{}, err
	}
	return out, nil
}

// recvDeadline reads an optional trailing `{within: <ms>}` options map.
func recvDeadline(args []native.Value, at int) (time.Duration, bool, error) {
	if len(args) <= at {
		return 0, false, nil
	}
	mp, _ := native.AsMap(args[at])
	if mp == nil {
		return 0, false, nil
	}
	v, ok := mp.Get("within")
	if !ok {
		return 0, false, nil
	}
	ms, err := v.AsConcreteInteger()
	if err != nil || ms < 0 {
		return 0, false, fmt.Errorf("within: must be a non-negative Integer (milliseconds)")
	}
	return time.Duration(ms) * time.Millisecond, true, nil
}

// mapNetErr converts a Go network error into the closed/timeout/transport
// error vocabulary the RFCs specify.
// mapNetErr delegates to the shared classifier in native so the socket
// words and fetch raise the SAME code for the same wire condition —
// see native.MapTransportErr and design/NETWORK-CLIENTS.0.md §8.2.
func mapNetErr(r *native.Registry, word string, err error) error {
	return native.MapTransportErr(r, word, err)
}

// ---- handlers ----

func listenHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	opts, err := parseNetAddr(r, args[0], "listen", true)
	if err != nil {
		return nil, err
	}
	if err := checkNetPolicy(r, "listen", "listen", opts.host, opts.port); err != nil {
		return nil, err
	}
	// Resolve the credential BEFORE binding: a listener bound and then
	// abandoned because the identity was misspelled leaves the port
	// held until GC, and the program sees a confusing second failure.
	var tlsCfg *tls.Config
	if opts.hasTLS {
		if pErr := native.CheckServerTLSPolicy(r, opts.serverTLS, opts.host, opts.port); pErr != nil {
			return nil, pErr
		}
		cfg, cErr := native.TLSServerConfigForProfile(r, opts.serverTLS, "listen")
		if cErr != nil {
			return nil, listenTLSErr(r, cErr)
		}
		tlsCfg = cfg
	}
	ln, lErr := net.Listen(opts.network, opts.addr)
	if lErr != nil {
		return nil, r.BoruError("transport", "listen: "+lErr.Error(), "listen")
	}
	if tlsCfg != nil {
		// TLS is a socket OPTION, not a separate API: the wrapped
		// listener still yields plain Sockets and every socket word
		// works on them unchanged (NETWORK-SERVERS.0.md §4.4). The bound
		// listener stays reachable so `accept {within:}` keeps working —
		// the TLS wrapper does not forward SetDeadline.
		return []native.Value{newListenerValueOn(tls.NewListener(ln, tlsCfg), ln)}, nil
	}
	return []native.Value{newListenerValue(ln)}, nil
}

// listenTLSErr codes the server-config failures. A coded error from the
// identity resolver passes through unchanged; the capabilities-package
// sentinels describe a malformed option, so they read as net_error
// rather than as a transport fault nobody wrote.
func listenTLSErr(r *native.Registry, err error) error {
	var coded *core.BoruError
	if errors.As(err, &coded) {
		return err
	}
	return r.BoruError("net_error", "listen: "+err.Error(), "listen")
}

func acceptHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	nl, ok := asListener(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "accept: expected a Listener, got "+args[0].Parent.String(), "accept")
	}
	within, has, dErr := recvDeadline(args, 1)
	if dErr != nil {
		return nil, r.BoruError("net_error", "accept: "+dErr.Error(), "accept")
	}
	if has {
		dl, canDeadline := nl.deadlineOn.(interface{ SetDeadline(time.Time) error })
		if !canDeadline {
			return nil, r.BoruError("net_error", "accept: this Listener does not support {within: ms}", "accept")
		}
		if sErr := dl.SetDeadline(time.Now().Add(within)); sErr != nil {
			return nil, mapNetErr(r, "accept", sErr)
		}
		// Clear the deadline once this accept resolves so later accepts on
		// the same Listener block indefinitely again (the documented default).
		defer func() { _ = dl.SetDeadline(time.Time{}) }()
	}
	conn, err := nl.ln.Accept()
	if err != nil {
		return nil, mapNetErr(r, "accept", err)
	}
	// Finish the handshake before the Socket escapes, mirroring the dial
	// side: a `recv` must never return plaintext from a peer that was
	// never authenticated, and a `require-client:` rejection belongs at
	// `accept` rather than surfacing later as an opaque read error.
	if hErr := tlsServerHandshake(conn, within, has); hErr != nil {
		_ = conn.Close()
		return nil, mapNetErr(r, "accept", hErr)
	}
	return []native.Value{newSocketValue(conn)}, nil
}

// tlsServerHandshake completes the server side of the handshake on an
// accepted connection, if it is a TLS one at all. `within` bounds it:
// without a deadline a peer that opens a connection and then says
// nothing would pin the acceptor indefinitely — the same denial the
// recv words' {within:} exists to prevent.
func tlsServerHandshake(conn net.Conn, within time.Duration, hasDeadline bool) error {
	tconn, ok := conn.(*tls.Conn)
	if !ok {
		return nil
	}
	if hasDeadline {
		// As on the dial side, SetDeadline's error is redundant: it
		// fails only on an already-closed connection, which Handshake
		// then reports properly and more specifically.
		_ = tconn.SetDeadline(time.Now().Add(within))
	}
	if err := tconn.Handshake(); err != nil {
		return err
	}
	if hasDeadline {
		_ = tconn.SetDeadline(time.Time{})
	}
	return nil
}

func connectRawHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	opts, err := parseNetAddr(r, args[0], "connect-raw", false)
	if err != nil {
		return nil, err
	}
	if err := checkNetPolicy(r, "connect-raw", "connect", opts.host, opts.port); err != nil {
		return nil, err
	}
	within, has, dErr := recvDeadline(args, 0) // within: may ride the same options map
	if dErr != nil {
		return nil, r.BoruError("net_error", "connect-raw: "+dErr.Error(), "connect-raw")
	}
	dialer := net.Dialer{}
	if has {
		dialer.Timeout = within
	}
	if opts.hasTLS {
		if pErr := native.CheckTLSPolicy(r, opts.tlsProfile, opts.host, opts.port); pErr != nil {
			return nil, pErr
		}
	}
	conn, cErr := dialer.Dial(opts.network, opts.addr)
	if cErr != nil {
		return nil, mapNetErr(r, "connect-raw", cErr)
	}
	if opts.hasTLS {
		tconn, tErr := tlsClientConn(r, conn, opts, within, has)
		if tErr != nil {
			_ = conn.Close()
			return nil, tErr
		}
		conn = tconn
	}
	return []native.Value{newSocketValue(conn)}, nil
}

// tlsClientConn wraps a dialled connection in a TLS client and completes
// the handshake before the Socket escapes, so a `recv` never returns
// plaintext from a connection whose peer was never verified. TLS is a
// socket OPTION, not a separate API (NETWORK-SERVERS.0.md §4.4): the
// returned Socket is the same type with the same words.
func tlsClientConn(r *native.Registry, conn net.Conn, opts netAddrOpts, within time.Duration, hasDeadline bool) (net.Conn, error) {
	cfg, err := native.TLSConfigForProfile(r, opts.tlsProfile, "connect-raw")
	if err != nil {
		return nil, err
	}
	if cfg.ServerName == "" && !cfg.InsecureSkipVerify {
		// Unlike http.Transport, a raw dial has no URL to derive SNI
		// and the verified name from — without this every TLS socket
		// would fail verification.
		cfg.ServerName = opts.host
	}
	tconn := tls.Client(conn, cfg)
	if hasDeadline {
		// SetDeadline's error is deliberately dropped: it fails only on
		// an already-closed connection, and this one was just dialled.
		// Were it somehow closed, Handshake below reports that properly
		// — checking here would only produce a second, less specific
		// error for the same condition.
		_ = tconn.SetDeadline(time.Now().Add(within))
	}
	if hErr := tconn.Handshake(); hErr != nil {
		return nil, mapNetErr(r, "connect-raw", hErr)
	}
	if hasDeadline {
		// The connect deadline bounds the handshake only; recv/send
		// carry their own {within:}. Same reasoning as above — a
		// failure here means a dead conn, which the next recv reports
		// as `closed`.
		_ = tconn.SetDeadline(time.Time{})
	}
	return tconn, nil
}

// serveRawHandler — `serve-raw {tcp: port} [handler]`: bind, then run an
// accept loop on its own goroutine, spawning one handler invocation per
// connection, each on its own forked registry, each recovered — one
// connection crashing dies alone (NETWORK-SERVERS.0.md §4.4).
func serveRawHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	handler := args[1]
	fnInfo, ok := native.FnDefFromValue(handler)
	if !ok {
		return nil, r.BoruError("net_error", "serve-raw: handler must be a function taking the connection Socket", "serve-raw")
	}
	lv, err := listenHandler(args[:1], nil, nil, r)
	if err != nil {
		return nil, err
	}
	nl, _ := asListener(lv[0])

	// Fork ON THE CALLER'S GOROUTINE (the ForkConcurrent contract); the
	// accept loop owns this fork and forks it again per connection.
	//
	// Fork from the handler's DEFINING registry, not from the caller's: the
	// handler body's free words resolve where it was WRITTEN
	// (design/FUNCTION-VALUE-SCOPE.0.md), so a `serve-raw` handler exported by
	// a module must see that module's private helpers. Forking is what makes
	// this safe to do here rather than just passing the module registry along —
	// the per-connection goroutines each get their own cloned Defs/Types and a
	// private context layer, so they never share mutable state with the module
	// or with each other. The fork inherits the DEFINING registry's
	// capabilities, which a module body already inherited from its importer at
	// load time, so this widens nothing.
	//
	// Writers stay the CALLER's (below): output belongs to the program that
	// started the server, not to whichever module supplied the handler.
	forkBase, _ := core.FnHome(r, fnInfo)
	acceptorFork := forkBase.ForkConcurrent()
	acceptorFork.Output = r.Output
	acceptorFork.ErrOutput = r.ErrOutput
	rt := r.Procs
	if rt != nil {
		acceptorFork.Output = rt.SyncOutput(r.Output)
		acceptorFork.ErrOutput = rt.SyncOutput(r.ErrOutput)
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil { //covergate:allow acceptor-goroutine recover body: the loop calls only Accept on a listener serveRawHandler itself minted (failure is an error at the covered 404 arm, never a panic) plus panic-free ForkConcurrent/newSocketValue (§native)
				fmt.Fprintf(acceptorFork.ErrOutput, "[boru/net] acceptor crashed: %v\n", rec)
			}
		}()
		for {
			conn, aErr := nl.ln.Accept()
			if aErr != nil {
				return // listener closed (deliberately or fatally) — loop ends
			}
			// The acceptor goroutine owns acceptorFork and is idle here,
			// so forking it per connection honours the fork contract.
			connFork := acceptorFork.ForkConcurrent()
			sock := newSocketValue(conn)
			go func() {
				defer func() {
					if rec := recover(); rec != nil { //covergate:allow per-connection recover body: MatchFnSig is nil-safe and CallBoru runs the handler in a NewTop sub-engine whose own recover converts body panics to internal_error BoruErrors, which flow through the covered error-logging arm (§native)
						fmt.Fprintf(connFork.ErrOutput, "[boru/net] connection handler crashed: %v\n", rec)
					}
					if sc, ok := asSocket(sock); ok {
						_ = sc.close()
					}
				}()
				// Handshake HERE, on the per-connection goroutine, not in
				// the accept loop above: a peer that connects and then
				// stalls must cost one goroutine, not the whole
				// acceptor. The handler therefore never sees a Socket
				// whose peer is unauthenticated.
				if hErr := tlsServerHandshake(conn, 0, false); hErr != nil {
					fmt.Fprintf(connFork.ErrOutput, "[boru/net] TLS handshake failed: %v\n", hErr)
					return
				}
				sig := native.MatchFnSig(handler, []native.Value{sock})
				if sig == nil {
					fmt.Fprintf(connFork.ErrOutput, "[boru/net] serve-raw handler does not accept a Socket argument\n")
					return
				}
				// Route through InvokeCallback: when the handler compiled to a
				// unit (the common capture-free case) and this per-connection fork
				// is idle, the body runs on the VM (RunUnit) — closing the ~19x
				// interpreter penalty the networking benchmark measured — else it
				// falls back to CallBoru, byte-identical.
				if _, hErr := core.InvokeCallbackFn(connFork, fnInfo, sig, []native.Value{sock}); hErr != nil {
					// `closed` is the normal end of a connection; anything
					// else is a real handler failure worth logging.
					var ae *core.BoruError
					if !errors.As(hErr, &ae) || ae.Code != "closed" {
						fmt.Fprintf(connFork.ErrOutput, "[boru/net] connection handler error: %v\n", hErr)
					}
				}
			}()
		}
	}()
	return lv, nil
}

func recvHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "recv: expected a Socket, got "+args[0].Parent.String(), "recv")
	}
	n, nErr := args[1].AsConcreteInteger()
	if nErr != nil || n < 0 {
		return nil, r.BoruError("net_error", "recv: byte count must be a non-negative Integer (0 = whatever is available)", "recv")
	}
	within, has, dErr := recvDeadline(args, 2)
	if dErr != nil {
		return nil, r.BoruError("net_error", "recv: "+dErr.Error(), "recv")
	}
	sc.rmu.Lock()
	defer sc.rmu.Unlock()
	if err := setReadDeadline(sc, within, has); err != nil {
		return nil, mapNetErr(r, "recv", err)
	}
	size := n
	if size == 0 {
		size = 4096
	}
	buf := make([]byte, size)
	read, err := sc.br.Read(buf)
	if read > 0 {
		return []native.Value{native.NewBytesValue(buf[:read])}, nil
	}
	if err != nil {
		return nil, mapNetErr(r, "recv", err)
	}
	return []native.Value{native.NewBytesValue(nil)}, nil
}

func recvBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "recv-bytes: expected a Socket, got "+args[0].Parent.String(), "recv-bytes")
	}
	n, nErr := args[1].AsConcreteInteger()
	if nErr != nil || n < 0 {
		return nil, r.BoruError("net_error", "recv-bytes: byte count must be a non-negative Integer", "recv-bytes")
	}
	within, has, dErr := recvDeadline(args, 2)
	if dErr != nil {
		return nil, r.BoruError("net_error", "recv-bytes: "+dErr.Error(), "recv-bytes")
	}
	sc.rmu.Lock()
	defer sc.rmu.Unlock()
	if err := setReadDeadline(sc, within, has); err != nil {
		return nil, mapNetErr(r, "recv-bytes", err)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(sc.br, buf); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, r.BoruError("closed", "recv-bytes: connection closed mid-frame", "recv-bytes")
		}
		return nil, mapNetErr(r, "recv-bytes", err)
	}
	return []native.Value{native.NewBytesValue(buf)}, nil
}

func recvUntilHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "recv-until: expected a Socket, got "+args[0].Parent.String(), "recv-until")
	}
	delim, ok := native.AsBytesValue(args[1])
	if !ok || len(delim) == 0 {
		return nil, r.BoruError("net_error", "recv-until: delimiter must be a non-empty Bytes (e.g. convert Bytes \"\\n\")", "recv-until")
	}
	within, has, dErr := recvDeadline(args, 2)
	if dErr != nil {
		return nil, r.BoruError("net_error", "recv-until: "+dErr.Error(), "recv-until")
	}
	sc.rmu.Lock()
	defer sc.rmu.Unlock()
	if err := setReadDeadline(sc, within, has); err != nil {
		return nil, mapNetErr(r, "recv-until", err)
	}
	var out []byte
	last := delim[len(delim)-1]
	for {
		chunk, err := sc.br.ReadBytes(last)
		out = append(out, chunk...)
		if err == nil && bytes.HasSuffix(out, delim) {
			return []native.Value{native.NewBytesValue(out[:len(out)-len(delim)])}, nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(out) > 0 {
				// Peer closed mid-line: surface `closed` — a framing loop
				// treats the partial tail as a broken frame.
				return nil, r.BoruError("closed", "recv-until: connection closed before delimiter", "recv-until")
			}
			return nil, mapNetErr(r, "recv-until", err)
		}
	}
}

func setReadDeadline(sc *socketConn, within time.Duration, has bool) error {
	if has {
		return sc.conn.SetReadDeadline(time.Now().Add(within))
	}
	return sc.conn.SetReadDeadline(time.Time{})
}

func sendBytesHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	data, ok := native.AsBytesValue(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "send-bytes: expected Bytes, got "+args[0].Parent.String(), "send-bytes")
	}
	sc, ok := asSocket(args[1])
	if !ok {
		return nil, r.BoruError("net_error", "send-bytes: expected a Socket, got "+args[1].Parent.String(), "send-bytes")
	}
	sc.wmu.Lock()
	defer sc.wmu.Unlock()
	if _, err := sc.conn.Write(data); err != nil {
		return nil, mapNetErr(r, "send-bytes", err)
	}
	return nil, nil
}

func shutdownHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "shutdown: expected a Socket, got "+args[0].Parent.String(), "shutdown")
	}
	half := native.ValToString(args[1])
	tcp, isTCP := sc.conn.(*net.TCPConn)
	if !isTCP {
		return nil, r.BoruError("net_error", "shutdown: half-close is only supported on TCP sockets", "shutdown")
	}
	var err error
	switch half {
	case "read":
		err = tcp.CloseRead()
	case "write":
		err = tcp.CloseWrite()
	case "both":
		err = sc.close()
	default:
		return nil, r.BoruError("net_error", `shutdown: half must be "read", "write" or "both"`, "shutdown")
	}
	if err != nil {
		return nil, mapNetErr(r, "shutdown", err)
	}
	return nil, nil
}

func closeHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	switch {
	case args[0].Parent.ConformsTo(TSocket):
		if sc, ok := asSocket(args[0]); ok {
			_ = sc.close()
			return nil, nil
		}
	case args[0].Parent.ConformsTo(TListener):
		if nl, ok := asListener(args[0]); ok {
			_ = nl.close()
			return nil, nil
		}
	case args[0].Parent.ConformsTo(native.TService):
		if closer := native.ServiceCloser(args[0]); closer != nil {
			_ = closer()
			return nil, nil
		}
		return nil, r.BoruError("net_error", "close: this Service is not a network endpoint", "close")
	}
	return nil, r.BoruError("net_error", "close: expected a Socket, Listener or Endpoint", "close")
}

func addrHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	nl, ok := asListener(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "addr: expected a Listener, got "+args[0].Parent.String(), "addr")
	}
	m := native.NewOrderedMap()
	host, portStr, err := net.SplitHostPort(nl.ln.Addr().String())
	if err != nil {
		m.Set("host", native.NewString(nl.ln.Addr().String()))
		m.Set("port", native.NewInteger(0))
	} else {
		port, _ := strconv.Atoi(portStr)
		m.Set("host", native.NewString(host))
		m.Set("port", native.NewInteger(int64(port)))
	}
	return []native.Value{native.NewMap(m)}, nil
}

func peerHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "peer: expected a Socket, got "+args[0].Parent.String(), "peer")
	}
	m := native.NewOrderedMap()
	addr := sc.conn.RemoteAddr()
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		m.Set("host", native.NewString(addr.String()))
		m.Set("port", native.NewInteger(0))
	} else {
		port, _ := strconv.Atoi(portStr)
		m.Set("host", native.NewString(host))
		m.Set("port", native.NewInteger(int64(port)))
	}
	return []native.Value{native.NewMap(m)}, nil
}

// peerCertHandler — `peer-cert <Socket>`: the verified peer certificate
// as a Map, or None when the connection is not TLS or the peer
// presented nothing.
//
// This is what makes `require-client:` useful rather than merely
// restrictive: the TLS layer proves the peer holds a key chaining to a
// trusted CA, and this word hands the resulting identity to boru so the
// AUTHORIZATION decision — which client may do what — can be written in
// the language rather than baked into the host.
//
// Only the VERIFIED chain is surfaced. `tls.ConnectionState.PeerCertificates`
// is populated even when verification was not required, so reading it
// unconditionally would hand a program an unauthenticated name that
// looks exactly like an authenticated one.
func peerCertHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	sc, ok := asSocket(args[0])
	if !ok {
		return nil, r.BoruError("net_error", "peer-cert: expected a Socket, got "+args[0].Parent.String(), "peer-cert")
	}
	tconn, isTLS := sc.conn.(*tls.Conn)
	if !isTLS {
		return []native.Value{native.NewNone()}, nil
	}
	state := tconn.ConnectionState()
	if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return []native.Value{native.NewNone()}, nil
	}
	return []native.Value{certValue(state.VerifiedChains[0][0])}, nil
}

// certValue projects an X.509 leaf onto the fields an authorization
// rule actually reads. Names stay Strings (a DN is text, and the
// distinguished-name grammar is not something boru should have to
// parse); validity is Instants so `TimeUtil.now` comparisons work; the
// serial is a String because it is a 20-octet integer that does not fit
// an Integer.
func certValue(cert *x509.Certificate) native.Value {
	m := native.NewOrderedMap()
	m.Set("subject", native.NewString(cert.Subject.String()))
	m.Set("common-name", native.NewString(cert.Subject.CommonName))
	m.Set("issuer", native.NewString(cert.Issuer.String()))
	m.Set("serial", native.NewString(cert.SerialNumber.String()))
	m.Set("not-before", native.NewInstant(cert.NotBefore))
	m.Set("not-after", native.NewInstant(cert.NotAfter))
	names := make([]native.Value, 0, len(cert.DNSNames))
	for _, n := range cert.DNSNames {
		names = append(names, native.NewString(n))
	}
	m.Set("dns-names", native.NewList(names))
	emails := make([]native.Value, 0, len(cert.EmailAddresses))
	for _, e := range cert.EmailAddresses {
		emails = append(emails, native.NewString(e))
	}
	m.Set("emails", native.NewList(emails))
	return native.NewMap(m)
}

// netNoReturns is the check-mode shape of the statement-form socket
// words (send-bytes / shutdown / close): they produce no values.
func netNoReturns(_ []native.Value, _ *native.Registry) []native.Value { return nil }

// netAddrMirror is the check-mode guaranteed-error mirror for a word
// whose options map is an ADDRESS: it runs parseNetAddr — the handler's
// own first step, and a pure one (it reads the map and formats an
// address; it opens nothing) — so a refusal decided by the literal
// options is reported at check time with the byte-identical code and
// detail. Reusing the handler's validator rather than restating its
// messages is the point: the two cannot drift, down to the en dash in
// "0–65535".
//
// The gate is DeepConcreteOptions, never plain concreteness: `{tcp: p}`
// for a computed p is a concrete Map whose field is a carrier, and
// parseNetAddr would read that carrier as a malformed port and flag a
// correct program. The corpus is full of such rows (`{tcp: (join …)}`),
// and so are the shipped example apps.
func netAddrMirror(word string, listening bool, result *native.Type) native.ReturnsFunc {
	return native.MirrorReturns(word, native.DeepConcreteOptions,
		func(args []native.Value, r *native.Registry) error {
			_, err := parseNetAddr(r, args[0], word, listening)
			return err
		},
		native.ReturnsStatic(result))
}

// socketNatives lists the Tier-1 words BuildNetModule registers.
func socketNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return []native.NativeFunc{
		{Name: "listen", Signatures: []native.Signature{
			{Args: T(native.TMap), Impl: native.Go(listenHandler), Returns: T(TListener),
				ReturnsFn: netAddrMirror("listen", true, TListener), BarrierPos: -1},
		}},
		{Name: "accept", Signatures: []native.Signature{
			{Args: T(TListener, native.TMap), Impl: native.Go(acceptHandler), Returns: T(TSocket), BarrierPos: -1},
			{Args: T(TListener), Impl: native.Go(acceptHandler), Returns: T(TSocket), BarrierPos: -1},
		}},
		{Name: "connect-raw", Signatures: []native.Signature{
			{Args: T(native.TMap), Impl: native.Go(connectRawHandler), Returns: T(TSocket),
				ReturnsFn: netAddrMirror("connect-raw", false, TSocket), BarrierPos: -1},
		}},
		{Name: "serve-raw", Signatures: []native.Signature{
			{Args: T(native.TMap, native.TAny), Impl: native.Go(serveRawHandler), Returns: T(TListener),
				BarrierPos: -1, CompileEffect: native.CompileStoresFn},
		}},
		{Name: "recv", Signatures: []native.Signature{
			{Args: T(TSocket, native.TInteger, native.TMap), Impl: native.Go(recvHandler), Returns: T(native.TBytes), BarrierPos: -1},
			{Args: T(TSocket, native.TInteger), Impl: native.Go(recvHandler), Returns: T(native.TBytes), BarrierPos: -1},
		}},
		{Name: "recv-bytes", Signatures: []native.Signature{
			{Args: T(TSocket, native.TInteger, native.TMap), Impl: native.Go(recvBytesHandler), Returns: T(native.TBytes), BarrierPos: -1},
			{Args: T(TSocket, native.TInteger), Impl: native.Go(recvBytesHandler), Returns: T(native.TBytes), BarrierPos: -1},
		}},
		{Name: "recv-until", Signatures: []native.Signature{
			{Args: T(TSocket, native.TBytes, native.TMap), Impl: native.Go(recvUntilHandler), Returns: T(native.TBytes), BarrierPos: -1},
			{Args: T(TSocket, native.TBytes), Impl: native.Go(recvUntilHandler), Returns: T(native.TBytes), BarrierPos: -1},
		}},
		{Name: "send-bytes", Signatures: []native.Signature{
			{Args: T(native.TBytes, TSocket), Impl: native.Go(sendBytesHandler), Returns: T(),
				ReturnsFn: netNoReturns, BarrierPos: -1},
		}},
		{Name: "shutdown", Signatures: []native.Signature{
			{Args: T(TSocket, native.TString), Impl: native.Go(shutdownHandler), Returns: T(),
				ReturnsFn: netNoReturns, BarrierPos: -1},
		}},
		{Name: "close", Signatures: []native.Signature{
			{Args: T(native.TAny), Impl: native.Go(closeHandler), Returns: T(),
				ReturnsFn: netNoReturns, BarrierPos: -1},
		}},
		{Name: "peer", Signatures: []native.Signature{
			{Args: T(TSocket), Impl: native.Go(peerHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
		{Name: "peer-cert", Signatures: []native.Signature{
			// The verified peer certificate, or None on a plain socket
			// (hence Any, not Map, in the return position).
			{Args: T(TSocket), Impl: native.Go(peerCertHandler), Returns: T(native.TAny), BarrierPos: -1},
		}},
		{Name: "addr", Signatures: []native.Signature{
			// addr <Listener> — the bound {host port} (an ephemeral `tcp: 0`
			// bind reads its real port here).
			{Args: T(TListener), Impl: native.Go(addrHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
	}
}
