package modules

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	parser "github.com/boru-lang/boru/parser/go"
)

// End-to-end coverage for the boru:net socket tier (net_socket.go) and
// codec tier (net_codec.go), over real loopback sockets on ephemeral
// ports (`{tcp: 0}` + `Net.addr`). Positive paths are paired with the
// negative contracts (timeout, closed, no codec, policy denial) per
// lang/go/CLAUDE.md.

func runNetSteps(t *testing.T, steps []string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.ParseFunc = parser.Parse
	InstallResolver(reg)
	engine := native.NewTop(reg)
	var result []native.Value
	for _, step := range steps {
		vals, pErr := parser.Parse(step)
		if pErr != nil {
			return nil, pErr
		}
		result, err = engine.Run(vals)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func lastString(t *testing.T, vals []native.Value) string {
	t.Helper()
	if len(vals) == 0 {
		t.Fatal("no result")
	}
	s, err := core.AsString(vals[len(vals)-1])
	if err != nil {
		t.Fatalf("expected String result, got %v", vals)
	}
	return s
}

// ---- Tier 1: raw sockets ----

// serve-raw echo: the NETWORK-SERVERS.0.md §1 low-level shape — raw
// bytes in, raw bytes out, framing by recv-until.
func TestNetServeRawEcho(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def nl (convert Bytes "\n")`,
		`def ln (Net.serve-raw {tcp: 0} ([conn:Any] => [
		   def line (Net.recv-until conn nl {within: 5000})
		   Net.send-bytes (line add nl) conn
		 ]))`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.send-bytes (convert Bytes "hello-raw\n") sock;`,
		`def reply (convert String (Net.recv-until sock nl {within: 5000}))`,
		`Net.close sock;`,
		`Net.close ln;`,
		`reply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, out); got != "hello-raw" {
		t.Errorf("echo reply = %q, want %q", got, "hello-raw")
	}
}

// recv-bytes reads exactly n; the listener side uses accept directly.
func TestNetListenAcceptRecvBytes(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def ln (Net.listen {tcp: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		// Server actor: accept one connection, read exactly 5 bytes,
		// send them back upper-reversed (just echo here), close.
		`spawn [
		   def conn (Net.accept ln)
		   def got (Net.recv-bytes conn 5 {within: 5000})
		   Net.send-bytes got conn;
		   Net.close conn;
		 ] drop`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.send-bytes (convert Bytes "12345") sock;`,
		`def reply (convert String (Net.recv-bytes sock 5 {within: 5000}))`,
		`Net.close sock;`,
		`Net.close ln;`,
		`reply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, out); got != "12345" {
		t.Errorf("recv-bytes reply = %q, want %q", got, "12345")
	}
}

// A recv deadline on a silent peer raises `timeout` — the slow-loris guard.
func TestNetRecvTimeoutRaises(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def ln (Net.listen {tcp: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.recv sock 10 {within: 60}`,
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("silent peer must raise timeout, got %v", err)
	}
}

// An accept deadline on an idle listener raises `timeout` — accept is
// never allowed to block a runner forever when {within} is given.
func TestNetAcceptTimeoutRaises(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def ln (Net.listen {tcp: 0})`,
		`Net.accept ln {within: 60}`,
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("idle listener must raise timeout, got %v", err)
	}
}

// accept with {within} still yields a pending connection, and the deadline
// is cleared afterwards: a second dial + plain accept on the SAME listener
// must not inherit the expired deadline.
func TestNetAcceptWithinAcceptsPending(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def ln (Net.listen {tcp: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		// The dial parks in the kernel backlog before accept runs.
		`def s1 (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`def c1 (Net.accept ln {within: 5000})`,
		// Second round: the {within} above must not linger on the listener.
		`def s2 (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`def c2 (Net.accept ln)`,
		`Net.send-bytes (convert Bytes "y") s2;`,
		`def got (convert String (Net.recv-bytes c2 1 {within: 5000}))`,
		`Net.close s1; Net.close c1; Net.close s2; Net.close c2; Net.close ln;`,
		`got`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, out); got != "y" {
		t.Errorf("second-round reply = %q, want %q", got, "y")
	}
}

// Reading from a peer-closed socket raises `closed`.
func TestNetRecvClosedRaises(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def ln (Net.serve-raw {tcp: 0} ([conn:Any] => [ Net.close conn ]))`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def sock (Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])})`,
		`Net.recv sock 10 {within: 5000}`,
	})
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("peer close must raise closed, got %v", err)
	}
}

// Dialing a dead port raises `transport`.
func TestNetConnectRefusedRaisesTransport(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		// Bind + close a listener to get a port that is now dead.
		`def ln (Net.listen {tcp: 0})`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`Net.close ln;`,
		`Net.connect-raw {tcp: (join "" ["127.0.0.1:" (convert String port)])}`,
	})
	if err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("refused dial must raise transport, got %v", err)
	}
}

func TestNetBadOptionsRejected(t *testing.T) {
	for _, src := range []string{
		`Net.listen {}`,                // no tcp:
		`Net.listen {tcp: -1}`,         // bad port
		`Net.listen {tcp: 99999}`,      // out of range
		`Net.connect-raw {tcp: 39001}`, // dial wants "host:port" String
		`Net.connect-raw {}`,           // no tcp:
	} {
		_, err := runNetSteps(t, []string{`import "boru:net"`, src})
		if err == nil || !strings.Contains(err.Error(), "net_error") {
			t.Errorf("%s: want net_error, got %v", src, err)
		}
	}
}

// ---- policy gating ----

func TestNetListenPolicyDeniedBySandbox(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkNetPolicy(reg, "listen", "listen", "", 8080); err == nil {
		t.Error("sandbox must deny network.listen")
	}
	if err := checkNetPolicy(reg, "connect-raw", "connect", "example.com", 443); err == nil {
		t.Error("sandbox must deny network.connect")
	}
}

func TestNetListenPolicyAllowedByFull(t *testing.T) {
	pol, err := policy.Load("full")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkNetPolicy(reg, "listen", "listen", "", 8080); err != nil {
		t.Errorf("full must allow network.listen: %v", err)
	}
}

// ---- Tier 2: codecs, listen {codec} svc, connect → Endpoint ----

func TestNetLinesCodecEndToEnd(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def echo-svc (service {})`,
		`add {} ([req:Map state:Any] => [ join "" ["<" req.line ">"] ]) echo-svc`,
		`def ln (Net.listen {tcp: 0 codec: Net.lines} echo-svc)`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.lines})`,
		`def reply ((call {line: "ping"} ep {timeout: 5000}).line)`,
		`Net.close ep;`,
		`Net.close ln;`,
		`reply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, out); got != "<ping>" {
		t.Errorf("lines echo = %q, want %q", got, "<ping>")
	}
}

func TestNetJSONLinesCodecEndToEnd(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def svc (service {})`,
		`add {op:"sum"} ([req:Map state:Any] => [ ({result: (add req.a req.b)}) ]) svc`,
		`def ln (Net.listen {tcp: 0 codec: Net.json-lines} svc)`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.json-lines})`,
		`def reply (call {op:"sum" a: 2 b: 3} ep {timeout: 5000})`,
		`Net.close ep;`,
		`Net.close ln;`,
		`reply.result`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	n, nErr := core.AsInteger(out[len(out)-1])
	if nErr != nil || n != 5 {
		t.Errorf("json-lines sum = %v, want 5", out)
	}
}

// The HTTP codec + router: patrun routes on method+path, `:id` templates
// surface req.params, unmatched paths become a 404 error reply.
func TestNetHTTPCodecWithRouteParams(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def api (service {})`,
		`add {method:"GET" path:"/hello"} ([req:Map state:Any] => [ {greeting: "hi"} ]) api`,
		`add {method:"GET" path:"/items/:id"} ([req:Map state:Any] => [ ({item: req.params.id}) ]) api`,
		`def ln (Net.listen {tcp: 0 codec: Net.http} api)`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.http})`,
		`def r1 (call {method:"GET" path:"/hello"} ep {timeout: 5000})`,
		`def r2 (call {method:"GET" path:"/items/42"} ep {timeout: 5000})`,
		`def r3 (call {method:"GET" path:"/nope"} ep {timeout: 5000})`,
		`Net.close ep;`,
		`Net.close ln;`,
		`join "," [(convert String r1.status) (convert String r1.body) (convert String r2.status) (convert String r2.body) (convert String r3.status)]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := lastString(t, out)
	for _, want := range []string{`200`, `"greeting"`, `"hi"`, `"item"`, `"42"`, `404`} {
		if !strings.Contains(got, want) {
			t.Errorf("http round-trip missing %q in %q", want, got)
		}
	}
}

// A custom codec written in boru — the §6.6 extension point: a trivial
// "shout" protocol (frames end in "!") defined as a plain {decode
// encode} map of lambdas.
func TestNetCustomBoruCodec(t *testing.T) {
	out, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`import "boru:string-util"`,
		`def bang (convert Bytes "!")`,
		`def shout-codec {
		   decode: ([buf:Any] => [
		     def txt (convert String buf)
		     def idx (StringUtil.indexof "!" txt)
		     if (idx gte 0) [
		       def head (convert String (slice 0 idx buf))
		       def tail (slice (add 1 idx) (size buf) buf)
		       {msg: {word: head} rest: tail}
		     ] [ {need: 1} ]
		   ])
		   encode: ([reply:Any] => [
		     def txt (if ((typeof reply) eq Map) [ reply.word ] [ reply ])
		     convert Bytes (join "" [txt "!"])
		   ])
		 }`,
		`def svc (service {})`,
		`add {} ([req:Map state:Any] => [ StringUtil.upper req.word ]) svc`,
		`def ln (Net.listen {tcp: 0 codec: shout-codec} svc)`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: shout-codec})`,
		`def reply ((call {word: "quiet"} ep {timeout: 5000}).word)`,
		`Net.close ep;`,
		`Net.close ln;`,
		`reply`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := lastString(t, out); got != "QUIET" {
		t.Errorf("custom codec reply = %q, want %q", got, "QUIET")
	}
}

// listen without a codec is refused — a wire protocol is never implicit.
func TestNetListenServiceRequiresCodec(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		`def svc (service {})`,
		`Net.listen {tcp: 0} svc`,
	})
	if err == nil || !strings.Contains(err.Error(), "codec") {
		t.Fatalf("listen without codec must be refused, got %v", err)
	}
}

// An endpoint call deadline on a stalled server raises `timeout`.
func TestNetEndpointCallTimeout(t *testing.T) {
	_, err := runNetSteps(t, []string{
		`import "boru:net"`,
		// A raw listener that accepts but never replies.
		`def ln (Net.serve-raw {tcp: 0} ([conn:Any] => [ Net.recv-bytes conn 999999 {within: 10000} ]))`,
		`def lna (Net.addr ln)`,
		`def port lna.port`,
		`def ep (Net.connect {tcp: (join "" ["127.0.0.1:" (convert String port)]) codec: Net.lines})`,
		`call {line: "x"} ep {timeout: 80}`,
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("stalled endpoint call must raise timeout, got %v", err)
	}
}
